# Volume Cloning Example

- supported from v4.3.0

## Create a Source PVC

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

## Create a PVC from an existing PVC
>  Make sure application is not writing data to source nfs share
```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/cloning/pvc-nfs-cloning.yaml
```
### Check the Creation Status

```console
$ kubectl describe pvc pvc-nfs-cloning
Name:          pvc-nfs-cloning
Namespace:     default
StorageClass:  nfs-csi
Status:        Bound
Volume:        pvc-5a00da0e-9afe-40f7-9f52-edabcf28df63
Labels:        <none>
Annotations:   kubectl.kubernetes.io/last-applied-configuration:
                 {"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"annotations":{},"name":"pvc-nfs-cloning","namespace":"default"},"spec":{"ac...
               pv.kubernetes.io/bind-completed: yes
               pv.kubernetes.io/bound-by-controller: yes
               volume.beta.kubernetes.io/storage-provisioner: nfs.csi.k8s.io
               volume.kubernetes.io/storage-provisioner: nfs.csi.k8s.io
Finalizers:    [kubernetes.io/pvc-protection]
Capacity:      10Gi
Access Modes:  RWX
VolumeMode:    Filesystem
Mounted By:    <none>
Events:
  Type    Reason                 Age   From                                                                                   Message
  ----    ------                 ----  ----                                                                                   -------
  Normal  ExternalProvisioning   5s    persistentvolume-controller                                                            waiting for a volume to be created, either by external provisioner "nfs.csi.k8s.io" or manually created by system administrator
  Normal  Provisioning           5s    nfs.csi.k8s.io_aks-nodepool1-34988195-vmss000000_534f1e86-3a71-4ca4-9b83-803c05a44d65  External provisioner is provisioning volume for claim "default/pvc-nfs-cloning"
  Normal  ProvisioningSucceeded  5s    nfs.csi.k8s.io_aks-nodepool1-34988195-vmss000000_534f1e86-3a71-4ca4-9b83-803c05a44d65  Successfully provisioned volume pvc-5a00da0e-9afe-40f7-9f52-edabcf28df63
```

## Restore the PVC into a Pod

```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/cloning/nginx-pod-restored-cloning.yaml
```

### Check Sample Data

```console
$ kubectl exec nginx-nfs-restored-cloning -- ls /mnt/nfs
outfile
```
