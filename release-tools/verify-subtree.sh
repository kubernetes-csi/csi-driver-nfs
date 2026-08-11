#! /bin/sh -e

# Copyright 2019 The Kubernetes Authors.
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

# Verify that DIR is a clean copy of upstream csi-release-tools.
#
# We read the upstream SHA from the "git-subtree-split" line that
# "git subtree" writes into the pull commit's message, fetch that
# SHA, and require its tree to equal HEAD:DIR.

DIR="$1"
if [ -z "$DIR" ]; then
    echo "usage: $0 <directory>" >&2
    exit 1
fi

# csi-release-tools itself does not contain release-tools/.
[ -d "$DIR" ] || exit 0

UPSTREAM_URL="${UPSTREAM_URL:-https://github.com/kubernetes-csi/csi-release-tools.git}"

REV=$(git log -1 --grep="^git-subtree-dir: $DIR\$" --format=%H)
if [ -z "$REV" ]; then
    echo "No subtree pull recorded for '$DIR'." >&2
    exit 1
fi

# A squashed PR with multiple pulls carries one trailer block per pull; the last one is HEAD.
SPLIT=$(git log -1 --format=%B "$REV" | grep '^git-subtree-split: ' | tail -1 | cut -d' ' -f2)
if [ -z "$SPLIT" ]; then
    echo "Subtree commit $REV has no git-subtree-split trailer." >&2
    exit 1
fi

git fetch --quiet --no-tags "$UPSTREAM_URL" "$SPLIT"

if [ "$(git rev-parse "HEAD:$DIR")" = "$(git rev-parse "$SPLIT^{tree}")" ]; then
    echo "$DIR matches upstream at $SPLIT."
    exit 0
fi

echo "$DIR diverges from upstream at $SPLIT:" >&2
git diff --stat "$SPLIT" "HEAD:$DIR" >&2
exit 1
