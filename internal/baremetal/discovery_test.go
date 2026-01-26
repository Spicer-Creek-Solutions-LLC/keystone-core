package baremetal

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestServerState(t *testing.T) {
	states := []ServerState{
		StateUnknown,
		StateDiscovering,
		StateAvailable,
		StateProvisioning,
		StateProvisioned,
		StateMaintenance,
		StateError,
		StateRetired,
	}

	for _, state := range states {
		if state == "" {
			t.Error("State should not be empty")
		}
	}
}

func TestDiscoveryMethod(t *testing.T) {
	methods := []DiscoveryMethod{
		MethodIPMI,
		MethodRedfish,
		MethodSSH,
		MethodAgent,
		MethodManual,
		MethodDHCP,
		MethodPXE,
	}

	for _, method := range methods {
		if method == "" {
			t.Error("Method should not be empty")
		}
	}
}

func TestHardwareInfo(t *testing.T) {
	hw := HardwareInfo{
		CPU: CPUInfo{
			Model:        "Intel Xeon Gold 6248R",
			Vendor:       "Intel",
			Cores:        24,
			Threads:      48,
			Sockets:      2,
			ClockMHz:     3000,
			CacheMB:      35.75,
			Features:     []string{"avx", "avx2", "avx512"},
			Architecture: "x86_64",
		},
		Memory: MemoryInfo{
			TotalMB:  524288, // 512GB
			UsableMB: 520000,
			Slots:    16,
			Type:     "DDR4",
			SpeedMHz: 2933,
			ECC:      true,
		},
		Storage: StorageInfo{
			TotalSizeMB: 3932160, // 3.75TB
			Devices: []StorageDevice{
				{
					Name:       "nvme0n1",
					Type:       "nvme",
					SizeMB:     1966080,
					Model:      "Samsung PM1733",
					Rotational: false,
					Transport:  "nvme",
				},
				{
					Name:       "nvme1n1",
					Type:       "nvme",
					SizeMB:     1966080,
					Model:      "Samsung PM1733",
					Rotational: false,
					Transport:  "nvme",
				},
			},
		},
		Network: NetworkInfo{
			Interfaces: []NetworkInterface{
				{
					Name:      "eth0",
					MAC:       "00:11:22:33:44:55",
					SpeedMbps: 25000,
					MTU:       9000,
					State:     "up",
				},
				{
					Name:      "eth1",
					MAC:       "00:11:22:33:44:56",
					SpeedMbps: 25000,
					MTU:       9000,
					State:     "up",
				},
			},
			Hostname: "server1",
			FQDN:     "server1.example.com",
		},
		BMC: &BMCInfo{
			Type:     "ipmi",
			IP:       "192.168.1.101",
			Vendor:   "Dell",
			Firmware: "2.82",
		},
		GPUs: []GPUInfo{
			{
				Model:             "NVIDIA A100",
				Vendor:            "NVIDIA",
				MemoryMB:          81920,
				PCIAddress:        "0000:3b:00.0",
				ComputeCapability: "8.0",
			},
		},
		Vendor: "Dell",
		Model:  "PowerEdge R750",
		Serial: "ABC123XYZ",
		UUID:   "550e8400-e29b-41d4-a716-446655440000",
	}

	if hw.CPU.Cores != 24 {
		t.Errorf("CPU cores = %d, want 24", hw.CPU.Cores)
	}
	if hw.Memory.TotalMB != 524288 {
		t.Errorf("Memory = %d MB, want 524288", hw.Memory.TotalMB)
	}
	if len(hw.Storage.Devices) != 2 {
		t.Errorf("Storage devices = %d, want 2", len(hw.Storage.Devices))
	}
	if len(hw.Network.Interfaces) != 2 {
		t.Errorf("Network interfaces = %d, want 2", len(hw.Network.Interfaces))
	}
	if len(hw.GPUs) != 1 {
		t.Errorf("GPUs = %d, want 1", len(hw.GPUs))
	}
}

