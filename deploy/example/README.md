# CSI driver example

You can use NFS CSI Driver to provision Persistent Volumes statically or dynamically. Please read [Kubernetes Persistent Volumes documentation](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) for more information about Static and Dynamic provisioning.

Please refer to [driver parameters](../../docs/driver-parameters.md) for more detailed usage.

## Prerequisite

- [Set up a NFS Server on a Kubernetes cluster](./nfs-provisioner/README.md) as an example
- [Install NFS CSI Driver](../../docs/install-nfs-csi-driver.md)

## Storage Class Usage (Dynamic Provisioning)

 -  Create a storage class
 > change `server`, `share` with your existing NFS server address and share name
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
  # csi.storage.k8s.io/provisioner-secret-name: "mount-options"
  # csi.storage.k8s.io/provisioner-secret-namespace: "default"
reclaimPolicy: Delete
volumeBindingMode: Immediate
mountOptions:
  - nconnect=8  # only supported on linux kernel version >= 5.3
  - hard
  - nfsvers=4.1
```

 - create PVC
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/pvc-nfs-csi-dynamic.yaml
```

## PV/PVC Usage (Static Provisioning)

- Follow the following command to create `PersistentVolume` and `PersistentVolumeClaim` statically.

```bash
# create PV
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/pv-nfs-csi.yaml

# create PVC
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/pvc-nfs-csi-static.yaml
```

## Create a deployment
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/deployment.yaml
```
