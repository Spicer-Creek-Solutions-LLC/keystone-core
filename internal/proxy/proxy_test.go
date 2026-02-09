package proxy

import (
	"context"
	"errors"
	"testing"
	"time"
)

// =============================================================================
// Types Tests
// =============================================================================

func TestDeviceType_Valid(t *testing.T) {
	tests := []struct {
		deviceType DeviceType
		valid      bool
	}{
		{DeviceTypeLinux, true},
		{DeviceTypeWindows, true},
		{DeviceTypeNetwork, true},
		{DeviceTypeFirewall, true},
		{DeviceTypeRouter, true},
		{DeviceTypeSwitch, true},
		{DeviceTypeAPM, true},
		{DeviceTypeIoT, true},
		{DeviceTypeCustom, true},
		{DeviceType("invalid"), false},
		{DeviceType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.deviceType), func(t *testing.T) {
			if got := tt.deviceType.Valid(); got != tt.valid {
				t.Errorf("DeviceType(%q).Valid() = %v, want %v", tt.deviceType, got, tt.valid)
			}
		})
	}
}

func TestProtocolType_Valid(t *testing.T) {
	tests := []struct {
		protocol ProtocolType
		valid    bool
	}{
		{ProtocolSSH, true},
		{ProtocolSNMP, true},
		{ProtocolREST, true},
		{ProtocolWinRM, true},
		{ProtocolType("invalid"), false},
		{ProtocolType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			if got := tt.protocol.Valid(); got != tt.valid {
				t.Errorf("ProtocolType(%q).Valid() = %v, want %v", tt.protocol, got, tt.valid)
			}
		})
	}
}

func TestDeviceStatus_IsHealthy(t *testing.T) {
	tests := []struct {
		status  DeviceStatus
		healthy bool
	}{
		{DeviceStatusOnline, true},
		{DeviceStatusOffline, false},
		{DeviceStatusDegraded, false},
		{DeviceStatusUnreachable, false},
		{DeviceStatusUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsHealthy(); got != tt.healthy {
				t.Errorf("DeviceStatus(%q).IsHealthy() = %v, want %v", tt.status, got, tt.healthy)
			}
		})
	}
}

func TestDeviceStatus_IsAvailable(t *testing.T) {
	tests := []struct {
		status    DeviceStatus
		available bool
	}{
		{DeviceStatusOnline, true},
		{DeviceStatusDegraded, true},
		{DeviceStatusOffline, false},
		{DeviceStatusUnreachable, false},
		{DeviceStatusUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsAvailable(); got != tt.available {
				t.Errorf("DeviceStatus(%q).IsAvailable() = %v, want %v", tt.status, got, tt.available)
			}
		})
	}
}

