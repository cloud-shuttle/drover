//! `drover resume` command - Resume interrupted runs

use clap::Parser;
use crate::cli::find_project_dir;
use crate::durable::DurableStore;
use crate::config::load_config;

#[derive(Parser, Debug)]
pub struct ResumeArgs {
    /// Run ID to resume (omit to list)
    run_id: Option<String>,
}

pub async fn execute(args: ResumeArgs) -> anyhow::Result<()> {
    let project_dir = find_project_dir()?;
    let config = load_config(&project_dir)?;

    let store = DurableStore::connect(&config.database).await?;
    store.init().await?;

    let run_id = if let Some(id) = args.run_id {
        uuid::Uuid::parse_str(&id)?
    } else {
        // List available runs
        list_runs(&store).await?;
        return Ok(());
    };

    // Get the run
    let run = store.get_run(&run_id).await?
        .ok_or_else(|| anyhow::anyhow!("Run not found"))?;

    if run.completed_at.is_some() {
        println!("Run already completed");
        return Ok(());
    }

    println!("Resuming run {} started at {}", run_id, run.started_at);
    println!("Note: Full resume not yet implemented");
    println!("Run a new `drover run` to continue processing tasks");

    Ok(())
}

async fn list_runs(store: &DurableStore) -> anyhow::Result<()> {
    let runs = store.list_runs().await?;

    if runs.is_empty() {
        println!("No runs found");
        return Ok(());
    }

    println!("Recent runs:\n");

    for run in runs.iter().take(10) {
        let status = if let Some(completed) = run.completed_at {
            if run.success.unwrap_or(false) {
                "✓ Success"
            } else {
                "✗ Failed"
            }
        } else {
            "◐ In Progress"
        };

        println!("  {} - {} ({} completed, {} failed)",
            run.id,
            status,
            run.tasks_completed,
            run.tasks_failed
        );
    }

    Ok(())
}
