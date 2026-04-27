//! `novanas-dataplane` — binary entry point for the data daemon
//! (architecture-v2 role `novanas-data`).
//!
//! Startup order:
//!   1. tokio multi-thread runtime + tracing.
//!   2. SPDK env init under `feature = "spdk-sys"`.
//!   3. First disk discovery scan.
//!   4. Connect to `novanas-meta` over UDS.
//!   5. First heartbeat; capture the desired CRUSH digest meta returns.
//!   6. Bind the NDP UDS server (`/var/run/novanas/ndp.sock`).
//!   7. Start the task runner (long-poll → dispatch → ack).
//!   8. Start the heartbeat + re-discovery timers (30s).
//!
//! On SIGINT/SIGTERM the runner is asked to drain, the SPDK reactor is
//! stopped, and the runtime is shut down.

use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use clap::Parser;
use tracing::{info, warn};

#[derive(Parser, Debug)]
#[command(name = "novanas-dataplane", version, about)]
struct Args {
    /// SPDK reactor CPU mask (hex)
    #[arg(long, default_value = "0x1")]
    reactor_mask: String,

    /// Hugepage memory in MB
    #[arg(long, default_value_t = 2048)]
    mem_size: u32,

    /// Path to the meta daemon's UDS.
    #[arg(long, default_value = novanas_dataplane::meta_client::DEFAULT_META_SOCKET)]
    meta_socket: PathBuf,

    /// Path the data daemon's NDP UDS server should bind.
    #[arg(long, default_value = novanas_dataplane::transport::ndp_server::NDP_UNIX_SOCKET)]
    ndp_socket: PathBuf,

    /// Stable node identifier. Defaults to `$HOSTNAME` when set.
    #[arg(long)]
    node_id: Option<String>,

    /// Sysfs root for disk discovery (override for tests / sandboxes).
    #[arg(long, default_value = "/sys")]
    sysfs_root: PathBuf,

    /// Log level
    #[arg(long, default_value = "info")]
    log_level: String,
}

const HEARTBEAT_INTERVAL: Duration = Duration::from_secs(30);
const DISCOVERY_INTERVAL: Duration = Duration::from_secs(30);

fn main() {
    let args = Args::parse();

    let runtime = tokio::runtime::Builder::new_multi_thread()
        .worker_threads(8)
        .enable_all()
        .build()
        .expect("failed to create tokio runtime");

    let _rt_guard = runtime.enter();
    novanas_dataplane::tracing_init::init_tracing("novanas-dataplane", &args.log_level);
    novanas_dataplane::set_tokio_handle(runtime.handle().clone());

    info!(
        "novanas-data starting (meta={}, ndp={}, sysfs={})",
        args.meta_socket.display(),
        args.ndp_socket.display(),
        args.sysfs_root.display()
    );

    let node_id = args
        .node_id
        .clone()
        .or_else(|| std::env::var("HOSTNAME").ok())
        .unwrap_or_else(|| "novanas-data".to_string());

    let shutdown = novanas_dataplane::task_runner::ShutdownToken::new();
    let shutdown_for_signals = shutdown.clone();
    runtime.spawn(async move {
        let _ = tokio::signal::ctrl_c().await;
        info!("SIGINT received — initiating shutdown");
        shutdown_for_signals.cancel();
    });

    // Bring up the meta-side pipeline.
    if let Err(e) = runtime.block_on(start_meta_pipeline(&args, &node_id, shutdown.clone())) {
        warn!("meta pipeline did not fully start: {e}");
        // Continue running so the NDP server stays up — operators can
        // restart the daemon once meta is reachable.
    }

    #[cfg(feature = "spdk-sys")]
    {
        use log::error;
        use novanas_dataplane::config::DataPlaneConfig;

        let config = DataPlaneConfig {
            rpc_socket: String::new(),
            reactor_mask: args.reactor_mask.clone(),
            mem_size: args.mem_size,
            transport_type: String::new(),
            listen_address: String::new(),
            listen_port: 0,
            grpc_port: 0,
            tls_ca_cert: String::new(),
            tls_server_cert: String::new(),
            tls_server_key: String::new(),
        };

        if let Err(e) = novanas_dataplane::spdk::run(config) {
            error!("data plane SPDK reactor failed: {}", e);
            std::process::exit(1);
        }
    }

    #[cfg(not(feature = "spdk-sys"))]
    {
        info!("SPDK not available (spdk-sys feature not enabled). Running tokio-only loop.");
        let _ = (args.reactor_mask, args.mem_size);
        runtime.block_on(async {
            shutdown.cancelled().await;
        });
    }

    novanas_dataplane::chunk::sync::destage_all_bitmaps();
    novanas_dataplane::tracing_init::shutdown_tracing();
    runtime.shutdown_background();
    info!("novanas-data stopped");
}

