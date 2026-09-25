package archive

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestStageTarGzKeepsTargetUntouched(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	sentinel := filepath.Join(target, "existing.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	archivePath := writeStageArchive(t, base, "restored.txt", []byte("restored"))

	staging, err := StageTarGz(archivePath, target, filepath.Join(base, "staging-base"))
	if err != nil {
		t.Fatalf("StageTarGz() error = %v", err)
	}
	defer staging.Cleanup()

	contents, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(contents) != "keep" {
		t.Fatalf("target sentinel = %q", contents)
	}
	staged, err := os.ReadFile(filepath.Join(staging.Dir, "restored.txt"))
	if err != nil {
		t.Fatalf("staged ReadFile() error = %v", err)
	}
	if string(staged) != "restored" {
		t.Fatalf("staged contents = %q", staged)
	}
}

func TestStageTarGzCleansStagingAfterExtractionFailure(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	stagingBase := filepath.Join(base, "staging-base")
	archivePath := writeStageArchive(t, base, "../escape.txt", []byte("escape"))

	if _, err := StageTarGz(archivePath, target, stagingBase); err == nil {
		t.Fatal("StageTarGz() unexpectedly succeeded")
	}
	entries, err := os.ReadDir(stagingBase)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging artifacts remain after failure: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(base, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("escape target stat error = %v", err)
	}
}

func TestStageTarGzRejectsStagingInsideTarget(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	_, err := StageTarGz("/does/not/matter.tar.gz", target, filepath.Join(target, "staging"))
	if err == nil {
		t.Fatal("StageTarGz() unexpectedly accepted staging inside target")
	}
}

func TestStageTarGzAllowsDefaultTempBaseBesideTarget(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	archivePath := writeStageArchive(t, base, "file.txt", []byte("content"))

	staging, err := StageTarGz(archivePath, target, "")
	if err != nil {
		t.Fatalf("StageTarGz() error = %v", err)
	}
	defer staging.Cleanup()
	if sameOrDescendant(staging.Dir, target) || sameOrDescendant(target, staging.Dir) {
		t.Fatalf("staging %q overlaps target %q", staging.Dir, target)
	}
}

func writeStageArchive(t *testing.T, base, name string, contents []byte) string {
	t.Helper()
	path := filepath.Join(base, "archive.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(contents))}); err != nil {
		t.Fatalf("WriteHeader() error = %v", err)
	}
	if _, err := tarWriter.Write(contents); err != nil {
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
