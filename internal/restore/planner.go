package restore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// PlanRestore computes the paths represented by staged content without
// creating, modifying, or deleting anything in the target.
func PlanRestore(stagingDir, targetDir string, mode Mode) (Plan, error) {
	if mode != ModeMerge && mode != ModeReplace {
		return Plan{}, fmt.Errorf("unsupported restore mode %q", mode)
	}
	if err := validateDirectory(stagingDir, "staging"); err != nil {
		return Plan{}, err
	}
	if err := validateTarget(targetDir); err != nil {
		return Plan{}, err
	}

	staged, regularFiles, err := collectPaths(stagingDir)
	if err != nil {
		return Plan{}, fmt.Errorf("collect staged content: %w", err)
	}

	plan := Plan{Mode: mode, RegularFiles: regularFiles}
	if mode == ModeReplace {
		plan.ResultPaths = staged
		targetPaths, _, err := collectPathsIfPresent(targetDir)
		if err != nil {
			return Plan{}, fmt.Errorf("collect target content: %w", err)
		}
		plan.Removed = difference(targetPaths, staged)
		return plan, nil
	}

	for _, path := range staged {
		fullPath := filepath.Join(stagingDir, filepath.FromSlash(path))
		info, err := os.Lstat(fullPath)
		if err != nil {
			return Plan{}, fmt.Errorf("inspect staged path %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		targetPath := filepath.Join(targetDir, filepath.FromSlash(path))
		_, err = os.Lstat(targetPath)
		switch {
		case err == nil:
			plan.Overwritten = append(plan.Overwritten, path)
		case os.IsNotExist(err):
			plan.Added = append(plan.Added, path)
		default:
			return Plan{}, fmt.Errorf("inspect target path %q: %w", path, err)
		}
	}
	return plan, nil
}

func validateDirectory(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%s directory: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s path must be a directory: %q", label, path)
	}
	return nil
}

func validateTarget(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("target directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("target path must be a directory and not a symlink: %q", path)
	}
	return nil
}

func collectPaths(root string) ([]string, int, error) {
	paths := make([]string, 0)
	regularFiles := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link is not allowed in restore content: %q", filepath.ToSlash(rel))
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported restore entry type at %q", filepath.ToSlash(rel))
		}
		paths = append(paths, filepath.ToSlash(rel))
		if entry.Type().IsRegular() {
			regularFiles++
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(paths)
	return paths, regularFiles, nil
}

func collectPathsIfPresent(root string) ([]string, int, error) {
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil, 0, nil
	} else if err != nil {
		return nil, 0, err
	}
	return collectPaths(root)
}

func difference(left, right []string) []string {
	known := make(map[string]struct{}, len(right))
	for _, path := range right {
		known[path] = struct{}{}
	}
	result := make([]string, 0)
	for _, path := range left {
		if _, ok := known[path]; !ok {
			result = append(result, path)
		}
	}
	sort.Strings(result)
	return result
}
