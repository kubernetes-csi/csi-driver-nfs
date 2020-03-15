FROM centos:latest

# Copy nfsplugin from build _output directory
COPY bin/nfsplugin /nfsplugin

RUN yum -y install nfs-utils epel-release jq && yum clean all

ENTRYPOINT ["/nfsplugin"]
