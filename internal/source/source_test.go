package source

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestAcquireLocalUsesExistingFileWithoutRemoteAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	artifact, err := acquireLocal(path, false)
	if err != nil {
		t.Fatalf("acquireLocal() error = %v", err)
	}
	if artifact.Path != path || artifact.Source != path || artifact.Size != int64(len("archive")) || artifact.Cleanup != nil {
		t.Fatalf("unexpected local artifact: %+v", artifact)
	}
}

func TestAcquireLocalRejectsUnreadableOrMissingSource(t *testing.T) {
	_, err := acquireLocal(filepath.Join(t.TempDir(), "missing.tar.gz"), false)
	if err == nil {
		t.Fatal("acquireLocal() unexpectedly succeeded")
	}
}

func TestAcquireS3DownloadsThroughClient(t *testing.T) {
	client := fakeS3Client{body: "archive from RustFS"}
	artifact, err := acquireS3(context.Background(), "s3://bucket/path/archive.tar.gz", client, false)
	if err != nil {
		t.Fatalf("acquireS3() error = %v", err)
	}
	defer func() { _ = artifact.Cleanup() }()

	contents, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(contents) != client.body {
		t.Fatalf("downloaded contents = %q, want %q", contents, client.body)
	}
}

func TestParseS3Source(t *testing.T) {
	tests := []struct {
		input  string
		bucket string
		key    string
	}{
		{input: "s3://bucket/archive.tar.gz", bucket: "bucket", key: "archive.tar.gz"},
		{input: "s3://bucket/path/archive.tar.gz", bucket: "bucket", key: "path/archive.tar.gz"},
	}
	for _, test := range tests {
		bucket, key, err := parseS3Source(test.input)
		if err != nil {
			t.Fatalf("parseS3Source(%q) error = %v", test.input, err)
		}
		if bucket != test.bucket || key != test.key {
			t.Errorf("parseS3Source(%q) = %q, %q", test.input, bucket, key)
		}
	}
}

func TestAcquireS3RejectsRequestFailure(t *testing.T) {
	_, err := acquireS3(context.Background(), "s3://bucket/archive.tar.gz", fakeS3Client{err: io.ErrUnexpectedEOF}, false)
	if err == nil || !strings.Contains(err.Error(), "downloading") {
		t.Fatalf("acquireS3() error = %v", err)
	}
}

func TestAcquireS3RejectsSizeMismatch(t *testing.T) {
	_, err := acquireS3(context.Background(), "s3://bucket/archive.tar.gz", fakeS3Client{
		body:          "short",
		contentLength: 100,
	}, false)
	if err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("acquireS3() error = %v", err)
	}
}

func TestAcquireS3SkipChecksumAllowsSizeMismatch(t *testing.T) {
	client := fakeS3Client{body: "short", contentLength: 100}
	artifact, err := acquireS3(context.Background(), "s3://bucket/archive.tar.gz", client, true)
	if err != nil {
		t.Fatalf("acquireS3() error = %v", err)
	}
	defer func() { _ = artifact.Cleanup() }()
}

func TestVerifySizeRejectsTruncatedReader(t *testing.T) {
	if err := verifySize(strings.NewReader("short"), 100); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("verifySize() error = %v", err)
	}
}

func TestCredentialWrapperHintDoesNotContainCredentialValues(t *testing.T) {
	if strings.Contains(credentialWrapperHint, "AWS_SECRET_ACCESS_KEY=") {
		t.Fatal("credential wrapper hint contains a credential assignment")
	}
	if !strings.Contains(credentialWrapperHint, "protected environment wrapper") {
		t.Fatalf("credential wrapper hint = %q", credentialWrapperHint)
	}
}

type fakeS3Client struct {
	body          string
	contentLength int64
	err           error
}

func (client fakeS3Client) GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if client.err != nil {
		return nil, client.err
	}
	contentLength := client.contentLength
	if contentLength == 0 {
		contentLength = int64(len(client.body))
	}
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(strings.NewReader(client.body)),
		ContentLength: &contentLength,
	}, nil
}
