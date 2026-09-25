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

// SafetyViolation describes one unsafe archive entry.
type SafetyViolation struct {
	Entry  string
	Reason string
}

// SafetyError contains every unsafe entry found during one complete scan.
type SafetyError struct {
	Violations []SafetyViolation
}

func (err *SafetyError) Error() string {
	if len(err.Violations) == 0 {
		return "archive content safety validation failed"
	}
	var builder strings.Builder
	builder.WriteString("unsafe archive content:")
	for _, violation := range err.Violations {
		fmt.Fprintf(&builder, "\n- %q: %s", violation.Entry, violation.Reason)
	}
	return builder.String()
}

// ValidateTarGzSafety scans every tar entry and rejects content that could
// escape, link through, or otherwise misuse the restore target. It performs
// no writes and reports all violations found in the archive.
func ValidateTarGzSafety(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening archive for safety validation: %w", err)
	}
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("opening archive gzip stream for safety validation: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()
	tarReader := tar.NewReader(gzipReader)
	violations := make([]SafetyViolation, 0)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive entry for safety validation: %w", err)
		}
		violations = append(violations, validateHeader(*header)...)
		if _, err := io.Copy(io.Discard, tarReader); err != nil {
			return fmt.Errorf("reading archive entry %q for safety validation: %w", header.Name, err)
		}
	}
	if _, err := io.Copy(io.Discard, gzipReader); err != nil {
		return fmt.Errorf("finishing archive safety validation: %w", err)
	}
	if len(violations) > 0 {
		return &SafetyError{Violations: violations}
	}
	return nil
}

func validateHeader(header tar.Header) []SafetyViolation {
	violations := make([]SafetyViolation, 0, 2)
	name := filepath.FromSlash(header.Name)
	if name == "" {
		return []SafetyViolation{{Entry: header.Name, Reason: "entry path is empty"}}
	}
	if filepath.IsAbs(name) {
		violations = append(violations, SafetyViolation{Entry: header.Name, Reason: "absolute path is not allowed"})
	} else {
		cleanName := filepath.Clean(name)
		if cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) {
			violations = append(violations, SafetyViolation{Entry: header.Name, Reason: "path traversal escapes the restore target"})
		}
	}

	switch header.Typeflag {
	case tar.TypeReg, tar.TypeDir:
	case tar.TypeSymlink:
		reason := "symbolic links are not allowed"
		if symlinkEscapes(header.Name, header.Linkname) {
			reason = "symbolic link resolves outside the restore target"
		}
		violations = append(violations, SafetyViolation{Entry: header.Name, Reason: reason})
	case tar.TypeLink:
		violations = append(violations, SafetyViolation{Entry: header.Name, Reason: "hard links are not allowed"})
	default:
		violations = append(violations, SafetyViolation{Entry: header.Name, Reason: fmt.Sprintf("file type %d is not allowed", header.Typeflag)})
	}
	return violations
}

func symlinkEscapes(name, linkname string) bool {
	link := filepath.FromSlash(linkname)
	if filepath.IsAbs(link) {
		return true
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(filepath.FromSlash(name)), link))
	return resolved == ".." || strings.HasPrefix(resolved, ".."+string(os.PathSeparator))
}
