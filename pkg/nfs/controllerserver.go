package nfs

import (
	"plugin"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ControllerServer struct {
	Driver *nfsDriver
}

func isSupported(pluginName string, symbolName string) bool {
	symbol, err := lookupSymbol(pluginName, symbolName)
	return err == nil && symbol != nil
}

func lookupSymbol(pluginName string, symbolName string) (interface{}, error) {
	if pluginName != "" {
		plug, err := plugin.Open(pluginName)
		if err != nil {
			glog.Infof("Failed to load plugin: %s error: %v", pluginName, err)
			return nil, err
		}
		symbol, err := plug.Lookup(symbolName)
		if err != nil {
			glog.Infof("Failed to lookup symbol: %s error: %v", symbolName, err)
			return nil, err
		}
		return symbol, nil
	}
	return nil, nil
}

func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	glog.Infof("CreateVolume called")
	symbol, err := lookupSymbol(cs.Driver.controllerPlugin, "CreateVolume")
	if err == nil && symbol != nil {
		createVolume, ok := symbol.(func(cs *ControllerServer, ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error))
		if ok {
			return createVolume(cs, ctx, req)
		}
	}

	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	glog.Infof("DeleteVolume called")
	symbol, err := lookupSymbol(cs.Driver.controllerPlugin, "DeleteVolume")
	if err == nil && symbol != nil {
		deleteVolume, ok := symbol.(func(cs *ControllerServer, ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error))
		if ok {
			return deleteVolume(cs, ctx, req)
		}
	}

	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	symbol, err := lookupSymbol(cs.Driver.controllerPlugin, "ValidateVolumeCapabilities")
	if err == nil && symbol != nil {
		validateVolumeCapabilities, ok := symbol.(func(cs *ControllerServer, ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error))
		if ok {
			return validateVolumeCapabilities(cs, ctx, req)
		}
	}

	return nil, status.Error(codes.Unimplemented, "")
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
	glog.V(5).Infof("Using default ControllerGetCapabilities")

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
