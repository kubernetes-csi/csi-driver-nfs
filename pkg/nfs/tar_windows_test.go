//go:build windows

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
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestTarUnpackRejectsSymlinkArchiveWindows verifies that openArchiveForRead
// rejects a symlink (reparse point) archive on Windows without reading the
// symlink target, analogous to TestTarUnpackRejectsSymlinkArchive on Unix.
func TestTarUnpackRejectsSymlinkArchiveWindows(t *testing.T) {
	dir := t.TempDir()

	target := filepath.Join(dir, "real.tar")
	if err := os.WriteFile(target, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(dir, "snap.tar")
	if err := os.Symlink(target, archive); err != nil {
		t.Skipf("cannot create symlink (requires developer mode or elevated privileges): %v", err)
	}

	err := TarUnpack(archive, t.TempDir(), false, TarLimits{MaxArchiveSize: 1 << 20})
	if err == nil {
		t.Fatal("expected error opening symlinked archive, got nil")
	}
	if !errors.Is(err, ErrArchiveInvalidType) {
		t.Fatalf("expected ErrArchiveInvalidType for symlink archive, got: %v", err)
	}
}

// TestTarUnpackRejectsDirectoryArchiveWindows verifies that a directory
// (which on Windows can be opened with FILE_FLAG_BACKUP_SEMANTICS) is
// rejected by the regular-file mode gate in checkArchiveFile.
func TestTarUnpackRejectsDirectoryArchiveWindows(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "snap.tar")
	if err := os.Mkdir(archive, 0o755); err != nil {
		t.Fatal(err)
	}

	err := TarUnpack(archive, t.TempDir(), false, TarLimits{MaxArchiveSize: 1 << 20})
	if err == nil {
		t.Fatal("expected error unpacking directory as archive, got nil")
	}
	if !errors.Is(err, ErrArchiveInvalidType) {
		t.Fatalf("expected ErrArchiveInvalidType for directory archive, got: %v", err)
	}
}
