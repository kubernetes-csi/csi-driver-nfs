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
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-driver-nfs/test/utils/testutil"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	targetTest = "./target_test"
)

func TestNodePublishVolume(t *testing.T) {
	ns, err := getTestNodeServer()
	if err != nil {
		t.Fatalf(err.Error())
	}

	params := map[string]string{
		"server":              "server",
		"share":               "share",
		mountPermissionsField: "0755",
	}
	paramsWithMetadata := map[string]string{
		"server":        "server",
		"share":         "share",
		pvcNameKey:      "pvcname",
		pvcNamespaceKey: "pvcnamespace",
		pvNameKey:       "pvname",
	}
	paramsWithZeroPermissions := map[string]string{
		"server":              "server",
		"share":               "share",
		mountPermissionsField: "0",
	}

	invalidParams := map[string]string{
		"server":              "server",
		"share":               "share",
		mountPermissionsField: "07ab",
	}

	volumeCap := csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}
	alreadyMountedTarget := testutil.GetWorkDirPath("false_is_likely_exist_target", t)
	targetTest := testutil.GetWorkDirPath("target_test", t)

	tests := []struct {
		desc          string
		setup         func()
		req           csi.NodePublishVolumeRequest
		skipOnWindows bool
		expectedErr   error
		cleanup       func()
	}{
		{
			desc:        "[Error] Volume capabilities missing",
			req:         csi.NodePublishVolumeRequest{},
			expectedErr: status.Error(codes.InvalidArgument, "Volume capability missing in request"),
		},
		{
			desc:        "[Error] Volume ID missing",
			req:         csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap}},
			expectedErr: status.Error(codes.InvalidArgument, "Volume ID missing in request"),
		},
		{
			desc: "[Error] Target path missing",
			req: csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId: "vol_1"},
			expectedErr: status.Error(codes.InvalidArgument, "Target path not provided"),
		},
		{
			desc: "[Success] Stage target path missing",
			req: csi.NodePublishVolumeRequest{
				VolumeContext:    params,
				VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:         "vol_1",
				TargetPath:       targetTest},
			expectedErr: nil,
		},
		{
			desc: "[Success] Valid request read only",
			req: csi.NodePublishVolumeRequest{
				VolumeContext:    params,
				VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:         "vol_1",
				TargetPath:       targetTest,
				Readonly:         true},
			expectedErr: nil,
		},
		{
			desc: "[Success] Valid request already mounted",
			req: csi.NodePublishVolumeRequest{
				VolumeContext:    params,
				VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:         "vol_1",
				TargetPath:       alreadyMountedTarget,
				Readonly:         true},
			expectedErr: nil,
		},
		{
			desc: "[Success] Valid request",
			req: csi.NodePublishVolumeRequest{
				VolumeContext:    params,
				VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:         "vol_1",
				TargetPath:       targetTest,
				Readonly:         true},
			expectedErr: nil,
		},
		{
			desc: "[Success] Valid request with pv/pvc metadata",
			req: csi.NodePublishVolumeRequest{
				VolumeContext:    paramsWithMetadata,
				VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:         "vol_1",
				TargetPath:       targetTest,
				Readonly:         true},
			expectedErr: nil,
		},
		{
			desc: "[Success] Valid request with 0 mountPermissions",
			req: csi.NodePublishVolumeRequest{
				VolumeContext:    paramsWithZeroPermissions,
				VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:         "vol_1",
				TargetPath:       targetTest,
				Readonly:         true},
			expectedErr: nil,
		},
		{
			desc: "[Error] invalid mountPermissions",
			req: csi.NodePublishVolumeRequest{
				VolumeContext:    invalidParams,
				VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:         "vol_1",
				TargetPath:       targetTest,
				Readonly:         true},
			expectedErr: status.Error(codes.InvalidArgument, "invalid mountPermissions 07ab"),
		},
	}

	// setup
	_ = makeDir(alreadyMountedTarget)
	_ = makeDir(targetTest)

	for _, tc := range tests {
		if tc.setup != nil {
			tc.setup()
		}
		_, err := ns.NodePublishVolume(context.Background(), &tc.req)
		if !reflect.DeepEqual(err, tc.expectedErr) {
			t.Errorf("Desc:%v\nUnexpected error: %v\nExpected: %v", tc.desc, err, tc.expectedErr)
		}
		if tc.cleanup != nil {
			tc.cleanup()
		}
	}

	// Clean up
	err = os.RemoveAll(targetTest)
	assert.NoError(t, err)
	err = os.RemoveAll(alreadyMountedTarget)
	assert.NoError(t, err)

}

