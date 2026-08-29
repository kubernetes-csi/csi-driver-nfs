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

//go:build !windows

package nfs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestTarUnpackRejectsSymlinkArchive(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.tar")
	if err := os.WriteFile(target, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "snap.tar")
	if err := os.Symlink(target, archive); err != nil {
		t.Fatal(err)
	}
	err := TarUnpack(archive, t.TempDir(), false, TarLimits{MaxArchiveSize: 1 << 20})
	if err == nil {
		t.Fatal("expected error opening symlinked archive under O_NOFOLLOW, got nil")
	}
}

func TestTarUnpackRejectsFIFOArchive(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "snap.tar")
	if err := syscall.Mkfifo(archive, 0o644); err != nil {
		t.Skipf("cannot create FIFO in test tmpdir: %v", err)
	}
	err := TarUnpack(archive, t.TempDir(), false, TarLimits{MaxArchiveSize: 1 << 20})
	if err == nil {
		t.Fatal("expected error unpacking FIFO archive, got nil")
	}
	// The error should not be io.EOF (which would indicate we hung reading
	// an empty pipe until close). Both O_NONBLOCK-open failure and the
	// non-regular-file mode gate produce non-nil errors quickly.
	if errors.Is(err, io.EOF) {
		t.Fatalf("expected non-EOF error rejecting FIFO, got: %v", err)
	}
	// When the descriptor-mode gate fires (rather than an O_NONBLOCK-open
	// failure at a lower layer), the returned error must be the distinct
	// invalid-type sentinel, not the size-limit sentinel — they mean
	// different things and callers may react differently.
	if errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("FIFO rejection should not surface as ErrArchiveTooLarge, got: %v", err)
	}
}

// TestTarUnpackRejectsFIFOArchiveWithoutMaxArchiveSize covers the case where
// only MaxFileSize / MaxFiles are configured (MaxArchiveSize left at zero).
// The previous checkArchiveFile fast-returned in that shape and never stat'd
// the descriptor, so a FIFO opened with O_NONBLOCK could be observed as EOF
// and accepted as an "empty archive". The regular-file gate now fires
// regardless of MaxArchiveSize.
func TestTarUnpackRejectsFIFOArchiveWithoutMaxArchiveSize(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "snap.tar")
	if err := syscall.Mkfifo(archive, 0o644); err != nil {
		t.Skipf("cannot create FIFO in test tmpdir: %v", err)
	}
	err := TarUnpack(archive, t.TempDir(), false, TarLimits{MaxFileSize: 1 << 20, MaxFiles: 100})
	if err == nil {
		t.Fatal("expected error unpacking FIFO archive (no MaxArchiveSize), got nil")
	}
	if errors.Is(err, io.EOF) {
		t.Fatalf("expected non-EOF error rejecting FIFO with no MaxArchiveSize, got: %v", err)
	}
	if errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("FIFO rejection should not surface as ErrArchiveTooLarge, got: %v", err)
	}
}

// TestTarPackRejectsFIFOSource proves that CreateSnapshot fails loudly at
// pack time when the source volume contains a mode the extractor cannot
// safely materialize (FIFO / device / socket). Without this gate
// tar.FileInfoHeader would happily serialize a FIFO header, ship a
// snapshot that TarUnpack later refuses, and only surface the problem at
// restore time.
func TestTarPackRejectsFIFOSource(t *testing.T) {
	src := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(src, "weird"), 0o644); err != nil {
		t.Skipf("cannot create FIFO in test tmpdir: %v", err)
	}
	archive := filepath.Join(t.TempDir(), "snap.tar")
	err := TarPack(src, archive, false, TarLimits{})
	if err == nil {
		t.Fatal("expected TarPack to reject FIFO source entry, got nil")
	}
}
