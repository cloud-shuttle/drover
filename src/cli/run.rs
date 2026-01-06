//! `drover run` command - Drive tasks to completion

use clap::Parser;
use std::path::PathBuf;

use crate::config::{load_config, Config, DroverConfig};
use crate::drover::{self, Drover};
use crate::durable::DurableStore;
use crate::cli::find_project_dir;

#[derive(Parser, Debug)]
pub struct RunArgs {
    /// Filter by epic ID
    #[arg(short, long)]
    epic: Option<String>,

    /// Number of workers
    #[arg(short, long)]
    workers: Option<usize>,

    /// Task timeout in seconds
    #[arg(short, long)]
    timeout: Option<u64>,

    /// Max retry attempts
    #[arg(short, long)]
    retries: Option<u32>,

    /// Task limit (for testing)
    #[arg(long)]
    limit: Option<usize>,

    /// Dry run - show what would be done
    #[arg(long)]
    dry_run: bool,

    /// Enable TUI dashboard
    #[arg(long)]
    dashboard: bool,

    /// Project directory
    #[arg(short, long)]
    project_dir: Option<PathBuf>,
}

pub async fn execute(args: RunArgs) -> anyhow::Result<()> {
    let project_dir = match args.project_dir {
        Some(dir) => dir,
        None => find_project_dir()?,
    };

    // Load and merge config
    let mut config = load_config(&project_dir)?;
    if let Some(workers) = args.workers {
        config.workers = workers;
    }
    if let Some(timeout) = args.timeout {
        config.timeout = timeout;
    }
    if let Some(retries) = args.retries {
        config.retries = retries;
    }

    // Discover work
    let manifest = drover::discover_work(&project_dir, args.epic.as_deref()).await?;

    if manifest.total_tasks == 0 {
        println!("No work found!");
        return Ok(());
    }

    println!("🐂 Drover - {}", manifest.target_description());
    println!();
    println!("Target: {}", manifest.target_description());
    println!("Tasks: {} total, {} ready, {} blocked",
        manifest.total_tasks,
        manifest.ready_tasks,
        manifest.blocked_tasks
    );
    println!("Workers: {}", config.workers);
    println!();

    if args.dry_run {
        println!("Dry run - not executing tasks");
        return Ok(());
    }

    // Initialize durable store
    let store = DurableStore::connect(&config.database).await?;
    store.init().await?;

    // Build drover config
    let drover_config: DroverConfig = config.into();
    let drover_config = drover_config
        .with_project_dir(project_dir.clone())
        .with_task_limit(args.limit);

    // Create and run drover
    let mut drover = Drover::new(manifest, drover_config, store).await?;

    let result = if args.dashboard {
        drover.run_with_dashboard().await?
    } else {
        drover.run().await?
    };

    // Print results
    print_results(&result);

    Ok(())
}

fn print_results(result: &drover::DroverResult) {
    println!();
    if result.success {
        println!("✅ SUCCESS");
    } else {
        println!("⚠️  COMPLETED WITH ISSUES");
    }

    println!("{}", "━".repeat(60));
    println!("Duration:        {:?}", result.duration);
    println!("Tasks completed: {}", result.tasks_completed);
    println!("Tasks failed:    {}", result.tasks_failed);
    println!("Success rate:    {:.1}%", result.success_rate * 100.0);

    if !result.blockers.is_empty() {
        println!();
        println!("Remaining blockers:");
        for blocker in &result.blockers {
            println!("  - {}", blocker);
        }
    }
}
