# Set up a NFS Server on a Kubernetes cluster

> Note: This example is for development perspective only. Because the NFS server is sticky to the node it is scheduled on, data shall be lost if the pod is rescheduled on another node.

To create a NFS provisioner on your Kubernetes cluster, run the following command

```bash
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/examples/kubernetes/nfs-provisioner/nfs-server.yaml
```

After deploying, a new service `nfs-server` is created. The file share path is accessible at `nfs-server.default.svc.cluster.local/nfsshare`.


To obtain a public IP for the service, run the following command instead

```bash
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/examples/kubernetes/nfs-provisioner/nfs-server-lb.yaml
```
