// Package performance contains performance and benchmark tests for Keystone Core.
// These tests measure throughput, latency, and resource usage under load.
//
// To run performance tests:
//
//	KSCORE_E2E_TESTS=1 KSCORE_PERF_TESTS=1 go test -v ./test/e2e/performance/...
//
// Or use the Makefile:
//
//	make -C test/e2e test-performance
package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/test/e2e/harness"
)

var testEnv *harness.TestEnvironment

// TestMain sets up the test environment for performance tests
func TestMain(m *testing.M) {
	// Skip if not running E2E tests
	if os.Getenv("KSCORE_E2E_TESTS") != "1" {
		os.Exit(0)
	}

	// Skip if not running performance tests
	if os.Getenv("KSCORE_PERF_TESTS") != "1" {
		fmt.Println("Skipping performance tests (set KSCORE_PERF_TESTS=1 to enable)")
		os.Exit(0)
	}

	var cfg *harness.Config
	if harness.IsVMMode() {
		vmCfg, _, err := harness.ConfigFromVM("")
		if err != nil {
			panic("failed to load VM config: " + err.Error())
		}
		cfg = vmCfg
		cfg.ProjectName = "kscore-e2e-performance"
		cfg.StartupTimeout = 180 * time.Second
	} else {
		// Find compose file
		composeFile := findComposeFile()
		if composeFile == "" {
			panic("could not find docker-compose.yml")
		}

		// Skip building if KSCORE_SKIP_BUILD=1
		buildImages := os.Getenv("KSCORE_SKIP_BUILD") != "1"

		cfg = &harness.Config{
			ComposeFile:    composeFile,
			ProjectName:    "kscore-e2e-performance",
			BuildImages:    buildImages,
			StartupTimeout: 180 * time.Second,
			ServerGRPCPort: 8080,
			ServerHTTPPort: 8081,
		}
	}

	var err error
	testEnv, err = harness.New(cfg)
	if err != nil {
		panic("failed to create test environment: " + err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := testEnv.Start(ctx, cfg); err != nil {
		panic("failed to start test environment: " + err.Error())
	}

	// Wait for agents
	if err := testEnv.WaitForAgents(ctx, 3, 60*time.Second); err != nil {
		panic("agents did not register: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Cleanup
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = testEnv.Stop(ctx)

	os.Exit(code)
}

func findComposeFile() string {
	candidates := []string{
		"../containers/docker-compose.yml",
		"test/e2e/containers/docker-compose.yml",
		"../../containers/docker-compose.yml",
	}

	if root := os.Getenv("KSCORE_ROOT"); root != "" {
		candidates = append(candidates, filepath.Join(root, "test/e2e/containers/docker-compose.yml"))
	}

	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}

	return ""
}

// =============================================================================
// Throughput Metrics
// =============================================================================

// ThroughputResult holds the results of a throughput test
type ThroughputResult struct {
	TestName      string        `json:"test_name"`
	TotalOps      int           `json:"total_ops"`
	SuccessfulOps int           `json:"successful_ops"`
	FailedOps     int           `json:"failed_ops"`
	Duration      time.Duration `json:"duration_ns"`
	OpsPerSecond  float64       `json:"ops_per_second"`
	AvgLatency    time.Duration `json:"avg_latency_ns"`
	MinLatency    time.Duration `json:"min_latency_ns"`
	MaxLatency    time.Duration `json:"max_latency_ns"`
	P50Latency    time.Duration `json:"p50_latency_ns"`
	P95Latency    time.Duration `json:"p95_latency_ns"`
	P99Latency    time.Duration `json:"p99_latency_ns"`
	ErrorRate     float64       `json:"error_rate"`
	Timestamp     time.Time     `json:"timestamp"`
}

// LatencyCollector collects latency measurements
type LatencyCollector struct {
	mu        sync.Mutex
	latencies []time.Duration
}

// Add adds a latency measurement
func (c *LatencyCollector) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latencies = append(c.latencies, d)
}

// Calculate calculates latency statistics
func (c *LatencyCollector) Calculate() (min, max, avg, p50, p95, p99 time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.latencies) == 0 {
		return
	}

	// Sort for percentile calculation
	sorted := make([]time.Duration, len(c.latencies))
	copy(sorted, c.latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	min = sorted[0]
	max = sorted[len(sorted)-1]

	var sum time.Duration
	for _, l := range sorted {
		sum += l
	}
	avg = sum / time.Duration(len(sorted))

	p50 = sorted[len(sorted)*50/100]
	p95 = sorted[len(sorted)*95/100]
	if len(sorted) > 100 {
		p99 = sorted[len(sorted)*99/100]
	} else {
		p99 = max
	}

	return
}

// =============================================================================
// Command Throughput Tests (T4.2)
// =============================================================================

// TestThroughput_SequentialCommands measures throughput for sequential commands
func TestThroughput_SequentialCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	agentID := "agent-web-1"
	numCommands := 100

	collector := &LatencyCollector{}
	successCount := 0
	failCount := 0

	start := time.Now()

	for i := 0; i < numCommands; i++ {
		cmdStart := time.Now()
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "echo", "test", fmt.Sprintf("%d", i))
		cmdDuration := time.Since(cmdStart)

		if err != nil || result.ExitCode != 0 {
			failCount++
		} else {
			successCount++
			collector.Add(cmdDuration)
		}
	}

	duration := time.Since(start)

	// Calculate metrics
	min, max, avg, p50, p95, p99 := collector.Calculate()
	opsPerSec := float64(numCommands) / duration.Seconds()
	errorRate := float64(failCount) / float64(numCommands) * 100

	result := ThroughputResult{
		TestName:      "sequential_commands",
		TotalOps:      numCommands,
		SuccessfulOps: successCount,
		FailedOps:     failCount,
		Duration:      duration,
		OpsPerSecond:  opsPerSec,
		AvgLatency:    avg,
		MinLatency:    min,
		MaxLatency:    max,
		P50Latency:    p50,
		P95Latency:    p95,
		P99Latency:    p99,
		ErrorRate:     errorRate,
		Timestamp:     time.Now(),
	}

	// Log results
	t.Logf("Sequential Command Throughput:")
	t.Logf("  Total commands: %d", numCommands)
	t.Logf("  Successful: %d, Failed: %d", successCount, failCount)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", opsPerSec)
	t.Logf("  Latency - Avg: %v, Min: %v, Max: %v", avg, min, max)
	t.Logf("  Latency - P50: %v, P95: %v, P99: %v", p50, p95, p99)
	t.Logf("  Error rate: %.2f%%", errorRate)

	// Save result
	saveResult(t, result)

	// Assertions
	if errorRate > 5 {
		t.Errorf("Error rate too high: %.2f%%", errorRate)
	}
	if opsPerSec < 1 {
		t.Errorf("Throughput too low: %.2f ops/sec", opsPerSec)
	}
}

