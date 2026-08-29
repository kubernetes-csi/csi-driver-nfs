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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/sumdb/dirhash"
)

const (
	code producedFrom = '0'
	cli  producedFrom = '1'
)

type producedFrom byte

const archiveFileExt = ".tar.gz"

func TestPackUnpack(t *testing.T) {
	inputPath := t.TempDir()
	generateFileSystem(t, inputPath)

	outputPath := t.TempDir()

	// produced file names (without extensions) have a suffix,
	// which determine the last operation:
	// "0" means that it was produced from code
	// "1" means that it was produced from CLI
	// e.g.: "testdata011.tar.gz" - was packed from code,
	// then unpacked from cli and packed again from cli

	pathsBySuffix := make(map[string]string)

	// number of pack/unpack operations
	opNum := 4

	// generate all operation combinations
	fileNum := int(math.Pow(2, float64(opNum)))
	for i := 0; i < fileNum; i++ {
		binStr := fmt.Sprintf("%b", i)

		// left-pad with zeroes
		binStr = strings.Repeat("0", opNum-len(binStr)) + binStr

		// copy slices to satisfy type system
		ops := make([]producedFrom, opNum)
		for opIdx := 0; opIdx < opNum; opIdx++ {
			ops[opIdx] = producedFrom(binStr[opIdx])
		}

		// produce folders and archives
		produce(t, pathsBySuffix, inputPath, outputPath, ops...)
	}

	// compare all unpacked directories
	paths := slices.Collect(maps.Values(pathsBySuffix))
	assertUnpackedFilesEqual(t, inputPath, paths)
}

func produce(
	t *testing.T,
	results map[string]string,
	inputDirPath string,
	outputDirPath string,
	ops ...producedFrom,
) {
	const baseName = "testdata"

	for i := 0; i < len(ops); i++ {
		packing := i%2 == 0

		srcPath := inputDirPath
		if i > 0 {
			prevSuffix := string(ops[:i])
			srcPath = filepath.Join(outputDirPath, baseName+prevSuffix)
			if !packing {
				srcPath += archiveFileExt
			}
		}

		suffix := string(ops[:i+1])
		dstPath := filepath.Join(outputDirPath, baseName+suffix)
		if packing {
			dstPath += archiveFileExt
		}

		if _, ok := results[suffix]; ok {
			continue
		}

		switch {
		case packing && ops[i] == code:
			// packing from code
			if err := TarPack(srcPath, dstPath, true, TarLimits{}); err != nil {
				t.Fatalf("packing '%s' with TarPack into '%s': %v", srcPath, dstPath, err)
			}
		case packing && ops[i] == cli:
			// packing from CLI
			if out, err := exec.Command("tar", "-C", srcPath, "-czvf", dstPath, ".").CombinedOutput(); err != nil {
				t.Log("TAR OUTPUT:", string(out))
				t.Fatalf("packing '%s' with tar into '%s': %v", srcPath, dstPath, err)
			}
		case !packing && ops[i] == code:
			// unpacking from code
			if err := TarUnpack(srcPath, dstPath, true, TarLimits{}); err != nil {
				t.Fatalf("unpacking '%s' with TarUnpack into '%s': %v", srcPath, dstPath, err)
			}
		case !packing && ops[i] == cli:
			// unpacking from CLI
			// tar requires destination directory to exist
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				t.Fatalf("making dir '%s' for unpacking with tar: %v", dstPath, err)
			}
			if out, err := exec.Command("tar", "-xzvf", srcPath, "-C", dstPath).CombinedOutput(); err != nil {
				t.Log("TAR OUTPUT:", string(out))
				t.Fatalf("unpacking '%s' with tar into '%s': %v", srcPath, dstPath, err)
			}
		default:
			t.Fatalf("unknown suffix: %s", string(ops[i]))
		}

		results[suffix] = dstPath
	}
}

