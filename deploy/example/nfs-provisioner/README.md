# Set up a NFS Server on a Kubernetes cluster

After the NFS CSI Driver is deployed in your cluster, you can follow this documentation to quickly deploy some example applications. You can use NFS CSI Driver to provision Persistent Volumes statically or dynamically. Please read Kubernetes Persistent Volumes for more information about Static and Dynamic provisioning.

There are multiple different NFS servers you can use for testing of 
the plugin, the major versions of the protocol v2, v3 and v4 should be supported
by the current implementation. This page will show you how to set up a NFS Server deployment on a Kubernetes cluster.

- (For linux/amd64) To create a NFS provisioner on your Kubernetes cluster, run the following command.

```bash
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/nfs-provisioner/nfs-server.yaml
```

- (For linux/arm) To create a NFS provisioner on your Kubernetes cluster, run the following command.

```bash
git clone https://github.com/sjiveson/nfs-server-alpine.git
cd nfs-server-alpine
docker build -t <your-name>/nfs-server-alpine:latest-arm .
wget https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/nfs-provisioner/nfs-server.yaml
sed -i 's/<your-name>/itsthenetwork/' nfs-server.yaml
kubectl create -f nfs-server.yaml
```

- During the deployment, a new service `nfs-server` will be created which exposes the NFS server endpoint `nfs-server.default.svc.cluster.local` and the share path `/`. You can specify `PersistentVolume` or `StorageClass` using these information.

- Deploy the NFS CSI driver, please refer to [install NFS CSI driver](../../../docs/install-nfs-csi-driver.md).

- To check if the NFS server is working, we can statically create a PersistentVolume and a PersistentVolumeClaim, and mount it onto a sample pod:

```bash
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/example/nfs-provisioner/nginx-pod.yaml
```

 - Verify if the NFS server is functional, you can check the mount point from the example pod.

 ```bash
kubectl exec nginx-nfs-example -- bash -c "findmnt /var/www -o TARGET,SOURCE,FSTYPE"
```

 - The output should look like the following:

 ```bash
TARGET   SOURCE                                 FSTYPE
/var/www nfs-server.default.svc.cluster.local:/ nfs4
```
