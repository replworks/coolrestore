package restore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyMergeAddsOverwritesAndPreservesTargetOnlyFiles(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	target := filepath.Join(root, "target")
	writeMergeFile(t, filepath.Join(staging, "new", "file.txt"), "from archive")
	writeMergeFile(t, filepath.Join(staging, "shared.txt"), "replacement")
	writeMergeFile(t, filepath.Join(target, "shared.txt"), "original")
	writeMergeFile(t, filepath.Join(target, "keep.txt"), "keep")

	plan, err := PlanRestore(staging, target, ModeMerge)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyMerge(staging, target, plan); err != nil {
		t.Fatal(err)
	}

	if got := readMergeFile(t, filepath.Join(target, "new", "file.txt")); got != "from archive" {
		t.Fatalf("added file = %q", got)
	}
	if got := readMergeFile(t, filepath.Join(target, "shared.txt")); got != "replacement" {
		t.Fatalf("overwritten file = %q", got)
	}
	if got := readMergeFile(t, filepath.Join(target, "keep.txt")); got != "keep" {
		t.Fatalf("target-only file = %q", got)
	}
}

func TestApplyMergeCreatesMissingTarget(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	target := filepath.Join(root, "new-target")
	writeMergeFile(t, filepath.Join(staging, "file.txt"), "content")

	plan, err := PlanRestore(staging, target, ModeMerge)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyMerge(staging, target, plan); err != nil {
		t.Fatal(err)
	}
	if got := readMergeFile(t, filepath.Join(target, "file.txt")); got != "content" {
		t.Fatalf("created target file = %q", got)
	}
}

func TestApplyMergeRejectsSymlinkedTargetParent(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	target := filepath.Join(root, "target")
	outside := filepath.Join(root, "outside")
	writeMergeFile(t, filepath.Join(staging, "file.txt"), "content")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(target, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := ApplyMerge(staging, target, Plan{Mode: ModeMerge, Added: []string{"linked/file.txt"}})
	if err == nil {
		t.Fatal("ApplyMerge() succeeded through a symlinked target parent")
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file was created: %v", err)
	}
}

func writeMergeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readMergeFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
