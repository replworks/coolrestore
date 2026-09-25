package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/replworks/coolrestore/internal/archive"
	"github.com/replworks/coolrestore/internal/cli"
	"github.com/replworks/coolrestore/internal/lock"
	"github.com/replworks/coolrestore/internal/restore"
	"github.com/replworks/coolrestore/internal/source"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "restore failed:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	invocation, err := cli.Parse(args)
	if err != nil {
		return err
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

func printPlan(w io.Writer, invocation cli.Invocation, plan restore.Plan) {
	fmt.Fprintf(w, "source: %s\ntarget: %s\nmode: %s\noutcome: planned\nregular_files: %d\n", invocation.Source, invocation.Target, plan.Mode, plan.RegularFiles)
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
	fmt.Fprintf(w, "source: %s\ntarget: %s\nmode: %s\noutcome: restored\nregular_files: %d\n", invocation.Source, invocation.Target, plan.Mode, plan.RegularFiles)
}

func printPaths(w io.Writer, label string, paths []string) {
	fmt.Fprintf(w, "%s:\n", label)
	for _, path := range paths {
		fmt.Fprintf(w, "  %s\n", path)
	}
}

func reportFailure(invocation cli.Invocation, step, targetState string, err error) error {
	fmt.Fprintf(os.Stderr, "source: %s\ntarget: %s\nmode: %s\noutcome: failed\nstep: %s\ntarget_state: %s\nerror: %v\n", invocation.Source, invocation.Target, invocation.Mode, step, targetState, err)
	return err
}
