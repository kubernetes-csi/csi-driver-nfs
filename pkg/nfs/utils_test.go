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
	"bytes"
	"flag"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

var (
	invalidEndpoint = "invalid-endpoint"
	emptyAddr       = "tcp://"
)

func TestParseEndpoint(t *testing.T) {
	cases := []struct {
		desc        string
		endpoint    string
		resproto    string
		respaddr    string
		expectedErr error
	}{
		{
			desc:        "invalid endpoint",
			endpoint:    invalidEndpoint,
			expectedErr: fmt.Errorf("Invalid endpoint: %v", invalidEndpoint),
		},
		{
			desc:        "empty address",
			endpoint:    emptyAddr,
			expectedErr: fmt.Errorf("Invalid endpoint: %v", emptyAddr),
		},
		{
			desc:        "valid tcp",
			endpoint:    "tcp://address",
			resproto:    "tcp",
			respaddr:    "address",
			expectedErr: nil,
		},
		{
			desc:        "valid unix",
			endpoint:    "unix://address",
			resproto:    "unix",
			respaddr:    "address",
			expectedErr: nil,
		},
	}

	for _, test := range cases {
		test := test //pin
		t.Run(test.desc, func(t *testing.T) {
			proto, addr, err := ParseEndpoint(test.endpoint)

			// Verify
			if test.expectedErr == nil && err != nil {
				t.Errorf("test %q failed: %v", test.desc, err)
			}
			if test.expectedErr != nil && err == nil {
				t.Errorf("test %q failed; expected error %v, got success", test.desc, test.expectedErr)
			}
			if test.expectedErr == nil {
				if test.resproto != proto {
					t.Errorf("test %q failed; expected proto %v, got proto %v", test.desc, test.resproto, proto)
				}
				if test.respaddr != addr {
					t.Errorf("test %q failed; expected addr %v, got addr %v", test.desc, test.respaddr, addr)
				}
			}
		})
	}
}

func TestGetLogLevel(t *testing.T) {
	tests := []struct {
		method string
		level  int32
	}{
		{
			method: "/csi.v1.Identity/Probe",
			level:  10,
		},
		{
			method: "/csi.v1.Node/NodeGetCapabilities",
			level:  10,
		},
		{
			method: "/csi.v1.Node/NodeGetVolumeStats",
			level:  10,
		},
		{
			method: "",
			level:  2,
		},
		{
			method: "unknown",
			level:  2,
		},
	}

	for _, test := range tests {
		level := getLogLevel(test.method)
		if level != test.level {
			t.Errorf("returned level: (%v), expected level: (%v)", level, test.level)
		}
	}
}

func TestLogGRPC(t *testing.T) {
	// SET UP
	klog.InitFlags(nil)
	if e := flag.Set("logtostderr", "false"); e != nil {
		t.Error(e)
	}
	if e := flag.Set("alsologtostderr", "false"); e != nil {
		t.Error(e)
	}
	if e := flag.Set("v", "100"); e != nil {
		t.Error(e)
	}
	flag.Parse()

	buf := new(bytes.Buffer)
	klog.SetOutput(buf)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) { return nil, nil }
	info := grpc.UnaryServerInfo{
		FullMethod: "fake",
	}

	tests := []struct {
		name   string
		req    interface{}
		expStr string
	}{
		{
			"with secrets",
			&csi.NodeStageVolumeRequest{
				VolumeId: "vol_1",
				Secrets: map[string]string{
					"account_name": "k8s",
					"account_key":  "testkey",
				},
				XXX_sizecache: 100,
			},
			`GRPC request: {"secrets":"***stripped***","volume_id":"vol_1"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// EXECUTE
			_, _ = logGRPC(context.Background(), test.req, &info, handler)
			klog.Flush()
			// ASSERT
			assert.Contains(t, buf.String(), "GRPC call: fake")
			assert.Contains(t, buf.String(), test.expStr)
			assert.Contains(t, buf.String(), "GRPC response: null")
			// CLEANUP
			buf.Reset()
		})
	}
}
