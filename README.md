# CSI NFS driver

## Overview

This is a repository for [NFS](https://en.wikipedia.org/wiki/Network_File_System) [CSI](https://kubernetes-csi.github.io/docs/) Driver.
Currently it implements bare minimum of the [CSI spec](https://github.com/container-storage-interface/spec) and is in the alpha state 
of the development.

#### CSI Feature matrix

| **nfs.csi.k8s.io** | K8s version compatibility | CSI versions compatibility | Dynamic Provisioning | Resize | Snapshots | Raw Block | AccessModes              | Status                                                                       |
|--------------------|---------------------------|----------------------------|----------------------|--------|-----------|-----------|--------------------------|------------------------------------------------------------------------------|
|master              | 1.14 +                    | v1.0 +                     |  no                  |  no    |  no       |  no       | Read/Write Multiple Pods | Alpha                                                                        |
|v2.0.0              | 1.14 +                    | v1.0 +                     |  no                  |  no    |  no       |  no       | Read/Write Multiple Pods | Alpha                                                                        |
|v1.0.0              | 1.9 - 1.15                | v1.0                       |  no                  |  no    |  no       |  no       | Read/Write Multiple Pods | [deprecated](https://github.com/kubernetes-csi/drivers/tree/master/pkg/nfs)  |

## Requirements

The CSI NFS driver requires Kubernetes cluster of version 1.14 or newer and 
preexisting NFS server, whether it is deployed on cluster or provisioned 
independently. The plugin itself provides only a communication layer between 
resources in the cluser and the NFS server.

## Install NFS CSI driver on a kubernetes cluster
Please refer to [install NFS CSI driver](https://github.com/kubernetes-csi/csi-driver-nfs/blob/master/docs/install-csi-driver.md).

## Example

There are multiple ways to create a kubernetes cluster, the NFS CSI plugin
should work invariantly of your cluster setup. Very simple way of getting
a local environment for testing can be achieved using for example
[kind](https://github.com/kubernetes-sigs/kind).

There are also multiple different NFS servers you can use for testing of 
the plugin, the major versions of the protocol v2, v3 and v4 should be supported
by the current implementation.

The example assumes you have your cluster created (e.g. `kind create cluster`)
and working NFS server (e.g. https://github.com/rootfs/nfs-ganesha-docker)

#### Deploy

Deploy the NFS plugin along with the `CSIDriver` info.
```console
kubectl create -f deploy/kubernetes
```

#### Example Nginx application

The [/examples/kubernetes/nginx.yaml](/examples/kubernetes/nginx.yaml) contains a `PersistentVolume`,
`PersistentVolumeClaim` and an nginx `Pod` mounting the NFS volume under `/var/www`.

You will need to update the NFS Server IP and the share information under 
`volumeAttributes` inside `PersistentVolume` in `nginx.yaml` file to match your
NFS server public end point and configuration. You can also provide additional 
`mountOptions`, such as protocol version, in the `PersistentVolume` `spec` 
relevant for your NFS Server.

```console
kubectl create -f examples/kubernetes/nginx.yaml
```

## Running Kubernetes End To End tests on an NFS Driver

First, stand up a local cluster `ALLOW_PRIVILEGED=1 hack/local-up-cluster.sh` (from your Kubernetes repo)
For Fedora/RHEL clusters, the following might be required:
```console
sudo chown -R $USER:$USER /var/run/kubernetes/
sudo chown -R $USER:$USER /var/lib/kubelet
sudo chcon -R -t svirt_sandbox_file_t /var/lib/kubelet
```
If you are plannig to test using your own private image, you could either install your nfs driver using your own set of YAML files, or edit the existing YAML files to use that private image.

When using the [existing set of YAML files](https://github.com/kubernetes-csi/csi-driver-nfs/tree/master/deploy/kubernetes), you would edit [csi-nfs-node.yaml](https://github.com/kubernetes-csi/csi-driver-nfs/blob/master/deploy/kubernetes/csi-nfs-node.yaml#L45) files to include your private image instead of the default one. After editing these files, skip to step 3 of the following steps.

If you already have a driver installed, skip to step 4 of the following steps.

1) Build the nfs driver by running `make`
2) Create NFS Driver Image, where the image tag would be whatever that is required by your YAML deployment files        `docker build -t quay.io/k8scsi/nfsplugin:v2.0.0 .`
3) Install the Driver: `kubectl create -f deploy/kubernetes`
4) Build E2E test binary: `make build-tests`
5) Run E2E Tests using the following command: `./bin/tests --ginkgo.v --ginkgo.progress --kubeconfig=/var/run/kubernetes/admin.kubeconfig`


## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/sig-storage)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-storage)


### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
