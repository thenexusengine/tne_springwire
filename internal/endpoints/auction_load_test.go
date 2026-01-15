//go:build loadtest
// +build loadtest

package endpoints

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// LoadTestConfig defines load test parameters
type LoadTestConfig struct {
	TargetRPS      int           // Target requests per second
	Duration       time.Duration // Test duration
	Warmup         time.Duration // Warmup period
	NumWorkers     int           // Concurrent workers
	LatencyTargets LatencyTargets
}

// LatencyTargets defines acceptable latency thresholds
type LatencyTargets struct {
	P50 time.Duration
	P95 time.Duration
	P99 time.Duration
}

// LoadTestResults contains test metrics
type LoadTestResults struct {
	TotalRequests   uint64
	SuccessRequests uint64
	FailedRequests  uint64
	ActualRPS       float64
	Latencies       []time.Duration
	P50             time.Duration
	P95             time.Duration
	P99             time.Duration
	MinLatency      time.Duration
	MaxLatency      time.Duration
	AvgLatency      time.Duration
}

// TestAuctionEndpoint_LoadTest_10K performs 10K RPS load test
func TestAuctionEndpoint_LoadTest_10K(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	config := LoadTestConfig{
		TargetRPS:  10000,
		Duration:   30 * time.Second,
		Warmup:     5 * time.Second,
		NumWorkers: 100,
		LatencyTargets: LatencyTargets{
			P50: 50 * time.Millisecond,
			P95: 100 * time.Millisecond,
			P99: 200 * time.Millisecond,
		},
	}

	results := runLoadTest(t, config)
	validateResults(t, config, results)
}

// TestAuctionEndpoint_LoadTest_50K performs 50K RPS load test (PRODUCTION TARGET)
func TestAuctionEndpoint_LoadTest_50K(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	config := LoadTestConfig{
		TargetRPS:  50000,
		Duration:   60 * time.Second,
		Warmup:     10 * time.Second,
		NumWorkers: 500,
		LatencyTargets: LatencyTargets{
			P50: 50 * time.Millisecond,
			P95: 100 * time.Millisecond,
			P99: 200 * time.Millisecond,
		},
	}

	results := runLoadTest(t, config)
	validateResults(t, config, results)
}

// runLoadTest executes the load test with given configuration
func runLoadTest(t *testing.T, config LoadTestConfig) *LoadTestResults {
	t.Logf("🚀 Starting load test: %d RPS for %v (warmup: %v, workers: %d)",
		config.TargetRPS, config.Duration, config.Warmup, config.NumWorkers)

	// Create test server (you'll need to replace this with actual auction handler)
	handler := createMockAuctionHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Metrics
	var (
		totalRequests   uint64
		successRequests uint64
		failedRequests  uint64
		latencies       = make([]time.Duration, 0, config.TargetRPS*int(config.Duration.Seconds()))
		latenciesLock   sync.Mutex
	)

	// Request generator
	requestInterval := time.Second / time.Duration(config.TargetRPS/config.NumWorkers)

	// Start workers
	var wg sync.WaitGroup
	startTime := time.Now()
	stopTime := startTime.Add(config.Warmup + config.Duration)
	warmupEndTime := startTime.Add(config.Warmup)

	for i := 0; i < config.NumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ticker := time.NewTicker(requestInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if time.Now().After(stopTime) {
						return
					}

					// Send request
					requestStart := time.Now()
					resp, err := http.Post(
						server.URL+"/auction",
						"application/json",
						bytes.NewBuffer(createSampleBidRequest()),
					)

					requestDuration := time.Since(requestStart)
					atomic.AddUint64(&totalRequests, 1)

					if err != nil || resp.StatusCode != http.StatusOK {
						atomic.AddUint64(&failedRequests, 1)
						if resp != nil {
							resp.Body.Close()
						}
						continue
					}

					resp.Body.Close()
					atomic.AddUint64(&successRequests, 1)

					// Only record latencies after warmup
					if time.Now().After(warmupEndTime) {
						latenciesLock.Lock()
						latencies = append(latencies, requestDuration)
						latenciesLock.Unlock()
					}
				}
			}
		}(i)
	}

	// Wait for completion
	wg.Wait()
	actualDuration := time.Since(startTime)

	t.Logf("✅ Load test complete. Total time: %v", actualDuration)

	// Calculate metrics
	results := &LoadTestResults{
		TotalRequests:   totalRequests,
		SuccessRequests: successRequests,
		FailedRequests:  failedRequests,
		ActualRPS:       float64(totalRequests) / actualDuration.Seconds(),
		Latencies:       latencies,
	}

	if len(latencies) > 0 {
		calculateLatencyPercentiles(results)
	}

	return results
}

// calculateLatencyPercentiles computes latency statistics
func calculateLatencyPercentiles(results *LoadTestResults) {
	latencies := results.Latencies

	// Sort latencies
	sortDurations(latencies)

	n := len(latencies)
	results.P50 = latencies[n*50/100]
	results.P95 = latencies[n*95/100]
	results.P99 = latencies[n*99/100]
	results.MinLatency = latencies[0]
	results.MaxLatency = latencies[n-1]

	var sum time.Duration
	for _, lat := range latencies {
		sum += lat
	}
	results.AvgLatency = sum / time.Duration(n)
}

