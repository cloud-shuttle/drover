//! Configuration file parsing and management

use std::path::PathBuf;
use std::time::Duration;
use serde::{Deserialize, Serialize};
use anyhow::Result;

/// Global configuration loaded from .drover.toml
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    /// Number of parallel workers
    #[serde(default = "default_workers")]
    pub workers: usize,

    /// Task timeout in seconds
    #[serde(default = "default_timeout")]
    pub timeout: u64,

    /// Max retry attempts per task
    #[serde(default = "default_retries")]
    pub retries: u32,

    /// Auto-create tasks to fix blockers
    #[serde(default = "default_auto_unblock")]
    pub auto_unblock: bool,

    /// Database URL for durable state
    #[serde(default = "default_database")]
    pub database: String,

    /// Stall threshold in seconds
    #[serde(default = "default_stall_threshold")]
    pub stall_threshold: u64,

    /// Poll interval in milliseconds
    #[serde(default = "default_poll_interval")]
    pub poll_interval: u64,

    /// Git worktree directory
    #[serde(default = "default_worktree_dir")]
    pub worktree_dir: Option<PathBuf>,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            workers: default_workers(),
            timeout: default_timeout(),
            retries: default_retries(),
            auto_unblock: default_auto_unblock(),
            database: default_database(),
            stall_threshold: default_stall_threshold(),
            poll_interval: default_poll_interval(),
            worktree_dir: default_worktree_dir(),
        }
    }
}

/// Runtime configuration for a Drover run
#[derive(Debug, Clone)]
pub struct DroverConfig {
    pub max_workers: usize,
    pub max_task_attempts: u32,
    pub task_timeout: Duration,
    pub stall_threshold: Duration,
    pub poll_interval: Duration,
    pub auto_unblock: bool,
    pub project_dir: PathBuf,
    pub task_limit: Option<usize>,
    pub worktree_dir: PathBuf,
}

impl From<Config> for DroverConfig {
    fn from(config: Config) -> Self {
        Self {
            max_workers: config.workers,
            max_task_attempts: config.retries,
            task_timeout: Duration::from_secs(config.timeout),
            stall_threshold: Duration::from_secs(config.stall_threshold),
            poll_interval: Duration::from_millis(config.poll_interval),
            auto_unblock: config.auto_unblock,
            project_dir: PathBuf::from("."),
            task_limit: None,
            worktree_dir: config.worktree_dir.unwrap_or_else(|| PathBuf::from(".drover/worktrees")),
        }
    }
}

impl DroverConfig {
    pub fn with_project_dir(mut self, dir: PathBuf) -> Self {
        self.project_dir = dir;
        self
    }

    pub fn with_task_limit(mut self, limit: Option<usize>) -> Self {
        self.task_limit = limit;
        self
    }
}

/// Load configuration from .drover.toml in the current directory
pub fn load_config(project_dir: &PathBuf) -> Result<Config> {
    let config_path = project_dir.join(".drover.toml");

    if !config_path.exists() {
        tracing::debug!("No .drover.toml found, using defaults");
        return Ok(Config::default());
    }

    let contents = std::fs::read_to_string(&config_path)?;
    let config: Config = toml::from_str(&contents)
        .map_err(|e| anyhow::anyhow!("Failed to parse .drover.toml: {}", e))?;

    tracing::debug!("Loaded config from {}", config_path.display());
    Ok(config)
}

// Default values
fn default_workers() -> usize { 4 }
fn default_timeout() -> u64 { 600 }
fn default_retries() -> u32 { 3 }
fn default_auto_unblock() -> bool { true }
fn default_database() -> String { "sqlite://.drover.db".to_string() }
fn default_stall_threshold() -> u64 { 300 }
fn default_poll_interval() -> u64 { 5000 }
fn default_worktree_dir() -> Option<PathBuf> { None }

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_config() {
        let config = Config::default();
        assert_eq!(config.workers, 4);
        assert_eq!(config.timeout, 600);
        assert_eq!(config.retries, 3);
        assert!(config.auto_unblock);
    }

    #[test]
    fn test_drover_config_from_config() {
        let config = Config::default();
        let drover_config = DroverConfig::from(config);
        assert_eq!(drover_config.max_workers, 4);
        assert_eq!(drover_config.task_timeout, Duration::from_secs(600));
    }
}
