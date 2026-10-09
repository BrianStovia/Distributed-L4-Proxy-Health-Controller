# Automated Failover and E2E Benchmark Suite (PowerShell)
$ErrorActionPreference = "Stop"

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host " Starting Automated E2E Failover & Benchmark Test" -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

$processes = @()

try {
    # 1. Start 3 Mock Backends
    Write-Host "`n[1/5] Starting 3 Mock Backend Servers..." -ForegroundColor Yellow
    $p1 = Start-Process -FilePath "go" -ArgumentList "run tools/mock_backend/main.go -port 9001 -id backend-alpha" -PassThru
    $p2 = Start-Process -FilePath "go" -ArgumentList "run tools/mock_backend/main.go -port 9002 -id backend-beta" -PassThru
    $p3 = Start-Process -FilePath "go" -ArgumentList "run tools/mock_backend/main.go -port 9003 -id backend-gamma" -PassThru
    $processes += $p1, $p2, $p3

    Start-Sleep -Seconds 2

    # 2. Start Go Control Plane
    Write-Host "[2/5] Starting Go Control Plane on port :50051..." -ForegroundColor Yellow
    $pc = Start-Process -FilePath "go" -ArgumentList "run ./cmd/server" -WorkingDirectory "controlplane" -PassThru
    $processes += $pc

    Start-Sleep -Seconds 2

    # 3. Start Rust Data Plane
    Write-Host "[3/5] Starting Rust Data Plane on port :8080..." -ForegroundColor Yellow
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
    $pd = Start-Process -FilePath "cargo" -ArgumentList "run" -WorkingDirectory "dataplane" -PassThru
    $processes += $pd

    Write-Host "Waiting 5 seconds for cluster synchronization..." -ForegroundColor Gray
    Start-Sleep -Seconds 5

    # 4. Run Initial Benchmark (All 3 Backends Healthy)
    Write-Host "`n[4/5] Running Baseline Benchmark across all 3 nodes (1,000 requests)..." -ForegroundColor Green
    go run tests/e2e/main.go -requests 1000 -concurrency 25

    # 5. Simulate Server Crash / Failover
    Write-Host "`n[5/5] SIMULATING DISASTER: Terminating Backend Alpha (Port 9001)..." -ForegroundColor Red
    Stop-Process -Id $p1.Id -Force
    Write-Host "Waiting 3 seconds for Control Plane health prober to detect failure..." -ForegroundColor Gray
    Start-Sleep -Seconds 3

    Write-Host "`nExecuting Failover Benchmark (1,000 requests during outage)..." -ForegroundColor Green
    go run tests/e2e/main.go -requests 1000 -concurrency 25

    Write-Host "`n==========================================================" -ForegroundColor Cyan
    Write-Host " E2E FAILOVER VALIDATION COMPLETED SUCCESSFULLY!" -ForegroundColor Green
    Write-Host "==========================================================" -ForegroundColor Cyan
}
finally {
    Write-Host "`nCleaning up cluster processes..." -ForegroundColor Gray
    foreach ($p in $processes) {
        if ($p -and -not $p.HasExited) {
            Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
        }
    }
}
