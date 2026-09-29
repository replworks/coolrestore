package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintVersion(t *testing.T) {
	var output bytes.Buffer
	printVersion(&output)
	if output.String() != "coolrestore "+version+"\n" {
		t.Fatalf("version output = %q", output.String())
	}
}

func TestRunHelpSucceedsWithoutRestoreArguments(t *testing.T) {
	for _, argument := range []string{"help", "--help"} {
		t.Run(argument, func(t *testing.T) {
			if err := run([]string{argument}); err != nil {
				t.Fatalf("run(%q) error = %v", argument, err)
			}
		})
	}
}

func TestRunDiagnoseHelpSucceedsWithoutSource(t *testing.T) {
	if err := run([]string{"diagnose", "--help"}); err != nil {
		t.Fatalf("run(diagnose --help) error = %v", err)
	}
}

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

func TestRunRejectsMalformedEnvFileBeforeAccessingArchiveOrTarget(t *testing.T) {
	base := t.TempDir()
	envPath := filepath.Join(base, "coolrestore.env")
	if err := os.WriteFile(envPath, []byte("export AWS_REGION=us-east-1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	target := filepath.Join(base, "target")
	err := run([]string{
		"--env-file", envPath,
		"--source", filepath.Join(base, "missing.tar.gz"),
		"--target", target,
	})
	if err == nil || !strings.Contains(err.Error(), "supported KEY=VALUE assignment") {
		t.Fatalf("run() error = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target stat error = %v, target should not be created", err)
	}
}

func TestRunRejectsEachUnsafeArchiveEntryWithoutChangingTarget(t *testing.T) {
	tests := []struct {
		name    string
		header  tar.Header
		content []byte
	}{
		{name: "path traversal", header: tar.Header{Name: "../escape", Typeflag: tar.TypeReg, Size: 1}, content: []byte("x")},
		{name: "absolute path", header: tar.Header{Name: "/escape", Typeflag: tar.TypeReg, Size: 1}, content: []byte("x")},
		{name: "symlink escape", header: tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../../outside"}},
		{name: "hard link", header: tar.Header{Name: "link", Typeflag: tar.TypeLink, Linkname: "file"}},
		{name: "special file", header: tar.Header{Name: "device", Typeflag: tar.TypeFifo}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			target := filepath.Join(base, "target")
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatalf("Mkdir() error = %v", err)
			}
			sentinel := filepath.Join(target, "sentinel")
			if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			archivePath := writeSingleEntryArchive(t, base, test.header, test.content)

			if err := run([]string{"--source", archivePath, "--target", target}); err == nil {
				t.Fatal("run() unexpectedly accepted unsafe archive")
			}
			contents, err := os.ReadFile(sentinel)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if string(contents) != "unchanged" {
				t.Fatalf("target sentinel = %q", contents)
			}
		})
	}
}

func writeSingleEntryArchive(t *testing.T, base string, header tar.Header, content []byte) string {
	t.Helper()
	path := filepath.Join(base, "archive.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&header); err != nil {
		t.Fatalf("WriteHeader() error = %v", err)
	}
	if len(content) > 0 {
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatalf("Write() error = %v", err)
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
