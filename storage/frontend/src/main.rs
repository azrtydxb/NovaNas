//! novanas-frontend daemon entrypoint.
//!
//! Without `--features spdk-sys` the daemon does NOT serve I/O — it
//! validates configuration, builds the meta client + NDP client +
//! chunk-map cache, runs one API reconciliation pass, and exits. This
//! mirrors the dataplane's pattern and lets CI / dev boxes exercise the
//! control surface without SPDK linkage.
//!
//! With `--features spdk-sys` the daemon runs the SPDK reactor (today
//! still a placeholder per `spdk/env.rs` — see Agent C's PR).

use std::sync::Arc;
use std::time::Duration;

use clap::Parser;

use novanas_frontend::api_subscriber::{ApiSubscriber, HttpBlockVolumeSource};
use novanas_frontend::chunk_engine::ChunkEngine;
use novanas_frontend::chunk_map_cache::ChunkMapCache;
use novanas_frontend::error::{FrontendError, Result};
use novanas_frontend::meta_client::{MetaClient, UdsMetaClient};
use novanas_frontend::ndp_client::{NdpChunkClient, UdsNdpClient};
use novanas_frontend::nvmf::{NoopNvmfTarget, NvmfTarget};
use novanas_frontend::reconciler::VolumeReconciler;
use novanas_frontend::volume_bdev::{NoopVolumeBdevManager, VolumeBdevManager};

#[derive(Parser, Debug)]
#[command(name = "novanas-frontend", about = "NovaNas frontend daemon")]
struct Args {
    /// Path to the meta gRPC UDS.
    #[arg(long, default_value = "/var/run/novanas/meta.sock")]
    meta_socket: String,

    /// Path to the data NDP UDS.
    #[arg(long, default_value = "/var/run/novanas/ndp.sock")]
    ndp_socket: String,

    /// Base URL of the API server.
    #[arg(long, default_value = "http://localhost:3000")]
    api_url: String,

    /// Polling interval against the API server.
    #[arg(long, default_value_t = 5)]
    api_poll_secs: u64,

    /// SPDK reactor mask (only used with --features spdk-sys).
    #[arg(long, default_value = "0x1")]
    reactor_mask: String,

    /// SPDK hugepage memory in MB.
    #[arg(long, default_value_t = 2048)]
    mem_size_mb: u32,

    /// NVMe-oF listen address.
    #[arg(long, default_value = "0.0.0.0")]
    listen_address: String,

    /// NVMe-oF listen port.
    #[arg(long, default_value_t = 4420)]
    listen_port: u16,

    /// Optional path to a redb file for the chunk-map cache.
    #[arg(long)]
    chunk_map_cache: Option<String>,
}

fn build_runtime() -> Result<tokio::runtime::Runtime> {
    tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .map_err(|e| FrontendError::Config(format!("tokio runtime: {}", e)))
}

async fn build_components(
    args: &Args,
) -> Result<(
    Arc<dyn MetaClient>,
    Arc<dyn NdpChunkClient>,
    Arc<ChunkMapCache>,
    Arc<ChunkEngine>,
    Arc<dyn VolumeBdevManager>,
    Arc<dyn NvmfTarget>,
)> {
    log::info!("connecting to meta at {}", args.meta_socket);
    let meta: Arc<dyn MetaClient> = Arc::new(UdsMetaClient::connect(&args.meta_socket).await?);

    log::info!("opening NDP client to {}", args.ndp_socket);
    let ndp: Arc<dyn NdpChunkClient> = Arc::new(UdsNdpClient::new(&args.ndp_socket));

    let cache = match args.chunk_map_cache.as_ref() {
        Some(path) => Arc::new(ChunkMapCache::open(path)?),
        None => Arc::new(ChunkMapCache::in_memory()),
    };
    cache.set_meta(meta.clone()).await;

    let engine = Arc::new(ChunkEngine::new(cache.clone(), ndp.clone()));

    // Choose bdev + nvmf implementation based on the SPDK feature flag.
    #[cfg(feature = "spdk-sys")]
    let (bdev_mgr, nvmf): (Arc<dyn VolumeBdevManager>, Arc<dyn NvmfTarget>) = (
        Arc::new(novanas_frontend::spdk::volume_bdev::SpdkVolumeBdevManager::new(engine.clone())),
        Arc::new(novanas_frontend::spdk_nvmf::SpdkNvmfTarget::new(
            args.listen_address.clone(),
            args.listen_port,
        )),
    );
    #[cfg(not(feature = "spdk-sys"))]
    let (bdev_mgr, nvmf): (Arc<dyn VolumeBdevManager>, Arc<dyn NvmfTarget>) = (
        Arc::new(NoopVolumeBdevManager::new(engine.clone())),
        Arc::new(NoopNvmfTarget::new()),
    );

    Ok((meta, ndp, cache, engine, bdev_mgr, nvmf))
}

fn main() -> Result<()> {
    env_logger::init();
    let args = Args::parse();
    log::info!("novanas-frontend starting; args = {:?}", args);

    let rt = build_runtime()?;
    rt.block_on(async move {
        let (meta, _ndp, _cache, _engine, bdev_mgr, nvmf) = match build_components(&args).await {
            Ok(c) => c,
            Err(e) => {
                log::error!("failed to build components: {}", e);
                return Err::<(), _>(e);
            }
        };

        let reconciler = Arc::new(VolumeReconciler::new(
            meta,
            bdev_mgr,
            nvmf,
            args.listen_address.clone(),
            args.listen_port,
        ));
        let source = Arc::new(HttpBlockVolumeSource::new(args.api_url.clone()));
        let subscriber = Arc::new(ApiSubscriber::new(
            source,
            reconciler,
            Duration::from_secs(args.api_poll_secs),
        ));

        #[cfg(feature = "spdk-sys")]
        {
            log::info!(
                "spawning subscriber and entering SPDK reactor on mask={} mem={}MB",
                args.reactor_mask,
                args.mem_size_mb
            );
            let sub = subscriber.clone();
            tokio::spawn(async move { sub.run().await });
            // SPDK reactor blocks the current thread.
            let res = tokio::task::spawn_blocking({
                let mask = args.reactor_mask.clone();
                let mem = args.mem_size_mb;
                let port = args.listen_port;
                move || novanas_frontend::spdk::run(&mask, mem, port)
            })
            .await
            .map_err(|e| FrontendError::SpdkInit(format!("reactor join: {}", e)))?;
            res
        }
        #[cfg(not(feature = "spdk-sys"))]
        {
            log::info!(
                "spdk-sys feature is OFF; running one reconcile pass and exiting (control-only)"
            );
            let mut known = std::collections::HashSet::new();
            if let Err(e) = subscriber.tick(&mut known).await {
                log::warn!("reconcile pass failed: {}", e);
            }
            log::info!("frontend daemon completed control-only run");
            Ok::<(), FrontendError>(())
        }
    })?;
    Ok(())
}
