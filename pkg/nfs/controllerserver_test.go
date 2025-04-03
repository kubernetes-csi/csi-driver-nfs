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
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	mount "k8s.io/mount-utils"
)

const (
	testServer                   = "test-server"
	testBaseDir                  = "test-base-dir"
	testBaseDirNested            = "test/base/dir"
	testCSIVolume                = "volume-name"
	testVolumeID                 = "test-server/test-base-dir/volume-name"
	newTestVolumeID              = "test-server#test-base-dir#volume-name##"
	newTestVolumeWithVolumeID    = "test-server#test-base-dir#volume-name#volume-name#"
	testVolumeIDNested           = "test-server/test/base/dir/volume-name"
	newTestVolumeIDNested        = "test-server#test/base/dir#volume-name#"
	newTestVolumeIDUUID          = "test-server#test-base-dir#volume-name#uuid"
	newTestVolumeOnDeleteRetain  = "test-server#test-base-dir#volume-name#uuid#retain"
	newTestVolumeOnDeleteDelete  = "test-server#test-base-dir#volume-name#uuid#delete"
	newTestVolumeOnDeleteArchive = "test-server#test-base-dir#volume-name##archive"
)

func initTestController(_ *testing.T) *ControllerServer {
	mounter := &mount.FakeMounter{MountPoints: []mount.MountPoint{}}
	driver := NewDriver(&DriverOptions{
		WorkingMountDir:  "/tmp",
		MountPermissions: 0777,
	})
	driver.ns = NewNodeServer(driver, mounter)
	cs := NewControllerServer(driver)
	return cs
}

//nolint:forbidigo
func teardown() {
	err := os.RemoveAll("/tmp/" + testCSIVolume)

	if err != nil {
		fmt.Print(err.Error())
		fmt.Printf("\n")
		fmt.Printf("\033[1;91m%s\033[0m\n", "> Teardown failed")
	} else {
		fmt.Printf("\033[1;36m%s\033[0m\n", "> Teardown completed")
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	teardown()
	os.Exit(code)
}

func TestCreateVolume(t *testing.T) {
	cases := []struct {
		name      string
		req       *csi.CreateVolumeRequest
		resp      *csi.CreateVolumeResponse
		expectErr bool
	}{
		{
			name: "valid defaults",
			req: &csi.CreateVolumeRequest{
				Name: testCSIVolume,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
						},
					},
				},
				Parameters: map[string]string{
					paramServer:           testServer,
					paramShare:            testBaseDir,
					mountPermissionsField: "0750",
				},
			},
			resp: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId: newTestVolumeID,
					VolumeContext: map[string]string{
						paramServer:           testServer,
						paramShare:            testBaseDir,
						paramSubDir:           testCSIVolume,
						mountPermissionsField: "0750",
					},
				},
			},
		},
		{
			name: "valid defaults with newTestVolumeID",
			req: &csi.CreateVolumeRequest{
				Name: testCSIVolume,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
						},
					},
				},
				Parameters: map[string]string{
					paramServer: testServer,
					paramShare:  testBaseDir,
					paramSubDir: testCSIVolume,
				},
			},
			resp: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId: newTestVolumeWithVolumeID,
					VolumeContext: map[string]string{
						paramServer: testServer,
						paramShare:  testBaseDir,
						paramSubDir: testCSIVolume,
					},
				},
			},
		},
		{
			name: "name empty",
			req: &csi.CreateVolumeRequest{
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
						},
					},
				},
				Parameters: map[string]string{
					paramServer: testServer,
					paramShare:  testBaseDir,
				},
			},
			expectErr: true,
		},
		{
			name: "invalid create context",
			req: &csi.CreateVolumeRequest{
				Name: testCSIVolume,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
						},
					},
				},
				Parameters: map[string]string{
					"unknown-parameter": "foo",
				},
			},
			expectErr: true,
		},
		{
			name: "[Error] invalid mountPermissions",
			req: &csi.CreateVolumeRequest{
				Name: testCSIVolume,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
						},
					},
				},
				Parameters: map[string]string{
					paramServer:           testServer,
					paramShare:            testBaseDir,
					mountPermissionsField: "07ab",
				},
			},
			expectErr: true,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// Setup
			cs := initTestController(t)
			// Run
			resp, err := cs.CreateVolume(context.TODO(), test.req)

			// Verify
			if !test.expectErr && err != nil {
				t.Errorf("test %q failed: %v", test.name, err)
			}
			if test.expectErr && err == nil {
				t.Errorf("test %q failed; got success", test.name)
			}
			if !reflect.DeepEqual(resp, test.resp) {
				t.Errorf("test %q failed: got resp %+v, expected %+v", test.name, resp, test.resp)
			}
			if !test.expectErr {
				info, err := os.Stat(filepath.Join(cs.Driver.workingMountDir, test.req.Name, test.req.Name))
				if err != nil {
					t.Errorf("test %q failed: couldn't find volume subdirectory: %v", test.name, err)
				}
				if !info.IsDir() {
					t.Errorf("test %q failed: subfile not a directory", test.name)
				}
			}
		})
	}
}

