package nfs

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
)

// ControllerPlugin is an interface for controller that implements minimum set of controller methods
type ControllerPlugin interface {
	// TODO: Consider ControllerGetCapabilities needs to be implemented depend on plugins
	//ControllerGetCapabilities(ctx context.Context, cs *ControllerServer, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error)
	ValidateVolumeCapabilities(ctx context.Context, cs *ControllerServer, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error)
}

// CreateDeleteVolumeControllerPlugin is an interface for controller that implements csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME
type CreateDeleteVolumeControllerPlugin interface {
	ControllerPlugin
	CreateVolume(ctx context.Context, cs *ControllerServer, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error)
	DeleteVolume(ctx context.Context, cs *ControllerServer, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error)
}

// PublishUnpublishVolumeControllerPlugin is an interface for controller that implements csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME
type PublishUnpublishVolumeControllerPlugin interface {
	ControllerPlugin
	ControllerPublishVolume(ctx context.Context, cs *ControllerServer, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error)
	ControllerUnpublishVolume(ctx context.Context, cs *ControllerServer, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error)
}

// ListVolumesControllerPlugin is an interface for controller that implements csi.ControllerServiceCapability_RPC_LIST_VOLUMES
type ListVolumesControllerPlugin interface {
	ControllerPlugin
	ListVolumes(ctx context.Context, cs *ControllerServer, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error)
}

// GetCapacityControllerPlugin is an interface for controller that implements csi.ControllerServiceCapability_RPC_GET_CAPACITY
type GetCapacityControllerPlugin interface {
	ControllerPlugin
	GetCapacity(ctx context.Context, cs *ControllerServer, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error)
}

// SnapshotControllerPlugin is an interface for controller that implements csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT
type SnapshotControllerPlugin interface {
	ControllerPlugin
	CreateSnapshot(ctx context.Context, cs *ControllerServer, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error)
	DeleteSnapshot(ctx context.Context, cs *ControllerServer, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error)
}

// ListSnapshotControllerPlugin is an interface for controller that implements csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS
type ListSnapshotControllerPlugin interface {
	ControllerPlugin
	ListSnapshots(ctx context.Context, cs *ControllerServer, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error)
}

// ExpandVolumeControllerPlugin is an interface for controller that implements csi.ControllerServiceCapability_RPC_EXPAND_VOLUME
type ExpandVolumeControllerPlugin interface {
	ControllerPlugin
	ControllerExpandVolume(ctx context.Context, cs *ControllerServer, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error)
}
