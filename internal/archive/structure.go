// Package archive validates archive structure before extraction.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

// ValidateTarGz verifies that path is a complete gzip-compressed tar stream.
// It does not extract entries or interpret their paths.
func ValidateTarGz(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("invalid gzip stream: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

	tarReader := tar.NewReader(gzipReader)
	for {
		_, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid tar stream: %w", err)
		}
		if _, err := io.Copy(io.Discard, tarReader); err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}
	}

	// Consume any remaining gzip bytes so the gzip checksum and trailer are
	// verified even for an empty tar stream.
	if _, err := io.Copy(io.Discard, gzipReader); err != nil {
		return fmt.Errorf("invalid gzip stream: %w", err)
	}
	return nil
}
