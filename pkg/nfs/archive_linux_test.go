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
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mount "k8s.io/mount-utils"
)

func archiveTestController(t *testing.T, share, subDir, uuid string, replace bool) (*ControllerServer, *csi.DeleteVolumeRequest) {
	t.Helper()
	driver := NewDriver(&DriverOptions{WorkingMountDir: t.TempDir(), RemoveArchivedVolumePath: replace})
	vol := &nfsVolume{server: "server", baseDir: "share", subDir: subDir, uuid: uuid, onDelete: archive}
	vol.id = getVolumeIDFromNfsVol(vol)
	path, err := getInternalMountPath(driver.workingMountDir, vol)
	require.NoError(t, err)
	// Symlink aliases model two controller mounts of the same share without
	// requiring CAP_SYS_ADMIN. File operations and flock are real, not mocked.
	require.NoError(t, os.Symlink(share, path))
	driver.ns = NewNodeServer(driver, &mount.FakeMounter{MountPoints: []mount.MountPoint{{Device: "server:/share", Path: share, Type: "nfs"}}})
	return NewControllerServer(driver), &csi.DeleteVolumeRequest{VolumeId: vol.id}
}

func archivePayload(t *testing.T, root, subDir, payload string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, subDir), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, subDir, "payload"), []byte(payload), 0600))
}

func requireArchivePayload(t *testing.T, share, subDir, expected string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(share, "archived-"+subDir, "payload"))
	require.NoError(t, err)
	require.Equal(t, expected, string(data))
}

func TestArchiveDeleteAcrossControllers(t *testing.T) {
	for _, sameID := range []bool{true, false} {
		t.Run(map[bool]string{true: "same volume ID", false: "different IDs same namespace"}[sameID], func(t *testing.T) {
			share := t.TempDir()
			archivePayload(t, share, "volume", "new data")
			archivePayload(t, share, "archived-volume", "old data")
			a, reqA := archiveTestController(t, share, "volume", "first", true)
			uuidB := "second"
			if sameID {
				uuidB = "first"
			}
			b, reqB := archiveTestController(t, share, "volume", uuidB, true)
			locked, resume := make(chan struct{}), make(chan struct{})
			defer close(resume)
			a.archiveLock = func(path string) (func(), error) {
				unlock, err := lockArchive(path)
				if err != nil {
					return nil, err
				}
				close(locked)
				<-resume
				return unlock, nil
			}
			done := make(chan error, 1)
			go func() { _, err := a.DeleteVolume(context.Background(), reqA); done <- err }()
			select {
			case <-locked:
			case err := <-done:
				t.Fatalf("first request failed before acquiring lock: %v", err)
			case <-time.After(10 * time.Second):
				t.Fatal("first request did not acquire lock")
			}
			_, err := b.DeleteVolume(context.Background(), reqB)
			require.Equal(t, codes.Aborted, status.Code(err), "%v", err)
			requireArchivePayload(t, share, "volume", "old data")
			resume <- struct{}{}
			require.NoError(t, <-done)
			requireArchivePayload(t, share, "volume", "new data")
			// Completely new instance: no cache, and a different local mount path.
			fresh, retry := archiveTestController(t, share, "volume", uuidB, true)
			_, err = fresh.DeleteVolume(context.Background(), retry)
			require.NoError(t, err)
			requireArchivePayload(t, share, "volume", "new data")
			require.FileExists(t, archiveLockPath(share, "volume"))
		})
	}
}

func TestArchiveDeleteRetryAndLayout(t *testing.T) {
	for _, replace := range []bool{false, true} {
		for _, subDir := range []string{"volume", "team/volume"} {
			t.Run(subDir+map[bool]string{false: "/keep", true: "/replace"}[replace], func(t *testing.T) {
				share := t.TempDir()
				archivePayload(t, share, subDir, "payload")
				cs, req := archiveTestController(t, share, subDir, "first", replace)
				_, err := cs.DeleteVolume(context.Background(), req)
				require.NoError(t, err)
				requireArchivePayload(t, share, subDir, "payload")
				cs, req = archiveTestController(t, share, subDir, "retry", replace)
				_, err = cs.DeleteVolume(context.Background(), req)
				require.NoError(t, err)
				requireArchivePayload(t, share, subDir, "payload")
			})
		}
	}
}

