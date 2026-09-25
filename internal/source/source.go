// Package source acquires local and S3-compatible archive sources.
package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const credentialWrapperHint = "hint: use a protected environment wrapper; see examples/coolrestore-wrapper.sh or the release examples bundle"

// Artifact is an archive made available to later restore stages. Local
// artifacts point at the supplied file; remote artifacts point at a temporary
// downloaded file and own its cleanup function.
type Artifact struct {
	Path    string
	Source  string
	Size    int64
	Cleanup func() error
}

// CleanupIfNeeded removes temporary acquisition data when the artifact owns
// it. Local artifacts require no cleanup.
func (artifact Artifact) CleanupIfNeeded() error {
	if artifact.Cleanup == nil {
		return nil
	}
	return artifact.Cleanup()
}

// S3API is the small AWS SDK surface required for archive acquisition.
type S3API interface {
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// Acquire makes source available as a local archive file. It does not inspect
// archive contents; integrity and structural validation are later stages.
func Acquire(ctx context.Context, source string, skipChecksum bool) (Artifact, error) {
	if strings.HasPrefix(source, "s3://") {
		client, err := newS3Client(ctx)
		if err != nil {
			return Artifact{}, err
		}
		return acquireS3(ctx, source, client, skipChecksum)
	}
	return acquireLocal(source, skipChecksum)
}

func acquireLocal(path string, skipChecksum bool) (Artifact, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Artifact{}, fmt.Errorf("opening local archive %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Artifact{}, fmt.Errorf("local archive is not a regular file: %q", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return Artifact{}, fmt.Errorf("reading local archive %q: %w", path, err)
	}
	if !skipChecksum {
		if err := verifySize(file, info.Size()); err != nil {
			_ = file.Close()
			return Artifact{}, fmt.Errorf("verifying local archive %q: %w", path, err)
		}
	}
	if err := file.Close(); err != nil {
		return Artifact{}, fmt.Errorf("closing local archive %q: %w", path, err)
	}
	return Artifact{Path: path, Source: path, Size: info.Size()}, nil
}

func acquireS3(ctx context.Context, source string, client S3API, skipChecksum bool) (Artifact, error) {
	bucket, key, err := parseS3Source(source)
	if err != nil {
		return Artifact{}, err
	}

	tempDir, err := os.MkdirTemp("", "coolrestore-source-")
	if err != nil {
		return Artifact{}, fmt.Errorf("creating source staging directory: %w", err)
	}
	cleanup := func() error { return os.RemoveAll(tempDir) }

	archivePath := filepath.Join(tempDir, "archive.tar.gz")
	file, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = cleanup()
		return Artifact{}, fmt.Errorf("creating downloaded archive: %w", err)
	}

	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		_ = file.Close()
		_ = cleanup()
		return Artifact{}, fmt.Errorf("downloading %s: %w", source, err)
	}
	if output == nil || output.Body == nil {
		_ = file.Close()
		_ = cleanup()
		return Artifact{}, fmt.Errorf("downloading %s: response body is missing", source)
	}
	bytesCopied, copyErr := io.Copy(file, output.Body)
	bodyCloseErr := output.Body.Close()
	fileCloseErr := file.Close()
	if err := errors.Join(copyErr, bodyCloseErr, fileCloseErr); err != nil {
		_ = cleanup()
		return Artifact{}, fmt.Errorf("saving downloaded archive %s: %w", source, err)
	}
	if !skipChecksum {
		if output.ContentLength == nil {
			_ = cleanup()
			return Artifact{}, fmt.Errorf("verifying downloaded archive %s: object size is missing", source)
		}
		if err := compareSize(bytesCopied, *output.ContentLength); err != nil {
			_ = cleanup()
			return Artifact{}, fmt.Errorf("verifying downloaded archive %s: %w", source, err)
		}
	}

	return Artifact{Path: archivePath, Source: source, Size: bytesCopied, Cleanup: cleanup}, nil
}

func verifySize(reader io.Reader, expected int64) error {
	actual, err := io.Copy(io.Discard, reader)
	if err != nil {
		return err
	}
	return compareSize(actual, expected)
}

func compareSize(actual, expected int64) error {
	if actual != expected {
		return fmt.Errorf("size mismatch: expected %d bytes, read %d bytes", expected, actual)
	}
	return nil
}

func parseS3Source(source string) (string, string, error) {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "s3" || parsed.Host == "" || parsed.Path == "" || parsed.Path == "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("source must be an S3 URI in the form s3://bucket/object-key: %q", source)
	}
	return parsed.Host, strings.TrimPrefix(parsed.Path, "/"), nil
}

func newS3Client(ctx context.Context) (*s3.Client, error) {
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading AWS configuration: %w; %s", err, credentialWrapperHint)
	}
	if _, err := awsConfig.Credentials.Retrieve(ctx); err != nil {
		return nil, fmt.Errorf("loading S3 credentials: %w; %s", err, credentialWrapperHint)
	}

	forcePathStyle, err := forcePathStyleFromEnvironment()
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		if endpoint := os.Getenv("AWS_ENDPOINT_URL"); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
		options.UsePathStyle = forcePathStyle
	}), nil
}

func forcePathStyleFromEnvironment() (bool, error) {
	value := os.Getenv("AWS_S3_FORCE_PATH_STYLE")
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("AWS_S3_FORCE_PATH_STYLE must be boolean: %w", err)
	}
	return parsed, nil
}
