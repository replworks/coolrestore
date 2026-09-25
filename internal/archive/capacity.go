package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SpaceChecker reports available bytes at a filesystem location.
type SpaceChecker interface {
	AvailableBytes(path string) (uint64, error)
}

// Capacity describes the space calculation used before extraction.
type Capacity struct {
	ArchiveBytes     uint64
	RegularFileBytes uint64
	RequiredBytes    uint64
	AvailableBytes   uint64
}

// CheckCapacity verifies that the working filesystem can hold the compressed
// archive and all regular-file bytes that extraction will materialize.
func CheckCapacity(archivePath, stagingBase string, checker SpaceChecker) (Capacity, error) {
	archiveInfo, err := os.Stat(archivePath)
	if err != nil {
		return Capacity{}, fmt.Errorf("stating archive for capacity check: %w", err)
	}
	if !archiveInfo.Mode().IsRegular() {
		return Capacity{}, fmt.Errorf("archive is not a regular file: %q", archivePath)
	}

	regularBytes, err := regularFileBytes(archivePath)
	if err != nil {
		return Capacity{}, fmt.Errorf("measuring archive contents: %w", err)
	}
	archiveBytes := uint64(archiveInfo.Size())
	required, err := addUint64(archiveBytes, regularBytes)
	if err != nil {
		return Capacity{}, fmt.Errorf("calculating staging capacity: %w", err)
	}

	if stagingBase == "" {
		stagingBase = os.TempDir()
	}
	available, err := checker.AvailableBytes(stagingBase)
	if err != nil {
		return Capacity{}, fmt.Errorf("checking available space at %q: %w", stagingBase, err)
	}
	capacity := Capacity{
		ArchiveBytes:     archiveBytes,
		RegularFileBytes: regularBytes,
		RequiredBytes:    required,
		AvailableBytes:   available,
	}
	if available < required {
		return capacity, fmt.Errorf("insufficient staging space: need %d bytes, have %d bytes", required, available)
	}
	return capacity, nil
}

func regularFileBytes(path string) (uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return 0, fmt.Errorf("invalid gzip stream: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	var total uint64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("invalid tar stream: %w", err)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}
		if header.Size < 0 {
			return 0, fmt.Errorf("negative regular-file size for %q", header.Name)
		}
		total, err = addUint64(total, uint64(header.Size))
		if err != nil {
			return 0, fmt.Errorf("regular-file size overflow at %q: %w", header.Name, err)
		}
	}
	return total, nil
}

func addUint64(left, right uint64) (uint64, error) {
	if ^uint64(0)-left < right {
		return 0, errors.New("size overflow")
	}
	return left + right, nil
}

// OSSpaceChecker checks the filesystem containing path. A not-yet-created
// staging base is resolved to its nearest existing ancestor.
type OSSpaceChecker struct{}

func (OSSpaceChecker) AvailableBytes(path string) (uint64, error) {
	existing, err := nearestExistingPath(path)
	if err != nil {
		return 0, err
	}
	return availableBytes(existing)
}

func nearestExistingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	candidate := filepath.Clean(absolute)
	for {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", fmt.Errorf("no existing ancestor for %q", path)
		}
		candidate = parent
	}
}
