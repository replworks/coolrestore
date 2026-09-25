package main

import (
	"context"
	"fmt"
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
		return err
	}

	artifact, err := source.Acquire(context.Background(), invocation.Source, invocation.SkipChecksum)
	if err != nil {
		_ = targetLock.Release()
		return err
	}
	if err := archive.ValidateTarGz(artifact.Path); err != nil {
		_ = artifact.CleanupIfNeeded()
		_ = targetLock.Release()
		return err
	}
	if _, err := archive.CheckCapacity(artifact.Path, invocation.Staging, archive.OSSpaceChecker{}); err != nil {
		_ = artifact.CleanupIfNeeded()
		_ = targetLock.Release()
		return err
	}
	if err := archive.ValidateTarGzSafety(artifact.Path); err != nil {
		_ = artifact.CleanupIfNeeded()
		_ = targetLock.Release()
		return err
	}
	staging, err := archive.StageTarGz(artifact.Path, invocation.Target, invocation.Staging)
	if err != nil {
		_ = artifact.CleanupIfNeeded()
		_ = targetLock.Release()
		return err
	}

	plan, _, runErr := restore.Run(invocation.Confirm,
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
		return runErr
	}
	if cleanupErr != nil {
		return cleanupErr
	}
	if stagingCleanupErr != nil {
		return stagingCleanupErr
	}
	if !invocation.Confirm {
		printPlan(plan)
	}
	return releaseErr
}

func printPlan(plan restore.Plan) {
	fmt.Printf("plan-only restore (mode=%s, regular files=%d)\n", plan.Mode, plan.RegularFiles)
	switch plan.Mode {
	case restore.ModeMerge:
		printPaths("added", plan.Added)
		printPaths("overwritten", plan.Overwritten)
	case restore.ModeReplace:
		printPaths("result", plan.ResultPaths)
		printPaths("removed", plan.Removed)
	}
}

func printPaths(label string, paths []string) {
	fmt.Printf("%s:\n", label)
	for _, path := range paths {
		fmt.Printf("  %s\n", path)
	}
}
