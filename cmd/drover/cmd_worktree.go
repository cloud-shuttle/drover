package main

import (
	"fmt"
	"path/filepath"

	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/spf13/cobra"
)

func worktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage git worktrees used for task execution",
		Long: `Manage git worktrees used for task execution.

Worktrees can consume significant disk space, especially with build artifacts
like Cargo targets (Rust) or node_modules (JavaScript). These commands help
manage and clean up worktrees to free up space.`,
	}

	cmd.AddCommand(
		worktreeListCmd(),
		worktreeCleanupCmd(),
		worktreePruneCmd(),
	)

	return cmd
}

// worktreeListCmd lists all worktrees with their status and disk usage
func worktreeListCmd() *cobra.Command {
	var verbose bool

	command := &cobra.Command{
		Use:   "list",
		Short: "List all worktrees with status and disk usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			worktreeDir := filepath.Join(projectDir, ".drover", "worktrees")

			// Get worktrees from database
			worktrees, err := store.ListWorktrees()
			if err != nil {
				return fmt.Errorf("querying worktrees: %w", err)
			}

			// Get worktrees on disk
			gitMgr := git.NewWorktreeManager(projectDir, worktreeDir)
			onDisk, _ := gitMgr.ListWorktreesOnDisk()

			// Build a map for quick lookup
			onDiskMap := make(map[string]bool)
			for _, id := range onDisk {
				onDiskMap[id] = true
			}

			if len(worktrees) == 0 && len(onDisk) == 0 {
				fmt.Println("No worktrees found")
				return nil
			}

			fmt.Println("\n🌳 Worktrees")
			fmt.Println("════════════")

			// Print tracked worktrees
			for _, w := range worktrees {
				onDiskIndicator := "✓"
				if !onDiskMap[w.TaskID] {
					onDiskIndicator = "✗ (missing)"
				}

				fmt.Printf("\n%s %s\n", onDiskIndicator, w.TaskID)
				fmt.Printf("  Status:    %s\n", w.Status)
				fmt.Printf("  Path:      %s\n", w.Path)
				if w.TaskTitle != "" {
					fmt.Printf("  Task:      %s\n", w.TaskTitle)
				}
				if w.TaskStatus != "" {
					fmt.Printf("  Task Status: %s\n", w.TaskStatus)
				}

				// Get disk usage
				if onDiskMap[w.TaskID] {
					size, _ := gitMgr.GetDiskUsage(w.TaskID)
					fmt.Printf("  Disk:      %s\n", formatBytes(size))

					// Show build artifacts if verbose
					if verbose {
						artifacts, _ := gitMgr.GetBuildArtifactSizes(w.TaskID)
						if len(artifacts) > 0 {
							fmt.Printf("  Artifacts:\n")
							for name, size := range artifacts {
								if size > 0 {
									fmt.Printf("    - %s: %s\n", name, formatBytes(size))
								}
							}
						}
					}
				}
			}

			// Print orphaned worktrees (on disk but not in database)
			var orphaned []string
			for _, id := range onDisk {
				found := false
				for _, w := range worktrees {
					if w.TaskID == id {
						found = true
						break
					}
				}
				if !found {
					orphaned = append(orphaned, id)
				}
			}

			if len(orphaned) > 0 {
				fmt.Println("\n👻 Orphaned (on disk but not tracked):")
				for _, id := range orphaned {
					size, _ := gitMgr.GetDiskUsage(id)
					fmt.Printf("  %s (%s)\n", id, formatBytes(size))
				}
			}

			return nil
		},
	}

	command.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show build artifact details")
	return command
}