func TestDeleteVolume(t *testing.T) {
	cases := []struct {
		desc                 string
		testOnWindows        bool
		req                  *csi.DeleteVolumeRequest
		resp                 *csi.DeleteVolumeResponse
		expectedDeleteSubDir bool
		expectedErr          error
	}{
		{
			desc:                 "Volume ID missing",
			testOnWindows:        true,
			req:                  &csi.DeleteVolumeRequest{},
			resp:                 nil,
			expectedErr:          status.Error(codes.InvalidArgument, "Volume ID missing in request"),
			expectedDeleteSubDir: false,
		},
		{
			desc:                 "Valid request",
			testOnWindows:        false,
			req:                  &csi.DeleteVolumeRequest{VolumeId: testVolumeID},
			resp:                 &csi.DeleteVolumeResponse{},
			expectedErr:          nil,
			expectedDeleteSubDir: true,
		},
		{
			desc:                 "Valid request with newTestVolumeID",
			testOnWindows:        true,
			req:                  &csi.DeleteVolumeRequest{VolumeId: newTestVolumeID},
			resp:                 &csi.DeleteVolumeResponse{},
			expectedErr:          nil,
			expectedDeleteSubDir: true,
		},
		{
			desc:                 "Valid request with onDelete:retain",
			testOnWindows:        true,
			req:                  &csi.DeleteVolumeRequest{VolumeId: newTestVolumeOnDeleteRetain},
			resp:                 &csi.DeleteVolumeResponse{},
			expectedErr:          nil,
			expectedDeleteSubDir: false,
		},
		{
			desc:                 "Valid request with onDelete:archive",
			testOnWindows:        false,
			req:                  &csi.DeleteVolumeRequest{VolumeId: newTestVolumeOnDeleteArchive},
			resp:                 &csi.DeleteVolumeResponse{},
			expectedErr:          nil,
			expectedDeleteSubDir: true,
		},
	}

	for _, test := range cases {
		if runtime.GOOS == "windows" && !test.testOnWindows {
			continue
		}
		t.Run(test.desc, func(t *testing.T) {
			cs := initTestController(t)
			_ = os.MkdirAll(filepath.Join(cs.Driver.workingMountDir, testCSIVolume), os.ModePerm)
			_, _ = os.Create(filepath.Join(cs.Driver.workingMountDir, testCSIVolume, testCSIVolume))

			resp, err := cs.DeleteVolume(context.TODO(), test.req)

			if test.expectedErr == nil && err != nil {
				t.Errorf("test %q failed: %v", test.desc, err)
			}
			if test.expectedErr != nil && err == nil {
				t.Errorf("test %q failed; expected error %v, got success", test.desc, test.expectedErr)
			}
			if !reflect.DeepEqual(resp, test.resp) {
				t.Errorf("test %q failed: got resp %+v, expected %+v", test.desc, resp, test.resp)
			}

			if _, err := os.Stat(filepath.Join(cs.Driver.workingMountDir, testCSIVolume, testCSIVolume)); test.expectedErr == nil {
				if !os.IsNotExist(err) && test.expectedDeleteSubDir {
					t.Errorf("test %q failed: expected volume subdirectory deleted, it still exists", test.desc)
				} else if os.IsNotExist(err) && !test.expectedDeleteSubDir {
					t.Errorf("test %q failed: expected volume subdirectory not deleted, it was deleted", test.desc)
				}
			}
		})
	}
}

