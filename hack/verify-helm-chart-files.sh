#!/bin/bash

# Copyright 2021 The Kubernetes Authors.
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

set -euo pipefail

echo "begin to verify chart tgz files ..."
git config core.filemode false

# verify whether chart config has changed
diff=`git diff`
if [[ -n "${diff}" ]]; then
  echo "${diff}"
  exit 1
fi

for dir in charts/*
do
  if [ -d $dir ]; then
    if [ -f $dir/*.tgz ]; then
      echo "verify $dir ..."
      tar -xvf $dir/*.tgz -C $dir/
    fi
  fi
done

diff=`git diff`
if [[ -n "${diff}" ]]; then
  echo
  echo
  echo "${diff}"
  echo
  echo "latest chart config has changed, pls run \"helm package charts/latest/csi-driver-nfs -d charts/latest/\" to update tgz file"
  exit 1
fi

echo "chart tgz files verified."

echo "verify helm chart index ..."
curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash
helm repo add csi-driver-smb https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/charts
helm search repo -l csi-driver-nfs
echo "helm chart index verified."