// TestThroughput_ParallelCommands measures throughput for parallel commands
func TestThroughput_ParallelCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	agents := []string{"agent-web-1", "agent-web-2", "agent-db-1"}
	commandsPerAgent := 30
	totalCommands := len(agents) * commandsPerAgent

	collector := &LatencyCollector{}
	var successCount, failCount int64

	start := time.Now()

	var wg sync.WaitGroup
	for _, agentID := range agents {
		for i := 0; i < commandsPerAgent; i++ {
			wg.Add(1)
			go func(agent string, idx int) {
				defer wg.Done()

				cmdStart := time.Now()
				result, err := testEnv.ExecuteCommandAndWait(ctx, agent, "echo", "parallel", fmt.Sprintf("%d", idx))
				cmdDuration := time.Since(cmdStart)

				if err != nil || result.ExitCode != 0 {
					atomic.AddInt64(&failCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
					collector.Add(cmdDuration)
				}
			}(agentID, i)
		}
	}

	wg.Wait()
	duration := time.Since(start)

	// Calculate metrics
	min, max, avg, p50, p95, p99 := collector.Calculate()
	opsPerSec := float64(totalCommands) / duration.Seconds()
	errorRate := float64(failCount) / float64(totalCommands) * 100

	result := ThroughputResult{
		TestName:      "parallel_commands",
		TotalOps:      totalCommands,
		SuccessfulOps: int(successCount),
		FailedOps:     int(failCount),
		Duration:      duration,
		OpsPerSecond:  opsPerSec,
		AvgLatency:    avg,
		MinLatency:    min,
		MaxLatency:    max,
		P50Latency:    p50,
		P95Latency:    p95,
		P99Latency:    p99,
		ErrorRate:     errorRate,
		Timestamp:     time.Now(),
	}

	// Log results
	t.Logf("Parallel Command Throughput:")
	t.Logf("  Total commands: %d (across %d agents)", totalCommands, len(agents))
	t.Logf("  Successful: %d, Failed: %d", successCount, failCount)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", opsPerSec)
	t.Logf("  Latency - Avg: %v, Min: %v, Max: %v", avg, min, max)
	t.Logf("  Latency - P50: %v, P95: %v, P99: %v", p50, p95, p99)
	t.Logf("  Error rate: %.2f%%", errorRate)

	// Save result
	saveResult(t, result)

	// Assertions
	if errorRate > 5 {
		t.Errorf("Error rate too high: %.2f%%", errorRate)
	}
}

