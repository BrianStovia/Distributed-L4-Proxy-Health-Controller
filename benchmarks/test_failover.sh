#!/usr/bin/env bash
# Automated Failover and E2E Benchmark Suite (Bash)
set -eo pipefail

echo "=========================================================="
echo " Starting Automated E2E Failover & Benchmark Test"
echo "=========================================================="

cleanup() {
    echo "Cleaning up background processes..."
    kill $(jobs -p) 2>/dev/null || true
}
trap cleanup EXIT

# 1. Start 3 Mock Backends
echo -e "\n[1/5] Starting 3 Mock Backend Servers..."
go run tools/mock_backend/main.go -port 9001 -id backend-alpha &
P1=$!
go run tools/mock_backend/main.go -port 9002 -id backend-beta &
P2=$!
go run tools/mock_backend/main.go -port 9003 -id backend-gamma &
P3=$!
sleep 2

# 2. Start Go Control Plane
echo "[2/5] Starting Go Control Plane on port :50051..."
(cd controlplane && go run ./cmd/server) &
sleep 2

# 3. Start Rust Data Plane
echo "[3/5] Starting Rust Data Plane on port :8080..."
(cd dataplane && cargo run) &
sleep 5

# 4. Run Baseline Benchmark
echo -e "\n[4/5] Running Baseline Benchmark across all 3 nodes (1,000 requests)..."
go run tests/e2e/main.go -requests 1000 -concurrency 25

# 5. Simulate Server Crash
echo -e "\n[5/5] SIMULATING DISASTER: Terminating Backend Alpha (Port 9001)..."
kill -9 $P1
echo "Waiting 3 seconds for Control Plane health prober to detect failure..."
sleep 3

echo -e "\nExecuting Failover Benchmark (1,000 requests during outage)..."
go run tests/e2e/main.go -requests 1000 -concurrency 25

echo -e "\n=========================================================="
echo " E2E FAILOVER VALIDATION COMPLETED SUCCESSFULLY!"
echo "=========================================================="
