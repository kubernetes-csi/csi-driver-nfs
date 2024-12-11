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
			if err := TarPack(srcPath, dstPath, true); err != nil {
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
			if err := TarUnpack(srcPath, dstPath, true); err != nil {
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
	if err := TarUnpack(mArchivePath, outputDir, true); err != nil {
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

func TestPackSameDir(t *testing.T) {
	inputDir := t.TempDir()

	err := TarPack(inputDir, filepath.Join(inputDir, "a.tar.gz"), false)

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
	if err := TarPack(inputDir, archivePath, true); err != nil {
		t.Fatalf("packing %s to %s: %v", inputDir, archivePath, err)
	}

	unpackedPath := filepath.Join(outputDir, "output")
	if err := TarUnpack(archivePath, unpackedPath, true); err != nil {
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
