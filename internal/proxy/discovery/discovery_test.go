package discovery

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestNewDiscovery(t *testing.T) {
	config := DefaultConfig()
	d := NewDiscovery(config)

	if d == nil {
		t.Fatal("NewDiscovery returned nil")
	}

	if d.scanners == nil {
		t.Error("scanners map is nil")
	}

	if d.discovered == nil {
		t.Error("discovered map is nil")
	}

	if d.approved == nil {
		t.Error("approved map is nil")
	}
}

func TestDefaultDiscoveryConfig(t *testing.T) {
	config := DefaultConfig()

	if config.ScanInterval != 1*time.Hour {
		t.Errorf("expected ScanInterval 1h, got %v", config.ScanInterval)
	}

	if config.ScanTimeout != 30*time.Second {
		t.Errorf("expected ScanTimeout 30s, got %v", config.ScanTimeout)
	}

	if config.MaxConcurrent != 50 {
		t.Errorf("expected MaxConcurrent 50, got %d", config.MaxConcurrent)
	}

	if config.AutoApprove {
		t.Error("expected AutoApprove false")
	}

	if config.SSHPort != 22 {
		t.Errorf("expected SSHPort 22, got %d", config.SSHPort)
	}

	if config.SNMPPort != 161 {
		t.Errorf("expected SNMPPort 161, got %d", config.SNMPPort)
	}

	if config.SNMPCommunity != "public" {
		t.Errorf("expected SNMPCommunity 'public', got %s", config.SNMPCommunity)
	}
}

func TestDiscovery_RegisterScanner(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	scanner := NewSSHScanner(22, 5*time.Second, 10)
	d.RegisterScanner(scanner)

	if len(d.scanners) != 1 {
		t.Errorf("expected 1 scanner, got %d", len(d.scanners))
	}

	if d.scanners["ssh"] == nil {
		t.Error("ssh scanner not registered")
	}
}

func TestDiscovery_RegisterMatcher(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	matcher := NewPatternMatcher()
	d.RegisterMatcher(matcher)

	if len(d.matchers) != 1 {
		t.Errorf("expected 1 matcher, got %d", len(d.matchers))
	}
}

func TestDiscovery_ApproveRejectIgnore(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	// Add a discovered device
	device := &DiscoveredDevice{
		ID:       "test-device-1",
		Address:  "192.168.1.1",
		Port:     22,
		Protocol: ProtocolSSH,
		Status:   StatusPending,
	}
	d.discovered["test-device-1"] = device

	// Test Approve
	err := d.Approve("test-device-1")
	if err != nil {
		t.Errorf("Approve failed: %v", err)
	}

	if device.Status != StatusApproved {
		t.Errorf("expected status approved, got %s", device.Status)
	}

	if d.approved["test-device-1"] == nil {
		t.Error("device not in approved map")
	}

	// Reset status
	device.Status = StatusPending
	delete(d.approved, "test-device-1")

	// Test Reject
	err = d.Reject("test-device-1")
	if err != nil {
		t.Errorf("Reject failed: %v", err)
	}

	if device.Status != StatusRejected {
		t.Errorf("expected status rejected, got %s", device.Status)
	}

	// Reset status
	device.Status = StatusPending

	// Test Ignore
	err = d.Ignore("test-device-1")
	if err != nil {
		t.Errorf("Ignore failed: %v", err)
	}

	if device.Status != StatusIgnored {
		t.Errorf("expected status ignored, got %s", device.Status)
	}
}

func TestDiscovery_Remove(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	device := &DiscoveredDevice{
		ID:      "test-device-1",
		Address: "192.168.1.1",
		Status:  StatusPending,
	}
	d.discovered["test-device-1"] = device
	d.approved["test-device-1"] = device

	err := d.Remove("test-device-1")
	if err != nil {
		t.Errorf("Remove failed: %v", err)
	}

	if d.discovered["test-device-1"] != nil {
		t.Error("device still in discovered map")
	}

	if d.approved["test-device-1"] != nil {
		t.Error("device still in approved map")
	}
}

func TestDiscovery_GetDiscovered(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	d.discovered["device-1"] = &DiscoveredDevice{ID: "device-1"}
	d.discovered["device-2"] = &DiscoveredDevice{ID: "device-2"}

	devices := d.GetDiscovered()
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}
}

func TestDiscovery_GetPending(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	d.discovered["device-1"] = &DiscoveredDevice{ID: "device-1", Status: StatusPending}
	d.discovered["device-2"] = &DiscoveredDevice{ID: "device-2", Status: StatusApproved}
	d.discovered["device-3"] = &DiscoveredDevice{ID: "device-3", Status: StatusPending}

	pending := d.GetPending()
	if len(pending) != 2 {
		t.Errorf("expected 2 pending devices, got %d", len(pending))
	}
}

func TestDiscovery_GetApproved(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	d.approved["device-1"] = &DiscoveredDevice{ID: "device-1", Status: StatusApproved}
	d.approved["device-2"] = &DiscoveredDevice{ID: "device-2", Status: StatusApproved}

	approved := d.GetApproved()
	if len(approved) != 2 {
		t.Errorf("expected 2 approved devices, got %d", len(approved))
	}
}

