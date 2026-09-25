package restore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ApplyMerge applies only the file paths contained in a merge plan. It does
// not inspect the target to discover additional work; the planner owns that
// decision.
func ApplyMerge(stagingDir, targetDir string, plan Plan) error {
	if plan.Mode != ModeMerge {
		return fmt.Errorf("cannot apply %s plan as merge", plan.Mode)
	}
	if err := ensureTargetDirectory(targetDir); err != nil {
		return err
	}

	paths := append(append([]string{}, plan.Added...), plan.Overwritten...)
	for _, path := range paths {
		if err := applyMergeFile(stagingDir, targetDir, path); err != nil {
			return err
		}
	}
	return nil
}

func ensureTargetDirectory(targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return fmt.Errorf("creating target directory: %w", err)
	}
	info, err := os.Lstat(targetDir)
	if err != nil {
		return fmt.Errorf("checking target directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("target path must be a directory and not a symlink: %q", targetDir)
	}
	return nil
}

func applyMergeFile(stagingDir, targetDir, slashPath string) error {
	relative, err := safeRelativePath(slashPath)
	if err != nil {
		return err
	}
	sourcePath := filepath.Join(stagingDir, relative)
	sourceInfo, err := os.Lstat(sourcePath)
	if err != nil {
		return fmt.Errorf("reading staged file %q: %w", slashPath, err)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf("staged merge entry is not a regular file: %q", slashPath)
	}

	targetPath := filepath.Join(targetDir, relative)
	parent := filepath.Dir(targetPath)
	if err := ensureTargetParents(targetDir, parent); err != nil {
		return fmt.Errorf("preparing target parent for %q: %w", slashPath, err)
	}

	temporary, err := os.CreateTemp(parent, ".coolrestore-merge-")
	if err != nil {
		return fmt.Errorf("creating temporary target file for %q: %w", slashPath, err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	source, err := os.Open(sourcePath)
	if err != nil {
		_ = temporary.Close()
		return fmt.Errorf("opening staged file %q: %w", slashPath, err)
	}
	_, copyErr := io.Copy(temporary, source)
	closeSourceErr := source.Close()
	closeTemporaryErr := temporary.Close()
	if err := errors.Join(copyErr, closeSourceErr, closeTemporaryErr); err != nil {
		return fmt.Errorf("copying staged file %q: %w", slashPath, err)
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return fmt.Errorf("installing restored file %q: %w", slashPath, err)
	}
	removeTemporary = false
	return nil
}

func ensureTargetParents(targetDir, parent string) error {
	relative, err := filepath.Rel(targetDir, parent)
	if err != nil {
		return err
	}
	if relative == "." {
		return nil
	}
	current := targetDir
	for _, component := range strings.Split(relative, string(os.PathSeparator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		switch {
		case os.IsNotExist(err):
			if err := os.Mkdir(current, 0o700); err != nil {
				return err
			}
		case err != nil:
			return err
		case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
			return fmt.Errorf("target parent is not a real directory: %q", current)
		}
	}
	return nil
}

func safeRelativePath(slashPath string) (string, error) {
	if slashPath == "" {
		return "", fmt.Errorf("restore path is empty")
	}
	relative := filepath.FromSlash(slashPath)
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("restore path is absolute: %q", slashPath)
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("restore path escapes target: %q", slashPath)
	}
	return clean, nil
}
