package restore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyReplaceInstallsExactStagedTree(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	target := filepath.Join(root, "target")
	writeReplaceFile(t, filepath.Join(staging, "new", "file.txt"), "archive")
	writeReplaceFile(t, filepath.Join(target, "old.txt"), "remove me")
	writeReplaceFile(t, filepath.Join(target, "keep.txt"), "remove me too")

	plan, err := PlanRestore(staging, target, ModeReplace)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyReplace(staging, target, plan); err != nil {
		t.Fatal(err)
	}

	if got := readReplaceFile(t, filepath.Join(target, "new", "file.txt")); got != "archive" {
		t.Fatalf("restored file = %q", got)
	}
	for _, path := range []string{"old.txt", "keep.txt"} {
		if _, err := os.Lstat(filepath.Join(target, path)); !os.IsNotExist(err) {
			t.Fatalf("target-only path %q still exists: %v", path, err)
		}
	}
	if _, err := os.Lstat(staging); !os.IsNotExist(err) {
		t.Fatalf("staging path after replace: %v", err)
	}
}

func TestApplyReplaceCreatesMissingTarget(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	target := filepath.Join(root, "new-target")
	writeReplaceFile(t, filepath.Join(staging, "file.txt"), "archive")

	if err := ApplyReplace(staging, target, Plan{Mode: ModeReplace, ResultPaths: []string{"file.txt"}}); err != nil {
		t.Fatal(err)
	}
	if got := readReplaceFile(t, filepath.Join(target, "file.txt")); got != "archive" {
		t.Fatalf("restored file = %q", got)
	}
}

func TestApplyReplaceRejectsNonReplacePlan(t *testing.T) {
	if err := ApplyReplace("/staging", "/target", Plan{Mode: ModeMerge}); err == nil {
		t.Fatal("ApplyReplace() accepted a merge plan")
	}
}

func writeReplaceFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readReplaceFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