func TestControllerGetCapabilities(t *testing.T) {
	cases := []struct {
		desc        string
		req         *csi.ControllerGetCapabilitiesRequest
		resp        *csi.ControllerGetCapabilitiesResponse
		expectedErr error
	}{
		{
			desc: "valid request",
			req:  &csi.ControllerGetCapabilitiesRequest{},
			resp: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
							},
						},
					},
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
							},
						},
					},
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
							},
						},
					},
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
							},
						},
					},
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
							},
						},
					},
				},
			},
			expectedErr: nil,
		},
	}

	for _, test := range cases {
		t.Run(test.desc, func(t *testing.T) {
			// Setup
			cs := initTestController(t)

			// Run
			resp, err := cs.ControllerGetCapabilities(context.TODO(), test.req)

			// Verify
			if test.expectedErr == nil && err != nil {
				t.Errorf("test %q failed: %v", test.desc, err)
			}
			if test.expectedErr != nil && err == nil {
				t.Errorf("test %q failed; expected error %v, got success", test.desc, test.expectedErr)
			}
			if !reflect.DeepEqual(resp, test.resp) {
				t.Errorf("test %q failed: got resp %+v, expected %+v", test.desc, resp, test.resp)
			}
		})
	}
}

func TestNfsVolFromId(t *testing.T) {
	cases := []struct {
		name      string
		volumeID  string
		resp      *nfsVolume
		expectErr bool
	}{
		{
			name:      "ID only server",
			volumeID:  testServer,
			resp:      nil,
			expectErr: true,
		},
		{
			name:      "ID missing subDir",
			volumeID:  strings.Join([]string{testServer, testBaseDir}, "/"),
			resp:      nil,
			expectErr: true,
		},
		{
			name:     "valid request single baseDir",
			volumeID: testVolumeID,
			resp: &nfsVolume{
				id:      testVolumeID,
				server:  testServer,
				baseDir: testBaseDir,
				subDir:  testCSIVolume,
			},
			expectErr: false,
		},
		{
			name:     "valid request single baseDir with newTestVolumeID",
			volumeID: newTestVolumeID,
			resp: &nfsVolume{
				id:       newTestVolumeID,
				server:   testServer,
				baseDir:  testBaseDir,
				subDir:   testCSIVolume,
				onDelete: "",
			},
			expectErr: false,
		},
		{
			name:     "valid request nested baseDir",
			volumeID: testVolumeIDNested,
			resp: &nfsVolume{
				id:      testVolumeIDNested,
				server:  testServer,
				baseDir: testBaseDirNested,
				subDir:  testCSIVolume,
			},
			expectErr: false,
		},
		{
			name:     "valid request nested baseDir with newTestVolumeIDNested",
			volumeID: newTestVolumeIDNested,
			resp: &nfsVolume{
				id:      newTestVolumeIDNested,
				server:  testServer,
				baseDir: testBaseDirNested,
				subDir:  testCSIVolume,
			},
			expectErr: false,
		},
		{
			name:     "valid request nested baseDir with newTestVolumeIDNested",
			volumeID: newTestVolumeIDUUID,
			resp: &nfsVolume{
				id:      newTestVolumeIDUUID,
				server:  testServer,
				baseDir: testBaseDir,
				subDir:  testCSIVolume,
				uuid:    "uuid",
			},
			expectErr: false,
		},
		{
			name:     "valid request nested ondelete retain",
			volumeID: newTestVolumeOnDeleteRetain,
			resp: &nfsVolume{
				id:       newTestVolumeOnDeleteRetain,
				server:   testServer,
				baseDir:  testBaseDir,
				subDir:   testCSIVolume,
				uuid:     "uuid",
				onDelete: "retain",
			},
			expectErr: false,
		},
		{
			name:     "valid request nested ondelete delete",
			volumeID: newTestVolumeOnDeleteDelete,
			resp: &nfsVolume{
				id:       newTestVolumeOnDeleteDelete,
				server:   testServer,
				baseDir:  testBaseDir,
				subDir:   testCSIVolume,
				uuid:     "uuid",
				onDelete: "delete",
			},
			expectErr: false,
		},
		{
			name:     "valid request nested ondelete archive",
			volumeID: newTestVolumeOnDeleteArchive,
			resp: &nfsVolume{
				id:       newTestVolumeOnDeleteArchive,
				server:   testServer,
				baseDir:  testBaseDir,
				subDir:   testCSIVolume,
				uuid:     "",
				onDelete: "archive",
			},
			expectErr: false,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			resp, err := getNfsVolFromID(test.volumeID)

			if !test.expectErr && err != nil {
				t.Errorf("test %q failed: %v", test.name, err)
			}
			if test.expectErr && err == nil {
				t.Errorf("test %q failed; got success", test.name)
			}
			if !reflect.DeepEqual(resp, test.resp) {
				t.Errorf("test %q failed: got resp %+v, expected %+v", test.name, resp, test.resp)
			}
		})
	}
}