func assertUnpackedFilesEqual(t *testing.T, originalDir string, paths []string) {
	originalDirHash, err := dirhash.HashDir(originalDir, "_", dirhash.DefaultHash)
	if err != nil {
		t.Fatal("failed hashing original dir ", err)
	}

	for _, p := range paths {
		if strings.HasSuffix(p, archiveFileExt) {
			// archive, not a directory
			continue
		}

		// unpacked directory
		hs, err := dirhash.HashDir(p, "_", dirhash.DefaultHash)
		if err != nil {
			t.Fatal("failed hashing dir ", err)
		}

		if hs != originalDirHash {
			t.Errorf("expected '%s' to have the same hash as '%s', got different", originalDir, p)
		}
	}
}

func generateFileSystem(t *testing.T, inputPath string) {
	// empty directory
	if err := os.MkdirAll(filepath.Join(inputPath, "empty_dir"), 0755); err != nil {
		t.Fatalf("generating empty directory: %v", err)
	}

	// deep empty directories
	deepEmptyDirPath := filepath.Join(inputPath, "deep_empty_dir", strings.Repeat("/0/1/2", 20))
	if err := os.MkdirAll(deepEmptyDirPath, 0755); err != nil {
		t.Fatalf("generating deep empty directory '%s': %v", deepEmptyDirPath, err)
	}

	// empty file
	f, err := os.Create(filepath.Join(inputPath, "empty_file"))
	if err != nil {
		t.Fatalf("generating empty file: %v", err)
	}
	f.Close()

	// big (100MB) file
	bigFilePath := filepath.Join(inputPath, "big_file")
	for i := byte(0); i < 100; i++ {
		// write 1MB
		err := os.WriteFile(bigFilePath, bytes.Repeat([]byte{i}, 1024*1024), 0755)
		if err != nil {
			t.Fatalf("generating empty file: %v", err)
		}
	}
}

func TestUnpackZipSlip(t *testing.T) {
	// Arrange: produce malicious archive
	inputDir := t.TempDir()

	const mContent = "malicious content"
	const mFileName = "malicious.txt"
	const mHeaderPath = "../" + mFileName // attack: path traversal
	var mArchivePath = filepath.Join(inputDir, "malicious.tar.gz")

	// temp file to pack
	maliciousFile, err := os.Create(mArchivePath)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	gzWriter := gzip.NewWriter(maliciousFile)
	tarWriter := tar.NewWriter(gzWriter)

	// define a malicious file header
	maliciousHeader := &tar.Header{
		Name: mHeaderPath,
		Size: int64(len(mContent)),
		Mode: 0600,
	}

	err = tarWriter.WriteHeader(maliciousHeader)
	if err != nil {
		t.Fatalf("failed to write malicious header: %v", err)
	}

	// write malicious content
	_, err = tarWriter.Write([]byte(mContent))
	if err != nil {
		t.Fatalf("failed to write content: %v", err)
	}

	err = errors.Join(tarWriter.Close(), gzWriter.Close(), maliciousFile.Close())
	if err != nil {
		t.Fatalf("failed to close writers: %v", err)
	}

	// Act & Assert: unpack nearby, expect error
	var outputDir = filepath.Join(inputDir, "output")
	if err := TarUnpack(mArchivePath, outputDir, true, TarLimits{}); err != nil {
		if !errors.Is(err, tar.ErrInsecurePath) {
			t.Fatalf("expected error tar.ErrInsecurePath, got: %v", err)
		}
	} else {
		t.Error("unpack of malicious file succeeded, expected it to fail")
	}

	// Assert: check that file did not escape
	var attackPath = filepath.Join(inputDir, mFileName)
	if _, err := os.Stat(attackPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed to check the existence of the malicious file: %v", err)
		}
	} else {
		t.Errorf("malicious file escaped the destination: %s", attackPath)
	}
}