func TestDiscovery_expandNetworks(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	// Test single IP
	targets, err := d.expandNetworks([]string{"192.168.1.1"})
	if err != nil {
		t.Errorf("expandNetworks failed: %v", err)
	}
	if len(targets) != 1 || targets[0] != "192.168.1.1" {
		t.Errorf("expected [192.168.1.1], got %v", targets)
	}

	// Test CIDR (small subnet)
	targets, err = d.expandNetworks([]string{"192.168.1.0/30"})
	if err != nil {
		t.Errorf("expandNetworks failed: %v", err)
	}
	if len(targets) != 4 {
		t.Errorf("expected 4 targets, got %d", len(targets))
	}

	// Test invalid network
	_, err = d.expandNetworks([]string{"invalid"})
	if err == nil {
		t.Error("expected error for invalid network")
	}
}

func TestDiscovery_filterExcluded(t *testing.T) {
	config := DefaultConfig()
	config.ExcludeHosts = []string{"192.168.1.1", "192.168.1.5"}
	d := NewDiscovery(config)

	targets := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3", "192.168.1.5"}
	filtered := d.filterExcluded(targets)

	if len(filtered) != 2 {
		t.Errorf("expected 2 targets after filtering, got %d", len(filtered))
	}

	for _, t := range filtered {
		if t == "192.168.1.1" || t == "192.168.1.5" {
			t := t
			_ = t // Avoid unused variable
		}
	}
}

func TestDiscovery_ErrorCases(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	// Test approving non-existent device
	err := d.Approve("non-existent")
	if err == nil {
		t.Error("expected error when approving non-existent device")
	}

	// Test rejecting non-existent device
	err = d.Reject("non-existent")
	if err == nil {
		t.Error("expected error when rejecting non-existent device")
	}

	// Test ignoring non-existent device
	err = d.Ignore("non-existent")
	if err == nil {
		t.Error("expected error when ignoring non-existent device")
	}
}

func TestIncrementIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.1", "192.168.1.2"},
		{"192.168.1.255", "192.168.2.0"},
		{"10.0.0.0", "10.0.0.1"},
	}

	for _, test := range tests {
		ip := net.ParseIP(test.input).To4()
		incrementIP(ip)
		result := ip.String()
		if result != test.expected {
			t.Errorf("incrementIP(%s) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

// Mock scanner for testing
type mockScanner struct {
	name    string
	devices []*DiscoveredDevice
	err     error
}

func (m *mockScanner) Name() string {
	return m.name
}

func (m *mockScanner) Scan(ctx context.Context, targets []string) ([]*DiscoveredDevice, error) {
	return m.devices, m.err
}

func TestDiscovery_Scan(t *testing.T) {
	config := DefaultConfig()
	config.Networks = []string{"192.168.1.1"}
	d := NewDiscovery(config)

	// Add mock scanner
	mockDevices := []*DiscoveredDevice{
		{
			ID:       "test-1",
			Address:  "192.168.1.1",
			Protocol: ProtocolSSH,
		},
	}
	d.RegisterScanner(&mockScanner{name: "test", devices: mockDevices})

	// Add matcher
	matcher := NewPatternMatcher()
	d.RegisterMatcher(matcher)

	// Run scan
	result, err := d.Scan(context.Background())
	if err != nil {
		t.Errorf("Scan failed: %v", err)
	}

	if result.TotalDiscovered != 1 {
		t.Errorf("expected 1 discovered device, got %d", result.TotalDiscovered)
	}

	if result.NewDevices != 1 {
		t.Errorf("expected 1 new device, got %d", result.NewDevices)
	}
}

func TestDiscoveryEventTypes(t *testing.T) {
	// Test event type constants
	if EventDeviceDiscovered != "device.discovered" {
		t.Errorf("unexpected event type: %s", EventDeviceDiscovered)
	}

	if EventDeviceApproved != "device.approved" {
		t.Errorf("unexpected event type: %s", EventDeviceApproved)
	}

	if EventScanStarted != "scan.started" {
		t.Errorf("unexpected event type: %s", EventScanStarted)
	}

	if EventScanCompleted != "scan.completed" {
		t.Errorf("unexpected event type: %s", EventScanCompleted)
	}
}

func TestDiscoveryStatusConstants(t *testing.T) {
	if StatusPending != "pending" {
		t.Errorf("unexpected status: %s", StatusPending)
	}

	if StatusApproved != "approved" {
		t.Errorf("unexpected status: %s", StatusApproved)
	}

	if StatusRejected != "rejected" {
		t.Errorf("unexpected status: %s", StatusRejected)
	}

	if StatusIgnored != "ignored" {
		t.Errorf("unexpected status: %s", StatusIgnored)
	}
}

func TestDiscoveryProtocolConstants(t *testing.T) {
	if ProtocolSSH != "ssh" {
		t.Errorf("unexpected protocol: %s", ProtocolSSH)
	}

	if ProtocolSNMP != "snmp" {
		t.Errorf("unexpected protocol: %s", ProtocolSNMP)
	}

	if ProtocolHTTP != "http" {
		t.Errorf("unexpected protocol: %s", ProtocolHTTP)
	}

	if ProtocolHTTPS != "https" {
		t.Errorf("unexpected protocol: %s", ProtocolHTTPS)
	}

	if ProtocolWinRM != "winrm" {
		t.Errorf("unexpected protocol: %s", ProtocolWinRM)
	}
}

func TestDeviceTypeConstants(t *testing.T) {
	if DeviceTypeRouter != "router" {
		t.Errorf("unexpected device type: %s", DeviceTypeRouter)
	}

	if DeviceTypeSwitch != "switch" {
		t.Errorf("unexpected device type: %s", DeviceTypeSwitch)
	}

	if DeviceTypeFirewall != "firewall" {
		t.Errorf("unexpected device type: %s", DeviceTypeFirewall)
	}

	if DeviceTypeServer != "server" {
		t.Errorf("unexpected device type: %s", DeviceTypeServer)
	}
}