// TestThroughput_BatchCommands measures batch command throughput
func TestThroughput_BatchCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := testEnv.Client()
	numBatches := 10

	collector := &LatencyCollector{}
	var totalAgents, successfulAgents int

	start := time.Now()

	for i := 0; i < numBatches; i++ {
		batchStart := time.Now()

		stream, err := client.BatchExecuteCommand(ctx, &pb.BatchExecuteCommandRequest{
			BatchJobId:  fmt.Sprintf("perf-batch-%d", i),
			Target:      "*",
			Command:     "echo",
			Args:        []string{"batch", fmt.Sprintf("%d", i)},
			Concurrency: 10,
		})
		if err != nil {
			t.Fatalf("Batch %d failed to start: %v", i, err)
		}

		var summary *pb.BatchSummary
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Batch %d stream error: %v", i, err)
			}
			if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
				summary = resp.Summary
			}
		}

		batchDuration := time.Since(batchStart)
		collector.Add(batchDuration)

		if summary != nil {
			totalAgents += int(summary.Total)
			successfulAgents += int(summary.Successful)
		}
	}

	duration := time.Since(start)

	// Calculate metrics
	min, max, avg, p50, p95, p99 := collector.Calculate()
	batchesPerSec := float64(numBatches) / duration.Seconds()
	agentsPerSec := float64(totalAgents) / duration.Seconds()

	result := ThroughputResult{
		TestName:      "batch_commands",
		TotalOps:      numBatches,
		SuccessfulOps: numBatches,
		Duration:      duration,
		OpsPerSecond:  batchesPerSec,
		AvgLatency:    avg,
		MinLatency:    min,
		MaxLatency:    max,
		P50Latency:    p50,
		P95Latency:    p95,
		P99Latency:    p99,
		Timestamp:     time.Now(),
	}

	// Log results
	t.Logf("Batch Command Throughput:")
	t.Logf("  Total batches: %d", numBatches)
	t.Logf("  Total agent executions: %d", totalAgents)
	t.Logf("  Successful agent executions: %d", successfulAgents)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Batch throughput: %.2f batches/sec", batchesPerSec)
	t.Logf("  Agent throughput: %.2f agents/sec", agentsPerSec)
	t.Logf("  Batch latency - Avg: %v, Min: %v, Max: %v", avg, min, max)
	t.Logf("  Batch latency - P50: %v, P95: %v, P99: %v", p50, p95, p99)

	// Save result
	saveResult(t, result)
}

// TestThroughput_SustainedLoad measures throughput under sustained load
func TestThroughput_SustainedLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping sustained load test in short mode")
	}

	// Run for 60 seconds instead of 5 minutes for E2E test environment
	testDuration := 60 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), testDuration+30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	collector := &LatencyCollector{}
	var successCount, failCount int64

	start := time.Now()
	deadline := start.Add(testDuration)

	// Rate limit to avoid overwhelming the system
	ticker := time.NewTicker(100 * time.Millisecond) // 10 ops/sec target
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			break
		case <-ticker.C:
			cmdStart := time.Now()
			result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "echo", "sustained")
			cmdDuration := time.Since(cmdStart)

			if err != nil || result.ExitCode != 0 {
				atomic.AddInt64(&failCount, 1)
			} else {
				atomic.AddInt64(&successCount, 1)
				collector.Add(cmdDuration)
			}
		}
	}

	duration := time.Since(start)
	totalOps := int(successCount + failCount)

	// Calculate metrics
	min, max, avg, p50, p95, p99 := collector.Calculate()
	opsPerSec := float64(totalOps) / duration.Seconds()
	errorRate := float64(failCount) / float64(totalOps) * 100

	result := ThroughputResult{
		TestName:      "sustained_load",
		TotalOps:      totalOps,
		SuccessfulOps: int(successCount),
		FailedOps:     int(failCount),
		Duration:      duration,
		OpsPerSecond:  opsPerSec,
		AvgLatency:    avg,
		MinLatency:    min,
		MaxLatency:    max,
		P50Latency:    p50,
		P95Latency:    p95,
		P99Latency:    p99,
		ErrorRate:     errorRate,
		Timestamp:     time.Now(),
	}

	// Log results
	t.Logf("Sustained Load Throughput (%v):", testDuration)
	t.Logf("  Total commands: %d", totalOps)
	t.Logf("  Successful: %d, Failed: %d", successCount, failCount)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", opsPerSec)
	t.Logf("  Latency - Avg: %v, Min: %v, Max: %v", avg, min, max)
	t.Logf("  Latency - P50: %v, P95: %v, P99: %v", p50, p95, p99)
	t.Logf("  Error rate: %.2f%%", errorRate)

	// Save result
	saveResult(t, result)

	// Assertions
	if errorRate > 5 {
		t.Errorf("Error rate too high during sustained load: %.2f%%", errorRate)
	}
}