func TestUnpackZipSlipAbsoluteName(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: "/etc/passwd",
		Size: 4,
		Mode: 0600,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("root")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "abs.tar")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	err := TarUnpack(archive, t.TempDir(), false, TarLimits{})
	if !errors.Is(err, tar.ErrInsecurePath) {
		t.Fatalf("expected tar.ErrInsecurePath for absolute entry name, got: %v", err)
	}
}

// TestUnpackAllowsDoubleDotInFilename asserts the tar unpack sanitizer does
// not over-reject: names that merely contain ".." as a substring (e.g.
// "report..txt") are legitimate outputs of TarPack and must round-trip. The
// segment-based check should treat ".." only as a path component, never as
// an arbitrary substring. Regression guard for review comment on PR #1255.
func TestUnpackAllowsDoubleDotInFilename(t *testing.T) {
	const fileName = "report..txt"
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := []byte("hello")
	if err := tw.WriteHeader(&tar.Header{
		Name: fileName,
		Size: int64(len(content)),
		Mode: 0600,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "double-dot.tar")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	if err := TarUnpack(archive, dst, false, TarLimits{}); err != nil {
		t.Fatalf("unexpected error for legitimate double-dot filename %q: %v", fileName, err)
	}

	extracted := filepath.Join(dst, fileName)
	got, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
}

func TestPackSameDir(t *testing.T) {
	inputDir := t.TempDir()

	err := TarPack(inputDir, filepath.Join(inputDir, "a.tar.gz"), false, TarLimits{})

	const expectedErr = "cannot be under source directory"
	if err == nil {
		t.Errorf("expected error '%s', got success", expectedErr)
	} else if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error '%s', got: %v", expectedErr, err)
	}
}

func TestSymlinks(t *testing.T) {
	inputDir := t.TempDir()

	testContent := []byte(time.Now().String())

	testFileName := "d.txt"
	testFilePath := filepath.Join(inputDir, testFileName)

	if err := os.WriteFile(testFilePath, testContent, 0644); err != nil {
		t.Fatalf("writing to %s: %v", testFilePath, err)
	}

	absSymlinkName := "abs_symlink_to_" + testFileName
	absSymlinkPath := filepath.Join(inputDir, absSymlinkName)
	if err := os.Symlink(testFilePath, absSymlinkPath); err != nil {
		t.Fatalf("creating absolute symlink %s: %v", absSymlinkPath, err)
	}

	relSymlinkName := "rel_symlink_to_" + testFileName
	relSymlinkPath := filepath.Join(inputDir, relSymlinkName)

	relSymlinkTgt := "." + string(filepath.Separator) + testFileName
	if err := os.Symlink(relSymlinkTgt, relSymlinkPath); err != nil {
		t.Fatalf("creating relative symlink %s: %v", relSymlinkPath, err)
	}

	outputDir := t.TempDir()

	archivePath := filepath.Join(outputDir, "output.tar.gz")
	if err := TarPack(inputDir, archivePath, true, TarLimits{}); err != nil {
		t.Fatalf("packing %s to %s: %v", inputDir, archivePath, err)
	}

	unpackedPath := filepath.Join(outputDir, "output")
	if err := TarUnpack(archivePath, unpackedPath, true, TarLimits{}); err != nil {
		t.Fatalf("unpacking %s to %s: %v", archivePath, unpackedPath, err)
	}

	// check absolute symlink
	outputAbsSymlinkPath := filepath.Join(unpackedPath, absSymlinkName)
	outputAbsSymlinkTgt, err := os.Readlink(outputAbsSymlinkPath)
	if err != nil {
		t.Fatalf("reading absolute link %s: %v", outputAbsSymlinkPath, err)
	}
	if outputAbsSymlinkTgt != testFilePath {
		t.Errorf("expected absolute symlink to point to %s, got %s", testFilePath, outputAbsSymlinkTgt)
	}
	if data, err := os.ReadFile(outputAbsSymlinkPath); err != nil {
		t.Fatalf("reading file %s: %v", outputAbsSymlinkPath, err)
	} else if !bytes.Equal(testContent, data) {
		t.Errorf("expected file %s to be: %X, got %X", outputAbsSymlinkPath, testContent, data)
	}

	// check relative symlink
	outputRelSymlinkPath := filepath.Join(unpackedPath, relSymlinkName)
	outputRelSymlinkTgt, err := os.Readlink(outputRelSymlinkPath)
	if err != nil {
		t.Fatalf("reading relative link %s: %v", outputRelSymlinkPath, err)
	}
	if outputRelSymlinkTgt != relSymlinkTgt {
		t.Errorf("expected relative symlink to point to %s, got %s", relSymlinkTgt, outputRelSymlinkTgt)
	}
	if data, err := os.ReadFile(outputRelSymlinkPath); err != nil {
		t.Fatalf("reading file %s: %v", outputRelSymlinkPath, err)
	} else if !bytes.Equal(testContent, data) {
		t.Errorf("expected file %s to be: %X, got %X", outputRelSymlinkPath, testContent, data)
	}
}

