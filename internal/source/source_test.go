package source

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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
	client := &fakeS3Client{body: "archive from RustFS"}
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

func TestDiagnoseS3UsesHeadObjectWithoutDownloadingBody(t *testing.T) {
	client := &fakeS3Client{headSize: 42}
	diagnosis, err := diagnoseS3(context.Background(), "s3://bucket/path/archive.tar.gz", client)
	if err != nil {
		t.Fatalf("diagnoseS3() error = %v", err)
	}
	if diagnosis.Bucket != "bucket" || diagnosis.Key != "path/archive.tar.gz" || diagnosis.Size != 42 {
		t.Fatalf("unexpected diagnosis: %+v", diagnosis)
	}
	if client.headCalls != 1 || client.getCalls != 0 {
		t.Fatalf("S3 calls = head:%d get:%d, want head:1 get:0", client.headCalls, client.getCalls)
	}
}

func TestDiagnoseLocalChecksReadableRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	diagnosis, err := diagnoseLocal(path)
	if err != nil {
		t.Fatalf("diagnoseLocal() error = %v", err)
	}
	if diagnosis.Source != path || diagnosis.Size != int64(len("archive")) {
		t.Fatalf("unexpected diagnosis: %+v", diagnosis)
	}
}

func TestDiagnoseS3RejectsHeadObjectFailure(t *testing.T) {
	_, err := diagnoseS3(context.Background(), "s3://bucket/archive.tar.gz", &fakeS3Client{headErr: io.ErrUnexpectedEOF})
	if err == nil || !strings.Contains(err.Error(), "checking S3 object") {
		t.Fatalf("diagnoseS3() error = %v", err)
	}
}

func TestListS3FiltersAndSortsArchives(t *testing.T) {
	older := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	newer := older.Add(24 * time.Hour)
	client := &fakeS3Client{listOutputs: []*s3.ListObjectsV2Output{{
		Contents: []types.Object{
			{Key: aws.String("backups/older.tar.gz"), Size: aws.Int64(10), LastModified: &older},
			{Key: aws.String("backups/readme.txt"), Size: aws.Int64(20), LastModified: &newer},
			{Key: aws.String("backups/newer.tar.gz"), Size: aws.Int64(30), LastModified: &newer},
		},
	}}}

	objects, err := listS3(context.Background(), "s3://bucket/backups/", client)
	if err != nil {
		t.Fatalf("listS3() error = %v", err)
	}
	if len(objects) != 2 {
		t.Fatalf("listed objects = %d, want 2", len(objects))
	}
	if objects[0].Source != "s3://bucket/backups/newer.tar.gz" || objects[0].Key != "backups/newer.tar.gz" {
		t.Fatalf("newest object = %+v", objects[0])
	}
	if objects[1].Key != "backups/older.tar.gz" {
		t.Fatalf("second object = %+v", objects[1])
	}
}

func TestListS3RejectsEmptyArchivePrefix(t *testing.T) {
	client := &fakeS3Client{listOutputs: []*s3.ListObjectsV2Output{{}}}
	_, err := listS3(context.Background(), "s3://bucket/backups/", client)
	if err == nil || !strings.Contains(err.Error(), "no .tar.gz objects found") {
		t.Fatalf("listS3() error = %v", err)
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
	_, err := acquireS3(context.Background(), "s3://bucket/archive.tar.gz", &fakeS3Client{err: io.ErrUnexpectedEOF}, false)
	if err == nil || !strings.Contains(err.Error(), "downloading") {
		t.Fatalf("acquireS3() error = %v", err)
	}
}

func TestAcquireS3RejectsSizeMismatch(t *testing.T) {
	_, err := acquireS3(context.Background(), "s3://bucket/archive.tar.gz", &fakeS3Client{
		body:          "short",
		contentLength: 100,
	}, false)
	if err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("acquireS3() error = %v", err)
	}
}

func TestAcquireS3SkipChecksumAllowsSizeMismatch(t *testing.T) {
	client := &fakeS3Client{body: "short", contentLength: 100}
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

func TestCredentialHintDoesNotContainCredentialValues(t *testing.T) {
	if strings.Contains(credentialWrapperHint, "AWS_SECRET_ACCESS_KEY=") {
		t.Fatal("credential wrapper hint contains a credential assignment")
	}
	if !strings.Contains(credentialWrapperHint, "--env-file") {
		t.Fatalf("credential wrapper hint = %q", credentialWrapperHint)
	}
}

type fakeS3Client struct {
	body          string
	contentLength int64
	err           error
	headSize      int64
	headErr       error
	headCalls     int
	getCalls      int
	listOutputs   []*s3.ListObjectsV2Output
	listErr       error
	listCalls     int
}

func (client *fakeS3Client) GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	client.getCalls++
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

func (client *fakeS3Client) HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	client.headCalls++
	if client.headErr != nil {
		return nil, client.headErr
	}
	size := client.headSize
	return &s3.HeadObjectOutput{ContentLength: &size}, nil
}

func (client *fakeS3Client) ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	client.listCalls++
	if client.listErr != nil {
		return nil, client.listErr
	}
	if len(client.listOutputs) == 0 {
		return &s3.ListObjectsV2Output{}, nil
	}
	output := client.listOutputs[0]
	client.listOutputs = client.listOutputs[1:]
	return output, nil
}
