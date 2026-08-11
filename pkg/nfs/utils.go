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

package nfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/klog/v2"
	netutil "k8s.io/utils/net"
)

//nolint:revive
const (
	separator                       = "#"
	delete                          = "delete"
	retain                          = "retain"
	archive                         = "archive"
	volumeOperationAlreadyExistsFmt = "An operation with the given Volume ID %s already exists"
)

var supportedOnDeleteValues = []string{"", delete, retain, archive}

func validateOnDeleteValue(onDelete string) error {
	for _, v := range supportedOnDeleteValues {
		if strings.EqualFold(v, onDelete) {
			return nil
		}
	}

	return fmt.Errorf("invalid value %s for OnDelete, supported values are %v", onDelete, supportedOnDeleteValues)
}

func NewDefaultIdentityServer(d *Driver) *IdentityServer {
	return &IdentityServer{
		Driver: d,
	}
}

func NewControllerServer(d *Driver) *ControllerServer {
	return &ControllerServer{
		Driver: d,
	}
}

func NewControllerServiceCapability(c csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
	return &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: c,
			},
		},
	}
}

func NewNodeServiceCapability(c csi.NodeServiceCapability_RPC_Type) *csi.NodeServiceCapability {
	return &csi.NodeServiceCapability{
		Type: &csi.NodeServiceCapability_Rpc{
			Rpc: &csi.NodeServiceCapability_RPC{
				Type: c,
			},
		},
	}
}

func ParseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
	}
	return "", "", fmt.Errorf("Invalid endpoint: %v", ep)
}

func getLogLevel(method string) int32 {
	if method == "/csi.v1.Identity/Probe" ||
		method == "/csi.v1.Node/NodeGetCapabilities" ||
		method == "/csi.v1.Node/NodeGetVolumeStats" {
		return 8
	}
	return 2
}

func logGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	level := klog.Level(getLogLevel(info.FullMethod))
	klog.V(level).Infof("GRPC call: %s", info.FullMethod)
	klog.V(level).Infof("GRPC request: %s", protosanitizer.StripSecrets(req))

	resp, err := handler(ctx, req)
	if err != nil {
		klog.Errorf("GRPC error: %v", err)
	} else {
		klog.V(level).Infof("GRPC response: %s", protosanitizer.StripSecrets(resp))
	}
	return resp, err
}

type VolumeLocks struct {
	locks sets.String //nolint:staticcheck
	mux   sync.Mutex
}

func NewVolumeLocks() *VolumeLocks {
	return &VolumeLocks{
		locks: sets.NewString(),
	}
}

func (vl *VolumeLocks) TryAcquire(volumeID string) bool {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	if vl.locks.Has(volumeID) {
		return false
	}
	vl.locks.Insert(volumeID)
	return true
}

func (vl *VolumeLocks) Release(volumeID string) {
	vl.mux.Lock()
	defer vl.mux.Unlock()
	vl.locks.Delete(volumeID)
}

// getMountOptions get mountOptions value from a map
func getMountOptions(context map[string]string) string {
	for k, v := range context {
		switch strings.ToLower(k) {
		case mountOptionsField:
			return v
		}
	}
	return ""
}

// unixModeToFileMode converts a raw Unix mode_t value (e.g. 02770) into Go's
// os.FileMode representation with correct bit positions for setuid, setgid,
// and sticky bits.
func unixModeToFileMode(mode uint32) os.FileMode {
	goMode := os.FileMode(mode) & os.ModePerm
	if mode&04000 != 0 {
		goMode |= os.ModeSetuid
	}
	if mode&02000 != 0 {
		goMode |= os.ModeSetgid
	}
	if mode&01000 != 0 {
		goMode |= os.ModeSticky
	}
	return goMode
}