func TestIsValidVolumeCapabilities(t *testing.T) {
	mountVolumeCapabilities := []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
		},
	}
	blockVolumeCapabilities := []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Block{
				Block: &csi.VolumeCapability_BlockVolume{},
			},
		},
	}

	cases := []struct {
		desc      string
		volCaps   []*csi.VolumeCapability
		expectErr error
	}{
		{
			volCaps:   mountVolumeCapabilities,
			expectErr: nil,
		},
		{
			volCaps:   blockVolumeCapabilities,
			expectErr: fmt.Errorf("block volume capability not supported"),
		},
		{
			volCaps:   []*csi.VolumeCapability{},
			expectErr: fmt.Errorf("volume capabilities missing in request"),
		},
	}

	for _, test := range cases {
		err := isValidVolumeCapabilities(test.volCaps)
		if !reflect.DeepEqual(err, test.expectErr) {
			t.Errorf("[test: %s] Unexpected error: %v, expected error: %v", test.desc, err, test.expectErr)
		}
	}
}

func TestGetInternalMountPath(t *testing.T) {
	cases := []struct {
		desc            string
		workingMountDir string
		vol             *nfsVolume
		result          string
	}{
		{
			desc:            "nil volume",
			workingMountDir: "/tmp",
			result:          "",
		},
		{
			desc:            "uuid not empty",
			workingMountDir: "/tmp",
			vol: &nfsVolume{
				subDir: "subdir",
				uuid:   "uuid",
			},
			result: filepath.Join("/tmp", "uuid"),
		},
		{
			desc:            "uuid empty",
			workingMountDir: "/tmp",
			vol: &nfsVolume{
				subDir: "subdir",
				uuid:   "",
			},
			result: filepath.Join("/tmp", "subdir"),
		},
	}

	for _, test := range cases {
		path := getInternalMountPath(test.workingMountDir, test.vol)
		assert.Equal(t, path, test.result)
	}
}

