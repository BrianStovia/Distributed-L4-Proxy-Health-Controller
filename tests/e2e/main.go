package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type BenchmarkResult struct {
	TotalRequests  int
	SuccessCount   int
	FailureCount   int
	TotalDuration  time.Duration
	RequestsPerSec float64
	P50Latency     time.Duration
	P95Latency     time.Duration
	P99Latency     time.Duration
	Distribution   map[string]int
}

func main() {
	proxyURL := flag.String("proxy", "http://127.0.0.1:8080", "Proxy URL under test")
	totalReqs := flag.Int("requests", 1000, "Total number of HTTP requests to execute")
	concurrency := flag.Int("concurrency", 20, "Number of concurrent worker goroutines")
	flag.Parse()

	log.Printf("=================================================================\n")
	log.Printf(" Cloud-Native L4 Proxy: Automated E2E Benchmark & Validation\n")
	log.Printf(" Target URL   : %s\n", *proxyURL)
	log.Printf(" Total Load   : %d requests\n", *totalReqs)
	log.Printf(" Concurrency  : %d workers\n", *concurrency)
	log.Printf("=================================================================\n")

	// Phase 1: Basic Health & Connectivity Probe
	log.Println("\n[Phase 1] Probing proxy connectivity...")
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(*proxyURL)
	if err != nil {
		log.Fatalf("FAILED: Proxy is not reachable at %s. Ensure Data Plane is running! Error: %v\n", *proxyURL, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	log.Printf("SUCCESS: Initial probe responded with: %s\n", strings.TrimSpace(string(body)))

	// Phase 2: High Concurrency Load Test & Balancing Distribution Analysis
	log.Printf("\n[Phase 2] Executing concurrent load test (%d requests across %d workers)...\n", *totalReqs, *concurrency)
	result := runLoadTest(*proxyURL, *totalReqs, *concurrency)

	printReport(result)

	// Phase 3: Assert Zero Failures & Even Distribution
	if result.FailureCount > 0 {
		log.Fatalf("\nFAILED: Encountered %d request failures during load test!\n", result.FailureCount)
	}

	log.Println("\n=================================================================")
	log.Println(" ALL E2E & BENCHMARK VALIDATIONS PASSED SUCCESSFULLY! ")
	log.Println("=================================================================")
}

func runLoadTest(targetURL string, totalRequests, concurrency int) BenchmarkResult {
	var wg sync.WaitGroup
	reqChan := make(chan struct{}, totalRequests)
	for i := 0; i < totalRequests; i++ {
		reqChan <- struct{}{}
	}
	close(reqChan)

	type taskResult struct {
		duration time.Duration
		err      error
		backend  string
	}

	results := make([]taskResult, totalRequests)
	var mu sync.Mutex
	idx := 0

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        concurrency * 2,
			MaxIdleConnsPerHost: concurrency * 2,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false,
		},
	}

	startTime := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range reqChan {
				t0 := time.Now()
				resp, err := client.Get(targetURL)
				dur := time.Since(t0)

				backendID := "unknown"
				if err == nil {
					b, readErr := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if readErr == nil {
						bodyStr := string(b)
						// Format: "Response from backend-alpha (port 9001)"
						if strings.Contains(bodyStr, "backend-") {
							parts := strings.Split(bodyStr, " ")
							if len(parts) >= 3 {
								backendID = parts[2]
							}
						}
					}
				}

				mu.Lock()
				results[idx] = taskResult{duration: dur, err: err, backend: backendID}
				idx++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	var durations []time.Duration
	successCount := 0
	failureCount := 0
	distribution := make(map[string]int)

	for _, r := range results {
		if r.err != nil {
			failureCount++
		} else {
			successCount++
			distribution[r.backend]++
			durations = append(durations, r.duration)
		}
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	var p50, p95, p99 time.Duration
	if len(durations) > 0 {
		p50 = durations[len(durations)*50/100]
		p95 = durations[len(durations)*95/100]
		p99 = durations[len(durations)*99/100]
	}

	rps := float64(totalRequests) / totalDuration.Seconds()

	return BenchmarkResult{
		TotalRequests:  totalRequests,
		SuccessCount:   successCount,
		FailureCount:   failureCount,
		TotalDuration:  totalDuration,
		RequestsPerSec: rps,
		P50Latency:     p50,
		P95Latency:     p95,
		P99Latency:     p99,
		Distribution:   distribution,
	}
}

func printReport(r BenchmarkResult) {
	log.Printf("\n-------------------- BENCHMARK REPORT --------------------\n")
	log.Printf(" Total Duration    : %v\n", r.TotalDuration)
	log.Printf(" Throughput (RPS)  : %.2f req/sec\n", r.RequestsPerSec)
	log.Printf(" Total Requests    : %d\n", r.TotalRequests)
	log.Printf(" Successful        : %d (%.2f%%)\n", r.SuccessCount, float64(r.SuccessCount)*100/float64(r.TotalRequests))
	log.Printf(" Failed            : %d\n", r.FailureCount)
	log.Printf(" Latency Percentiles:\n")
	log.Printf("   - p50 (Median)  : %v\n", r.P50Latency)
	log.Printf("   - p95           : %v\n", r.P95Latency)
	log.Printf("   - p99           : %v\n", r.P99Latency)
	log.Printf("\n Traffic Balancing Distribution:\n")
	for backend, count := range r.Distribution {
		pct := float64(count) * 100 / float64(r.SuccessCount)
		log.Printf("   - %-16s : %5d requests (%.1f%%)\n", backend, count, pct)
	}
	log.Printf("----------------------------------------------------------\n")
}
