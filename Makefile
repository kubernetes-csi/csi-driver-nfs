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
DEPLOY_FOLDER = ./deploy
CMDS=nfsplugin
PKG = github.com/kubernetes-csi/csi-driver-nfs
GINKGO_FLAGS = -ginkgo.v
GO111MODULE = on
GOPATH ?= $(shell go env GOPATH)
GOBIN ?= $(GOPATH)/bin
DOCKER_CLI_EXPERIMENTAL = enabled
export GOPATH GOBIN GO111MODULE DOCKER_CLI_EXPERIMENTAL

include release-tools/build.make
LDFLAGS = "-X ${PKG}/pkg/nfs.driverVersion=${IMAGE_VERSION} -s -w -extldflags '-static'"
GIT_COMMIT ?= $(shell git rev-parse HEAD)
IMAGE_VERSION ?= v0.5.0
# Use a custom version for E2E tests if we are testing in CI
ifdef CI
ifndef PUBLISH
override IMAGE_VERSION := e2e-$(GIT_COMMIT)
endif
endif
IMAGENAME ?= nfsplugin
REGISTRY ?= andyzhangx
REGISTRY_NAME ?= $(shell echo $(REGISTRY) | sed "s/.azurecr.io//g")
IMAGE_TAG = $(REGISTRY)/$(IMAGENAME):$(IMAGE_VERSION)
IMAGE_TAG_LATEST = $(REGISTRY)/$(IMAGENAME):latest

all: nfs

.PHONY: verify
verify: unit-test
	hack/verify-all.sh

.PHONY: unit-test
unit-test:
	go test -covermode=count -coverprofile=profile.cov ./pkg/... -v

.PHONY: sanity-test
sanity-test: nfs
	./test/sanity/run-test.sh

.PHONY: integration-test
integration-test: nfs
	./test/integration/run-test.sh

.PHONY: local-build-push
local-build-push: nfs
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

.PHONY: nfs
nfs:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags ${LDFLAGS} -mod vendor -o bin/nfsplugin ./cmd/nfsplugin

.PHONY: container
container: nfs
	docker build --no-cache -t $(IMAGE_TAG) .

.PHONY: push
push:
	docker push $(IMAGE_TAG)

.PHONY: push-latest
push-latest:
	docker tag $(IMAGE_TAG) $(IMAGE_TAG_LATEST)
	docker push $(IMAGE_TAG_LATEST)

.PHONY: install-nfs-server
install-nfs-server:
	kubectl apply -f ./deploy/example/nfs-provisioner/nfs-server.yaml

.PHONY: install-helm
install-helm:
	curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash

.PHONY: e2e-bootstrap
e2e-bootstrap: install-helm
	docker pull $(IMAGE_TAG) || make container push
	helm install csi-driver-nfs ./charts/latest/csi-driver-nfs --namespace kube-system --wait --timeout=15m -v=5 --debug \
	--set image.nfs.repository=$(REGISTRY)/$(IMAGENAME) \
	--set image.nfs.tag=$(IMAGE_VERSION) \
	--set image.nfs.pullPolicy=Always

.PHONY: e2e-teardown
e2e-teardown:
	helm delete csi-driver-nfs --namespace kube-system

.PHONY: e2e-test
e2e-test:
	go test -v -timeout=0 ./test/e2e ${GINKGO_FLAGS}

.PHONY: create-example-deployment
create-example-deployment:
	kubectl apply -f ./deploy/example/storageclass-nfs.yaml
	kubectl apply -f ./deploy/example/deployment.yaml
	kubectl apply -f ./deploy/example/statefulset.yaml
