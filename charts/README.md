# Installation with Helm 3

Follow this guide to install the NFS Driver for Kubernetes.

## Prerequisites

 - [install Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

## Install latest CSI Driver via `helm install`

```console
$ cd $GOPATH/src/github.com/kubernetes-csi/csi-driver-nfs/charts/latest
$ helm package csi-driver-nfs
$ helm install csi-driver-nfs csi-driver-nfs-latest.tgz --namespace kube-system
```

### Install a specific version
Specify the version of the chart to be installed using the `--version` parameter.
```console
helm install --name csi-driver-nfs csi-driver-nfs/csi-driver-nfs --namespace kube-system --version v0.2.0
```

### Search for available chart versions

```console
$ helm search repo -l csi-driver-nfs
```
## Chart configuration

The following table lists the configurable parameters of the latest NFS CSI Driver chart and their default values.

| Parameter                                         | Description                                                | Default                                                           |
|---------------------------------------------------|------------------------------------------------------------|-------------------------------------------------------------------|
| `image.nfs.repository`                            | csi-driver-nfs docker image                                | gcr.io/k8s-staging-sig-storage/nfsplugin                          |
| `image.nfs.tag`                                   | csi-driver-nfs docker image tag                            | amd64-linux-canary                                                |
| `image.nfs.pullPolicy`                            | csi-driver-nfs image pull policy                           | IfNotPresent                                                      |
| `image.csiProvisioner.repository`                 | csi-provisioner docker image                               | k8s.gcr.io/sig-storage/csi-provisioner                            |
| `image.csiProvisioner.tag`                        | csi-provisioner docker image tag                           | v2.0.4                                                            |
| `image.csiProvisioner.pullPolicy`                 | csi-provisioner image pull policy                          | IfNotPresent                                                      |
| `image.livenessProbe.repository`                  | liveness-probe docker image                                | k8s.gcr.io/sig-storage/livenessprobe                              |
| `image.livenessProbe.tag`                         | liveness-probe docker image tag                            | v2.1.0                                                            |
| `image.livenessProbe.pullPolicy`                  | liveness-probe image pull policy                           | IfNotPresent                                                      |
| `image.nodeDriverRegistrar.repository`            | csi-node-driver-registrar docker image                     | k8s.gcr.io/sig-storage/csi-node-driver-registrar                  |
| `image.nodeDriverRegistrar.tag`                   | csi-node-driver-registrar docker image tag                 | v2.0.1                                                            |
| `image.nodeDriverRegistrar.pullPolicy`            | csi-node-driver-registrar image pull policy                | IfNotPresent                                                      |
| `serviceAccount.create`                           | whether create service account of csi-nfs-controller       | true                                                              |
| `rbac.create`                                     | whether create rbac of csi-nfs-controller                  | true                                                              |
| `controller.replicas`                             | the replicas of csi-nfs-controller                         | 2                                                                 |

## Troubleshooting
 - Add `--wait -v=5 --debug` in `helm install` command to get detailed error
 - Use `kubectl describe` to acquire more info
