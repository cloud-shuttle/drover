//! Drover - Drive your project to completion with parallel AI agents
//!
//! Drover orchestrates task execution across multiple AI workers,
//! providing durable execution, progress tracking, and auto-unblocking.

pub mod cli;
pub mod config;
pub mod dashboard;
pub mod durable;
pub mod drover;
pub mod workers;

// Re-export commonly used types
pub use config::{Config, DroverConfig};
pub use drover::{Epic, ProjectStatus, Task, TaskStatus, WorkManifest};
pub use workers::WorkerEvent;