// worktreeCleanupCmd removes all worktrees
func worktreeCleanupCmd() *cobra.Command {
	var force bool

	command := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove all worktrees to free disk space",
		Long: `Remove all worktrees to free disk space.

This command aggressively removes all worktrees and their build artifacts
including target/, node_modules/, vendor/, etc.

Use --force to skip confirmation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			worktreeDir := filepath.Join(projectDir, ".drover", "worktrees")
			gitMgr := git.NewWorktreeManager(projectDir, worktreeDir)

			// Get list of worktrees to clean
			worktrees, err := store.ListWorktrees()
			if err != nil {
				return fmt.Errorf("querying worktrees: %w", err)
			}

			if len(worktrees) == 0 {
				// Check for orphaned worktrees
				onDisk, _ := gitMgr.ListWorktreesOnDisk()
				if len(onDisk) == 0 {
					fmt.Println("No worktrees to clean")
					return nil
				}
				fmt.Printf("Found %d orphaned worktrees (not tracked in database)\n", len(onDisk))
			} else {
				fmt.Printf("Found %d tracked worktrees\n", len(worktrees))
			}

			// Calculate total disk usage
			var totalSize int64
			for _, w := range worktrees {
				size, _ := gitMgr.GetDiskUsage(w.TaskID)
				totalSize += size
			}

			if totalSize > 0 {
				fmt.Printf("Total disk usage: %s\n", formatBytes(totalSize))
			}

			// Confirm unless --force
			if !force {
				fmt.Print("\nRemove all worktrees? [y/N] ")
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Aborted")
					return nil
				}
			}

			// Clean up all worktrees
			count, freed, err := gitMgr.CleanupAll()
			if err != nil {
				return fmt.Errorf("cleaning up worktrees: %w", err)
			}

			// Mark all worktrees as removed in database
			for _, w := range worktrees {
				store.UpdateWorktreeStatus(w.TaskID, "removed")
			}

			fmt.Printf("\n✅ Removed %d worktrees, freed %s\n", count, formatBytes(freed))
			return nil
		},
	}

	command.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	return command
}

// worktreePruneCmd removes worktrees for completed/failed tasks
func worktreePruneCmd() *cobra.Command {
	var force bool
	var aggressive bool

	command := &cobra.Command{
		Use:   "prune",
		Short: "Remove worktrees for completed or failed tasks",
		Long: `Remove worktrees for completed or failed tasks.

This is safer than 'cleanup' as it only removes worktrees for tasks that
are no longer active (completed or failed).

Use --aggressive to also remove build artifacts (target/, node_modules/, etc.)
Use --force to skip confirmation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			worktreeDir := filepath.Join(projectDir, ".drover", "worktrees")
			gitMgr := git.NewWorktreeManager(projectDir, worktreeDir)

			// Get worktrees that can be pruned
			worktrees, err := store.GetWorktreesForCleanup(true) // completedOnly=true
			if err != nil {
				return fmt.Errorf("querying worktrees for cleanup: %w", err)
			}

			// Also check for orphaned worktrees upfront (on disk but not in database)
			orphanedPaths, err := store.GetOrphanedWorktrees(worktreeDir)
			if err != nil {
				return fmt.Errorf("listing orphaned worktrees: %w", err)
			}

			// Extract task IDs from orphaned paths
			orphanedTaskIDs := make([]string, 0, len(orphanedPaths))
			for _, path := range orphanedPaths {
				orphanedTaskIDs = append(orphanedTaskIDs, filepath.Base(path))
			}

			if len(worktrees) == 0 && len(orphanedTaskIDs) == 0 {
				fmt.Println("No worktrees to prune (no completed/failed tasks with worktrees)")
				return nil
			}

			// If we only have orphaned worktrees, clean them up
			if len(worktrees) == 0 && len(orphanedTaskIDs) > 0 {
				fmt.Printf("Found %d orphaned worktree(s) (not tracked in database)\n", len(orphanedTaskIDs))

				// Calculate sizes before removal
				orphanedSizes := make(map[string]int64)
				var totalOrphanedSize int64
				for _, taskID := range orphanedTaskIDs {
					if size, err := gitMgr.GetDiskUsage(taskID); err == nil {
						orphanedSizes[taskID] = size
						totalOrphanedSize += size
					}
				}
				if totalOrphanedSize > 0 {
					fmt.Printf("Total disk usage: %s\n", formatBytes(totalOrphanedSize))
				}

				fmt.Println("\nOrphaned worktrees to be removed:")
				for _, taskID := range orphanedTaskIDs {
					fmt.Printf("  - %s (%s)\n", taskID, formatBytes(orphanedSizes[taskID]))
				}

				// Confirm unless --force
				if !force {
					fmt.Print("\nRemove these orphaned worktrees? [y/N] ")
					var response string
					fmt.Scanln(&response)
					if response != "y" && response != "Y" {
						fmt.Println("Aborted")
						return nil
					}
				}

				// Remove orphaned worktrees
				var totalFreed int64
				for _, taskID := range orphanedTaskIDs {
					worktreePath := filepath.Join(worktreeDir, taskID)
					var freed int64
					var err error

					if aggressive {
						freed, err = gitMgr.RemoveAggressiveByPath(worktreePath)
					} else {
						err = gitMgr.RemoveByPath(worktreePath)
						// For non-aggressive removal, use the pre-calculated size
						freed = orphanedSizes[taskID]
					}

					if err != nil {
						fmt.Printf("⚠️  Failed to remove %s: %v\n", taskID, err)
						continue
					}
					totalFreed += freed
				}

				fmt.Printf("\n%s Pruned %d orphaned worktrees, freed %s\n", "✅", len(orphanedTaskIDs), formatBytes(totalFreed))
				return nil
			}

			fmt.Printf("Found %d worktrees for completed/failed tasks\n", len(worktrees))

			// Calculate total disk usage
			var totalSize int64
			for _, w := range worktrees {
				size, _ := gitMgr.GetDiskUsage(w.TaskID)
				totalSize += size
			}

			if totalSize > 0 {
				fmt.Printf("Total disk usage: %s\n", formatBytes(totalSize))
			}

			fmt.Println("\nWorktrees to be removed:")
			for _, w := range worktrees {
				size, _ := gitMgr.GetDiskUsage(w.TaskID)
				fmt.Printf("  - %s: %s (%s)\n", w.TaskID, w.TaskTitle, formatBytes(size))
			}

			// Confirm unless --force
			if !force {
				fmt.Print("\nRemove these worktrees? [y/N] ")
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Aborted")
					return nil
				}
			}

			// Remove each worktree
			var totalFreed int64
			for _, w := range worktrees {
				var freed int64
				var err error

				if aggressive {
					freed, err = gitMgr.RemoveAggressive(w.TaskID)
				} else {
					err = gitMgr.Remove(w.TaskID)
				}

				if err != nil {
					fmt.Printf("⚠️  Failed to remove %s: %v\n", w.TaskID, err)
					continue
				}

				totalFreed += freed
				store.UpdateWorktreeStatus(w.TaskID, "removed")
				store.DeleteWorktree(w.TaskID)
			}

			fmt.Printf("\n%s Pruned %d worktrees, freed %s\n", "✅", len(worktrees), formatBytes(totalFreed))

			// Prune orphaned worktrees too
			orphaned, orphanedFreed, err := gitMgr.PruneOrphaned()
			if err == nil && len(orphaned) > 0 {
				fmt.Printf("Also removed %d orphaned worktrees, freed %s\n", len(orphaned), formatBytes(orphanedFreed))
			}

			return nil
		},
	}

	command.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	command.Flags().BoolVarP(&aggressive, "aggressive", "a", false, "Remove build artifacts (target/, node_modules/, etc.)")
	return command
}
