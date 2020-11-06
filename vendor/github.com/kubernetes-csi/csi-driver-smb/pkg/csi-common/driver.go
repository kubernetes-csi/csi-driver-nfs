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

package csicommon

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/klog/v2"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CSIDriver struct {
	Name    string
	NodeID  string
	Version string
	Cap     []*csi.ControllerServiceCapability
	VC      []*csi.VolumeCapability_AccessMode
	NSCap   []*csi.NodeServiceCapability
}

// Creates a NewCSIDriver object. Assumes vendor version is equal to driver version &
// does not support optional driver plugin info manifest field. Refer to CSI spec for more details.
func NewCSIDriver(name string, v string, nodeID string) *CSIDriver {
	if name == "" {
		klog.Errorf("Driver name missing")
		return nil
	}

	if nodeID == "" {
		klog.Errorf("NodeID missing")
		return nil
	}
	// TODO version format and validation
	if len(v) == 0 {
		klog.Errorf("Version argument missing, now skip it")
		//return nil
	}

	driver := CSIDriver{
		Name:    name,
		Version: v,
		NodeID:  nodeID,
	}

	return &driver
}

func (d *CSIDriver) ValidateControllerServiceRequest(c csi.ControllerServiceCapability_RPC_Type) error {
	if c == csi.ControllerServiceCapability_RPC_UNKNOWN {
		return nil
	}

	for _, cap := range d.Cap {
		if c == cap.GetRpc().GetType() {
			return nil
		}
	}
	return status.Error(codes.InvalidArgument, c.String())
}

func (d *CSIDriver) ValidateNodeServiceRequest(c csi.NodeServiceCapability_RPC_Type) error {
	if c == csi.NodeServiceCapability_RPC_UNKNOWN {
		return nil
	}

	for _, cap := range d.NSCap {
		if c == cap.GetRpc().GetType() {
			return nil
		}
	}
	return status.Error(codes.InvalidArgument, c.String())
}

func (d *CSIDriver) AddControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) {
	var csc []*csi.ControllerServiceCapability

	for _, c := range cl {
		klog.Infof("Enabling controller service capability: %v", c.String())
		csc = append(csc, NewControllerServiceCapability(c))
	}

	d.Cap = csc
}

func (d *CSIDriver) AddNodeServiceCapabilities(nl []csi.NodeServiceCapability_RPC_Type) {
	var nsc []*csi.NodeServiceCapability
	for _, n := range nl {
		klog.V(2).Infof("Enabling node service capability: %v", n.String())
		nsc = append(nsc, NewNodeServiceCapability(n))
	}
	d.NSCap = nsc
}

func (d *CSIDriver) AddVolumeCapabilityAccessModes(vc []csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability_AccessMode {
	var vca []*csi.VolumeCapability_AccessMode
	for _, c := range vc {
		klog.Infof("Enabling volume access mode: %v", c.String())
		vca = append(vca, NewVolumeCapabilityAccessMode(c))
	}
	d.VC = vca
	return vca
}

func (d *CSIDriver) GetVolumeCapabilityAccessModes() []*csi.VolumeCapability_AccessMode {
	return d.VC
}