func TestArchiveDeleteReplacementDisabled(t *testing.T) {
	share := t.TempDir()
	archivePayload(t, share, "volume", "new")
	archivePayload(t, share, "archived-volume", "old")
	cs, req := archiveTestController(t, share, "volume", "first", false)
	_, err := cs.DeleteVolume(context.Background(), req)
	require.Equal(t, codes.Internal, status.Code(err))
	requireArchivePayload(t, share, "volume", "old")
	require.FileExists(t, filepath.Join(share, "volume", "payload"))
}

func TestArchiveDeleteFailedRenameRetry(t *testing.T) {
	share := t.TempDir()
	archivePayload(t, share, "volume", "new")
	archivePayload(t, share, "archived-volume", "old")
	// Force a real rename failure using a dangling source symlink and an
	// existing archive directory. Lstat must not mistake that source for absent.
	require.NoError(t, os.RemoveAll(filepath.Join(share, "volume")))
	require.NoError(t, os.Symlink("missing", filepath.Join(share, "volume")))
	cs, req := archiveTestController(t, share, "volume", "first", false)
	_, err := cs.DeleteVolume(context.Background(), req)
	require.Equal(t, codes.Internal, status.Code(err))
	requireArchivePayload(t, share, "volume", "old")
	// Restore the source and retry with replacement enabled using a fresh Driver.
	require.NoError(t, os.Remove(filepath.Join(share, "volume")))
	archivePayload(t, share, "volume", "new")
	cs, req = archiveTestController(t, share, "volume", "retry", true)
	_, err = cs.DeleteVolume(context.Background(), req)
	require.NoError(t, err)
	requireArchivePayload(t, share, "volume", "new")
}

func TestArchiveLockLifecycle(t *testing.T) {
	path := archiveLockPath(t.TempDir(), "team/volume")
	unlock, err := lockArchive(path)
	require.NoError(t, err)
	before, err := os.Stat(path)
	require.NoError(t, err)
	_, err = lockArchive(path)
	require.Equal(t, codes.Aborted, status.Code(err))
	unlock()
	unlock, err = lockArchive(path)
	require.NoError(t, err)
	unlock()
	after, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after), "lock inode must persist across operations")
	require.Equal(t, archiveLockPath("/share", "team/volume"), archiveLockPath("/share", "team/./volume"))
}

func TestArchiveLockProcessExit(t *testing.T) {
	if path := os.Getenv("CSI_NFS_TEST_ARCHIVE_LOCK"); path != "" {
		_, err := lockArchive(path)
		if err != nil {
			os.Exit(2)
		}
		// Exit without invoking the release function.
		os.Exit(0)
	}
	path := archiveLockPath(t.TempDir(), "volume")
	cmd := exec.Command(os.Args[0], "-test.run=^TestArchiveLockProcessExit$")
	cmd.Env = append(os.Environ(), "CSI_NFS_TEST_ARCHIVE_LOCK="+path)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	unlock, err := lockArchive(path)
	require.NoError(t, err)
	unlock()
	require.FileExists(t, path)
}

func TestArchiveLockRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	path := archiveLockPath(root, "volume")
	target := filepath.Join(root, "target")
	require.NoError(t, os.WriteFile(target, []byte("unchanged"), 0600))
	require.NoError(t, os.Symlink(target, path))
	_, err := lockArchive(path)
	require.Equal(t, codes.Internal, status.Code(err))
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "unchanged", string(data))
}

func TestArchiveLockMountOptions(t *testing.T) {
	for _, test := range []struct {
		name, fs string
		options  []string
		fail     bool
	}{
		{"v3", "nfs", []string{"lookupcache=none", "vers=3", "local_lock=none"}, false},
		{"v4", "nfs4", []string{"lookupcache=none", "vers=4.0", "local_lock=none"}, false},
		{"v41", "nfs4", []string{"lookupcache=none", "vers=4.1"}, false},
		{"v42", "nfs", []string{"lookupcache=none", "vers=4.2"}, false},
		{"posix only local", "nfs", []string{"lookupcache=none", "vers=3", "local_lock=posix"}, false},
		{"nolock", "nfs", []string{"lookupcache=none", "vers=3", "nolock"}, true},
		{"flock local", "nfs4", []string{"lookupcache=none", "vers=4.1", "local_lock=flock"}, true},
		{"all local", "nfs", []string{"lookupcache=none", "vers=3", "local_lock=all"}, true},
		{"unknown locking", "nfs", []string{"lookupcache=none", "vers=3", "local_lock=unknown"}, true},
		{"v2", "nfs", []string{"lookupcache=none", "vers=2"}, true},
		{"missing version", "nfs4", nil, true},
		{"default lookup cache", "nfs4", []string{"vers=4.1"}, true},
		{"positive lookup cache", "nfs4", []string{"vers=4.1", "lookupcache=positive"}, true},
		{"all lookup cache", "nfs4", []string{"vers=4.1", "lookupcache=all"}, true},
		{"conflicting lookup cache", "nfs4", []string{"vers=4.1", "lookupcache=none", "lookupcache=all"}, true},
		{"local", "ext4", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, super := range []bool{false, true} {
				entry := mount.MountInfo{MountPoint: `/share\040name`, FsType: test.fs}
				if super {
					entry.SuperOptions = test.options
				} else {
					entry.MountOptions = test.options
				}
				mounts := []mount.MountInfo{{MountPoint: "/", FsType: "ext4"}, entry, {MountPoint: "/share name-other", FsType: "nfs"}}
				err := validateArchiveLockMount("/share name/nested", mounts)
				require.Equal(t, test.fail, err != nil, "%v", err)
			}
		})
	}
	require.Error(t, validateArchiveLockMount("/missing", nil))
}

