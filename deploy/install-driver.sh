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

# min version of this nfs plugin supporting CSI snapshots
snap_ver='4.3.0'

# external-snapshotter to install for snapshots to work
snap_controller_repo='github.com/kubernetes-csi/external-snapshotter/deploy/kubernetes/snapshot-controller?ref=v6.2.1'
snap_crd_repo='github.com/kubernetes-csi/external-snapshotter/client/config/crd?ref=v6.2.1'

ver="master"
if [[ "$#" -gt 0 ]]; then
  ver="$1"
fi

repo="https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/$ver/deploy"
if [[ "$#" -gt 1 ]]; then
  if [[ "$2" == *"local"* ]]; then
    echo "use local deploy"
    repo="./deploy"
  fi
fi

if [ $ver != "master" ]; then
  repo="$repo/$ver"
fi

# check for min supported version for snapshots or master to install external-snapshotter
if [[ "$ver" == master || "$(printf '%s\n' "$ver" "$snap_ver" | sort -V | head -n1)" = "$snap_ver" ]]; then
  kubectl kustomize "$snap_crd_repo" | kubectl apply -f -
  kubectl -n kube-system kustomize "$snap_controller_repo" | kubectl apply -f -
fi

echo "Installing NFS CSI driver, version: $ver ..."
kubectl apply -f $repo/rbac-csi-nfs.yaml
kubectl apply -f $repo/csi-nfs-driverinfo.yaml
kubectl apply -f $repo/csi-nfs-controller.yaml
kubectl apply -f $repo/csi-nfs-node.yaml
echo 'NFS CSI driver installed successfully.'
