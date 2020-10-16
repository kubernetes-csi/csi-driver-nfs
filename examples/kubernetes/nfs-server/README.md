# Set up a NFS Server on a Kubernetes cluster

> Note: This example is for development only. Because the NFS server is sticky to the node it is scheduled on, data shall be lost if the pod is rescheduled on another node.

- To create a NFS provisioner on your Kubernetes cluster, run the following command

```bash
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/examples/kubernetes/nfs-provisioner/nfs-server.yaml
```

- After deploying, a new service `nfs-server` is created, nfs share path is`nfs-server.default.svc.cluster.local:/`.

- To check if the server is working, we can statically create a `PersistentVolume` and a `PersistentVolumeClaim`, and mount it onto a sample pod:

```bash
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/examples/kubernetes/nfs-provisioner/app.yaml
```

Verify if the newly create deployment is Running:

```bash
# kubectl exec -it nfs-busybox-8cd8d9c5b-sf8mx sh
/ # df -h
Filesystem                Size      Used Available Use% Mounted on
...
nfs-server.default.svc.cluster.local:/
                        123.9G     15.2G    108.6G  12% /mnt
...
```