func TestTarUnpackPreservesTimestamps(t *testing.T) {
	// Create a source directory with known timestamps
	srcDir := t.TempDir()
	subDir := filepath.Join(srcDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(subDir, "testfile.txt")
	if err := os.WriteFile(filePath, []byte("hello timestamps"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set known timestamps (2020-06-15 12:00:00 UTC)
	knownTime := time.Date(2020, 6, 15, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filePath, knownTime, knownTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(subDir, knownTime, knownTime); err != nil {
		t.Fatal(err)
	}

	// Pack
	archivePath := filepath.Join(t.TempDir(), "test.tar.gz")
	if err := TarPack(srcDir, archivePath, true, TarLimits{}); err != nil {
		t.Fatalf("TarPack failed: %v", err)
	}

	// Unpack
	dstDir := t.TempDir()
	if err := TarUnpack(archivePath, dstDir, true, TarLimits{}); err != nil {
		t.Fatalf("TarUnpack failed: %v", err)
	}

	// Verify file timestamp
	restoredFile := filepath.Join(dstDir, "subdir", "testfile.txt")
	fi, err := os.Stat(restoredFile)
	if err != nil {
		t.Fatal(err)
	}
	if diff := fi.ModTime().Sub(knownTime); diff < -time.Second || diff > time.Second {
		t.Errorf("file mtime: got %v, want %v (diff %v)", fi.ModTime(), knownTime, diff)
	}

	// Verify directory timestamp
	restoredDir := filepath.Join(dstDir, "subdir")
	di, err := os.Stat(restoredDir)
	if err != nil {
		t.Fatal(err)
	}
	if diff := di.ModTime().Sub(knownTime); diff < -time.Second || diff > time.Second {
		t.Errorf("dir mtime: got %v, want %v (diff %v)", di.ModTime(), knownTime, diff)
	}
}

func TestUnpackLimits(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("more data"), 0644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := TarPack(srcDir, archivePath, true, TarLimits{}); err != nil {
		t.Fatalf("TarPack failed: %v", err)
	}
	fi, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		limits  TarLimits
		wantErr error
	}{
		{
			name:   "within limits",
			limits: TarLimits{MaxArchiveSize: fi.Size(), MaxFileSize: 100, MaxFiles: 10},
		},
		{
			name:    "archive too large",
			limits:  TarLimits{MaxArchiveSize: fi.Size() - 1},
			wantErr: ErrArchiveTooLarge,
		},
		{
			name:    "file too large",
			limits:  TarLimits{MaxFileSize: 4},
			wantErr: ErrFileTooLarge,
		},
		{
			name:    "too many files",
			limits:  TarLimits{MaxFiles: 1},
			wantErr: ErrTooManyFiles,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dst := t.TempDir()
			err := TarUnpack(archivePath, dst, true, test.limits)
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %v, got success", test.wantErr)
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("expected error %v, got: %v", test.wantErr, err)
			}
		})
	}
}

func TestPackLimits(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("more data"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("file too large", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "snap.tar.gz")
		err := TarPack(srcDir, dst, true, TarLimits{MaxFileSize: 4})
		if err == nil {
			t.Fatal("expected error, got success")
		}
		if !errors.Is(err, ErrFileTooLarge) {
			t.Fatalf("expected ErrFileTooLarge, got: %v", err)
		}
		assertArchiveRemoved(t, dst)
	})

	t.Run("too many files", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "snap.tar.gz")
		err := TarPack(srcDir, dst, true, TarLimits{MaxFiles: 1})
		if err == nil {
			t.Fatal("expected error, got success")
		}
		if !errors.Is(err, ErrTooManyFiles) {
			t.Fatalf("expected ErrTooManyFiles, got: %v", err)
		}
		assertArchiveRemoved(t, dst)
	})

	t.Run("archive too large", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "snap.tar.gz")
		err := TarPack(srcDir, dst, true, TarLimits{MaxArchiveSize: 1})
		if err == nil {
			t.Fatal("expected error, got success")
		}
		if !errors.Is(err, ErrArchiveTooLarge) {
			t.Fatalf("expected ErrArchiveTooLarge, got: %v", err)
		}
		assertArchiveRemoved(t, dst)
	})

	t.Run("within limits", func(t *testing.T) {
		if err := TarPack(srcDir, filepath.Join(t.TempDir(), "snap.tar.gz"), true, TarLimits{MaxFileSize: 100, MaxFiles: 10, MaxArchiveSize: 1 << 20}); err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
	})
}

