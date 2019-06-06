FROM centos:7.4.1708

# Copy nfsplugin from build _output directory
COPY bin/nfsplugin /nfsplugin
RUN mkdir -p /simplenfs/bin
COPY simplenfs/bin/plugin.so /simplenfs/plugin.so

RUN yum -y install nfs-utils && yum -y install epel-release && yum -y install jq && yum clean all

ENTRYPOINT ["/nfsplugin"]
