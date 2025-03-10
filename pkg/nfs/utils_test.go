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
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
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
			level:  8,
		},
		{
			method: "/csi.v1.Node/NodeGetCapabilities",
			level:  8,
		},
		{
			method: "/csi.v1.Node/NodeGetVolumeStats",
			level:  8,
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

func TestGetMountOptions(t *testing.T) {
	tests := []struct {
		desc    string
		context map[string]string
		result  string
	}{
		{
			desc:    "nil context",
			context: nil,
			result:  "",
		},
		{
			desc:    "empty context",
			context: map[string]string{},
			result:  "",
		},
		{
			desc:    "valid mountOptions",
			context: map[string]string{"mountOptions": "nfsvers=3"},
			result:  "nfsvers=3",
		},
		{
			desc:    "valid mountOptions(lowercase)",
			context: map[string]string{"mountoptions": "nfsvers=4"},
			result:  "nfsvers=4",
		},
	}

	for _, test := range tests {
		result := getMountOptions(test.context)
		if result != test.result {
			t.Errorf("Unexpected result: %s, expected: %s", result, test.result)
		}
	}
}

func TestChmodIfPermissionMismatch(t *testing.T) {
	permissionMatchingPath, _ := getWorkDirPath("permissionMatchingPath")
	_ = makeDir(permissionMatchingPath)
	defer os.RemoveAll(permissionMatchingPath)

	permissionMismatchPath, _ := getWorkDirPath("permissionMismatchPath")
	_ = os.MkdirAll(permissionMismatchPath, os.FileMode(0721))
	defer os.RemoveAll(permissionMismatchPath)

	tests := []struct {
		desc          string
		path          string
		mode          os.FileMode
		expectedError error
	}{
		{
			desc:          "Invalid path",
			path:          "invalid-path",
			mode:          0755,
			expectedError: fmt.Errorf("CreateFile invalid-path: The system cannot find the file specified"),
		},
		{
			desc:          "permission matching path",
			path:          permissionMatchingPath,
			mode:          0755,
			expectedError: nil,
		},
		{
			desc:          "permission mismatch path",
			path:          permissionMismatchPath,
			mode:          0755,
			expectedError: nil,
		},
	}

	for _, test := range tests {
		err := chmodIfPermissionMismatch(test.path, test.mode)
		if !reflect.DeepEqual(err, test.expectedError) {
			if err == nil || test.expectedError == nil && !strings.Contains(err.Error(), test.expectedError.Error()) {
				t.Errorf("test[%s]: unexpected error: %v, expected error: %v", test.desc, err, test.expectedError)
			}
		}
	}
}

// getWorkDirPath returns the path to the current working directory
func getWorkDirPath(dir string) (string, error) {
	path, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%c%s", path, os.PathSeparator, dir), nil
}

func TestGetServerFromSource(t *testing.T) {
	tests := []struct {
		desc   string
		server string
		result string
	}{
		{
			desc:   "ipv4",
			server: "10.127.0.1",
			result: "10.127.0.1",
		},
		{
			desc:   "ipv6",
			server: "0:0:0:0:0:0:0:1",
			result: "[0:0:0:0:0:0:0:1]",
		},
		{
			desc:   "ipv6 with brackets",
			server: "[0:0:0:0:0:0:0:2]",
			result: "[0:0:0:0:0:0:0:2]",
		},
		{
			desc:   "other fqdn",
			server: "bing.com",
			result: "bing.com",
		},
	}

	for _, test := range tests {
		result := getServerFromSource(test.server)
		if result != test.result {
			t.Errorf("Unexpected result: %s, expected: %s", result, test.result)
		}
	}
}

func TestSetKeyValueInMap(t *testing.T) {
	tests := []struct {
		desc     string
		m        map[string]string
		key      string
		value    string
		expected map[string]string
	}{
		{
			desc:  "nil map",
			key:   "key",
			value: "value",
		},
		{
			desc:     "empty map",
			m:        map[string]string{},
			key:      "key",
			value:    "value",
			expected: map[string]string{"key": "value"},
		},
		{
			desc:  "non-empty map",
			m:     map[string]string{"k": "v"},
			key:   "key",
			value: "value",
			expected: map[string]string{
				"k":   "v",
				"key": "value",
			},
		},
		{
			desc:     "same key already exists",
			m:        map[string]string{"subDir": "value2"},
			key:      "subDir",
			value:    "value",
			expected: map[string]string{"subDir": "value"},
		},
		{
			desc:     "case insensitive key already exists",
			m:        map[string]string{"subDir": "value2"},
			key:      "subdir",
			value:    "value",
			expected: map[string]string{"subDir": "value"},
		},
	}

	for _, test := range tests {
		setKeyValueInMap(test.m, test.key, test.value)
		if !reflect.DeepEqual(test.m, test.expected) {
			t.Errorf("test[%s]: unexpected output: %v, expected result: %v", test.desc, test.m, test.expected)
		}
	}
}

func TestValidateOnDeleteValue(t *testing.T) {
	tests := []struct {
		desc     string
		onDelete string
		expected error
	}{
		{
			desc:     "empty value",
			onDelete: "",
			expected: nil,
		},
		{
			desc:     "delete value",
			onDelete: "delete",
			expected: nil,
		},
		{
			desc:     "retain value",
			onDelete: "retain",
			expected: nil,
		},
		{
			desc:     "Retain value",
			onDelete: "Retain",
			expected: nil,
		},
		{
			desc:     "Delete value",
			onDelete: "Delete",
			expected: nil,
		},
		{
			desc:     "Delete value",
			onDelete: "DeleteRootSubDir",
			expected: nil,
		},
		{
			desc:     "Archive value",
			onDelete: "Archive",
			expected: nil,
		},
		{
			desc:     "archive value",
			onDelete: "archive",
			expected: nil,
		},
		{
			desc:     "invalid value",
			onDelete: "invalid",
			expected: fmt.Errorf("invalid value %s for OnDelete, supported values are %v", "invalid", supportedOnDeleteValues),
		},
	}

	for _, test := range tests {
		result := validateOnDeleteValue(test.onDelete)
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("test[%s]: unexpected output: %v, expected result: %v", test.desc, result, test.expected)
		}
	}
}

