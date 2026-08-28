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
	"fmt"
	"os"
)

// openArchiveForRead opens path for reading. On Windows FIFOs behave
// differently (named pipes need explicit connection semantics) and O_NOFOLLOW
// is not a POSIX flag; the checkArchiveFile mode gate (regular file only)
// remains the primary defense together with the io.LimitReader byte cap that
// TarUnpack applies around the returned descriptor.
func openArchiveForRead(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening archive %s: %w", path, err)
	}
	return f, nil
}
