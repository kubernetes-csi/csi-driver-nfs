//go:build !windows

/*
Copyright 2024 The Kubernetes Authors.

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
	"syscall"
)

// openArchiveForRead opens path for reading without following symlinks and
// without blocking on FIFOs. Under an attacker-controlled path (the snapshot
// archive lives on NFS), symlink replacement is refused by the kernel
// (O_NOFOLLOW → ELOOP) and FIFO replacement returns ENXIO instead of
// blocking os.Open forever (O_NONBLOCK). Regular-file I/O with O_NONBLOCK is
// a no-op on Linux, so no extra fcntl is needed after the mode check in
// checkArchiveFile confirms the descriptor is a regular file.
func openArchiveForRead(path string) (*os.File, error) {
	flag := os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	f, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return nil, fmt.Errorf("opening archive %s: %w", path, err)
	}
	return f, nil
}
