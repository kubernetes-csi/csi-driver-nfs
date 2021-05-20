# Install CSI driver with Helm 3

## Prerequisites
 - [install Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

## install latest version
```console
helm repo add csi-driver-nfs https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/charts
helm install csi-driver-nfs csi-driver-nfs/csi-driver-nfs --namespace kube-system
```

### install a specific version
```console
helm repo add csi-driver-nfs https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/charts
helm install csi-driver-nfs csi-driver-nfs/csi-driver-nfs --namespace kube-system --version v3.0.0
```

### search for all available chart versions
```console
helm search repo -l csi-driver-nfs
```

## uninstall CSI driver
```console
helm uninstall csi-driver-nfs -n kube-system
```

## latest chart configuration

The following table lists the configurable parameters of the latest NFS CSI Driver chart and default values.

| Parameter                                         | Description                                                | Default                                                           |
|---------------------------------------------------|------------------------------------------------------------|-------------------------------------------------------------------|
| `image.nfs.repository`                            | csi-driver-nfs docker image                                | gcr.io/k8s-staging-sig-storage/nfsplugin                          |
| `image.nfs.tag`                                   | csi-driver-nfs docker image tag                            | amd64-linux-canary                                                |
| `image.nfs.pullPolicy`                            | csi-driver-nfs image pull policy                           | IfNotPresent                                                      |
| `image.csiProvisioner.repository`                 | csi-provisioner docker image                               | k8s.gcr.io/sig-storage/csi-provisioner                            |
| `image.csiProvisioner.tag`                        | csi-provisioner docker image tag                           | v2.0.4                                                            |
| `image.csiProvisioner.pullPolicy`                 | csi-provisioner image pull policy                          | IfNotPresent                                                      |
| `image.livenessProbe.repository`                  | liveness-probe docker image                                | k8s.gcr.io/sig-storage/livenessprobe                              |
| `image.livenessProbe.tag`                         | liveness-probe docker image tag                            | v2.3.0                                                            |
| `image.livenessProbe.pullPolicy`                  | liveness-probe image pull policy                           | IfNotPresent                                                      |
| `image.nodeDriverRegistrar.repository`            | csi-node-driver-registrar docker image                     | k8s.gcr.io/sig-storage/csi-node-driver-registrar                  |
| `image.nodeDriverRegistrar.tag`                   | csi-node-driver-registrar docker image tag                 | v2.2.0                                                            |
| `image.nodeDriverRegistrar.pullPolicy`            | csi-node-driver-registrar image pull policy                | IfNotPresent                                                      |
| `imagePullSecrets`                                | Specify docker-registry secret names as an array           | [] (does not add image pull secrets to deployed pods)                                                           |
| `serviceAccount.create`                           | whether create service account of csi-nfs-controller       | true                                                              |
| `rbac.create`                                     | whether create rbac of csi-nfs-controller                  | true                                                              |
| `controller.replicas`                             | the replicas of csi-nfs-controller                         | 2                                                                 |
| `controller.runOnMaster`                          | run controller on master node                              | false                                                             |
| `controller.logLevel`                             | controller driver log level                                                          |`5`                                                           |
| `node.logLevel`                                   | node driver log level                                                          |`5`                                                           |
| `node.livenessProbe.healthPort `                  | the health check port for liveness probe                    |`29653`                                                           |

## troubleshooting
 - Add `--wait -v=5 --debug` in `helm install` command to get detailed error
 - Use `kubectl describe` to acquire more info