func TestProxiedDevice_Validate(t *testing.T) {
	tests := []struct {
		name    string
		device  *ProxiedDevice
		wantErr bool
	}{
		{
			name: "valid device",
			device: &ProxiedDevice{
				ID:           "switch-01",
				ProxyAgentID: "proxy-dc1",
				Type:         DeviceTypeSwitch,
				Protocol:     ProtocolSSH,
				Address:      "192.168.1.1",
				ProfileID:    "cisco_ios",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			device: &ProxiedDevice{
				ProxyAgentID: "proxy-dc1",
				Type:         DeviceTypeSwitch,
				Protocol:     ProtocolSSH,
				Address:      "192.168.1.1",
				ProfileID:    "cisco_ios",
			},
			wantErr: true,
		},
		{
			name: "missing proxy agent ID",
			device: &ProxiedDevice{
				ID:        "switch-01",
				Type:      DeviceTypeSwitch,
				Protocol:  ProtocolSSH,
				Address:   "192.168.1.1",
				ProfileID: "cisco_ios",
			},
			wantErr: true,
		},
		{
			name: "invalid device type",
			device: &ProxiedDevice{
				ID:           "switch-01",
				ProxyAgentID: "proxy-dc1",
				Type:         DeviceType("invalid"),
				Protocol:     ProtocolSSH,
				Address:      "192.168.1.1",
				ProfileID:    "cisco_ios",
			},
			wantErr: true,
		},
		{
			name: "invalid protocol",
			device: &ProxiedDevice{
				ID:           "switch-01",
				ProxyAgentID: "proxy-dc1",
				Type:         DeviceTypeSwitch,
				Protocol:     ProtocolType("invalid"),
				Address:      "192.168.1.1",
				ProfileID:    "cisco_ios",
			},
			wantErr: true,
		},
		{
			name: "missing address",
			device: &ProxiedDevice{
				ID:           "switch-01",
				ProxyAgentID: "proxy-dc1",
				Type:         DeviceTypeSwitch,
				Protocol:     ProtocolSSH,
				ProfileID:    "cisco_ios",
			},
			wantErr: true,
		},
		{
			name: "missing profile ID",
			device: &ProxiedDevice{
				ID:           "switch-01",
				ProxyAgentID: "proxy-dc1",
				Type:         DeviceTypeSwitch,
				Protocol:     ProtocolSSH,
				Address:      "192.168.1.1",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.device.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ProxiedDevice.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProxiedDevice_FullID(t *testing.T) {
	device := &ProxiedDevice{
		ID:           "switch-01",
		ProxyAgentID: "proxy-dc1",
	}

	expected := "proxy-dc1/switch-01"
	if got := device.FullID(); got != expected {
		t.Errorf("ProxiedDevice.FullID() = %q, want %q", got, expected)
	}
}

func TestProxiedDevice_Clone(t *testing.T) {
	device := &ProxiedDevice{
		ID:           "switch-01",
		ProxyAgentID: "proxy-dc1",
		Metadata:     map[string]string{"key": "value"},
		Labels:       map[string]string{"env": "prod"},
	}

	clone := device.Clone()

	// Verify clone is equal
	if clone.ID != device.ID {
		t.Errorf("Clone ID mismatch")
	}

	// Verify maps are independent
	clone.Metadata["new"] = "data"
	if _, exists := device.Metadata["new"]; exists {
		t.Errorf("Clone should not affect original metadata")
	}

	clone.Labels["new"] = "label"
	if _, exists := device.Labels["new"]; exists {
		t.Errorf("Clone should not affect original labels")
	}
}

// =============================================================================
// Registry Tests
// =============================================================================

func TestInMemoryDeviceRegistry_Register(t *testing.T) {
	registry := NewInMemoryDeviceRegistry()
	ctx := context.Background()

	device := &ProxiedDevice{
		ID:           "switch-01",
		ProxyAgentID: "proxy-dc1",
		Type:         DeviceTypeSwitch,
		Protocol:     ProtocolSSH,
		Address:      "192.168.1.1",
		ProfileID:    "cisco_ios",
	}

	// Register device
	err := registry.Register(ctx, device)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify device is registered
	got, err := registry.Get(ctx, device.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != device.ID {
		t.Errorf("Get() ID = %q, want %q", got.ID, device.ID)
	}

	// Try to register duplicate
	err = registry.Register(ctx, device)
	if !errors.Is(err, ErrDeviceAlreadyExists) {
		t.Errorf("Register() duplicate error = %v, want ErrDeviceAlreadyExists", err)
	}
}

func TestInMemoryDeviceRegistry_Unregister(t *testing.T) {
	registry := NewInMemoryDeviceRegistry()
	ctx := context.Background()

	device := &ProxiedDevice{
		ID:           "switch-01",
		ProxyAgentID: "proxy-dc1",
		Type:         DeviceTypeSwitch,
		Protocol:     ProtocolSSH,
		Address:      "192.168.1.1",
		ProfileID:    "cisco_ios",
	}

	// Register and then unregister
	_ = registry.Register(ctx, device)

	err := registry.Unregister(ctx, device.ID)
	if err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	// Verify device is gone
	_, err = registry.Get(ctx, device.ID)
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Errorf("Get() after unregister error = %v, want ErrDeviceNotFound", err)
	}

	// Try to unregister non-existent
	err = registry.Unregister(ctx, "non-existent")
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Errorf("Unregister() non-existent error = %v, want ErrDeviceNotFound", err)
	}
}

func TestInMemoryDeviceRegistry_List(t *testing.T) {
	registry := NewInMemoryDeviceRegistry()
	ctx := context.Background()

	// Register multiple devices
	devices := []*ProxiedDevice{
		{ID: "switch-01", ProxyAgentID: "proxy-dc1", Type: DeviceTypeSwitch, Protocol: ProtocolSSH, Address: "192.168.1.1", ProfileID: "cisco_ios", Vendor: "cisco"},
		{ID: "router-01", ProxyAgentID: "proxy-dc1", Type: DeviceTypeRouter, Protocol: ProtocolSSH, Address: "192.168.1.2", ProfileID: "cisco_ios", Vendor: "cisco"},
		{ID: "firewall-01", ProxyAgentID: "proxy-dc2", Type: DeviceTypeFirewall, Protocol: ProtocolREST, Address: "192.168.1.3", ProfileID: "pfsense", Vendor: "netgate"},
	}

	for _, d := range devices {
		_ = registry.Register(ctx, d)
	}

	// List all
	all, err := registry.List(ctx, nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List() returned %d devices, want 3", len(all))
	}

	// Filter by proxy agent
	filtered, _ := registry.List(ctx, &DeviceFilter{ProxyAgentID: "proxy-dc1"})
	if len(filtered) != 2 {
		t.Errorf("List() by proxy agent returned %d devices, want 2", len(filtered))
	}

	// Filter by type
	filtered, _ = registry.List(ctx, &DeviceFilter{Types: []DeviceType{DeviceTypeSwitch}})
	if len(filtered) != 1 {
		t.Errorf("List() by type returned %d devices, want 1", len(filtered))
	}

	// Filter by vendor
	filtered, _ = registry.List(ctx, &DeviceFilter{Vendors: []string{"cisco"}})
	if len(filtered) != 2 {
		t.Errorf("List() by vendor returned %d devices, want 2", len(filtered))
	}

	// Filter by protocol
	filtered, _ = registry.List(ctx, &DeviceFilter{Protocols: []ProtocolType{ProtocolREST}})
	if len(filtered) != 1 {
		t.Errorf("List() by protocol returned %d devices, want 1", len(filtered))
	}

	// Test limit and offset
	limited, _ := registry.List(ctx, &DeviceFilter{Limit: 2})
	if len(limited) != 2 {
		t.Errorf("List() with limit returned %d devices, want 2", len(limited))
	}

	offset, _ := registry.List(ctx, &DeviceFilter{Offset: 2})
	if len(offset) != 1 {
		t.Errorf("List() with offset returned %d devices, want 1", len(offset))
	}
}

func TestInMemoryDeviceRegistry_UpdateStatus(t *testing.T) {
	registry := NewInMemoryDeviceRegistry()
	ctx := context.Background()

	device := &ProxiedDevice{
		ID:           "switch-01",
		ProxyAgentID: "proxy-dc1",
		Type:         DeviceTypeSwitch,
		Protocol:     ProtocolSSH,
		Address:      "192.168.1.1",
		ProfileID:    "cisco_ios",
	}

	_ = registry.Register(ctx, device)

	// Update status
	err := registry.UpdateStatus(ctx, device.ID, DeviceStatusOnline, "connected")
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	// Verify status
	got, _ := registry.Get(ctx, device.ID)
	if got.Status != DeviceStatusOnline {
		t.Errorf("Status = %v, want %v", got.Status, DeviceStatusOnline)
	}
	if got.StatusMessage != "connected" {
		t.Errorf("StatusMessage = %q, want %q", got.StatusMessage, "connected")
	}

	// Update non-existent
	err = registry.UpdateStatus(ctx, "non-existent", DeviceStatusOnline, "")
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Errorf("UpdateStatus() non-existent error = %v, want ErrDeviceNotFound", err)
	}
}

func TestInMemoryDeviceRegistry_GetStats(t *testing.T) {
	registry := NewInMemoryDeviceRegistry()
	ctx := context.Background()

	// Register devices with different statuses
	devices := []*ProxiedDevice{
		{ID: "d1", ProxyAgentID: "p1", Type: DeviceTypeSwitch, Protocol: ProtocolSSH, Address: "1.1.1.1", ProfileID: "p", Status: DeviceStatusOnline},
		{ID: "d2", ProxyAgentID: "p1", Type: DeviceTypeSwitch, Protocol: ProtocolSSH, Address: "1.1.1.2", ProfileID: "p", Status: DeviceStatusOnline},
		{ID: "d3", ProxyAgentID: "p2", Type: DeviceTypeRouter, Protocol: ProtocolSNMP, Address: "1.1.1.3", ProfileID: "p", Status: DeviceStatusOffline},
	}

	for _, d := range devices {
		_ = registry.Register(ctx, d)
		_ = registry.UpdateStatus(ctx, d.ID, d.Status, "")
	}

	stats, err := registry.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if stats.TotalDevices != 3 {
		t.Errorf("TotalDevices = %d, want 3", stats.TotalDevices)
	}
	if stats.OnlineDevices != 2 {
		t.Errorf("OnlineDevices = %d, want 2", stats.OnlineDevices)
	}
	if stats.OfflineDevices != 1 {
		t.Errorf("OfflineDevices = %d, want 1", stats.OfflineDevices)
	}
	if stats.ByType[DeviceTypeSwitch] != 2 {
		t.Errorf("ByType[switch] = %d, want 2", stats.ByType[DeviceTypeSwitch])
	}
	if stats.ByProtocol[ProtocolSSH] != 2 {
		t.Errorf("ByProtocol[ssh] = %d, want 2", stats.ByProtocol[ProtocolSSH])
	}
	if stats.ByProxyAgent["p1"] != 2 {
		t.Errorf("ByProxyAgent[p1] = %d, want 2", stats.ByProxyAgent["p1"])
	}
}

// =============================================================================
// Executor Tests
// =============================================================================

func TestRoutingExecutor_Execute(t *testing.T) {
	registry := NewInMemoryDeviceRegistry()
	executor := NewRoutingExecutor(registry, nil)
	ctx := context.Background()

	// Register stub adapter
	adapter := NewStubAdapter(ProtocolSSH)
	executor.RegisterAdapter(adapter)

	// Register device
	device := &ProxiedDevice{
		ID:           "switch-01",
		ProxyAgentID: "proxy-dc1",
		Type:         DeviceTypeSwitch,
		Protocol:     ProtocolSSH,
		Address:      "192.168.1.1",
		ProfileID:    "cisco_ios",
	}
	_ = registry.Register(ctx, device)

	// Execute command
	req := &ProxiedExecuteRequest{
		DeviceID: device.ID,
		Command:  "show version",
	}

	result, err := executor.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if len(result.Stdout) == 0 {
		t.Errorf("Stdout should not be empty")
	}
}

func TestRoutingExecutor_UnsupportedProtocol(t *testing.T) {
	registry := NewInMemoryDeviceRegistry()
	executor := NewRoutingExecutor(registry, nil)
	ctx := context.Background()

	// Register device with no matching adapter
	device := &ProxiedDevice{
		ID:           "switch-01",
		ProxyAgentID: "proxy-dc1",
		Type:         DeviceTypeSwitch,
		Protocol:     ProtocolSNMP, // No adapter registered
		Address:      "192.168.1.1",
		ProfileID:    "cisco_ios",
	}
	_ = registry.Register(ctx, device)

	req := &ProxiedExecuteRequest{
		DeviceID: device.ID,
		Command:  "show version",
	}

	_, err := executor.Execute(ctx, req)
	if err == nil {
		t.Fatal("Execute() should fail with unsupported protocol")
	}
}

// =============================================================================
// Subject Tests
// =============================================================================

func TestProxySubjectBuilder(t *testing.T) {
	builder := NewProxySubjectBuilder("mycluster")

	tests := []struct {
		name     string
		subject  string
		expected string
	}{
		{"DeviceRegister", builder.DeviceRegister(), "kscore.mycluster.proxy.device.register"},
		{"DeviceUnregister", builder.DeviceUnregister(), "kscore.mycluster.proxy.device.unregister"},
		{"DeviceHeartbeat", builder.DeviceHeartbeat("proxy-dc1"), "kscore.mycluster.proxy.proxy-dc1.heartbeat"},
		{"DeviceCommand", builder.DeviceCommand("proxy-dc1", "switch-01"), "kscore.mycluster.proxy.proxy-dc1.switch-01.command"},
		{"DeviceResult", builder.DeviceResult("proxy-dc1", "switch-01"), "kscore.mycluster.proxy.proxy-dc1.switch-01.result"},
		{"CredentialFetch", builder.CredentialFetch("proxy-dc1"), "kscore.mycluster.proxy.proxy-dc1.credential.fetch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.subject != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.subject, tt.expected)
			}
		})
	}
}

func TestParseDeviceSubject(t *testing.T) {
	tests := []struct {
		subject    string
		wantProxy  string
		wantDevice string
		wantOp     string
		wantErr    bool
	}{
		{"kscore.mycluster.proxy.proxy-dc1.switch-01.command", "proxy-dc1", "switch-01", "command", false},
		{"kscore.mycluster.proxy.proxy-dc1.switch-01.result", "proxy-dc1", "switch-01", "result", false},
		{"invalid", "", "", "", true},
		{"too.few.parts", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			proxy, device, op, err := ParseDeviceSubject(tt.subject)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDeviceSubject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if proxy != tt.wantProxy {
				t.Errorf("proxyAgentID = %q, want %q", proxy, tt.wantProxy)
			}
			if device != tt.wantDevice {
				t.Errorf("deviceID = %q, want %q", device, tt.wantDevice)
			}
			if op != tt.wantOp {
				t.Errorf("operation = %q, want %q", op, tt.wantOp)
			}
		})
	}
}

