//! Git worktree management for isolated worker environments

use std::path::PathBuf;
use anyhow::{Result, Context};
use tokio::process::Command;

/// Manages git worktrees for parallel task execution
pub struct WorktreeManager {
    project_dir: PathBuf,
    worktree_base: PathBuf,
}

impl WorktreeManager {
    pub fn new(project_dir: PathBuf, worktree_base: PathBuf) -> Self {
        Self {
            project_dir,
            worktree_base,
        }
    }

    /// Create a new worktree for a worker
    pub async fn create_worktree(&self, worker_id: &str) -> Result<PathBuf> {
        let worktree_path = self.worktree_base.join(worker_id);

        // Create base directory if needed
        tokio::fs::create_dir_all(&self.worktree_base).await
            .context("Failed to create worktree base directory")?;

        // Create the worktree
        let output = Command::new("git")
            .args([
                "worktree",
                "add",
                "-b",
                &format!("drover/{}", worker_id),
                worktree_path.to_str().unwrap(),
            ])
            .current_dir(&self.project_dir)
            .output()
            .await
            .context("Failed to create git worktree")?;

        if !output.status.success() {
            anyhow::bail!(
                "Failed to create worktree: {}",
                String::from_utf8_lossy(&output.stderr)
            );
        }

        tracing::info!("Created worktree for {} at {:?}", worker_id, worktree_path);
        Ok(worktree_path)
    }

    /// Remove a worktree
    pub async fn remove_worktree(&self, worker_id: &str) -> Result<()> {
        let worktree_path = self.worktree_base.join(worker_id);

        // Remove the worktree
        let output = Command::new("git")
            .args([
                "worktree",
                "remove",
                "--force",
                worktree_path.to_str().unwrap(),
            ])
            .current_dir(&self.project_dir)
            .output()
            .await
            .context("Failed to remove git worktree")?;

        if !output.status.success() {
            // Non-fatal, log and continue
            tracing::warn!(
                "Failed to remove worktree: {}",
                String::from_utf8_lossy(&output.stderr)
            );
        } else {
            tracing::info!("Removed worktree for {}", worker_id);
        }

        Ok(())
    }

    /// Clean up all worktrees
    pub async fn cleanup_all(&self) -> Result<()> {
        let output = Command::new("git")
            .args(["worktree", "list", "--porcelain"])
            .current_dir(&self.project_dir)
            .output()
            .await
            .context("Failed to list worktrees")?;

        let stdout = String::from_utf8_lossy(&output.stdout);

        for line in stdout.lines() {
            if line.starts_with("worktree ") {
                let path = line.trim_start_matches("worktree ");
                let path_buf = PathBuf::from(path);

                // Only remove worktrees in our managed directory
                if path_buf.starts_with(&self.worktree_base) {
                    if let Some(name) = path_buf.file_name() {
                        let _ = self.remove_worktree(&name.to_string_lossy()).await;
                    }
                }
            }
        }

        Ok(())
    }

    /// List all active worktrees
    pub async fn list_worktrees(&self) -> Result<Vec<String>> {
        let output = Command::new("git")
            .args(["worktree", "list", "--porcelain"])
            .current_dir(&self.project_dir)
            .output()
            .await
            .context("Failed to list worktrees")?;

        let stdout = String::from_utf8_lossy(&output.stdout);
        let mut worktrees = Vec::new();

        for line in stdout.lines() {
            if line.starts_with("worktree ") {
                let path = line.trim_start_matches("worktree ");
                let path_buf = PathBuf::from(path);

                if path_buf.starts_with(&self.worktree_base) {
                    if let Some(name) = path_buf.file_name() {
                        worktrees.push(name.to_string_lossy().to_string());
                    }
                }
            }
        }

        Ok(worktrees)
    }
}
