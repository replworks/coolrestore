package lock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireRejectsSecondLockForSameTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "missing-target")

	first, err := Acquire(target)
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer func() { _ = first.Release() }()

	second, err := Acquire(target)
	if second != nil {
		_ = second.Release()
		t.Fatal("second Acquire() unexpectedly succeeded")
	}
	if !errors.Is(err, ErrAlreadyHeld) {
		t.Fatalf("second Acquire() error = %v, want ErrAlreadyHeld", err)
	}
}

func TestDifferentTargetsCanBeLockedIndependently(t *testing.T) {
	base := t.TempDir()
	first, err := Acquire(filepath.Join(base, "first"))
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer func() { _ = first.Release() }()

	second, err := Acquire(filepath.Join(base, "second"))
	if err != nil {
		t.Fatalf("second Acquire() error = %v", err)
	}
	defer func() { _ = second.Release() }()
}

func TestReleaseAllowsLaterAcquisition(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	first, err := Acquire(target)
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	second, err := Acquire(target)
	if err != nil {
		t.Fatalf("second Acquire() error = %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("second Release() error = %v", err)
	}
}

func TestEquivalentParentSymlinkPathsShareLock(t *testing.T) {
	base := t.TempDir()
	realParent := filepath.Join(base, "real")
	linkParent := filepath.Join(base, "link")
	if err := os.MkdirAll(realParent, 0o755); err != nil {
		t.Fatalf("ensureDirectory() error = %v", err)
	}
	if err := os.Symlink(realParent, linkParent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	first, err := Acquire(filepath.Join(realParent, "target"))
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer func() { _ = first.Release() }()

	second, err := Acquire(filepath.Join(linkParent, "target"))
	if second != nil {
		_ = second.Release()
		t.Fatal("equivalent symlink path unexpectedly acquired a second lock")
	}
	if !errors.Is(err, ErrAlreadyHeld) {
		t.Fatalf("second Acquire() error = %v, want ErrAlreadyHeld", err)
	}
}
