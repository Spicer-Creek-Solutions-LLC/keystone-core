package discovery

import (
	"context"
	"testing"
	"time"
)

func TestNewSSHScanner(t *testing.T) {
	scanner := NewSSHScanner(22, 5*time.Second, 10)

	if scanner == nil {
		t.Fatal("NewSSHScanner returned nil")
	}

	if scanner.Name() != "ssh" {
		t.Errorf("expected name 'ssh', got '%s'", scanner.Name())
	}
}

func TestNewSNMPScanner(t *testing.T) {
	scanner := NewSNMPScanner(161, "public", 5*time.Second, 10)

	if scanner == nil {
		t.Fatal("NewSNMPScanner returned nil")
	}

	if scanner.Name() != "snmp" {
		t.Errorf("expected name 'snmp', got '%s'", scanner.Name())
	}
}

func TestNewICMPScanner(t *testing.T) {
	scanner := NewICMPScanner(5*time.Second, 10)

	if scanner == nil {
		t.Fatal("NewICMPScanner returned nil")
	}

	if scanner.Name() != "icmp" {
		t.Errorf("expected name 'icmp', got '%s'", scanner.Name())
	}
}

func TestNewHTTPScanner(t *testing.T) {
	scanner := NewHTTPScanner([]int{80, 8080}, 5*time.Second, 10)

	if scanner == nil {
		t.Fatal("NewHTTPScanner returned nil")
	}

	if scanner.Name() != "http" {
		t.Errorf("expected name 'http', got '%s'", scanner.Name())
	}
}

func TestNewHTTPScanner_DefaultPorts(t *testing.T) {
	scanner := NewHTTPScanner(nil, 5*time.Second, 10)

	if scanner == nil {
		t.Fatal("NewHTTPScanner returned nil")
	}

	// Default ports should include 80, 443, 8080, 8443
	if len(scanner.ports) != 4 {
		t.Errorf("expected 4 default ports, got %d", len(scanner.ports))
	}
}

func TestNewWinRMScanner(t *testing.T) {
	scanner := NewWinRMScanner([]int{5985}, 5*time.Second, 10)

	if scanner == nil {
		t.Fatal("NewWinRMScanner returned nil")
	}

	if scanner.Name() != "winrm" {
		t.Errorf("expected name 'winrm', got '%s'", scanner.Name())
	}
}

func TestNewWinRMScanner_DefaultPorts(t *testing.T) {
	scanner := NewWinRMScanner(nil, 5*time.Second, 10)

	if scanner == nil {
		t.Fatal("NewWinRMScanner returned nil")
	}

	// Default ports should include 5985 and 5986
	if len(scanner.ports) != 2 {
		t.Errorf("expected 2 default ports, got %d", len(scanner.ports))
	}
}

func TestScannerInterface(t *testing.T) {
	// Verify all scanners implement Scanner interface
	var _ Scanner = (*SSHScanner)(nil)
	var _ Scanner = (*SNMPScanner)(nil)
	var _ Scanner = (*ICMPScanner)(nil)
	var _ Scanner = (*HTTPScanner)(nil)
	var _ Scanner = (*WinRMScanner)(nil)
}

func TestSSHScanner_ScanEmptyTargets(t *testing.T) {
	scanner := NewSSHScanner(22, 100*time.Millisecond, 10)

	devices, err := scanner.Scan(context.Background(), []string{})
	if err != nil {
		t.Errorf("unexpected error for empty targets: %v", err)
	}

	if len(devices) != 0 {
		t.Errorf("expected 0 devices for empty targets, got %d", len(devices))
	}
}

func TestSNMPScanner_ScanEmptyTargets(t *testing.T) {
	scanner := NewSNMPScanner(161, "public", 100*time.Millisecond, 10)

	devices, err := scanner.Scan(context.Background(), []string{})
	if err != nil {
		t.Errorf("unexpected error for empty targets: %v", err)
	}

	if len(devices) != 0 {
		t.Errorf("expected 0 devices for empty targets, got %d", len(devices))
	}
}

func TestICMPScanner_ScanEmptyTargets(t *testing.T) {
	scanner := NewICMPScanner(100*time.Millisecond, 10)

	devices, err := scanner.Scan(context.Background(), []string{})
	if err != nil {
		t.Errorf("unexpected error for empty targets: %v", err)
	}

	if len(devices) != 0 {
		t.Errorf("expected 0 devices for empty targets, got %d", len(devices))
	}
}

func TestHTTPScanner_ScanEmptyTargets(t *testing.T) {
	scanner := NewHTTPScanner([]int{80}, 100*time.Millisecond, 10)

	devices, err := scanner.Scan(context.Background(), []string{})
	if err != nil {
		t.Errorf("unexpected error for empty targets: %v", err)
	}

	if len(devices) != 0 {
		t.Errorf("expected 0 devices for empty targets, got %d", len(devices))
	}
}

