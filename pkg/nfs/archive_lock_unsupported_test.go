//go:build !linux

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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestArchiveUnsupportedPlatformPreservesData(t *testing.T) {
	for _, replace := range []bool{false, true} {
		for _, sourceExists := range []bool{false, true} {
			t.Run(fmt.Sprintf("replace=%t/source=%t", replace, sourceExists), func(t *testing.T) {
				share := t.TempDir()
				source := filepath.Join(share, "volume")
				archived := filepath.Join(share, "archived-volume")
				require.NoError(t, os.Mkdir(archived, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(archived, "payload"), []byte("old archive"), 0600))
				if sourceExists {
					require.NoError(t, os.Mkdir(source, 0700))
					require.NoError(t, os.WriteFile(filepath.Join(source, "payload"), []byte("new source"), 0600))
				}

				cs := NewControllerServer(NewDriver(&DriverOptions{RemoveArchivedVolumePath: replace}))
				err := cs.archiveVolume(share, source, "volume")
				require.Equal(t, codes.FailedPrecondition, status.Code(err))
				require.EqualError(t, err, "rpc error: code = FailedPrecondition desc = shared NFS archive locking requires Linux")

				data, err := os.ReadFile(filepath.Join(archived, "payload"))
				require.NoError(t, err)
				require.Equal(t, "old archive", string(data))
				if sourceExists {
					data, err = os.ReadFile(filepath.Join(source, "payload"))
					require.NoError(t, err)
					require.Equal(t, "new source", string(data))
				} else {
					require.NoDirExists(t, source)
				}
				require.NoFileExists(t, archiveLockPath(share, "volume"))
			})
		}
	}
}
