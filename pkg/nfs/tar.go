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
	"sort"
	"strings"
	"time"
)

// TarLimits bounds snapshot archive packing and extraction.
// A zero or negative value for a field means that limit is not enforced.
type TarLimits struct {
	// MaxArchiveSize is the maximum size of the archive file on disk, in bytes.
	MaxArchiveSize int64
	// MaxFileSize is the maximum uncompressed size of any regular file, in bytes.
	MaxFileSize int64
	// MaxFiles is the maximum number of archive entries, including directories
	// and symlinks.
	MaxFiles int64
}

func (l TarLimits) hasLimits() bool {
	return l.MaxArchiveSize > 0 || l.MaxFileSize > 0 || l.MaxFiles > 0
}

var (
	// ErrArchiveTooLarge is returned when a snapshot archive exceeds MaxArchiveSize.
	ErrArchiveTooLarge = errors.New("snapshot archive exceeds max size")
	// ErrArchiveInvalidType is returned when a snapshot archive path is not a
	// regular file (e.g. FIFO, device, symlink, directory). This is a
	// descriptor-mode validation failure, not a size violation — kept
	// distinct from ErrArchiveTooLarge so operators can tell the two apart
	// without parsing error text, and so callers can react differently
	// (e.g. surface a different Kubernetes Event).
	ErrArchiveInvalidType = errors.New("snapshot archive is not a regular file")
	// ErrFileTooLarge is returned when a file in a snapshot archive exceeds MaxFileSize.
	ErrFileTooLarge = errors.New("snapshot archive contains a file that exceeds max size")
	// ErrTooManyFiles is returned when a snapshot archive exceeds MaxFiles entries.
	ErrTooManyFiles = errors.New("snapshot archive exceeds max file count")
)

type tarPackState struct {
	limits    TarLimits
	fileCount int64
}

// limitedWriter stops writes once limit bytes have been written to the
// underlying archive file (the on-disk .tar / .tar.gz size).
type limitedWriter struct {
	w     io.Writer
	n     int64
	limit int64
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.limit <= 0 {
		return l.w.Write(p)
	}
	remaining := l.limit - l.n
	if remaining <= 0 {
		return 0, fmt.Errorf("%w: wrote %d bytes, max %d", ErrArchiveTooLarge, l.n, l.limit)
	}
	truncated := false
	if int64(len(p)) > remaining {
		p = p[:remaining]
		truncated = true
	}
	n, err := l.w.Write(p)
	l.n += int64(n)
	if err != nil {
		return n, err
	}
	if truncated {
		return n, fmt.Errorf("%w: wrote %d bytes, max %d", ErrArchiveTooLarge, l.n, l.limit)
	}
	return n, nil
}

