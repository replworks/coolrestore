package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/replworks/coolrestore/internal/cli"
	"github.com/replworks/coolrestore/internal/restore"
)

func TestPrintPlanContainsStableInvocationAndOutcomeFields(t *testing.T) {
	var output bytes.Buffer
	printPlan(&output, cli.Invocation{Source: "/backup.tar.gz", Target: "/restore", Mode: "merge"}, restore.Plan{
		Mode:         restore.ModeMerge,
		Added:        []string{"new.txt"},
		RegularFiles: 1,
	})
	for _, expected := range []string{
		"source: /backup.tar.gz",
		"target: /restore",
		"mode: merge",
		"outcome: planned",
		"regular_files: 1",
		"new.txt",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("plan report missing %q:\n%s", expected, output.String())
		}
	}
}

func TestPrintResultContainsRestoredOutcome(t *testing.T) {
	var output bytes.Buffer
	printResult(&output, cli.Invocation{Source: "s3://bucket/key", Target: "/restore", Mode: "replace"}, restore.Plan{
		Mode:         restore.ModeReplace,
		RegularFiles: 2,
	})
	if !strings.Contains(output.String(), "outcome: restored") || !strings.Contains(output.String(), "regular_files: 2") {
		t.Fatalf("unexpected result report:\n%s", output.String())
	}
}