func TestPackStopsWhenArchiveSizeExceeded(t *testing.T) {
	srcDir := t.TempDir()
	payload := bytes.Repeat([]byte("x"), 64*1024)
	if err := os.WriteFile(filepath.Join(srcDir, "big.bin"), payload, 0644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "snap.tar")
	err := TarPack(srcDir, dst, false, TarLimits{MaxArchiveSize: 1024})
	if err == nil {
		t.Fatal("expected error, got success")
	}
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("expected ErrArchiveTooLarge, got: %v", err)
	}
	assertArchiveRemoved(t, dst)
}

func assertArchiveRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected archive %s to be removed, stat: %v", path, err)
	}
}

// TestTarUnpackRejectsUnsupportedTypeflag proves that an entry whose
// FileInfo().Mode() reports irregular (not dir, symlink, or regular file)
// is rejected at the top of the extraction loop. Without this gate the
// MaxFileSize check and the per-file io.LimitReader would both be
// short-circuited (they only fire on IsRegular()), letting the entry
// stream unbounded bytes into an ordinary file on disk.
func TestTarUnpackRejectsUnsupportedTypeflag(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "weird.tar")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	// TypeChar (character device) maps to os.ModeDevice|os.ModeCharDevice,
	// which is neither IsDir, IsRegular, nor a symlink bit. It is a stand-in
	// for the general "tar entry with a payload but non-regular mode"
	// hazard (extension headers, block devices, FIFOs, sockets, or reserved
	// typeflags): the extractor is not equipped to safely materialize them.
	if err := tw.WriteHeader(&tar.Header{
		Name:     "dev/weird",
		Mode:     0o600,
		Size:     0,
		Typeflag: tar.TypeChar,
		Devmajor: 1,
		Devminor: 3,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := TarUnpack(archive, t.TempDir(), false, TarLimits{}); err == nil {
		t.Fatal("expected TarUnpack to reject unsupported tar entry type, got nil")
	} else if !strings.Contains(err.Error(), "unsupported tar entry") {
		t.Fatalf("expected 'unsupported tar entry' error, got: %v", err)
	}
}

func TestTarUnpackBoundsBytesReadPastValidatedSize(t *testing.T) {
	// End-to-end path: an archive that grows between initial write and
	// TarUnpack must be rejected. Note that in this shape checkArchiveFile
	// runs Stat *after* the append and observes the grown size, so it is
	// checkArchiveFile — not the io.LimitReader cap — that fires here.
	// The direct-reader test below (TestBoundedArchiveReaderCapsPostValidationGrowth)
	// exercises the LimitReader cap in isolation so a regression in the
	// TOCTOU protection is detected even if checkArchiveFile passes.
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "snap.tar")
	if err := TarPack(srcDir, archive, false, TarLimits{}); err != nil {
		t.Fatal(err)
	}
	origInfo, err := os.Stat(archive)
	if err != nil {
		t.Fatal(err)
	}
	origSize := origInfo.Size()

	// Append garbage that would blow past MaxArchiveSize if read.
	f, err := os.OpenFile(archive, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(bytes.Repeat([]byte{0}, 4<<20)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// checkArchiveFile with the original size should reject the grown file.
	err = TarUnpack(archive, t.TempDir(), false, TarLimits{MaxArchiveSize: origSize})
	if err == nil {
		t.Fatal("expected ErrArchiveTooLarge on grown archive, got nil")
	}
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("expected ErrArchiveTooLarge, got: %v", err)
	}
}

// TestBoundedArchiveReaderCapsPostValidationGrowth focuses on the exact
// reader TarUnpack wraps around the archive file after checkArchiveFile
// returns. It models the case where the file grows AFTER the Stat/size
// check has already passed — the checkArchiveFile Stat pass in the
// end-to-end test above would not catch this because it inspects the
// current size at call time. If the io.LimitReader cap in TarUnpack were
// removed or set to zero for a positive MaxArchiveSize, this test would
// read all 100 bytes instead of the 10-byte cap and fail.
func TestBoundedArchiveReaderCapsPostValidationGrowth(t *testing.T) {
	src := bytes.NewReader(bytes.Repeat([]byte{'x'}, 100))
	r := boundedArchiveReader(src, 10)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("boundedArchiveReader read %d bytes, want capped at 10", len(got))
	}

	// Zero limit means "unbounded" (pre-existing behavior); ensure the helper
	// preserves that semantic and does not accidentally wrap in a 0-byte cap.
	src = bytes.NewReader(bytes.Repeat([]byte{'x'}, 100))
	r = boundedArchiveReader(src, 0)
	got, err = io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll (unbounded): %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("boundedArchiveReader with MaxArchiveSize=0 read %d bytes, want unbounded (100)", len(got))
	}

	// Negative limit is equivalent to "unbounded" per boundedArchiveReader's
	// contract (matches how checkArchiveSize treats it). Guard against a
	// future refactor that starts passing io.LimitReader a negative n.
	src = bytes.NewReader(bytes.Repeat([]byte{'x'}, 100))
	r = boundedArchiveReader(src, -1)
	got, err = io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll (negative): %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("boundedArchiveReader with MaxArchiveSize=-1 read %d bytes, want unbounded (100)", len(got))
	}
}

