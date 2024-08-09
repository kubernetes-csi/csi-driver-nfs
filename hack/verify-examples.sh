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

rollout_and_wait() {
    echo "Applying config \"$1\""
    trap "echo \"Failed to apply config \\\"$1\\\"\" >&2" err

    APPNAME=$(kubectl apply -f $1 | grep -E "^(:?daemonset|deployment|statefulset|pod)" | awk '{printf $1}')
    if [[ -n $(expr "${APPNAME}" : "\(daemonset\|deployment\|statefulset\|pod\)" || true) ]]; then
        kubectl rollout status $APPNAME --watch --timeout=5m -n default
    else
        kubectl wait "${APPNAME}" --for condition=ready --timeout=5m -n default
    fi
}

echo "begin to create deployment examples ..."

kubectl apply -f ./deploy/example/storageclass-nfs.yaml

EXAMPLES+=(\
    deploy/example/deployment.yaml \
    deploy/example/statefulset.yaml \
)

if [[ "$#" -gt 0 ]]&&[[ "$1" == *"ephemeral"* ]]; then
    EXAMPLES+=(\
        deploy/example/daemonset-nfs-ephemeral.yaml \
    )
fi

for EXAMPLE in "${EXAMPLES[@]}"; do
    rollout_and_wait $EXAMPLE
done

echo "deployment examples running completed."
