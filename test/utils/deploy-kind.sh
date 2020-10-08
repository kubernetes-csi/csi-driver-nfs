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

#!/bin/bash

KUBERNETES_VERSION=v1.18.8
KUBECONFIG=$HOME/.kube/config

# Setup and download kind
# curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.9.0/kind-linux-amd64 && chmod +x ./kind && sudo mv ./kind /usr/local/bin/kind
# curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/$KUBERNETES_VERSION/bin/linux/amd64/kubectl && chmod +x kubectl && sudo mv kubectl /usr/local/bin/
# curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash

sudo kind create cluster --image kindest/node:$KUBERNETES_VERSION
mkdir -p $HOME/.kube
sudo chown -R $USER: $HOME/.kube/
sudo kind get kubeconfig > $KUBECONFIG

echo "Seting up nfs-server-provisioner and deploying NFS CSI driver"

kubectl cluster-info
JSONPATH='{range .items[*]}{@.metadata.name}:{range @.status.conditions[*]}{@.type}={@.status};{end}{end}'; until kubectl -n kube-system get pods -lk8s-app=kube-dns -o jsonpath="$JSONPATH" 2>&1 | grep -q "Ready=True"; do sleep 1;echo "waiting for kube-dns to be available";  done

echo "Cluster is running"