// TestTarPackRejectsSymlinkDestination verifies that TarPack refuses to write
// through a pre-existing symlink at the destination path. Without this gate
// an attacker who can write to the snapshot directory could plant a symlink
// and redirect the archive write to an arbitrary path.
func TestTarPackRejectsSymlinkDestination(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "data.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	dstDir := t.TempDir()
	target := filepath.Join(dstDir, "real.tar")
	archive := filepath.Join(dstDir, "snap.tar")

	// Pre-create a symlink at the archive destination.
	if err := os.Symlink(target, archive); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	err := TarPack(src, archive, false, TarLimits{})
	if err == nil {
		t.Fatal("expected TarPack to reject symlink destination, got nil")
	}
	if !errors.Is(err, ErrArchiveInvalidType) {
		t.Fatalf("expected ErrArchiveInvalidType, got: %v", err)
	}

	// Verify the target file was not created (symlink was not followed).
	if _, statErr := os.Stat(target); statErr == nil {
		t.Fatal("symlink target was created; TarPack followed the symlink")
	}
}

// TestTarUnpackHardLink verifies that TarUnpack correctly materializes
// hard-link entries (tar.TypeLink) instead of creating empty files.
// Without explicit Typeflag handling, hard-link entries report regular
// file mode with Size==0, causing silent data loss on restore.
func TestTarUnpackHardLink(t *testing.T) {
	// Build a tar archive containing a regular file and a hard link to it.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	content := []byte("hard-link-content")
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     "original.txt",
		Size:     int64(len(content)),
		Mode:     0o644,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeLink,
		Name:     "link.txt",
		Linkname: "original.txt",
		Mode:     0o644,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Write the archive and unpack it.
	archive := filepath.Join(t.TempDir(), "test.tar")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	if err := TarUnpack(archive, dst, false, TarLimits{}); err != nil {
		t.Fatalf("TarUnpack failed: %v", err)
	}

	// Verify both files exist and have the same content.
	origData, err := os.ReadFile(filepath.Join(dst, "original.txt"))
	if err != nil {
		t.Fatalf("reading original.txt: %v", err)
	}
	linkData, err := os.ReadFile(filepath.Join(dst, "link.txt"))
	if err != nil {
		t.Fatalf("reading link.txt: %v", err)
	}
	if string(origData) != string(content) {
		t.Fatalf("original.txt content = %q, want %q", origData, content)
	}
	if string(linkData) != string(content) {
		t.Fatalf("link.txt content = %q, want %q (hard link not materialized)", linkData, content)
	}
}

