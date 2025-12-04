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
	"runtime"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/klog/v2"
	mount "k8s.io/mount-utils"

	azcache "sigs.k8s.io/cloud-provider-azure/pkg/cache"
)

// DriverOptions defines driver parameters specified in driver deployment
type DriverOptions struct {
	NodeID                       string
	DriverName                   string
	Endpoint                     string
	MountPermissions             uint64
	WorkingMountDir              string
	DefaultOnDeletePolicy        string
	VolStatsCacheExpireInMinutes int
	RemoveArchivedVolumePath     bool
	UseTarCommandInSnapshot      bool
	EnableSnapshotCompression    bool
}

type Driver struct {
	name                      string
	nodeID                    string
	version                   string
	endpoint                  string
	mountPermissions          uint64
	workingMountDir           string
	defaultOnDeletePolicy     string
	removeArchivedVolumePath  bool
	useTarCommandInSnapshot   bool
	enableSnapshotCompression bool

	//ids *identityServer
	ns          *NodeServer
	cscap       []*csi.ControllerServiceCapability
	nscap       []*csi.NodeServiceCapability
	volumeLocks *VolumeLocks

	// a timed cache storing volume stats <volumeID, volumeStats>
	volStatsCache                azcache.Resource
	volStatsCacheExpireInMinutes int
	// a timed cache storing volume deletion records <volumeID, "">
	volDeletionCache azcache.Resource
}

const (
	DefaultDriverName = "nfs.csi.k8s.io"
	// Address of the NFS server
	paramServer = "server"
	// Base directory of the NFS server to create volumes under.
	// The base directory must be a direct child of the root directory.
	// The root directory is omitted from the string, for example:
	//     "base" instead of "/base"
	paramShare            = "share"
	paramSubDir           = "subdir"
	paramOnDelete         = "ondelete"
	mountOptionsField     = "mountoptions"
	mountPermissionsField = "mountpermissions"
	pvcNameKey            = "csi.storage.k8s.io/pvc/name"
	pvcNamespaceKey       = "csi.storage.k8s.io/pvc/namespace"
	pvNameKey             = "csi.storage.k8s.io/pv/name"
	pvcNameMetadata       = "${pvc.metadata.name}"
	pvcNamespaceMetadata  = "${pvc.metadata.namespace}"
	pvNameMetadata        = "${pv.metadata.name}"
)

func NewDriver(options *DriverOptions) *Driver {
	klog.V(2).Infof("Driver: %v version: %v", options.DriverName, driverVersion)

	n := &Driver{
		name:                         options.DriverName,
		version:                      driverVersion,
		nodeID:                       options.NodeID,
		endpoint:                     options.Endpoint,
		mountPermissions:             options.MountPermissions,
		workingMountDir:              options.WorkingMountDir,
		volStatsCacheExpireInMinutes: options.VolStatsCacheExpireInMinutes,
		removeArchivedVolumePath:     options.RemoveArchivedVolumePath,
		useTarCommandInSnapshot:      options.UseTarCommandInSnapshot,
		enableSnapshotCompression:    options.EnableSnapshotCompression,
		defaultOnDeletePolicy:        options.DefaultOnDeletePolicy,
	}

	n.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
	})

	n.AddNodeServiceCapabilities([]csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
		csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
		csi.NodeServiceCapability_RPC_UNKNOWN,
	})
	n.volumeLocks = NewVolumeLocks()

	if options.VolStatsCacheExpireInMinutes <= 0 {
		options.VolStatsCacheExpireInMinutes = 10 // default expire in 10 minutes
	}

	var err error
	getter := func(_ string) (interface{}, error) { return nil, nil }
	if n.volStatsCache, err = azcache.NewTimedCache(time.Duration(options.VolStatsCacheExpireInMinutes)*time.Minute, getter, false); err != nil {
		klog.Fatalf("%v", err)
	}
	if n.volDeletionCache, err = azcache.NewTimedCache(time.Minute, getter, false); err != nil {
		klog.Fatalf("%v", err)
	}
	return n
}

func NewNodeServer(n *Driver, mounter mount.Interface) *NodeServer {
	return &NodeServer{
		Driver:  n,
		mounter: mounter,
	}
}

func (n *Driver) Run(testMode bool) {
	versionMeta, err := GetVersionYAML(n.name)
	if err != nil {
		klog.Fatalf("%v", err)
	}
	klog.V(2).Infof("\nDRIVER INFORMATION:\n-------------------\n%s\n\nStreaming logs below:", versionMeta)

	mounter := mount.New("")
	if runtime.GOOS == "linux" {
		// MounterForceUnmounter is only implemented on Linux now
		mounter = mounter.(mount.MounterForceUnmounter)
	}
	n.ns = NewNodeServer(n, mounter)
	s := NewNonBlockingGRPCServer()
	s.Start(n.endpoint,
		NewDefaultIdentityServer(n),
		// NFS plugin has not implemented ControllerServer
		// using default controllerserver.
		NewControllerServer(n),
		n.ns,
		testMode)
	s.Wait()
}

func (n *Driver) AddControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) {
	var csc []*csi.ControllerServiceCapability
	for _, c := range cl {
		csc = append(csc, NewControllerServiceCapability(c))
	}
	n.cscap = csc
}

func (n *Driver) AddNodeServiceCapabilities(nl []csi.NodeServiceCapability_RPC_Type) {
	var nsc []*csi.NodeServiceCapability
	for _, n := range nl {
		nsc = append(nsc, NewNodeServiceCapability(n))
	}
	n.nscap = nsc
}

func IsCorruptedDir(dir string) bool {
	_, pathErr := mount.PathExists(dir)
	return pathErr != nil && mount.IsCorruptedMnt(pathErr)
}

// replaceWithMap replace key with value for str
func replaceWithMap(str string, m map[string]string) string {
	for k, v := range m {
		if k != "" {
			str = strings.ReplaceAll(str, k, v)
		}
	}
	return str
}