// validateResults checks if results meet targets
func validateResults(t *testing.T, config LoadTestConfig, results *LoadTestResults) {
	t.Logf("📊 Load Test Results:")
	t.Logf("   Total Requests:   %d", results.TotalRequests)
	t.Logf("   Success:          %d (%.2f%%)", results.SuccessRequests,
		float64(results.SuccessRequests)/float64(results.TotalRequests)*100)
	t.Logf("   Failed:           %d (%.2f%%)", results.FailedRequests,
		float64(results.FailedRequests)/float64(results.TotalRequests)*100)
	t.Logf("   Actual RPS:       %.0f (target: %d)", results.ActualRPS, config.TargetRPS)
	t.Logf("   Latency P50:      %v (target: %v)", results.P50, config.LatencyTargets.P50)
	t.Logf("   Latency P95:      %v (target: %v)", results.P95, config.LatencyTargets.P95)
	t.Logf("   Latency P99:      %v (target: %v)", results.P99, config.LatencyTargets.P99)
	t.Logf("   Latency Min:      %v", results.MinLatency)
	t.Logf("   Latency Max:      %v", results.MaxLatency)
	t.Logf("   Latency Avg:      %v", results.AvgLatency)

	// Validate success rate
	successRate := float64(results.SuccessRequests) / float64(results.TotalRequests)
	if successRate < 0.99 {
		t.Errorf("❌ Success rate %.2f%% below 99%% threshold", successRate*100)
	} else {
		t.Logf("✅ Success rate: %.2f%%", successRate*100)
	}

	// Validate throughput (allow 10% margin)
	minRPS := float64(config.TargetRPS) * 0.9
	if results.ActualRPS < minRPS {
		t.Errorf("❌ Actual RPS %.0f below target %d (min: %.0f)", results.ActualRPS, config.TargetRPS, minRPS)
	} else {
		t.Logf("✅ Throughput: %.0f RPS (target: %d)", results.ActualRPS, config.TargetRPS)
	}

	// Validate latencies
	if results.P50 > config.LatencyTargets.P50 {
		t.Errorf("❌ P50 latency %v exceeds target %v", results.P50, config.LatencyTargets.P50)
	}
	if results.P95 > config.LatencyTargets.P95 {
		t.Errorf("❌ P95 latency %v exceeds target %v", results.P95, config.LatencyTargets.P95)
	}
	if results.P99 > config.LatencyTargets.P99 {
		t.Errorf("❌ P99 latency %v exceeds target %v", results.P99, config.LatencyTargets.P99)
	}

	if results.P95 <= config.LatencyTargets.P95 {
		t.Logf("✅ P95 latency: %v (target: %v)", results.P95, config.LatencyTargets.P95)
	}
}

// createMockAuctionHandler creates a realistic mock auction handler
func createMockAuctionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate realistic auction processing time (10-50ms)
		time.Sleep(time.Duration(10+fastRand()%40) * time.Millisecond)

		// Return mock bid response
		response := map[string]interface{}{
			"id": "test-auction-123",
			"seatbid": []map[string]interface{}{
				{
					"bid": []map[string]interface{}{
						{
							"id":    "bid-1",
							"impid": "1",
							"price": 2.50,
						},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})
}

// createSampleBidRequest creates a realistic OpenRTB bid request
func createSampleBidRequest() []byte {
	request := map[string]interface{}{
		"id": "test-request-123",
		"imp": []map[string]interface{}{
			{
				"id": "1",
				"banner": map[string]interface{}{
					"w": 728,
					"h": 90,
				},
				"bidfloor": 1.0,
			},
		},
		"site": map[string]interface{}{
			"domain": "example.com",
		},
		"device": map[string]interface{}{
			"ua": "Mozilla/5.0...",
			"ip": "192.168.1.1",
		},
	}

	data, _ := json.Marshal(request)
	return data
}

// Helper functions

// sortDurations sorts a slice of durations in place
func sortDurations(durations []time.Duration) {
	// Simple insertion sort (good enough for latencies)
	for i := 1; i < len(durations); i++ {
		key := durations[i]
		j := i - 1
		for j >= 0 && durations[j] > key {
			durations[j+1] = durations[j]
			j--
		}
		durations[j+1] = key
	}
}

// fastRand provides a fast random number (not cryptographically secure)
var rngSeed uint32 = 12345

func fastRand() uint32 {
	rngSeed = rngSeed*1664525 + 1013904223
	return rngSeed
}

// BenchmarkAuctionEndpoint_Concurrent benchmarks concurrent auction requests
func BenchmarkAuctionEndpoint_Concurrent(b *testing.B) {
	handler := createMockAuctionHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := http.Post(
				server.URL+"/auction",
				"application/json",
				bytes.NewBuffer(createSampleBidRequest()),
			)
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "req/s")
}

// BenchmarkAuctionEndpoint_Latency benchmarks auction latency
func BenchmarkAuctionEndpoint_Latency(b *testing.B) {
	handler := createMockAuctionHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		resp, err := http.Post(
			server.URL+"/auction",
			"application/json",
			bytes.NewBuffer(createSampleBidRequest()),
		)
		latency := time.Since(start)

		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()

		if latency > 100*time.Millisecond {
			b.Errorf("Latency %v exceeds 100ms threshold", latency)
		}
	}
}
