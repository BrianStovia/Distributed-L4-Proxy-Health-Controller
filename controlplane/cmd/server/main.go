package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/arcgg/controlplane/internal/health"
	"github.com/arcgg/controlplane/internal/server"
	apiv1 "github.com/arcgg/controlplane/pkg/api/v1"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

func main() {
	log.Println("[ControlPlane] Starting Go Control Plane...")

	backendEnv := os.Getenv("BACKEND_TARGETS")
	var initialTargets []*health.Target

	if backendEnv != "" {
		// Example: "backend-1:9001,backend-2:9002,backend-3:9003"
		addrs := strings.Split(backendEnv, ",")
		for i, addr := range addrs {
			trimmed := strings.TrimSpace(addr)
			if trimmed == "" {
				continue
			}
			initialTargets = append(initialTargets, &health.Target{
				ID:      fmt.Sprintf("backend-%d", i+1),
				Address: trimmed,
				Weight:  100,
				State:   apiv1.BackendState_BACKEND_STATE_UNSPECIFIED,
			})
		}
	} else {
		// Default local development targets
		initialTargets = []*health.Target{
			{
				ID:      "backend-alpha",
				Address: "127.0.0.1:9001",
				Weight:  100,
				State:   apiv1.BackendState_BACKEND_STATE_UNSPECIFIED,
			},
			{
				ID:      "backend-beta",
				Address: "127.0.0.1:9002",
				Weight:  100,
				State:   apiv1.BackendState_BACKEND_STATE_UNSPECIFIED,
			},
			{
				ID:      "backend-gamma",
				Address: "127.0.0.1:9003",
				Weight:  100,
				State:   apiv1.BackendState_BACKEND_STATE_UNSPECIFIED,
			},
		}
	}

	prober := health.NewProber(initialTargets, 2*time.Second, 500*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Launch active health prober
	go prober.Start(ctx)

	listenPort := os.Getenv("GRPC_PORT")
	if listenPort == "" {
		listenPort = ":50051"
	}
	if !strings.HasPrefix(listenPort, ":") {
		listenPort = ":" + listenPort
	}
	lis, err := net.Listen("tcp", listenPort)
	if err != nil {
		log.Fatalf("[ControlPlane] Failed to bind TCP listener on %s: %v", listenPort, err)
	}

	grpcServer := grpc.NewServer()
	controlServer := server.NewProxyControlServer(prober)
	apiv1.RegisterProxyControlServiceServer(grpcServer, controlServer)

	// Background worker for gRPC server
	go func() {
		log.Printf("[ControlPlane] gRPC Control Service actively listening on %s\n", listenPort)
		if err := grpcServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			log.Fatalf("[ControlPlane] gRPC server error: %v", err)
		}
	}()

	// Expose Prometheus metrics endpoint on HTTP
	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = ":9090"
	}
	if !strings.HasPrefix(metricsPort, ":") {
		metricsPort = ":" + metricsPort
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:    metricsPort,
		Handler: metricsMux,
	}

	go func() {
		log.Printf("[ControlPlane] Prometheus metrics endpoint listening on http://0.0.0.0%s/metrics\n", metricsPort)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[ControlPlane] Metrics HTTP server error: %v\n", err)
		}
	}()

	// Wait for OS termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("[ControlPlane] Shutting down gracefully...")
	cancel()
	_ = metricsServer.Shutdown(context.Background())
	grpcServer.GracefulStop()
	log.Println("[ControlPlane] Server stopped successfully.")
}