func TestNewNFSVolume(t *testing.T) {
	cases := []struct {
		desc      string
		name      string
		size      int64
		params    map[string]string
		expectVol *nfsVolume
		expectErr error
	}{
		{
			desc: "subDir is specified",
			name: "pv-name",
			size: 100,
			params: map[string]string{
				paramServer: "//nfs-server.default.svc.cluster.local",
				paramShare:  "share",
				paramSubDir: "subdir",
			},
			expectVol: &nfsVolume{
				id:       "nfs-server.default.svc.cluster.local#share#subdir#pv-name#",
				server:   "//nfs-server.default.svc.cluster.local",
				baseDir:  "share",
				subDir:   "subdir",
				size:     100,
				uuid:     "pv-name",
				onDelete: "delete",
			},
		},
		{
			desc: "subDir with pv/pvc metadata is specified",
			name: "pv-name",
			size: 100,
			params: map[string]string{
				paramServer:     "//nfs-server.default.svc.cluster.local",
				paramShare:      "share",
				paramSubDir:     fmt.Sprintf("subdir-%s-%s-%s", pvcNameMetadata, pvcNamespaceMetadata, pvNameMetadata),
				pvcNameKey:      "pvcname",
				pvcNamespaceKey: "pvcnamespace",
				pvNameKey:       "pvname",
			},
			expectVol: &nfsVolume{
				id:       "nfs-server.default.svc.cluster.local#share#subdir-pvcname-pvcnamespace-pvname#pv-name#",
				server:   "//nfs-server.default.svc.cluster.local",
				baseDir:  "share",
				subDir:   "subdir-pvcname-pvcnamespace-pvname",
				size:     100,
				uuid:     "pv-name",
				onDelete: "delete",
			},
		},
		{
			desc: "subDir not specified",
			name: "pv-name",
			size: 200,
			params: map[string]string{
				paramServer: "//nfs-server.default.svc.cluster.local",
				paramShare:  "share",
			},
			expectVol: &nfsVolume{
				id:       "nfs-server.default.svc.cluster.local#share#pv-name##",
				server:   "//nfs-server.default.svc.cluster.local",
				baseDir:  "share",
				subDir:   "pv-name",
				size:     200,
				uuid:     "",
				onDelete: "delete",
			},
		},
		{
			desc:      "server value is empty",
			params:    map[string]string{},
			expectVol: nil,
			expectErr: fmt.Errorf("%s is a required parameter", paramServer),
		},
		{
			desc: "invalid onDelete value",
			params: map[string]string{
				paramServer:   "//nfs-server.default.svc.cluster.local",
				paramShare:    "share",
				paramOnDelete: "invalid",
			},
			expectVol: nil,
			expectErr: fmt.Errorf("invalid value %s for OnDelete, supported values are %v", "invalid", supportedOnDeleteValues),
		},
	}

	for _, test := range cases {
		vol, err := newNFSVolume(test.name, test.size, test.params, "delete")
		if !reflect.DeepEqual(err, test.expectErr) {
			t.Errorf("[test: %s] Unexpected error: %v, expected error: %v", test.desc, err, test.expectErr)
		}
		if !reflect.DeepEqual(vol, test.expectVol) {
			t.Errorf("[test: %s] Unexpected vol: %v, expected vol: %v", test.desc, vol, test.expectVol)
		}
	}
}

