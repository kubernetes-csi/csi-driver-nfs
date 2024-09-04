# Volume Snapshot Example

This creates a snapshot of a volume using `tar`.

- supported from v4.3.0
- Make sure you have `externalSnapshotter.enabled=true` if you are using the Helm chart.

## Create source PVC and an example pod to write data 

```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/storageclass-nfs.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/pvc-nfs-csi-dynamic.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/nginx-pod-nfs.yaml
```

### Check the Source PVC

```console
$ kubectl exec nginx-nfs -- ls /mnt/nfs
outfile
```

## Create a snapshot on source PVC
```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/snapshot/snapshotclass-nfs.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/snapshot/snapshot-nfs-dynamic.yaml
```
- Check snapshot Status

```console
$ kubectl describe volumesnapshot test-nfs-snapshot
Name:         test-nfs-snapshot
Namespace:    default
Labels:       <none>
Annotations:  <none>
API Version:  snapshot.storage.k8s.io/v1
Kind:         VolumeSnapshot
Metadata:
  Creation Timestamp:  2023-12-01T06:37:55Z
  Finalizers:
    snapshot.storage.kubernetes.io/volumesnapshot-as-source-protection
    snapshot.storage.kubernetes.io/volumesnapshot-bound-protection
  Generation:        1
  Resource Version:  3901120
  UID:               9a159fca-4824-4053-8d90-a92c25fb860f
Spec:
  Source:
    Persistent Volume Claim Name:  pvc-nfs-dynamic
  Volume Snapshot Class Name:      csi-nfs-snapclass
Status:
  Bound Volume Snapshot Content Name:  snapcontent-9a159fca-4824-4053-8d90-a92c25fb860f
  Creation Time:                       2023-12-01T06:37:57Z
  Ready To Use:                        true
  Restore Size:                        656257
Events:
  Type    Reason            Age   From                 Message
  ----    ------            ----  ----                 -------
  Normal  CreatingSnapshot  22s   snapshot-controller  Waiting for a snapshot default/test-nfs-snapshot to be created by the CSI driver.
  Normal  SnapshotCreated   20s   snapshot-controller  Snapshot default/test-nfs-snapshot was successfully created by the CSI driver.
  Normal  SnapshotReady     20s   snapshot-controller  Snapshot default/test-nfs-snapshot is ready to use.
```
> In above example, `snapcontent-9a159fca-4824-4053-8d90-a92c25fb860f` is the snapshot name

## Create a new PVC based on snapshot

```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/snapshot/pvc-nfs-snapshot-restored.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/snapshot/nginx-pod-restored-snapshot.yaml
```

 - Check data

```console
$ kubectl exec nginx-nfs-restored-snapshot -- ls /mnt/nfs
outfile
```

### Links
 - [CSI Snapshotter](https://github.com/kubernetes-csi/external-snapshotter)
