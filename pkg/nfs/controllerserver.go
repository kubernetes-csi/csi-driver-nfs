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
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	azcache "sigs.k8s.io/cloud-provider-azure/pkg/cache"

	"k8s.io/klog/v2"
)

// ControllerServer controller server setting
type ControllerServer struct {
	Driver *Driver
	csi.UnimplementedControllerServer
}

// nfsVolume is an internal representation of a volume
// created by the provisioner.
type nfsVolume struct {
	// Volume id
	id string
	// Address of the NFS server.
	// Matches paramServer.
	server string
	// Base directory of the NFS server to create volumes under
	// Matches paramShare.
	baseDir string
	// Subdirectory of the NFS server to create volumes under
	subDir string
	// size of volume
	size int64
	// pv name when subDir is not empty
	uuid string
	// on delete action
	onDelete string
}

// nfsSnapshot is an internal representation of a volume snapshot
// created by the provisioner.
type nfsSnapshot struct {
	// Snapshot id.
	id string
	// Address of the NFS server.
	// Matches paramServer.
	server string
	// Base directory of the NFS server to create snapshots under
	// Matches paramShare.
	baseDir string
	// Snapshot name.
	uuid string
	// Source volume.
	src string
}

func (snap nfsSnapshot) archiveName() string {
	return fmt.Sprintf("%v.tar.gz", snap.src)
}

// Ordering of elements in the CSI volume id.
// ID is of the form {server}/{baseDir}/{subDir}.
// TODO: This volume id format limits baseDir and
// subDir to only be one directory deep.
// Adding a new element should always go at the end
// before totalIDElements
const (
	idServer = iota
	idBaseDir
	idSubDir
	idUUID
	idOnDelete
	totalIDElements // Always last
)

// Ordering of elements in the CSI snapshot id.
// ID is of the form {server}/{baseDir}/{snapName}/{srcVolumeName}.
// Adding a new element should always go at the end
// before totalSnapIDElements
const (
	idSnapServer = iota
	idSnapBaseDir
	idSnapUUID
	idSnapArchivePath
	idSnapArchiveName
	totalIDSnapElements // Always last
)

// CreateVolume create a volume
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	name := req.GetName()
	if len(name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume name must be provided")
	}

	if err := isValidVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	mountPermissions := cs.Driver.mountPermissions
	reqCapacity := req.GetCapacityRange().GetRequiredBytes()
	parameters := req.GetParameters()
	if parameters == nil {
		parameters = make(map[string]string)
	}
	// validate parameters (case-insensitive)
	for k, v := range parameters {
		switch strings.ToLower(k) {
		case paramServer:
		case paramShare:
		case paramSubDir:
		case paramOnDelete:
		case pvcNamespaceKey:
		case pvcNameKey:
		case pvNameKey:
			// no op
		case mountPermissionsField:
			if v != "" {
				var err error
				if mountPermissions, err = strconv.ParseUint(v, 8, 32); err != nil {
					return nil, status.Errorf(codes.InvalidArgument, "invalid mountPermissions %s in storage class", v)
				}
			}
		default:
			return nil, status.Errorf(codes.InvalidArgument, "invalid parameter %q in storage class", k)
		}
	}

	if acquired := cs.Driver.volumeLocks.TryAcquire(name); !acquired {
		return nil, status.Errorf(codes.Aborted, volumeOperationAlreadyExistsFmt, name)
	}
	defer cs.Driver.volumeLocks.Release(name)

	nfsVol, err := newNFSVolume(name, reqCapacity, parameters, cs.Driver.defaultOnDeletePolicy)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var volCap *csi.VolumeCapability
	if len(req.GetVolumeCapabilities()) > 0 {
		volCap = req.GetVolumeCapabilities()[0]
	}
	// Mount nfs base share so we can create a subdirectory
	if err = cs.internalMount(ctx, nfsVol, parameters, volCap); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mount nfs server: %v", err)
	}
	defer func() {
		if err = cs.internalUnmount(ctx, nfsVol); err != nil {
			klog.Warningf("failed to unmount nfs server: %v", err)
		}
	}()

	// Create subdirectory under base-dir
	internalVolumePath := getInternalVolumePath(cs.Driver.workingMountDir, nfsVol)
	if err = os.MkdirAll(internalVolumePath, 0777); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to make subdirectory: %v", err)
	}

	if mountPermissions > 0 {
		// Reset directory permissions because of umask problems
		if err = os.Chmod(internalVolumePath, os.FileMode(mountPermissions)); err != nil {
			klog.Warningf("failed to chmod subdirectory: %v", err)
		}
	}

	if req.GetVolumeContentSource() != nil {
		if err := cs.copyVolume(ctx, req, nfsVol); err != nil {
			return nil, err
		}
	}

	setKeyValueInMap(parameters, paramSubDir, nfsVol.subDir)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      nfsVol.id,
			CapacityBytes: 0, // by setting it to zero, Provisioner will use PVC requested size as PV size
			VolumeContext: parameters,
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

