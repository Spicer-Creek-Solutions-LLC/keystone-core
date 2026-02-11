package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()

	if cmd == nil {
		t.Fatal("newRootCmd should not return nil")
	}

	if cmd.Use != "kscore-proxy" {
		t.Errorf("Use = %v, want kscore-proxy", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cmd := newRootCmd()

	expectedSubcommands := []string{
		"device",
		"credential",
		"discover",
		"drift",
		"state",
		"status",
		"version",
	}

	for _, expected := range expectedSubcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", expected)
		}
	}
}

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd()

	if cmd == nil {
		t.Fatal("newVersionCmd should not return nil")
	}
	if cmd.Use != "version" {
		t.Errorf("Use = %v, want version", cmd.Use)
	}

	// Test execution
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("version command should produce output")
	}
}

func TestNewDeviceCmd(t *testing.T) {
	cmd := newDeviceCmd()

	if cmd == nil {
		t.Fatal("newDeviceCmd should not return nil")
	}
	if cmd.Use != "device" {
		t.Errorf("Use = %v, want device", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"list", "add", "import", "show", "update", "remove", "test", "health", "ping", "status", "config", "connect"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestNewDeviceListCmd(t *testing.T) {
	cmd := newDeviceListCmd()

	if cmd == nil {
		t.Fatal("newDeviceListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	// Check flags exist
	flags := []string{"proxy", "vendor", "type", "status"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewDeviceAddCmd(t *testing.T) {
	cmd := newDeviceAddCmd()

	if cmd == nil {
		t.Fatal("newDeviceAddCmd should not return nil")
	}
	if cmd.Use != "add" {
		t.Errorf("Use = %v, want add", cmd.Use)
	}

	// Check flags exist
	flags := []string{"name", "address", "protocol", "vendor", "type", "profile", "credential", "labels"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewCredentialCmd(t *testing.T) {
	cmd := newCredentialCmd()

	if cmd == nil {
		t.Fatal("newCredentialCmd should not return nil")
	}
	if cmd.Use != "credential" {
		t.Errorf("Use = %v, want credential", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"add", "list", "show", "test", "rotate", "delete", "verify", "backend-status"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestNewDiscoverCmd(t *testing.T) {
	cmd := newDiscoverCmd()

	if cmd == nil {
		t.Fatal("newDiscoverCmd should not return nil")
	}
	if cmd.Use != "discover" {
		t.Errorf("Use = %v, want discover", cmd.Use)
	}

	// Check aliases
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "discovery" {
		t.Error("expected alias 'discovery' not found")
	}

	// Should have subcommands
	subcommands := []string{"scan", "list", "status", "approve", "approve-all", "reject", "ignore", "auto-approve", "logs", "config"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestDiscoverScanFlags(t *testing.T) {
	cmd := newDiscoverScanCmd()

	expected := []string{"network", "subnet", "networks", "protocols", "ports", "timeout", "workers", "debug"}
	for _, flag := range expected {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found on discover scan", flag)
		}
	}
}

func TestDiscoverScanWithNetwork(t *testing.T) {
	cmd := newDiscoverCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"scan", "--network", "192.168.1.0/24"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "192.168.1.0/24") {
		t.Errorf("output should contain the scanned network, got:\n%s", out)
	}
}

func TestDiscoverScanWithSubnet(t *testing.T) {
	cmd := newDiscoverCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"scan", "--subnet", "10.0.0.0/24"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "10.0.0.0/24") {
		t.Errorf("output should contain the scanned subnet, got:\n%s", out)
	}
}

func TestDiscoverScanWithProtocolsAndPorts(t *testing.T) {
	cmd := newDiscoverCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"scan", "--network", "192.168.1.0/24", "--protocols", "ssh,snmp", "--ports", "22,161"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Protocols: ssh, snmp") {
		t.Errorf("output should show protocols, got:\n%s", out)
	}
	if !strings.Contains(out, "Ports: 22, 161") {
		t.Errorf("output should show ports, got:\n%s", out)
	}
}

func TestDiscoverScanShowsTimeoutAndWorkers(t *testing.T) {
	cmd := newDiscoverCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"scan", "--network", "192.168.1.0/24", "--timeout", "10s", "--workers", "50"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Timeout: 10s") {
		t.Errorf("output should show timeout, got:\n%s", out)
	}
	if !strings.Contains(out, "Workers: 50") {
		t.Errorf("output should show workers, got:\n%s", out)
	}
}

func TestDiscoverScanDebugProtocols(t *testing.T) {
	cmd := newDiscoverCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"scan", "--network", "192.168.1.0/24", "--protocols", "ssh,rest", "--debug"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[DEBUG] Starting SSH scan") {
		t.Errorf("debug output should list SSH protocol, got:\n%s", out)
	}
	if !strings.Contains(out, "[DEBUG] Starting REST scan") {
		t.Errorf("debug output should list REST protocol, got:\n%s", out)
	}
}

func TestDiscoverScanRequiresNetwork(t *testing.T) {
	cmd := newDiscoverCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"scan"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no network specified")
	}
}

func TestDiscoverScanSubnetNetworkMutuallyExclusive(t *testing.T) {
	cmd := newDiscoverCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"scan", "--network", "192.168.1.0/24", "--subnet", "10.0.0.0/24"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when both --network and --subnet specified")
	}
}

func TestNewDriftCmd(t *testing.T) {
	cmd := newDriftCmd()

	if cmd == nil {
		t.Fatal("newDriftCmd should not return nil")
	}
	if cmd.Use != "drift" {
		t.Errorf("Use = %v, want drift", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"check", "report", "remediate"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestNewStateCmd(t *testing.T) {
	cmd := newStateCmd()

	if cmd == nil {
		t.Fatal("newStateCmd should not return nil")
	}
	if cmd.Use != "state" {
		t.Errorf("Use = %v, want state", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"apply", "check", "logs"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestStateCheckFlags(t *testing.T) {
	cmd := newStateCheckCmd()

	for _, flag := range []string{"device", "target"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found on state check", flag)
		}
	}
}

func TestStateCheckRequiresStateFile(t *testing.T) {
	cmd := newStateCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"check"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no state file provided")
	}
}

func TestStateCheckWithDevice(t *testing.T) {
	cmd := newStateCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"check", "network-baseline.yaml", "--device", "router-01"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "network-baseline.yaml") {
		t.Errorf("output should show state file name, got:\n%s", out)
	}
	if !strings.Contains(out, "Device: router-01") {
		t.Errorf("output should show device filter, got:\n%s", out)
	}
	if !strings.Contains(out, "compliant") {
		t.Errorf("output should show compliance results, got:\n%s", out)
	}
}

func TestStateCheckWithTarget(t *testing.T) {
	cmd := newStateCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"check", "network-baseline.yaml", "--target", "vendor:cisco"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Target: vendor:cisco") {
		t.Errorf("output should show target expression, got:\n%s", out)
	}
}

func TestProxyDeviceStructure(t *testing.T) {
	now := time.Now()
	device := ProxyDevice{
		ID:         "dev-001",
		Name:       "core-router-01",
		Address:    "192.168.1.1",
		Protocol:   "ssh",
		Vendor:     "cisco",
		DeviceType: "router",
		Profile:    "cisco_ios",
		Credential: "cisco-ssh",
		ProxyAgent: "proxy-agent-01",
		Labels:     map[string]string{"env": "prod"},
		Status:     "connected",
		Health:     "healthy",
		LastSeen:   now,
		CreatedAt:  now,
	}

	if device.ID != "dev-001" {
		t.Errorf("ID = %v, want dev-001", device.ID)
	}
	if device.Protocol != "ssh" {
		t.Errorf("Protocol = %v, want ssh", device.Protocol)
	}
	if device.Health != "healthy" {
		t.Errorf("Health = %v, want healthy", device.Health)
	}
}

func TestProxyCredentialStructure(t *testing.T) {
	now := time.Now()
	cred := ProxyCredential{
		ID:          "cred-001",
		Name:        "cisco-ssh",
		Type:        "ssh-password",
		Username:    "admin",
		Protocol:    "ssh",
		DeviceTypes: []string{"router", "switch"},
		Backend:     "vault",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if cred.ID != "cred-001" {
		t.Errorf("ID = %v, want cred-001", cred.ID)
	}
	if cred.Type != "ssh-password" {
		t.Errorf("Type = %v, want ssh-password", cred.Type)
	}
	if cred.Backend != "vault" {
		t.Errorf("Backend = %v, want vault", cred.Backend)
	}
	if len(cred.DeviceTypes) != 2 {
		t.Errorf("DeviceTypes count = %d, want 2", len(cred.DeviceTypes))
	}
}

func TestDiscoveredDeviceStructure(t *testing.T) {
	now := time.Now()
	device := DiscoveredDevice{
		ID:              "disc-001",
		Address:         "192.168.1.1",
		Hostname:        "router-01",
		Vendor:          "cisco",
		Model:           "ISR4321",
		Profile:         "cisco_ios",
		Status:          "pending",
		DiscoveredAt:    now,
		DiscoveryMethod: "ssh-scan",
	}

	if device.ID != "disc-001" {
		t.Errorf("ID = %v, want disc-001", device.ID)
	}
	if device.Status != "pending" {
		t.Errorf("Status = %v, want pending", device.Status)
	}
	if device.DiscoveryMethod != "ssh-scan" {
		t.Errorf("DiscoveryMethod = %v, want ssh-scan", device.DiscoveryMethod)
	}
}

func TestDriftResultStructure(t *testing.T) {
	now := time.Now()
	result := DriftResult{
		DeviceID:   "dev-001",
		DeviceName: "core-router-01",
		HasDrift:   true,
		DriftCount: 2,
		Severity:   "medium",
		Items: []DriftItem{
			{Path: "hostname", Expected: "router-01", Actual: "ROUTER-01", Severity: "low"},
		},
		CheckedAt: now,
	}

	if result.DeviceID != "dev-001" {
		t.Errorf("DeviceID = %v, want dev-001", result.DeviceID)
	}
	if !result.HasDrift {
		t.Error("HasDrift should be true")
	}
	if result.DriftCount != 2 {
		t.Errorf("DriftCount = %d, want 2", result.DriftCount)
	}
	if len(result.Items) != 1 {
		t.Errorf("Items count = %d, want 1", len(result.Items))
	}
}

func TestDriftItemStructure(t *testing.T) {
	item := DriftItem{
		Path:     "ntp.server",
		Expected: "10.0.0.1",
		Actual:   "10.0.0.2",
		Severity: "medium",
	}

	if item.Path != "ntp.server" {
		t.Errorf("Path = %v, want ntp.server", item.Path)
	}
	if item.Expected != "10.0.0.1" {
		t.Errorf("Expected = %v, want 10.0.0.1", item.Expected)
	}
	if item.Actual != "10.0.0.2" {
		t.Errorf("Actual = %v, want 10.0.0.2", item.Actual)
	}
	if item.Severity != "medium" {
		t.Errorf("Severity = %v, want medium", item.Severity)
	}
}

func TestNewStatusCmd(t *testing.T) {
	cmd := newStatusCmd()

	if cmd == nil {
		t.Fatal("newStatusCmd should not return nil")
	}
	if cmd.Use != "status" {
		t.Errorf("Use = %v, want status", cmd.Use)
	}
}
