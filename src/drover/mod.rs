// drover/mod.rs - Core orchestration logic
// Discovers all work in a project and drives it to completion

use std::collections::{HashMap, HashSet};
use std::path::PathBuf;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::{mpsc, RwLock};
use tokio::process::Command;
use serde::{Deserialize, Serialize};
use uuid::Uuid;
use tracing::{info, warn, error, instrument};

use crate::durable::DurableStore;
use crate::workers::{WorkerPool, WorkerEvent, WorkerState};
use crate::dashboard::Dashboard;
use crate::config::DroverConfig;

// ============================================================================
// TYPES
// ============================================================================

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Task {
    pub id: String,
    pub title: String,
    pub description: Option<String>,
    pub priority: i32,
    pub status: TaskStatus,
    pub parent_epic: Option<String>,
    pub blocked_by: Vec<String>,
    pub labels: Vec<String>,
    pub attempts: u32,
    pub last_error: Option<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum TaskStatus {
    Ready,
    Claimed,
    InProgress,
    Blocked,
    Completed,
    Failed,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Epic {
    pub id: String,
    pub title: String,
    pub tasks: Vec<Task>,
    pub progress: f32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkManifest {
    pub epics: Vec<Epic>,
    pub standalone_tasks: Vec<Task>,
    pub total_tasks: usize,
    pub ready_tasks: usize,
    pub blocked_tasks: usize,
    pub completed_tasks: usize,
}

impl WorkManifest {
    pub fn target_description(&self) -> String {
        if self.epics.len() == 1 && self.standalone_tasks.is_empty() {
            format!("Epic: {}", self.epics[0].title)
        } else if self.epics.is_empty() && !self.standalone_tasks.is_empty() {
            format!("{} standalone tasks", self.standalone_tasks.len())
        } else {
            format!("{} epics, {} standalone tasks", self.epics.len(), self.standalone_tasks.len())
        }
    }

    pub fn all_tasks(&self) -> Vec<&Task> {
        let mut tasks: Vec<&Task> = self.epics.iter()
            .flat_map(|e| e.tasks.iter())
            .collect();
        tasks.extend(self.standalone_tasks.iter());
        tasks
    }

    pub fn all_tasks_mut(&mut self) -> Vec<&mut Task> {
        let mut tasks: Vec<&mut Task> = self.epics.iter_mut()
            .flat_map(|e| e.tasks.iter_mut())
            .collect();
        tasks.extend(self.standalone_tasks.iter_mut());
        tasks
    }
}

#[derive(Debug, Clone, Serialize)]
pub struct DroverResult {
    pub success: bool,
    pub duration: Duration,
    pub tasks_completed: u32,
    pub tasks_failed: u32,
    pub success_rate: f32,
    pub blockers: Vec<String>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ProjectStatus {
    pub total: usize,
    pub completed: usize,
    pub ready: usize,
    pub blocked: usize,
    pub failed: usize,
    pub progress: f32,
    pub elapsed: Option<Duration>,
    pub eta: Option<Duration>,
}

// ============================================================================
// WORK DISCOVERY
// ============================================================================

/// Discover all work in the project
pub async fn discover_work(
    project_dir: &PathBuf,
    epic_filter: Option<&str>,
) -> anyhow::Result<WorkManifest> {
    if let Some(epic_id) = epic_filter {
        // Single epic mode
        let epic = load_epic(project_dir, epic_id).await?;
        let stats = calculate_stats(&epic.tasks);

        Ok(WorkManifest {
            total_tasks: epic.tasks.len(),
            ready_tasks: stats.ready,
            blocked_tasks: stats.blocked,
            completed_tasks: stats.completed,
            epics: vec![epic],
            standalone_tasks: vec![],
        })
    } else {
        // Full project mode - get all open work
        load_all_work(project_dir).await
    }
}

async fn load_epic(project_dir: &PathBuf, epic_id: &str) -> anyhow::Result<Epic> {
    let output = Command::new("bd")
        .args(["show", epic_id, "--json", "--recursive"])
        .current_dir(project_dir)
        .output()
        .await?;

    if !output.status.success() {
        anyhow::bail!("Failed to load epic: {}", String::from_utf8_lossy(&output.stderr));
    }

    let bead: BeadItem = serde_json::from_slice(&output.stdout)?;
    let tasks = load_epic_tasks(project_dir, epic_id).await?;

    Ok(Epic {
        id: bead.id,
        title: bead.title,
        progress: calculate_progress(&tasks),
        tasks,
    })
}

async fn load_epic_tasks(project_dir: &PathBuf, epic_id: &str) -> anyhow::Result<Vec<Task>> {
    let output = Command::new("bd")
        .args(["ls", "--parent", epic_id, "--json", "--all"])
        .current_dir(project_dir)
        .output()
        .await?;

    if !output.status.success() {
        return Ok(vec![]);
    }

    let beads: Vec<BeadItem> = serde_json::from_slice(&output.stdout).unwrap_or_default();
    Ok(beads.into_iter().map(|b| b.into_task(Some(epic_id.to_string()))).collect())
}

async fn load_all_work(project_dir: &PathBuf) -> anyhow::Result<WorkManifest> {
    // Get all open items
    let output = Command::new("bd")
        .args(["ls", "--json", "--all"])
        .current_dir(project_dir)
        .output()
        .await?;

    if !output.status.success() {
        anyhow::bail!("Failed to list tasks: {}", String::from_utf8_lossy(&output.stderr));
    }

    let beads: Vec<BeadItem> = serde_json::from_slice(&output.stdout).unwrap_or_default();

    // Separate epics from tasks
    let mut epics: HashMap<String, Epic> = HashMap::new();
    let mut standalone: Vec<Task> = vec![];
    let mut epic_ids: HashSet<String> = HashSet::new();

    // First pass: identify epics
    for bead in &beads {
        if bead.is_epic.unwrap_or(false) || bead.labels.contains(&"epic".to_string()) {
            epic_ids.insert(bead.id.clone());
            epics.insert(bead.id.clone(), Epic {
                id: bead.id.clone(),
                title: bead.title.clone(),
                tasks: vec![],
                progress: 0.0,
            });
        }
    }

    // Second pass: assign tasks to epics or standalone
    for bead in beads {
        if epic_ids.contains(&bead.id) {
            continue; // Skip the epic itself
        }

        let task = bead.into_task(None);

        if let Some(parent) = &task.parent_epic {
            if let Some(epic) = epics.get_mut(parent) {
                epic.tasks.push(task);
            } else {
                standalone.push(task);
            }
        } else {
            standalone.push(task);
        }
    }

    // Calculate progress for each epic
    for epic in epics.values_mut() {
        epic.progress = calculate_progress(&epic.tasks);
    }

    // Filter out completed epics
    let active_epics: Vec<Epic> = epics.into_values()
        .filter(|e| e.progress < 100.0 || e.tasks.iter().any(|t| t.status != TaskStatus::Completed))
        .collect();

    // Filter out completed standalone tasks
    let active_standalone: Vec<Task> = standalone.into_iter()
        .filter(|t| t.status != TaskStatus::Completed)
        .collect();

    let all_tasks: Vec<&Task> = active_epics.iter()
        .flat_map(|e| e.tasks.iter())
        .chain(active_standalone.iter())
        .collect();

    let stats = calculate_stats_from_refs(&all_tasks);

    Ok(WorkManifest {
        total_tasks: all_tasks.len(),
        ready_tasks: stats.ready,
        blocked_tasks: stats.blocked,
        completed_tasks: stats.completed,
        epics: active_epics,
        standalone_tasks: active_standalone,
    })
}

pub async fn list_all_tasks(project_dir: &PathBuf) -> anyhow::Result<Vec<Task>> {
    let manifest = load_all_work(project_dir).await?;
    let mut tasks: Vec<Task> = manifest.epics.into_iter()
        .flat_map(|e| e.tasks)
        .collect();
    tasks.extend(manifest.standalone_tasks);
    Ok(tasks)
}

pub async fn get_project_status(
    project_dir: &PathBuf,
    epic: Option<&str>,
) -> anyhow::Result<ProjectStatus> {
    let manifest = discover_work(project_dir, epic).await?;

    let failed = manifest.all_tasks().iter()
        .filter(|t| t.status == TaskStatus::Failed)
        .count();

    let progress = if manifest.total_tasks > 0 {
        (manifest.completed_tasks as f32 / manifest.total_tasks as f32) * 100.0
    } else {
        100.0
    };

    Ok(ProjectStatus {
        total: manifest.total_tasks,
        completed: manifest.completed_tasks,
        ready: manifest.ready_tasks,
        blocked: manifest.blocked_tasks,
        failed,
        progress,
        elapsed: None, // Would come from active run
        eta: None,
    })
}

// ============================================================================
// ORCHESTRATOR
// ============================================================================

pub struct Drover {
    manifest: WorkManifest,
    config: DroverConfig,
    store: DurableStore,
    run_id: Uuid,
    state: Arc<RwLock<DroverState>>,
}

struct DroverState {
    tasks: HashMap<String, Task>,
    started_at: Instant,
    completed_count: u32,
    failed_count: u32,
    last_progress: Instant,
    auto_created_tasks: HashSet<String>,
}

impl Drover {
    pub async fn new(
        manifest: WorkManifest,
        config: DroverConfig,
        store: DurableStore,
    ) -> anyhow::Result<Self> {
        let run_id = Uuid::new_v4();

        // Build task map
        let mut tasks = HashMap::new();
        for epic in &manifest.epics {
            for task in &epic.tasks {
                tasks.insert(task.id.clone(), task.clone());
            }
        }
        for task in &manifest.standalone_tasks {
            tasks.insert(task.id.clone(), task.clone());
        }

        let state = DroverState {
            tasks,
            started_at: Instant::now(),
            completed_count: 0,
            failed_count: 0,
            last_progress: Instant::now(),
            auto_created_tasks: HashSet::new(),
        };

        // Checkpoint initial state
        store.start_run(&run_id, &manifest).await?;

        Ok(Self {
            manifest,
            config,
            store,
            run_id,
            state: Arc::new(RwLock::new(state)),
        })
    }

    #[instrument(skip(self))]
    pub async fn run(&mut self) -> anyhow::Result<DroverResult> {
        info!(run_id = %self.run_id, "Starting Drover run");

        // Build tasks list for worker state
        let all_tasks: Vec<Task> = self.manifest.epics.iter()
            .flat_map(|e| e.tasks.clone())
            .chain(self.manifest.standalone_tasks.clone())
            .collect();

        let worker_state = WorkerState::new(all_tasks);

        // Create worker pool
        let (event_tx, mut event_rx) = mpsc::channel(1000);
        let pool_config = crate::workers::WorkerPoolConfig {
            max_workers: self.config.max_workers,
            task_timeout: self.config.task_timeout,
            project_dir: self.config.project_dir.clone(),
            worktree_dir: self.config.worktree_dir.clone(),
        };

        let pool = WorkerPool::new(
            pool_config,
            worker_state,
            event_tx,
        )?;

        // Spawn workers
        let worker_handles = pool.spawn_workers().await?;

        // Spawn background tasks
        let stall_detector = self.spawn_stall_detector();

        let mut tasks_processed = 0;

        // Main event loop
        loop {
            tokio::select! {
                Some(event) = event_rx.recv() => {
                    self.handle_event(event).await?;
                    tasks_processed += 1;

                    // Check task limit
                    if let Some(limit) = self.config.task_limit {
                        if tasks_processed >= limit {
                            info!("Task limit reached");
                            break;
                        }
                    }
                }
                _ = tokio::time::sleep(self.config.poll_interval) => {
                    // Periodic check
                    if self.is_complete().await {
                        break;
                    }
                }
            }

            if self.is_complete().await {
                break;
            }
        }

        // Cleanup
        stall_detector.abort();
        for handle in worker_handles {
            handle.abort();
        }

        // Build result
        let state = self.state.read().await;
        let duration = state.started_at.elapsed();
        let total = state.completed_count + state.failed_count;
        let success_rate = if total > 0 {
            state.completed_count as f32 / total as f32
        } else {
            1.0
        };

        let blockers: Vec<String> = state.tasks.values()
            .filter(|t| t.status == TaskStatus::Blocked)
            .flat_map(|t| t.blocked_by.iter().cloned())
            .collect::<HashSet<_>>()
            .into_iter()
            .collect();

        let result = DroverResult {
            success: state.failed_count == 0 && blockers.is_empty(),
            duration,
            tasks_completed: state.completed_count,
            tasks_failed: state.failed_count,
            success_rate,
            blockers,
        };

        // Checkpoint completion
        self.store.complete_run(&self.run_id, &result).await?;

        Ok(result)
    }

    pub async fn run_with_dashboard(&mut self) -> anyhow::Result<DroverResult> {
        // For now, just run normally without TUI
        self.run().await
    }

    async fn handle_event(&mut self, event: WorkerEvent) -> anyhow::Result<()> {
        match event {
            WorkerEvent::TaskCompleted { task_id, duration } => {
                info!(task_id = %task_id, duration = ?duration, "Task completed");

                let mut state = self.state.write().await;
                if let Some(task) = state.tasks.get_mut(&task_id) {
                    task.status = TaskStatus::Completed;
                }
                state.completed_count += 1;
                state.last_progress = Instant::now();

                // Update Beads
                self.close_task(&task_id, "Completed by Drover").await?;

                // Check for unblocked tasks
                self.check_unblocked(&task_id).await?;
            }

            WorkerEvent::TaskFailed { task_id, error, retriable } => {
                warn!(task_id = %task_id, error = %error, "Task failed");

                let mut state = self.state.write().await;
                if let Some(task) = state.tasks.get_mut(&task_id) {
                    task.attempts += 1;
                    task.last_error = Some(error.clone());

                    if retriable && task.attempts < self.config.max_task_attempts {
                        task.status = TaskStatus::Ready;
                        info!(task_id = %task_id, attempt = task.attempts, "Requeueing");
                    } else {
                        task.status = TaskStatus::Failed;
                        state.failed_count += 1;
                        error!(task_id = %task_id, "Task permanently failed");
                    }
                }
            }

            WorkerEvent::TaskBlocked { task_id, blocked_by } => {
                warn!(task_id = %task_id, blocked_by = ?blocked_by, "Task blocked");

                let mut state = self.state.write().await;
                if let Some(task) = state.tasks.get_mut(&task_id) {
                    task.status = TaskStatus::Blocked;
                    task.blocked_by = blocked_by.clone();
                }
                drop(state);

                // Auto-unblock
                if self.config.auto_unblock {
                    for blocker in blocked_by {
                        self.create_unblock_task(&blocker).await?;
                    }
                }
            }

            WorkerEvent::Stalled { duration } => {
                warn!(duration = ?duration, "Progress stalled");
                self.handle_stall().await?;
            }
        }

        Ok(())
    }

    async fn is_complete(&self) -> bool {
        let state = self.state.read().await;
        state.tasks.values().all(|t| {
            matches!(t.status, TaskStatus::Completed | TaskStatus::Failed)
        })
    }

    async fn check_unblocked(&self, completed_id: &str) -> anyhow::Result<()> {
        let mut state = self.state.write().await;

        for task in state.tasks.values_mut() {
            if task.status == TaskStatus::Blocked {
                task.blocked_by.retain(|b| b != completed_id);
                if task.blocked_by.is_empty() {
                    task.status = TaskStatus::Ready;
                    info!(task_id = %task.id, "Task unblocked");
                }
            }
        }

        Ok(())
    }

    async fn create_unblock_task(&self, blocker: &str) -> anyhow::Result<()> {
        let mut state = self.state.write().await;

        if state.auto_created_tasks.contains(blocker) {
            return Ok(());
        }

        let task_id = format!("drover-fix-{}", &blocker[..8.min(blocker.len())]);

        let task = Task {
            id: task_id.clone(),
            title: format!("Fix: {}", blocker),
            description: Some(format!("Auto-created by Drover to unblock dependent tasks.\n\nBlocker: {}", blocker)),
            priority: 100,
            status: TaskStatus::Ready,
            parent_epic: None,
            blocked_by: vec![],
            labels: vec!["drover-auto".to_string()],
            attempts: 0,
            last_error: None,
        };

        state.tasks.insert(task_id.clone(), task);
        state.auto_created_tasks.insert(blocker.to_string());

        // Create in Beads
        drop(state);
        self.create_beads_task(&task_id, blocker).await?;

        info!(task_id = %task_id, blocker = %blocker, "Created unblock task");

        Ok(())
    }

    async fn create_beads_task(&self, _id: &str, title: &str) -> anyhow::Result<()> {
        Command::new("bd")
            .args(["new", title, "--json"])
            .current_dir(&self.config.project_dir)
            .output()
            .await?;
        Ok(())
    }

    async fn close_task(&self, task_id: &str, reason: &str) -> anyhow::Result<()> {
        Command::new("bd")
            .args(["close", task_id, "--reason", reason])
            .current_dir(&self.config.project_dir)
            .output()
            .await?;
        Ok(())
    }

    async fn handle_stall(&self) -> anyhow::Result<()> {
        let state = self.state.read().await;

        let blocked: Vec<_> = state.tasks.values()
            .filter(|t| t.status == TaskStatus::Blocked)
            .collect();

        let ready = state.tasks.values()
            .filter(|t| t.status == TaskStatus::Ready)
            .count();

        if !blocked.is_empty() && ready == 0 {
            warn!("All work blocked! {} tasks waiting on blockers", blocked.len());

            for task in blocked.iter().take(3) {
                warn!(task = %task.id, blocked_by = ?task.blocked_by, "Blocked task");
            }
        }

        Ok(())
    }

    fn spawn_stall_detector(&self) -> tokio::task::JoinHandle<()> {
        let state = Arc::clone(&self.state);
        let threshold = self.config.stall_threshold;

        tokio::spawn(async move {
            loop {
                tokio::time::sleep(Duration::from_secs(60)).await;

                let s = state.read().await;
                if s.last_progress.elapsed() > threshold {
                    warn!(elapsed = ?s.last_progress.elapsed(), "Stall detected");
                }
            }
        })
    }
}

// ============================================================================
// BEADS PARSING
// ============================================================================

#[derive(Debug, Deserialize)]
struct BeadItem {
    id: String,
    title: String,
    #[serde(default)]
    description: Option<String>,
    #[serde(default)]
    priority: i32,
    #[serde(default)]
    status: String,
    #[serde(default)]
    parent: Option<String>,
    #[serde(default)]
    blocked_by: Vec<String>,
    #[serde(default)]
    labels: Vec<String>,
    #[serde(default)]
    is_epic: Option<bool>,
}

impl BeadItem {
    fn into_task(self, parent_override: Option<String>) -> Task {
        let status = match self.status.as_str() {
            "open" => TaskStatus::Ready,
            "in-progress" => TaskStatus::InProgress,
            "blocked" => TaskStatus::Blocked,
            "closed" => TaskStatus::Completed,
            _ => TaskStatus::Ready,
        };

        Task {
            id: self.id,
            title: self.title,
            description: self.description,
            priority: self.priority,
            status,
            parent_epic: parent_override.or(self.parent),
            blocked_by: self.blocked_by,
            labels: self.labels,
            attempts: 0,
            last_error: None,
        }
    }
}

// ============================================================================
// HELPERS
// ============================================================================

struct TaskStats {
    ready: usize,
    blocked: usize,
    completed: usize,
}

fn calculate_stats(tasks: &[Task]) -> TaskStats {
    TaskStats {
        ready: tasks.iter().filter(|t| t.status == TaskStatus::Ready).count(),
        blocked: tasks.iter().filter(|t| t.status == TaskStatus::Blocked).count(),
        completed: tasks.iter().filter(|t| t.status == TaskStatus::Completed).count(),
    }
}

fn calculate_stats_from_refs(tasks: &[&Task]) -> TaskStats {
    TaskStats {
        ready: tasks.iter().filter(|t| t.status == TaskStatus::Ready).count(),
        blocked: tasks.iter().filter(|t| t.status == TaskStatus::Blocked).count(),
        completed: tasks.iter().filter(|t| t.status == TaskStatus::Completed).count(),
    }
}

fn calculate_progress(tasks: &[Task]) -> f32 {
    if tasks.is_empty() {
        return 100.0;
    }
    let completed = tasks.iter().filter(|t| t.status == TaskStatus::Completed).count();
    (completed as f32 / tasks.len() as f32) * 100.0
}
