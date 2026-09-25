package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Staging owns one fresh directory created for a single restore invocation.
type Staging struct {
	Dir string
}

// Cleanup removes the staging directory owned by this value.
func (staging Staging) Cleanup() error {
	if staging.Dir == "" {
		return nil
	}
	return os.RemoveAll(staging.Dir)
}

// StageTarGz creates an isolated staging child and extracts the archive into
// it. The target is used only to reject overlapping staging paths; it is
// never opened, created, or written.
func StageTarGz(archivePath, target, stagingBase string) (Staging, error) {
	stagingDir, err := createStaging(target, stagingBase)
	if err != nil {
		return Staging{}, err
	}
	staging := Staging{Dir: stagingDir}
	if err := extractTarGz(archivePath, stagingDir); err != nil {
		_ = staging.Cleanup()
		return Staging{}, err
	}
	return staging, nil
}

func createStaging(target, stagingBase string) (string, error) {
	if stagingBase == "" {
		stagingBase = os.TempDir()
	}
	resolvedTarget, err := resolvePathForComparison(target)
	if err != nil {
		return "", fmt.Errorf("resolving target for staging: %w", err)
	}
	resolvedBase, err := resolvePathForComparison(stagingBase)
	if err != nil {
		return "", fmt.Errorf("resolving staging base: %w", err)
	}
	// A base inside the target would require creating staging content under
	// the target before the overlap could be checked, so reject it first.
	if sameOrDescendant(resolvedBase, resolvedTarget) {
		return "", fmt.Errorf("staging base %q is the target or is inside target %q", stagingBase, target)
	}

	if err := os.MkdirAll(stagingBase, 0o700); err != nil {
		return "", fmt.Errorf("creating staging base: %w", err)
	}
	stagingDir, err := os.MkdirTemp(stagingBase, "coolrestore-")
	if err != nil {
		return "", fmt.Errorf("creating staging directory: %w", err)
	}

	resolvedStaging, err := resolvePathForComparison(stagingDir)
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		return "", fmt.Errorf("resolving staging directory: %w", err)
	}
	if sameOrDescendant(resolvedStaging, resolvedTarget) || sameOrDescendant(resolvedTarget, resolvedStaging) {
		_ = os.RemoveAll(stagingDir)
		return "", fmt.Errorf("staging directory %q overlaps target %q", stagingDir, target)
	}
	return stagingDir, nil
}

func extractTarGz(archivePath, stagingDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive for staging: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("opening archive gzip stream for staging: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive entry for staging: %w", err)
		}
		targetPath, err := safeStagingPath(stagingDir, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o700); err != nil {
				return fmt.Errorf("creating staged directory %q: %w", header.Name, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
				return fmt.Errorf("creating staged parent for %q: %w", header.Name, err)
			}
			output, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return fmt.Errorf("creating staged file %q: %w", header.Name, err)
			}
			_, copyErr := io.Copy(output, tarReader)
			closeErr := output.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				return fmt.Errorf("writing staged file %q: %w", header.Name, err)
			}
		default:
			return fmt.Errorf("unsupported archive entry type %d at %q", header.Typeflag, header.Name)
		}
	}

	if _, err := io.Copy(io.Discard, gzipReader); err != nil {
		return fmt.Errorf("finishing staged gzip stream: %w", err)
	}
	return nil
}

func safeStagingPath(stagingDir, name string) (string, error) {
	if name == "" {
		return "", errors.New("archive entry has an empty path")
	}
	name = filepath.FromSlash(name)
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("archive entry %q is an absolute path", name)
	}
	cleanName := filepath.Clean(name)
	if cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry %q escapes staging directory", name)
	}
	if cleanName == "." {
		return filepath.Clean(stagingDir), nil
	}
	return filepath.Join(stagingDir, cleanName), nil
}

func resolvePathForComparison(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(absolute)
	missing := make([]string, 0)
	candidate := clean
	for {
		if _, err := os.Lstat(candidate); err == nil {
			resolved, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				return "", err
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", fmt.Errorf("no existing ancestor for %q", path)
		}
		missing = append(missing, filepath.Base(candidate))
		candidate = parent
	}
}

func sameOrDescendant(path, parent string) bool {
	relative, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)))
}
