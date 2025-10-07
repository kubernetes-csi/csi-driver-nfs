## Driver Parameters
> This driver requires existing and already configured NFSv3 or NFSv4 server, it supports dynamic provisioning of Persistent Volumes via Persistent Volume Claims by creating a new sub directory under NFS server.

### storage class usage (dynamic provisioning)
> [`StorageClass` example](../deploy/example/storageclass-nfs.yaml)

Name | Meaning | Example Value | Mandatory | Default value
--- | --- | --- | --- | ---
server | NFS Server address | domain name `nfs-server.default.svc.cluster.local` <br>or IP address `127.0.0.1` | Yes |
share | NFS share path | `/` | Yes |
subDir | sub directory under nfs share |  | No | if sub directory does not exist, this driver would create a new one
mountPermissions | mounted folder permissions. The default is `0`, if set as non-zero, driver will perform `chmod` after mount |  | No |
onDelete | when volume is deleted, keep the directory if it's `retain` | `delete`(default), `retain`, `archive`  | No | `delete`
volumePrefix | prefix of volume name created by the driver | `prefix-` | No | 

 - VolumeID(`volumeHandle`) is the identifier of the volume handled by the driver, format of VolumeID:
```
{nfs-server-address}#{sub-dir-name}#{share-name}
```
> example: `nfs-server.default.svc.cluster.local/share#subdir#`

### PV/PVC usage (static provisioning)
> [`PersistentVolume` example](../deploy/example/pv-nfs-csi.yaml)

Name | Meaning | Example Value | Mandatory | Default value
--- | --- | --- | --- | ---
volumeHandle | Specify a value the driver can use to uniquely identify the share in the cluster. | A recommended way to produce a unique value is to combine the nfs-server address, sub directory name and share name: `{nfs-server-address}#{sub-dir-name}#{share-name}`. | Yes |
volumeAttributes.server | NFS Server address | domain name `nfs-server.default.svc.cluster.local` <br>or IP address `127.0.0.1` | Yes |
volumeAttributes.share | NFS share path | `/` |  Yes  |
volumeAttributes.mountPermissions | mounted folder permissions. The default is `0`, if set as non-zero, driver will perform `chmod` after mount |  | No |

### `VolumeSnapshotClass`

Name | Meaning | Available Value | Mandatory | Default value
--- | --- | --- | --- | ---
server | NFS Server address | domain name `nfs-server.default.svc.cluster.local` <br>or IP address `127.0.0.1` |  No | use server from source volume by default
share | NFS share path | `/` | No | use share from source volume by default
mountOptions | mount options separated by comma during snapshot creation, e.g. `"nfsvers=4.1,sec=sys"` |  | No | ""

### Tips
#### `subDir` parameter supports following pv/pvc metadata conversion
> if `subDir` value contains following strings, it would be converted into corresponding pv/pvc name or namespace
 - `${pvc.metadata.name}`
 - `${pvc.metadata.namespace}`
 - `${pv.metadata.name}`

#### provide `mountOptions` for `DeleteVolume` and `DeleteSnapshot`
> since `DeleteVolumeRequest` and `DeleteSnapshotRequest` does not provide `mountOptions`, following is the workaround to provide `mountOptions` for `DeleteVolume` and `DeleteSnapshot`, check details [here](https://github.com/kubernetes-csi/csi-driver-nfs/issues/260)
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
allowVolumeExpansion: true
mountOptions:
  - nfsvers=4.1
```
  - define a storage class with `csi.storage.k8s.io/snapshotter-secret-name` and `csi.storage.k8s.io/snapshotter-secret-namespace` setting:
```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: csi-nfs-snapclass
driver: nfs.csi.k8s.io
deletionPolicy: Delete
parameters:
  # csi.storage.k8s.io/snapshotter-secret is only needed for providing mountOptions in DeleteSnapshot
  csi.storage.k8s.io/snapshotter-secret-name: "mount-options"
  csi.storage.k8s.io/snapshotter-secret-namespace: "default"
```
