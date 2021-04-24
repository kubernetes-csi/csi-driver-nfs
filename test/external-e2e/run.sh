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

set -xe

PROJECT_ROOT=$(git rev-parse --show-toplevel)

install_ginkgo () {
    apt update -y
    apt install -y golang-ginkgo-dev
}

setup_e2e_binaries() {
    # download k8s external e2e binary for kubernetes v1.19
    curl -sL https://storage.googleapis.com/kubernetes-release/release/v1.19.0/kubernetes-test-linux-amd64.tar.gz --output e2e-tests.tar.gz
    tar -xvf e2e-tests.tar.gz && rm e2e-tests.tar.gz

    # install the csi driver nfs
    mkdir -p /tmp/csi-nfs && cp deploy/example/storageclass-nfs.yaml /tmp/csi-nfs/storageclass.yaml
    make e2e-bootstrap
    make install-nfs-server
}

print_logs() {
    echo "print out driver logs ..."
    bash ./test/utils/nfs_log.sh
}

install_ginkgo
setup_e2e_binaries
trap print_logs EXIT

ginkgo -p --progress --v -focus='External.Storage.*nfs.csi.k8s.io' \
       -skip='\[Disruptive\]|\[Slow\]' kubernetes/test/bin/e2e.test  -- \
       -storage.testdriver=$PROJECT_ROOT/test/external-e2e/testdriver.yaml \
       --kubeconfig=$KUBECONFIG
