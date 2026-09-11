//! Rolle CLI. Thin client for the daemon.

use clap::{Parser, Subcommand};

#[derive(Parser)]
#[command(name = "rolle", version, about = "Assume any role, any cloud")]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Show daemon and session status.
    Status,
}

fn main() -> anyhow::Result<()> {
    let cli = Cli::parse();
    match cli.command {
        Command::Status => println!("rolle {}: daemon not running", env!("CARGO_PKG_VERSION")),
    }
    Ok(())
}
