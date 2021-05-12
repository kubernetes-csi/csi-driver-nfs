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

ARG ARCH=amd64

FROM k8s.gcr.io/build-image/debian-base:buster-v1.6.0

# Copy nfsplugin from build _output directory
COPY bin/nfsplugin /nfsplugin

# this is a workaround to install nfs-common & nfs-kernel-server and don't quit with error
# https://github.com/kubernetes-sigs/blob-csi-driver/issues/214#issuecomment-781602430
RUN apt update && apt install ca-certificates mount nfs-common nfs-kernel-server -y || true

ENTRYPOINT ["/nfsplugin"]
