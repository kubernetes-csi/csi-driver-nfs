package tar

import (
	"bytes"
	"fmt"
	"maps"
	"math"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/kubernetes-csi/csi-driver-nfs/pkg/nfs"
	"golang.org/x/mod/sumdb/dirhash"
)

const (
	code packApi = '0'
	cli  packApi = '1'
)

type packApi byte

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
		ops := make([]packApi, opNum)
		for opIdx := 0; opIdx < opNum; opIdx++ {
			ops[opIdx] = packApi(binStr[opIdx])
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
	ops ...packApi,
) {
	baseName := path.Base(inputDirPath)

	for i := 0; i < len(ops); i++ {
		packing := i%2 == 0

		srcPath := inputDirPath
		if i > 0 {
			prevSuffix := string(ops[:i])
			srcPath = path.Join(outputDirPath, baseName+prevSuffix)
			if !packing {
				srcPath += archiveFileExt
			}
		}

		suffix := string(ops[:i+1])
		dstPath := path.Join(outputDirPath, baseName+suffix)
		if packing {
			dstPath += archiveFileExt
		}

		if _, ok := results[suffix]; ok {
			continue
		}

		switch {
		case packing && ops[i] == code:
			// packing from code
			if err := nfs.TarPack(srcPath, dstPath, true); err != nil {
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
			if err := nfs.TarUnpack(srcPath, dstPath, true); err != nil {
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
	if err := os.MkdirAll(path.Join(inputPath, "empty_dir"), 0755); err != nil {
		t.Fatalf("generating empty directory: %v", err)
	}

	// deep empty directories
	deepEmptyDirPath := path.Join(inputPath, "deep_empty_dir", strings.Repeat("/0/1/2", 20))
	if err := os.MkdirAll(deepEmptyDirPath, 0755); err != nil {
		t.Fatalf("generating deep empty directory '%s': %v", deepEmptyDirPath, err)
	}

	// empty file
	f, err := os.Create(path.Join(inputPath, "empty_file"))
	if err != nil {
		t.Fatalf("generating empty file: %v", err)
	}
	f.Close()

	// big (100MB) file
	bigFilePath := path.Join(inputPath, "big_file")
	for i := byte(0); i < 100; i++ {
		// write 1MB
		err := os.WriteFile(bigFilePath, bytes.Repeat([]byte{i}, 1024*1024), 0755)
		if err != nil {
			t.Fatalf("generating empty file: %v", err)
		}
	}
}
