//! `drover muster` command - List all work

use clap::Parser;
use crate::drover;
use crate::cli::find_project_dir;

#[derive(Parser, Debug)]
pub struct MusterArgs {
    /// Show only ready tasks
    #[arg(short, long)]
    ready: bool,

    /// Show only blocked tasks
    #[arg(short, long)]
    blocked: bool,

    /// Show task dependencies
    #[arg(short, long)]
    deps: bool,

    /// Output JSON
    #[arg(long)]
    json: bool,

    /// Filter by epic ID
    #[arg(short, long)]
    epic: Option<String>,
}

pub async fn execute(args: MusterArgs) -> anyhow::Result<()> {
    let project_dir = find_project_dir()?;
    let manifest = drover::discover_work(&project_dir, args.epic.as_deref()).await?;

    if args.json {
        println!("{}", serde_json::to_string_pretty(&manifest)?);
        return Ok(());
    }

    println!("🐂 Mustering the herd...\n");

    // Group tasks by epic
    for epic in &manifest.epics {
        println!("📦 {} ({:.0}%)", epic.title, epic.progress);

        for task in &epic.tasks {
            if args.ready && task.status != crate::drover::TaskStatus::Ready {
                continue;
            }
            if args.blocked && task.status != crate::drover::TaskStatus::Blocked {
                continue;
            }

            print_task(task, args.deps);
        }
        println!();
    }

    // Standalone tasks
    if !manifest.standalone_tasks.is_empty() {
        println!("📝 Standalone Tasks");

        for task in &manifest.standalone_tasks {
            if args.ready && task.status != crate::drover::TaskStatus::Ready {
                continue;
            }
            if args.blocked && task.status != crate::drover::TaskStatus::Blocked {
                continue;
            }

            print_task(task, args.deps);
        }
        println!();
    }

    // Summary
    println!("Summary: {} ready, {} blocked, {} done",
        manifest.ready_tasks,
        manifest.blocked_tasks,
        manifest.completed_tasks
    );

    Ok(())
}

fn print_task(task: &crate::drover::Task, show_deps: bool) {
    let status_symbol = match task.status {
        crate::drover::TaskStatus::Ready => "○",
        crate::drover::TaskStatus::Claimed => "◐",
        crate::drover::TaskStatus::InProgress => "◑",
        crate::drover::TaskStatus::Blocked => "⊘",
        crate::drover::TaskStatus::Completed => "✓",
        crate::drover::TaskStatus::Failed => "✗",
    };

    let status_color = match task.status {
        crate::drover::TaskStatus::Ready => "\x1b[36m",  // Cyan
        crate::drover::TaskStatus::Blocked => "\x1b[33m", // Yellow
        crate::drover::TaskStatus::Completed => "\x1b[32m", // Green
        crate::drover::TaskStatus::Failed => "\x1b[31m",   // Red
        _ => "\x1b[0m",
    };

    println!("  {}[{}]{} \x1b[0m{}",
        status_color,
        status_symbol,
        "\x1b[0m",
        task.title
    );

    if show_deps && !task.blocked_by.is_empty() {
        println!("      └─ blocked by: {:?}", task.blocked_by);
    }
}
