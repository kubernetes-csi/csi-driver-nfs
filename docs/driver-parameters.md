## Driver Parameters
> This driver requires existing and already configured NFSv3 or NFSv4 server, it supports dynamic provisioning of Persistent Volumes via Persistent Volume Claims by creating a new sub directory under NFS server.

### storage class usage (dynamic provisioning)
> [`StorageClass` example](../deploy/example/storageclass-nfs.yaml)

Name | Meaning | Example Value | Mandatory | Default value
--- | --- | --- | --- | ---
server | NFS Server address | domain name `nfs-server.default.svc.cluster.local` <br>or IP address `127.0.0.1` | Yes |
share | NFS share path | `/` | Yes |
subDir | sub directory under nfs share |  | No | if sub directory does not exist, this driver would create a new one
mountPermissions | mounted folder permissions. The default is `0`, if set as non-zero, driver will perform `chmod` after mount. See [When to set `mountPermissions`](#when-to-set-mountpermissions) below. |  | No |
onDelete | when volume is deleted, keep the directory if it's `retain` | `delete`(default), `retain`, `archive`  | No | `delete`

 - VolumeID(`volumeHandle`) is the identifier of the volume handled by the driver, format of VolumeID:
```
{nfs-server-address}#{share-name}#{sub-dir-name}
```
> example: `nfs-server.default.svc.cluster.local/share#subdir#`

### PV/PVC usage (static provisioning)
> [`PersistentVolume` example](../deploy/example/pv-nfs-csi.yaml)

Name | Meaning | Example Value | Mandatory | Default value
--- | --- | --- | --- | ---
volumeHandle | Specify a value the driver can use to uniquely identify the share in the cluster. | A recommended way to produce a unique value is to combine the nfs-server address, sub directory name and share name: `{nfs-server-address}#{share-name}#{sub-dir-name}`. | Yes |
volumeAttributes.server | NFS Server address | domain name `nfs-server.default.svc.cluster.local` <br>or IP address `127.0.0.1` | Yes |
volumeAttributes.share | NFS share path | `/` |  Yes  |
volumeAttributes.mountPermissions | mounted folder permissions. The default is `0`, if set as non-zero, driver will perform `chmod` after mount. See [When to set `mountPermissions`](#when-to-set-mountpermissions) below. |  | No |

### `VolumeSnapshotClass`

Name | Meaning | Available Value | Mandatory | Default value
--- | --- | --- | --- | ---
server | NFS Server address | domain name `nfs-server.default.svc.cluster.local` <br>or IP address `127.0.0.1` |  No | use server from source volume by default
share | NFS share path | `/` | No | use share from source volume by default
mountOptions | mount options separated by comma during snapshot creation, e.g. `"nfsvers=4.1,sec=sys"` |  | No | ""

### Driver Deployment Parameters

The following parameters can be set when deploying the driver:

Name | Meaning | Available Value | Mandatory | Default value
--- | --- | --- | --- | ---
`--enable-snapshot-compression` | enable compression when creating volume snapshots | `true`, `false` | No | `true`
`--max-snapshot-archive-size` | maximum snapshot archive file size in bytes. `0` means unlimited. | integer | No | `0`
`--max-snapshot-file-size` | maximum uncompressed size in bytes of any file in a snapshot archive. `0` means unlimited. | integer | No | `0`
`--max-snapshot-files` | maximum number of entries (files, directories, and symlinks) in a snapshot archive. `0` means unlimited. | integer | No | `0`

> **Note:** When `--enable-snapshot-compression=false`, snapshots are stored without gzip compression (using `.tar` format instead of `.tar.gz`). This can significantly speed up snapshot creation and restoration for volumes containing already-compressed data. The driver automatically detects the archive format when restoring from a snapshot, ensuring backward compatibility with existing compressed snapshots.

> **Note:** Snapshot archives are stored on the NFS server. Path traversal in those archives is rejected, and the size limits above optionally bound restore (and create) against a replaced, corrupted, or very large tarball.
>
> - `--max-snapshot-archive-size` caps the on-disk `.tar` / `.tar.gz` length while packing, and rejects an oversized archive before restore. It does not cap how large files expand after decompression.
> - `--max-snapshot-file-size` caps the uncompressed size of any single regular file.
> - `--max-snapshot-files` caps the number of archive entries (files, directories, and symlinks). It does not cap extracted bytes: one entry can still be an arbitrarily large file.
>
> Bounding extracted regular-file bytes requires `--max-snapshot-file-size`. `--max-snapshot-files` only limits how many entries are created. If any of these limits is set, the driver uses the Go tar implementation even when `--use-tar-command-in-snapshot=true`.

### Tips
#### When to set `mountPermissions`
> By default (`mountPermissions: 0`) the driver does **not** run `chmod` on the mount root after mounting. For **dynamically provisioned** volumes the controller creates the sub-directory with `os.MkdirAll(..., 0777)`, which is then masked by the controller pod's umask (typically `022`), so the resulting mode is usually `0755`. For **statically provisioned** volumes no `MkdirAll` runs and the mount root keeps whatever mode the NFS server already has on the share. Either way, the driver leaves the mode to the underlying filesystem rather than forcing a permissive bit pattern.

If your pods run as **non-root** and get `Permission denied` when writing to the volume, you have three options, listed from most to least preferred:

1. **Use `securityContext.fsGroup` on the pod.** The kubelet's fsGroup logic changes ownership of the mounted volume so that the pod's group can write to it. No driver-side `chmod` needed. This is the standard Kubernetes pattern.
   ```yaml
   spec:
     securityContext:
       fsGroup: 2000
   ```
   > Prerequisites: the `CSIDriver` object must advertise `fsGroupPolicy: File` (the driver ships this by default — see [`deploy/example/fsgroup/README.md`](../deploy/example/fsgroup/README.md)), and the NFS export must allow the kubelet-issued `chown`/`chgrp` on the mount root (i.e. `no_root_squash`, or a `squash_uid` that owns the share). Root-squashed exports may reject the ownership change and surface as a mount failure instead of a write failure.
2. **Relax the umask on the CSI controller process** so newly created sub-directories are group-writable. Lower the umask of the CSI controller container (e.g. wrap the entrypoint with `umask 002` in the container `command`) so subsequent `MkdirAll(0777)` calls stay at `0775`/`0777`. This is a per-directory fix and only affects sub-directories created after the change.
3. **Set `mountPermissions` as a last resort.** The driver will `chmod` the mount root to the given mode after each mount. Where to set it depends on how the driver is deployed / how the volume is provisioned:
   - **Cluster-wide default** — set it once in the Helm chart. `driver.mountPermissions` in `values.yaml` is threaded into the `--mount-permissions` flag on both the controller and node DaemonSet (`charts/latest/csi-driver-nfs/templates/csi-nfs-{controller,node}.yaml`), so every volume served by that driver install picks it up:
     ```yaml
     driver:
       mountPermissions: "0775"
     ```
   - **Dynamic provisioning** — override per-StorageClass under `parameters`:
     ```yaml
     parameters:
       mountPermissions: "0777"
     ```
   - **Static provisioning** — set it under `spec.csi.volumeAttributes` on the PersistentVolume:
     ```yaml
     spec:
       csi:
         volumeAttributes:
           mountPermissions: "0777"
     ```
   > ⚠️ `mountPermissions: "0777"` makes the share world-writable and does not solve cross-GID isolation: any pod on the node can write. Do not use this on NFS servers shared across trust boundaries or multi-tenant clusters. Prefer option 1 (with a matching GID) or option 2 first.

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
