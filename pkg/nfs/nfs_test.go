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

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
)

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

func TestNewFakeDriver(t *testing.T) {
	d := NewEmptyDriver("version")
	assert.Empty(t, d.version)

	d = NewEmptyDriver("name")
	assert.Empty(t, d.name)
}

func TestIsCorruptedDir(t *testing.T) {
	existingMountPath, err := ioutil.TempDir(os.TempDir(), "csi-mount-test")
	if err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}
	defer os.RemoveAll(existingMountPath)

	curruptedPath := filepath.Join(existingMountPath, "curruptedPath")
	if err := os.Symlink(existingMountPath, curruptedPath); err != nil {
		t.Fatalf("failed to create curruptedPath: %v", err)
	}

	tests := []struct {
		desc           string
		dir            string
		expectedResult bool
	}{
		{
			desc:           "NotExist dir",
			dir:            "/tmp/NotExist",
			expectedResult: false,
		},
		{
			desc:           "Existing dir",
			dir:            existingMountPath,
			expectedResult: false,
		},
	}

	for i, test := range tests {
		isCorruptedDir := IsCorruptedDir(test.dir)
		assert.Equal(t, test.expectedResult, isCorruptedDir, "TestCase[%d]: %s", i, test.desc)
	}
}

func TestRun(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Successful run",
			testFunc: func(t *testing.T) {
				d := NewEmptyDriver("")
				d.endpoint = "tcp://127.0.0.1:0"
				d.Run(true)
			},
		},
		{
			name: "Successful run with node ID missing",
			testFunc: func(t *testing.T) {
				d := NewEmptyDriver("")
				d.endpoint = "tcp://127.0.0.1:0"
				d.nodeID = ""
				d.Run(true)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}
