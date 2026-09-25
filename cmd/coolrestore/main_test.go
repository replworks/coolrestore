package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSkipChecksumStillRejectsInvalidArchiveStructure(t *testing.T) {
	base := t.TempDir()
	archivePath := filepath.Join(base, "invalid.tar.gz")
	if err := os.WriteFile(archivePath, []byte("not gzip"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	target := filepath.Join(base, "target")

	err := run([]string{
		"--source", archivePath,
		"--target", target,
		"--skip-checksum",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid gzip stream") {
		t.Fatalf("run() error = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target stat error = %v, target should not be created", err)
	}
}
