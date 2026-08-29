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

//go:build windows

package nfs

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// openArchiveForRead opens path for reading with Windows reparse-point /
// no-follow semantics, analogous to O_NOFOLLOW on Unix.
//
// Motivation: on an attacker-controlled snapshot path, os.Open would follow
// a symlink / mount point / any other NTFS reparse point transparently, and
// the subsequent checkArchiveFile mode gate — which stats the file handle —
// would observe the *target's* regular-file mode and let the read proceed.
// That defeats the "archive path must be a regular file" defense on the
// only supported non-Unix platform, so a hostile actor who can write to
// the snapshot directory could redirect the restore read to any other
// readable regular file on the machine.
//
// The fix opens the handle with FILE_FLAG_OPEN_REPARSE_POINT, which tells
// Windows to open the reparse point itself instead of the target. We then
// inspect the file attributes: if FILE_ATTRIBUTE_REPARSE_POINT is set, the
// path is a reparse point (symlink, mount point, WSL AF_UNIX socket, etc.)
// and we reject it here without following. Regular files return an fd
// backed by the actual snapshot bytes, and the downstream checkArchiveFile
// mode / size gates apply as before.
//
// FILE_FLAG_BACKUP_SEMANTICS is passed so directories (which the mode gate
// rejects a moment later) can be opened at all — otherwise CreateFile fails
// with ERROR_ACCESS_DENIED on directories, masking the real
// ErrArchiveInvalidType error from checkArchiveFile with a confusing
// permission error.
func openArchiveForRead(path string) (*os.File, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("converting archive path %s: %w", path, err)
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("opening archive %s: %w", path, err)
	}
	// Reject reparse points (symlinks, mount points, other NTFS reparse
	// tags) before handing the descriptor to the tar reader. The mode
	// check in checkArchiveFile is the second line of defense; catching
	// it here means the error is a specific ErrArchiveInvalidType with
	// path context rather than an opaque "not a regular file" from Stat
	// on a handle the caller doesn't yet know is a reparse point.
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("getting file information for %s: %w", path, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("%w: %s (reparse point / symlink)", ErrArchiveInvalidType, path)
	}
	return os.NewFile(uintptr(handle), path), nil
}