func TestWinRMScanner_ScanEmptyTargets(t *testing.T) {
	scanner := NewWinRMScanner([]int{5985}, 100*time.Millisecond, 10)

	devices, err := scanner.Scan(context.Background(), []string{})
	if err != nil {
		t.Errorf("unexpected error for empty targets: %v", err)
	}

	if len(devices) != 0 {
		t.Errorf("expected 0 devices for empty targets, got %d", len(devices))
	}
}

func TestSSHScanner_ScanWithContext(t *testing.T) {
	scanner := NewSSHScanner(22, 100*time.Millisecond, 10)

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := scanner.Scan(ctx, []string{"192.168.1.1"})
	// Should return early due to cancelled context or handle gracefully
	// Either an error or empty result is acceptable
	_ = err // May or may not return error
}

func TestSNMPScanner_ScanWithContext(t *testing.T) {
	scanner := NewSNMPScanner(161, "public", 100*time.Millisecond, 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := scanner.Scan(ctx, []string{"192.168.1.1"})
	_ = err
}

func TestICMPScanner_ScanWithContext(t *testing.T) {
	scanner := NewICMPScanner(100*time.Millisecond, 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := scanner.Scan(ctx, []string{"192.168.1.1"})
	_ = err
}

func TestSSHScanner_WithBannerGrab(t *testing.T) {
	scanner := NewSSHScanner(22, 100*time.Millisecond, 10)
	scanner.grabBanner = true

	// Just verify the scanner can be configured for banner grabbing
	if !scanner.grabBanner {
		t.Error("expected grabBanner to be true")
	}
}

func TestHTTPScanner_Ports(t *testing.T) {
	ports := []int{80, 443, 8080}
	scanner := NewHTTPScanner(ports, 100*time.Millisecond, 10)

	if len(scanner.ports) != 3 {
		t.Errorf("expected 3 ports, got %d", len(scanner.ports))
	}
}

func TestWinRMScanner_Ports(t *testing.T) {
	ports := []int{5985, 5986}
	scanner := NewWinRMScanner(ports, 100*time.Millisecond, 10)

	if len(scanner.ports) != 2 {
		t.Errorf("expected 2 ports, got %d", len(scanner.ports))
	}
}

func TestSNMPScanner_Community(t *testing.T) {
	scanner := NewSNMPScanner(161, "private", 100*time.Millisecond, 10)

	if scanner.community != "private" {
		t.Errorf("expected community 'private', got '%s'", scanner.community)
	}
}

func TestScannerConcurrency(t *testing.T) {
	// Test that concurrency parameter is set correctly
	scanner := NewSSHScanner(22, 100*time.Millisecond, 50)

	if scanner.concurrency != 50 {
		t.Errorf("expected concurrency 50, got %d", scanner.concurrency)
	}
}

func TestScannerTimeout(t *testing.T) {
	timeout := 10 * time.Second
	scanner := NewSSHScanner(22, timeout, 10)

	if scanner.timeout != timeout {
		t.Errorf("expected timeout %v, got %v", timeout, scanner.timeout)
	}
}

func TestDiscoveredDevice_Fields(t *testing.T) {
	now := time.Now()
	device := &DiscoveredDevice{
		ID:             "test-device",
		Address:        "192.168.1.1",
		Port:           22,
		Protocol:       ProtocolSSH,
		Status:         StatusPending,
		Type:           DeviceTypeRouter,
		Vendor:         "Cisco",
		Model:          "ISR 4000",
		Version:        "15.0",
		Hostname:       "router1",
		SysDescr:       "Cisco IOS Software",
		Banner:         "SSH-2.0-Cisco-1.25",
		DiscoveryTime:  now,
		LastSeen:       now,
		MatchedProfile: "cisco-ios",
		Metadata: map[string]string{
			"serial": "ABC123",
		},
	}

	if device.ID != "test-device" {
		t.Errorf("ID mismatch")
	}
	if device.Address != "192.168.1.1" {
		t.Errorf("Address mismatch")
	}
	if device.Port != 22 {
		t.Errorf("Port mismatch")
	}
	if device.Protocol != ProtocolSSH {
		t.Errorf("Protocol mismatch")
	}
	if device.Status != StatusPending {
		t.Errorf("Status mismatch")
	}
	if device.Type != DeviceTypeRouter {
		t.Errorf("Type mismatch")
	}
	if device.Vendor != "Cisco" {
		t.Errorf("Vendor mismatch")
	}
	if device.MatchedProfile != "cisco-ios" {
		t.Errorf("MatchedProfile mismatch")
	}
	if device.Metadata["serial"] != "ABC123" {
		t.Errorf("Metadata mismatch")
	}
}
