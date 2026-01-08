// Package main
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install Drover globally",
		Long: `Install the Drover binary to a system location.

This command builds and installs Drover to:
  ~/bin/ (if it exists and is in PATH)
  /usr/local/bin/ (on Unix/Linux/macOS, requires sudo)
  C:\Program Files\Drover\ (on Windows)

After installation, you can run 'drover' from any directory.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			binDir := os.Getenv("GOBIN")
			if binDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("could not determine home directory: %w", err)
				}
				binDir = filepath.Join(homeDir, "bin")
			}

			// Check if bin directory exists
			if _, err := os.Stat(binDir); os.IsNotExist(err) {
				// Try /usr/local/bin as fallback
				binDir = "/usr/local/bin"
			}

			// Get the path to the current binary
			execPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("could not determine executable path: %w", err)
			}

			// Build for current platform if we're running from source
			if filepath.Base(execPath) != "drover" {
				fmt.Println("Building Drover...")
				buildCmd := exec.Command("go", "build", "-o", "drover", "./cmd/drover")
				buildCmd.Dir = filepath.Dir(execPath)
				if err := buildCmd.Run(); err != nil {
					return fmt.Errorf("build failed: %w", err)
				}
				execPath = filepath.Join(filepath.Dir(execPath), "drover")
			}

			// Copy to bin directory
			destPath := filepath.Join(binDir, "drover")
			input, err := os.ReadFile(execPath)
			if err != nil {
				return fmt.Errorf("reading binary: %w", err)
			}

			if err := os.WriteFile(destPath, input, 0755); err != nil {
				return fmt.Errorf("installing to %s: %w\nTry running with sudo", destPath, err)
			}

			fmt.Printf("✅ Installed Drover to %s\n", destPath)

			// Check if it's in PATH
			if _, err := exec.LookPath("drover"); err != nil {
				fmt.Printf("\n⚠️  Warning: %s may not be in your PATH\n", binDir)
				fmt.Println("\nAdd this to your ~/.bashrc or ~/.zshrc:")
				fmt.Printf("   export PATH=\"$PATH:%s\"\n", binDir)
				fmt.Println("\nThen run: source ~/.bashrc (or source ~/.zshrc)")
			} else {
				fmt.Println("✨ Drover is ready to use from any directory!")
			}

			fmt.Println("\nQuick start:")
			fmt.Println("  cd /path/to/your/project")
			fmt.Println("  drover init")
			fmt.Println("  drover add \"My first task\"")
			fmt.Println("  drover run")

			return nil
		},
	}
}
