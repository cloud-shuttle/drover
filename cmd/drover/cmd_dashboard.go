package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cloud-shuttle/drover/internal/dashboard"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/spf13/cobra"
)

func dashboardCmd() *cobra.Command {
	var (
		port string
		open bool
	)

	command := &cobra.Command{
		Use:   "dashboard",
		Short: "Start the web dashboard",
		Long:  `Start a local web dashboard for visualizing project progress, tasks, and workers in real-time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			return runDashboard(store, projectDir, port, open)
		},
	}

	command.Flags().StringVarP(&port, "port", "p", "3847", "Port to run dashboard on")
	command.Flags().BoolVar(&open, "open", false, "Open browser automatically")
	return command
}

func runDashboard(store *db.Store, projectDir string, port string, openBrowser bool) error {
	// Import dashboard package
	dash := dashboard.Config{
		Addr:        ":" + port,
		DatabaseURL: filepath.Join(projectDir, ".drover", "drover.db"),
		Store:       store,
	}

	server, err := dashboard.New(dash)
	if err != nil {
		return fmt.Errorf("creating dashboard: %w", err)
	}

	// Set global dashboard for event broadcasting
	dashboard.SetGlobal(server)

	// Open browser if requested
	if openBrowser {
		go func() {
			time.Sleep(500 * time.Millisecond)
			url := fmt.Sprintf("http://localhost:%s", port)
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				cmd = exec.Command("open", url)
			case "windows":
				cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
			default: // linux, bsd, etc.
				cmd = exec.Command("xdg-open", url)
			}
			_ = cmd.Run()
		}()
	}

	return server.Start()
}
