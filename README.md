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
If you are plannig to test using your own private image, you could either install your nfs driver using your own set of YAML files, or edit the existing YAML files to use that private image.

When using the [existing set of YAML files](https://github.com/kubernetes-csi/csi-driver-nfs/tree/master/deploy/kubernetes), you would edit the [csi-attacher-nfsplugin.yaml](https://github.com/kubernetes-csi/csi-driver-nfs/blob/master/deploy/kubernetes/csi-attacher-nfsplugin.yaml#L46) and [csi-nodeplugin-nfsplugin.yaml](https://github.com/kubernetes-csi/csi-driver-nfs/blob/master/deploy/kubernetes/csi-nodeplugin-nfsplugin.yaml#L45) files to include your private image instead of the default one. After editing these files, skip to step 3 of the following steps.

If you already have a driver installed, skip to step 4 of the following steps.

1) Build the nfs driver by running `make`
2) Create NFS Driver Image, where the image tag would be whatever that is required by your YAML deployment files        `docker build -t quay.io/k8scsi/nfsplugin:v1.0.0 .`
3) Install the Driver: `kubectl create -f deploy/kubernetes`
4) Build E2E test binary: `make build-tests`
5) Run E2E Tests using the following command: `./bin/tests --ginkgo.v --ginkgo.progress --kubeconfig=/var/run/kubernetes/admin.kubeconfig`


## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/sig-storage)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-storage)


### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