// =============================================================================
// API Throughput Tests
// =============================================================================

// TestThroughput_ListAgents measures ListAgents API throughput
func TestThroughput_ListAgents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := testEnv.Client()
	numRequests := 100

	collector := &LatencyCollector{}
	successCount := 0
	failCount := 0

	start := time.Now()

	for i := 0; i < numRequests; i++ {
		reqStart := time.Now()
		_, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
		reqDuration := time.Since(reqStart)

		if err != nil {
			failCount++
		} else {
			successCount++
			collector.Add(reqDuration)
		}
	}

	duration := time.Since(start)

	// Calculate metrics
	min, max, avg, p50, p95, p99 := collector.Calculate()
	opsPerSec := float64(numRequests) / duration.Seconds()

	result := ThroughputResult{
		TestName:      "list_agents_api",
		TotalOps:      numRequests,
		SuccessfulOps: successCount,
		FailedOps:     failCount,
		Duration:      duration,
		OpsPerSecond:  opsPerSec,
		AvgLatency:    avg,
		MinLatency:    min,
		MaxLatency:    max,
		P50Latency:    p50,
		P95Latency:    p95,
		P99Latency:    p99,
		Timestamp:     time.Now(),
	}

	// Log results
	t.Logf("ListAgents API Throughput:")
	t.Logf("  Total requests: %d", numRequests)
	t.Logf("  Successful: %d, Failed: %d", successCount, failCount)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", opsPerSec)
	t.Logf("  Latency - Avg: %v, Min: %v, Max: %v", avg, min, max)
	t.Logf("  Latency - P50: %v, P95: %v, P99: %v", p50, p95, p99)

	// Save result
	saveResult(t, result)
}

// TestThroughput_GetAgent measures GetAgent API throughput
func TestThroughput_GetAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := testEnv.Client()
	agentID := "agent-web-1"
	numRequests := 100

	collector := &LatencyCollector{}
	successCount := 0
	failCount := 0

	start := time.Now()

	for i := 0; i < numRequests; i++ {
		reqStart := time.Now()
		_, err := client.GetAgent(ctx, &pb.GetAgentRequest{AgentId: agentID})
		reqDuration := time.Since(reqStart)

		if err != nil {
			failCount++
		} else {
			successCount++
			collector.Add(reqDuration)
		}
	}

	duration := time.Since(start)

	// Calculate metrics
	min, max, avg, p50, p95, p99 := collector.Calculate()
	opsPerSec := float64(numRequests) / duration.Seconds()

	result := ThroughputResult{
		TestName:      "get_agent_api",
		TotalOps:      numRequests,
		SuccessfulOps: successCount,
		FailedOps:     failCount,
		Duration:      duration,
		OpsPerSecond:  opsPerSec,
		AvgLatency:    avg,
		MinLatency:    min,
		MaxLatency:    max,
		P50Latency:    p50,
		P95Latency:    p95,
		P99Latency:    p99,
		Timestamp:     time.Now(),
	}

	// Log results
	t.Logf("GetAgent API Throughput:")
	t.Logf("  Total requests: %d", numRequests)
	t.Logf("  Successful: %d, Failed: %d", successCount, failCount)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", opsPerSec)
	t.Logf("  Latency - Avg: %v, Min: %v, Max: %v", avg, min, max)
	t.Logf("  Latency - P50: %v, P95: %v, P99: %v", p50, p95, p99)

	// Save result
	saveResult(t, result)
}

// =============================================================================
// Helpers
// =============================================================================

// saveResult saves a result to JSON file
func saveResult(t *testing.T, result ThroughputResult) {
	t.Helper()

	// Create reports directory
	reportsDir := "reports"
	if root := os.Getenv("KSCORE_ROOT"); root != "" {
		reportsDir = filepath.Join(root, "test/e2e/performance/reports")
	}
	_ = os.MkdirAll(reportsDir, 0755)

	// Save individual result
	filename := filepath.Join(reportsDir, fmt.Sprintf("%s_%d.json", result.TestName, result.Timestamp.Unix()))
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Logf("Failed to marshal result: %v", err)
		return
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		t.Logf("Failed to save result: %v", err)
	} else {
		t.Logf("Result saved to: %s", filename)
	}
}
