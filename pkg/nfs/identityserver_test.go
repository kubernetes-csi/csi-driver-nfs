/*
Copyright 2020 The Kubernetes Authors.

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
	"context"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetPluginInfo(t *testing.T) {
	req := csi.GetPluginInfoRequest{}
	emptyNameDriver := NewEmptyDriver("name")
	emptyVersionDriver := NewEmptyDriver("version")
	tests := []struct {
		desc        string
		driver      *Driver
		expectedErr error
	}{
		{
			desc:        "Successful Request",
			driver:      NewEmptyDriver(""),
			expectedErr: nil,
		},
		{
			desc:        "Driver name missing",
			driver:      emptyNameDriver,
			expectedErr: status.Error(codes.Unavailable, "Driver name not configured"),
		},
		{
			desc:        "Driver version missing",
			driver:      emptyVersionDriver,
			expectedErr: status.Error(codes.Unavailable, "Driver is missing version"),
		},
	}

	for _, test := range tests {
		fakeIdentityServer := IdentityServer{
			Driver: test.driver,
		}
		_, err := fakeIdentityServer.GetPluginInfo(context.Background(), &req)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("Unexpected error: %v\nExpected: %v", err, test.expectedErr)
		}
	}
}

func TestProbe(t *testing.T) {
	d := NewEmptyDriver("")
	req := csi.ProbeRequest{}
	fakeIdentityServer := IdentityServer{
		Driver: d,
	}
	resp, err := fakeIdentityServer.Probe(context.Background(), &req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Ready.Value, true)
}

func TestGetPluginCapabilities(t *testing.T) {
	expectedCap := []*csi.PluginCapability{
		{
			Type: &csi.PluginCapability_Service_{
				Service: &csi.PluginCapability_Service{
					Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
				},
			},
		},
	}

	d := NewEmptyDriver("")
	fakeIdentityServer := IdentityServer{
		Driver: d,
	}
	req := csi.GetPluginCapabilitiesRequest{}
	resp, err := fakeIdentityServer.GetPluginCapabilities(context.Background(), &req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Capabilities, expectedCap)

}
