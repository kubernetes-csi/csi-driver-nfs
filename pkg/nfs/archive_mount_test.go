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
	"path/filepath"
	"runtime"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	mount "k8s.io/mount-utils"
)

func TestArchiveVolumeCapability(t *testing.T) {
	for _, test := range []struct {
		name  string
		flags []string
		want  []string
	}{
		{name: "empty", want: []string{"lookupcache=none", "nosharecache"}},
		{name: "transport and security", flags: []string{"vers=4.1,sec=krb5p,proto=tcp", "port=2049", "hard,timeo=600,retrans=2"}, want: []string{"vers=4.1", "sec=krb5p", "proto=tcp", "port=2049", "hard", "timeo=600", "retrans=2", "lookupcache=none", "nosharecache"}},
		{name: "conflicting cache settings", flags: []string{"lookupcache=all,sharecache", "lookupcache=positive", "nosharecache,lookupcache=none,sharecache"}, want: []string{"lookupcache=none", "nosharecache"}},
		{name: "explicit locks preserved", flags: []string{"vers=3,nolock,local_lock=all", "lock", "local_lock=flock,local_lock=posix,local_lock=none"}, want: []string{"vers=3", "nolock", "local_lock=all", "lock", "local_lock=flock", "local_lock=posix", "local_lock=none", "lookupcache=none", "nosharecache"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{FsType: "nfs", MountFlags: test.flags, VolumeMountGroup: "123"}},
				AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
			}
			before := proto.Clone(input).(*csi.VolumeCapability)
			got := archiveVolumeCapability(input)
			want := proto.Clone(before).(*csi.VolumeCapability)
			if runtime.GOOS == "linux" {
				want.GetMount().MountFlags = test.want
			}
			require.True(t, proto.Equal(want, got), "want %v, got %v", want, got)
			require.True(t, proto.Equal(before, input), "input capability mutated")
			require.True(t, proto.Equal(got, archiveVolumeCapability(got)), "normalization must be idempotent")
			if runtime.GOOS == "linux" {
				got.GetMount().MountFlags[0] = "changed"
				got.GetMount().FsType = "changed"
				got.AccessMode.Mode = csi.VolumeCapability_AccessMode_UNKNOWN
				require.True(t, proto.Equal(before, input), "normalized capability must not alias input")
			}
		})
	}
	t.Run("nil capability", func(t *testing.T) {
		got := archiveVolumeCapability(nil)
		if runtime.GOOS != "linux" {
			require.Nil(t, got)
			return
		}
		require.Equal(t, []string{"lookupcache=none", "nosharecache"}, got.GetMount().GetMountFlags())
	})
	t.Run("block capability not converted", func(t *testing.T) {
		input := &csi.VolumeCapability{AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}}}
		require.True(t, proto.Equal(input, archiveVolumeCapability(input)))
	})
}

// Record the real NodePublishVolume mount arguments while using local files.
// These tests do not exercise NFS caching or remote locking.
type archiveRecordingMounter struct {
	*mount.FakeMounter
	options [][]string
}

func (m *archiveRecordingMounter) Mount(source, target, fsType string, options []string) error {
	m.options = append(m.options, append([]string(nil), options...))
	return m.FakeMounter.Mount(source, target, fsType, options)
}

func TestDeleteVolumeArchiveMountOptions(t *testing.T) {
	for _, test := range []struct {
		name, policy, defaultPolicy, secret string
		normalized                          []string
	}{
		{name: "archive defaults", policy: archive, normalized: []string{"lookupcache=none", "nosharecache"}},
		{name: "default policy archive", defaultPolicy: archive, normalized: []string{"lookupcache=none", "nosharecache"}},
		{name: "case insensitive archive", policy: "ARCHIVE", secret: "vers=4.1,sec=krb5p,proto=tcp,lookupcache=all,sharecache,nosharecache", normalized: []string{"vers=4.1", "sec=krb5p", "proto=tcp", "lookupcache=none", "nosharecache"}},
		{name: "explicit unsafe lock preserved", policy: archive, secret: "vers=3,nolock,local_lock=all,lookupcache=positive", normalized: []string{"vers=3", "nolock", "local_lock=all", "lookupcache=none", "nosharecache"}},
		{name: "ordinary delete unchanged", policy: delete, secret: "vers=3,nolock,lookupcache=all,sharecache"},
		{name: "default delete unchanged"},
		{name: "retain never mounts", policy: retain, secret: "vers=4.1,lookupcache=all,sharecache"},
	} {
		t.Run(test.name, func(t *testing.T) {
			driver := NewDriver(&DriverOptions{WorkingMountDir: t.TempDir(), DefaultOnDeletePolicy: test.defaultPolicy})
			mounter := &archiveRecordingMounter{FakeMounter: mount.NewFakeMounter(nil)}
			driver.ns = NewNodeServer(driver, mounter)
			cs := NewControllerServer(driver)
			vol := &nfsVolume{server: "server", baseDir: "share", subDir: "volume", uuid: "test", onDelete: test.policy}
			vol.id = getVolumeIDFromNfsVol(vol)
			if test.policy == delete {
				// Explicit delete must override an archive default too.
				vol.id += delete
				driver.defaultOnDeletePolicy = archive
			}
			source, err := getInternalVolumePath(driver.workingMountDir, vol)
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(source, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(source, "payload"), []byte("unchanged"), 0600))
			req := &csi.DeleteVolumeRequest{VolumeId: vol.id}
			if test.secret != "" {
				req.Secrets = map[string]string{mountOptionsField: test.secret}
			}
			before := proto.Clone(req).(*csi.DeleteVolumeRequest)
			_, err = cs.DeleteVolume(context.Background(), req)
			isArchive := test.normalized != nil
			if isArchive && runtime.GOOS != "linux" {
				require.Equal(t, codes.FailedPrecondition, status.Code(err))
				require.FileExists(t, filepath.Join(source, "payload"))
			} else {
				// Local filesystem locks succeed, even for requested nolock flags:
				// effective NFS lock rejection is covered separately with mountinfo.
				require.NoError(t, err)
			}
			require.True(t, proto.Equal(before, req), "request secret mutated")
			if test.policy == retain {
				require.Empty(t, mounter.options)
				require.FileExists(t, filepath.Join(source, "payload"))
				return
			}
			require.Len(t, mounter.options, 1)
			var want []string
			if test.secret != "" {
				want = []string{test.secret}
			}
			if isArchive && runtime.GOOS == "linux" {
				want = test.normalized
			}
			require.Equal(t, want, mounter.options[0])
		})
	}
}
