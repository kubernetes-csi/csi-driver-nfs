/*
Copyright 2019 The Kubernetes Authors.

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

import "github.com/container-storage-interface/spec/lib/go/csi"

const (
	fakeNodeID = "fakeNodeID"
)

func NewEmptyDriver(emptyField string) *Driver {
	var d *Driver
	var perm *uint32
	switch emptyField {
	case "version":
		d = &Driver{
			name:    DriverName,
			version: "",
			nodeID:  fakeNodeID,
			cap:     map[csi.VolumeCapability_AccessMode_Mode]bool{},
			perm:    perm,
		}
	case "name":
		d = &Driver{
			name:    "",
			version: version,
			nodeID:  fakeNodeID,
			cap:     map[csi.VolumeCapability_AccessMode_Mode]bool{},
			perm:    perm,
		}
	default:
		d = &Driver{
			name:    DriverName,
			version: version,
			nodeID:  fakeNodeID,
			cap:     map[csi.VolumeCapability_AccessMode_Mode]bool{},
			perm:    perm,
		}
	}

	return d
}
