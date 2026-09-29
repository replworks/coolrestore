package config

import (
	"os"
	"strings"
	"testing"
)

func TestParseEnvFileAcceptsSupportedAssignmentsAndComments(t *testing.T) {
	values, err := parseEnvFile(strings.NewReader(`# comment

AWS_ACCESS_KEY_ID=access
AWS_SECRET_ACCESS_KEY=secret
AWS_REGION=us-east-1
AWS_ENDPOINT_URL=https://nas.example.ts.net:9000
AWS_S3_FORCE_PATH_STYLE=true
`))
	if err != nil {
		t.Fatalf("parseEnvFile() error = %v", err)
	}
	if values["AWS_ACCESS_KEY_ID"] != "access" || values["AWS_SECRET_ACCESS_KEY"] != "secret" {
		t.Fatalf("unexpected credentials: %#v", values)
	}
	if values["AWS_ENDPOINT_URL"] != "https://nas.example.ts.net:9000" || values["AWS_S3_FORCE_PATH_STYLE"] != "true" {
		t.Fatalf("unexpected endpoint settings: %#v", values)
	}
}

func TestParseEnvFileRejectsShellSyntaxUnknownKeysMalformedAndDuplicates(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "shell export", file: "export AWS_REGION=us-east-1\n"},
		{name: "unknown key", file: "HOME=/tmp\n"},
		{name: "malformed", file: "AWS_REGION\n"},
		{name: "duplicate", file: "AWS_REGION=one\nAWS_REGION=two\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseEnvFile(strings.NewReader(test.file)); err == nil {
				t.Fatal("parseEnvFile() unexpectedly succeeded")
			}
		})
	}
}

func TestLoadEnvFileSetsValuesOnlyAfterSuccessfulParse(t *testing.T) {
	t.Setenv("AWS_REGION", "original")
	path := t.TempDir() + "/coolrestore.env"
	if err := os.WriteFile(path, []byte("AWS_REGION=us-east-1\nAWS_ENDPOINT_URL=http://nas\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile() error = %v", err)
	}
	if os.Getenv("AWS_REGION") != "us-east-1" || os.Getenv("AWS_ENDPOINT_URL") != "http://nas" {
		t.Fatalf("environment values were not loaded")
	}
}