async fn start_meta_pipeline(
    args: &Args,
    node_id: &str,
    shutdown: novanas_dataplane::task_runner::ShutdownToken,
) -> std::result::Result<(), Box<dyn std::error::Error + Send + Sync>> {
    use novanas_dataplane::disk_discovery;
    use novanas_dataplane::meta_client::{MetaClient, MetaClientConfig};
    use novanas_dataplane::task_handlers::HandlerContext;
    use novanas_dataplane::task_runner::{TaskRunner, TaskRunnerConfig};

    // 1. Initial disk discovery.
    let block_root = args.sysfs_root.join("block");
    let initial = disk_discovery::discover_in(&block_root).unwrap_or_default();
    info!(
        "disk_discovery: found {} local disks at startup",
        initial.len()
    );

    // 2. Meta client.
    let cfg = MetaClientConfig {
        socket_path: args.meta_socket.clone(),
        ..Default::default()
    };
    let mut client = MetaClient::connect(cfg).await?;

    // 3. First heartbeat.
    let disks: Vec<_> = initial.iter().map(|d| d.to_disk()).collect();
    match client
        .heartbeat(node_id, env!("CARGO_PKG_VERSION"), disks)
        .await
    {
        Ok(resp) => {
            info!(
                "heartbeat: meta acked, pending tasks={}",
                resp.pending_task_count
            );
        }
        Err(e) => warn!("heartbeat failed at startup: {e}"),
    }

    // 4. Heartbeat / discovery timer.
    let hb_node = node_id.to_string();
    let hb_root = args.sysfs_root.clone();
    let hb_shutdown = shutdown.clone();
    let hb_socket = args.meta_socket.clone();
    tokio::spawn(async move {
        let mut hb_interval = tokio::time::interval(HEARTBEAT_INTERVAL);
        let mut disc_interval = tokio::time::interval(DISCOVERY_INTERVAL);
        let mut hb_client_opt = None;
        loop {
            tokio::select! {
                _ = hb_shutdown.cancelled() => break,
                _ = hb_interval.tick() => {
                    if hb_client_opt.is_none() {
                        match MetaClient::connect(MetaClientConfig { socket_path: hb_socket.clone(), ..Default::default() }).await {
                            Ok(c) => hb_client_opt = Some(c),
                            Err(e) => { warn!("heartbeat: reconnect failed: {e}"); continue; }
                        }
                    }
                    let infos = disk_discovery::discover_in(&hb_root.join("block")).unwrap_or_default();
                    let disks: Vec<_> = infos.iter().map(|d| d.to_disk()).collect();
                    if let Some(c) = hb_client_opt.as_mut() {
                        if let Err(e) = c.heartbeat(&hb_node, env!("CARGO_PKG_VERSION"), disks).await {
                            warn!("heartbeat: failed: {e} (will reconnect)");
                            hb_client_opt = None;
                        }
                    }
                }
                _ = disc_interval.tick() => {
                    let n = disk_discovery::discover_in(&hb_root.join("block")).map(|v| v.len()).unwrap_or(0);
                    log::debug!("disk_discovery: re-scan found {n} disks");
                }
            }
        }
        info!("heartbeat / discovery loop stopped");
    });

    // 5. NDP UDS server: a chunk store will be registered as disks claim;
    //    until then we serve no chunks but keep the socket bound so the
    //    frontend can connect.
    let ndp_cfg = novanas_dataplane::transport::ndp_server::NdpServerConfig {
        unix_socket: args.ndp_socket.clone(),
    };
    let placeholder_store = Arc::new(novanas_dataplane::backend::chunk_store::NullChunkStore);
    if let Err(e) =
        novanas_dataplane::transport::ndp_server::start_ndp_server(ndp_cfg, placeholder_store).await
    {
        warn!("NDP UDS server failed to start: {e}");
    }

    // 6. Task runner.
    let handler_ctx =
        Arc::new(HandlerContext::new(node_id).with_sysfs_root(args.sysfs_root.clone()));
    let runner_cfg = TaskRunnerConfig {
        node_id: node_id.to_string(),
        ..Default::default()
    };
    let runner = TaskRunner::new(runner_cfg, handler_ctx);
    tokio::spawn(async move {
        if let Err(e) = runner.run(client, shutdown.clone()).await {
            warn!("task_runner exited with error: {e}");
        }
    });

    Ok(())
}
