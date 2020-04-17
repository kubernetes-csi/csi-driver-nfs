# v2.0.0

## Breaking Changes

- Changing name of the driver from "csi-nfsplugin" to "nfs.csi.k8s.io" ([#26](https://github.com/kubernetes-csi/csi-driver-nfs/pull/26), [@wozniakjan](https://github.com/wozniakjan))

## New Features

- Add support for CSI spec 1.0.
- Remove external-attacher and update deployment specs to apps/v1.
  ([#24](https://github.com/kubernetes-csi/csi-driver-nfs/pull/24),
  [@wozniakjan](https://github.com/wozniakjan))

## Bug Fixes

- Adds support for all access modes. ([#15](https://github.com/kubernetes-csi/csi-driver-nfs/pull/15), [@msau42](https://github.com/msau42))

## Other Notable Changes

- Update base image to centos8.
  ([#28](https://github.com/kubernetes-csi/csi-driver-nfs/pull/28), [@wozniakjan](https://github.com/wozniakjan))
- Switch to go mod and update dependencies. ([#22](https://github.com/kubernetes-csi/csi-driver-nfs/pull/22), [@wozniakjan](https://github.com/wozniakjan))
