//go:build !windows
// +build !windows

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
	"os"
	"syscall"
	"testing"
)

func TestChmodIfPermissionMismatchSpecialBits(t *testing.T) {
	tests := []struct {
		desc          string
		initialMode   uint32
		requestedMode uint32
	}{
		{
			desc:          "setgid bit 02770",
			initialMode:   0770,
			requestedMode: 02770,
		},
		{
			desc:          "sticky bit 01777",
			initialMode:   0777,
			requestedMode: 01777,
		},
		{
			desc:          "setgid already set 02770 -> 02770 no change",
			initialMode:   02770,
			requestedMode: 02770,
		},
		{
			desc:          "setuid bit 04755",
			initialMode:   0755,
			requestedMode: 04755,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			dir := t.TempDir()
			targetPath := dir + "/testdir"
			if err := os.Mkdir(targetPath, 0777); err != nil {
				t.Fatalf("failed to create test dir: %v", err)
			}
			// Set initial permissions using syscall to support special bits
			if err := syscall.Chmod(targetPath, test.initialMode); err != nil {
				t.Fatalf("failed to set initial mode: %v", err)
			}

			if err := chmodIfPermissionMismatch(targetPath, test.requestedMode); err != nil {
				t.Fatalf("chmodIfPermissionMismatch failed: %v", err)
			}

			// Verify the final permissions
			info, err := os.Lstat(targetPath)
			if err != nil {
				t.Fatalf("failed to stat: %v", err)
			}

			// Get raw mode bits via syscall for accurate comparison
			stat := info.Sys().(*syscall.Stat_t)
			actualMode := uint32(stat.Mode) & 07777
			if actualMode != test.requestedMode {
				t.Errorf("expected mode 0%o, got 0%o", test.requestedMode, actualMode)
			}
		})
	}
}
