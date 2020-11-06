// +build windows

/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mounter

import (
	"context"
	"fmt"
	"os"
	filepath "path/filepath"
	"strings"

	fsv1alpha1 "github.com/kubernetes-csi/csi-proxy/client/api/filesystem/v1alpha1"
	fsclientv1alpha1 "github.com/kubernetes-csi/csi-proxy/client/groups/filesystem/v1alpha1"

	smbv1alpha1 "github.com/kubernetes-csi/csi-proxy/client/api/smb/v1alpha1"
	smbclientv1alpha1 "github.com/kubernetes-csi/csi-proxy/client/groups/smb/v1alpha1"

	"k8s.io/klog/v2"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

var _ mount.Interface = &CSIProxyMounter{}

type CSIProxyMounter struct {
	FsClient  *fsclientv1alpha1.Client
	SMBClient *smbclientv1alpha1.Client
}

func normalizeWindowsPath(path string) string {
	normalizedPath := strings.Replace(path, "/", "\\", -1)
	if strings.HasPrefix(normalizedPath, "\\") {
		normalizedPath = "c:" + normalizedPath
	}
	return normalizedPath
}

func (mounter *CSIProxyMounter) SMBMount(source, target, fsType string, mountOptions, sensitiveMountOptions []string) error {
	klog.V(4).Infof("SMBMount: remote path: %s. local path: %s", source, target)

	if len(mountOptions) == 0 || len(sensitiveMountOptions) == 0 {
		return fmt.Errorf("empty mountOptions(len: %d) or sensitiveMountOptions(len: %d) is not allowed", len(mountOptions), len(sensitiveMountOptions))
	}

	parentDir := filepath.Dir(target)
	parentExists, err := mounter.ExistsPath(parentDir)
	if err != nil {
		return fmt.Errorf("parent dir: %s exist check failed with err: %v", parentDir, err)
	}

	if !parentExists {
		klog.Infof("Parent directory %s does not exists. Creating the directory", parentDir)
		err := mounter.MakeDir(parentDir)
		if err != nil {
			return fmt.Errorf("create of parent dir: %s dailed with error: %v", parentDir, err)
		}
	}

	smbMountRequest := &smbv1alpha1.NewSmbGlobalMappingRequest{
		LocalPath:  normalizeWindowsPath(target),
		RemotePath: source,
		Username:   mountOptions[0],
		Password:   sensitiveMountOptions[0],
	}
	_, err = mounter.SMBClient.NewSmbGlobalMapping(context.Background(), smbMountRequest)
	if err != nil {
		return fmt.Errorf("smb mapping failed with error: %v", err)
	}
	return nil
}

func (mounter *CSIProxyMounter) SMBUnmount(target string) error {
	klog.V(4).Infof("SMBUnmount: local path: %s", target)
	// TODO: We need to remove the SMB mapping. The change to remove the
	// directory brings the CSI code in parity with the in-tree.
	return mounter.Rmdir(target)
}

// Mount just creates a soft link at target pointing to source.
func (mounter *CSIProxyMounter) Mount(source string, target string, fstype string, options []string) error {
	klog.V(4).Infof("Mount: old name: %s. new name: %s", source, target)
	// Mount is called after the format is done.
	// TODO: Confirm that fstype is empty.
	linkRequest := &fsv1alpha1.LinkPathRequest{
		SourcePath: normalizeWindowsPath(source),
		TargetPath: normalizeWindowsPath(target),
	}
	_, err := mounter.FsClient.LinkPath(context.Background(), linkRequest)
	if err != nil {
		return err
	}
	return nil
}

// Rmdir - delete the given directory
// TODO: Call separate rmdir for pod context and plugin context. v1alpha1 for CSI
//       proxy does a relaxed check for prefix as c:\var\lib\kubelet, so we can do
//       rmdir with either pod or plugin context.
func (mounter *CSIProxyMounter) Rmdir(path string) error {
	klog.V(4).Infof("Remove directory: %s", path)
	rmdirRequest := &fsv1alpha1.RmdirRequest{
		Path:    normalizeWindowsPath(path),
		Context: fsv1alpha1.PathContext_POD,
		Force:   true,
	}
	_, err := mounter.FsClient.Rmdir(context.Background(), rmdirRequest)
	if err != nil {
		return err
	}
	return nil
}

// Unmount - Removes the directory - equivalent to unmount on Linux.
func (mounter *CSIProxyMounter) Unmount(target string) error {
	klog.V(4).Infof("Unmount: %s", target)
	return mounter.Rmdir(target)
}

func (mounter *CSIProxyMounter) List() ([]mount.MountPoint, error) {
	return []mount.MountPoint{}, fmt.Errorf("List not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounter) IsMountPointMatch(mp mount.MountPoint, dir string) bool {
	return mp.Path == dir
}

// IsLikelyMountPoint - If the directory does not exists, the function will return os.ErrNotExist error.
//   If the path exists, call to CSI proxy will check if its a link, if its a link then existence of target
//   path is checked.
func (mounter *CSIProxyMounter) IsLikelyNotMountPoint(path string) (bool, error) {
	klog.V(4).Infof("IsLikelyNotMountPoint: %s", path)
	isExists, err := mounter.ExistsPath(path)
	if err != nil {
		return false, err
	}
	if !isExists {
		return true, os.ErrNotExist
	}

	response, err := mounter.FsClient.IsMountPoint(context.Background(),
		&fsv1alpha1.IsMountPointRequest{
			Path: normalizeWindowsPath(path),
		})
	if err != nil {
		return false, err
	}
	return !response.IsMountPoint, nil
}

func (mounter *CSIProxyMounter) PathIsDevice(pathname string) (bool, error) {
	return false, fmt.Errorf("PathIsDevice not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounter) DeviceOpened(pathname string) (bool, error) {
	return false, fmt.Errorf("DeviceOpened not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounter) GetDeviceNameFromMount(mountPath, pluginMountDir string) (string, error) {
	return "", fmt.Errorf("GetDeviceNameFromMount not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounter) MakeRShared(path string) error {
	return fmt.Errorf("MakeRShared not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounter) MakeFile(pathname string) error {
	return fmt.Errorf("MakeFile not implemented for CSIProxyMounter")
}

// MakeDir - Creates a directory. The CSI proxy takes in context information.
// Currently the make dir is only used from the staging code path, hence we call it
// with Plugin context..
func (mounter *CSIProxyMounter) MakeDir(path string) error {
	klog.V(4).Infof("Make directory: %s", path)
	mkdirReq := &fsv1alpha1.MkdirRequest{
		Path:    normalizeWindowsPath(path),
		Context: fsv1alpha1.PathContext_PLUGIN,
	}
	_, err := mounter.FsClient.Mkdir(context.Background(), mkdirReq)
	if err != nil {
		return err
	}

	return nil
}

// ExistsPath - Checks if a path exists. Unlike util ExistsPath, this call does not perform follow link.
func (mounter *CSIProxyMounter) ExistsPath(path string) (bool, error) {
	klog.V(4).Infof("Exists path: %s", path)
	isExistsResponse, err := mounter.FsClient.PathExists(context.Background(),
		&fsv1alpha1.PathExistsRequest{
			Path: normalizeWindowsPath(path),
		})
	return isExistsResponse.Exists, err
}

func (mounter *CSIProxyMounter) EvalHostSymlinks(pathname string) (string, error) {
	return "", fmt.Errorf("EvalHostSymlinks not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounter) GetMountRefs(pathname string) ([]string, error) {
	return []string{}, fmt.Errorf("GetMountRefs not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounter) GetFSGroup(pathname string) (int64, error) {
	return -1, fmt.Errorf("GetFSGroup not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounter) GetSELinuxSupport(pathname string) (bool, error) {
	return false, fmt.Errorf("GetSELinuxSupport not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounter) GetMode(pathname string) (os.FileMode, error) {
	return 0, fmt.Errorf("GetMode not implemented for CSIProxyMounter")
}

func (mounter *CSIProxyMounter) MountSensitive(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return fmt.Errorf("MountSensitive not implemented for CSIProxyMounter")
}

// NewCSIProxyMounter - creates a new CSI Proxy mounter struct which encompassed all the
// clients to the CSI proxy - filesystem, disk and volume clients.
func NewCSIProxyMounter() (*CSIProxyMounter, error) {
	fsClient, err := fsclientv1alpha1.NewClient()
	if err != nil {
		return nil, err
	}
	smbClient, err := smbclientv1alpha1.NewClient()
	if err != nil {
		return nil, err
	}

	return &CSIProxyMounter{
		FsClient:  fsClient,
		SMBClient: smbClient,
	}, nil
}

func NewSafeMounter() (*mount.SafeFormatAndMount, error) {
	csiProxyMounter, err := NewCSIProxyMounter()
	if err != nil {
		return nil, err
	}
	return &mount.SafeFormatAndMount{
		Interface: csiProxyMounter,
		Exec:      utilexec.New(),
	}, nil
}
