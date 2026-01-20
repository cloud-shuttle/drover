// Package main provides the entry point for the drover-worker binary
package main

import (
	"fmt"
	"os"

	"github.com/cloud-shuttle/drover/internal/worker"
)

func main() {
	cli := worker.NewCLI()
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