func TestArchiveDeleteErrorsPreserveData(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*testing.T, string)
		code    codes.Code
	}{
		{
			name: "lock path is a directory",
			prepare: func(t *testing.T, share string) {
				require.NoError(t, os.Mkdir(archiveLockPath(share, "volume"), 0700))
			},
			code: codes.Internal,
		},
		{
			name: "lock symlink",
			prepare: func(t *testing.T, share string) {
				require.NoError(t, os.Symlink("volume/payload", archiveLockPath(share, "volume")))
			},
			code: codes.Internal,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			share := t.TempDir()
			archivePayload(t, share, "volume", "new")
			archivePayload(t, share, "archived-volume", "old")
			test.prepare(t, share)
			cs, req := archiveTestController(t, share, "volume", "first", true)
			_, err := cs.DeleteVolume(context.Background(), req)
			require.Equal(t, test.code, status.Code(err))
			requireArchivePayload(t, share, "volume", "old")
			data, err := os.ReadFile(filepath.Join(share, "volume", "payload"))
			require.NoError(t, err)
			require.Equal(t, "new", string(data))
		})
	}
}

func TestArchiveDeleteSourceInspectionError(t *testing.T) {
	share := t.TempDir()
	// Lstat of loop/volume returns ELOOP, not ENOENT. Never remove the archive.
	require.NoError(t, os.Symlink("loop", filepath.Join(share, "loop")))
	archivePayload(t, share, "archived-loop/volume", "old")
	cs, req := archiveTestController(t, share, "loop/volume", "first", true)
	_, err := cs.DeleteVolume(context.Background(), req)
	require.Equal(t, codes.Internal, status.Code(err))
	requireArchivePayload(t, share, "loop/volume", "old")
}

func TestArchiveDeleteMissingSource(t *testing.T) {
	share := t.TempDir()
	cs, req := archiveTestController(t, share, "volume", "first", true)
	_, err := cs.DeleteVolume(context.Background(), req)
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(share, "archived-volume"))
}

func TestArchiveLockRejectsUnsafeNFSMount(t *testing.T) {
	root := os.Getenv("CSI_NFS_TEST_UNSAFE_MOUNT")
	if root == "" {
		t.Skip("set CSI_NFS_TEST_UNSAFE_MOUNT to test an existing nolock/local-only NFS mount")
	}
	path := archiveLockPath(root, "unsafe-mount-test")
	_, err := os.Lstat(path)
	require.True(t, os.IsNotExist(err), "test requires an unused lock name")
	_, err = lockArchive(path)
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "%v", err)
	_, err = os.Lstat(path)
	require.True(t, os.IsNotExist(err), "unsafe mount rejection must precede lockfile creation")
}

func TestArchiveRejectsRootOrEscapingSource(t *testing.T) {
	for _, subDir := range []string{"", ".", "team/..", "..", "../volume", "/volume"} {
		t.Run(subDir, func(t *testing.T) {
			share := t.TempDir()
			archivePayload(t, share, "archived-volume", "old")
			cs := NewControllerServer(NewDriver(&DriverOptions{RemoveArchivedVolumePath: true}))
			err := cs.archiveVolume(share, filepath.Join(share, subDir), subDir)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			requireArchivePayload(t, share, "volume", "old")
			entries, err := os.ReadDir(share)
			require.NoError(t, err)
			require.Len(t, entries, 1, "validation must precede metadata creation")
		})
	}
}
