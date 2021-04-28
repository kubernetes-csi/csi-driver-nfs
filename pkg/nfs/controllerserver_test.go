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
	"strings"
	"testing"

	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mount "k8s.io/mount-utils"
)

const (
	testServer         = "test-server"
	testBaseDir        = "test-base-dir"
	testBaseDirNested  = "test/base/dir"
	testCSIVolume      = "test-csi"
	testVolumeID       = "test-server/test-base-dir/test-csi"
	testVolumeIDNested = "test-server/test/base/dir/test-csi"
)

// for Windows support in the future
var (
	testShare = filepath.Join(string(filepath.Separator), testBaseDir, string(filepath.Separator), testCSIVolume)
)

func initTestController(t *testing.T) *ControllerServer {
	var perm *uint32
	mounter := &mount.FakeMounter{MountPoints: []mount.MountPoint{}}
	driver := NewNFSdriver("", "", perm)
	driver.ns = NewNodeServer(driver, mounter)
	cs := NewControllerServer(driver)
	cs.workingMountDir = "/tmp"
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
					paramServer: testServer,
					paramShare:  testBaseDir,
				},
			},
			resp: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId: testVolumeID,
					VolumeContext: map[string]string{
						paramServer: testServer,
						paramShare:  testShare,
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
			name: "invalid volume capability",
			req: &csi.CreateVolumeRequest{
				Name: testCSIVolume,
				VolumeCapabilities: []*csi.VolumeCapability{
					{
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
				info, err := os.Stat(filepath.Join(cs.workingMountDir, test.req.Name, test.req.Name))
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
		desc        string
		req         *csi.DeleteVolumeRequest
		resp        *csi.DeleteVolumeResponse
		expectedErr error
	}{
		{
			desc:        "Volume ID missing",
			req:         &csi.DeleteVolumeRequest{},
			resp:        nil,
			expectedErr: status.Error(codes.InvalidArgument, "Volume ID missing in request"),
		},
		{
			desc:        "Valid request",
			req:         &csi.DeleteVolumeRequest{VolumeId: testVolumeID},
			resp:        &csi.DeleteVolumeResponse{},
			expectedErr: nil,
		},
	}

	for _, test := range cases {
		test := test //pin
		t.Run(test.desc, func(t *testing.T) {
			// Setup
			cs := initTestController(t)
			_ = os.MkdirAll(filepath.Join(cs.workingMountDir, testCSIVolume), os.ModePerm)
			_, _ = os.Create(filepath.Join(cs.workingMountDir, testCSIVolume, testCSIVolume))

			// Run
			resp, err := cs.DeleteVolume(context.TODO(), test.req)

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
			if _, err := os.Stat(filepath.Join(cs.workingMountDir, testCSIVolume, testCSIVolume)); test.expectedErr == nil && !os.IsNotExist(err) {
				t.Errorf("test %q failed: expected volume subdirectory deleted, it still exists", test.desc)
			}
		})
	}
}

func TestValidateVolumeCapabilities(t *testing.T) {
	cases := []struct {
		desc        string
		req         *csi.ValidateVolumeCapabilitiesRequest
		resp        *csi.ValidateVolumeCapabilitiesResponse
		expectedErr error
	}{
		{
			desc:        "Volume ID missing",
			req:         &csi.ValidateVolumeCapabilitiesRequest{},
			resp:        nil,
			expectedErr: status.Error(codes.InvalidArgument, "Volume ID missing in request"),
		},
		{
			desc:        "Volume capabilities missing",
			req:         &csi.ValidateVolumeCapabilitiesRequest{VolumeId: testVolumeID},
			resp:        nil,
			expectedErr: status.Error(codes.InvalidArgument, "Volume capabilities missing in request"),
		},
		{
			desc: "valid request",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId: testVolumeID,
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
			},
			resp:        &csi.ValidateVolumeCapabilitiesResponse{Message: ""},
			expectedErr: nil,
		},
	}

	for _, test := range cases {
		test := test //pin
		t.Run(test.desc, func(t *testing.T) {
			// Setup
			cs := initTestController(t)

			// Run
			resp, err := cs.ValidateVolumeCapabilities(context.TODO(), test.req)

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
		req       string
		resp      *nfsVolume
		expectErr bool
	}{
		{
			name:      "ID only server",
			req:       testServer,
			resp:      nil,
			expectErr: true,
		},
		{
			name:      "ID missing subDir",
			req:       strings.Join([]string{testServer, testBaseDir}, "/"),
			resp:      nil,
			expectErr: true,
		},
		{
			name: "valid request single baseDir",
			req:  testVolumeID,
			resp: &nfsVolume{
				id:      testVolumeID,
				server:  testServer,
				baseDir: testBaseDir,
				subDir:  testCSIVolume,
			},
			expectErr: false,
		},
		{
			name: "valid request nested baseDir",
			req:  testVolumeIDNested,
			resp: &nfsVolume{
				id:      testVolumeIDNested,
				server:  testServer,
				baseDir: testBaseDirNested,
				subDir:  testCSIVolume,
			},
			expectErr: false,
		},
	}

	for _, test := range cases {
		test := test //pin
		t.Run(test.name, func(t *testing.T) {
			// Setup
			cs := initTestController(t)

			// Run
			resp, err := cs.getNfsVolFromID(test.req)

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
		})
	}
}