func TestWaitForPathNotExistWithTimeout(t *testing.T) {
	tests := []struct {
		desc     string
		path     string
		timeout  int
		expected error
	}{
		{
			desc:     "path does not exist",
			path:     "non-existent-path",
			timeout:  1,
			expected: nil,
		},
		{
			desc:     "path exists",
			path:     "/",
			timeout:  2,
			expected: fmt.Errorf("time out waiting for path / not exist"),
		},
	}

	for _, test := range tests {
		err := waitForPathNotExistWithTimeout(test.path, time.Duration(test.timeout))
		if !reflect.DeepEqual(err, test.expected) {
			t.Errorf("test[%s]: unexpected output: %v, expected result: %v", test.desc, err, test.expected)
		}
	}
}

func TestGetRootPath(t *testing.T) {
	tests := []struct {
		desc     string
		dir      string
		expected string
	}{
		{
			desc:     "empty path",
			dir:      "",
			expected: "",
		},
		{
			desc:     "root path",
			dir:      "/",
			expected: "",
		},
		{
			desc:     "subdir path",
			dir:      "/subdir",
			expected: "",
		},
		{
			desc:     "subdir path without leading slash",
			dir:      "subdir",
			expected: "subdir",
		},
		{
			desc:     "multiple subdir path without leading slash",
			dir:      "subdir/subdir2",
			expected: "subdir",
		},
	}

	for _, test := range tests {
		result := getRootDir(test.dir)
		if result != test.expected {
			t.Errorf("Unexpected result: %s, expected: %s", result, test.expected)
		}
	}
}

func TestWaitUntilTimeout(t *testing.T) {
	tests := []struct {
		desc        string
		timeout     time.Duration
		execFunc    ExecFunc
		timeoutFunc TimeoutFunc
		expectedErr error
	}{
		{
			desc:    "execFunc returns error",
			timeout: 1 * time.Second,
			execFunc: func() error {
				return fmt.Errorf("execFunc error")
			},
			timeoutFunc: func() error {
				return fmt.Errorf("timeout error")
			},
			expectedErr: fmt.Errorf("execFunc error"),
		},
		{
			desc:    "execFunc timeout",
			timeout: 1 * time.Second,
			execFunc: func() error {
				time.Sleep(2 * time.Second)
				return nil
			},
			timeoutFunc: func() error {
				return fmt.Errorf("timeout error")
			},
			expectedErr: fmt.Errorf("timeout error"),
		},
		{
			desc:    "execFunc completed successfully",
			timeout: 1 * time.Second,
			execFunc: func() error {
				return nil
			},
			timeoutFunc: func() error {
				return fmt.Errorf("timeout error")
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		err := WaitUntilTimeout(test.timeout, test.execFunc, test.timeoutFunc)
		if err != nil && (err.Error() != test.expectedErr.Error()) {
			t.Errorf("unexpected error: %v, expected error: %v", err, test.expectedErr)
		}
	}
}