// DeleteVolume delete a volume
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is empty")
	}
	nfsVol, err := getNfsVolFromID(volumeID)
	if err != nil {
		// An invalid ID should be treated as doesn't exist
		klog.Warningf("failed to get nfs volume for volume id %v deletion: %v", volumeID, err)
		return &csi.DeleteVolumeResponse{}, nil
	}

	var volCap *csi.VolumeCapability
	mountOptions := getMountOptions(req.GetSecrets())
	if mountOptions != "" {
		klog.V(2).Infof("DeleteVolume: found mountOptions(%s) for volume(%s)", mountOptions, volumeID)
		volCap = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					MountFlags: []string{mountOptions},
				},
			},
		}
	}

	if nfsVol.onDelete == "" {
		nfsVol.onDelete = cs.Driver.defaultOnDeletePolicy
	}

	if acquired := cs.Driver.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, volumeOperationAlreadyExistsFmt, volumeID)
	}
	defer cs.Driver.volumeLocks.Release(volumeID)

	if !strings.EqualFold(nfsVol.onDelete, retain) {
		// check whether volumeID is in the cache
		cache, err := cs.Driver.volDeletionCache.Get(volumeID, azcache.CacheReadTypeDefault)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "%v", err)
		}
		if cache != nil {
			klog.V(2).Infof("DeleteVolume: volume %s is already deleted", volumeID)
			return &csi.DeleteVolumeResponse{}, nil
		}
		// mount nfs base share so we can delete the subdirectory
		if err = cs.internalMount(ctx, nfsVol, nil, volCap); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to mount nfs server: %v", err)
		}
		defer func() {
			if err = cs.internalUnmount(ctx, nfsVol); err != nil {
				klog.Warningf("failed to unmount nfs server: %v", err)
			}
		}()

		internalVolumePath := getInternalVolumePath(cs.Driver.workingMountDir, nfsVol)

		if strings.EqualFold(nfsVol.onDelete, archive) {
			archivedInternalVolumePath := filepath.Join(getInternalMountPath(cs.Driver.workingMountDir, nfsVol), "archived-"+nfsVol.subDir)
			if strings.Contains(nfsVol.subDir, "/") {
				parentDir := filepath.Dir(archivedInternalVolumePath)
				klog.V(2).Infof("DeleteVolume: subdirectory(%s) contains '/', make sure the parent directory(%s) exists", nfsVol.subDir, parentDir)
				if err = os.MkdirAll(parentDir, 0777); err != nil {
					return nil, status.Errorf(codes.Internal, "create parent directory(%s) of %s failed with %v", parentDir, archivedInternalVolumePath, err)
				}
			}

			// archive subdirectory under base-dir, remove stale archived copy if exists.
			klog.V(2).Infof("archiving subdirectory %s --> %s", internalVolumePath, archivedInternalVolumePath)
			if cs.Driver.removeArchivedVolumePath {
				klog.V(2).Infof("removing archived subdirectory at %v", archivedInternalVolumePath)
				if err = os.RemoveAll(archivedInternalVolumePath); err != nil {
					return nil, status.Errorf(codes.Internal, "failed to delete archived subdirectory %s: %v", archivedInternalVolumePath, err)
				}
				klog.V(2).Infof("removed archived subdirectory at %v", archivedInternalVolumePath)
			}
			if err = os.Rename(internalVolumePath, archivedInternalVolumePath); err != nil {
				return nil, status.Errorf(codes.Internal, "archive subdirectory(%s, %s) failed with %v", internalVolumePath, archivedInternalVolumePath, err)
			}
			// make sure internalVolumePath does not exist with 1 minute timeout
			if err = waitForPathNotExistWithTimeout(internalVolumePath, time.Minute); err != nil {
				return nil, status.Errorf(codes.Internal, "DeleteVolume: internalVolumePath(%s) still exists after 1 minute", internalVolumePath)
			}
			klog.V(2).Infof("archived subdirectory %s --> %s", internalVolumePath, archivedInternalVolumePath)
		} else {
			rootDir := getRootDir(nfsVol.subDir)
			if rootDir != "" {
				rootDir = filepath.Join(getInternalMountPath(cs.Driver.workingMountDir, nfsVol), rootDir)
			} else {
				rootDir = internalVolumePath
			}
			// delete subdirectory under base-dir
			klog.V(2).Infof("removing subdirectory at %v on internalVolumePath %s", rootDir, internalVolumePath)
			if err = os.RemoveAll(rootDir); err != nil {
				return nil, status.Errorf(codes.Internal, "delete subdirectory(%s) failed with %v", internalVolumePath, err)
			}
		}
	} else {
		klog.V(2).Infof("DeleteVolume: volume(%s) is set to retain, not deleting/archiving subdirectory", volumeID)
	}

	cs.Driver.volDeletionCache.Set(volumeID, "")
	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *ControllerServer) ControllerPublishVolume(_ context.Context, _ *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerUnpublishVolume(_ context.Context, _ *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerGetVolume(_ context.Context, _ *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ValidateVolumeCapabilities(_ context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if err := isValidVolumeCapabilities(req.GetVolumeCapabilities()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
		Message: "",
	}, nil
}

func (cs *ControllerServer) ListVolumes(_ context.Context, _ *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) GetCapacity(_ context.Context, _ *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ControllerModifyVolume(_ context.Context, _ *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities implements the default GRPC callout.
// Default supports all capabilities
func (cs *ControllerServer) ControllerGetCapabilities(_ context.Context, _ *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.Driver.cscap,
	}, nil
}

func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateSnapshot name must be provided")
	}
	if len(req.GetSourceVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateSnapshot source volume ID must be provided")
	}

	srcVol, err := getNfsVolFromID(req.GetSourceVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "failed to create source volume: %v", err)
	}
	snapshot, err := newNFSSnapshot(req.GetName(), req.GetParameters(), srcVol)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "failed to create nfsSnapshot: %v", err)
	}
	snapVol := volumeFromSnapshot(snapshot)
	if err = cs.internalMount(ctx, snapVol, nil, nil); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mount snapshot nfs server: %v", err)
	}
	defer func() {
		if err = cs.internalUnmount(ctx, snapVol); err != nil {
			klog.Warningf("failed to unmount snapshot nfs server: %v", err)
		}
	}()
	snapInternalVolPath := getInternalVolumePath(cs.Driver.workingMountDir, snapVol)
	if err = os.MkdirAll(snapInternalVolPath, 0777); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to make subdirectory: %v", err)
	}
	if err := validateSnapshot(snapInternalVolPath, snapshot); err != nil {
		return nil, err
	}

	if err = cs.internalMount(ctx, srcVol, nil, nil); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mount src nfs server: %v", err)
	}
	defer func() {
		if err = cs.internalUnmount(ctx, srcVol); err != nil {
			klog.Warningf("failed to unmount src nfs server: %v", err)
		}
	}()

	srcPath := getInternalVolumePath(cs.Driver.workingMountDir, srcVol)
	dstPath := filepath.Join(snapInternalVolPath, snapshot.archiveName())

	klog.V(2).Infof("tar %v -> %v", srcPath, dstPath)
	if cs.Driver.useTarCommandInSnapshot {
		if out, err := exec.Command("tar", "-C", srcPath, "-czvf", dstPath, ".").CombinedOutput(); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create archive for snapshot: %v: %v", err, string(out))
		}
	} else {
		if err := TarPack(srcPath, dstPath, true); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create archive for snapshot: %v", err)
		}
	}
	klog.V(2).Infof("tar %s -> %s complete", srcPath, dstPath)

	var snapshotSize int64
	fi, err := os.Stat(dstPath)
	if err != nil {
		klog.Warningf("failed to determine snapshot size: %v", err)
	} else {
		snapshotSize = fi.Size()
	}
	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SnapshotId:     snapshot.id,
			SourceVolumeId: srcVol.id,
			SizeBytes:      snapshotSize,
			CreationTime:   timestamppb.Now(),
			ReadyToUse:     true,
		},
	}, nil
}

