# Copyright 2017 The Kubernetes Authors.
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
CMDS=nfsplugin
DEPLOY_FOLDER = ./deploy/kubernetes
CMDS=nfsplugin
all: build

include release-tools/build.make

.PHONY: sanity-test
sanity-test: build
	./test/sanity/run-test.sh

.PHONY: local-build-push
local-build-push: build
	docker build -t $(LOCAL_USER)/nfsplugin:latest .
	docker push $(LOCAL_USER)/nfsplugin

.PHONY: local-k8s-install
local-k8s-install:
	echo "Instlling locally"
	kubectl apply -f $(DEPLOY_FOLDER)/rbac-csi-nfs-controller.yaml
	kubectl apply -f $(DEPLOY_FOLDER)/csi-nfs-driverinfo.yaml
	kubectl apply -f $(DEPLOY_FOLDER)/csi-nfs-controller.yaml
	kubectl apply -f $(DEPLOY_FOLDER)/csi-nfs-node.yaml
	echo "Successfully installed"

.PHONY: local-k8s-uninstall
local-k8s-uninstall:
	echo "Uninstalling driver"
	kubectl delete -f $(DEPLOY_FOLDER)/csi-nfs-controller.yaml --ignore-not-found
	kubectl delete -f $(DEPLOY_FOLDER)/csi-nfs-node.yaml --ignore-not-found
	kubectl delete -f $(DEPLOY_FOLDER)/csi-nfs-driverinfo.yaml --ignore-not-found
	kubectl delete -f $(DEPLOY_FOLDER)/rbac-csi-nfs-controller.yaml --ignore-not-found
	echo "Uninstalled NFS driver"
