package source

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestRustFSE2E(t *testing.T) {
	if os.Getenv("COOLRESTORE_SKIP_RUSTFS_E2E") == "1" {
		t.Skip("RustFS E2E explicitly disabled")
	}

	containerName := fmt.Sprintf("coolrestore-rustfs-test-%d", time.Now().UnixNano())
	start := exec.Command("docker", "run", "--rm", "-d", "--name", containerName,
		"-p", "127.0.0.1::9000",
		"-e", "RUSTFS_ACCESS_KEY=coolrestore-test-access",
		"-e", "RUSTFS_SECRET_KEY=coolrestore-test-secret",
		"-e", "RUSTFS_ADDRESS=:9000",
		"-e", "RUSTFS_CONSOLE_ENABLE=false",
		"rustfs/rustfs:latest", "/data")
	output, err := start.CombinedOutput()
	if err != nil {
		t.Fatalf("starting RustFS container: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	})

	endpoint := waitForRustFSEndpoint(t, containerName)
	t.Setenv("AWS_ACCESS_KEY_ID", "coolrestore-test-access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "coolrestore-test-secret")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ENDPOINT_URL", endpoint)
	t.Setenv("AWS_S3_FORCE_PATH_STYLE", "true")

	ctx := context.Background()
	client, err := newS3Client(ctx)
	if err != nil {
		t.Fatalf("newS3Client() error = %v", err)
	}

	bucket := "coolrestore-e2e"
	key := "backups/archive.tar.gz"
	want := []byte("real RustFS object")
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(string(want)),
	}); err != nil {
		t.Fatalf("PutObject() error = %v", err)
	}

	artifact, err := Acquire(ctx, "s3://"+bucket+"/"+key)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	t.Cleanup(func() { _ = artifact.Cleanup() })
	got, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("downloaded contents = %q, want %q", got, want)
	}
}

func waitForRustFSEndpoint(t *testing.T, containerName string) string {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		output, err := exec.Command("docker", "port", containerName, "9000/tcp").Output()
		if err == nil {
			address := strings.TrimSpace(string(output))
			_, port, splitErr := net.SplitHostPort(address)
			if splitErr == nil {
				endpoint := "http://127.0.0.1:" + port
				response, healthErr := http.Get(endpoint + "/health")
				if healthErr == nil {
					_, _ = io.Copy(io.Discard, response.Body)
					_ = response.Body.Close()
					if response.StatusCode == http.StatusOK {
						return endpoint
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("RustFS container did not become healthy")
	return ""
}
