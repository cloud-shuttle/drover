//! `drover status` command - Check project status

use clap::Parser;
use crate::drover;
use crate::cli::find_project_dir;

#[derive(Parser, Debug)]
pub struct StatusArgs {
    /// Filter by epic ID
    #[arg(short, long)]
    epic: Option<String>,

    /// Watch mode (live updates)
    #[arg(short, long)]
    watch: bool,
}

pub async fn execute(args: StatusArgs) -> anyhow::Result<()> {
    let project_dir = find_project_dir()?;

    if args.watch {
        loop {
            print!("\x1b[2J\x1b[H"); // Clear screen
            show_status(&project_dir, args.epic.as_deref()).await?;
            tokio::time::sleep(tokio::time::Duration::from_secs(2)).await;
        }
    } else {
        show_status(&project_dir, args.epic.as_deref()).await?;
    }

    Ok(())
}

async fn show_status(project_dir: &std::path::PathBuf, epic: Option<&str>) -> anyhow::Result<()> {
    let status = drover::get_project_status(project_dir, epic).await?;

    println!("🐂 Drover Status\n");
    println!("  Total:   {}", status.total);
    println!("  Ready:   {}", status.ready);
    println!("  Blocked: {}", status.blocked);
    println!("  Failed:  {}", status.failed);
    println!();

    // Progress bar
    let bar_len = 40;
    let filled = (status.progress / 100.0 * bar_len as f32) as usize;
    let empty = bar_len - filled;

    println!("  Progress:");
    print!("    [");
    print!("{}", "█".repeat(filled));
    print!("{}", "░".repeat(empty));
    println!("] {:.1}%", status.progress);

    if let Some(elapsed) = status.elapsed {
        println!("\n  Elapsed: {:?}", elapsed);
    }

    if let Some(eta) = status.eta {
        println!("  ETA: {:?}", eta);
    }

    Ok(())
}
