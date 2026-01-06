//! Worker pool for parallel task execution

pub mod executor;
pub mod git;

use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::{mpsc, RwLock};
use uuid::Uuid;
use anyhow::Result;
use serde::{Serialize, Deserialize};

use crate::drover::{Task, TaskStatus};
use self::executor::TaskExecutor;

/// Events emitted by workers
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum WorkerEvent {
    TaskCompleted { task_id: String, duration: Duration },
    TaskFailed { task_id: String, error: String, retriable: bool },
    TaskBlocked { task_id: String, blocked_by: Vec<String> },
    Stalled { duration: Duration },
}

/// Shared state between workers and orchestrator
#[derive(Clone)]
pub struct WorkerState {
    inner: Arc<RwLock<WorkerStateInner>>,
}

struct WorkerStateInner {
    tasks: Vec<Task>,
    current_assignments: std::collections::HashMap<String, String>, // worker_id -> task_id
}

impl WorkerState {
    pub fn new(tasks: Vec<Task>) -> Self {
        Self {
            inner: Arc::new(RwLock::new(WorkerStateInner {
                tasks,
                current_assignments: Default::default(),
            })),
        }
    }

    /// Try to claim a ready task
    pub async fn claim_task(&self, worker_id: &str) -> Option<Task> {
        let mut inner = self.inner.write().await;

        // Find highest priority ready task that's not claimed
        let task_idx = inner.tasks.iter().enumerate()
            .filter(|(_, t)| t.status == TaskStatus::Ready)
            .filter(|(_, t)| !inner.current_assignments.values().any(|id| id == &t.id))
            .max_by_key(|(_, t)| t.priority)
            .map(|(i, _)| i);

        if let Some(idx) = task_idx {
            let task = inner.tasks[idx].clone();
            inner.current_assignments.insert(worker_id.to_string(), task.id.clone());
            Some(task)
        } else {
            None
        }
    }

    /// Release a task claim
    pub async fn release_task(&self, worker_id: &str) {
        let mut inner = self.inner.write().await;
        inner.current_assignments.remove(worker_id);
    }

    /// Get count of ready tasks
    pub async fn ready_count(&self) -> usize {
        let inner = self.inner.read().await;
        inner.tasks.iter()
            .filter(|t| t.status == TaskStatus::Ready)
            .count()
    }
}

/// Pool of worker tasks
pub struct WorkerPool {
    config: WorkerPoolConfig,
    state: WorkerState,
    event_tx: mpsc::Sender<WorkerEvent>,
}

#[derive(Clone)]
pub struct WorkerPoolConfig {
    pub max_workers: usize,
    pub task_timeout: Duration,
    pub project_dir: PathBuf,
    pub worktree_dir: PathBuf,
}

impl WorkerPool {
    pub fn new(
        config: WorkerPoolConfig,
        state: WorkerState,
        event_tx: mpsc::Sender<WorkerEvent>,
    ) -> Result<Self> {
        Ok(Self {
            config,
            state,
            event_tx,
        })
    }

    /// Spawn worker tasks
    pub async fn spawn_workers(&self) -> Result<Vec<tokio::task::JoinHandle<()>>> {
        let mut handles = vec![];

        for i in 0..self.config.max_workers {
            let worker_id = format!("worker-{}", i);
            let state = self.state.clone();
            let event_tx = self.event_tx.clone();
            let config = self.config.clone();
            let executor = TaskExecutor::new(config.project_dir.clone(), config.worktree_dir.clone());

            let handle = tokio::spawn(async move {
                tracing::info!("Worker {} started", worker_id);

                loop {
                    // Try to claim a task
                    let task = match state.claim_task(&worker_id).await {
                        Some(t) => t,
                        None => {
                            // No work available, sleep and check again
                            tokio::time::sleep(Duration::from_secs(5)).await;

                            // Check if there's any work at all
                            if state.ready_count().await == 0 {
                                tracing::debug!("Worker {} exiting - no work", worker_id);
                                break;
                            }
                            continue;
                        }
                    };

                    tracing::info!("Worker {} claimed task {}", worker_id, task.id);

                    // Execute the task
                    let result: Result<Duration, anyhow::Error> = executor.execute(&task).await;

                    // Release the claim
                    state.release_task(&worker_id).await;

                    // Send event
                    match result {
                        Ok(duration) => {
                            let _ = event_tx.send(WorkerEvent::TaskCompleted {
                                task_id: task.id.clone(),
                                duration,
                            }).await;
                        }
                        Err(e) => {
                            let error_msg = e.to_string();
                            let retriable = !error_msg.contains("blocked");

                            if retriable {
                                let _ = event_tx.send(WorkerEvent::TaskFailed {
                                    task_id: task.id.clone(),
                                    error: error_msg,
                                    retriable: true,
                                }).await;
                            } else {
                                // Check if it's a blocking error
                                if error_msg.contains("blocked by") {
                                    let blockers = extract_blockers(&error_msg);
                                    let _ = event_tx.send(WorkerEvent::TaskBlocked {
                                        task_id: task.id.clone(),
                                        blocked_by: blockers,
                                    }).await;
                                } else {
                                    let _ = event_tx.send(WorkerEvent::TaskFailed {
                                        task_id: task.id.clone(),
                                        error: error_msg,
                                        retriable: false,
                                    }).await;
                                }
                            }
                        }
                    }
                }

                tracing::info!("Worker {} stopped", worker_id);
            });

            handles.push(handle);
        }

        Ok(handles)
    }
}

/// Extract blocker IDs from an error message
fn extract_blockers(msg: &str) -> Vec<String> {
    // Simple parsing - look for bead IDs like "bd-abc123"
    let re = regex::Regex::new(r"bd-[a-z0-9]+").unwrap();
    re.find_iter(msg)
        .map(|m: regex::Match| m.as_str().to_string())
        .collect()
}