func TestFullDeviceID(t *testing.T) {
	fullID := FullDeviceID("proxy-dc1", "switch-01")
	if fullID != "proxy-dc1/switch-01" {
		t.Errorf("FullDeviceID() = %q, want %q", fullID, "proxy-dc1/switch-01")
	}

	proxy, device, err := ParseFullDeviceID(fullID)
	if err != nil {
		t.Fatalf("ParseFullDeviceID() error = %v", err)
	}
	if proxy != "proxy-dc1" {
		t.Errorf("proxyAgentID = %q, want %q", proxy, "proxy-dc1")
	}
	if device != "switch-01" {
		t.Errorf("deviceID = %q, want %q", device, "switch-01")
	}

	// Test invalid
	_, _, err = ParseFullDeviceID("no-slash")
	if err == nil {
		t.Error("ParseFullDeviceID() should fail without slash")
	}
}

// =============================================================================
// Config Tests
// =============================================================================

func TestParseConfig(t *testing.T) {
	yaml := `
agent:
  id: proxy-dc1
  cluster_name: production

nats:
  urls:
    - nats://localhost:4222

health:
  interval: 30s
  timeout: 10s
  max_concurrent: 5
  stale_threshold: 5m

devices:
  - id: switch-01
    name: Core Switch 1
    type: switch
    vendor: cisco
    protocol: ssh
    address: 192.168.1.1
    profile_id: cisco_ios
    credential_ref: vault://network/switch-01
    labels:
      env: production
      dc: dc1
`

	config, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}

	if config.Agent.ID != "proxy-dc1" {
		t.Errorf("Agent.ID = %q, want %q", config.Agent.ID, "proxy-dc1")
	}
	if len(config.Devices) != 1 {
		t.Errorf("len(Devices) = %d, want 1", len(config.Devices))
	}
	if config.Devices[0].ID != "switch-01" {
		t.Errorf("Devices[0].ID = %q, want %q", config.Devices[0].ID, "switch-01")
	}
	if config.Health.Interval.Duration() != 30*time.Second {
		t.Errorf("Health.Interval = %v, want 30s", config.Health.Interval.Duration())
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Agent: AgentConfig{ID: "proxy-dc1"},
				NATS:  NATSConfig{URLs: []string{"nats://localhost:4222"}},
				Devices: []DeviceConfig{
					{ID: "d1", Address: "1.1.1.1", Protocol: "ssh"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing agent ID",
			config: &Config{
				NATS: NATSConfig{URLs: []string{"nats://localhost:4222"}},
			},
			wantErr: true,
		},
		{
			name: "missing NATS URLs",
			config: &Config{
				Agent: AgentConfig{ID: "proxy-dc1"},
			},
			wantErr: true,
		},
		{
			name: "duplicate device ID",
			config: &Config{
				Agent: AgentConfig{ID: "proxy-dc1"},
				NATS:  NATSConfig{URLs: []string{"nats://localhost:4222"}},
				Devices: []DeviceConfig{
					{ID: "d1", Address: "1.1.1.1", Protocol: "ssh"},
					{ID: "d1", Address: "1.1.1.2", Protocol: "ssh"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid protocol",
			config: &Config{
				Agent: AgentConfig{ID: "proxy-dc1"},
				NATS:  NATSConfig{URLs: []string{"nats://localhost:4222"}},
				Devices: []DeviceConfig{
					{ID: "d1", Address: "1.1.1.1", Protocol: "invalid"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_ToProxiedDevices(t *testing.T) {
	config := &Config{
		Agent: AgentConfig{ID: "proxy-dc1"},
		Devices: []DeviceConfig{
			{ID: "switch-01", Name: "Switch 1", Type: "switch", Protocol: "ssh", Address: "1.1.1.1", ProfileID: "cisco_ios"},
			{ID: "router-01", Name: "Router 1", Type: "router", Protocol: "ssh", Address: "1.1.1.2", ProfileID: "cisco_ios"},
		},
	}

	devices := config.ToProxiedDevices()
	if len(devices) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(devices))
	}

	if devices[0].ProxyAgentID != "proxy-dc1" {
		t.Errorf("ProxyAgentID = %q, want %q", devices[0].ProxyAgentID, "proxy-dc1")
	}
	if devices[0].Type != DeviceTypeSwitch {
		t.Errorf("Type = %v, want %v", devices[0].Type, DeviceTypeSwitch)
	}
}

// =============================================================================
// Manager Tests
// =============================================================================

func TestNewManager(t *testing.T) {
	config := &ManagerConfig{
		AgentID: "proxy-dc1",
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if manager.State() != ProxyAgentStateStopped {
		t.Errorf("State() = %v, want %v", manager.State(), ProxyAgentStateStopped)
	}

	// Test missing agent ID
	_, err = NewManager(&ManagerConfig{})
	if err == nil {
		t.Error("NewManager() should fail without agent ID")
	}
}

func TestManager_StartStop(t *testing.T) {
	config := &ManagerConfig{
		AgentID: "proxy-dc1",
	}

	manager, _ := NewManager(config)
	ctx := context.Background()

	// Start
	err := manager.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if manager.State() != ProxyAgentStateRunning {
		t.Errorf("State() = %v, want %v", manager.State(), ProxyAgentStateRunning)
	}

	// Stop
	err = manager.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if manager.State() != ProxyAgentStateStopped {
		t.Errorf("State() = %v, want %v", manager.State(), ProxyAgentStateStopped)
	}
}

func TestManager_Stats(t *testing.T) {
	config := &ManagerConfig{
		AgentID: "proxy-dc1",
	}

	manager, _ := NewManager(config)
	ctx := context.Background()
	_ = manager.Start(ctx)
	defer manager.Stop(ctx)

	stats := manager.Stats()
	if stats.DevicesTotal != 0 {
		t.Errorf("DevicesTotal = %d, want 0", stats.DevicesTotal)
	}
	if stats.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
}

// =============================================================================
// Health Checker Tests
// =============================================================================

func TestHealthChecker_GetHealthSummary(t *testing.T) {
	registry := NewInMemoryDeviceRegistry()
	executor := NewRoutingExecutor(registry, nil)
	executor.RegisterAdapter(NewStubAdapter(ProtocolSSH))

	ctx := context.Background()

	// Register devices
	devices := []*ProxiedDevice{
		{ID: "d1", ProxyAgentID: "p1", Type: DeviceTypeSwitch, Protocol: ProtocolSSH, Address: "1.1.1.1", ProfileID: "p"},
		{ID: "d2", ProxyAgentID: "p1", Type: DeviceTypeRouter, Protocol: ProtocolSSH, Address: "1.1.1.2", ProfileID: "p"},
	}
	for _, d := range devices {
		_ = registry.Register(ctx, d)
	}
	_ = registry.UpdateStatus(ctx, "d1", DeviceStatusOnline, "")
	_ = registry.UpdateStatus(ctx, "d2", DeviceStatusOffline, "")

	checker := NewHealthChecker(&HealthCheckerConfig{
		Registry: registry,
		Executor: executor,
	})

	summary, err := checker.GetHealthSummary(ctx)
	if err != nil {
		t.Fatalf("GetHealthSummary() error = %v", err)
	}

	if summary.Total != 2 {
		t.Errorf("Total = %d, want 2", summary.Total)
	}
	if summary.Online != 1 {
		t.Errorf("Online = %d, want 1", summary.Online)
	}
	if summary.Offline != 1 {
		t.Errorf("Offline = %d, want 1", summary.Offline)
	}
}

// =============================================================================
// Credential Tests
// =============================================================================

func TestCredential_IsExpired(t *testing.T) {
	// Non-expiring credential
	cred := &Credential{}
	if cred.IsExpired() {
		t.Error("Credential with zero expiry should not be expired")
	}

	// Expired credential
	cred.ExpiresAt = time.Now().Add(-time.Hour)
	if !cred.IsExpired() {
		t.Error("Credential should be expired")
	}

	// Future expiry
	cred.ExpiresAt = time.Now().Add(time.Hour)
	if cred.IsExpired() {
		t.Error("Credential with future expiry should not be expired")
	}
}
