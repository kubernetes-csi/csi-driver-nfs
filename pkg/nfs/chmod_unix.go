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

import "syscall"

// chmod uses syscall.Chmod to correctly handle setuid/setgid/sticky bits
// (e.g. 02770), since os.Chmod maps os.FileMode bits differently from raw
// Unix mode bits.
func chmod(path string, mode uint32) error {
	return syscall.Chmod(path, mode)
}
