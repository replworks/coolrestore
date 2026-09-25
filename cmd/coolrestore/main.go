package main

import (
	"fmt"
	"os"

	"github.com/replworks/coolrestore/internal/cli"
	"github.com/replworks/coolrestore/internal/lock"
	"github.com/replworks/coolrestore/internal/restore"
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

	_, runErr := restore.Run(invocation.Confirm,
		func() (restore.Plan, error) {
			return restore.Plan{}, nil
		},
		func(restore.Plan) error {
			return fmt.Errorf("restore application is not implemented yet")
		},
	)
	releaseErr := targetLock.Release()
	if runErr != nil {
		return runErr
	}
	return releaseErr
}
