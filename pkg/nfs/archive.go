/*
Copyright 2026 The Kubernetes Authors.

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
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"k8s.io/klog/v2"
)

// archiveVolumeCapability manages cache options only for the controller's
// archive deletion mount. Never change user locking, security, or transport
// choices, or mutate a capability that another mount may share.
func archiveVolumeCapability(volCap *csi.VolumeCapability) *csi.VolumeCapability {
	if runtime.GOOS != "linux" {
		// Archive locking fails closed on other platforms. Do not introduce
		// Linux-only mount options before reaching that check.
		return volCap
	}
	if volCap == nil {
		volCap = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
		}
	} else {
		volCap = proto.Clone(volCap).(*csi.VolumeCapability)
	}
	mountVolume := volCap.GetMount()
	if mountVolume == nil {
		return volCap
	}
	options := make([]string, 0, len(mountVolume.MountFlags)+2)
	for _, flags := range mountVolume.MountFlags {
		// The deletion secret supplies a comma-packed string, while other
		// CSI callers may supply individual flags. Handle both forms.
		for _, option := range strings.Split(flags, ",") {
			key, _, _ := strings.Cut(option, "=")
			switch key {
			case "lookupcache", "sharecache", "nosharecache":
				continue
			default:
				options = append(options, option)
			}
		}
	}
	// Independent caches prevent an earlier mount of the export from
	// silently supplying its lookup cache policy. Effective mountinfo checks
	// remain necessary, including when NodePublishVolume reuses a target.
	mountVolume.MountFlags = append(options, "lookupcache=none", "nosharecache")
	return volCap
}

// archiveLockPath identifies the namespace, not the CSI volume ID or the local
// mount path. Controllers using different mount paths/IDs for the same share
// and subdirectory must open the same server-side inode. Never unlink it: an
// opener could still hold the old inode and a later opener would lock a new one.
func archiveLockPath(mountPath, subDir string) string {
	key := sha256.Sum256([]byte(filepath.Clean(subDir)))
	return filepath.Join(mountPath, fmt.Sprintf(".csi-nfs-archive-%x.lock", key))
}

func (cs *ControllerServer) archiveVolume(mountPath, sourcePath, subDir string) error {
	cleanSubDir := filepath.Clean(subDir)
	if cleanSubDir == "." || cleanSubDir == ".." || filepath.IsAbs(cleanSubDir) || strings.HasPrefix(cleanSubDir, ".."+string(filepath.Separator)) {
		return status.Errorf(codes.InvalidArgument, "archive requires a non-root relative subdirectory: %q", subDir)
	}
	acquire := cs.archiveLock
	if acquire == nil {
		acquire = lockArchive
	}
	unlock, err := acquire(archiveLockPath(mountPath, subDir))
	if err != nil {
		return err
	}
	defer unlock()

	// Check under the shared lock, before touching the archive. A previous
	// request may have completed even when its response or local cache was lost.
	// Lstat also distinguishes a dangling symlink from an absent source.
	if _, err := os.Lstat(sourcePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return status.Errorf(codes.Internal, "inspect archive source %s: %v", sourcePath, err)
	}

	archivedPath := filepath.Join(mountPath, "archived-"+subDir)
	if strings.Contains(subDir, "/") {
		if err := os.MkdirAll(filepath.Dir(archivedPath), 0777); err != nil {
			return status.Errorf(codes.Internal, "create parent directory of %s: %v", archivedPath, err)
		}
	}
	klog.V(2).Infof("archiving subdirectory %s --> %s", sourcePath, archivedPath)
	if cs.Driver.removeArchivedVolumePath {
		if err := os.RemoveAll(archivedPath); err != nil {
			return status.Errorf(codes.Internal, "failed to delete archived subdirectory %s: %v", archivedPath, err)
		}
	}
	// Do not suppress ENOENT here: it can mean a missing destination parent or
	// interference by a writer that does not participate in this locking protocol.
	if err := os.Rename(sourcePath, archivedPath); err != nil {
		return status.Errorf(codes.Internal, "archive subdirectory(%s, %s) failed with %v", sourcePath, archivedPath, err)
	}
	if err := waitForPathNotExistWithTimeout(sourcePath, time.Minute); err != nil {
		return status.Errorf(codes.Internal, "DeleteVolume: internalVolumePath(%s) still exists after 1 minute", sourcePath)
	}
	return nil
}
