package proxy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteDeviceRegistry_CRUD(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "proxy-sqlite-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create registry
	registry, err := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{
		Path:    filepath.Join(tmpDir, "devices.db"),
		WALMode: true,
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	defer registry.Close()

	ctx := context.Background()

	// Test Register
	device := &ProxiedDevice{
		ID:           "device-1",
		ProxyAgentID: "proxy-1",
		Name:         "Test Device",
		Type:         DeviceTypeLinux,
		Protocol:     ProtocolSSH,
		Address:      "192.168.1.100",
		Port:         22,
		ProfileID:    "linux-ssh",
		Status:       DeviceStatusOnline,
		Labels: map[string]string{
			"env": "test",
		},
		Metadata: map[string]string{
			"os": "ubuntu",
		},
	}

	err = registry.Register(ctx, device)
	if err != nil {
		t.Fatalf("failed to register device: %v", err)
	}

	// Test duplicate registration
	err = registry.Register(ctx, device)
	if !errors.Is(err, ErrDeviceAlreadyExists) {
		t.Errorf("expected ErrDeviceAlreadyExists, got %v", err)
	}

	// Test Get
	retrieved, err := registry.Get(ctx, "device-1")
	if err != nil {
		t.Fatalf("failed to get device: %v", err)
	}
	if retrieved.Name != device.Name {
		t.Errorf("expected name %q, got %q", device.Name, retrieved.Name)
	}
	if retrieved.Labels["env"] != "test" {
		t.Errorf("expected label env=test, got %q", retrieved.Labels["env"])
	}

	// Test Get non-existent
	_, err = registry.Get(ctx, "non-existent")
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}

	// Test Update
	retrieved.Name = "Updated Device"
	retrieved.Labels["datacenter"] = "dc1"
	err = registry.Update(ctx, retrieved)
	if err != nil {
		t.Fatalf("failed to update device: %v", err)
	}

	updated, err := registry.Get(ctx, "device-1")
	if err != nil {
		t.Fatalf("failed to get updated device: %v", err)
	}
	if updated.Name != "Updated Device" {
		t.Errorf("expected name %q, got %q", "Updated Device", updated.Name)
	}
	if updated.Labels["datacenter"] != "dc1" {
		t.Errorf("expected label datacenter=dc1, got %q", updated.Labels["datacenter"])
	}

	// Test UpdateStatus
	err = registry.UpdateStatus(ctx, "device-1", DeviceStatusOffline, "Connection lost")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	statusUpdated, err := registry.Get(ctx, "device-1")
	if err != nil {
		t.Fatalf("failed to get status-updated device: %v", err)
	}
	if statusUpdated.Status != DeviceStatusOffline {
		t.Errorf("expected status %q, got %q", DeviceStatusOffline, statusUpdated.Status)
	}

	// Test Count
	count, err := registry.Count(ctx)
	if err != nil {
		t.Fatalf("failed to count devices: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// Test Unregister
	err = registry.Unregister(ctx, "device-1")
	if err != nil {
		t.Fatalf("failed to unregister device: %v", err)
	}

	count, err = registry.Count(ctx)
	if err != nil {
		t.Fatalf("failed to count devices: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}

	// Test Unregister non-existent
	err = registry.Unregister(ctx, "non-existent")
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}
}

func TestSQLiteDeviceRegistry_List(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proxy-sqlite-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	registry, err := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{
		Path: filepath.Join(tmpDir, "devices.db"),
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	defer registry.Close()

	ctx := context.Background()

	// Register multiple devices
	devices := []*ProxiedDevice{
		{ID: "dev-1", ProxyAgentID: "proxy-1", Name: "Device 1", Type: DeviceTypeLinux, Protocol: ProtocolSSH, Address: "192.168.1.1", ProfileID: "linux-ssh", Status: DeviceStatusOnline, Labels: map[string]string{"env": "prod"}},
		{ID: "dev-2", ProxyAgentID: "proxy-1", Name: "Device 2", Type: DeviceTypeNetwork, Protocol: ProtocolSNMP, Address: "192.168.1.2", ProfileID: "network-snmp", Status: DeviceStatusOffline, Labels: map[string]string{"env": "staging"}},
		{ID: "dev-3", ProxyAgentID: "proxy-2", Name: "Device 3", Type: DeviceTypeLinux, Protocol: ProtocolSSH, Address: "192.168.1.3", ProfileID: "linux-ssh", Status: DeviceStatusOnline, Labels: map[string]string{"env": "prod"}},
	}

	for _, d := range devices {
		if err := registry.Register(ctx, d); err != nil {
			t.Fatalf("failed to register device: %v", err)
		}
	}

	// Test List all
	all, err := registry.List(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list all devices: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 devices, got %d", len(all))
	}

	// Test List by proxy agent
	byProxy, err := registry.List(ctx, &DeviceFilter{ProxyAgentID: "proxy-1"})
	if err != nil {
		t.Fatalf("failed to list by proxy: %v", err)
	}
	if len(byProxy) != 2 {
		t.Errorf("expected 2 devices for proxy-1, got %d", len(byProxy))
	}

	// Test List by type
	byType, err := registry.List(ctx, &DeviceFilter{Types: []DeviceType{DeviceTypeLinux}})
	if err != nil {
		t.Fatalf("failed to list by type: %v", err)
	}
	if len(byType) != 2 {
		t.Errorf("expected 2 Linux devices, got %d", len(byType))
	}

	// Test List by status
	byStatus, err := registry.List(ctx, &DeviceFilter{Statuses: []DeviceStatus{DeviceStatusOnline}})
	if err != nil {
		t.Fatalf("failed to list by status: %v", err)
	}
	if len(byStatus) != 2 {
		t.Errorf("expected 2 online devices, got %d", len(byStatus))
	}

	// Test List by labels
	byLabel, err := registry.List(ctx, &DeviceFilter{Labels: map[string]string{"env": "prod"}})
	if err != nil {
		t.Fatalf("failed to list by labels: %v", err)
	}
	if len(byLabel) != 2 {
		t.Errorf("expected 2 prod devices, got %d", len(byLabel))
	}

	// Test List with limit
	limited, err := registry.List(ctx, &DeviceFilter{Limit: 2})
	if err != nil {
		t.Fatalf("failed to list with limit: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected 2 devices with limit, got %d", len(limited))
	}

	// Test List with offset
	offset, err := registry.List(ctx, &DeviceFilter{Offset: 1})
	if err != nil {
		t.Fatalf("failed to list with offset: %v", err)
	}
	if len(offset) != 2 {
		t.Errorf("expected 2 devices with offset, got %d", len(offset))
	}
}

func TestSQLiteDeviceRegistry_GetOnlineOffline(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proxy-sqlite-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	registry, err := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{
		Path: filepath.Join(tmpDir, "devices.db"),
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	defer registry.Close()

	ctx := context.Background()

	// Register devices with different statuses
	devices := []*ProxiedDevice{
		{ID: "dev-1", ProxyAgentID: "proxy-1", Name: "Device 1", Type: DeviceTypeLinux, Protocol: ProtocolSSH, Address: "192.168.1.1", ProfileID: "linux-ssh", Status: DeviceStatusOnline},
		{ID: "dev-2", ProxyAgentID: "proxy-1", Name: "Device 2", Type: DeviceTypeLinux, Protocol: ProtocolSSH, Address: "192.168.1.2", ProfileID: "linux-ssh", Status: DeviceStatusOffline},
		{ID: "dev-3", ProxyAgentID: "proxy-1", Name: "Device 3", Type: DeviceTypeLinux, Protocol: ProtocolSSH, Address: "192.168.1.3", ProfileID: "linux-ssh", Status: DeviceStatusUnreachable},
	}

	for _, d := range devices {
		if err := registry.Register(ctx, d); err != nil {
			t.Fatalf("failed to register device: %v", err)
		}
	}

	// Test GetOnlineDevices
	online, err := registry.GetOnlineDevices(ctx)
	if err != nil {
		t.Fatalf("failed to get online devices: %v", err)
	}
	if len(online) != 1 {
		t.Errorf("expected 1 online device, got %d", len(online))
	}

	// Test GetOfflineDevices
	offline, err := registry.GetOfflineDevices(ctx)
	if err != nil {
		t.Fatalf("failed to get offline devices: %v", err)
	}
	if len(offline) != 2 {
		t.Errorf("expected 2 offline devices, got %d", len(offline))
	}
}

func TestSQLiteDeviceRegistry_GetStats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proxy-sqlite-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	registry, err := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{
		Path: filepath.Join(tmpDir, "devices.db"),
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	defer registry.Close()

	ctx := context.Background()

	// Register devices
	devices := []*ProxiedDevice{
		{ID: "dev-1", ProxyAgentID: "proxy-1", Name: "Device 1", Type: DeviceTypeLinux, Protocol: ProtocolSSH, Address: "192.168.1.1", ProfileID: "linux-ssh", Status: DeviceStatusOnline},
		{ID: "dev-2", ProxyAgentID: "proxy-1", Name: "Device 2", Type: DeviceTypeNetwork, Protocol: ProtocolSNMP, Address: "192.168.1.2", ProfileID: "network-snmp", Status: DeviceStatusOffline},
		{ID: "dev-3", ProxyAgentID: "proxy-2", Name: "Device 3", Type: DeviceTypeLinux, Protocol: ProtocolSSH, Address: "192.168.1.3", ProfileID: "linux-ssh", Status: DeviceStatusDegraded},
	}

	for _, d := range devices {
		if err := registry.Register(ctx, d); err != nil {
			t.Fatalf("failed to register device: %v", err)
		}
	}

	stats, err := registry.GetStats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.TotalDevices != 3 {
		t.Errorf("expected 3 total devices, got %d", stats.TotalDevices)
	}
	if stats.OnlineDevices != 1 {
		t.Errorf("expected 1 online device, got %d", stats.OnlineDevices)
	}
	if stats.OfflineDevices != 1 {
		t.Errorf("expected 1 offline device, got %d", stats.OfflineDevices)
	}
	if stats.DegradedDevices != 1 {
		t.Errorf("expected 1 degraded device, got %d", stats.DegradedDevices)
	}
	if stats.ByType[DeviceTypeLinux] != 2 {
		t.Errorf("expected 2 Linux devices, got %d", stats.ByType[DeviceTypeLinux])
	}
	if stats.ByProtocol[ProtocolSSH] != 2 {
		t.Errorf("expected 2 SSH devices, got %d", stats.ByProtocol[ProtocolSSH])
	}
	if stats.ByProxyAgent["proxy-1"] != 2 {
		t.Errorf("expected 2 devices for proxy-1, got %d", stats.ByProxyAgent["proxy-1"])
	}
}

func TestSQLiteDeviceRegistry_GetStaleDevices(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proxy-sqlite-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	registry, err := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{
		Path: filepath.Join(tmpDir, "devices.db"),
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	defer registry.Close()

	ctx := context.Background()

	// Register a device
	device := &ProxiedDevice{
		ID:           "dev-1",
		ProxyAgentID: "proxy-1",
		Name:         "Device 1",
		Type:         DeviceTypeLinux,
		Protocol:     ProtocolSSH,
		Address:      "192.168.1.1",
		ProfileID:    "linux-ssh",
		Status:       DeviceStatusOnline,
		LastSeen:     time.Now().Add(-10 * time.Minute),
	}

	if err := registry.Register(ctx, device); err != nil {
		t.Fatalf("failed to register device: %v", err)
	}

	// Get stale devices with 5 minute threshold
	stale, err := registry.GetStaleDevices(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("failed to get stale devices: %v", err)
	}
	if len(stale) != 1 {
		t.Errorf("expected 1 stale device, got %d", len(stale))
	}

	// Get stale devices with 15 minute threshold (should return none)
	notStale, err := registry.GetStaleDevices(ctx, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to get stale devices: %v", err)
	}
	if len(notStale) != 0 {
		t.Errorf("expected 0 stale devices, got %d", len(notStale))
	}
}

func TestSQLiteDeviceRegistry_Clear(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proxy-sqlite-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	registry, err := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{
		Path: filepath.Join(tmpDir, "devices.db"),
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	defer registry.Close()

	ctx := context.Background()

	// Register some devices
	for i := 0; i < 5; i++ {
		device := &ProxiedDevice{
			ID:           "dev-" + string(rune('a'+i)),
			ProxyAgentID: "proxy-1",
			Name:         "Device",
			Type:         DeviceTypeLinux,
			Protocol:     ProtocolSSH,
			Address:      "192.168.1.1",
			ProfileID:    "linux-ssh",
			Status:       DeviceStatusOnline,
		}
		if err := registry.Register(ctx, device); err != nil {
			t.Fatalf("failed to register device: %v", err)
		}
	}

	count, _ := registry.Count(ctx)
	if count != 5 {
		t.Errorf("expected 5 devices, got %d", count)
	}

	// Clear all devices
	if err := registry.Clear(ctx); err != nil {
		t.Fatalf("failed to clear registry: %v", err)
	}

	count, _ = registry.Count(ctx)
	if count != 0 {
		t.Errorf("expected 0 devices after clear, got %d", count)
	}
}

func TestSQLiteDeviceRegistry_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proxy-sqlite-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "devices.db")
	ctx := context.Background()

	// Create registry and add a device
	registry1, err := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{
		Path: dbPath,
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	device := &ProxiedDevice{
		ID:           "persistent-dev",
		ProxyAgentID: "proxy-1",
		Name:         "Persistent Device",
		Type:         DeviceTypeLinux,
		Protocol:     ProtocolSSH,
		Address:      "192.168.1.100",
		ProfileID:    "linux-ssh",
		Status:       DeviceStatusOnline,
	}

	if err := registry1.Register(ctx, device); err != nil {
		t.Fatalf("failed to register device: %v", err)
	}

	// Close the registry
	if err := registry1.Close(); err != nil {
		t.Fatalf("failed to close registry: %v", err)
	}

	// Open a new registry with the same database
	registry2, err := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{
		Path: dbPath,
	})
	if err != nil {
		t.Fatalf("failed to create second registry: %v", err)
	}
	defer registry2.Close()

	// Verify the device is still there
	retrieved, err := registry2.Get(ctx, "persistent-dev")
	if err != nil {
		t.Fatalf("failed to get device after reopen: %v", err)
	}
	if retrieved.Name != "Persistent Device" {
		t.Errorf("expected name %q, got %q", "Persistent Device", retrieved.Name)
	}
}

func TestSQLiteDeviceRegistry_Observer(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proxy-sqlite-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	registry, err := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{
		Path: filepath.Join(tmpDir, "devices.db"),
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	defer registry.Close()

	ctx := context.Background()

	// Track observer calls
	registered := make(chan *ProxiedDevice, 1)
	updated := make(chan *ProxiedDevice, 1)
	statusChanged := make(chan string, 1)
	unregistered := make(chan string, 1)

	observer := &testObserver{
		onRegistered:    func(d *ProxiedDevice) { registered <- d },
		onUpdated:       func(d *ProxiedDevice) { updated <- d },
		onStatusChanged: func(id string, _, _ DeviceStatus) { statusChanged <- id },
		onUnregistered:  func(id string) { unregistered <- id },
	}

	registry.AddObserver(observer)

	// Register device
	device := &ProxiedDevice{
		ID:           "obs-dev",
		ProxyAgentID: "proxy-1",
		Name:         "Observer Test Device",
		Type:         DeviceTypeLinux,
		Protocol:     ProtocolSSH,
		Address:      "192.168.1.1",
		ProfileID:    "linux-ssh",
		Status:       DeviceStatusOnline,
	}

	if err := registry.Register(ctx, device); err != nil {
		t.Fatalf("failed to register device: %v", err)
	}

	select {
	case <-registered:
		// OK
	case <-time.After(time.Second):
		t.Error("expected OnDeviceRegistered to be called")
	}

	// Update device
	device.Name = "Updated Name"
	if err := registry.Update(ctx, device); err != nil {
		t.Fatalf("failed to update device: %v", err)
	}

	select {
	case <-updated:
		// OK
	case <-time.After(time.Second):
		t.Error("expected OnDeviceUpdated to be called")
	}

	// Update status
	if err := registry.UpdateStatus(ctx, "obs-dev", DeviceStatusOffline, "test"); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	select {
	case <-statusChanged:
		// OK
	case <-time.After(time.Second):
		t.Error("expected OnDeviceStatusChanged to be called")
	}

	// Unregister device
	if err := registry.Unregister(ctx, "obs-dev"); err != nil {
		t.Fatalf("failed to unregister device: %v", err)
	}

	select {
	case <-unregistered:
		// OK
	case <-time.After(time.Second):
		t.Error("expected OnDeviceUnregistered to be called")
	}
}

// testObserver is a test helper that implements DeviceObserver.
type testObserver struct {
	onRegistered    func(*ProxiedDevice)
	onUnregistered  func(string)
	onUpdated       func(*ProxiedDevice)
	onStatusChanged func(string, DeviceStatus, DeviceStatus)
}

func (o *testObserver) OnDeviceRegistered(device *ProxiedDevice) {
	if o.onRegistered != nil {
		o.onRegistered(device)
	}
}

func (o *testObserver) OnDeviceUnregistered(deviceID string) {
	if o.onUnregistered != nil {
		o.onUnregistered(deviceID)
	}
}

func (o *testObserver) OnDeviceUpdated(device *ProxiedDevice) {
	if o.onUpdated != nil {
		o.onUpdated(device)
	}
}

func (o *testObserver) OnDeviceStatusChanged(deviceID string, oldStatus, newStatus DeviceStatus) {
	if o.onStatusChanged != nil {
		o.onStatusChanged(deviceID, oldStatus, newStatus)
	}
}
