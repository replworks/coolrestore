package restore

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPlanRestoreMergeListsOnlyStagedFileChanges(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Join(staging, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(staging, "new.txt"), "new")
	writeFile(t, filepath.Join(staging, "dir", "shared.txt"), "replacement")
	writeFile(t, filepath.Join(target, "dir", "shared.txt"), "original")
	writeFile(t, filepath.Join(target, "target-only.txt"), "keep")

	plan, err := PlanRestore(staging, target, ModeMerge)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Added, []string{"new.txt"}) {
		t.Fatalf("added = %#v", plan.Added)
	}
	if !reflect.DeepEqual(plan.Overwritten, []string{"dir/shared.txt"}) {
		t.Fatalf("overwritten = %#v", plan.Overwritten)
	}
	if plan.RegularFiles != 2 {
		t.Fatalf("regular files = %d, want 2", plan.RegularFiles)
	}
	if got := readFile(t, filepath.Join(target, "dir", "shared.txt")); got != "original" {
		t.Fatalf("target changed during planning: %q", got)
	}
}

func TestPlanRestoreReplaceListsResultAndRemovedPaths(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Join(staging, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(staging, "dir", "file.txt"), "content")
	writeFile(t, filepath.Join(target, "old.txt"), "old")

	plan, err := PlanRestore(staging, target, ModeReplace)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.ResultPaths, []string{"dir", "dir/file.txt"}) {
		t.Fatalf("result paths = %#v", plan.ResultPaths)
	}
	if !reflect.DeepEqual(plan.Removed, []string{"old.txt"}) {
		t.Fatalf("removed = %#v", plan.Removed)
	}
	if _, err := os.Stat(filepath.Join(target, "old.txt")); err != nil {
		t.Fatalf("target changed during planning: %v", err)
	}
}

func TestPlanRestoreDoesNotCreateMissingTarget(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	target := filepath.Join(root, "missing-target")
	if err := os.Mkdir(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(staging, "file.txt"), "content")

	if _, err := PlanRestore(staging, target, ModeMerge); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target existence after planning: %v", err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
