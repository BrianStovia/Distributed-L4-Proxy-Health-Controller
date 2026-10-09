package server

import (
	"fmt"
	"io"
	"log"
	"sync/atomic"

	"github.com/arcgg/controlplane/internal/health"
	"github.com/arcgg/controlplane/internal/metrics"
	apiv1 "github.com/arcgg/controlplane/pkg/api/v1"
)

// ProxyControlServer implements the gRPC ProxyControlService interface.
type ProxyControlServer struct {
	apiv1.UnimplementedProxyControlServiceServer
	prober  *health.Prober
	version atomic.Uint64
}

// NewProxyControlServer initializes the server.
func NewProxyControlServer(prober *health.Prober) *ProxyControlServer {
	return &ProxyControlServer{
		prober: prober,
	}
}

// WatchClusterConfig streams cluster snapshots to connected Data Plane nodes.
func (s *ProxyControlServer) WatchClusterConfig(
	req *apiv1.WatchConfigRequest,
	stream apiv1.ProxyControlService_WatchClusterConfigServer,
) error {
	log.Printf("[gRPC] Data Plane node connected: id=%s current_ver=%s\n",
		req.GetProxyNodeId(), req.GetCurrentVersion())
	metrics.DataPlaneNodesConnected.Inc()
	defer metrics.DataPlaneNodesConnected.Dec()

	// Push immediate initial snapshot upon connection
	currentVer := fmt.Sprintf("rev-%d", s.version.Add(1))
	metrics.ConfigRevisionsTotal.Inc()
	initialSnapshot := s.prober.GetSnapshot(currentVer)
	if err := stream.Send(initialSnapshot); err != nil {
		log.Printf("[gRPC] Failed to send initial snapshot to node %s: %v\n", req.GetProxyNodeId(), err)
		return err
	}

	stateChanges := s.prober.SubscribeStateChanges()

	for {
		select {
		case <-stream.Context().Done():
			log.Printf("[gRPC] Data Plane node disconnected: id=%s\n", req.GetProxyNodeId())
			return stream.Context().Err()

		case <-stateChanges:
			newVer := fmt.Sprintf("rev-%d", s.version.Add(1))
			metrics.ConfigRevisionsTotal.Inc()
			snapshot := s.prober.GetSnapshot(newVer)
			log.Printf("[gRPC] Pushing configuration update %s to node %s (%d backends)\n",
				newVer, req.GetProxyNodeId(), len(snapshot.GetBackends()))

			if err := stream.Send(snapshot); err != nil {
				log.Printf("[gRPC] Error sending snapshot to node %s: %v\n", req.GetProxyNodeId(), err)
				return err
			}
		}
	}
}

// ReportMetrics ingests telemetry streams from Data Plane instances.
func (s *ProxyControlServer) ReportMetrics(stream apiv1.ProxyControlService_ReportMetricsServer) error {
	var totalBatches uint64
	for {
		batch, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&apiv1.MetricAck{
				Accepted: true,
				Message:  fmt.Sprintf("Processed %d telemetry batches", totalBatches),
			})
		}
		if err != nil {
			return err
		}

		totalBatches++
		for _, m := range batch.GetMetrics() {
			metrics.ActiveConnections.WithLabelValues(batch.GetProxyNodeId()).Set(float64(m.GetActiveConnections()))
			metrics.TotalConnections.WithLabelValues(batch.GetProxyNodeId()).Add(float64(m.GetTotalConnectionsHandled()))

			log.Printf("[Metrics] Node=%s Backend=%s ActiveConn=%d RxBytes=%d TxBytes=%d\n",
				batch.GetProxyNodeId(), m.GetBackendId(), m.GetActiveConnections(), m.GetBytesRx(), m.GetBytesTx())
		}
	}
}
