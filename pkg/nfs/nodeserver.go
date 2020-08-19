/*
Copyright 2017 The Kubernetes Authors.

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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/golang/glog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/mount"
)

type nodeServer struct {
	Driver  *nfsDriver
	mounter mount.Interface
}

const (
	// Deadline for unmount. After this time, umount -f is performed.
	unmountTimeout = time.Minute
)

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	notMnt, err := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mo := req.GetVolumeCapability().GetMount().GetMountFlags()
	if req.GetReadonly() {
		mo = append(mo, "ro")
	}

	s := req.GetVolumeContext()["server"]
	ep := req.GetVolumeContext()["share"]
	source := fmt.Sprintf("%s:%s", s, ep)

	err = ns.mounter.Mount(source, targetPath, "nfs", mo)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if ns.Driver.perm != nil {
		if err := os.Chmod(targetPath, os.FileMode(*ns.Driver.perm)); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) IsNotMountPoint(path string) (bool, error) {
	mtab, err := ns.mounter.List()
	if err != nil {
		return false, err
	}

	for _, mnt := range mtab {
		// This is how a directory deleted on the NFS server looks like
		deletedDir := fmt.Sprintf("%s\\040(deleted)", mnt.Path)

		if mnt.Path == path || mnt.Path == deletedDir {
			return false, nil
		}
	}
	return true, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	glog.V(6).Infof("NodeUnpublishVolume started for %s", targetPath)

	notMnt, err := ns.IsNotMountPoint(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.V(4).Infof("NodeUnpublishVolume: path %s is *not* a mount point: %t", targetPath, notMnt)
	if !notMnt {

		err := ns.tryUnmount(targetPath)
		if err != nil {
			if err == context.DeadlineExceeded {
				glog.V(2).Infof("Timed out waiting for unmount of %s, trying with -f", targetPath)
				err = ns.forceUnmount(targetPath)
			}
		}
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		glog.V(2).Infof("Unmounted %s", targetPath)
	}

	if err := os.Remove(targetPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	glog.V(4).Infof("Cleaned %s", targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// tryUnmount calls plain "umount" and waits for unmountTimeout for it to finish.
func (ns *nodeServer) tryUnmount(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), unmountTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "umount", path)
	out, cmderr := cmd.CombinedOutput()

	// CombinedOutput() does not return DeadlineExceeded, make sure it's
	// propagated on timeout.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if cmderr != nil {
		return fmt.Errorf("failed to unmount volume: %s: %s", cmderr, string(out))
	}
	return nil
}

func (ns *nodeServer) forceUnmount(path string) error {
	cmd := exec.Command("umount", "-f", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to force-unmount volume: %s: %s", err, string(out))
	}
	return nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	glog.V(5).Infof("Using default NodeGetInfo")

	return &csi.NodeGetInfoResponse{
		NodeId: ns.Driver.nodeID,
	}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	glog.V(5).Infof("Using default NodeGetCapabilities")

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
		},
	}, nil
}

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, in *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
