# Install NFS CSI driver master version on a kubernetes cluster

If you have already installed Helm, you can also use it to install NFS CSI driver. Please see [Installation with Helm](../charts/README.md).

## Install with kubectl
 - remote install
```console
curl -skSL https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/install-driver.sh | bash -s master --
```

 - local install
```console
git clone https://github.com/kubernetes-csi/csi-driver-nfs.git
cd csi-driver-nfs
./deploy/install-driver.sh master local
```

- check pods status:
```console
kubectl -n kube-system get pod -o wide -l app=csi-nfs-controller
kubectl -n kube-system get pod -o wide -l app=csi-nfs-node
```

example output:

```console
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-nfs-controller-56bfddd689-dh5tk       4/4     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-nfs-controller-56bfddd689-8pgr4       4/4     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-nfs-node-cvgbs                        3/3     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-nfs-node-dr4s4                        3/3     Running   0          35s     10.240.0.4     k8s-agentpool-22533604-0
```

- clean up NFS CSI driver
```console
curl -skSL https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/deploy/uninstall-driver.sh | bash -s master --
```