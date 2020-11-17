## Integration Test
Integration test verifies the functionality of CSI driver as a standalone server outside Kubernetes. It exercises the lifecycle of the volume by creating, attaching, staging, mounting volumes and the reverse operations.

## Run Integration Tests Locally
### Prerequisite
 - Make sure golang is installed.
 - Make sure Docker is installed and running. The test will spin up a docker container hosting the NFS server.

### Run integration tests
```
make integration-test
```