func TestServer(t *testing.T) {
	server := &Server{
		ID:    "server-001",
		Name:  "compute-node-1",
		State: StateAvailable,
		Hardware: HardwareInfo{
			CPU: CPUInfo{
				Cores: 32,
			},
			Memory: MemoryInfo{
				TotalMB: 262144,
			},
		},
		DiscoveredAt:    time.Now().Add(-time.Hour),
		LastSeenAt:      time.Now(),
		DiscoveryMethod: MethodIPMI,
		Location: &Location{
			Datacenter: "dc1",
			Rack:       "rack-42",
			Position:   10,
		},
		Labels: map[string]string{
			"environment": "production",
			"role":        "compute",
		},
		Pool: "gpu-pool",
	}

	if server.State != StateAvailable {
		t.Errorf("State = %s, want %s", server.State, StateAvailable)
	}
	if server.Location.Rack != "rack-42" {
		t.Errorf("Rack = %s, want rack-42", server.Location.Rack)
	}
}

func TestNewEngine(t *testing.T) {
	config := &DiscoveryConfig{
		Networks: []string{"192.168.1.0/24"},
		Methods:  []DiscoveryMethod{MethodIPMI},
	}
	store := NewInMemoryStore()

	engine := NewEngine(config, store)

	if engine == nil {
		t.Fatal("Expected non-nil engine")
	}
	if engine.config.Concurrency != 10 {
		t.Errorf("Default concurrency = %d, want 10", engine.config.Concurrency)
	}
	if engine.config.Timeout != 30*time.Second {
		t.Errorf("Default timeout = %v, want 30s", engine.config.Timeout)
	}
	if engine.config.ScanInterval != 5*time.Minute {
		t.Errorf("Default scan interval = %v, want 5m", engine.config.ScanInterval)
	}
}

type mockDriver struct {
	method  DiscoveryMethod
	servers []*Server
}

func (m *mockDriver) Method() DiscoveryMethod {
	return m.method
}

func (m *mockDriver) Discover(ctx context.Context, network string) ([]*Server, error) {
	return m.servers, nil
}

func (m *mockDriver) Probe(ctx context.Context, ip string) (*Server, error) {
	for _, s := range m.servers {
		// Check if any interface has this IP
		for _, iface := range s.Hardware.Network.Interfaces {
			for _, ifaceIP := range iface.IPs {
				if ifaceIP == ip {
					return s, nil
				}
			}
		}
	}
	return nil, nil
}

func (m *mockDriver) Refresh(ctx context.Context, server *Server) error {
	return nil
}

