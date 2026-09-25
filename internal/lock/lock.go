// Package lock provides process-visible exclusivity for restore targets.
package lock

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ErrAlreadyHeld indicates that another restore invocation owns the target
// lock. The lock is intentionally not made stale automatically.
var ErrAlreadyHeld = errors.New("restore already in progress for target")

// Handle owns one target lock until Release is called.
type Handle struct {
	path       string
	file       *os.File
	release    sync.Once
	releaseErr error
}

// Acquire creates an exclusive lock for target. The lock name is derived from
// the target's resolved absolute path, so equivalent paths share a lock even
// when the target does not yet exist.
func Acquire(target string) (*Handle, error) {
	canonical, err := canonicalTarget(target)
	if err != nil {
		return nil, fmt.Errorf("normalizing target for lock: %w", err)
	}

	lockPath := lockPath(canonical)
	file, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: %s", ErrAlreadyHeld, canonical)
		}
		return nil, fmt.Errorf("creating target lock: %w", err)
	}
	if _, err := file.WriteString(canonical + "\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("writing target lock: %w", err)
	}

	return &Handle{path: lockPath, file: file}, nil
}

// Release closes and removes the lock. It is safe to call more than once.
func (handle *Handle) Release() error {
	if handle == nil {
		return nil
	}
	handle.release.Do(func() {
		closeErr := handle.file.Close()
		removeErr := os.Remove(handle.path)
		handle.releaseErr = errors.Join(closeErr, removeErr)
	})
	return handle.releaseErr
}

func lockPath(canonical string) string {
	digest := sha256.Sum256([]byte(canonical))
	return filepath.Join(os.TempDir(), fmt.Sprintf("coolrestore-lock-%x", digest[:]))
}

func canonicalTarget(target string) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", errors.New("target is empty")
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(absolute)

	// Resolve the existing ancestor and append any missing suffix. This keeps
	// lock identity stable for a target that has not been created yet while
	// still resolving symlinked parent directories.
	missing := make([]string, 0)
	candidate := clean
	for {
		_, statErr := os.Lstat(candidate)
		if statErr == nil {
			resolved, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				return "", err
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}

		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", fmt.Errorf("cannot resolve target ancestor: %s", target)
		}
		missing = append(missing, filepath.Base(candidate))
		candidate = parent
	}
}
