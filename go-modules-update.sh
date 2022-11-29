#!/bin/sh -x

# Copyright 2023 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# Usage: go-modules-update.sh
#
# Batch update dependencies for sidecars.
#
# Required environment variables
# CSI_RELEASE_TOKEN: Github token needed for generating release notes
# GITHUB_USER: Github username to create PRs with
#
# Instructions:
# 1. Login with "gh auth login"
# 2. Copy this script to the kubernetes-csi directory (one directory above the
# repos)
# 3. Update the repos and branches
# 4. Set environment variables
# 5. Run script from the kubernetes-csi directory
#
# Caveats:
# - This script doesn't handle interface incompatibility of updates.


die () {
    echo >&2 "$@"
    exit 1
}

MAX_RETRY=10

# Get the options
while getopts ":u:v:" option; do
   case $option in
      u) # Set username
         username=$OPTARG;;
      v) # Set version
         v=$OPTARG;;
     \?) # Invalid option
         echo "Error: Invalid option: $OPTARG"
         exit;;
   esac
done

# Only need to do this once
gh auth login || die "gh auth login failed"

while read -r repo branches; do
    if [ "$repo" != "#" ]; then
    (
        cd "$repo" || die "$repo: does not exit"
        git fetch origin || die "$repo: git fetch"
        for i in $branches; do
            if [ "$(git rev-parse --verify "module-update-$i" 2>/dev/null)" ]; then
                git checkout master && git branch -d "module-update-$i"
            fi
            git checkout -B "module-update-$i" "origin/$i" || die "$repo:$i checkout"
            rm -rf .git/MERGE*
            if ! git subtree pull --squash --prefix=release-tools https://github.com/kubernetes-csi/csi-release-tools.git master; then
                # Sometimes "--squash" leads to merge conflicts. Because we know that "release-tools"
                # is an unmodified copy of csi-release-tools, we can automatically resolve that
                # by replacing it completely.
                if [ -e .git/MERGE_MSG ] && [ -e .git/FETCH_HEAD ] && grep -q "^# Conflict" .git/MERGE_MSG; then
                    rm -rf release-tools
                    mkdir release-tools
                    git archive FETCH_HEAD  | tar -C release-tools -xf - || die "failed to re-create release-tools from FETCH_HEAD"
                    git add release-tools || die "add release-tools"
                    git commit --file=.git/MERGE_MSG || die "commit squashed release-tools"
                else
                    die "git subtree pull --squash failed, cannot reover."
                fi
            fi
            RETRY=0
            while ! ./release-tools/go-get-kubernetes.sh -p "$v" && RETRY < $MAX_RETRY
                do
                  RETRY=$((RETRY+1))
                  go mod tidy && go mod vendor && go mod tidy
            done   
            go mod tidy && go mod vendor && go mod tidy || die "last go mod vendor && go mod tidy failed"
            git add --all || die "git add -all failed"
            git commit -m "Update dependency go modules for k8s v$v" || die "commit update modules"
            git remote set-url origin "https://github.com/$username/$repo.git" || die "git remote set-url failed"
            make test || die "$repo:$i make test"
            git push origin "module-update-$i" --force || die "origin:module-update-$i push failed - probably there is already an unmerged branch and pending PR"
            # Create PR
prbody=$(cat <<EOF
Ran kubernetes-csi/csi-release-tools go-get-kubernetes.sh -p ${v}.


\`\`\`release-note
Update kubernetes dependencies to v${v}
\`\`\`
EOF
)
            gh pr create --title="Update dependency go modules for k8s v$v" --body "$prbody"  --head "$username:module-update-master" --base "master" --repo="kubernetes-csi/$repo"
        done  
    ) || die "failed"
    fi
done <<EOF
csi-driver-host-path master
csi-driver-iscsi master
csi-driver-nfs master
csi-lib-utils master
csi-proxy master
csi-test master
external-attacher master
external-health-monitor master
external-provisioner master
external-resizer master
external-snapshotter master
livenessprobe master
node-driver-registrar master
EOF
