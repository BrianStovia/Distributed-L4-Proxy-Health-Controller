mod balancer;
mod controller_client;
mod proxy;

use balancer::LoadBalancer;
use proxy::ProxyMetrics;
use std::net::SocketAddr;
use std::sync::Arc;
use tracing::info;
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Initialize structured logging
    tracing_subscriber::registry()
        .with(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info,dataplane=debug".into()),
        )
        .with(tracing_subscriber::fmt::layer())
        .init();

    info!("Starting Rust Data Plane Proxy Engine...");

    let listen_addr_str =
        std::env::var("PROXY_LISTEN_ADDR").unwrap_or_else(|_| "127.0.0.1:8080".into());
    let listen_addr: SocketAddr = listen_addr_str.parse()?;

    let controller_addr =
        std::env::var("CONTROLLER_ADDR").unwrap_or_else(|_| "http://127.0.0.1:50051".into());
    let node_id = std::env::var("PROXY_NODE_ID").unwrap_or_else(|_| "node-edge-01".into());

    let balancer = LoadBalancer::new();
    let metrics = Arc::new(ProxyMetrics::new());

    // Launch background worker for dynamic configuration synchronization from Go Control Plane
    let balancer_clone = balancer.clone();
    let controller_addr_clone = controller_addr.clone();
    let node_id_clone = node_id.clone();
    tokio::spawn(async move {
        controller_client::start_config_watcher(
            controller_addr_clone,
            balancer_clone,
            node_id_clone,
        )
        .await;
    });

    // Launch background worker for periodic telemetry reporting to Go Control Plane
    let metrics_clone = metrics.clone();
    tokio::spawn(async move {
        controller_client::start_metrics_reporter(controller_addr, metrics_clone, node_id).await;
    });

    // Run primary TCP proxy engine
    proxy::run_proxy(listen_addr, balancer, metrics).await?;

    Ok(())
}
