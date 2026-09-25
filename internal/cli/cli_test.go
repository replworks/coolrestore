package cli

import "testing"

func TestParseAcceptsValidMergeRequestWithoutAccessingResources(t *testing.T) {
	invocation, err := Parse([]string{
		"--source", "/does/not/exist/backup.tar.gz",
		"--target", "/tmp/coolrestore-target-does-not-exist",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if invocation.Mode != "merge" || invocation.Confirm {
		t.Fatalf("unexpected invocation: %+v", invocation)
	}
}

func TestParseAcceptsValidS3Request(t *testing.T) {
	_, err := Parse([]string{
		"--source", "s3://backup-bucket/path/backup.tar.gz",
		"--target", "/tmp/coolrestore-target-does-not-exist",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing source", args: []string{"--target", "/tmp/target"}},
		{name: "missing target", args: []string{"--source", "/tmp/archive.tar.gz"}},
		{name: "relative source", args: []string{"--source", "archive.tar.gz", "--target", "/tmp/target"}},
		{name: "relative target", args: []string{"--source", "/tmp/archive.tar.gz", "--target", "target"}},
		{name: "invalid mode", args: []string{"--source", "/tmp/archive.tar.gz", "--target", "/tmp/target", "--mode", "unknown"}},
		{name: "replace without confirmation", args: []string{"--source", "/tmp/archive.tar.gz", "--target", "/tmp/target", "--mode", "replace"}},
		{name: "relative staging", args: []string{"--source", "/tmp/archive.tar.gz", "--target", "/tmp/target", "--staging", "stage"}},
		{name: "root target", args: []string{"--source", "/tmp/archive.tar.gz", "--target", "/"}},
		{name: "malformed S3 URI", args: []string{"--source", "s3://bucket", "--target", "/tmp/target"}},
		{name: "S3 query", args: []string{"--source", "s3://bucket/key?x=1", "--target", "/tmp/target"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse(test.args); err == nil {
				t.Fatal("Parse() unexpectedly accepted invalid request")
			}
		})
	}
}

func TestProtectedTargets(t *testing.T) {
	for _, target := range protectedTargets("linux") {
		if !isProtectedTargetForOS(target, "linux") {
			t.Errorf("isProtectedTarget(%q) = false", target)
		}
	}
	if isProtectedTargetForOS("/tmp/coolrestore-target", "linux") {
		t.Fatal("/tmp child unexpectedly treated as protected")
	}
}
