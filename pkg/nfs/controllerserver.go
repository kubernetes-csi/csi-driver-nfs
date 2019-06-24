package nfs

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ControllerServer struct {
	Driver *nfsDriver
}

func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(CreateDeleteVolumeControllerPlugin); ok {
		return plug.CreateVolume(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(CreateDeleteVolumeControllerPlugin); ok {
		return plug.DeleteVolume(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(PublishUnpublishVolumeControllerPlugin); ok {
		return plug.ControllerPublishVolume(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(PublishUnpublishVolumeControllerPlugin); ok {
		return plug.ControllerUnpublishVolume(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(ControllerPlugin); ok {
		return plug.ValidateVolumeCapabilities(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(ListVolumesControllerPlugin); ok {
		return plug.ListVolumes(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(GetCapacityControllerPlugin); ok {
		return plug.GetCapacity(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.Driver.cscap,
	}, nil
}

func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(SnapshotControllerPlugin); ok {
		return plug.CreateSnapshot(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(SnapshotControllerPlugin); ok {
		return plug.DeleteSnapshot(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(ListSnapshotControllerPlugin); ok {
		return plug.ListSnapshots(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if plug, ok := cs.Driver.csPlugin.(ExpandVolumeControllerPlugin); ok {
		return plug.ControllerExpandVolume(ctx, cs, req)
	}
	return nil, status.Error(codes.Unimplemented, "")
}
