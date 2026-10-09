use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use tokio::sync::RwLock;
use tracing::info;

#[derive(Clone, Debug)]
pub struct BackendItem {
    pub id: String,
    pub address: String,
    #[allow(dead_code)]
    pub weight: u32,
    pub is_healthy: bool,
}

#[derive(Clone)]
pub struct LoadBalancer {
    backends: Arc<RwLock<Vec<BackendItem>>>,
    index: Arc<AtomicUsize>,
}

impl LoadBalancer {
    pub fn new() -> Self {
        Self {
            backends: Arc::new(RwLock::new(Vec::new())),
            index: Arc::new(AtomicUsize::new(0)),
        }
    }

    /// Selects the next healthy backend using Round Robin balancing.
    pub async fn select_backend(&self) -> Option<BackendItem> {
        let backends = self.backends.read().await;
        let healthy: Vec<&BackendItem> = backends.iter().filter(|b| b.is_healthy).collect();

        if healthy.is_empty() {
            return None;
        }

        let curr = self.index.fetch_add(1, Ordering::Relaxed);
        let selected = healthy[curr % healthy.len()];
        Some(selected.clone())
    }

    /// Dynamically updates the backend targets from a control-plane snapshot.
    pub async fn update_backends(&self, new_backends: Vec<BackendItem>) {
        let mut backends = self.backends.write().await;
        info!(
            previous_count = backends.len(),
            new_count = new_backends.len(),
            "Dynamically updating load balancer backend targets"
        );
        *backends = new_backends;
    }

    /// Returns a copy of the current registered backends.
    #[allow(dead_code)]
    pub async fn get_backends(&self) -> Vec<BackendItem> {
        self.backends.read().await.clone()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_round_robin_selection() {
        let balancer = LoadBalancer::new();
        assert!(balancer.select_backend().await.is_none());

        let targets = vec![
            BackendItem {
                id: "b1".into(),
                address: "127.0.0.1:9001".into(),
                weight: 100,
                is_healthy: true,
            },
            BackendItem {
                id: "b2".into(),
                address: "127.0.0.1:9002".into(),
                weight: 100,
                is_healthy: true,
            },
            BackendItem {
                id: "b3".into(),
                address: "127.0.0.1:9003".into(),
                weight: 100,
                is_healthy: false, // Down
            },
        ];

        balancer.update_backends(targets).await;

        let first = balancer.select_backend().await.unwrap();
        let second = balancer.select_backend().await.unwrap();
        let third = balancer.select_backend().await.unwrap();

        assert_eq!(first.id, "b1");
        assert_eq!(second.id, "b2");
        assert_eq!(third.id, "b1"); // Wraps back, skips unhealthy b3
    }
}