func TarPack(srcDirPath string, dstPath string, enableCompression bool, limits TarLimits) (err error) {
	// normalize all paths to be absolute and clean
	dstPath, err = filepath.Abs(dstPath)
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

	// Reject pre-existing symlinks at the destination path before creating
	// the archive. Without this check os.Create follows the symlink and
	// overwrites its target — an attacker who can write to the snapshot
	// directory could pre-plant a symlink and redirect the archive write to
	// an arbitrary path.
	if fi, lErr := os.Lstat(dstPath); lErr == nil && fi.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%w: destination %s is a symlink", ErrArchiveInvalidType, dstPath)
	}

	tarFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("creating destination file: %w", err)
	}
	// Run last so writers are closed before we inspect or remove the file.
	defer func() {
		if err != nil {
			_ = os.Remove(dstPath)
			return
		}
		if sizeErr := checkArchiveSize(dstPath, limits); sizeErr != nil {
			_ = os.Remove(dstPath)
			err = sizeErr
		}
	}()
	defer func() {
		err = errors.Join(err, closeAndWrapErr(tarFile, "closing destination file %s: %w", dstPath))
	}()

	var tarDst io.Writer = tarFile
	if limits.MaxArchiveSize > 0 {
		tarDst = &limitedWriter{w: tarFile, limit: limits.MaxArchiveSize}
	}
	if enableCompression {
		gzipWriter := gzip.NewWriter(tarDst)
		defer func() {
			err = errors.Join(err, closeAndWrapErr(gzipWriter, "closing gzip writer"))
		}()
		tarDst = gzipWriter
	}

	tarWriter := tar.NewWriter(tarDst)
	defer func() {
		err = errors.Join(err, closeAndWrapErr(tarWriter, "closing tar writer"))
	}()

	state := &tarPackState{limits: limits}
	// recursively visit every file and write it
	if err = filepath.Walk(
		srcDirPath,
		func(srcSubPath string, fileInfo fs.FileInfo, walkErr error) error {
			return tarVisitFileToPack(tarWriter, srcDirPath, srcSubPath, fileInfo, walkErr, state)
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
	state *tarPackState,
) (err error) {
	if walkErr != nil {
		return walkErr
	}

	state.fileCount++
	if state.limits.MaxFiles > 0 && state.fileCount > state.limits.MaxFiles {
		return fmt.Errorf("%w: packed %d entries, max %d", ErrTooManyFiles, state.fileCount, state.limits.MaxFiles)
	}
	// Reject source entry modes the extractor is not equipped to materialize,
	// so CreateSnapshot fails loudly here instead of shipping a ready snapshot
	// that TarUnpack later refuses. tar.FileInfoHeader is happy to serialize
	// FIFOs / devices / sockets from the source volume, but the extractor
	// only ever writes directories, symlinks, and regular files.
	mode := fileInfo.Mode()
	if !mode.IsDir() && mode&fs.ModeSymlink == 0 && !mode.IsRegular() {
		return fmt.Errorf("unsupported source entry %s: mode %s", srcSubPath, mode)
	}
	if mode.IsRegular() && state.limits.MaxFileSize > 0 && fileInfo.Size() > state.limits.MaxFileSize {
		return fmt.Errorf("%w: %s is %d bytes, max %d", ErrFileTooLarge, srcSubPath, fileInfo.Size(), state.limits.MaxFileSize)
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

func TarUnpack(srcPath, dstDirPath string, enableCompression bool, limits TarLimits) (err error) {
	// normalize all paths to be absolute and clean
	srcPath, err = filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("normalizing archive path: %w", err)
	}

	dstDirPath, err = filepath.Abs(dstDirPath)
	if err != nil {
		return fmt.Errorf("normalizing archive destination path: %w", err)
	}
	// Ensure destination exists before resolving, so EvalSymlinks works on all platforms.
	if mkErr := os.MkdirAll(dstDirPath, 0755); mkErr != nil {
		return fmt.Errorf("creating destination directory: %w", mkErr)
	}
	// Resolve symlinks in dstDirPath itself so containment checks work correctly
	// on platforms where temp paths are symlinks (e.g., macOS /tmp -> /private/tmp)
	// or short path names (e.g., Windows RUNNER~1 -> runneradmin).
	dstDirPath, err = filepath.EvalSymlinks(dstDirPath)
	if err != nil {
		return fmt.Errorf("resolving destination path: %w", err)
	}

	tarFile, err := openArchiveForRead(srcPath)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, closeAndWrapErr(tarFile, "closing archive %s: %w", srcPath))
	}()
	if err = checkArchiveFile(tarFile, srcPath, limits); err != nil {
		return err
	}

	// Bound bytes actually read to the size we validated in checkArchiveFile.
	// This closes the TOCTOU window between Stat() and Read(): a concurrent
	// writer growing the file (or an attacker replacing the fd contents via a
	// hard-linked path) cannot stream more than MaxArchiveSize bytes past the
	// size check. Only apply the cap when a MaxArchiveSize was configured; a
	// zero limit means "unbounded" and pre-existed the hardening.
	tarDst := boundedArchiveReader(tarFile, limits.MaxArchiveSize)
	if enableCompression {
		var gzipReader *gzip.Reader
		// Read gzip from the capped tarDst, not the raw file, so the archive
		// byte cap is enforced on the compressed side too. (Decompression-bomb
		// growth is a separate concern already bounded per-entry by MaxFileSize
		// and MaxFiles further down.)
		gzipReader, err = gzip.NewReader(tarDst)
		if err != nil {
			return fmt.Errorf("creating gzip reader: %w", err)
		}
		defer func() {
			err = errors.Join(err, closeAndWrapErr(gzipReader, "closing gzip reader: %w"))
		}()

		tarDst = gzipReader
	}

	tarReader := tar.NewReader(tarDst)

	// Collect directory timestamps to restore after all files are written,
	// because creating files inside a directory updates the directory's mtime.
	type dirTimestamp struct {
		path    string
		modTime time.Time
		accTime time.Time
	}
	var dirTimestamps []dirTimestamp
	var fileCount int64

	for {
		var tarHeader *tar.Header
		tarHeader, err = tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar header of %s: %w", srcPath, err)
		}

		fileCount++
		if limits.MaxFiles > 0 && fileCount > limits.MaxFiles {
			return fmt.Errorf("%w: archive %s exceeds max %d entries", ErrTooManyFiles, srcPath, limits.MaxFiles)
		}

		fileInfo := tarHeader.FileInfo()
		mode := fileInfo.Mode()
		// Only three entry shapes are ever materialized below (directory,
		// symlink, regular file). Reject anything else — including tar
		// entries whose Typeflag maps to an irregular Mode (extension
		// headers, GNU/character/block/FIFO/socket types, or reserved flags)
		// — up front. Without this gate an irregular entry would skip the
		// header MaxFileSize check just below and reach tarUnpackFile /
		// tarWriteFile, which also short-circuits its per-file io.LimitReader
		// on !IsRegular(), so a hostile archive could stream unbounded bytes
		// into an oversized ordinary file on disk.
		if !mode.IsDir() && mode&fs.ModeSymlink == 0 && !mode.IsRegular() {
			return fmt.Errorf("unsupported tar entry %s: type %q / mode %s", tarHeader.Name, string(tarHeader.Typeflag), mode)
		}
		// Apply header-size sanity check to ALL entry types, not just
		// regular files. A malformed directory or symlink header can
		// declare a large Size; tar.Reader.Next() drains that payload
		// through gzip before any downstream limit, so a small
		// compressed archive could force unbounded decompression.
		// Directories and symlinks should have Size==0; reject any
		// non-regular entry that claims a non-zero payload.
		if !mode.IsRegular() && tarHeader.Size > 0 {
			return fmt.Errorf("non-regular entry %s (type %q) declares payload size %d; rejecting",
				tarHeader.Name, string(tarHeader.Typeflag), tarHeader.Size)
		}
		if mode.IsRegular() {
			if tarHeader.Size < 0 || (limits.MaxFileSize > 0 && tarHeader.Size > limits.MaxFileSize) {
				return fmt.Errorf("%w: %s is %d bytes, max %d", ErrFileTooLarge, tarHeader.Name, tarHeader.Size, limits.MaxFileSize)
			}
		}

		filePath := filepath.Join(dstDirPath, tarHeader.Name)

		// Sanitize the archive entry name to prevent directory traversal
		// ("Zip Slip", CWE-22). The cleaned relative name must not escape
		// the destination directory via ".." components.
		cleanName := filepath.Clean(tarHeader.Name)
		if strings.Contains(cleanName, "..") {
			return tar.ErrInsecurePath
		}

		// Robust containment check: use filepath.Rel to prevent prefix collisions
		// (e.g., dstDirPath="/tmp/out" vs filePath="/tmp/out2/...")
		if !isPathWithinBase(dstDirPath, filePath) {
			return tar.ErrInsecurePath
		}

		// Symlink-traversal guard: verify that the real (resolved) parent directory
		// of filePath is still within dstDirPath. This catches cases where a prior
		// symlink entry created a link inside dstDirPath that points outside, and a
		// subsequent entry tries to write through that symlink (e.g., symlink "a" -> /etc
		// followed by entry "a/passwd").
		// For directories, check filepath.Dir(filePath) — the existing parent — so that
		// a symlink "a" followed by "a/b" is caught before MkdirAll creates "b" outside.
		parentDir := filepath.Dir(filePath)
		if parentDir != dstDirPath && filePath != dstDirPath {
			// Walk up to the nearest existing ancestor to handle cases where
			// parentDir doesn't exist yet but an ancestor symlink escapes.
			checkDir := parentDir
			for checkDir != dstDirPath {
				if realDir, evalErr := filepath.EvalSymlinks(checkDir); evalErr == nil {
					if !isPathWithinBase(dstDirPath, realDir) {
						return tar.ErrInsecurePath
					}
					break
				}
				checkDir = filepath.Dir(checkDir)
			}
		}

		fileDirPath := filePath
		if !fileInfo.Mode().IsDir() {
			fileDirPath = filepath.Dir(fileDirPath)
		}

		if err = os.MkdirAll(fileDirPath, 0755); err != nil {
			return fmt.Errorf("making dirs for path %s: %w", fileDirPath, err)
		}

		if fileInfo.Mode().IsDir() {
			// Remove any existing symlink at filePath to prevent MkdirAll
			// and later Chtimes from following it outside dstDirPath.
			if existing, lErr := os.Lstat(filePath); lErr == nil && existing.Mode()&fs.ModeSymlink != 0 {
				if err = os.Remove(filePath); err != nil {
					return fmt.Errorf("removing symlink before mkdir %s: %w", filePath, err)
				}
				if err = os.MkdirAll(filePath, 0755); err != nil {
					return fmt.Errorf("making directory %s: %w", filePath, err)
				}
			}
			dirTimestamps = append(dirTimestamps, dirTimestamp{
				path:    filePath,
				modTime: tarHeader.ModTime,
				accTime: tarHeader.AccessTime,
			})
			continue
		}

		// Hard-link entries (tar.TypeLink) report regular-file mode in Go,
		// so the mode gate above does not catch them. GNU tar can emit
		// hard-link entries for ordinary files, and their Size is zero.
		// Extracting them through the regular-file path would create an
		// empty file and silently lose the linked content. Materialize
		// them as proper hard links when both the link target and the
		// destination are within the extraction directory; reject
		// otherwise.
		if tarHeader.Typeflag == tar.TypeLink {
			linkTarget := filepath.Join(dstDirPath, tarHeader.Linkname)
			if !isPathWithinBase(dstDirPath, linkTarget) {
				return fmt.Errorf("hard link %s -> %s escapes extraction directory", tarHeader.Name, tarHeader.Linkname)
			}
			// Resolve symlinks in the link target path to prevent
			// traversal via intermediate symlinks. An archive could
			// create "pivot -> .." then hard-link to "pivot/victim";
			// the lexical check above passes but os.Link would follow
			// the symlink and link to a file outside dstDirPath.
			if resolvedTarget, evalErr := filepath.EvalSymlinks(linkTarget); evalErr == nil {
				if !isPathWithinBase(dstDirPath, resolvedTarget) {
					return fmt.Errorf("hard link %s -> %s resolves to %s outside extraction directory",
						tarHeader.Name, tarHeader.Linkname, resolvedTarget)
				}
			}
			if existing, lErr := os.Lstat(filePath); lErr == nil && existing.Mode()&fs.ModeSymlink != 0 {
				if err = os.Remove(filePath); err != nil {
					return fmt.Errorf("removing symlink before hard link %s: %w", filePath, err)
				}
			}
			if err = os.Link(linkTarget, filePath); err != nil {
				return fmt.Errorf("creating hard link %s -> %s: %w", filePath, linkTarget, err)
			}
			continue
		}

		if fileInfo.Mode()&fs.ModeSymlink != 0 {
			if err := os.Symlink(tarHeader.Linkname, filePath); err != nil {
				return fmt.Errorf("creating symlink %s: %w", filePath, err)
			}
			continue
		}

		// Remove any existing symlink at filePath to prevent following it
		// when writing a regular file (a malicious tar could plant a symlink
		// then overwrite it with a regular file entry targeting outside dst).
		if existing, lErr := os.Lstat(filePath); lErr == nil && existing.Mode()&fs.ModeSymlink != 0 {
			if err = os.Remove(filePath); err != nil {
				return fmt.Errorf("removing symlink before file write %s: %w", filePath, err)
			}
		}

		if err = tarUnpackFile(filePath, tarReader, tarHeader, limits.MaxFileSize); err != nil {
			return fmt.Errorf("unpacking file %s: %w", filePath, err)
		}
	}

	// Restore directory timestamps deepest-first so that restoring a parent's
	// mtime is not undone by a subsequent Chtimes on a child directory.
	sort.Slice(dirTimestamps, func(i, j int) bool {
		return strings.Count(dirTimestamps[i].path, string(os.PathSeparator)) >
			strings.Count(dirTimestamps[j].path, string(os.PathSeparator))
	})
	for _, dt := range dirTimestamps {
		if dt.modTime.IsZero() {
			continue
		}
		accTime := dt.accTime
		if accTime.IsZero() {
			accTime = dt.modTime
		}
		if err := os.Chtimes(dt.path, accTime, dt.modTime); err != nil {
			return fmt.Errorf("restoring timestamps for directory %s: %w", dt.path, err)
		}
	}

	return nil
}

