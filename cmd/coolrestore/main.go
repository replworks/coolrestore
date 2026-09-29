package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/replworks/coolrestore/internal/archive"
	"github.com/replworks/coolrestore/internal/cli"
	"github.com/replworks/coolrestore/internal/config"
	"github.com/replworks/coolrestore/internal/lock"
	"github.com/replworks/coolrestore/internal/restore"
	"github.com/replworks/coolrestore/internal/source"
)

const (
	exitSuccess = 0
	exitFailure = 1
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "coolrestore failed:", err)
		os.Exit(exitFailure)
	}
	os.Exit(exitSuccess)
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "--version" {
		printVersion(os.Stdout)
		return nil
	}
	if len(args) > 0 && args[0] == "diagnose" {
		return runDiagnose(args[1:])
	}
	return runRestore(args)
}

func printVersion(w io.Writer) {
	_, _ = fmt.Fprintf(w, "coolrestore %s\n", version)
}

func runDiagnose(args []string) error {
	invocation, err := cli.ParseDiagnose(args)
	if err != nil {
		return err
	}
	if invocation.EnvFile != "" {
		if err := config.LoadEnvFile(invocation.EnvFile); err != nil {
			return reportDiagnosisFailure(invocation, "environment file", err)
		}
	}

	diagnosis, err := source.Diagnose(context.Background(), invocation.Source)
	if err != nil {
		return reportDiagnosisFailure(invocation, "source access", err)
	}
	printDiagnosis(os.Stdout, diagnosis)
	return nil
}

func runRestore(args []string) error {
	invocation, err := cli.Parse(args)
	if err != nil {
		return err
	}
	if invocation.EnvFile != "" {
		if err := config.LoadEnvFile(invocation.EnvFile); err != nil {
			return reportFailure(invocation, "environment file", "unchanged", err)
		}
	}

	targetLock, err := lock.Acquire(invocation.Target)
	if err != nil {
		return reportFailure(invocation, "target lock", "unchanged", err)
	}

	artifact, err := source.Acquire(context.Background(), invocation.Source, invocation.SkipChecksum)
	if err != nil {
		_ = targetLock.Release()
		return reportFailure(invocation, "archive acquisition", "unchanged", err)
	}
	if err := archive.ValidateTarGz(artifact.Path); err != nil {
		_ = artifact.CleanupIfNeeded()
		_ = targetLock.Release()
		return reportFailure(invocation, "archive validation", "unchanged", err)
	}
	if _, err := archive.CheckCapacity(artifact.Path, invocation.Staging, archive.OSSpaceChecker{}); err != nil {
		_ = artifact.CleanupIfNeeded()
		_ = targetLock.Release()
		return reportFailure(invocation, "capacity check", "unchanged", err)
	}
	if err := archive.ValidateTarGzSafety(artifact.Path); err != nil {
		_ = artifact.CleanupIfNeeded()
		_ = targetLock.Release()
		return reportFailure(invocation, "content safety validation", "unchanged", err)
	}
	staging, err := archive.StageTarGz(artifact.Path, invocation.Target, invocation.Staging)
	if err != nil {
		_ = artifact.CleanupIfNeeded()
		_ = targetLock.Release()
		return reportFailure(invocation, "staged extraction", "unchanged", err)
	}

	plan, outcome, runErr := restore.Run(invocation.Confirm,
		func() (restore.Plan, error) {
			return restore.PlanRestore(staging.Dir, invocation.Target, restore.Mode(invocation.Mode))
		},
		func(plan restore.Plan) error {
			if restore.Mode(invocation.Mode) == restore.ModeMerge {
				return restore.ApplyMerge(staging.Dir, invocation.Target, plan)
			}
			return restore.ApplyReplace(staging.Dir, invocation.Target, plan)
		},
	)
	stagingCleanupErr := staging.Cleanup()
	cleanupErr := artifact.CleanupIfNeeded()
	releaseErr := targetLock.Release()
	if runErr != nil {
		if outcome == restore.OutcomeApplied {
			if restore.Mode(invocation.Mode) == restore.ModeReplace {
				return reportFailure(invocation, "change application", "restored by atomic rollback", runErr)
			}
			return reportFailure(invocation, "change application", "may contain partial changes", runErr)
		}
		return reportFailure(invocation, "restore planning", "unchanged", runErr)
	}
	if cleanupErr != nil {
		return reportFailure(invocation, "archive cleanup", "may contain changes", cleanupErr)
	}
	if stagingCleanupErr != nil {
		return reportFailure(invocation, "staging cleanup", "may contain changes", stagingCleanupErr)
	}
	if releaseErr != nil {
		return reportFailure(invocation, "target lock release", "may contain changes", releaseErr)
	}
	if !invocation.Confirm {
		printPlan(os.Stdout, invocation, plan)
		return nil
	}
	printResult(os.Stdout, invocation, plan)
	return nil
}

func printDiagnosis(w io.Writer, diagnosis source.Diagnosis) {
	_, _ = fmt.Fprintf(w, "source: %s\n", diagnosis.Source)
	if diagnosis.Bucket != "" {
		endpoint := "AWS SDK default"
		if os.Getenv("AWS_ENDPOINT_URL") != "" {
			endpoint = "configured"
		}
		_, _ = fmt.Fprintf(w, "endpoint: %s\nbucket: %s\nobject: %s\nobject_size: %d\noutcome: reachable\n", endpoint, diagnosis.Bucket, diagnosis.Key, diagnosis.Size)
		return
	}
	_, _ = fmt.Fprintf(w, "source_type: local\nsize: %d\noutcome: readable\n", diagnosis.Size)
}

func reportDiagnosisFailure(invocation cli.DiagnoseInvocation, step string, err error) error {
	_, _ = fmt.Fprintf(os.Stderr, "source: %s\noutcome: failed\nstep: %s\nerror: %v\n", invocation.Source, step, err)
	return err
}

func printPlan(w io.Writer, invocation cli.Invocation, plan restore.Plan) {
	_, _ = fmt.Fprintf(w, "source: %s\ntarget: %s\nmode: %s\noutcome: planned\nregular_files: %d\n", invocation.Source, invocation.Target, plan.Mode, plan.RegularFiles)
	switch plan.Mode {
	case restore.ModeMerge:
		printPaths(w, "added", plan.Added)
		printPaths(w, "overwritten", plan.Overwritten)
	case restore.ModeReplace:
		printPaths(w, "result", plan.ResultPaths)
		printPaths(w, "removed", plan.Removed)
	}
}

func printResult(w io.Writer, invocation cli.Invocation, plan restore.Plan) {
	_, _ = fmt.Fprintf(w, "source: %s\ntarget: %s\nmode: %s\noutcome: restored\nregular_files: %d\n", invocation.Source, invocation.Target, plan.Mode, plan.RegularFiles)
}

func printPaths(w io.Writer, label string, paths []string) {
	_, _ = fmt.Fprintf(w, "%s:\n", label)
	for _, path := range paths {
		_, _ = fmt.Fprintf(w, "  %s\n", path)
	}
}

func reportFailure(invocation cli.Invocation, step, targetState string, err error) error {
	_, _ = fmt.Fprintf(os.Stderr, "source: %s\ntarget: %s\nmode: %s\noutcome: failed\nstep: %s\ntarget_state: %s\nerror: %v\n", invocation.Source, invocation.Target, invocation.Mode, step, targetState, err)
	return err
}
