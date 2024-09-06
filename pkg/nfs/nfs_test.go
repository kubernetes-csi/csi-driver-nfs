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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	azcache "sigs.k8s.io/cloud-provider-azure/pkg/cache"
)

const (
	fakeNodeID = "fakeNodeID"
)

func NewEmptyDriver(emptyField string) *Driver {
	var d *Driver
	switch emptyField {
	case "version":
		d = &Driver{
			name:    DefaultDriverName,
			version: "",
			nodeID:  fakeNodeID,
		}
	case "name":
		d = &Driver{
			name:    "",
			version: driverVersion,
			nodeID:  fakeNodeID,
		}
	default:
		d = &Driver{
			name:    DefaultDriverName,
			version: driverVersion,
			nodeID:  fakeNodeID,
		}
	}
	d.volumeLocks = NewVolumeLocks()
	getter := func(_ string) (interface{}, error) { return nil, nil }
	d.volStatsCache, _ = azcache.NewTimedCache(time.Minute, getter, false)
	return d
}

func TestNewFakeDriver(t *testing.T) {
	d := NewEmptyDriver("version")
	assert.Empty(t, d.version)

	d = NewEmptyDriver("name")
	assert.Empty(t, d.name)
}

func TestIsCorruptedDir(t *testing.T) {
	existingMountPath, err := os.MkdirTemp(os.TempDir(), "csi-mount-test")
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
			testFunc: func(_ *testing.T) {
				d := NewEmptyDriver("")
				d.endpoint = "tcp://127.0.0.1:0"
				d.Run(true)
			},
		},
		{
			name: "Successful run with node ID missing",
			testFunc: func(_ *testing.T) {
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

func TestNewControllerServiceCapability(t *testing.T) {
	tests := []struct {
		cap csi.ControllerServiceCapability_RPC_Type
	}{
		{
			cap: csi.ControllerServiceCapability_RPC_UNKNOWN,
		},
		{
			cap: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		},
		{
			cap: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		},
		{
			cap: csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		},
		{
			cap: csi.ControllerServiceCapability_RPC_GET_CAPACITY,
		},
	}
	for _, test := range tests {
		resp := NewControllerServiceCapability(test.cap)
		assert.NotNil(t, resp)
		assert.Equal(t, resp.XXX_sizecache, int32(0))
	}
}

func TestNewNodeServiceCapability(t *testing.T) {
	tests := []struct {
		cap csi.NodeServiceCapability_RPC_Type
	}{
		{
			cap: csi.NodeServiceCapability_RPC_UNKNOWN,
		},
		{
			cap: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		},
		{
			cap: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
		},
		{
			cap: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		},
	}
	for _, test := range tests {
		resp := NewNodeServiceCapability(test.cap)
		assert.NotNil(t, resp)
		assert.Equal(t, resp.XXX_sizecache, int32(0))
	}
}

func TestReplaceWithMap(t *testing.T) {
	tests := []struct {
		desc     string
		str      string
		m        map[string]string
		expected string
	}{
		{
			desc:     "empty string",
			str:      "",
			expected: "",
		},
		{
			desc:     "empty map",
			str:      "",
			m:        map[string]string{},
			expected: "",
		},
		{
			desc:     "empty key",
			str:      "prefix-" + pvNameMetadata,
			m:        map[string]string{"": "pv"},
			expected: "prefix-" + pvNameMetadata,
		},
		{
			desc:     "empty value",
			str:      "prefix-" + pvNameMetadata,
			m:        map[string]string{pvNameMetadata: ""},
			expected: "prefix-",
		},
		{
			desc:     "one replacement",
			str:      "prefix-" + pvNameMetadata,
			m:        map[string]string{pvNameMetadata: "pv"},
			expected: "prefix-pv",
		},
		{
			desc:     "multiple replacements",
			str:      pvcNamespaceMetadata + pvcNameMetadata,
			m:        map[string]string{pvcNamespaceMetadata: "namespace", pvcNameMetadata: "pvcname"},
			expected: "namespacepvcname",
		},
	}

	for _, test := range tests {
		result := replaceWithMap(test.str, test.m)
		if result != test.expected {
			t.Errorf("test[%s]: unexpected output: %v, expected result: %v", test.desc, result, test.expected)
		}
	}
}
