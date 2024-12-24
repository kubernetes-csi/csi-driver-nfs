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
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func TarPack(srcDirPath string, dstPath string, enableCompression bool) error {
	// normalize all paths to be absolute and clean
	dstPath, err := filepath.Abs(dstPath)
	if err != nil {
		return fmt.Errorf("normalizing destination path: %w", err)
	}

	srcDirPath, err = filepath.Abs(srcDirPath)
	if err != nil {
		return fmt.Errorf("normalizing source path: %w", err)
	}

	if strings.HasPrefix(filepath.Dir(dstPath), srcDirPath) {
		return fmt.Errorf("destination file %s cannot be under source directory %s", dstPath, srcDirPath)
	}

	tarFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("creating destination file: %w", err)
	}
	defer func() {
		err = errors.Join(err, closeAndWrapErr(tarFile, "closing destination file %s: %w", dstPath))
	}()

	var tarDst io.Writer = tarFile
	if enableCompression {
		gzipWriter := gzip.NewWriter(tarFile)
		defer func() {
			err = errors.Join(err, closeAndWrapErr(gzipWriter, "closing gzip writer"))
		}()
		tarDst = gzipWriter
	}

	tarWriter := tar.NewWriter(tarDst)
	defer func() {
		err = errors.Join(err, closeAndWrapErr(tarWriter, "closing tar writer"))
	}()

	// recursively visit every file and write it
	if err = filepath.Walk(
		srcDirPath,
		func(srcSubPath string, fileInfo fs.FileInfo, walkErr error) error {
			return tarVisitFileToPack(tarWriter, srcDirPath, srcSubPath, fileInfo, walkErr)
		},
	); err != nil {
		return fmt.Errorf("walking source directory: %w", err)
	}

	return nil
}

func tarVisitFileToPack(
	tarWriter *tar.Writer,
	srcPath string,
	srcSubPath string,
	fileInfo os.FileInfo,
	walkErr error,
) (err error) {
	if walkErr != nil {
		return walkErr
	}

	linkTarget := ""
	if fileInfo.Mode()&fs.ModeSymlink != 0 {
		linkTarget, err = os.Readlink(srcSubPath)
		if err != nil {
			return fmt.Errorf("reading link %s: %w", srcSubPath, err)
		}
	}

	tarHeader, err := tar.FileInfoHeader(fileInfo, linkTarget)
	if err != nil {
		return fmt.Errorf("creating tar header for %s: %w", srcSubPath, err)
	}

	// srcSubPath always starts with srcPath and both are absolute
	tarHeader.Name, err = filepath.Rel(srcPath, srcSubPath)
	if err != nil {
		return fmt.Errorf("making tar header name for file %s: %w", srcSubPath, err)
	}

	if err = tarWriter.WriteHeader(tarHeader); err != nil {
		return fmt.Errorf("writing tar header for file %s: %w", srcSubPath, err)
	}

	if !fileInfo.Mode().IsRegular() {
		return nil
	}

	srcFile, err := os.Open(srcSubPath)
	if err != nil {
		return fmt.Errorf("opening file being packed %s: %w", srcSubPath, err)
	}
	defer func() {
		err = errors.Join(err, closeAndWrapErr(srcFile, "closing file being packed %s: %w", srcSubPath))
	}()
	_, err = io.Copy(tarWriter, srcFile)
	if err != nil {
		return fmt.Errorf("packing file %s: %w", srcSubPath, err)
	}
	return nil
}

func TarUnpack(srcPath, dstDirPath string, enableCompression bool) (err error) {
	// normalize all paths to be absolute and clean
	srcPath, err = filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("normalizing archive path: %w", err)
	}

	dstDirPath, err = filepath.Abs(dstDirPath)
	if err != nil {
		return fmt.Errorf("normalizing archive destination path: %w", err)
	}

	tarFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("opening archive %s: %w", srcPath, err)
	}
	defer func() {
		err = errors.Join(err, closeAndWrapErr(tarFile, "closing archive %s: %w", srcPath))
	}()

	var tarDst io.Reader = tarFile
	if enableCompression {
		var gzipReader *gzip.Reader
		gzipReader, err = gzip.NewReader(tarFile)
		if err != nil {
			return fmt.Errorf("creating gzip reader: %w", err)
		}
		defer func() {
			err = errors.Join(err, closeAndWrapErr(gzipReader, "closing gzip reader: %w"))
		}()

		tarDst = gzipReader
	}

	tarReader := tar.NewReader(tarDst)

	for {
		var tarHeader *tar.Header
		tarHeader, err = tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar header of %s: %w", srcPath, err)
		}

		fileInfo := tarHeader.FileInfo()

		filePath := filepath.Join(dstDirPath, tarHeader.Name)

		// protect against "Zip Slip"
		if !strings.HasPrefix(filePath, dstDirPath) {
			// mimic standard error, which will be returned in future versions of Go by default
			// more info can be found by "tarinsecurepath" variable name
			return tar.ErrInsecurePath
		}

		fileDirPath := filePath
		if !fileInfo.Mode().IsDir() {
			fileDirPath = filepath.Dir(fileDirPath)
		}

		if err = os.MkdirAll(fileDirPath, 0755); err != nil {
			return fmt.Errorf("making dirs for path %s: %w", fileDirPath, err)
		}

		if fileInfo.Mode().IsDir() {
			continue
		}

		if fileInfo.Mode()&fs.ModeSymlink != 0 {
			if err := os.Symlink(tarHeader.Linkname, filePath); err != nil {
				return fmt.Errorf("creating symlink %s: %w", filePath, err)
			}
			continue
		}

		if err = tarUnpackFile(filePath, tarReader, fileInfo); err != nil {
			return fmt.Errorf("unpacking file %s: %w", filePath, err)
		}
	}
	return nil
}

func tarUnpackFile(dstFileName string, src io.Reader, srcFileInfo fs.FileInfo) (err error) {
	var dstFile *os.File
	dstFile, err = os.OpenFile(dstFileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcFileInfo.Mode().Perm())
	if err != nil {
		return fmt.Errorf("opening destination file %s: %w", dstFileName, err)
	}
	defer func() {
		err = errors.Join(err, closeAndWrapErr(dstFile, "closing destination file %s: %w", dstFile))
	}()

	n, err := io.Copy(dstFile, src)
	if err != nil {
		return fmt.Errorf("copying to destination file %s: %w", dstFileName, err)
	}

	if srcFileInfo.Mode().IsRegular() && n != srcFileInfo.Size() {
		return fmt.Errorf("written size check failed for %s: wrote %d, want %d", dstFileName, n, srcFileInfo.Size())
	}

	return nil
}

func closeAndWrapErr(closer io.Closer, errFormat string, a ...any) error {
	if err := closer.Close(); err != nil {
		a = append(a, err)
		return fmt.Errorf(errFormat, a...)
	}
	return nil
}
