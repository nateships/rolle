//! Rolle daemon. Serves the CLI and desktop app over a local socket.

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    println!("rolled {}", env!("CARGO_PKG_VERSION"));
    Ok(())
}
