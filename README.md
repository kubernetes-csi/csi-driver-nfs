# NFS CSI driver for Kubernetes
![build status](https://github.com/kubernetes-csi/csi-driver-nfs/actions/workflows/linux.yaml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/kubernetes-csi/csi-driver-nfs/badge.svg?branch=master)](https://coveralls.io/github/kubernetes-csi/csi-driver-nfs?branch=master)

### Overview

This is a repository for [NFS](https://en.wikipedia.org/wiki/Network_File_System) [CSI](https://kubernetes-csi.github.io/docs/) driver, csi plugin name: `nfs.csi.k8s.io`. This driver requires existing and already configured NFSv3 or NFSv4 server, it supports dynamic provisioning of Persistent Volumes via Persistent Volume Claims by creating a new sub directory under NFS server.

### Project status: GA

### Container Images & Kubernetes Compatibility:
|driver version  | supported k8s version | status |
|----------------|-----------------------|--------|
|master branch   | 1.20+                 | GA     |
|v4.1.0          | 1.20+                 | GA     |
|v4.0.0          | 1.20+                 | GA     |
|v3.1.0          | 1.19+                 | beta   |

### Install driver on a Kubernetes cluster
 > [install NFS CSI driver on microk8s](https://microk8s.io/docs/nfs)
 - install by [kubectl](./docs/install-nfs-csi-driver.md)
 - install by [helm charts](./charts)

### Driver parameters
Please refer to [`nfs.csi.k8s.io` driver parameters](./docs/driver-parameters.md)

### Examples
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
