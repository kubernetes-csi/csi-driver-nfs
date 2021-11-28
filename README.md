# CSI NFS driver
[![Coverage Status](https://coveralls.io/repos/github/kubernetes-csi/csi-driver-nfs/badge.svg?branch=master)](https://coveralls.io/github/kubernetes-csi/csi-driver-nfs?branch=master)

### Overview

This is a repository for [NFS](https://en.wikipedia.org/wiki/Network_File_System) [CSI](https://kubernetes-csi.github.io/docs/) Driver, csi plugin name: `nfs.csi.k8s.io`

### Container Images & Kubernetes Compatibility:
|driver version  | supported k8s version |
|----------------|-----------------------|
|master branch   | 1.19+                 |
|v2.0.0          | 1.14+                 |

### Requirements

This driver requires existing NFSv3 or NFSv4 server.

### Install driver on a Kubernetes cluster
 - install by [kubectl](./docs/install-csi-driver.md)
 - install by [helm charts](./charts)

### Driver parameters
Please refer to [`nfs.csi.k8s.io` driver parameters](./docs/driver-parameters.md)

### Examples
 - [Set up a NFS Server on a Kubernetes cluster](./deploy/example/nfs-provisioner/README.md)
 - [Basic usage](./deploy/example/README.md)
 - [fsGroupPolicy](./deploy/example/fsgroup)

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
