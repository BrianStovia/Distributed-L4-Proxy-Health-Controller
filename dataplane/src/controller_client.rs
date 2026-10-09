pub mod control {
    tonic::include_proto!("proxy.control.v1");
}

use crate::balancer::{BackendItem, LoadBalancer};
use control::proxy_control_service_client::ProxyControlServiceClient;
use control::{BackendState, WatchConfigRequest};
use std::sync::Arc;
use std::time::Duration;
use tracing::{error, info, warn};

pub async fn start_config_watcher(
    controller_endpoint: String,
    balancer: LoadBalancer,
    node_id: String,
) {
    loop {
        info!(endpoint = %controller_endpoint, "Attempting connection to Go Control Plane...");

        match ProxyControlServiceClient::connect(controller_endpoint.clone()).await {
            Ok(mut client) => {
                info!("Connected to Go Control Plane via gRPC. Subscribing to config stream...");

                let request = tonic::Request::new(WatchConfigRequest {
                    proxy_node_id: node_id.clone(),
                    current_version: String::new(),
                });

                match client.watch_cluster_config(request).await {
                    Ok(response) => {
                        let mut stream = response.into_inner();

                        while let Ok(Some(snapshot)) = stream.message().await {
                            info!(
                                version = %snapshot.version,
                                backend_count = snapshot.backends.len(),
                                "Received cluster configuration snapshot from Control Plane"
                            );

                            let items: Vec<BackendItem> = snapshot
                                .backends
                                .into_iter()
                                .map(|b| {
                                    let is_healthy = b.state() == BackendState::Healthy;
                                    BackendItem {
                                        id: b.id,
                                        address: b.address,
                                        weight: b.weight,
                                        is_healthy,
                                    }
                                })
                                .collect();

                            balancer.update_backends(items).await;
                        }

                        warn!("Configuration stream terminated by Control Plane. Reconnecting...");
                    }
                    Err(err) => {
                        error!("Failed to subscribe via watch_cluster_config: {:?}", err);
                    }
                }
            }
            Err(err) => {
                warn!(
                    "Control plane unreachable ({:?}). Retrying in 3 seconds...",
                    err
                );
            }
        }

        tokio::time::sleep(Duration::from_secs(3)).await;
    }
}

pub async fn start_metrics_reporter(
    controller_endpoint: String,
    metrics: Arc<crate::proxy::ProxyMetrics>,
    node_id: String,
) {
    loop {
        if let Ok(mut client) =
            ProxyControlServiceClient::connect(controller_endpoint.clone()).await
        {
            let (tx, rx) = tokio::sync::mpsc::channel(4);
            let outbound = tokio_stream::wrappers::ReceiverStream::new(rx);

            let node_id_clone = node_id.clone();
            let metrics_clone = metrics.clone();

            tokio::spawn(async move {
                loop {
                    tokio::time::sleep(Duration::from_secs(2)).await;
                    let active = metrics_clone
                        .active_connections
                        .load(std::sync::atomic::Ordering::Relaxed);
                    let total = metrics_clone
                        .total_connections
                        .load(std::sync::atomic::Ordering::Relaxed);

                    let batch = control::MetricBatch {
                        proxy_node_id: node_id_clone.clone(),
                        timestamp_unix_epoch_ms: std::time::SystemTime::now()
                            .duration_since(std::time::UNIX_EPOCH)
                            .unwrap_or_default()
                            .as_millis() as i64,
                        metrics: vec![control::BackendMetric {
                            backend_id: "global".to_string(),
                            active_connections: active,
                            total_connections_handled: total,
                            bytes_rx: 0,
                            bytes_tx: 0,
                            connect_errors: 0,
                        }],
                    };

                    if tx.send(batch).await.is_err() {
                        break;
                    }
                }
            });

            if let Ok(response) = client.report_metrics(tonic::Request::new(outbound)).await {
                info!("ReportMetrics channel closed: {:?}", response);
            }
        }
        tokio::time::sleep(Duration::from_secs(3)).await;
    }
}
