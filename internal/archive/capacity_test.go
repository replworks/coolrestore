package archive

import (
	"archive/tar"
	"compress/gzip"
	"path/filepath"
	"testing"

	"os"
)

func TestCheckCapacityAcceptsSufficientSpace(t *testing.T) {
	archivePath := writeCapacityArchive(t, 11)
	archiveInfo, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	regularBytes := uint64(11)
	required := uint64(archiveInfo.Size()) + regularBytes

	capacity, err := CheckCapacity(archivePath, filepath.Join(t.TempDir(), "missing-staging"), &fakeSpaceChecker{available: required})
	if err != nil {
		t.Fatalf("CheckCapacity() error = %v", err)
	}
	if capacity.RequiredBytes != required || capacity.RegularFileBytes != regularBytes {
		t.Fatalf("unexpected capacity: %+v", capacity)
	}
}

func TestCheckCapacityRejectsInsufficientSpaceBeforeExtraction(t *testing.T) {
	archivePath := writeCapacityArchive(t, 11)
	_, err := CheckCapacity(archivePath, t.TempDir(), &fakeSpaceChecker{available: 0})
	if err == nil {
		t.Fatal("CheckCapacity() unexpectedly succeeded")
	}
}

func TestCheckCapacityDoesNotUseRealDiskSpace(t *testing.T) {
	archivePath := writeCapacityArchive(t, 3)
	checker := &fakeSpaceChecker{available: 12345}
	if _, err := CheckCapacity(archivePath, "/path/that/need/not/exist", checker); err != nil {
		t.Fatalf("CheckCapacity() error = %v", err)
	}
	if checker.path != "/path/that/need/not/exist" {
		t.Fatalf("checker path = %q", checker.path)
	}
}

func writeCapacityArchive(t *testing.T, size int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "file", Mode: 0o600, Size: size}); err != nil {
		t.Fatalf("WriteHeader() error = %v", err)
	}
	if _, err := tarWriter.Write(make([]byte, size)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tar Close() error = %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close() error = %v", err)
	}
	return path
}

type fakeSpaceChecker struct {
	available uint64
	path      string
}

func (checker *fakeSpaceChecker) AvailableBytes(path string) (uint64, error) {
	checker.path = path
	return checker.available, nil
}
