## Driver Parameters
> This driver requires existing and already configured NFSv3 or NFSv4 server, it supports dynamic provisioning of Persistent Volumes via Persistent Volume Claims by creating a new sub directory under NFS server.

### storage class usage (dynamic provisioning)
> [`StorageClass` example](../deploy/example/storageclass-nfs.yaml)

Name | Meaning | Example Value | Mandatory | Default value
--- | --- | --- | --- | ---
server | NFS Server address | domain name `nfs-server.default.svc.cluster.local` <br>or IP address `127.0.0.1` | Yes |
share | NFS share path | `/` | Yes |
mountPermissions | mounted folder permissions. The default is `0777`, if set as `0`, driver will not perform `chmod` after mount |  | No |

### PV/PVC usage (static provisioning)
> [`PersistentVolume` example](../deploy/example/pv-nfs-csi.yaml)

Name | Meaning | Example Value | Mandatory | Default value
--- | --- | --- | --- | ---
volumeAttributes.server | NFS Server address | domain name `nfs-server.default.svc.cluster.local` <br>or IP address `127.0.0.1` | Yes |
volumeAttributes.share | NFS share path | `/` |  Yes  |
volumeAttributes.mountPermissions | mounted folder permissions. The default is `0777` |  | No |

### Tips
#### provide `mountOptions` for `DeleteVolume`
> since `DeleteVolumeRequest` does not provide `mountOptions`, following is the workaround to provide `mountOptions` for `DeleteVolume`, check details [here](https://github.com/kubernetes-csi/csi-driver-nfs/issues/260)
  - create a secret with `mountOptions`
```console
kubectl create secret generic mount-options --from-literal mountOptions="nfsvers=3,hard"
```
  - define a storage class with `csi.storage.k8s.io/provisioner-secret-name` and `csi.storage.k8s.io/provisioner-secret-namespace` setting:
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: nfs-csi
provisioner: nfs.csi.k8s.io
parameters:
  server: nfs-server.default.svc.cluster.local
  share: /
  # csi.storage.k8s.io/provisioner-secret is only needed for providing mountOptions in DeleteVolume
  csi.storage.k8s.io/provisioner-secret-name: "mount-options"
  csi.storage.k8s.io/provisioner-secret-namespace: "default"
reclaimPolicy: Delete
volumeBindingMode: Immediate
mountOptions:
  - hard
  - nfsvers=3
```
