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

FROM k8s.gcr.io/build-image/debian-base:buster-v1.6.0

# Architecture for bin folder
ARG ARCH

# Copy nfsplugin from build _output directory
COPY bin/${ARCH}/nfsplugin /nfsplugin

RUN apt update && apt-mark unhold libcap2
# this is a workaround to install nfs-common & nfs-kernel-server and don't quit with error
# https://github.com/kubernetes-sigs/blob-csi-driver/issues/214#issuecomment-781602430
RUN apt install ca-certificates mount libssl1.1 nfs-common nfs-kernel-server -y || true

ENTRYPOINT ["/nfsplugin"]
