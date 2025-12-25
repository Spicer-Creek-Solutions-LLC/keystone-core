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
