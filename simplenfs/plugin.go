// +build simplenfs

package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/kubernetes-csi/csi-driver-nfs/pkg/nfs"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/pkg/util/mount"
)

const (
	mountPathBase = "/csi-nfs-volume"
)

func CreateVolume(cs *nfs.ControllerServer, ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	glog.Infof("plugin.CreateVolume called")
	var volSize int64
	if req.GetCapacityRange() != nil {
		volSize = req.GetCapacityRange().GetRequiredBytes()
	}
	volInfo := volumeInfo{req.GetParameters()["server"], req.GetParameters()["rootpath"], req.GetName()}
	volID, err := encodeVolID(volInfo)
	if err != nil {
		glog.Warningf("encodeVolID for volInfo %v failed: %v", volInfo, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Create /csi-nfs-volume/{UUID}/ directory and mount nfs rootpath to it
	mountPath := filepath.Join(mountPathBase, string(uuid.NewUUID()))
	if err := setupMountPath(mountPath, volInfo.server, volInfo.rootpath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	// Unmount nfs rootpath from /csi-nfs-volume/{UUID}/{volID} directory and delete the directory
	defer teardownMountPath(mountPath)

	// Create directory in nfs rootpath by creating directory /csi-nfs-volume/{UUID}/{volID}
	fullPath := filepath.Join(mountPath, volID)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		glog.V(4).Infof("creating path %s", fullPath)
		if err := os.MkdirAll(fullPath, 0777); err != nil {
			return nil, errors.New("unable to create directory to create volume: " + err.Error())
		}
		os.Chmod(fullPath, 0777)
	}

	// Add share:{rootPath}/{volID} to volumeContext
	volContext := req.GetParameters()
	volContext["share"] = filepath.Join(volInfo.rootpath, volID)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volID,
			CapacityBytes: volSize,
			VolumeContext: volContext,
		},
	}, nil
}

func DeleteVolume(cs *nfs.ControllerServer, ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	glog.Infof("plugin.DeleteVolume called")
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty volume ID in request")
	}
	glog.Infof("volumeID: %s", volumeID)

	volInfo, err := decodeVolID(volumeID)
	if err != nil {
		glog.Warningf("decodeVolID for volumeID %s failed: %v", volumeID, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Create /csi-nfs-volume/{UUID}/ directory and mount nfs rootpath to it
	mountPath := filepath.Join(mountPathBase, string(uuid.NewUUID()))
	if err := setupMountPath(mountPath, volInfo.server, volInfo.rootpath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	// Unmount nfs rootpath from /csi-nfs-volume/{UUID}/{volID} directory and delete the directory
	defer teardownMountPath(mountPath)

	// Delete directory in nfs rootpath by deleting directory /csi-nfs-volume/{UUID}/{volID}
	fullPath := filepath.Join(mountPath, volumeID)
	glog.V(4).Infof("creating path %s", fullPath)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		glog.Warningf("path %s does not exist, deletion skipped", fullPath)
		return &csi.DeleteVolumeResponse{}, nil
	}
	if err := os.RemoveAll(fullPath); err != nil {
		glog.Warningf("Failed to remove %s: %v", fullPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func ValidateVolumeCapabilities(cs *nfs.ControllerServer, ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Empty volume ID in request")
	}

	if len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Empty volume capabilities in request")
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.VolumeCapabilities,
		},
	}, nil
}

func setupMountPath(mountPath string, server string, rootpath string) error {
	// Create mountPath /csi-nfs-volume/{UUID}
	if err := os.MkdirAll(mountPath, 0750); err != nil {
		glog.Warningf("Failed to create mountPath %s: %v", mountPath, err)
		return err
	}

	// Mount nfs rootpath to mountPath /csi-nfs-volume/{UUID}
	source := fmt.Sprintf("%s:%s", server, rootpath)

	mounter := mount.New("")
	if err := mounter.Mount(source, mountPath, "nfs", []string{"nolock"}); err != nil {
		glog.Warningf("Failed to mount source %s to mountPath %s: %v", source, mountPath, err)
		return err
	}

	return nil
}

func teardownMountPath(mountPath string) error {
	// Unmount nfs rootpath from mountPath /csi-nfs-volume/{UUID} and delete the path
	if err := mount.CleanupMountPoint(mountPath, mount.New(""), false); err != nil {
		glog.Warningf("Failed to cleanup mountPath %s: %v", mountPath, err)
		return err
	}

	return nil
}

type volumeInfo struct {
	server   string
	rootpath string
	volID    string
}

func encodeVolID(vol volumeInfo) (string, error) {
	if len(vol.server) == 0 {
		return "", fmt.Errorf("Server information in VolumeInfo shouldn't be empty: %v", vol)
	}

	if len(vol.rootpath) == 0 {
		return "", fmt.Errorf("Rootpath information in VolumeInfo shouldn't be empty: %v", vol)
	}

	if len(vol.volID) == 0 {
		return "", fmt.Errorf("volID information in VolumeInfo shouldn't be empty: %v", vol)
	}

	encServer := strings.ReplaceAll(base64.RawStdEncoding.EncodeToString([]byte(vol.server)), "/", "-")
	encRootpath := strings.ReplaceAll(base64.RawStdEncoding.EncodeToString([]byte(vol.rootpath)), "/", "-")
	encVolID := strings.ReplaceAll(base64.RawStdEncoding.EncodeToString([]byte(vol.volID)), "/", "-")
	return strings.Join([]string{encServer, encRootpath, encVolID}, "_"), nil
}

func decodeVolID(volID string) (*volumeInfo, error) {
	var volInfo volumeInfo
	volIDs := strings.SplitN(volID, "_", 3)

	if len(volIDs) != 3 {
		return nil, fmt.Errorf("Failed to decode information from %s: not enough fields", volID)
	}

	serverByte, err := base64.RawStdEncoding.DecodeString(strings.ReplaceAll(volIDs[0], "-", "/"))
	if err != nil {
		return nil, fmt.Errorf("Failed to decode server information from %s: %v", volID, err)
	}
	volInfo.server = string(serverByte)

	rootpathByte, err := base64.RawStdEncoding.DecodeString(strings.ReplaceAll(volIDs[1], "-", "/"))
	if err != nil {
		return nil, fmt.Errorf("Failed to decode rootpath information from %s: %v", volID, err)
	}
	volInfo.rootpath = string(rootpathByte)

	volIDByte, err := base64.RawStdEncoding.DecodeString(strings.ReplaceAll(volIDs[2], "-", "/"))
	if err != nil {
		return nil, fmt.Errorf("Failed to decode volID information from %s: %v", volID, err)
	}
	volInfo.volID = string(volIDByte)

	return &volInfo, nil
}
