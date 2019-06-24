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
	"plugin"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
)

type nfsDriver struct {
	name    string
	nodeID  string
	version string

	endpoint string

	//ids *identityServer
	ns    *nodeServer
	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability

	csPlugin interface{}
}

const (
	driverName       = "csi-nfsplugin"
	pluginSymbolName = "NfsPlugin"
)

var (
	version = "1.0.0-rc2"
)

func loadControllerPlugin(pluginName string) (interface{}, []csi.ControllerServiceCapability_RPC_Type, error) {
	csc := []csi.ControllerServiceCapability_RPC_Type{}

	if pluginName == "" {
		csc = append(csc, csi.ControllerServiceCapability_RPC_UNKNOWN)
		return nil, csc, nil
	}

	plug, err := plugin.Open(pluginName)
	if err != nil {
		glog.Infof("Failed to load plugin: %s error: %v", pluginName, err)
		return nil, csc, err
	}

	csPlugin, err := plug.Lookup(pluginSymbolName)
	if err != nil {
		glog.Infof("Failed to lookup csPlugin: %s error: %v", pluginSymbolName, err)
		return nil, csc, err
	}

	// Check if csPlugin implements each capability and add it to implenentation
	if _, ok := csPlugin.(ControllerPlugin); !ok {
		glog.Infof("Plugin doesn't implement mandatory methods for controller")
		return nil, csc, fmt.Errorf("Plugin doesn't implement mandatory methods for controller")
	}

	if _, ok := csPlugin.(CreateDeleteVolumeControllerPlugin); ok {
		csc = append(csc, csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME)
	}

	if _, ok := csPlugin.(PublishUnpublishVolumeControllerPlugin); ok {
		csc = append(csc, csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME)
	}

	if _, ok := csPlugin.(ListVolumesControllerPlugin); ok {
		csc = append(csc, csi.ControllerServiceCapability_RPC_LIST_VOLUMES)
	}

	if _, ok := csPlugin.(GetCapacityControllerPlugin); ok {
		csc = append(csc, csi.ControllerServiceCapability_RPC_GET_CAPACITY)
	}

	if _, ok := csPlugin.(SnapshotControllerPlugin); ok {
		csc = append(csc, csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT)
	}

	if _, ok := csPlugin.(ListSnapshotControllerPlugin); ok {
		csc = append(csc, csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS)
	}

	if _, ok := csPlugin.(ExpandVolumeControllerPlugin); ok {
		csc = append(csc, csi.ControllerServiceCapability_RPC_EXPAND_VOLUME)
	}

	// TODO: Need to handle clone volume and publish read only capability?

	if len(csc) == 0 {
		csc = append(csc, csi.ControllerServiceCapability_RPC_UNKNOWN)
	}

	return csPlugin, csc, nil
}

func NewNFSdriver(nodeID, endpoint, controllerPlugin string) (*nfsDriver, error) {
	glog.Infof("Driver: %v version: %v", driverName, version)

	n := &nfsDriver{
		name:     driverName,
		version:  version,
		nodeID:   nodeID,
		endpoint: endpoint,
	}

	n.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})

	csPlugin, csc, err := loadControllerPlugin(controllerPlugin)
	if err != nil {
		return nil, fmt.Errorf("Failed to load plugin %s: %v", controllerPlugin, err)
	}
	n.csPlugin = csPlugin
	n.AddControllerServiceCapabilities(csc)

	return n, nil
}

func NewNodeServer(n *nfsDriver) *nodeServer {
	return &nodeServer{
		Driver: n,
	}
}

func (n *nfsDriver) Run() {
	s := NewNonBlockingGRPCServer()
	s.Start(n.endpoint,
		NewDefaultIdentityServer(n),
		NewControllerServer(n),
		NewNodeServer(n))
	s.Wait()
}

func (n *nfsDriver) AddVolumeCapabilityAccessModes(vc []csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability_AccessMode {
	var vca []*csi.VolumeCapability_AccessMode
	for _, c := range vc {
		glog.Infof("Enabling volume access mode: %v", c.String())
		vca = append(vca, &csi.VolumeCapability_AccessMode{Mode: c})
	}
	n.cap = vca
	return vca
}

func (n *nfsDriver) AddControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) {
	var csc []*csi.ControllerServiceCapability

	for _, c := range cl {
		glog.Infof("Enabling controller service capability: %v", c.String())
		csc = append(csc, NewControllerServiceCapability(c))
	}

	n.cscap = csc

	return
}