// TestTarUnpackHardLinkEscapeRejected verifies that a hard-link entry
// whose target points outside the extraction directory is rejected.
func TestTarUnpackHardLinkEscapeRejected(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeLink,
		Name:     "evil.txt",
		Linkname: "../../../etc/passwd",
		Mode:     0o644,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "test.tar")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	err := TarUnpack(archive, dst, false, TarLimits{})
	if err == nil {
		t.Fatal("expected error for hard link escaping extraction directory, got nil")
	}
}

// TestTarUnpackRejectsMalformedDirSize verifies that a directory entry
// with a non-zero Size is rejected before the payload is decompressed.
func TestTarUnpackRejectsMalformedDirSize(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     "baddir/",
		Size:     1 << 30, // 1 GiB
		Mode:     0o755,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "test.tar")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	err := TarUnpack(archive, t.TempDir(), false, TarLimits{MaxFileSize: 1 << 20})
	if err == nil {
		t.Fatal("expected error for directory entry with non-zero size, got nil")
	}
}

// TestTarUnpackHardLinkSymlinkTraversal verifies that a hard-link entry
// whose target traverses through an intermediate symlink outside the
// extraction directory is rejected.
func TestTarUnpackHardLinkSymlinkTraversal(t *testing.T) {
	// Build an archive: symlink "pivot" -> "..", then hardlink to "pivot/x"
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// First: a regular file so there's something to link to outside
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeSymlink,
		Name:     "pivot",
		Linkname: "..",
		Mode:     0o777,
	}); err != nil {
		t.Fatal(err)
	}

	// Create a target file outside dst that the hard link would reach
	dst := t.TempDir()
	victimPath := filepath.Join(filepath.Dir(dst), "victim.txt")
	if err := os.WriteFile(victimPath, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(victimPath)

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeLink,
		Name:     "escaped.txt",
		Linkname: "pivot/victim.txt",
		Mode:     0o644,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "test.tar")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	err := TarUnpack(archive, dst, false, TarLimits{})
	if err == nil {
		t.Fatal("expected error for hard link traversing symlink outside dst, got nil")
	}
}