func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if len(req.GetSnapshotId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Snapshot ID is required for deletion")
	}
	snap, err := getNfsSnapFromID(req.GetSnapshotId())
	if err != nil {
		// An invalid ID should be treated as doesn't exist
		klog.Warningf("failed to get nfs snapshot for id %v deletion: %v", req.GetSnapshotId(), err)
		return &csi.DeleteSnapshotResponse{}, nil
	}

	var volCap *csi.VolumeCapability
	mountOptions := getMountOptions(req.GetSecrets())
	if mountOptions != "" {
		klog.V(2).Infof("DeleteSnapshot: found mountOptions(%s) for snapshot(%s)", mountOptions, req.GetSnapshotId())
		volCap = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					MountFlags: []string{mountOptions},
				},
			},
		}
	}
	vol := volumeFromSnapshot(snap)
	if err = cs.internalMount(ctx, vol, nil, volCap); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mount nfs server for snapshot deletion: %v", err)
	}
	defer func() {
		if err = cs.internalUnmount(ctx, vol); err != nil {
			klog.Warningf("failed to unmount nfs server after snapshot deletion: %v", err)
		}
	}()

	// delete snapshot archive
	internalVolumePath := getInternalVolumePath(cs.Driver.workingMountDir, vol)
	klog.V(2).Infof("Removing snapshot archive at %v", internalVolumePath)
	if err = os.RemoveAll(internalVolumePath); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete subdirectory: %v", err)
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (cs *ControllerServer) ListSnapshots(_ context.Context, _ *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerExpandVolume(_ context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if req.GetCapacityRange() == nil {
		return nil, status.Error(codes.InvalidArgument, "Capacity Range missing in request")
	}

	volSizeBytes := int64(req.GetCapacityRange().GetRequiredBytes())
	klog.V(2).Infof("ControllerExpandVolume(%s) successfully, currentQuota: %d bytes", req.VolumeId, volSizeBytes)

	return &csi.ControllerExpandVolumeResponse{CapacityBytes: req.GetCapacityRange().GetRequiredBytes()}, nil
}

// Mount nfs server at base-dir
func (cs *ControllerServer) internalMount(ctx context.Context, vol *nfsVolume, volumeContext map[string]string, volCap *csi.VolumeCapability) error {
	if volCap == nil {
		volCap = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
		}
	}

	sharePath := filepath.Join(string(filepath.Separator) + vol.baseDir)
	targetPath := getInternalMountPath(cs.Driver.workingMountDir, vol)

	volContext := map[string]string{
		paramServer: vol.server,
		paramShare:  sharePath,
	}
	for k, v := range volumeContext {
		// don't set subDir field since only nfs-server:/share should be mounted in CreateVolume/DeleteVolume
		if strings.ToLower(k) != paramSubDir {
			volContext[k] = v
		}
	}

	klog.V(2).Infof("internally mounting %s:%s at %s", vol.server, sharePath, targetPath)
	_, err := cs.Driver.ns.NodePublishVolume(ctx, &csi.NodePublishVolumeRequest{
		TargetPath:       targetPath,
		VolumeContext:    volContext,
		VolumeCapability: volCap,
		VolumeId:         vol.id,
	})
	return err
}

