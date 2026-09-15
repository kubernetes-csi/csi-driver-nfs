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
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWaitForMountProcessExitTimesOutWhenWaitBlocked(t *testing.T) {
	waitCh := make(chan error)
	gracePeriod := 50 * time.Millisecond

	start := time.Now()
	exited, err := waitForMountProcessExit(waitCh, gracePeriod)
	elapsed := time.Since(start)

	if exited {
		t.Fatal("expected blocked wait channel to time out")
	}
	if err != nil {
		t.Fatalf("expected nil error on grace-period timeout, got: %v", err)
	}
	if elapsed < gracePeriod {
		t.Fatalf("wait returned too early: elapsed=%v gracePeriod=%v", elapsed, gracePeriod)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("wait was not bounded by grace period: elapsed=%v gracePeriod=%v", elapsed, gracePeriod)
	}
}

func TestRunNFSMountCommandContextKillsProcessGroup(t *testing.T) {
	pidFile, err := os.CreateTemp(t.TempDir(), "mount-helper-pids-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	pidFilePath := pidFile.Name()
	_ = pidFile.Close()

	oldExecCommand := execCommand
	execCommand = func(_ string, _ ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestRunNFSMountCommandContextHelper", "--")
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"GO_HELPER_PID_FILE="+pidFilePath,
		)
		return cmd
	}
	defer func() { execCommand = oldExecCommand }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan error, 1)
	start := time.Now()
	go func() {
		resultCh <- runNFSMountCommandContext(ctx, "server:/share", "/target", []string{"nolock", "nfsvers=4"})
	}()

	parentPID, childPID := readHelperPIDs(t, pidFilePath)
	cancel()

	err = <-resultCh
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("expected bounded wait after cancellation, elapsed=%v", elapsed)
	}

	assertProcessGoneEventually(t, parentPID, 2*time.Second)
	assertProcessNotRunningEventually(t, childPID, 2*time.Second)
}

func TestRunNFSMountCommandContextHelper(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	pidFilePath := os.Getenv("GO_HELPER_PID_FILE")
	if pidFilePath == "" {
		fmt.Fprintln(os.Stderr, "GO_HELPER_PID_FILE is required")
		os.Exit(2)
	}

	child := exec.Command("sleep", "300")
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start child: %v\n", err)
		os.Exit(2)
	}

	content := fmt.Sprintf("%d %d", os.Getpid(), child.Process.Pid)
	if err := os.WriteFile(pidFilePath, []byte(content), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write pid file: %v\n", err)
		_ = child.Process.Kill()
		os.Exit(2)
	}

	time.Sleep(300 * time.Second)
	os.Exit(0)
}

func readHelperPIDs(t *testing.T, pidFilePath string) (int, int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		content, err := os.ReadFile(pidFilePath)
		if err == nil && strings.TrimSpace(string(content)) != "" {
			fields := strings.Fields(string(content))
			if len(fields) != 2 {
				t.Fatalf("unexpected pid file content %q", string(content))
			}
			parentPID, err := strconv.Atoi(fields[0])
			if err != nil {
				t.Fatalf("failed to parse parent pid %q: %v", fields[0], err)
			}
			childPID, err := strconv.Atoi(fields[1])
			if err != nil {
				t.Fatalf("failed to parse child pid %q: %v", fields[1], err)
			}
			return parentPID, childPID
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for pid file %s", pidFilePath)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func assertProcessGoneEventually(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d still exists after %v", pid, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func assertProcessNotRunningEventually(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		state, err := getProcessState(pid)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			return
		}
		if err == nil && state == 'Z' {
			return
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("process %d still appears to be running after %v (state err: %v)", pid, timeout, err)
			}
			t.Fatalf("process %d still appears to be running after %v (state=%q)", pid, timeout, string(state))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func getProcessState(pid int) (byte, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, os.ErrNotExist
		}
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, fmt.Errorf("unexpected /proc stat format for pid %d: %q", pid, string(data))
	}
	return fields[2][0], nil
}
