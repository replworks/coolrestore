package archive

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTarGzAcceptsValidArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "valid.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "file.txt", Mode: 0o600, Size: 5}); err != nil {
		t.Fatalf("WriteHeader() error = %v", err)
	}
	if _, err := tarWriter.Write([]byte("hello")); err != nil {
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

	if err := ValidateTarGz(path); err != nil {
		t.Fatalf("ValidateTarGz() error = %v", err)
	}
}

func TestValidateTarGzRejectsInvalidGzip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-gzip.tar.gz")
	if err := os.WriteFile(path, []byte("not gzip"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := ValidateTarGz(path); err == nil {
		t.Fatal("ValidateTarGz() unexpectedly accepted invalid gzip")
	}
}

func TestValidateTarGzRejectsInvalidTar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-tar.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	if _, err := gzipWriter.Write([]byte("not tar")); err != nil {
		t.Fatalf("gzip Write() error = %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close() error = %v", err)
	}

	if err := ValidateTarGz(path); err == nil {
		t.Fatal("ValidateTarGz() unexpectedly accepted invalid tar")
	}
}
