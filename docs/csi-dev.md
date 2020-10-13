# CSI Driver Development Guide

## Build this project

- Clone this repo

```bash
git clone https://github.com/kubernetes-csi/csi-driver-nfs
```

- Build CSI Driver

```bash
$ cd csi-driver-nfs
$ make
```

- Verify code before submitting PRs

```bash
make verify
```
## Test CSI Driver locally

> WIP

## Test CSI Driver in a Kubernetes Cluster

- Build container image and push to DockerHub

```bash
# Run `docker login` first
$ export LOCAL_USER=<DockerHub Username>
$ make local-build-push
```

- Replace `quay.io/k8scsi/nfsplugin:v2.0.0` in `deploy/kubernetes/csi-nfs-controller.yaml` and `deploy/kubernetes/csi-nfs-node.yaml` with `<YOUR DOCKERHUB ID>/nfsplugin:latest`

- Install driver locally

```bash
make local-k8s-install 
```

- Uninstall driver

```bash
make local-k8s-uninstall
```