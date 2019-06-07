# CSI NFS driver

## Kubernetes
### Requirements

The folllowing feature gates and runtime config have to be enabled to deploy the driver

```
FEATURE_GATES=CSIPersistentVolume=true,MountPropagation=true
RUNTIME_CONFIG="storage.k8s.io/v1alpha1=true"
```

Mountprogpation requries support for privileged containers. So, make sure privileged containers are enabled in the cluster.

### Example local-up-cluster.sh

```ALLOW_PRIVILEGED=true FEATURE_GATES=CSIPersistentVolume=true,MountPropagation=true RUNTIME_CONFIG="storage.k8s.io/v1alpha1=true" LOG_LEVEL=5 hack/local-up-cluster.sh```

### Deploy

```kubectl -f deploy/kubernetes create```

### Example Nginx application
Please update the NFS Server & share information in nginx.yaml file.

```kubectl -f examples/kubernetes/nginx.yaml create```

## Using CSC tool

### Build nfsplugin
```
$ make nfs
```

### Start NFS driver
```
$ sudo ./_output/nfsplugin --endpoint tcp://127.0.0.1:10000 --nodeid CSINode -v=5
```

## Test
Get ```csc``` tool from https://github.com/rexray/gocsi/tree/master/csc

#### Get plugin info
```
$ csc identity plugin-info --endpoint tcp://127.0.0.1:10000
"NFS"	"0.1.0"
```

#### NodePublish a volume
```
$ export NFS_SERVER="Your Server IP (Ex: 10.10.10.10)"
$ export NFS_SHARE="Your NFS share"
$ csc node publish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nfs --attrib server=$NFS_SERVER --attrib share=$NFS_SHARE nfstestvol
nfstestvol
```

#### NodeUnpublish a volume
```
$ csc node unpublish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nfs nfstestvol
nfstestvol
```

#### Get NodeID
```
$ csc node get-id --endpoint tcp://127.0.0.1:10000
CSINode
```

## Running Kubernetes End To End tests on an NFS Driver

First, stand up a local cluster `ALLOW_PRIVILEGED=1 hack/local-up-cluster.sh` (from your Kubernetes repo)
For Fedora/RHEL clusters, the following might be required:
  ```
  sudo chown -R $USER:$USER /var/run/kubernetes/
  sudo chown -R $USER:$USER /var/lib/kubelet
  sudo chcon -R -t svirt_sandbox_file_t /var/lib/kubelet
  ```

For all of the following commands, set the `KUBECONFIG` env variables as instructed by `local-up-cluster.sh` or as needed for some other cluster.

`deploy/kubernetes/deploy.sh` will deploy the nfs driver using an
image from quay.io which (at the time of writing this) isn't available
yet.

It is possible to use a locally built image without any registry:
``` sh
$ make container
...
Successfully tagged nfsplugin:latest

$ NFSPLUGIN_REGISTRY=none NFSPLUGIN_TAG=latest deploy/kubernetes/deploy.sh
applying RBAC rules
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-attacher/v1.0.1/deploy/kubernetes/rbac.yaml
serviceaccount/csi-attacher created
clusterrole.rbac.authorization.k8s.io/external-attacher-runner created
clusterrolebinding.rbac.authorization.k8s.io/csi-attacher-role created
role.rbac.authorization.k8s.io/external-attacher-cfg created
rolebinding.rbac.authorization.k8s.io/csi-attacher-role-cfg created
deploying nfs plugin components
   deploy/kubernetes/csi-attacher-nfsplugin.yaml
        using           image: quay.io/k8scsi/csi-attacher:v1.0.1
        using           image: nfsplugin:latest
service/csi-attacher-nfsplugin created
statefulset.apps/csi-attacher-nfsplugin created
   deploy/kubernetes/csi-nodeplugin-nfsplugin.yaml
        using           image: quay.io/k8scsi/csi-node-driver-registrar:v1.0.2
        using           image: nfsplugin:latest
daemonset.apps/csi-nodeplugin-nfsplugin created
   deploy/kubernetes/csi-nodeplugin-rbac.yaml
serviceaccount/csi-nodeplugin created
clusterrole.rbac.authorization.k8s.io/csi-nodeplugin created
clusterrolebinding.rbac.authorization.k8s.io/csi-nodeplugin created
10:53:11 waiting for nfs deployment to complete, attempt #0
10:53:21 waiting for nfs deployment to complete, attempt #1
```

Other clusters may need a registry to pull from:
``` sh
$ make push REGISTRY_NAME=my-registry:5000
...
$ NFSPLUGIN_REGISTRY=my-registry:5000 NFSPLUGIN_TAG=latest deploy/kubernetes/deploy.sh
```


Once you have the driver installed, tests can be run with:
``` sh
$ make build-tests
mkdir -p bin
CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X main.version=4fa924a251193c9eef937042112462433089d658 -extldflags "-static"' -o ./bin/tests ./cmd/tests
$ ./bin/tests --ginkgo.v --ginkgo.progress
Jun  7 10:57:39.667: INFO: The --provider flag is not set. Continuing as if --provider=skeleton had been used.
Running Suite: CSI Suite
========================
Random Seed: 1559897859 - Will randomize all specs
Will run 103 of 103 specs
...

```

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/sig-storage)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-storage)


### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
