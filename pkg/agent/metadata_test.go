package agent

import (
	"runtime"
	"testing"
)

func TestCollectMetadata(t *testing.T) {
	metadata, err := CollectMetadata()
	if err != nil {
		t.Fatalf("Failed to collect metadata: %v", err)
	}

	// Verify basic fields
	if metadata.Hostname == "" {
		t.Error("Hostname should not be empty")
	}

	if metadata.OS != runtime.GOOS {
		t.Errorf("Expected OS %s, got %s", runtime.GOOS, metadata.OS)
	}

	if metadata.Architecture != runtime.GOARCH {
		t.Errorf("Expected Architecture %s, got %s", runtime.GOARCH, metadata.Architecture)
	}

	if metadata.AgentVersion == "" {
		t.Error("AgentVersion should not be empty")
	}

	if metadata.Labels == nil {
		t.Error("Labels map should be initialized")
	}

	// IP addresses may be empty in some test environments, so just check it's initialized
	if metadata.IPAddresses == nil {
		t.Error("IPAddresses should be initialized")
	}
}

func TestCollectMetrics(t *testing.T) {
	metrics, err := CollectMetrics()
	if err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	// Verify metrics are initialized
	if metrics == nil {
		t.Fatal("Metrics should not be nil")
	}

	// Verify goroutine count is positive
	if metrics.NumGoroutines <= 0 {
		t.Errorf("Expected positive goroutine count, got %d", metrics.NumGoroutines)
	}

	// Memory usage should be positive
	if metrics.MemoryUsage <= 0 {
		t.Errorf("Expected positive memory usage, got %d", metrics.MemoryUsage)
	}

	// Load average should be initialized (even if zeros in test)
	if metrics.LoadAverage == nil {
		t.Error("LoadAverage should be initialized")
	}
	if len(metrics.LoadAverage) != 3 {
		t.Errorf("Expected 3 load average values, got %d", len(metrics.LoadAverage))
	}

	// CPU percent should be in valid range (0-100)
	if metrics.CPUPercent < 0 || metrics.CPUPercent > 100 {
		t.Errorf("CPU percent should be 0-100, got %f", metrics.CPUPercent)
	}

	// Memory percent should be in valid range (0-100)
	if metrics.MemoryPercent < 0 || metrics.MemoryPercent > 100 {
		t.Errorf("Memory percent should be 0-100, got %f", metrics.MemoryPercent)
	}

	// Disk percent should be in valid range (0-100)
	if metrics.DiskPercent < 0 || metrics.DiskPercent > 100 {
		t.Errorf("Disk percent should be 0-100, got %f", metrics.DiskPercent)
	}

	t.Logf("Metrics: CPU=%.1f%%, Memory=%.1f%%, Disk=%.1f%%, Load=[%.2f, %.2f, %.2f]",
		metrics.CPUPercent, metrics.MemoryPercent, metrics.DiskPercent,
		metrics.LoadAverage[0], metrics.LoadAverage[1], metrics.LoadAverage[2])
}

func TestCollectMetricsNonBlocking(t *testing.T) {
	metrics, err := CollectMetricsNonBlocking()
	if err != nil {
		t.Fatalf("Failed to collect non-blocking metrics: %v", err)
	}

	// Verify metrics are initialized
	if metrics == nil {
		t.Fatal("Metrics should not be nil")
	}

	// Verify goroutine count is positive
	if metrics.NumGoroutines <= 0 {
		t.Errorf("Expected positive goroutine count, got %d", metrics.NumGoroutines)
	}

	// Memory percent should be in valid range (0-100)
	if metrics.MemoryPercent < 0 || metrics.MemoryPercent > 100 {
		t.Errorf("Memory percent should be 0-100, got %f", metrics.MemoryPercent)
	}

	// Load average should be initialized
	if metrics.LoadAverage == nil || len(metrics.LoadAverage) != 3 {
		t.Error("LoadAverage should have 3 values")
	}
}

func TestGetIPAddresses(t *testing.T) {
	ips, err := getIPAddresses()

	// In some test environments, this might fail or return empty
	// So we just verify it doesn't panic and returns a slice
	if err != nil {
		t.Logf("Warning: Failed to get IP addresses: %v", err)
	}

	if ips == nil {
		t.Error("IP addresses should return a slice, even if empty")
	}
}
