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

FROM debian:stable-slim

ARG ARCH
ARG binary=./bin/${ARCH}/nfsplugin
COPY ${binary} /nfsplugin

RUN apt update && apt upgrade -y && apt-mark unhold libcap2 && apt-get install -y --reinstall --purge ca-certificates mount nfs-common netbase krb5-user lsb-base bash

RUN cat > /etc/default/nfs-common <<EOC
NEED_STATD=yes

NEED_IDMAPD=yes

NEED_GSSD=yes
EOC

RUN cat > /usr/local/bin/entry.sh <<'EOF'
#!/bin/sh
set -x

if [ "$1" = "true" ]; then
	shift 1
	service rpcbind start
	service nfs-common start
fi

/nfsplugin $@
EOF
RUN chmod +x /usr/local/bin/entry.sh

ENTRYPOINT ["entry.sh"]
