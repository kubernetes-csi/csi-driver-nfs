package nfs

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ControllerServer struct {
	*csicommon.DefaultControllerServer
}

func (cs ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func getControllerServer(csiDriver *csicommon.CSIDriver) ControllerServer {
	return ControllerServer{
		csicommon.NewDefaultControllerServer(csiDriver),
	}
}
