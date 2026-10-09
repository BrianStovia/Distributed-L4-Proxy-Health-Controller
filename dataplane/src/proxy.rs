use crate::balancer::LoadBalancer;
use std::net::SocketAddr;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use tokio::io::copy_bidirectional;
use tokio::net::{TcpListener, TcpStream};
use tracing::{error, info, warn};

pub struct ProxyMetrics {
    pub total_connections: Arc<AtomicU64>,
    pub active_connections: Arc<AtomicU64>,
}

impl ProxyMetrics {
    pub fn new() -> Self {
        Self {
            total_connections: Arc::new(AtomicU64::new(0)),
            active_connections: Arc::new(AtomicU64::new(0)),
        }
    }
}

/// Runs the TCP proxy listener on the designated socket address (e.g., 0.0.0.0:8080).
pub async fn run_proxy(
    listen_addr: SocketAddr,
    balancer: LoadBalancer,
    metrics: Arc<ProxyMetrics>,
) -> anyhow::Result<()> {
    let listener = TcpListener::bind(listen_addr).await?;
    info!(
        "Data plane TCP proxy actively listening on: {}",
        listen_addr
    );

    loop {
        match listener.accept().await {
            Ok((client_stream, client_addr)) => {
                metrics.total_connections.fetch_add(1, Ordering::Relaxed);
                metrics.active_connections.fetch_add(1, Ordering::Relaxed);

                let balancer_clone = balancer.clone();
                let metrics_clone = metrics.clone();

                tokio::spawn(async move {
                    handle_client(client_stream, client_addr, balancer_clone).await;
                    metrics_clone
                        .active_connections
                        .fetch_sub(1, Ordering::Relaxed);
                });
            }
            Err(err) => {
                error!("Failed to accept incoming TCP connection: {:?}", err);
            }
        }
    }
}

async fn handle_client(
    mut client_stream: TcpStream,
    client_addr: SocketAddr,
    balancer: LoadBalancer,
) {
    let backend = match balancer.select_backend().await {
        Some(target) => target,
        None => {
            warn!(
                client = %client_addr,
                "No healthy backend targets available to service client request"
            );
            return;
        }
    };

    match TcpStream::connect(&backend.address).await {
        Ok(mut backend_stream) => {
            // Forward bidirectional stream data efficiently between client and backend
            if let Err(err) = copy_bidirectional(&mut client_stream, &mut backend_stream).await {
                tracing::debug!(
                    backend = %backend.address,
                    client = %client_addr,
                    "Proxy connection completed or closed: {:?}",
                    err
                );
            }
        }
        Err(err) => {
            error!(
                backend_id = %backend.id,
                backend_addr = %backend.address,
                "Failed to connect to designated backend target: {:?}",
                err
            );
        }
    }
}
