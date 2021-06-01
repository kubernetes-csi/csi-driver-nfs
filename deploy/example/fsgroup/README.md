# fsGroup Support

[fsGroupPolicy](https://kubernetes-csi.github.io/docs/support-fsgroup.html) feature is supported from Kubernetes 1.20, default CSI driver installation does not have this feature, follow below steps to enable this feature.

### Option#1: Enable fsGroupPolicy support in [driver helm installation](../../../charts)

add `--set feature.enableFSGroupPolicy=true` in helm installation command.

### Option#2: Enable fsGroupPolicy support on a cluster with CSI driver already installed

```console
kubectl delete CSIDriver nfs.csi.k8s.io
cat <<EOF | kubectl create -f -
apiVersion: storage.k8s.io/v1beta1
kind: CSIDriver
metadata:
  name: nfs.csi.k8s.io
spec:
  attachRequired: false
  volumeLifecycleModes:
    - Persistent
  fsGroupPolicy: File
EOF
```