func tarUnpackFile(dstFileName string, src io.Reader, header *tar.Header, maxFileSize int64) (err error) {
	srcFileInfo := header.FileInfo()

	if err = tarWriteFile(dstFileName, src, srcFileInfo, maxFileSize); err != nil {
		return err
	}

	// Restore original timestamps from tar header after the file is closed,
	// since some platforms (e.g. Windows) cannot change timestamps on open files.
	if header.ModTime.IsZero() {
		return nil
	}
	accTime := header.AccessTime
	if accTime.IsZero() {
		accTime = header.ModTime
	}
	if err = os.Chtimes(dstFileName, accTime, header.ModTime); err != nil {
		return fmt.Errorf("restoring timestamps for %s: %w", dstFileName, err)
	}

	return nil
}

func tarWriteFile(dstFileName string, src io.Reader, srcFileInfo fs.FileInfo, maxFileSize int64) (err error) {
	if srcFileInfo.Mode().IsRegular() {
		if srcFileInfo.Size() < 0 || (maxFileSize > 0 && srcFileInfo.Size() > maxFileSize) {
			return fmt.Errorf("%w: %s is %d bytes, max %d", ErrFileTooLarge, dstFileName, srcFileInfo.Size(), maxFileSize)
		}
	}

	var dstFile *os.File
	dstFile, err = os.OpenFile(dstFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcFileInfo.Mode().Perm())
	if err != nil {
		return fmt.Errorf("opening destination file %s: %w", dstFileName, err)
	}
	defer func() {
		err = errors.Join(err, closeAndWrapErr(dstFile, "closing destination file %s: %w", dstFileName))
	}()

	var r io.Reader = src
	if srcFileInfo.Mode().IsRegular() {
		// Never copy more than the header claims so a truncated or lying
		// stream cannot fill the destination filesystem.
		r = io.LimitReader(src, srcFileInfo.Size())
	}

	n, err := io.Copy(dstFile, r)
	if err != nil {
		return fmt.Errorf("copying to destination file %s: %w", dstFileName, err)
	}

	if srcFileInfo.Mode().IsRegular() && n != srcFileInfo.Size() {
		return fmt.Errorf("written size check failed for %s: wrote %d, want %d", dstFileName, n, srcFileInfo.Size())
	}

	return nil
}

