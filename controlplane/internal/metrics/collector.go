package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// BackendHealthStatus tracks whether a backend is currently healthy (1) or down (0).
	BackendHealthStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "proxy",
			Subsystem: "cluster",
			Name:      "backend_health_status",
			Help:      "Health state of registered backend servers (1 = Healthy, 0 = Unhealthy).",
		},
		[]string{"backend_id", "address"},
	)

	// ConfigRevisionsTotal counts how many config updates have been broadcast.
	ConfigRevisionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "proxy",
			Subsystem: "control",
			Name:      "config_revisions_total",
			Help:      "Total count of configuration revisions broadcast to Data Plane nodes.",
		},
	)

	// DataPlaneNodesConnected measures current connected Data Plane proxies.
	DataPlaneNodesConnected = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "proxy",
			Subsystem: "control",
			Name:      "dataplane_nodes_connected",
			Help:      "Number of currently connected Data Plane proxy instances.",
		},
	)

	// ActiveConnections tracks live client connections reported by Data Plane.
	ActiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "proxy",
			Subsystem: "dataplane",
			Name:      "active_connections",
			Help:      "Current active TCP connections per Data Plane node.",
		},
		[]string{"node_id"},
	)

	// TotalConnections tracks total cumulative connections handled.
	TotalConnections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "proxy",
			Subsystem: "dataplane",
			Name:      "connections_total",
			Help:      "Cumulative TCP client connections handled.",
		},
		[]string{"node_id"},
	)
)

// RecordBackendHealth updates the Prometheus gauge for a target backend.
func RecordBackendHealth(backendID, address string, isHealthy bool) {
	val := 0.0
	if isHealthy {
		val = 1.0
	}
	BackendHealthStatus.WithLabelValues(backendID, address).Set(val)
}