// Unmount nfs server at base-dir
func (cs *ControllerServer) internalUnmount(ctx context.Context, vol *nfsVolume) error {
	targetPath := getInternalMountPath(cs.Driver.workingMountDir, vol)

	// Unmount nfs server at base-dir
	klog.V(4).Infof("internally unmounting %v", targetPath)
	_, err := cs.Driver.ns.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{
		VolumeId:   vol.id,
		TargetPath: targetPath,
	})
	return err
}

func (cs *ControllerServer) copyFromSnapshot(ctx context.Context, req *csi.CreateVolumeRequest, dstVol *nfsVolume) error {
	snap, err := getNfsSnapFromID(req.VolumeContentSource.GetSnapshot().GetSnapshotId())
	if err != nil {
		return status.Error(codes.NotFound, err.Error())
	}
	snapVol := volumeFromSnapshot(snap)

	var volCap *csi.VolumeCapability
	if len(req.GetVolumeCapabilities()) > 0 {
		volCap = req.GetVolumeCapabilities()[0]
	}

	if err = cs.internalMount(ctx, snapVol, nil, volCap); err != nil {
		return status.Errorf(codes.Internal, "failed to mount src nfs server for snapshot volume copy: %v", err)
	}
	defer func() {
		if err = cs.internalUnmount(ctx, snapVol); err != nil {
			klog.Warningf("failed to unmount src nfs server after snapshot volume copy: %v", err)
		}
	}()
	if err = cs.internalMount(ctx, dstVol, nil, volCap); err != nil {
		return status.Errorf(codes.Internal, "failed to mount dst nfs server for snapshot volume copy: %v", err)
	}
	defer func() {
		if err = cs.internalUnmount(ctx, dstVol); err != nil {
			klog.Warningf("failed to unmount dst nfs server after snapshot volume copy: %v", err)
		}
	}()

	// untar snapshot archive to dst path
	snapPath := filepath.Join(getInternalVolumePath(cs.Driver.workingMountDir, snapVol), snap.archiveName())
	dstPath := getInternalVolumePath(cs.Driver.workingMountDir, dstVol)
	klog.V(2).Infof("copy volume from snapshot %v -> %v", snapPath, dstPath)

	if cs.Driver.useTarCommandInSnapshot {
		if out, err := exec.Command("tar", "-xzvf", snapPath, "-C", dstPath).CombinedOutput(); err != nil {
			return status.Errorf(codes.Internal, "failed to copy volume for snapshot: %v: %v", err, string(out))
		}
	} else {
		if err := TarUnpack(snapPath, dstPath, true); err != nil {
			return status.Errorf(codes.Internal, "failed to copy volume for snapshot: %v", err)
		}
	}
	klog.V(2).Infof("volume copied from snapshot %v -> %v", snapPath, dstPath)
	return nil
}

