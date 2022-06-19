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
	mount "k8s.io/mount-utils"
)

const (
	testServer            = "test-server"
	testBaseDir           = "test-base-dir"
	testBaseDirNested     = "test/base/dir"
	testCSIVolume         = "volume-name"
	testVolumeID          = "test-server/test-base-dir/volume-name"
	newTestVolumeID       = "test-server#test-base-dir#volume-name#"
	testVolumeIDNested    = "test-server/test/base/dir/volume-name"
	newTestVolumeIDNested = "test-server#test/base/dir#volume-name#"
	newTestVolumeIDUUID   = "test-server#test-base-dir#volume-name#uuid"
)

func initTestController(t *testing.T) *ControllerServer {
	mounter := &mount.FakeMounter{MountPoints: []mount.MountPoint{}}
	driver := NewDriver(&DriverOptions{
		WorkingMountDir:  "/tmp",
		MountPermissions: 0777,
	})
	driver.ns = NewNodeServer(driver, mounter)
	cs := NewControllerServer(driver)
	return cs
}

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
					VolumeId: newTestVolumeID + testCSIVolume,
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
		test := test //pin
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
		desc          string
		testOnWindows bool
		req           *csi.DeleteVolumeRequest
		resp          *csi.DeleteVolumeResponse
		expectedErr   error
	}{
		{
			desc:          "Volume ID missing",
			testOnWindows: true,
			req:           &csi.DeleteVolumeRequest{},
			resp:          nil,
			expectedErr:   status.Error(codes.InvalidArgument, "Volume ID missing in request"),
		},
		{
			desc:          "Valid request",
			testOnWindows: false,
			req:           &csi.DeleteVolumeRequest{VolumeId: testVolumeID},
			resp:          &csi.DeleteVolumeResponse{},
			expectedErr:   nil,
		},
		{
			desc:          "Valid request with newTestVolumeID",
			testOnWindows: true,
			req:           &csi.DeleteVolumeRequest{VolumeId: newTestVolumeID},
			resp:          &csi.DeleteVolumeResponse{},
			expectedErr:   nil,
		},
	}

	for _, test := range cases {
		test := test //pin
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
			if _, err := os.Stat(filepath.Join(cs.Driver.workingMountDir, testCSIVolume, testCSIVolume)); test.expectedErr == nil && !os.IsNotExist(err) {
				t.Errorf("test %q failed: expected volume subdirectory deleted, it still exists", test.desc)
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
				},
			},
			expectedErr: nil,
		},
	}

	for _, test := range cases {
		test := test //pin
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
				id:      newTestVolumeID,
				server:  testServer,
				baseDir: testBaseDir,
				subDir:  testCSIVolume,
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
	}

	for _, test := range cases {
		test := test //pin
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
				id:      "nfs-server.default.svc.cluster.local#share#subdir#pv-name",
				server:  "//nfs-server.default.svc.cluster.local",
				baseDir: "share",
				subDir:  "subdir",
				size:    100,
				uuid:    "pv-name",
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
				id:      "nfs-server.default.svc.cluster.local#share#subdir-pvcname-pvcnamespace-pvname#pv-name",
				server:  "//nfs-server.default.svc.cluster.local",
				baseDir: "share",
				subDir:  "subdir-pvcname-pvcnamespace-pvname",
				size:    100,
				uuid:    "pv-name",
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
				id:      "nfs-server.default.svc.cluster.local#share#pv-name#",
				server:  "//nfs-server.default.svc.cluster.local",
				baseDir: "share",
				subDir:  "pv-name",
				size:    200,
				uuid:    "",
			},
		},
		{
			desc:      "server value is empty",
			params:    map[string]string{},
			expectVol: nil,
			expectErr: fmt.Errorf("%s is a required parameter", paramServer),
		},
	}

	for _, test := range cases {
		vol, err := newNFSVolume(test.name, test.size, test.params)
		if !reflect.DeepEqual(err, test.expectErr) {
			t.Errorf("[test: %s] Unexpected error: %v, expected error: %v", test.desc, err, test.expectErr)
		}
		if !reflect.DeepEqual(vol, test.expectVol) {
			t.Errorf("[test: %s] Unexpected vol: %v, expected vol: %v", test.desc, vol, test.expectVol)
		}
	}
}
