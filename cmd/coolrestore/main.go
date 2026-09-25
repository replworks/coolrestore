package main

import (
	"fmt"
	"os"

	"github.com/replworks/coolrestore/internal/cli"
	"github.com/replworks/coolrestore/internal/restore"
)

func main() {
	invocation, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid restore request:", err)
		os.Exit(1)
	}

	_, err = restore.Run(invocation.Confirm,
		func() (restore.Plan, error) {
			return restore.Plan{}, nil
		},
		func(restore.Plan) error {
			return fmt.Errorf("restore application is not implemented yet")
		},
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "restore failed:", err)
		os.Exit(1)
	}
}
