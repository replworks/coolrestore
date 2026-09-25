package main

import (
	"fmt"
	"os"

	"github.com/replworks/coolrestore/internal/cli"
)

func main() {
	if _, err := cli.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "invalid restore request:", err)
		os.Exit(1)
	}
}
