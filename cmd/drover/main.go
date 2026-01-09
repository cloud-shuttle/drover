// Package main is the entry point for the Drover CLI
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/spf13/cobra"
)

var cfg *config.Config

func main() {
	var err error
	cfg, err = config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	rootCmd := &cobra.Command{
		Use:   "drover",
		Short: "Drive your project to completion with parallel AI agents",
		Long: `Drover is a durable workflow orchestrator that runs multiple Claude Code
agents in parallel to complete your entire project. It manages task dependencies,
handles failures gracefully, and guarantees progress through crashes and restarts.`,
		Version: "0.1.0",
	}

	rootCmd.AddCommand(
		initCmd(),
		runCmd(),
		addCmd(),
		epicCmd(),
		statusCmd(),
		resumeCmd(),
		resetCmd(),
		exportCmd(),
		installCmd(),
		dbosDemoCmd(),
		worktreeCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// findProjectDir locates the drover project root by searching upward
func findProjectDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".drover")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not a drover project (or any parent up to root)")
		}
		dir = parent
	}
}

// requireProject ensures we're in a drover project directory
func requireProject() (string, *db.Store, error) {
	dir, err := findProjectDir()
	if err != nil {
		return "", nil, err
	}

	store, err := db.Open(filepath.Join(dir, ".drover", "drover.db"))
	if err != nil {
		return "", nil, fmt.Errorf("opening database: %w", err)
	}

	return dir, store, nil
}
