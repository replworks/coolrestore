//go:build !windows

package restore

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func requireSameFilesystem(first, second string) error {
	firstInfo, err := os.Stat(first)
	if err != nil {
		return err
	}
	secondPath, err := existingAncestor(second)
	if err != nil {
		return err
	}
	secondInfo, err := os.Stat(secondPath)
	if err != nil {
		return err
	}
	firstStat, ok := firstInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot inspect filesystem for %q", first)
	}
	secondStat, ok := secondInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot inspect filesystem for %q", secondPath)
	}
	if firstStat.Dev != secondStat.Dev {
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