func (cs *ControllerServer) copyFromVolume(ctx context.Context, req *csi.CreateVolumeRequest, dstVol *nfsVolume) error {
	srcVol, err := getNfsVolFromID(req.GetVolumeContentSource().GetVolume().GetVolumeId())
	if err != nil {
		return status.Error(codes.NotFound, err.Error())
	}
	// Note that the source path must include trailing '/.', can't use 'filepath.Join()' as it performs path cleaning
	srcPath := fmt.Sprintf("%v/.", getInternalVolumePath(cs.Driver.workingMountDir, srcVol))
	dstPath := getInternalVolumePath(cs.Driver.workingMountDir, dstVol)
	klog.V(2).Infof("copy volume from volume %v -> %v", srcPath, dstPath)

	var volCap *csi.VolumeCapability
	if len(req.GetVolumeCapabilities()) > 0 {
		volCap = req.GetVolumeCapabilities()[0]
	}
	if err = cs.internalMount(ctx, srcVol, nil, volCap); err != nil {
		return status.Errorf(codes.Internal, "failed to mount src nfs server: %v", err)
	}
	defer func() {
		if err = cs.internalUnmount(ctx, srcVol); err != nil {
			klog.Warningf("failed to unmount nfs server: %v", err)
		}
	}()
	if err = cs.internalMount(ctx, dstVol, nil, volCap); err != nil {
		return status.Errorf(codes.Internal, "failed to mount dst nfs server: %v", err)
	}
	defer func() {
		if err = cs.internalUnmount(ctx, dstVol); err != nil {
			klog.Warningf("failed to unmount dst nfs server: %v", err)
		}
	}()

	// recursive 'cp' with '-a' to handle symlinks
	out, err := exec.Command("cp", "-a", srcPath, dstPath).CombinedOutput()
	if err != nil {
		return status.Errorf(codes.Internal, "failed to copy volume %v: %v", err, string(out))
	}
	klog.V(2).Infof("copied %s -> %s", srcPath, dstPath)
	return nil
}

func (cs *ControllerServer) copyVolume(ctx context.Context, req *csi.CreateVolumeRequest, vol *nfsVolume) error {
	vs := req.VolumeContentSource
	switch vs.Type.(type) {
	case *csi.VolumeContentSource_Snapshot:
		return cs.copyFromSnapshot(ctx, req, vol)
	case *csi.VolumeContentSource_Volume:
		return cs.copyFromVolume(ctx, req, vol)
	default:
		return status.Errorf(codes.InvalidArgument, "%v not a proper volume source", vs)
	}
}

