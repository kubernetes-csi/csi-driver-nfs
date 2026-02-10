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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"go.uber.org/goleak"
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

func TestRemoveEmptyDirs(t *testing.T) {
	parentDir, _ := os.Getwd()
	emptyDirOneLevel, _ := getWorkDirPath("emptyDir1")
	emptyDirTwoLevels, _ := getWorkDirPath("emptyDir2/emptyDir2")
	emptyDirThreeLevels, _ := getWorkDirPath("emptyDir3/emptyDir2/emptyDir3")

	tests := []struct {
		desc      string
		parentDir string
		dir       string
		expected  error
	}{
		{
			desc:      "empty path",
			parentDir: parentDir,
			expected:  nil,
		},
		{
			desc:      "empty dir with one level",
			parentDir: parentDir,
			dir:       emptyDirOneLevel,
			expected:  nil,
		},
		{
			desc:      "dir is not a subdirectory of parentDir",
			parentDir: "/dir1",
			dir:       "/dir2",
			expected:  fmt.Errorf("dir /dir2 is not a subdirectory of parentDir /dir1"),
		},
		{
			desc:      "empty dir with two levels",
			parentDir: parentDir,
			dir:       emptyDirTwoLevels,
			expected:  nil,
		},
		{
			desc:      "empty dir with three levels",
			parentDir: parentDir,
			dir:       emptyDirThreeLevels,
			expected:  nil,
		},
	}

	for _, test := range tests {
		if strings.Contains(test.dir, "emptyDir") {
			_ = makeDir(test.dir)
			defer os.RemoveAll(test.dir)
		}
		err := removeEmptyDirs(test.parentDir, test.dir)
		if !reflect.DeepEqual(err, test.expected) {
			t.Errorf("test[%s]: unexpected output: %v, expected result: %v", test.desc, err, test.expected)
		}
		if strings.Contains(test.dir, "emptyDir") {
			// directory should be removed
			if _, err := os.Stat(emptyDirOneLevel); !os.IsNotExist(err) {
				t.Errorf("test[%s]: directory %s should be removed", test.desc, emptyDirOneLevel)
			}
		}
	}
}

func TestWaitUntilTimeout(t *testing.T) {
	defer goleak.VerifyNone(t)
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
		if err != nil {
			time.Sleep(2 * time.Second)
			if err.Error() != test.expectedErr.Error() {
				t.Errorf("unexpected error: %v, expected error: %v", err, test.expectedErr)
			}
		}
	}
}
func TestGetVolumeCapabilityFromSecret(t *testing.T) {
	tests := []struct {
		desc     string
		volumeID string
		secret   map[string]string
		expected *csi.VolumeCapability
	}{
		{
			desc:     "secret contains mountOptions",
			volumeID: "vol-123",
			secret:   map[string]string{"mountOptions": "nfsvers=3"},
			expected: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{
						MountFlags: []string{"nfsvers=3"},
					},
				},
			},
		},
		{
			desc:     "secret does not contain mountOptions",
			volumeID: "vol-456",
			secret:   map[string]string{"otherKey": "otherValue"},
			expected: nil,
		},
		{
			desc:     "empty secret",
			volumeID: "vol-789",
			secret:   map[string]string{},
			expected: nil,
		},
		{
			desc:     "nil secret",
			volumeID: "vol-000",
			secret:   nil,
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result := getVolumeCapabilityFromSecret(test.volumeID, test.secret)
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("test[%s]: unexpected result: %v, expected: %v", test.desc, result, test.expected)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		desc     string
		path     string
		expected error
	}{
		{
			desc:     "valid path",
			path:     "/home/user/data",
			expected: nil,
		},
		{
			desc:     "valid relative path",
			path:     "user/data/file.txt",
			expected: nil,
		},
		{
			desc:     "empty path",
			path:     "",
			expected: nil,
		},
		{
			desc:     "root path",
			path:     "/",
			expected: nil,
		},
		{
			desc:     "path with single dot",
			path:     "/home/./user",
			expected: nil,
		},
		{
			desc:     "path with directory traversal at start",
			path:     "../etc/passwd",
			expected: fmt.Errorf("path contains directory traversal sequence"),
		},
		{
			desc:     "path with directory traversal in middle",
			path:     "/home/../etc/passwd",
			expected: fmt.Errorf("path contains directory traversal sequence"),
		},
		{
			desc:     "path with directory traversal at end",
			path:     "/home/user/..",
			expected: fmt.Errorf("path contains directory traversal sequence"),
		},
		{
			desc:     "path with multiple directory traversals",
			path:     "/home/../../etc/passwd",
			expected: fmt.Errorf("path contains directory traversal sequence"),
		},
		{
			desc:     "path with only directory traversal",
			path:     "..",
			expected: fmt.Errorf("path contains directory traversal sequence"),
		},
		{
			desc:     "path with triple dots (valid)",
			path:     "/home/.../data",
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result := validatePath(test.path)
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("test[%s]: unexpected output: %v, expected result: %v", test.desc, result, test.expected)
			}
		})
	}
}
