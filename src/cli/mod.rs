//! CLI command handlers

pub mod muster;
pub mod resume;
pub mod run;
pub mod status;

use clap::Subcommand;
use std::path::PathBuf;

#[derive(Subcommand, Debug)]
pub enum Commands {
    /// Muster the herd - list all work
    Muster(muster::MusterArgs),

    /// Check project status
    Status(status::StatusArgs),

    /// Drive tasks to completion
    Run(run::RunArgs),

    /// Resume interrupted runs
    Resume(resume::ResumeArgs),
}

pub async fn handle_command(cmd: Commands) -> anyhow::Result<()> {
    match cmd {
        Commands::Muster(args) => muster::execute(args).await,
        Commands::Status(args) => status::execute(args).await,
        Commands::Run(args) => run::execute(args).await,
        Commands::Resume(args) => resume::execute(args).await,
    }
}

fn find_project_dir() -> anyhow::Result<PathBuf> {
    let current = std::env::current_dir()?;

    // Look for .drover.toml or .beads directory
    for ancestor in current.ancestors() {
        let drover_config = ancestor.join(".drover.toml");
        let beads_dir = ancestor.join(".beads");

        if drover_config.exists() || beads_dir.exists() {
            return Ok(PathBuf::from(ancestor));
        }
    }

    // Default to current directory
    Ok(current)
}
