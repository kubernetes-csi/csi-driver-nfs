//go:build linux
// +build linux

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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"k8s.io/klog/v2"
	mount "k8s.io/mount-utils"
)

const (
	mountWaitNoChildProcesses = "wait: no child processes"
	mountKillGracePeriod      = 2 * time.Second
)

func mountNFSWithTimeout(mounter mount.Interface, source, targetPath string, mountOptions []string, timeout time.Duration) error {
	if _, ok := mounter.(*mount.Mounter); !ok {
		execFunc := func() error {
			return mounter.Mount(source, targetPath, "nfs", mountOptions)
		}
		timeoutFunc := func() error {
			return fmt.Errorf("mount volume %s to %s timeout after %ds", source, targetPath, int(timeout/time.Second))
		}
		return WaitUntilTimeout(timeout, execFunc, timeoutFunc)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := runNFSMountCommandContext(ctx, source, targetPath, mountOptions); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("mount volume %s to %s timeout after %ds", source, targetPath, int(timeout/time.Second))
		}
		return err
	}
	return nil
}

var execCommand = exec.Command

func waitForMountProcessExit(waitCh <-chan error, gracePeriod time.Duration) (bool, error) {
	select {
	case err := <-waitCh:
		return true, err
	case <-time.After(gracePeriod):
		return false, nil
	}
}

func runNFSMountCommandContext(ctx context.Context, source, targetPath string, mountOptions []string) error {
	mountArgs, mountArgsLogStr := mount.MakeMountArgsSensitive(source, targetPath, "nfs", mountOptions, nil)
	klog.V(4).Infof("Mounting cmd (%s) with arguments (%s)", "mount", mountArgsLogStr)

	cmd := execCommand("mount", mountArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		klog.Errorf("Mount failed to start: %v\nMounting command: %s\nMounting arguments: %s\n", err, "mount", mountArgsLogStr)
		return fmt.Errorf("mount failed to start: %v\nMounting command: %s\nMounting arguments: %s", err, "mount", mountArgsLogStr)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		if err != nil {
			if err.Error() == mountWaitNoChildProcesses {
				if cmd.ProcessState != nil && cmd.ProcessState.Success() {
					return nil
				}
				err = &exec.ExitError{ProcessState: cmd.ProcessState}
			}
			klog.Errorf("Mount failed: %v\nMounting command: %s\nMounting arguments: %s\nOutput: %s\n", err, "mount", mountArgsLogStr, output.String())
			return fmt.Errorf("mount failed: %v\nMounting command: %s\nMounting arguments: %s\nOutput: %s", err, "mount", mountArgsLogStr, output.String())
		}
		return nil
	case <-ctx.Done():
		if cmd.Process != nil {
			klog.Warningf("Mount timed out/cancelled, killing process group for pid %d (command: mount %s)", cmd.Process.Pid, mountArgsLogStr)
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
				klog.Warningf("Failed to kill mount process group for pid %d: %v", cmd.Process.Pid, err)
			}
		}
		if exited, err := waitForMountProcessExit(waitCh, mountKillGracePeriod); exited {
			if err != nil && err.Error() != mountWaitNoChildProcesses {
				klog.Warningf("Mount process for pid %d exited after timeout with error: %v", cmd.Process.Pid, err)
			}
		} else if cmd.Process != nil {
			klog.Warningf("Mount process group for pid %d did not exit within %v after SIGKILL; returning timeout", cmd.Process.Pid, mountKillGracePeriod)
		}
		return ctx.Err()
	}
}
