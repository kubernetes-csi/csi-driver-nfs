## CSI driver debug tips

### case#1: volume create/delete failed
> There could be multiple controller pods (only one pod is the leader), if there are no helpful logs, try to get logs from the leader controller pod.
 - find csi driver controller pod
```console
$ kubectl get pod -o wide -n kube-system | grep csi-nfs-controller
NAME                                     READY   STATUS    RESTARTS   AGE     IP             NODE
csi-nfs-controller-56bfddd689-dh5tk      5/5     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-nfs-controller-56bfddd689-sl4ll      5/5     Running   0          35s     10.240.0.23    k8s-agentpool-22533604-1
```
 - get pod description and logs
```console
$ kubectl describe pod csi-nfs-controller-56bfddd689-dh5tk -n kube-system > csi-nfs-controller-description.log
$ kubectl logs csi-nfs-controller-56bfddd689-dh5tk -c nfs -n kube-system > csi-nfs-controller.log
```

### case#2: volume mount/unmount failed
 - locate csi driver pod that does the actual volume mount/unmount

```console
$ kubectl get pod -o wide -n kube-system | grep csi-nfs-node
NAME                                      READY   STATUS    RESTARTS   AGE     IP             NODE
csi-nfs-node-cvgbs                        3/3     Running   0          7m4s    10.240.0.35    k8s-agentpool-22533604-1
csi-nfs-node-dr4s4                        3/3     Running   0          7m4s    10.240.0.4     k8s-agentpool-22533604-0
```

 - get pod description and logs
```console
$ kubectl describe po csi-nfs-node-cvgbs -n kube-system > csi-nfs-node-description.log
$ kubectl logs csi-nfs-node-cvgbs -c nfs -n kube-system > csi-nfs-node.log
```

 - check nfs mount inside driver
```console
kubectl exec -it csi-nfs-node-cvgbss -n kube-system -c nfs -- mount | grep nfs
```

### troubleshooting connection failure on agent node
```console
mkdir /tmp/test
mount -v -t nfs -o ... nfs-server:/path /tmp/test
```

### case#3: `mount.nfs: Operation not permitted` on RHEL/AlmaLinux/CentOS/Fedora/Rocky with SELinux Enforcing

#### Symptom
`CreateVolume` (controller) and/or `NodePublishVolume` (node) fail with logs like:

```
mount failed: exit status 32
Mounting command: mount
Mounting arguments: -t nfs -o nfsvers=4.1,... <server>:/<share> /tmp/pvc-...
Output: mount.nfs: Operation not permitted
```

This happens **even though** the controller and node containers are configured with `privileged: true` and `capabilities.add: [SYS_ADMIN]`. The error is `EPERM` returned by the kernel `mount(2)` syscall — *not* a server-side ACL denial (which would surface as `access denied by server`).

`ausearch -m AVC -ts recent` may show no audit lines, because SELinux denials inside privileged containers are not always logged.

#### Cause
On RHEL-family distributions with SELinux in `Enforcing` mode, the [`container-selinux`](https://github.com/containers/container-selinux/blob/main/container.te) policy gates the `mount(2)` syscall on NFS filesystems behind the `virt_use_nfs` boolean (default `off` on some images, including stock AlmaLinux/RHEL 10):

```te
tunable_policy(`virt_use_nfs',`
    fs_manage_nfs_dirs(container_domain)
    fs_manage_nfs_files(container_domain)
    fs_mount_nfs(container_domain)
    fs_unmount_nfs(container_domain)
    ...
')
```

With the boolean off, `fs_mount_nfs(container_domain)` is not granted, and the kernel returns `EPERM` regardless of capabilities held by the container.

#### Fix
Run on **every node** that runs the csi-driver-nfs controller or node plugin (in practice, every Kubernetes node, since the node plugin is a DaemonSet):

```console
sudo getsebool virt_use_nfs            # check current state
sudo setsebool -P virt_use_nfs on      # -P = persist across reboots
sudo getsebool virt_use_nfs            # expect: virt_use_nfs --> on
```

`setsebool -P` reloads the loaded policy at runtime — no service restart, no node reboot. Already-running pods pick up the change on their next mount attempt; the external-provisioner has built-in retry, so a stuck PVC will provision automatically within ~30s with no manual intervention.

If a node is already in `Permissive` mode (`getenforce`), the boolean change is unnecessary there.

#### Verify the fix
```console
kubectl get pvc -A                     # PVC should bind within ~30s
kubectl logs -n kube-system <controller-pod> -c nfs --tail=20
# expect: mount succeeds, NodePublishVolume completes
```
