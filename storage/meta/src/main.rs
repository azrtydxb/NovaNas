//! novanas-meta entrypoint.

use std::path::PathBuf;
use std::time::Duration;

use anyhow::{Context, Result};
use clap::Parser;
use tokio::net::{TcpListener, UnixListener};
use tokio_stream::wrappers::{TcpListenerStream, UnixListenerStream};
use tonic::transport::Server;
use tracing::info;

use novanas_meta::api_client::{ApiSubscriber, ApiSubscriberConfig};
use novanas_meta::policy::{PolicyChecker, PolicyConfig};
use novanas_meta::server::MetaServer;
use novanas_meta::store::Store;

#[derive(Debug, Parser)]
#[command(name = "novanas-meta", about = "NovaNAS metadata daemon")]
struct Args {
    /// Persistent state directory.
    #[arg(long, default_value = "/var/lib/novanas-meta")]
    data_dir: PathBuf,

    /// Path to the Unix socket the gRPC server listens on.
    #[arg(long, default_value = "/var/run/novanas/meta.sock")]
    listen_uds: PathBuf,

    /// Optional TCP listen address (e.g. 127.0.0.1:7777). Useful for tests.
    #[arg(long)]
    listen_tcp: Option<String>,

    /// Base URL of `novanas-api`.
    #[arg(long, default_value = "http://localhost:3000")]
    api_url: String,

    /// API poll interval, in seconds.
    #[arg(long, default_value_t = 5)]
    api_poll_secs: u64,

    /// Policy check interval, in seconds.
    #[arg(long, default_value_t = 30)]
    policy_secs: u64,

    /// Local node name. Single-host architecture, but disks carry a node
    /// label for symmetry with the API schema.
    #[arg(long, default_value = "local")]
    node_name: String,
}

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info,novanas_meta=debug".into()),
        )
        .init();

    let args = Args::parse();

    std::fs::create_dir_all(&args.data_dir)
        .with_context(|| format!("create data dir {}", args.data_dir.display()))?;
    let store_path = args.data_dir.join("meta.redb");
    let store = Store::open(&store_path)
        .with_context(|| format!("open redb at {}", store_path.display()))?;
    info!(path = %store_path.display(), "store opened");

    // ----- gRPC server -------------------------------------------------
    let svc = MetaServer::new(store.clone()).into_service();

    let server_handle: tokio::task::JoinHandle<Result<()>> = if let Some(addr) = &args.listen_tcp {
        let listener = TcpListener::bind(addr)
            .await
            .with_context(|| format!("bind tcp {addr}"))?;
        info!(addr = %addr, "gRPC listening (tcp)");
        let stream = TcpListenerStream::new(listener);
        tokio::spawn(async move {
            Server::builder()
                .add_service(svc)
                .serve_with_incoming(stream)
                .await
                .context("grpc server (tcp) crashed")
        })
    } else {
        if let Some(parent) = args.listen_uds.parent() {
            std::fs::create_dir_all(parent)
                .with_context(|| format!("create uds parent dir {}", parent.display()))?;
        }
        let _ = std::fs::remove_file(&args.listen_uds);
        let uds = UnixListener::bind(&args.listen_uds)
            .with_context(|| format!("bind uds {}", args.listen_uds.display()))?;
        info!(path = %args.listen_uds.display(), "gRPC listening (uds)");
        let stream = UnixListenerStream::new(uds);
        tokio::spawn(async move {
            Server::builder()
                .add_service(svc)
                .serve_with_incoming(stream)
                .await
                .context("grpc server (uds) crashed")
        })
    };

    // ----- API subscriber ---------------------------------------------
    let api_cfg = ApiSubscriberConfig {
        base_url: args.api_url,
        poll_interval: Duration::from_secs(args.api_poll_secs),
        node_name: args.node_name,
    };
    let api = ApiSubscriber::new(api_cfg, store.clone()).context("build api subscriber")?;
    let api_handle = tokio::spawn(async move {
        if let Err(e) = api.run().await {
            tracing::error!(error = %e, "api subscriber exited");
        }
    });

    // ----- Policy loop ------------------------------------------------
    let policy = PolicyChecker::new(
        PolicyConfig {
            interval: Duration::from_secs(args.policy_secs),
        },
        store.clone(),
    );
    let policy_handle = tokio::spawn(async move {
        if let Err(e) = policy.run().await {
            tracing::error!(error = %e, "policy checker exited");
        }
    });

    // Block on the gRPC server task; it should run forever.
    match server_handle.await {
        Ok(Ok(())) => {}
        Ok(Err(e)) => return Err(e),
        Err(e) => return Err(anyhow::anyhow!("server join error: {e}")),
    }
    api_handle.abort();
    policy_handle.abort();
    Ok(())
}
