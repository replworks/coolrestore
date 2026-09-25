//go:build windows

package restore

import (
	"fmt"
	"os"
	"path/filepath"
)

func requireSameFilesystem(first, second string) error {
	firstVolume := filepath.VolumeName(first)
	secondPath, err := existingAncestor(second)
	if err != nil {
		return err
	}
	if firstVolume == "" || firstVolume != filepath.VolumeName(secondPath) {
		return fmt.Errorf("staging and target are on different filesystems")
	}
	return nil
}

func existingAncestor(path string) (string, error) {
	current := path
	for {
		if _, err := os.Lstat(current); err == nil {
			return current, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %q", path)
		}
		current = parent
	}
}
