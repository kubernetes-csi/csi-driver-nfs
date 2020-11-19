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
	"os"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-driver-nfs/test/utils/testutil"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNodePublishVolume(t *testing.T) {
	volumeCap := csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}
	alreadyMountedTarget := testutil.GetWorkDirPath("false_is_likely_exist_target", t)
	targetTest := testutil.GetWorkDirPath("target_test", t)

	tests := []struct {
		desc          string
		req           csi.NodePublishVolumeRequest
		skipOnWindows bool
		expectedErr   error
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
			req: csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:   "vol_1",
				TargetPath: targetTest},
			expectedErr: nil,
		},
		{
			desc: "[Success] Valid request read only",
			req: csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:   "vol_1",
				TargetPath: targetTest,
				Readonly:   true},
			expectedErr: nil,
		},
		{
			desc: "[Success] Valid request already mounted",
			req: csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:   "vol_1",
				TargetPath: alreadyMountedTarget,
				Readonly:   true},
			expectedErr: nil,
		},
		{
			desc: "[Success] Valid request",
			req: csi.NodePublishVolumeRequest{VolumeCapability: &csi.VolumeCapability{AccessMode: &volumeCap},
				VolumeId:   "vol_1",
				TargetPath: targetTest,
				Readonly:   true},
			expectedErr: nil,
		},
	}

	// setup
	_ = makeDir(alreadyMountedTarget)

	ns, err := getTestNodeServer()
	if err != nil {
		t.Fatalf(err.Error())
	}

	for _, tc := range tests {
		_, err := ns.NodePublishVolume(context.Background(), &tc.req)
		if !reflect.DeepEqual(err, tc.expectedErr) {
			t.Errorf("Desc:%v\nUnexpected error: %v\nExpected: %v", tc.desc, err, tc.expectedErr)
		}
	}

	// Clean up
	err = os.RemoveAll(targetTest)
	assert.NoError(t, err)
	err = os.RemoveAll(alreadyMountedTarget)
	assert.NoError(t, err)

}

func TestNodeUnpublishVolume(t *testing.T) {
	errorTarget := testutil.GetWorkDirPath("error_is_likely_target", t)
	targetTest := testutil.GetWorkDirPath("target_test", t)
	targetFile := testutil.GetWorkDirPath("abc.go", t)

	tests := []struct {
		desc        string
		req         csi.NodeUnpublishVolumeRequest
		expectedErr error
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
			expectedErr: status.Error(codes.Internal, "fake IsLikelyNotMountPoint: fake error"),
		},
		{
			desc:        "[Error] Volume not mounted",
			req:         csi.NodeUnpublishVolumeRequest{TargetPath: targetFile, VolumeId: "vol_1"},
			expectedErr: status.Error(codes.NotFound, "Volume not mounted"),
		},
	}

	// Setup
	_ = makeDir(errorTarget)

	ns, err := getTestNodeServer()
	if err != nil {
		t.Fatalf(err.Error())
	}

	for _, tc := range tests {
		_, err := ns.NodeUnpublishVolume(context.Background(), &tc.req)
		if !reflect.DeepEqual(err, tc.expectedErr) {
			t.Errorf("Desc:%v\nUnexpected error: %v\nExpected: %v", tc.desc, err, tc.expectedErr)
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
