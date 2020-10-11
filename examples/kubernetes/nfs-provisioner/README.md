# Set up a NFS Server on a Kubernetes cluster

> Note: This example is for development only. Because the NFS server is sticky to the node it is scheduled on, data shall be lost if the pod is rescheduled on another node.

- To create a NFS provisioner on your Kubernetes cluster, run the following command

```bash
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/examples/kubernetes/nfs-provisioner/nfs-server.yaml
```

- After deploying, a new service `nfs-service` is created. The file share path is accessible at  `10.0.171.239`. Verify if the NFS Server pod is running

```bash
$ kubectl get po nfs-server-pod
```

- To check if the server is working, we can statically create a `PersistentVolume` and a `PersistentVolumeClaim`, and mount it onto a sample pod:

```bash
kubectl create -f https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/examples/kubernetes/nfs-provisioner/app.yaml
```

Verify if the newly create deployment is Running:

```bash
$ kubectl get deploy nfs-busybox
```

