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

	_, runErr := restore.Run(invocation.Confirm,
		func() (restore.Plan, error) {
			return restore.Plan{}, nil
		},
		func(restore.Plan) error {
			return fmt.Errorf("restore application is not implemented yet")
		},
	)
	cleanupErr := artifact.CleanupIfNeeded()
	releaseErr := targetLock.Release()
	if runErr != nil {
		return runErr
	}
	if cleanupErr != nil {
		return cleanupErr
	}
	return releaseErr
}
