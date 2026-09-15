//go:build !linux
// +build !linux

/*
Copyright 2026 The Kubernetes Authors.

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
	"time"

	mount "k8s.io/mount-utils"
)

func mountNFSWithTimeout(mounter mount.Interface, source, targetPath string, mountOptions []string, timeout time.Duration) error {
	execFunc := func() error {
		return mounter.Mount(source, targetPath, "nfs", mountOptions)
	}
	timeoutFunc := func() error {
		return fmt.Errorf("mount volume %s to %s timeout after %ds", source, targetPath, int(timeout/time.Second))
	}
	return WaitUntilTimeout(timeout, execFunc, timeoutFunc)
}
