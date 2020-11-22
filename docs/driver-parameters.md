## Driver Parameters
> This plugin driver itself only provides a communication layer between resources in the cluser and the NFS server, you need to bring your own NFS server before using this driver.

### Storage Class Usage (Dynamic Provisioning)
> [`StorageClass` example](../deploy/example/storageclass-nfs.yaml)

Name | Meaning | Example Value | Mandatory | Default value
--- | --- | --- | --- | ---
server | NFS Server endpoint | Domain name `nfs-server.default.svc.cluster.local` <br>Or IP address `127.0.0.1` | Yes |
share | NFS share path | `/` | Yes |

### PV/PVC Usage (Static Provisioning)
> [`PersistentVolume` example](../deploy/example/pv-nfs-csi.yaml)

Name | Meaning | Example Value | Mandatory | Default value
--- | --- | --- | --- | ---
volumeAttributes.source | NFS Server endpoint | Domain name `nfs-server.default.svc.cluster.local` <br>Or IP address `127.0.0.1` | Yes |
volumeAttributes.share | NFS share path | `/` |  Yes  |
