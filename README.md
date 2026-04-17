# NFS CSI Driver for Kubernetes

![build status](https://github.com/kubernetes-csi/csi-driver-nfs/actions/workflows/linux.yaml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/kubernetes-csi/csi-driver-nfs/badge.svg?branch=master)](https://coveralls.io/github/kubernetes-csi/csi-driver-nfs?branch=master)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/csi-driver-nfs)](https://artifacthub.io/packages/search?repo=csi-driver-nfs)

## About

This driver allows Kubernetes to access [NFS](https://en.wikipedia.org/wiki/Network_File_System) server. The driver requires an existing and already configured NFSv3 or NFSv4 server. It supports dynamic provisioning of Persistent Volumes via Persistent Volume Claims by creating a new sub directory under the NFS server.

- **CSI plugin name:** `nfs.csi.k8s.io`
- **Project status:** GA

## Container Images & Kubernetes Compatibility

| Driver Version | Supported K8s Version | Status |
|----------------|-----------------------|--------|
| master branch  | 1.21+                 | GA     |
| v4.13.2        | 1.21+                 | GA     |
| v4.12.1        | 1.21+                 | GA     |
| v4.11.0        | 1.21+                 | GA     |

## Driver Parameters

Please refer to [`nfs.csi.k8s.io` driver parameters](./docs/driver-parameters.md).

## Installation

Install the driver on a Kubernetes cluster:

- Install by [Helm charts](./charts)
- Install by [kubectl](./docs/install-nfs-csi-driver.md)

> You can also [install NFS CSI driver on MicroK8s](https://microk8s.io/docs/how-to-nfs).

## Examples

- [Basic usage](./deploy/example/README.md)
- [fsGroupPolicy](./deploy/example/fsgroup)
- [Snapshot](./deploy/example/snapshot)
- [Volume cloning](./deploy/example/cloning)

## Troubleshooting

- [CSI driver troubleshooting guide](./docs/csi-debug.md)

## Development

Please refer to the [development guide](./docs/csi-dev.md).

## CI Results

- TestGrid [sig-storage-csi-nfs](https://testgrid.k8s.io/sig-storage-csi-other) dashboard.
- Driver image build pipeline: [post-csi-driver-nfs-push-images](https://testgrid.k8s.io/sig-storage-image-build#post-csi-driver-nfs-push-images)

## Community, Discussion, Contribution, and Support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/sig-storage)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-storage)

## Code of Conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

## Links

- [Kubernetes CSI Documentation](https://kubernetes-csi.github.io/docs/)
- [CSI Drivers](https://github.com/kubernetes-csi/drivers)
- [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec)
