# CSI driver example

After the NFS CSI Driver is deployed in your cluster, you can follow this documentation to quickly deploy some examples. 

You can use NFS CSI Driver to provision Persistent Volumes statically or dynamically. Please read [Kubernetes Persistent Volumes documentation](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) for more information about Static and Dynamic provisioning.

Please refer to [driver parameters](../../docs/driver-parameters.md) for more detailed usage.

## Prerequisite

- [Set up a NFS Server on a Kubernetes cluster](./nfs-provisioner/README.md)
- [Install NFS CSI Driver](../../docs/install-csi-driver.md)

## Storage Class Usage (Dynamic Provisioning)

- Follow the following command to create a `StorageClass`, and then `PersistentVolume` and `PersistentVolumeClaim` dynamically.

```bash
# create StorageClass
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/storageclass-nfs.yaml

# create PVC
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

## Deployment/Statefulset Usage

- Follow the following command to create `Deployment` and `Statefulset` .

```bash
# create Deployment and Statefulset
git clone https://github.com/kubernetes-csi/csi-driver-nfs.git
cd csi-driver-nfs
./hack/verify-examples.sh
```