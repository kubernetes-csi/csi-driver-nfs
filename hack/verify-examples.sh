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

echo "begin to create deployment examples ..."

kubectl apply -f ./deploy/example/storageclass-nfs.yaml
kubectl apply -f ./deploy/example/deployment.yaml
kubectl apply -f ./deploy/example/statefulset.yaml
kubectl apply -f ./deploy/example/daemonset-nfs-ephemeral.yaml

echo "sleep 60s ..."
sleep 60

echo "begin to check pod status ..."
kubectl get pods -o wide

kubectl get pods --field-selector status.phase=Running | grep deployment-nfs
kubectl get pods --field-selector status.phase=Running | grep statefulset-nfs-0
kubectl get pods --field-selector status.phase=Running | grep daemonset-nfs-ephemeral

echo "deployment examples running completed."
