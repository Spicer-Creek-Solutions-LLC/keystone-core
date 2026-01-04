package agent

import (
	"net"
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

func TestCollectNetworkInfo(t *testing.T) {
	info, err := CollectNetworkInfo()
	if err != nil {
		t.Logf("Warning: Failed to collect network info: %v", err)
	}

	if info == nil {
		t.Fatal("NetworkInfo should not be nil")
	}

	// All slices should be initialized (even if empty)
	if info.IPv4Addresses == nil {
		t.Error("IPv4Addresses should be initialized")
	}
	if info.IPv6Addresses == nil {
		t.Error("IPv6Addresses should be initialized")
	}
	if info.AllAddresses == nil {
		t.Error("AllAddresses should be initialized")
	}

	// AllAddresses should equal IPv4 + IPv6
	expectedLen := len(info.IPv4Addresses) + len(info.IPv6Addresses)
	if len(info.AllAddresses) != expectedLen {
		t.Errorf("AllAddresses length mismatch: got %d, want %d (IPv4=%d, IPv6=%d)",
			len(info.AllAddresses), expectedLen,
			len(info.IPv4Addresses), len(info.IPv6Addresses))
	}

	// IsDualStack should be true if both families have addresses
	expectedDualStack := len(info.IPv4Addresses) > 0 && len(info.IPv6Addresses) > 0
	if info.IsDualStack != expectedDualStack {
		t.Errorf("IsDualStack = %v, want %v", info.IsDualStack, expectedDualStack)
	}

	t.Logf("Network info: IPv4=%v, IPv6=%v, IsDualStack=%v",
		info.IPv4Addresses, info.IPv6Addresses, info.IsDualStack)
}

func TestCollectMetadata_IPv6Fields(t *testing.T) {
	metadata, err := CollectMetadata()
	if err != nil {
		t.Fatalf("Failed to collect metadata: %v", err)
	}

	// Verify new IPv6 fields are initialized
	if metadata.IPv4Addresses == nil {
		t.Error("IPv4Addresses should be initialized")
	}
	if metadata.IPv6Addresses == nil {
		t.Error("IPv6Addresses should be initialized")
	}

	// IPAddresses (backward compat) should be populated
	if metadata.IPAddresses == nil {
		t.Error("IPAddresses should be initialized")
	}

	// IsDualStack should be consistent with addresses
	expectedDualStack := len(metadata.IPv4Addresses) > 0 && len(metadata.IPv6Addresses) > 0
	if metadata.IsDualStack != expectedDualStack {
		t.Errorf("IsDualStack = %v, want %v", metadata.IsDualStack, expectedDualStack)
	}

	t.Logf("Metadata: IPv4=%v, IPv6=%v, IsDualStack=%v",
		metadata.IPv4Addresses, metadata.IPv6Addresses, metadata.IsDualStack)
}

func TestIsIPv4LinkLocal(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"link-local low", "169.254.0.1", true},
		{"link-local high", "169.254.255.255", true},
		{"link-local mid", "169.254.100.50", true},
		{"not link-local 10.x", "10.0.0.1", false},
		{"not link-local 192.168", "192.168.1.1", false},
		{"not link-local 172.x", "172.16.0.1", false},
		{"public address", "8.8.8.8", false},
		{"loopback", "127.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIPForTest(t, tt.ip)
			got := isIPv4LinkLocal(ip)
			if got != tt.want {
				t.Errorf("isIPv4LinkLocal(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsIPv6LinkLocal(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"link-local fe80::", "fe80::1", true},
		{"link-local fe80::abcd", "fe80::abcd:1234", true},
		{"link-local febf::", "febf::1", true},
		{"not link-local 2001:db8", "2001:db8::1", false},
		{"not link-local ::1", "::1", false},
		{"not link-local ff00 multicast", "ff00::1", false},
		{"not link-local fc00 ULA", "fc00::1", false},
		{"not link-local fd00 ULA", "fd00::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIPForTest(t, tt.ip)
			got := isIPv6LinkLocal(ip)
			if got != tt.want {
				t.Errorf("isIPv6LinkLocal(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// parseIPForTest is a test helper to parse IP addresses
func parseIPForTest(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("Failed to parse IP: %s", s)
	}
	return ip
}
