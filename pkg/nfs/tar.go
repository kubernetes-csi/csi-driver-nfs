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

func tarPack(dstFilePath, srcPath string, enableCompression bool) error {
	// normalize all paths to be absolute and clean
	dstFilePath, err := filepath.Abs(dstFilePath)
	if err != nil {
		return fmt.Errorf("normalizing destination path: %w", err)
	}

	srcPath, err = filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("normalizing source path: %w", err)
	}

	if strings.Index(dstFilePath, srcPath) == 0 {
		return fmt.Errorf("destination file %s cannot be under source directory %s", dstFilePath, srcPath)
	}

	tarFile, err := os.Create(dstFilePath)
	if err != nil {
		return fmt.Errorf("creating destination file: %w", err)
	}
	defer func() {
		err = errors.Join(err, closeAndWrapErr(tarFile, "closing destination file %s: %w", dstFilePath))
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
		srcPath,
		func(srcSubPath string, fileInfo fs.FileInfo, walkErr error) error {
			return tarVisitFileToPack(tarWriter, srcPath, srcSubPath, fileInfo, walkErr)
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

func tarUnpack(archivePath, dstPath string, enableCompression bool) (err error) {
	// normalize all paths to be absolute and clean
	archivePath, err = filepath.Abs(archivePath)
	if err != nil {
		return fmt.Errorf("normalizing archive path: %w", err)
	}

	dstPath, err = filepath.Abs(dstPath)
	if err != nil {
		return fmt.Errorf("normalizing archive destination path: %w", err)
	}

	tarFile, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive %s: %w", archivePath, err)
	}
	defer func() {
		err = errors.Join(err, closeAndWrapErr(tarFile, "closing archive %s: %w", archivePath))
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

		tarDst = tar.NewReader(gzipReader)
	}

	tarReader := tar.NewReader(tarDst)

	for {
		var tarHeader *tar.Header
		tarHeader, err = tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar header of %s: %w", archivePath, err)
		}

		fileInfo := tarHeader.FileInfo()

		filePath := filepath.Join(dstPath, tarHeader.Name)

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

		err = tarUnpackFile(filePath, tarReader, fileInfo)
		if err != nil {
			return fmt.Errorf("unpacking archive %s: %w", filePath, err)
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
