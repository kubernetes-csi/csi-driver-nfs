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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"k8s.io/klog/v2"
)

// ControllerServer controller server setting
type ControllerServer struct {
	Driver *Driver
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
	totalIDElements // Always last
)

const separator = "#"

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
		case pvcNamespaceKey:
		case pvcNameKey:
		case pvNameKey:
			// no op
		case mountPermissionsField:
			if v != "" {
				var err error
				if mountPermissions, err = strconv.ParseUint(v, 8, 32); err != nil {
					return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("invalid mountPermissions %s in storage class", v))
				}
			}
		default:
			return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("invalid parameter %q in storage class", k))
		}
	}

	nfsVol, err := newNFSVolume(name, reqCapacity, parameters)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var volCap *csi.VolumeCapability
	if len(req.GetVolumeCapabilities()) > 0 {
		volCap = req.GetVolumeCapabilities()[0]
	}
	// Mount nfs base share so we can create a subdirectory
	if err = cs.internalMount(ctx, nfsVol, parameters, volCap); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mount nfs server: %v", err.Error())
	}
	defer func() {
		if err = cs.internalUnmount(ctx, nfsVol); err != nil {
			klog.Warningf("failed to unmount nfs server: %v", err.Error())
		}
	}()

	// Create subdirectory under base-dir
	internalVolumePath := getInternalVolumePath(cs.Driver.workingMountDir, nfsVol)
	if err = os.Mkdir(internalVolumePath, 0777); err != nil && !os.IsExist(err) {
		return nil, status.Errorf(codes.Internal, "failed to make subdirectory: %v", err.Error())
	}

	if mountPermissions > 0 {
		// Reset directory permissions because of umask problems
		if err = os.Chmod(internalVolumePath, os.FileMode(mountPermissions)); err != nil {
			klog.Warningf("failed to chmod subdirectory: %v", err.Error())
		}
	}

	setKeyValueInMap(parameters, paramSubDir, nfsVol.subDir)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      nfsVol.id,
			CapacityBytes: 0, // by setting it to zero, Provisioner will use PVC requested size as PV size
			VolumeContext: parameters,
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

	// mount nfs base share so we can delete the subdirectory
	if err = cs.internalMount(ctx, nfsVol, nil, volCap); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mount nfs server: %v", err.Error())
	}
	defer func() {
		if err = cs.internalUnmount(ctx, nfsVol); err != nil {
			klog.Warningf("failed to unmount nfs server: %v", err.Error())
		}
	}()

	// delete subdirectory under base-dir
	internalVolumePath := getInternalVolumePath(cs.Driver.workingMountDir, nfsVol)

	klog.V(2).Infof("Removing subdirectory at %v", internalVolumePath)
	if err = os.RemoveAll(internalVolumePath); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete subdirectory: %v", err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
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

func (cs *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities implements the default GRPC callout.
// Default supports all capabilities
func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.Driver.cscap,
	}, nil
}

func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
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

// newNFSVolume Convert VolumeCreate parameters to an nfsVolume
func newNFSVolume(name string, size int64, params map[string]string) (*nfsVolume, error) {
	var server, baseDir, subDir string
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
	return strings.Join(idElements, separator)
}

// Given a CSI volume id, return a nfsVolume
// sample volume Id:
//
//	  new volumeID:
//		    nfs-server.default.svc.cluster.local#share#pvc-4bcbf944-b6f7-4bd0-b50f-3c3dd00efc64
//		    nfs-server.default.svc.cluster.local#share#subdir#pvc-4bcbf944-b6f7-4bd0-b50f-3c3dd00efc64
//	  old volumeID: nfs-server.default.svc.cluster.local/share/pvc-4bcbf944-b6f7-4bd0-b50f-3c3dd00efc64
func getNfsVolFromID(id string) (*nfsVolume, error) {
	var server, baseDir, subDir, uuid string
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
	}

	return &nfsVolume{
		id:      id,
		server:  server,
		baseDir: baseDir,
		subDir:  subDir,
		uuid:    uuid,
	}, nil
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