func TestCopyVolume(t *testing.T) {
	cases := []struct {
		desc      string
		req       *csi.CreateVolumeRequest
		dstVol    *nfsVolume
		expectErr bool
		prepare   func() error
		cleanup   func() error
	}{
		{
			desc: "copy volume from valid volume",
			req: &csi.CreateVolumeRequest{
				Name: "snapshot-name",
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "nfs-server.default.svc.cluster.local#share#subdir#src-pv-name",
						},
					},
				},
			},
			dstVol: &nfsVolume{
				id:      "nfs-server.default.svc.cluster.local#share#subdir#dst-pv-name",
				server:  "//nfs-server.default.svc.cluster.local",
				baseDir: "share",
				subDir:  "subdir",
				uuid:    "dst-pv-name",
			},
			prepare: func() error { return os.MkdirAll("/tmp/src-pv-name/subdir", 0777) },
			cleanup: func() error { return os.RemoveAll("/tmp/src-pv-name") },
		},
		{
			desc: "copy volume from valid snapshot",
			req: &csi.CreateVolumeRequest{
				Name: "snapshot-name",
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: "nfs-server.default.svc.cluster.local#share#snapshot-name#snapshot-name#src-pv-name",
						},
					},
				},
			},
			dstVol: &nfsVolume{
				id:      "nfs-server.default.svc.cluster.local#share#subdir#dst-pv-name",
				server:  "//nfs-server.default.svc.cluster.local",
				baseDir: "share",
				subDir:  "subdir",
				uuid:    "dst-pv-name",
			},
			prepare: func() error {
				if err := os.MkdirAll("/tmp/snapshot-name/snapshot-name", 0777); err != nil {
					return err
				}
				file, err := os.Create("/tmp/snapshot-name/snapshot-name/src-pv-name.tar.gz")
				if err != nil {
					return err
				}
				defer file.Close()
				gzipWriter := gzip.NewWriter(file)
				defer gzipWriter.Close()
				tarWriter := tar.NewWriter(gzipWriter)
				defer tarWriter.Close()
				body := "test file"
				hdr := &tar.Header{
					Name: "test.txt",
					Mode: 0777,
					Size: int64(len(body)),
				}
				if err := tarWriter.WriteHeader(hdr); err != nil {
					return err
				}
				if _, err := tarWriter.Write([]byte(body)); err != nil {
					return err
				}
				return nil
			},
			cleanup: func() error { return os.RemoveAll("/tmp/snapshot-name") },
		},
		{
			desc: "copy volume missing source id",
			req: &csi.CreateVolumeRequest{
				Name: "snapshot-name",
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "nfs-server.default.svc.cluster.local#share#subdir#src-pv-name",
						},
					},
				},
			},
			dstVol: &nfsVolume{
				server:  "//nfs-server.default.svc.cluster.local",
				baseDir: "share",
				subDir:  "subdir",
				uuid:    "dst-pv-name",
			},
			expectErr: true,
		},
		{
			desc: "copy volume missing dst",
			req: &csi.CreateVolumeRequest{
				Name: "snapshot-name",
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{},
					},
				},
			},
			dstVol: &nfsVolume{
				id:      "nfs-server.default.svc.cluster.local#share#subdir#dst-pv-name",
				server:  "//nfs-server.default.svc.cluster.local",
				baseDir: "share",
				subDir:  "subdir",
				uuid:    "dst-pv-name",
			},
			expectErr: true,
		},
		{
			desc: "copy volume from broken snapshot",
			req: &csi.CreateVolumeRequest{
				Name: "snapshot-name",
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{
							SnapshotId: "nfs-server.default.svc.cluster.local#share#snapshot-name#snapshot-name#src-pv-name",
						},
					},
				},
			},
			dstVol: &nfsVolume{
				id:      "nfs-server.default.svc.cluster.local#share#subdir#dst-pv-name",
				server:  "//nfs-server.default.svc.cluster.local",
				baseDir: "share",
				subDir:  "subdir",
				uuid:    "dst-pv-name",
			},
			expectErr: true,
		},
		{
			desc: "copy volume from missing snapshot",
			req: &csi.CreateVolumeRequest{
				Name: "snapshot-name",
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{},
					},
				},
			},
			dstVol: &nfsVolume{
				id:      "nfs-server.default.svc.cluster.local#share#subdir#dst-pv-name",
				server:  "//nfs-server.default.svc.cluster.local",
				baseDir: "share",
				subDir:  "subdir",
				uuid:    "dst-pv-name",
			},
			expectErr: true,
		},
		{
			desc: "copy volume from snapshot into missing dst volume",
			req: &csi.CreateVolumeRequest{
				Name: "snapshot-name",
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{
						Snapshot: &csi.VolumeContentSource_SnapshotSource{},
					},
				},
			},
			dstVol: &nfsVolume{
				server:  "//nfs-server.default.svc.cluster.local",
				baseDir: "share",
				subDir:  "subdir",
				uuid:    "dst-pv-name",
			},
			expectErr: true,
		},
	}
	for _, test := range cases {
		t.Run(test.desc, func(t *testing.T) {
			if test.prepare != nil {
				if err := test.prepare(); err != nil {
					t.Errorf(`[test: %s] prepare failed: "%v"`, test.desc, err)
				}
			}
			cs := initTestController(t)
			err := cs.copyVolume(context.TODO(), test.req, test.dstVol)
			if (err == nil) == test.expectErr {
				t.Errorf(`[test: %s] Error expectation mismatch, expected error: "%v", received: %q`, test.desc, test.expectErr, err)
			}
			if test.cleanup != nil {
				if err := test.cleanup(); err != nil {
					t.Errorf(`[test: %s] cleanup failed: "%v"`, test.desc, err)
				}
			}
		})
	}
}