// boundedArchiveReader wraps the archive file with an io.LimitReader when a
// positive MaxArchiveSize is configured, otherwise returns the file directly.
// Exposed as a small helper so tests can exercise the exact cap that TarUnpack
// applies without having to stand up a whole tar archive and rely on the
// pre-read checkArchiveFile Stat pass (which would reject a grown file before
// this reader ever runs). A zero limit means "unbounded" and pre-existed the
// hardening.
func boundedArchiveReader(r io.Reader, maxArchiveSize int64) io.Reader {
	if maxArchiveSize <= 0 {
		return r
	}
	return io.LimitReader(r, maxArchiveSize)
}

func checkArchiveSize(path string, limits TarLimits) error {
	if limits.MaxArchiveSize <= 0 {
		return nil
	}
	f, err := openArchiveForRead(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	return checkArchiveFile(f, path, limits)
}

func checkArchiveFile(f *os.File, path string, limits TarLimits) error {
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat archive %s: %w", path, err)
	}
	// Reject non-regular files: FIFO / device / symlink / directory. On an
	// attacker-controlled NFS mount the archive path can be replaced between
	// TarPack finish and TarUnpack read; a FIFO would block os.Open forever
	// (or, opened O_NONBLOCK, report EOF and be accepted as an empty archive)
	// and a device / symlink can report Size()==0 which trivially bypasses
	// MaxArchiveSize while still streaming unbounded bytes to the reader.
	// This check runs regardless of MaxArchiveSize — configurations that only
	// set MaxFileSize / MaxFiles still need the descriptor to be a real file.
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: %s (mode=%s)", ErrArchiveInvalidType, path, fi.Mode())
	}
	if limits.MaxArchiveSize > 0 && fi.Size() > limits.MaxArchiveSize {
		return fmt.Errorf("%w: %s is %d bytes, max %d", ErrArchiveTooLarge, path, fi.Size(), limits.MaxArchiveSize)
	}
	return nil
}

// openArchiveForRead opens path for reading with defenses against
// attacker-controlled path replacement. Implementation is platform-specific
// (see tar_unix.go / tar_windows.go). Callers still validate mode/size via
// checkArchiveFile before trusting the returned descriptor.

func closeAndWrapErr(closer io.Closer, errFormat string, a ...any) error {
	if err := closer.Close(); err != nil {
		a = append(a, err)
		return fmt.Errorf(errFormat, a...)
	}
	return nil
}
