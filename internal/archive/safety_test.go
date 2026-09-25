package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTarGzSafetyAcceptsRegularFilesAndDirectories(t *testing.T) {
	path := writeSafetyArchive(t, []tar.Header{
		{Name: "safe", Typeflag: tar.TypeDir, Mode: 0o700},
		{Name: "safe/file.txt", Typeflag: tar.TypeReg, Mode: 0o600, Size: 4},
	}, [][]byte{nil, []byte("safe")})
	if err := ValidateTarGzSafety(path); err != nil {
		t.Fatalf("ValidateTarGzSafety() error = %v", err)
	}
}

func TestValidateTarGzSafetyReportsEveryUnsafeEntry(t *testing.T) {
	path := writeSafetyArchive(t, []tar.Header{
		{Name: "../traversal", Typeflag: tar.TypeReg, Mode: 0o600, Size: 1},
		{Name: "/absolute", Typeflag: tar.TypeReg, Mode: 0o600, Size: 1},
		{Name: "escape-link", Typeflag: tar.TypeSymlink, Linkname: "../../outside"},
		{Name: "hard-link", Typeflag: tar.TypeLink, Linkname: "safe"},
		{Name: "device", Typeflag: tar.TypeFifo},
	}, [][]byte{[]byte("x"), []byte("x"), nil, nil, nil})

	var safetyErr *SafetyError
	err := ValidateTarGzSafety(path)
	if !errors.As(err, &safetyErr) {
		t.Fatalf("ValidateTarGzSafety() error = %v, want SafetyError", err)
	}
	if len(safetyErr.Violations) != 5 {
		t.Fatalf("violations = %d, want 5: %v", len(safetyErr.Violations), safetyErr)
	}
	for _, want := range []string{"traversal", "absolute", "escape-link", "hard-link", "device"} {
		if !containsViolation(safetyErr.Violations, want) {
			t.Errorf("missing violation for %q: %v", want, safetyErr.Violations)
		}
	}
}

func TestValidateTarGzSafetyDoesNotTouchTarget(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	sentinel := filepath.Join(target, "sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	archivePath := writeSafetyArchive(t, []tar.Header{{Name: "../escape", Typeflag: tar.TypeReg, Size: 1}}, [][]byte{[]byte("x")})

	if err := ValidateTarGzSafety(archivePath); err == nil {
		t.Fatal("ValidateTarGzSafety() unexpectedly succeeded")
	}
	contents, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(contents) != "unchanged" {
		t.Fatalf("target sentinel = %q", contents)
	}
}

func containsViolation(violations []SafetyViolation, entry string) bool {
	for _, violation := range violations {
		if violation.Entry == entry || filepath.Base(violation.Entry) == entry {
			return true
		}
	}
	return false
}

func writeSafetyArchive(t *testing.T, headers []tar.Header, contents [][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "unsafe.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for index, header := range headers {
		if err := tarWriter.WriteHeader(&header); err != nil {
			t.Fatalf("WriteHeader() error = %v", err)
		}
		if index < len(contents) && len(contents[index]) > 0 {
			if _, err := tarWriter.Write(contents[index]); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
		}
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