// chmodIfPermissionMismatch only performs chmod when permission mismatches.
// The mode parameter is a raw Unix mode_t value (e.g. 02770).
// Compares both regular permission bits (0777) and special bits (setuid/setgid/sticky)
// to avoid unnecessary chmod calls while still detecting special-bit differences.
// Note: on Windows, the chmod fallback (os.Chmod) cannot apply special bits, so
// modes with setuid/setgid/sticky will never fully converge there.
func chmodIfPermissionMismatch(targetPath string, mode uint32) error {
	info, err := os.Lstat(targetPath)
	if err != nil {
		return err
	}
	// Convert the raw Unix mode to Go's FileMode representation for comparison.
	desiredMode := unixModeToFileMode(mode)
	// Mask for perm bits + special bits in Go's representation.
	mask := os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	currentMode := info.Mode() & mask
	if currentMode != desiredMode {
		klog.V(2).Infof("chmod targetPath(%s, currentMode:0%o) with desiredMode(0%o)", targetPath, mode, mode)
		if err := chmod(targetPath, mode); err != nil {
			return err
		}
	} else {
		klog.V(2).Infof("skip chmod on targetPath(%s) since mode is already 0%o", targetPath, mode)
	}
	return nil
}

// getServerFromSource if server is IPv6, return [IPv6]
func getServerFromSource(server string) string {
	if netutil.IsIPv6String(server) {
		return fmt.Sprintf("[%s]", server)
	}
	return server
}

// setKeyValueInMap set key/value pair in map
// key in the map is case insensitive, if key already exists, overwrite existing value
func setKeyValueInMap(m map[string]string, key, value string) {
	if m == nil {
		return
	}
	for k := range m {
		if strings.EqualFold(k, key) {
			m[k] = value
			return
		}
	}
	m[key] = value
}

func waitForPathNotExistWithTimeout(path string, timeout time.Duration) error {
	// Loop until the path no longer exists or the timeout is reached
	timeoutTime := time.Now().Add(timeout)
	for {
		if _, err := os.Lstat(path); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		if time.Now().After(timeoutTime) {
			return fmt.Errorf("time out waiting for path %s not exist", path)
		}
		time.Sleep(500 * time.Microsecond)
	}
}

// removeEmptyDirs removes empty directories in the given directory dir until the parent directory parentDir
// It will remove all empty directories in the path from the given directory to the parent directory
// It will not remove the parent directory parentDir
func removeEmptyDirs(parentDir, dir string) error {
	if parentDir == "" || dir == "" {
		return nil
	}

	absParentDir, err := filepath.Abs(parentDir)
	if err != nil {
		return err
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(absDir, absParentDir) {
		return fmt.Errorf("dir %s is not a subdirectory of parentDir %s", dir, parentDir)
	}

	var depth int
	for absDir != absParentDir {
		entries, err := os.ReadDir(absDir)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			klog.V(2).Infof("Removing empty directory %s", absDir)
			if err := os.Remove(absDir); err != nil {
				return err
			}
		} else {
			klog.V(2).Infof("Directory %s is not empty", absDir)
			break
		}
		if depth++; depth > 10 {
			return fmt.Errorf("depth of directory %s is too deep", dir)
		}
		absDir = filepath.Dir(absDir)
	}

	return nil
}

// ExecFunc returns a exec function's output and error
type ExecFunc func() (err error)

// TimeoutFunc returns output and error if an ExecFunc timeout
type TimeoutFunc func() (err error)

// WaitUntilTimeout waits for the exec function to complete or return timeout error
func WaitUntilTimeout(timeout time.Duration, execFunc ExecFunc, timeoutFunc TimeoutFunc) error {
	// Create a channel to receive the result of the exec function
	done := make(chan bool, 1)
	var err error

	// Start the exec function in a goroutine
	go func() {
		err = execFunc()
		done <- true
	}()

	// Wait for the function to complete or time out
	select {
	case <-done:
		return err
	case <-time.After(timeout):
		return timeoutFunc()
	}
}

// getVolumeCapabilityFromSecret retrieves the volume capability from the secret
// if secret contains mountOptions, it will return the volume capability
// if secret does not contain mountOptions, it will return nil
func getVolumeCapabilityFromSecret(volumeID string, secret map[string]string) *csi.VolumeCapability {
	mountOptions := getMountOptions(secret)
	if mountOptions != "" {
		klog.V(2).Infof("found mountOptions(%s) for volume(%s)", mountOptions, volumeID)
		return &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					MountFlags: []string{mountOptions},
				},
			},
		}
	}
	return nil
}

func validatePath(path string) error {
	for _, segment := range strings.Split(path, "/") {
		if segment == ".." {
			return fmt.Errorf("path contains directory traversal sequence")
		}
	}
	return nil
}