func TestCreateSnapshot(t *testing.T) {
	cases := []struct {
		desc      string
		req       *csi.CreateSnapshotRequest
		expResp   *csi.CreateSnapshotResponse
		expectErr bool
		prepare   func() error
		cleanup   func() error
	}{
		{
			desc: "create snapshot with valid request",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "nfs-server.default.svc.cluster.local#share#subdir#src-pv-name",
				Name:           "snapshot-name",
				Parameters:     map[string]string{"mountOptions": "nfsvers=4.1,sec=sys"},
			},
			expResp: &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SnapshotId:     "nfs-server.default.svc.cluster.local#share#snapshot-name#snapshot-name#src-pv-name",
					SourceVolumeId: "nfs-server.default.svc.cluster.local#share#subdir#src-pv-name",
					ReadyToUse:     true,
					SizeBytes:      1,                 // doesn't match exact size, just denotes non-zero size expected
					CreationTime:   timestamppb.Now(), // doesn't match exact timestamp, just denotes non-zero ts expected
				},
			},
			prepare: func() error { return os.MkdirAll("/tmp/src-pv-name/subdir", 0777) },
			cleanup: func() error { return os.RemoveAll("/tmp/src-pv-name") },
		},
		{
			desc: "create snapshot from nonexisting volume",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "nfs-server.default.svc.cluster.local#share#subdir#src-pv-name",
				Name:           "snapshot-name",
			},
			expectErr: true,
		},
		{
			desc: "create snapshot with non supported parameters",
			req: &csi.CreateSnapshotRequest{
				SourceVolumeId: "nfs-server.default.svc.cluster.local#share#subdir#src-pv-name",
				Name:           "snapshot-name",
				Parameters:     map[string]string{"unknown": "value"},
			},
			expectErr: true,
		},
	}
	for _, test := range cases {
		t.Run(test.desc, func(t *testing.T) {
			if test.prepare != nil {
				if err := test.prepare(); err != nil {
					t.Errorf(`[test: %s] prepare failed: "%v"`, test.desc, err)
				}
			}
			cs := initTestController(t)
			resp, err := cs.CreateSnapshot(context.TODO(), test.req)
			if (err == nil) == test.expectErr {
				t.Errorf(`[test: %s] Error expectation mismatch, expected error: "%v", received: %q`, test.desc, test.expectErr, err)
			}
			if err := matchCreateSnapshotResponse(test.expResp, resp); err != nil {
				t.Errorf("[test: %s] failed %q: got resp %+v, expected %+v", test.desc, err, resp, test.expResp)
			}
			if test.cleanup != nil {
				if err := test.cleanup(); err != nil {
					t.Errorf(`[test: %s] cleanup failed: "%v"`, test.desc, err)
				}
			}
		})
	}
}

