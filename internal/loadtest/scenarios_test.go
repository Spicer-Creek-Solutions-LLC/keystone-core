// Package loadtest provides load testing infrastructure for Keystone Core.
package loadtest

import (
	"context"
	"os"
	"testing"
	"time"
)

// skipIfNotLoadTest skips the test if KSCORE_LOAD_TEST is not set.
func skipIfNotLoadTest(t *testing.T) {
	if os.Getenv("KSCORE_LOAD_TEST") == "" {
		t.Skip("Skipping load test (set KSCORE_LOAD_TEST=1 to run)")
	}
}

// TestLoad_Registration tests agent registration performance.
func TestLoad_Registration(t *testing.T) {
	skipIfNotLoadTest(t)

	cfg := ConfigFromEnv()
	t.Logf("Testing registration with %d agents", cfg.AgentCount)

	harness := NewTestHarness(cfg)
	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer harness.Stop()

	result := &LoadTestResult{
		TestName:  "registration",
		StartTime: time.Now(),
		Config: ResultConfig{
			AgentCount:        cfg.AgentCount,
			HeartbeatInterval: cfg.HeartbeatInterval,
		},
	}

	// Create and start agent pool
	pool, err := harness.CreateAgentPool()
	if err != nil {
		t.Fatalf("Failed to create agent pool: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	regStart := time.Now()
	if err := pool.StartAll(ctx); err != nil {
		t.Fatalf("Failed to start agents: %v", err)
	}
	regDuration := time.Since(regStart)

	// Collect metrics
	metrics := pool.AggregateMetrics()
	metrics.RegistrationTime = regDuration
	metrics.AgentsRegistered = pool.AgentCount()

	result.Metrics = metrics
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = metrics.AgentsRegistered == cfg.AgentCount

	// Stop agents
	pool.StopAll()

	// Log results
	t.Logf("Registration Results:")
	t.Logf("  Agents registered: %d/%d", metrics.AgentsRegistered, cfg.AgentCount)
	t.Logf("  Total registration time: %v", regDuration)
	t.Logf("  Avg registration time: %v", metrics.RegistrationTime/time.Duration(cfg.AgentCount))
	t.Logf("  Min latency: %v", metrics.MinLatency)
	t.Logf("  Max latency: %v", metrics.MaxLatency)
	t.Logf("  P50 latency: %v", metrics.P50Latency)
	t.Logf("  P95 latency: %v", metrics.P95Latency)
	t.Logf("  P99 latency: %v", metrics.P99Latency)

	// Save result
	if cfg.ReportDir != "" {
		if err := SaveResult(result, cfg.ReportDir); err != nil {
			t.Logf("Warning: failed to save result: %v", err)
		}
	}

	if !result.Success {
		t.Errorf("Registration failed: expected %d agents, got %d", cfg.AgentCount, metrics.AgentsRegistered)
	}
}

// TestLoad_Heartbeat tests sustained heartbeat performance.
func TestLoad_Heartbeat(t *testing.T) {
	skipIfNotLoadTest(t)

	cfg := ConfigFromEnv()
	t.Logf("Testing heartbeat with %d agents for %v", cfg.AgentCount, cfg.TestDuration)

	harness := NewTestHarness(cfg)
	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer harness.Stop()

	result := &LoadTestResult{
		TestName:  "heartbeat",
		StartTime: time.Now(),
		Config: ResultConfig{
			AgentCount:        cfg.AgentCount,
			HeartbeatInterval: cfg.HeartbeatInterval,
			TestDuration:      cfg.TestDuration,
		},
	}

	// Create and start agent pool
	pool, err := harness.CreateAgentPool()
	if err != nil {
		t.Fatalf("Failed to create agent pool: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := pool.StartAll(ctx); err != nil {
		t.Fatalf("Failed to start agents: %v", err)
	}

	// Run for test duration
	t.Logf("Running heartbeat test for %v...", cfg.TestDuration)
	time.Sleep(cfg.TestDuration)

	// Collect metrics
	metrics := pool.AggregateMetrics()
	cpMetrics := harness.ControlPlane().Metrics()

	metrics.HeartbeatsSent = cpMetrics.Heartbeats
	metrics.HeartbeatsReceived = cpMetrics.Heartbeats
	metrics.AgentsRegistered = pool.AgentCount()

	// Calculate expected heartbeats
	expectedHeartbeats := int64(cfg.AgentCount) * int64(cfg.TestDuration/cfg.HeartbeatInterval)
	heartbeatRate := float64(metrics.HeartbeatsSent) / cfg.TestDuration.Seconds()

	result.Metrics = metrics
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = metrics.HeartbeatsSent > 0

	// Stop agents
	pool.StopAll()

	// Log results
	t.Logf("Heartbeat Results:")
	t.Logf("  Agents: %d", metrics.AgentsRegistered)
	t.Logf("  Heartbeats sent: %d", metrics.HeartbeatsSent)
	t.Logf("  Expected heartbeats: ~%d", expectedHeartbeats)
	t.Logf("  Heartbeat rate: %.2f/sec", heartbeatRate)

	// Save result
	if cfg.ReportDir != "" {
		if err := SaveResult(result, cfg.ReportDir); err != nil {
			t.Logf("Warning: failed to save result: %v", err)
		}
	}

	if !result.Success {
		t.Error("Heartbeat test failed: no heartbeats received")
	}
}

// TestLoad_Commands tests command execution performance.
func TestLoad_Commands(t *testing.T) {
	skipIfNotLoadTest(t)

	cfg := ConfigFromEnv()
	totalCommands := cfg.AgentCount * cfg.CommandsPerAgent
	t.Logf("Testing commands: %d agents, %d commands/agent, %d total",
		cfg.AgentCount, cfg.CommandsPerAgent, totalCommands)

	harness := NewTestHarness(cfg)
	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer harness.Stop()

	result := &LoadTestResult{
		TestName:  "commands",
		StartTime: time.Now(),
		Config: ResultConfig{
			AgentCount:         cfg.AgentCount,
			HeartbeatInterval:  cfg.HeartbeatInterval,
			CommandsPerAgent:   cfg.CommandsPerAgent,
			ConcurrentCommands: cfg.ConcurrentCommands,
		},
	}

	// Create and start agent pool
	pool, err := harness.CreateAgentPool()
	if err != nil {
		t.Fatalf("Failed to create agent pool: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := pool.StartAll(ctx); err != nil {
		t.Fatalf("Failed to start agents: %v", err)
	}

	// Give agents time to fully register
	time.Sleep(500 * time.Millisecond)

	// Get registered agent IDs
	agentIDs := harness.ControlPlane().RegisteredAgentIDs()
	if len(agentIDs) == 0 {
		t.Fatal("No agents registered")
	}

	// Build command list - each agent gets CommandsPerAgent commands
	var commands []string
	for i := 0; i < cfg.CommandsPerAgent; i++ {
		for _, id := range agentIDs {
			commands = append(commands, id)
		}
	}

	// Execute commands
	t.Logf("Sending %d commands with concurrency %d...", len(commands), cfg.ConcurrentCommands)
	cmdStart := time.Now()
	success, failed, latencyCollector := harness.ControlPlane().BroadcastCommand(
		ctx, commands, "echo", []string{"test"}, cfg.CommandTimeout, cfg.ConcurrentCommands)
	cmdDuration := time.Since(cmdStart)

	// Calculate metrics
	min, max, avg, p50, p95, p99 := latencyCollector.Calculate()
	opsPerSecond := float64(success+failed) / cmdDuration.Seconds()

	metrics := Metrics{
		TotalOps:          int64(success + failed),
		SuccessfulOps:     int64(success),
		FailedOps:         int64(failed),
		OpsPerSecond:      opsPerSecond,
		AvgLatency:        avg,
		MinLatency:        min,
		MaxLatency:        max,
		P50Latency:        p50,
		P95Latency:        p95,
		P99Latency:        p99,
		CommandsSent:      int64(success + failed),
		CommandsCompleted: int64(success),
		CommandsFailed:    int64(failed),
		AgentsRegistered:  len(agentIDs),
	}

	if metrics.TotalOps > 0 {
		metrics.ErrorRate = float64(failed) / float64(success+failed) * 100
	}

	result.Metrics = metrics
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = failed == 0 && success > 0

	// Stop agents
	pool.StopAll()

	// Log results
	t.Logf("Command Results:")
	t.Logf("  Total commands: %d", success+failed)
	t.Logf("  Successful: %d", success)
	t.Logf("  Failed: %d", failed)
	t.Logf("  Duration: %v", cmdDuration)
	t.Logf("  Throughput: %.2f ops/sec", opsPerSecond)
	t.Logf("  Avg latency: %v", avg)
	t.Logf("  Min latency: %v", min)
	t.Logf("  Max latency: %v", max)
	t.Logf("  P50 latency: %v", p50)
	t.Logf("  P95 latency: %v", p95)
	t.Logf("  P99 latency: %v", p99)
	t.Logf("  Error rate: %.2f%%", metrics.ErrorRate)

	// Save result
	if cfg.ReportDir != "" {
		if err := SaveResult(result, cfg.ReportDir); err != nil {
			t.Logf("Warning: failed to save result: %v", err)
		}
	}

	if failed > 0 {
		t.Errorf("Command test had %d failures", failed)
	}
}

// TestLoad_RampUp tests gradual agent ramp-up.
func TestLoad_RampUp(t *testing.T) {
	skipIfNotLoadTest(t)

	cfg := ConfigFromEnv()
	t.Logf("Testing ramp-up: %d agents over %v", cfg.AgentCount, cfg.RampUpDuration)

	harness := NewTestHarness(cfg)
	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer harness.Stop()

	result := &LoadTestResult{
		TestName:  "ramp_up",
		StartTime: time.Now(),
		Config: ResultConfig{
			AgentCount:        cfg.AgentCount,
			HeartbeatInterval: cfg.HeartbeatInterval,
			TestDuration:      cfg.RampUpDuration,
		},
	}

	// Create agent pool
	pool, err := harness.CreateAgentPool()
	if err != nil {
		t.Fatalf("Failed to create agent pool: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.RampUpDuration+time.Minute)
	defer cancel()

	// Track registration rate
	checkInterval := cfg.RampUpDuration / 10
	if checkInterval < 100*time.Millisecond {
		checkInterval = 100 * time.Millisecond
	}

	// Start agents with ramp-up in background
	done := make(chan error, 1)
	rampStart := time.Now()
	go func() {
		done <- pool.StartAll(ctx)
	}()

	// Monitor registration progress
	var checkpoints []int
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

monitoring:
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Failed to start agents: %v", err)
			}
			break monitoring
		case <-ticker.C:
			count := harness.ControlPlane().RegisteredAgentCount()
			checkpoints = append(checkpoints, count)
			t.Logf("  Progress: %d/%d agents (%.1f%%)", count, cfg.AgentCount, float64(count)/float64(cfg.AgentCount)*100)
		case <-ctx.Done():
			break monitoring
		}
	}
	rampDuration := time.Since(rampStart)

	// Collect metrics
	metrics := pool.AggregateMetrics()
	metrics.AgentsRegistered = pool.AgentCount()
	metrics.RegistrationTime = rampDuration

	result.Metrics = metrics
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = metrics.AgentsRegistered == cfg.AgentCount

	// Stop agents
	pool.StopAll()

	// Log results
	t.Logf("Ramp-up Results:")
	t.Logf("  Target agents: %d", cfg.AgentCount)
	t.Logf("  Registered: %d", metrics.AgentsRegistered)
	t.Logf("  Ramp-up duration: %v", rampDuration)
	t.Logf("  Rate: %.2f agents/sec", float64(metrics.AgentsRegistered)/rampDuration.Seconds())

	// Save result
	if cfg.ReportDir != "" {
		if err := SaveResult(result, cfg.ReportDir); err != nil {
			t.Logf("Warning: failed to save result: %v", err)
		}
	}

	if !result.Success {
		t.Errorf("Ramp-up failed: expected %d agents, got %d", cfg.AgentCount, metrics.AgentsRegistered)
	}
}

// TestLoad_Sustained tests sustained load over time.
func TestLoad_Sustained(t *testing.T) {
	skipIfNotLoadTest(t)

	cfg := ConfigFromEnv()
	t.Logf("Testing sustained load: %d agents for %v", cfg.AgentCount, cfg.TestDuration)

	harness := NewTestHarness(cfg)
	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}
	defer harness.Stop()

	result := &LoadTestResult{
		TestName:  "sustained",
		StartTime: time.Now(),
		Config: ResultConfig{
			AgentCount:         cfg.AgentCount,
			HeartbeatInterval:  cfg.HeartbeatInterval,
			TestDuration:       cfg.TestDuration,
			CommandsPerAgent:   cfg.CommandsPerAgent,
			ConcurrentCommands: cfg.ConcurrentCommands,
		},
	}

	// Create and start agent pool
	pool, err := harness.CreateAgentPool()
	if err != nil {
		t.Fatalf("Failed to create agent pool: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.TestDuration+2*time.Minute)
	defer cancel()

	if err := pool.StartAll(ctx); err != nil {
		t.Fatalf("Failed to start agents: %v", err)
	}

	// Give agents time to register
	time.Sleep(500 * time.Millisecond)

	agentIDs := harness.ControlPlane().RegisteredAgentIDs()
	if len(agentIDs) == 0 {
		t.Fatal("No agents registered")
	}

	// Run sustained load with periodic commands
	t.Logf("Running sustained load test...")
	cmdInterval := cfg.TestDuration / time.Duration(cfg.CommandsPerAgent)
	if cmdInterval < 100*time.Millisecond {
		cmdInterval = 100 * time.Millisecond
	}

	testStart := time.Now()
	var totalSuccess, totalFailed int
	latencyCollector := NewLatencyCollector()

	ticker := time.NewTicker(cmdInterval)
	defer ticker.Stop()

	endTime := time.Now().Add(cfg.TestDuration)
	for time.Now().Before(endTime) {
		select {
		case <-ticker.C:
			success, failed, lc := harness.ControlPlane().BroadcastCommand(
				ctx, agentIDs, "echo", []string{"test"}, cfg.CommandTimeout, cfg.ConcurrentCommands)
			totalSuccess += success
			totalFailed += failed

			// Merge latencies
			lc.mu.Lock()
			latencyCollector.AddBatch(lc.latencies)
			lc.mu.Unlock()
		case <-ctx.Done():
			break
		}
	}
	testDuration := time.Since(testStart)

	// Collect final metrics
	cpMetrics := harness.ControlPlane().Metrics()
	min, max, avg, p50, p95, p99 := latencyCollector.Calculate()

	metrics := Metrics{
		TotalOps:           int64(totalSuccess + totalFailed),
		SuccessfulOps:      int64(totalSuccess),
		FailedOps:          int64(totalFailed),
		OpsPerSecond:       float64(totalSuccess+totalFailed) / testDuration.Seconds(),
		AvgLatency:         avg,
		MinLatency:         min,
		MaxLatency:         max,
		P50Latency:         p50,
		P95Latency:         p95,
		P99Latency:         p99,
		HeartbeatsSent:     cpMetrics.Heartbeats,
		HeartbeatsReceived: cpMetrics.Heartbeats,
		CommandsSent:       int64(totalSuccess + totalFailed),
		CommandsCompleted:  int64(totalSuccess),
		CommandsFailed:     int64(totalFailed),
		AgentsRegistered:   len(agentIDs),
	}

	if metrics.TotalOps > 0 {
		metrics.ErrorRate = float64(totalFailed) / float64(totalSuccess+totalFailed) * 100
	}

	result.Metrics = metrics
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = totalFailed == 0 && totalSuccess > 0

	// Stop agents
	pool.StopAll()

	// Log results
	t.Logf("Sustained Load Results:")
	t.Logf("  Duration: %v", testDuration)
	t.Logf("  Agents: %d", metrics.AgentsRegistered)
	t.Logf("  Total commands: %d", totalSuccess+totalFailed)
	t.Logf("  Successful: %d", totalSuccess)
	t.Logf("  Failed: %d", totalFailed)
	t.Logf("  Throughput: %.2f ops/sec", metrics.OpsPerSecond)
	t.Logf("  Heartbeats: %d", metrics.HeartbeatsSent)
	t.Logf("  Avg latency: %v", avg)
	t.Logf("  P50 latency: %v", p50)
	t.Logf("  P95 latency: %v", p95)
	t.Logf("  P99 latency: %v", p99)
	t.Logf("  Error rate: %.2f%%", metrics.ErrorRate)

	// Save result
	if cfg.ReportDir != "" {
		if err := SaveResult(result, cfg.ReportDir); err != nil {
			t.Logf("Warning: failed to save result: %v", err)
		}
	}

	if totalFailed > 0 {
		t.Errorf("Sustained test had %d failures", totalFailed)
	}
}
