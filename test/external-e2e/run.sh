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
DRIVER="test"

install_ginkgo () {
    go install github.com/onsi/ginkgo/ginkgo@v1.14.0
}

setup_e2e_binaries() {
    # download k8s external e2e binary
    curl -sL https://dl.k8s.io/release/v1.24.0/kubernetes-test-linux-amd64.tar.gz --output e2e-tests.tar.gz
    tar -xvf e2e-tests.tar.gz && rm e2e-tests.tar.gz

    export EXTRA_HELM_OPTIONS="--set driver.name=$DRIVER.csi.k8s.io --set controller.name=csi-$DRIVER-controller --set node.name=csi-$DRIVER-node --set feature.enableInlineVolume=true"

    # test on alternative driver name
    sed -i "s/nfs.csi.k8s.io/$DRIVER.csi.k8s.io/g" deploy/example/storageclass-nfs.yaml
    sed -i "s/nfs.csi.k8s.io/$DRIVER.csi.k8s.io/g" deploy/example/snapshotclass-nfs.yaml
    # install csi driver
    mkdir -p /tmp/csi
    cp deploy/example/storageclass-nfs.yaml /tmp/csi/storageclass.yaml
    cp deploy/example/snapshotclass-nfs.yaml /tmp/csi/snapshotclass.yaml
    make e2e-bootstrap
    make install-nfs-server
}

print_logs() {
    bash ./hack/verify-examples.sh ephemeral
    echo "print out driver logs ..."
    bash ./test/utils/nfs_log.sh $DRIVER
}

install_ginkgo
setup_e2e_binaries
trap print_logs EXIT

ginkgo -p --progress --v -focus="External.Storage.*$DRIVER.csi.k8s.io" \
       -skip='\[Disruptive\]|new pod with same fsgroup skips ownership changes to the volume contents|should provision storage with any volume data source' kubernetes/test/bin/e2e.test  -- \
       -storage.testdriver=$PROJECT_ROOT/test/external-e2e/testdriver.yaml \
       --kubeconfig=$KUBECONFIG
