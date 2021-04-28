## Sanity Tests
Testing the NFS CSI driver using the [`sanity`](https://github.com/kubernetes-csi/csi-test/tree/master/pkg/sanity) package test suite.

## Run Sanity Tests Locally
### Prerequisite
 - Make sure golang is installed.
 - Make sure Docker is installed and running. The test will spin up a docker container hosting the NFS server.

### Run sanity tests
```
make sanity-test
```