func TestDeleteSnapshot(t *testing.T) {
	cases := []struct {
		desc      string
		req       *csi.DeleteSnapshotRequest
		expResp   *csi.DeleteSnapshotResponse
		expectErr bool
		prepare   func() error
		cleanup   func() error
	}{
		{
			desc: "delete valid snapshot",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: "nfs-server.default.svc.cluster.local#share#snapshot-name#snapshot-name#src-pv-name",
			},
			expResp: &csi.DeleteSnapshotResponse{},
			prepare: func() error {
				if err := os.MkdirAll("/tmp/snapshot-name/snapshot-name/", 0777); err != nil {
					return err
				}
				f, err := os.OpenFile("/tmp/snapshot-name/snapshot-name/src-pv-name.tar.gz", os.O_CREATE, 0777)
				if err != nil {
					return err
				}
				return f.Close()
			},
			cleanup: func() error { return os.RemoveAll("/tmp/snapshot-name") },
		},
		{
			desc: "delete nonexisting snapshot",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: "nfs-server.default.svc.cluster.local#share#snapshot-name#snapshot-name#src-pv-name",
			},
			expResp: &csi.DeleteSnapshotResponse{},
		},
		{
			desc: "delete snapshot with improper id",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: "incorrect-snap-id",
			},
			expResp: &csi.DeleteSnapshotResponse{},
		},
		{
			desc: "delete valid snapshot with mount options",
			req: &csi.DeleteSnapshotRequest{
				SnapshotId: "nfs-server.default.svc.cluster.local#share#snapshot-name#snapshot-name#src-pv-name",
				Secrets:    map[string]string{"mountoptions": "nfsvers=4.1"},
			},
			expResp: &csi.DeleteSnapshotResponse{},
			prepare: func() error {
				if err := os.MkdirAll("/tmp/snapshot-name/snapshot-name/", 0777); err != nil {
					return err
				}
				f, err := os.OpenFile("/tmp/snapshot-name/snapshot-name/src-pv-name.tar.gz", os.O_CREATE, 0777)
				if err != nil {
					return err
				}
				return f.Close()
			},
			cleanup: func() error { return os.RemoveAll("/tmp/snapshot-name") },
		},
	}
	for _, test := range cases {
		t.Run(test.desc, func(t *testing.T) {
			if test.prepare != nil {
				if err := test.prepare(); err != nil {
					t.Errorf(`[test: %s] prepare failed: "%v"`, test.desc, err)
				}
			}
			cs := initTestController(t)
			resp, err := cs.DeleteSnapshot(context.TODO(), test.req)
			if (err == nil) == test.expectErr {
				t.Errorf(`[test: %s] Error expectation mismatch, expected error: "%v", received: %q`, test.desc, test.expectErr, err)
			}
			if !reflect.DeepEqual(test.expResp, resp) {
				t.Errorf("[test: %s] got resp %+v, expected %+v", test.desc, resp, test.expResp)
			}
			if test.cleanup != nil {
				if err := test.cleanup(); err != nil {
					t.Errorf(`[test: %s] cleanup failed: "%v"`, test.desc, err)
				}
			}
		})
	}
}

func TestControllerExpandVolume(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "volume ID missing",
			testFunc: func(t *testing.T) {
				d := initTestController(t)
				req := &csi.ControllerExpandVolumeRequest{}
				_, err := d.ControllerExpandVolume(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Volume ID missing in request")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "Capacity Range missing",
			testFunc: func(t *testing.T) {
				d := initTestController(t)
				req := &csi.ControllerExpandVolumeRequest{
					VolumeId: "unit-test",
				}
				_, err := d.ControllerExpandVolume(context.Background(), req)
				expectedErr := status.Error(codes.InvalidArgument, "Capacity Range missing in request")
				if !reflect.DeepEqual(err, expectedErr) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, expectedErr)
				}
			},
		},
		{
			name: "Error = nil",
			testFunc: func(t *testing.T) {
				d := initTestController(t)
				req := &csi.ControllerExpandVolumeRequest{
					VolumeId: "unit-test",
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: 10000,
					},
				}
				_, err := d.ControllerExpandVolume(context.Background(), req)
				if !reflect.DeepEqual(err, nil) {
					t.Errorf("actualErr: (%v), expectedErr: (%v)", err, nil)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func matchCreateSnapshotResponse(e, r *csi.CreateSnapshotResponse) error {
	if e == nil && r == nil {
		return nil
	}
	if e == nil || e.Snapshot == nil {
		return fmt.Errorf("expected nil response")
	}
	if r == nil || r.Snapshot == nil {
		return fmt.Errorf("unexpected nil response")
	}
	es, rs := e.Snapshot, r.Snapshot

	var errs []string
	// comparing ts and size just for presence, not the exact value
	if es.CreationTime.IsValid() != rs.CreationTime.IsValid() {
		errs = append(errs, "CreationTime")
	}
	if (es.SizeBytes == 0) != (rs.SizeBytes == 0) {
		errs = append(errs, "SizeBytes")
	}
	// comparing remaining fields for exact match
	if es.ReadyToUse != rs.ReadyToUse {
		errs = append(errs, "ReadyToUse")
	}
	if es.SnapshotId != rs.SnapshotId {
		errs = append(errs, "SnapshotId")
	}
	if es.SourceVolumeId != rs.SourceVolumeId {
		errs = append(errs, "SourceVolumeId")
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("mismatch CreateSnapshotResponse in fields: %v", strings.Join(errs, ", "))
}
