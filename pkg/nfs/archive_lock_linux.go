//go:build linux

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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mount "k8s.io/mount-utils"
)

// lockArchive uses flock rather than POSIX process-associated locks: separate
// opens must contend even for separate Driver instances in the same process.
// On Linux NFS, flock is implemented as a whole-file server-side byte-range
// lock (the file must be writable). Never fall back to a process-local lock.
func lockArchive(path string) (func(), error) {
	if err := checkArchiveLockMount(path); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "archive requires shared filesystem locking: %v", err)
	}
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "open archive lock %s: %v", path, err)
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, status.Errorf(codes.Internal, "archive lock %s is not a regular file or cannot be inspected: %v", path, err)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, status.Errorf(codes.Aborted, "archive operation already exists for %s", path)
		}
		return nil, status.Errorf(codes.Internal, "acquire archive lock %s: %v", path, err)
	}
	// Closing the last descriptor releases the lock, including on process exit.
	return func() { file.Close() }, nil
}

func checkArchiveLockMount(path string) error {
	// Resolve mount aliases before matching mountinfo. The lock itself must not
	// be a symlink (O_NOFOLLOW above); its parent must already exist.
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return err
	}
	mounts, err := mount.ParseMountInfo("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	return validateArchiveLockMount(parent, mounts)
}

func validateArchiveLockMount(path string, mounts []mount.MountInfo) error {
	// mountinfo escapes whitespace and backslashes in mount points. Select the
	// most specific mount, not an enclosing root or the requested mount flags.
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	var selected *mount.MountInfo
	longest := -1
	for i := range mounts {
		point := filepath.Clean(unescape.Replace(mounts[i].MountPoint))
		if path == point || strings.HasPrefix(path, strings.TrimSuffix(point, "/")+"/") {
			if len(point) >= longest {
				selected, longest = &mounts[i], len(point)
			}
		}
	}
	if selected == nil {
		return fmt.Errorf("cannot identify mount containing %s", path)
	}
	if selected.FsType != "nfs" && selected.FsType != "nfs4" {
		// Local filesystems also support flock (used in tests). Production
		// internalMount mounts an NFS share before reaching this function.
		return nil
	}
	version := ""
	lookupCacheNone := false
	for _, options := range [][]string{selected.MountOptions, selected.SuperOptions} {
		for _, option := range options {
			switch {
			case option == "nolock", option == "local_lock=all", option == "local_lock=flock":
				return fmt.Errorf("NFS mount %s uses %s, which does not provide remote flock locks", selected.MountPoint, option)
			case strings.HasPrefix(option, "local_lock=") && option != "local_lock=none" && option != "local_lock=posix":
				return fmt.Errorf("NFS mount %s has unsupported locking option %s", selected.MountPoint, option)
			case strings.HasPrefix(option, "lookupcache="):
				if option != "lookupcache=none" {
					return fmt.Errorf("NFS mount %s uses %s; archive requires lookupcache=none", selected.MountPoint, option)
				}
				lookupCacheNone = true
			case strings.HasPrefix(option, "vers="):
				version = strings.TrimPrefix(option, "vers=")
			}
		}
	}
	// flock revalidates the lock inode, not source/parent directory entries.
	// A stale positive source lookup could otherwise delete the only archive.
	// Inspect effective options: NFS sharecache can inherit an earlier mount's
	// settings even when the requested mount options contain lookupcache=none.
	if !lookupCacheNone {
		return fmt.Errorf("NFS mount %s requires effective lookupcache=none for coherent archive source lookups", selected.MountPoint)
	}
	if version != "3" && version != "4" && version != "4.0" && version != "4.1" && version != "4.2" {
		return fmt.Errorf("NFS mount %s has unsupported or unknown version %q", selected.MountPoint, version)
	}
	return nil
}