// newNFSSnapshot Convert VolumeSnapshot parameters to a nfsSnapshot
func newNFSSnapshot(name string, params map[string]string, vol *nfsVolume) (*nfsSnapshot, error) {
	server := vol.server
	baseDir := vol.baseDir
	for k, v := range params {
		switch strings.ToLower(k) {
		case paramServer:
			server = v
		case paramShare:
			baseDir = v
		default:
			return nil, status.Errorf(codes.InvalidArgument, "invalid parameter %q in snapshot storage class", k)
		}
	}

	if server == "" {
		return nil, fmt.Errorf("%v is a required parameter", paramServer)
	}
	snapshot := &nfsSnapshot{
		server:  server,
		baseDir: baseDir,
		uuid:    name,
	}
	if vol.subDir != "" {
		snapshot.src = vol.subDir
	}
	if vol.uuid != "" {
		snapshot.src = vol.uuid
	}
	if snapshot.src == "" {
		return nil, fmt.Errorf("missing required source volume name")
	}
	snapshot.id = getSnapshotIDFromNfsSnapshot(snapshot)
	return snapshot, nil
}

// newNFSVolume Convert VolumeCreate parameters to an nfsVolume
func newNFSVolume(name string, size int64, params map[string]string, defaultOnDeletePolicy string) (*nfsVolume, error) {
	var server, baseDir, subDir, onDelete string
	subDirReplaceMap := map[string]string{}

	// validate parameters (case-insensitive)
	for k, v := range params {
		switch strings.ToLower(k) {
		case paramServer:
			server = v
		case paramShare:
			baseDir = v
		case paramSubDir:
			subDir = v
		case paramOnDelete:
			onDelete = v
		case pvcNamespaceKey:
			subDirReplaceMap[pvcNamespaceMetadata] = v
		case pvcNameKey:
			subDirReplaceMap[pvcNameMetadata] = v
		case pvNameKey:
			subDirReplaceMap[pvNameMetadata] = v
		}
	}

	if server == "" {
		return nil, fmt.Errorf("%v is a required parameter", paramServer)
	}

	vol := &nfsVolume{
		server:  server,
		baseDir: baseDir,
		size:    size,
	}
	if subDir == "" {
		// use pv name by default if not specified
		vol.subDir = name
	} else {
		// replace pv/pvc name namespace metadata in subDir
		vol.subDir = replaceWithMap(subDir, subDirReplaceMap)
		// make volume id unique if subDir is provided
		vol.uuid = name
	}

	if err := validateOnDeleteValue(onDelete); err != nil {
		return nil, err
	}

	vol.onDelete = defaultOnDeletePolicy
	if onDelete != "" {
		vol.onDelete = onDelete
	}

	vol.id = getVolumeIDFromNfsVol(vol)
	return vol, nil
}

// getInternalMountPath: get working directory for CreateVolume and DeleteVolume
func getInternalMountPath(workingMountDir string, vol *nfsVolume) string {
	if vol == nil {
		return ""
	}
	mountDir := vol.uuid
	if vol.uuid == "" {
		mountDir = vol.subDir
	}
	return filepath.Join(workingMountDir, mountDir)
}

// Get internal path where the volume is created
// The reason why the internal path is "workingDir/subDir/subDir" is because:
//   - the semantic is actually "workingDir/volId/subDir" and volId == subDir.
//   - we need a mount directory per volId because you can have multiple
//     CreateVolume calls in parallel and they may use the same underlying share.
//     Instead of refcounting how many CreateVolume calls are using the same
//     share, it's simpler to just do a mount per request.
func getInternalVolumePath(workingMountDir string, vol *nfsVolume) string {
	return filepath.Join(getInternalMountPath(workingMountDir, vol), vol.subDir)
}

// Given a nfsVolume, return a CSI volume id
func getVolumeIDFromNfsVol(vol *nfsVolume) string {
	idElements := make([]string, totalIDElements)
	idElements[idServer] = strings.Trim(vol.server, "/")
	idElements[idBaseDir] = strings.Trim(vol.baseDir, "/")
	idElements[idSubDir] = strings.Trim(vol.subDir, "/")
	idElements[idUUID] = vol.uuid
	if strings.EqualFold(vol.onDelete, retain) || strings.EqualFold(vol.onDelete, archive) {
		idElements[idOnDelete] = vol.onDelete
	}

	return strings.Join(idElements, separator)
}

