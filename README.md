# CSI NFS driver

### Overview

This is a repository for [NFS](https://en.wikipedia.org/wiki/Network_File_System) [CSI](https://kubernetes-csi.github.io/docs/) Driver.
Currently it implements bare minimum of the [CSI spec](https://github.com/container-storage-interface/spec) and is in the alpha state 
of the development.

#### CSI Feature matrix

| **nfs.csi.k8s.io** | K8s version compatibility | CSI versions compatibility | Dynamic Provisioning | Resize | Snapshots | Raw Block | AccessModes              | Status                                                                       |
|--------------------|---------------------------|----------------------------|----------------------|--------|-----------|-----------|--------------------------|------------------------------------------------------------------------------|
|master              | 1.14 +                    | v1.0 +                     |  yes                 |  no    |  no       |  no       | Read/Write Multiple Pods | Alpha                                                                        |
|v2.0.0              | 1.14 +                    | v1.0 +                     |  no                  |  no    |  no       |  no       | Read/Write Multiple Pods | Alpha                                                                        |
|v1.0.0              | 1.9 - 1.15                | v1.0                       |  no                  |  no    |  no       |  no       | Read/Write Multiple Pods | [deprecated](https://github.com/kubernetes-csi/drivers/tree/master/pkg/nfs)  |

### Requirements

The CSI NFS driver requires Kubernetes cluster of version 1.14 or newer and 
preexisting NFS server, whether it is deployed on cluster or provisioned 
independently. The plugin itself provides only a communication layer between 
resources in the cluser and the NFS server.

### Install NFS CSI driver on a kubernetes cluster
Please refer to [install NFS CSI driver](./docs/install-csi-driver.md).

### Driver parameters
Please refer to [`nfs.csi.k8s.io` driver parameters](./docs/driver-parameters.md)

### Examples
 - [Set up a NFS Server on a Kubernetes cluster](./deploy/example/nfs-provisioner/README.md)
 - [Basic usage](./deploy/example/README.md)

### Troubleshooting
 - [CSI driver troubleshooting guide](./docs/csi-debug.md) 

## Kubernetes Development
Please refer to [development guide](./docs/csi-dev.md)

### Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/sig-storage)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-storage)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
