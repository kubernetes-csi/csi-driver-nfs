## CSI driver debug tips

### Case#1: volume create/delete failed
 - locate csi driver pod
```console
$ kubectl get pod -o wide -n kube-system | grep csi-nfs-controller
NAME                                     READY   STATUS    RESTARTS   AGE     IP             NODE
csi-nfs-controller-56bfddd689-dh5tk      5/5     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-nfs-controller-56bfddd689-sl4ll      5/5     Running   0          35s     10.240.0.23    k8s-agentpool-22533604-1
```
 - get csi driver logs
```console
$ kubectl logs csi-nfs-controller-56bfddd689-dh5tk -c nfs -n kube-system > csi-nfs-controller.log
```
> note: there could be multiple controller pods, if there are no helpful logs, try to get logs from other controller pods

### Case#2: volume mount/unmount failed
 - locate csi driver pod and figure out which pod does tha actual volume mount/unmount

```console
$ kubectl get pod -o wide -n kube-system | grep csi-nfs-node
NAME                                      READY   STATUS    RESTARTS   AGE     IP             NODE
csi-nfs-node-cvgbs                        3/3     Running   0          7m4s    10.240.0.35    k8s-agentpool-22533604-1
csi-nfs-node-dr4s4                        3/3     Running   0          7m4s    10.240.0.4     k8s-agentpool-22533604-0
```

 - get csi driver logs
```console
$ kubectl logs csi-nfs-node-cvgbs -c nfs -n kube-system > csi-nfs-node.log
```
