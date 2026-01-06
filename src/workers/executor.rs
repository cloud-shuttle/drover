//! Task execution using Claude Code

use std::path::PathBuf;
use std::time::{Duration, Instant};
use tokio::process::Command;
use anyhow::{Result, Context};

use crate::drover::Task;

/// Executes tasks using Claude Code
pub struct TaskExecutor {
    project_dir: PathBuf,
    worktree_dir: PathBuf,
}

impl TaskExecutor {
    pub fn new(project_dir: PathBuf, worktree_dir: PathBuf) -> Self {
        Self {
            project_dir,
            worktree_dir,
        }
    }

    /// Execute a task using Claude Code
    pub async fn execute(&self, task: &Task) -> Result<Duration> {
        let start = Instant::now();

        // Build the prompt from the task
        let prompt = self.build_prompt(task);

        // Execute Claude Code
        let output = Command::new("claude")
            .args([
                "code",
                "--prompt",
                &prompt,
                "--non-interactive",
            ])
            .current_dir(&self.project_dir)
            .output()
            .await
            .context("Failed to execute Claude Code")?;

        let duration = start.elapsed();

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("Claude Code failed: {}", stderr);
        }

        tracing::info!("Task {} completed in {:?}", task.id, duration);
        Ok(duration)
    }

    fn build_prompt(&self, task: &Task) -> String {
        let mut prompt = format!("Task: {}\n", task.title);

        if let Some(desc) = &task.description {
            prompt.push_str(&format!("Description: {}\n", desc));
        }

        prompt.push_str("\nPlease implement this task completely.");

        if !task.blocked_by.is_empty() {
            prompt.push_str(&format!(
                "\n\nNote: This task was previously blocked by: {:?}. These should now be resolved.",
                task.blocked_by
            ));
        }

        prompt
    }
}
