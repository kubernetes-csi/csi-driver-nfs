#!/bin/bash

# Copyright 2020 The Kubernetes Authors.
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

if [[ "$#" -lt 1 ]]; then
  echo "please provide a registry name"  
  exit 1
fi

export REGISTRY_NAME="$1"
export REGISTRY=$REGISTRY_NAME.azurecr.io
export IMAGE_NAME=public/k8s/csi/smb-csi
export CI=1
export PUBLISH=1
az acr login --name $REGISTRY_NAME
make smb-container
make push
make push-latest

echo "sleep 60s ..."
sleep 60
image="mcr.microsoft.com/k8s/csi/smb-csi:latest"
docker pull $image
docker inspect $image | grep Created