func TestEngine_RegisterDriver(t *testing.T) {
	config := &DiscoveryConfig{
		Networks: []string{"192.168.1.0/24"},
		Methods:  []DiscoveryMethod{MethodIPMI},
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	driver := &mockDriver{method: MethodIPMI}
	engine.RegisterDriver(driver)

	if len(engine.drivers) != 1 {
		t.Errorf("Drivers count = %d, want 1", len(engine.drivers))
	}
}

func TestEngine_RunDiscovery(t *testing.T) {
	config := &DiscoveryConfig{
		Networks:    []string{"192.168.1.0/24"},
		Methods:     []DiscoveryMethod{MethodIPMI},
		Concurrency: 5,
		Timeout:     5 * time.Second,
		DefaultLabels: map[string]string{
			"managed-by": "keystone",
		},
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	mockServers := []*Server{
		{
			ID:    "server-001",
			State: StateAvailable,
			Hardware: HardwareInfo{
				CPU:    CPUInfo{Cores: 16},
				Memory: MemoryInfo{TotalMB: 65536},
			},
		},
		{
			ID:    "server-002",
			State: StateAvailable,
			Hardware: HardwareInfo{
				CPU:    CPUInfo{Cores: 32},
				Memory: MemoryInfo{TotalMB: 131072},
			},
		},
	}

	driver := &mockDriver{
		method:  MethodIPMI,
		servers: mockServers,
	}
	engine.RegisterDriver(driver)

	var events []*DiscoveryEvent
	var eventMu sync.Mutex
	engine.AddListener(func(e *DiscoveryEvent) {
		eventMu.Lock()
		events = append(events, e)
		eventMu.Unlock()
	})

	ctx := context.Background()
	result, err := engine.RunDiscovery(ctx)
	if err != nil {
		t.Fatalf("RunDiscovery failed: %v", err)
	}

	if result.ServersFound != 2 {
		t.Errorf("ServersFound = %d, want 2", result.ServersFound)
	}

	// Check that servers were saved with default labels
	servers, _ := store.List(ctx, nil)
	if len(servers) != 2 {
		t.Errorf("Stored servers = %d, want 2", len(servers))
	}

	for _, s := range servers {
		if s.Labels["managed-by"] != "keystone" {
			t.Errorf("Label managed-by = %s, want keystone", s.Labels["managed-by"])
		}
	}

	// Check events
	eventMu.Lock()
	eventCount := len(events)
	eventMu.Unlock()
	if eventCount < 2 {
		t.Errorf("Events count = %d, want at least 2", eventCount)
	}
}

func TestInMemoryStore(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	server := &Server{
		ID:    "test-server",
		State: StateAvailable,
		Hardware: HardwareInfo{
			CPU:     CPUInfo{Cores: 8},
			Memory:  MemoryInfo{TotalMB: 32768},
			Storage: StorageInfo{TotalSizeMB: 512000},
		},
		Labels: map[string]string{
			"environment": "test",
		},
		Pool: "default",
	}

	// Test Save
	if err := store.Save(ctx, server); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Test Get
	retrieved, err := store.Get(ctx, "test-server")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.ID != "test-server" {
		t.Errorf("ID = %s, want test-server", retrieved.ID)
	}

	// Test List
	servers, err := store.List(ctx, nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(servers) != 1 {
		t.Errorf("List count = %d, want 1", len(servers))
	}

	// Test UpdateState
	if err := store.UpdateState(ctx, "test-server", StateProvisioning); err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	retrieved, _ = store.Get(ctx, "test-server")
	if retrieved.State != StateProvisioning {
		t.Errorf("State = %s, want %s", retrieved.State, StateProvisioning)
	}

	// Test Delete
	if err := store.Delete(ctx, "test-server"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(ctx, "test-server")
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestServerFilter(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	servers := []*Server{
		{
			ID:    "server-1",
			State: StateAvailable,
			Hardware: HardwareInfo{
				CPU:     CPUInfo{Cores: 8},
				Memory:  MemoryInfo{TotalMB: 32768},
				Storage: StorageInfo{TotalSizeMB: 256000},
			},
			Labels: map[string]string{"env": "prod"},
			Pool:   "pool-a",
		},
		{
			ID:    "server-2",
			State: StateProvisioned,
			Hardware: HardwareInfo{
				CPU:     CPUInfo{Cores: 16},
				Memory:  MemoryInfo{TotalMB: 65536},
				Storage: StorageInfo{TotalSizeMB: 512000},
				GPUs:    []GPUInfo{{Model: "A100"}},
			},
			Labels: map[string]string{"env": "prod"},
			Pool:   "pool-b",
		},
		{
			ID:    "server-3",
			State: StateAvailable,
			Hardware: HardwareInfo{
				CPU:     CPUInfo{Cores: 32},
				Memory:  MemoryInfo{TotalMB: 131072},
				Storage: StorageInfo{TotalSizeMB: 1024000},
			},
			Labels: map[string]string{"env": "staging"},
			Pool:   "pool-a",
		},
	}

	for _, s := range servers {
		store.Save(ctx, s)
	}

	tests := []struct {
		name     string
		filter   *ServerFilter
		expected int
	}{
		{
			name:     "no filter",
			filter:   nil,
			expected: 3,
		},
		{
			name: "filter by state available",
			filter: &ServerFilter{
				States: []ServerState{StateAvailable},
			},
			expected: 2,
		},
		{
			name: "filter by state provisioned",
			filter: &ServerFilter{
				States: []ServerState{StateProvisioned},
			},
			expected: 1,
		},
		{
			name: "filter by label",
			filter: &ServerFilter{
				Labels: map[string]string{"env": "prod"},
			},
			expected: 2,
		},
		{
			name: "filter by pool",
			filter: &ServerFilter{
				Pool: "pool-a",
			},
			expected: 2,
		},
		{
			name: "filter by min CPU",
			filter: &ServerFilter{
				MinCPUCores: 16,
			},
			expected: 2,
		},
		{
			name: "filter by min memory",
			filter: &ServerFilter{
				MinMemoryMB: 100000,
			},
			expected: 1,
		},
		{
			name: "filter by GPU",
			filter: &ServerFilter{
				HasGPU: true,
			},
			expected: 1,
		},
		{
			name: "combined filter",
			filter: &ServerFilter{
				States:      []ServerState{StateAvailable},
				MinCPUCores: 16,
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := store.List(ctx, tt.filter)
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}
			if len(result) != tt.expected {
				t.Errorf("Got %d servers, want %d", len(result), tt.expected)
			}
		})
	}
}

func TestEngine_ListAvailableServers(t *testing.T) {
	config := &DiscoveryConfig{
		Networks: []string{"192.168.1.0/24"},
		Methods:  []DiscoveryMethod{MethodIPMI},
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	ctx := context.Background()

	// Add some servers with different states
	servers := []*Server{
		{ID: "s1", State: StateAvailable},
		{ID: "s2", State: StateProvisioned},
		{ID: "s3", State: StateAvailable},
		{ID: "s4", State: StateMaintenance},
	}

	for _, s := range servers {
		store.Save(ctx, s)
	}

	available, err := engine.ListAvailableServers(ctx)
	if err != nil {
		t.Fatalf("ListAvailableServers failed: %v", err)
	}

	if len(available) != 2 {
		t.Errorf("Available servers = %d, want 2", len(available))
	}
}

func TestEngine_SetServerState(t *testing.T) {
	config := &DiscoveryConfig{
		Networks: []string{"192.168.1.0/24"},
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	ctx := context.Background()
	store.Save(ctx, &Server{ID: "test", State: StateAvailable})

	var events []*DiscoveryEvent
	engine.AddListener(func(e *DiscoveryEvent) {
		events = append(events, e)
	})

	if err := engine.SetServerState(ctx, "test", StateProvisioning); err != nil {
		t.Fatalf("SetServerState failed: %v", err)
	}

	server, _ := store.Get(ctx, "test")
	if server.State != StateProvisioning {
		t.Errorf("State = %s, want %s", server.State, StateProvisioning)
	}

	if len(events) != 1 || events[0].Type != "server_state_changed" {
		t.Error("Expected state change event")
	}
}

func TestEngine_AssignToPool(t *testing.T) {
	config := &DiscoveryConfig{
		Networks: []string{"192.168.1.0/24"},
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	ctx := context.Background()
	store.Save(ctx, &Server{ID: "test", State: StateAvailable})

	if err := engine.AssignToPool(ctx, "test", "gpu-pool"); err != nil {
		t.Fatalf("AssignToPool failed: %v", err)
	}

	server, _ := store.Get(ctx, "test")
	if server.Pool != "gpu-pool" {
		t.Errorf("Pool = %s, want gpu-pool", server.Pool)
	}
}

func TestParseCIDR(t *testing.T) {
	tests := []struct {
		cidr     string
		minCount int
		wantErr  bool
	}{
		{"192.168.1.0/30", 2, false},  // 4 addresses - 2 (network + broadcast) = 2
		{"192.168.1.0/29", 6, false},  // 8 - 2 = 6
		{"192.168.1.0/28", 14, false}, // 16 - 2 = 14
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			ips, err := ParseCIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr = %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(ips) < tt.minCount {
				t.Errorf("IPs count = %d, want at least %d", len(ips), tt.minCount)
			}
		})
	}
}

func TestDiscoveryConfig(t *testing.T) {
	config := &DiscoveryConfig{
		Networks: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Methods:  []DiscoveryMethod{MethodIPMI, MethodRedfish, MethodSSH},
		Credentials: &DiscoveryCredentials{
			IPMI: &IPMICredentials{
				Username: "admin",
				Password: "password",
			},
			SSH: &SSHCredentials{
				Username:       "root",
				PrivateKeyPath: "/path/to/key",
			},
			Redfish: &RedfishCredentials{
				Username: "admin",
				Password: "password",
			},
		},
		ScanInterval: 10 * time.Minute,
		Timeout:      60 * time.Second,
		Concurrency:  20,
		DefaultLabels: map[string]string{
			"datacenter": "dc1",
		},
		DefaultLocation: &Location{
			Datacenter: "dc1",
			Region:     "us-east",
		},
	}

	if len(config.Networks) != 2 {
		t.Errorf("Networks = %d, want 2", len(config.Networks))
	}
	if len(config.Methods) != 3 {
		t.Errorf("Methods = %d, want 3", len(config.Methods))
	}
	if config.Credentials.IPMI.Username != "admin" {
		t.Errorf("IPMI username = %s, want admin", config.Credentials.IPMI.Username)
	}
}

func TestLocation(t *testing.T) {
	loc := &Location{
		Datacenter: "dc1",
		Room:       "room-a",
		Row:        "row-3",
		Rack:       "rack-42",
		Position:   10,
		Region:     "us-east",
		Zone:       "zone-1",
	}

	if loc.Datacenter != "dc1" {
		t.Errorf("Datacenter = %s, want dc1", loc.Datacenter)
	}
	if loc.Position != 10 {
		t.Errorf("Position = %d, want 10", loc.Position)
	}
}

func TestEngine_StartStop(t *testing.T) {
	config := &DiscoveryConfig{
		Networks:     []string{"192.168.1.0/24"},
		Methods:      []DiscoveryMethod{MethodIPMI},
		ScanInterval: 100 * time.Millisecond,
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	driver := &mockDriver{method: MethodIPMI}
	engine.RegisterDriver(driver)

	var started, stopped bool
	engine.AddListener(func(e *DiscoveryEvent) {
		if e.Type == "engine_started" {
			started = true
		}
		if e.Type == "engine_stopped" {
			stopped = true
		}
	})

	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		return started, nil
	}); err != nil {
		t.Fatalf("expected engine started event: %v", err)
	}

	// Try to start again - should fail
	if err := engine.Start(ctx); err == nil {
		t.Error("Expected error when starting twice")
	}

	engine.Stop()
	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		return stopped, nil
	}); err != nil {
		t.Fatalf("expected engine stopped event: %v", err)
	}

	if !started {
		t.Error("Expected engine_started event")
	}
	if !stopped {
		t.Error("Expected engine_stopped event")
	}
}

func TestSMARTInfo(t *testing.T) {
	smart := &SMARTInfo{
		Healthy:            true,
		PowerOnHours:       8760, // 1 year
		Temperature:        35,
		WearPercentage:     5,
		ReallocatedSectors: 0,
	}

	if !smart.Healthy {
		t.Error("Expected SMART healthy = true")
	}
	if smart.PowerOnHours != 8760 {
		t.Errorf("PowerOnHours = %d, want 8760", smart.PowerOnHours)
	}
}

func TestStorageController(t *testing.T) {
	controller := StorageController{
		Name:       "PERC H740P",
		Type:       "raid",
		Model:      "PERC H740P Mini",
		CacheSize:  8192,
		BBUPresent: true,
	}

	if controller.Type != "raid" {
		t.Errorf("Type = %s, want raid", controller.Type)
	}
	if !controller.BBUPresent {
		t.Error("Expected BBU present")
	}
}

func TestDiscoveryResult(t *testing.T) {
	start := time.Now()
	result := &DiscoveryResult{
		ServersFound:   5,
		ServersUpdated: 10,
		ServersLost:    2,
		Errors:         []string{"error1", "error2"},
		Duration:       5 * time.Second,
		StartedAt:      start,
		CompletedAt:    start.Add(5 * time.Second),
	}

	if result.ServersFound != 5 {
		t.Errorf("ServersFound = %d, want 5", result.ServersFound)
	}
	if len(result.Errors) != 2 {
		t.Errorf("Errors = %d, want 2", len(result.Errors))
	}
}

func TestLocationFilter(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	servers := []*Server{
		{
			ID:       "s1",
			State:    StateAvailable,
			Location: &Location{Datacenter: "dc1", Rack: "rack-1", Region: "us-east"},
		},
		{
			ID:       "s2",
			State:    StateAvailable,
			Location: &Location{Datacenter: "dc1", Rack: "rack-2", Region: "us-east"},
		},
		{
			ID:       "s3",
			State:    StateAvailable,
			Location: &Location{Datacenter: "dc2", Rack: "rack-1", Region: "us-west"},
		},
	}

	for _, s := range servers {
		store.Save(ctx, s)
	}

	// Filter by datacenter
	result, _ := store.List(ctx, &ServerFilter{
		Location: &Location{Datacenter: "dc1"},
	})
	if len(result) != 2 {
		t.Errorf("DC filter: got %d, want 2", len(result))
	}

	// Filter by rack
	result, _ = store.List(ctx, &ServerFilter{
		Location: &Location{Rack: "rack-1"},
	})
	if len(result) != 2 {
		t.Errorf("Rack filter: got %d, want 2", len(result))
	}

	// Filter by region
	result, _ = store.List(ctx, &ServerFilter{
		Location: &Location{Region: "us-west"},
	})
	if len(result) != 1 {
		t.Errorf("Region filter: got %d, want 1", len(result))
	}
}

func TestEngine_SetProfileMatcher(t *testing.T) {
	config := &DiscoveryConfig{
		Networks: []string{"192.168.1.0/24"},
		Methods:  []DiscoveryMethod{MethodIPMI},
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	matcher := NewHardwareProfileMatcher()
	matcher.AddProfile(&HardwareProfile{
		Name:     "test-profile",
		Priority: 100,
		Criteria: HardwareProfileCriteria{
			MinCPUCores: 8,
		},
		Labels: map[string]string{
			"role": "compute",
		},
		Pool: "compute-pool",
	})

	engine.SetProfileMatcher(matcher)

	if engine.profileMatcher == nil {
		t.Error("Expected profile matcher to be set")
	}
}

func TestEngine_RunDiscovery_WithProfileMatching(t *testing.T) {
	trueBool := true

	config := &DiscoveryConfig{
		Networks:    []string{"192.168.1.0/24"},
		Methods:     []DiscoveryMethod{MethodIPMI},
		Concurrency: 5,
		Timeout:     5 * time.Second,
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	// Set up profile matcher
	matcher := NewHardwareProfileMatcher()
	matcher.AddProfile(&HardwareProfile{
		Name:        "compute-gpu",
		Description: "GPU compute nodes",
		Priority:    200,
		Criteria: HardwareProfileCriteria{
			RequireGPU:  &trueBool,
			MinCPUCores: 16,
		},
		Labels: map[string]string{
			"role":     "compute",
			"workload": "gpu",
		},
		Pool: "gpu-pool",
	})
	matcher.AddProfile(&HardwareProfile{
		Name:        "compute-standard",
		Description: "Standard compute nodes",
		Priority:    100,
		Criteria: HardwareProfileCriteria{
			MinCPUCores: 8,
			MinMemoryMB: 32768,
		},
		Labels: map[string]string{
			"role": "compute",
		},
		Pool: "compute-pool",
	})
	engine.SetProfileMatcher(matcher)

	// Create mock servers
	mockServers := []*Server{
		{
			ID: "gpu-server",
			Hardware: HardwareInfo{
				CPU:    CPUInfo{Cores: 32},
				Memory: MemoryInfo{TotalMB: 131072},
				GPUs:   []GPUInfo{{Vendor: "NVIDIA", MemoryMB: 40960}},
			},
		},
		{
			ID: "standard-server",
			Hardware: HardwareInfo{
				CPU:    CPUInfo{Cores: 16},
				Memory: MemoryInfo{TotalMB: 65536},
			},
		},
		{
			ID: "small-server",
			Hardware: HardwareInfo{
				CPU:    CPUInfo{Cores: 4},
				Memory: MemoryInfo{TotalMB: 8192},
			},
		},
	}

	driver := &mockDriver{
		method:  MethodIPMI,
		servers: mockServers,
	}
	engine.RegisterDriver(driver)

	ctx := context.Background()
	result, err := engine.RunDiscovery(ctx)
	if err != nil {
		t.Fatalf("RunDiscovery failed: %v", err)
	}

	if result.ServersFound != 3 {
		t.Errorf("ServersFound = %d, want 3", result.ServersFound)
	}

	// Check GPU server got compute-gpu profile
	gpuServer, _ := store.Get(ctx, "gpu-server")
	if gpuServer.Labels["hardware-profile"] != "compute-gpu" {
		t.Errorf("GPU server profile = %s, want compute-gpu", gpuServer.Labels["hardware-profile"])
	}
	if gpuServer.Labels["role"] != "compute" {
		t.Errorf("GPU server role = %s, want compute", gpuServer.Labels["role"])
	}
	if gpuServer.Labels["workload"] != "gpu" {
		t.Errorf("GPU server workload = %s, want gpu", gpuServer.Labels["workload"])
	}
	if gpuServer.Pool != "gpu-pool" {
		t.Errorf("GPU server pool = %s, want gpu-pool", gpuServer.Pool)
	}

	// Check standard server got compute-standard profile
	stdServer, _ := store.Get(ctx, "standard-server")
	if stdServer.Labels["hardware-profile"] != "compute-standard" {
		t.Errorf("Standard server profile = %s, want compute-standard", stdServer.Labels["hardware-profile"])
	}
	if stdServer.Pool != "compute-pool" {
		t.Errorf("Standard server pool = %s, want compute-pool", stdServer.Pool)
	}

	// Check small server has no profile (doesn't meet any criteria)
	smallServer, _ := store.Get(ctx, "small-server")
	if smallServer.Labels["hardware-profile"] != "" {
		t.Errorf("Small server should have no profile, got %s", smallServer.Labels["hardware-profile"])
	}
}

func TestEngine_RunDiscovery_ProfileDoesNotOverrideExistingLabels(t *testing.T) {
	config := &DiscoveryConfig{
		Networks:    []string{"192.168.1.0/24"},
		Methods:     []DiscoveryMethod{MethodIPMI},
		Concurrency: 5,
		Timeout:     5 * time.Second,
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	// Set up profile matcher
	matcher := NewHardwareProfileMatcher()
	matcher.AddProfile(&HardwareProfile{
		Name:     "compute",
		Priority: 100,
		Criteria: HardwareProfileCriteria{
			MinCPUCores: 8,
		},
		Labels: map[string]string{
			"role":        "compute",
			"environment": "default",
		},
		Pool: "default-pool",
	})
	engine.SetProfileMatcher(matcher)

	// Create mock server with existing labels
	mockServers := []*Server{
		{
			ID: "server-with-labels",
			Hardware: HardwareInfo{
				CPU:    CPUInfo{Cores: 16},
				Memory: MemoryInfo{TotalMB: 65536},
			},
			Labels: map[string]string{
				"environment": "production", // This should NOT be overridden
			},
		},
	}

	driver := &mockDriver{
		method:  MethodIPMI,
		servers: mockServers,
	}
	engine.RegisterDriver(driver)

	ctx := context.Background()
	_, err := engine.RunDiscovery(ctx)
	if err != nil {
		t.Fatalf("RunDiscovery failed: %v", err)
	}

	server, _ := store.Get(ctx, "server-with-labels")

	// Profile should be applied
	if server.Labels["hardware-profile"] != "compute" {
		t.Errorf("Profile label = %s, want compute", server.Labels["hardware-profile"])
	}

	// Role should be added (didn't exist before)
	if server.Labels["role"] != "compute" {
		t.Errorf("Role label = %s, want compute", server.Labels["role"])
	}

	// Existing label should NOT be overridden
	if server.Labels["environment"] != "production" {
		t.Errorf("Environment label = %s, want production (should not be overridden)", server.Labels["environment"])
	}
}

func TestEngine_DiscoverServer_WithProfileMatching(t *testing.T) {
	config := &DiscoveryConfig{
		Networks: []string{"192.168.1.0/24"},
		Methods:  []DiscoveryMethod{MethodIPMI},
		Timeout:  5 * time.Second,
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	// Set up profile matcher
	matcher := NewHardwareProfileMatcher()
	matcher.AddProfile(&HardwareProfile{
		Name:     "compute",
		Priority: 100,
		Criteria: HardwareProfileCriteria{
			MinCPUCores: 8,
		},
		Labels: map[string]string{
			"role": "compute",
		},
		Pool: "compute-pool",
	})
	engine.SetProfileMatcher(matcher)

	// Create mock server with IP
	mockServers := []*Server{
		{
			ID: "server-001",
			Hardware: HardwareInfo{
				CPU:    CPUInfo{Cores: 16},
				Memory: MemoryInfo{TotalMB: 65536},
				Network: NetworkInfo{
					Interfaces: []NetworkInterface{
						{IPs: []string{"192.168.1.100"}},
					},
				},
			},
		},
	}

	driver := &mockDriver{
		method:  MethodIPMI,
		servers: mockServers,
	}
	engine.RegisterDriver(driver)

	ctx := context.Background()
	server, err := engine.DiscoverServer(ctx, "192.168.1.100")
	if err != nil {
		t.Fatalf("DiscoverServer failed: %v", err)
	}

	if server == nil {
		t.Fatal("Expected server to be returned")
	}

	// Check profile was applied
	if server.Labels["hardware-profile"] != "compute" {
		t.Errorf("Profile = %s, want compute", server.Labels["hardware-profile"])
	}
	if server.Labels["role"] != "compute" {
		t.Errorf("Role = %s, want compute", server.Labels["role"])
	}
	if server.Pool != "compute-pool" {
		t.Errorf("Pool = %s, want compute-pool", server.Pool)
	}
}

func TestEngine_RunDiscovery_WithoutProfileMatcher(t *testing.T) {
	config := &DiscoveryConfig{
		Networks:    []string{"192.168.1.0/24"},
		Methods:     []DiscoveryMethod{MethodIPMI},
		Concurrency: 5,
		Timeout:     5 * time.Second,
	}
	store := NewInMemoryStore()
	engine := NewEngine(config, store)

	// No profile matcher set

	mockServers := []*Server{
		{
			ID: "server-001",
			Hardware: HardwareInfo{
				CPU:    CPUInfo{Cores: 16},
				Memory: MemoryInfo{TotalMB: 65536},
			},
		},
	}

	driver := &mockDriver{
		method:  MethodIPMI,
		servers: mockServers,
	}
	engine.RegisterDriver(driver)

	ctx := context.Background()
	result, err := engine.RunDiscovery(ctx)
	if err != nil {
		t.Fatalf("RunDiscovery failed: %v", err)
	}

	if result.ServersFound != 1 {
		t.Errorf("ServersFound = %d, want 1", result.ServersFound)
	}

	// Server should have no hardware-profile label
	server, _ := store.Get(ctx, "server-001")
	if server.Labels != nil && server.Labels["hardware-profile"] != "" {
		t.Errorf("Expected no profile label, got %s", server.Labels["hardware-profile"])
	}
}