// Given a nfsSnapshot, return a CSI snapshot id.
func getSnapshotIDFromNfsSnapshot(snap *nfsSnapshot) string {
	idElements := make([]string, totalIDSnapElements)
	idElements[idSnapServer] = strings.Trim(snap.server, "/")
	idElements[idSnapBaseDir] = strings.Trim(snap.baseDir, "/")
	idElements[idSnapUUID] = snap.uuid
	idElements[idSnapArchivePath] = snap.uuid
	idElements[idSnapArchiveName] = snap.src
	return strings.Join(idElements, separator)
}

// Given a CSI volume id, return a nfsVolume
// sample volume Id:
//
//	  new volumeID:
//		    nfs-server.default.svc.cluster.local#share#pvc-4bcbf944-b6f7-4bd0-b50f-3c3dd00efc64
//		    nfs-server.default.svc.cluster.local#share#subdir#pvc-4bcbf944-b6f7-4bd0-b50f-3c3dd00efc64#retain
//	  old volumeID: nfs-server.default.svc.cluster.local/share/pvc-4bcbf944-b6f7-4bd0-b50f-3c3dd00efc64
func getNfsVolFromID(id string) (*nfsVolume, error) {
	var server, baseDir, subDir, uuid, onDelete string
	segments := strings.Split(id, separator)
	if len(segments) < 3 {
		klog.V(2).Infof("could not split %s into server, baseDir and subDir with separator(%s)", id, separator)
		// try with separator "/"
		volRegex := regexp.MustCompile("^([^/]+)/(.*)/([^/]+)$")
		tokens := volRegex.FindStringSubmatch(id)
		if tokens == nil || len(tokens) < 4 {
			return nil, fmt.Errorf("could not split %s into server, baseDir and subDir with separator(%s)", id, "/")
		}
		server = tokens[1]
		baseDir = tokens[2]
		subDir = tokens[3]
	} else {
		server = segments[0]
		baseDir = segments[1]
		subDir = segments[2]
		if len(segments) >= 4 {
			uuid = segments[3]
		}
		if len(segments) >= 5 {
			onDelete = segments[4]
		}
	}

	return &nfsVolume{
		id:       id,
		server:   server,
		baseDir:  baseDir,
		subDir:   subDir,
		uuid:     uuid,
		onDelete: onDelete,
	}, nil
}

// Given a CSI snapshot ID, return a nfsSnapshot
// sample snapshot ID:
//
//	nfs-server.default.svc.cluster.local#share#snapshot-016f784f-56f4-44d1-9041-5f59e82dbce1#snapshot-016f784f-56f4-44d1-9041-5f59e82dbce1#pvc-4bcbf944-b6f7-4bd0-b50f-3c3dd00efc64
func getNfsSnapFromID(id string) (*nfsSnapshot, error) {
	segments := strings.Split(id, separator)
	if len(segments) == totalIDSnapElements {
		return &nfsSnapshot{
			id:      id,
			server:  segments[idSnapServer],
			baseDir: segments[idSnapBaseDir],
			src:     segments[idSnapArchiveName],
			uuid:    segments[idSnapUUID],
		}, nil
	}

	return &nfsSnapshot{}, fmt.Errorf("failed to create nfsSnapshot from snapshot ID")
}

// isValidVolumeCapabilities validates the given VolumeCapability array is valid
func isValidVolumeCapabilities(volCaps []*csi.VolumeCapability) error {
	if len(volCaps) == 0 {
		return fmt.Errorf("volume capabilities missing in request")
	}
	for _, c := range volCaps {
		if c.GetBlock() != nil {
			return fmt.Errorf("block volume capability not supported")
		}
	}
	return nil
}

// Validate snapshot after internal mount
func validateSnapshot(snapInternalVolPath string, snap *nfsSnapshot) error {
	return filepath.WalkDir(snapInternalVolPath, func(path string, d fs.DirEntry, err error) error {
		if path == snapInternalVolPath {
			// skip root
			return nil
		}
		if err != nil {
			return err
		}
		if d.Name() != snap.archiveName() {
			// there should be just one archive in the snapshot path and archive name should match
			return status.Errorf(codes.AlreadyExists, "snapshot with the same name but different source volume ID already exists: found %q, desired %q", d.Name(), snap.archiveName())
		}
		return nil
	})
}

// Volume for snapshot internal mount/unmount
func volumeFromSnapshot(snap *nfsSnapshot) *nfsVolume {
	return &nfsVolume{
		id:      snap.id,
		server:  snap.server,
		baseDir: snap.baseDir,
		subDir:  snap.uuid,
		uuid:    snap.uuid,
	}
}
