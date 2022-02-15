#! /bin/bash

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

# A Prow job can override these defaults, but this shouldn't be necessary.

# Only these tests make sense for csi-driver-nfs until we can integrate k/k
# e2es.
: ${CSI_PROW_TESTS:="unit"}

. release-tools/prow.sh

./release-tools/verify-boilerplate.sh "$(pwd)"
./release-tools/verify-spelling.sh "$(pwd)"

main