func TestNodeUnpublishVolume(t *testing.T) {
	ns, err := getTestNodeServer()
	if err != nil {
		t.Fatalf(err.Error())
	}

	errorTarget := testutil.GetWorkDirPath("error_is_likely_target", t)
	targetTest := testutil.GetWorkDirPath("target_test", t)
	targetFile := testutil.GetWorkDirPath("abc.go", t)

	tests := []struct {
		desc        string
		setup       func()
		req         csi.NodeUnpublishVolumeRequest
		expectedErr error
		cleanup     func()
	}{
		{
			desc:        "[Error] Volume ID missing",
			req:         csi.NodeUnpublishVolumeRequest{TargetPath: targetTest},
			expectedErr: status.Error(codes.InvalidArgument, "Volume ID missing in request"),
		},
		{
			desc:        "[Error] Target missing",
			req:         csi.NodeUnpublishVolumeRequest{VolumeId: "vol_1"},
			expectedErr: status.Error(codes.InvalidArgument, "Target path missing in request"),
		},
		{
			desc:        "[Error] Unmount error mocked by IsLikelyNotMountPoint",
			req:         csi.NodeUnpublishVolumeRequest{TargetPath: errorTarget, VolumeId: "vol_1"},
			expectedErr: fmt.Errorf("fake IsLikelyNotMountPoint: fake error"),
		},
		{
			desc: "[Success] Volume not mounted",
			req:  csi.NodeUnpublishVolumeRequest{TargetPath: targetFile, VolumeId: "vol_1"},
		},
	}

	// Setup
	_ = makeDir(errorTarget)

	for _, tc := range tests {
		if tc.setup != nil {
			tc.setup()
		}
		_, err := ns.NodeUnpublishVolume(context.Background(), &tc.req)
		if !reflect.DeepEqual(err, tc.expectedErr) {
			if err == nil || tc.expectedErr == nil || !strings.Contains(err.Error(), tc.expectedErr.Error()) {
				t.Errorf("Desc:%v\nUnexpected error: %v\nExpected: %v", tc.desc, err, tc.expectedErr)
			}
		}
		if tc.cleanup != nil {
			tc.cleanup()
		}
	}

	// Clean up
	err = os.RemoveAll(errorTarget)
	assert.NoError(t, err)
}

func TestNodeGetInfo(t *testing.T) {
	ns, err := getTestNodeServer()
	if err != nil {
		t.Fatalf(err.Error())
	}

	// Test valid request
	req := csi.NodeGetInfoRequest{}
	resp, err := ns.NodeGetInfo(context.Background(), &req)
	assert.NoError(t, err)
	assert.Equal(t, resp.GetNodeId(), fakeNodeID)
}

func TestNodeGetCapabilities(t *testing.T) {
	ns, err := getTestNodeServer()
	if err != nil {
		t.Fatalf(err.Error())
	}

	capType := &csi.NodeServiceCapability_Rpc{
		Rpc: &csi.NodeServiceCapability_RPC{
			Type: csi.NodeServiceCapability_RPC_UNKNOWN,
		},
	}

	capList := []*csi.NodeServiceCapability{{
		Type: capType,
	}}
	ns.Driver.nscap = capList

	// Test valid request
	req := csi.NodeGetCapabilitiesRequest{}
	resp, err := ns.NodeGetCapabilities(context.Background(), &req)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Capabilities[0].GetType(), capType)
	assert.NoError(t, err)
}

func getTestNodeServer() (NodeServer, error) {
	d := NewEmptyDriver("")
	mounter, err := NewFakeMounter()
	if err != nil {
		return NodeServer{}, errors.New("failed to get fake mounter")
	}
	return NodeServer{
		Driver:  d,
		mounter: mounter,
	}, nil
}

func TestNodeGetVolumeStats(t *testing.T) {
	nonexistedPath := "/not/a/real/directory"
	fakePath := "/tmp/fake-volume-path"

	tests := []struct {
		desc        string
		req         csi.NodeGetVolumeStatsRequest
		expectedErr error
	}{
		{
			desc:        "[Error] Volume ID missing",
			req:         csi.NodeGetVolumeStatsRequest{VolumePath: targetTest},
			expectedErr: status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume ID was empty"),
		},
		{
			desc:        "[Error] VolumePath missing",
			req:         csi.NodeGetVolumeStatsRequest{VolumeId: "vol_1"},
			expectedErr: status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume path was empty"),
		},
		{
			desc:        "[Error] Incorrect volume path",
			req:         csi.NodeGetVolumeStatsRequest{VolumePath: nonexistedPath, VolumeId: "vol_1"},
			expectedErr: status.Errorf(codes.NotFound, "path /not/a/real/directory does not exist"),
		},
		{
			desc:        "[Success] Standard success",
			req:         csi.NodeGetVolumeStatsRequest{VolumePath: fakePath, VolumeId: "vol_1"},
			expectedErr: nil,
		},
	}

	// Setup
	_ = makeDir(fakePath)
	ns, err := getTestNodeServer()
	if err != nil {
		t.Fatalf(err.Error())
	}

	for _, test := range tests {
		_, err := ns.NodeGetVolumeStats(context.Background(), &test.req)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("desc: %v, expected error: %v, actual error: %v", test.desc, test.expectedErr, err)
		}
	}

	// Clean up
	err = os.RemoveAll(fakePath)
	assert.NoError(t, err)
}
