package protocols

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// MockAdapter is a mock protocol adapter for testing.
type MockAdapter struct {
	connected   bool
	executeErr  error
	healthErr   error
	metrics     *AdapterMetrics
	lastRequest *ExecuteRequest
}

func NewMockAdapter() *MockAdapter {
	return &MockAdapter{
		metrics: &AdapterMetrics{},
	}
}

func (m *MockAdapter) Type() ProtocolType {
	return ProtocolType("mock")
}

func (m *MockAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	m.connected = true
	m.metrics.ConnectionCount++
	return nil
}

func (m *MockAdapter) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error) {
	m.lastRequest = req
	m.metrics.ExecutionCount++

	if m.executeErr != nil {
		m.metrics.ExecutionErrors++
		return &ExecuteResult{
			Error:    m.executeErr.Error(),
			ExitCode: 1,
		}, m.executeErr
	}

	return &ExecuteResult{
		ExitCode:  0,
		Stdout:    []byte("mock output"),
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}, nil
}

func (m *MockAdapter) Disconnect(ctx context.Context) error {
	m.connected = false
	return nil
}

func (m *MockAdapter) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	if m.healthErr != nil {
		return &HealthCheckResult{
			Healthy:   false,
			Status:    "unhealthy",
			LastCheck: time.Now(),
		}, m.healthErr
	}

	return &HealthCheckResult{
		Healthy:   m.connected,
		Status:    "healthy",
		Latency:   time.Millisecond,
		LastCheck: time.Now(),
	}, nil
}

func (m *MockAdapter) IsConnected() bool {
	return m.connected
}

func TestProtocolType(t *testing.T) {
	tests := []struct {
		name     string
		protocol ProtocolType
		expected string
	}{
		{"SSH", ProtocolSSH, "ssh"},
		{"SNMP", ProtocolSNMP, "snmp"},
		{"REST", ProtocolREST, "rest"},
		{"WinRM", ProtocolWinRM, "winrm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.protocol) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.protocol)
			}
		})
	}
}

func TestExecuteResult_Success(t *testing.T) {
	tests := []struct {
		name     string
		result   ExecuteResult
		expected bool
	}{
		{
			name: "successful execution",
			result: ExecuteResult{
				ExitCode: 0,
			},
			expected: true,
		},
		{
			name: "non-zero exit code",
			result: ExecuteResult{
				ExitCode: 1,
			},
			expected: false,
		},
		{
			name: "error message",
			result: ExecuteResult{
				ExitCode: 0,
				Error:    "some error",
			},
			expected: false,
		},
		{
			name: "timeout",
			result: ExecuteResult{
				ExitCode: 0,
				Timeout:  true,
			},
			expected: false,
		},
		{
			name: "killed",
			result: ExecuteResult{
				ExitCode: 0,
				Killed:   true,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.Success() != tt.expected {
				t.Errorf("expected Success() = %v, got %v", tt.expected, tt.result.Success())
			}
		})
	}
}

func TestTransferResult_Success(t *testing.T) {
	tests := []struct {
		name     string
		result   TransferResult
		expected bool
	}{
		{
			name: "successful transfer",
			result: TransferResult{
				BytesTransferred: 100,
			},
			expected: true,
		},
		{
			name: "error message",
			result: TransferResult{
				Error: "transfer failed",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.Success() != tt.expected {
				t.Errorf("expected Success() = %v, got %v", tt.expected, tt.result.Success())
			}
		})
	}
}

func TestDefaultPTYConfig(t *testing.T) {
	config := DefaultPTYConfig()

	if config.Term != "xterm" {
		t.Errorf("expected Term = xterm, got %s", config.Term)
	}
	if config.Rows != 24 {
		t.Errorf("expected Rows = 24, got %d", config.Rows)
	}
	if config.Cols != 80 {
		t.Errorf("expected Cols = 80, got %d", config.Cols)
	}
}

func TestDefaultConnectionConfig(t *testing.T) {
	config := DefaultConnectionConfig()

	if config.Timeout != 30*time.Second {
		t.Errorf("expected Timeout = 30s, got %v", config.Timeout)
	}
	if !config.KeepAlive {
		t.Error("expected KeepAlive = true")
	}
	if config.KeepAliveInterval != 30*time.Second {
		t.Errorf("expected KeepAliveInterval = 30s, got %v", config.KeepAliveInterval)
	}
	if config.MaxRetries != 3 {
		t.Errorf("expected MaxRetries = 3, got %d", config.MaxRetries)
	}
	if config.RetryDelay != 5*time.Second {
		t.Errorf("expected RetryDelay = 5s, got %v", config.RetryDelay)
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	// Test Register
	mockFactory := func(config *ConnectionConfig) (ProtocolAdapter, error) {
		return NewMockAdapter(), nil
	}

	registry.Register(ProtocolType("test"), mockFactory)

	// Test Has
	if !registry.Has(ProtocolType("test")) {
		t.Error("expected registry to have 'test' protocol")
	}

	if registry.Has(ProtocolType("nonexistent")) {
		t.Error("expected registry to not have 'nonexistent' protocol")
	}

	// Test Create
	adapter, err := registry.Create(ProtocolType("test"), nil)
	if err != nil {
		t.Errorf("unexpected error creating adapter: %v", err)
	}
	if adapter == nil {
		t.Error("expected adapter to be non-nil")
	}

	// Test Create with nonexistent protocol
	_, err = registry.Create(ProtocolType("nonexistent"), nil)
	if err == nil {
		t.Error("expected error for nonexistent protocol")
	}

	// Test ListProtocols
	protocols := registry.ListProtocols()
	if len(protocols) != 1 {
		t.Errorf("expected 1 protocol, got %d", len(protocols))
	}

	// Test Unregister
	registry.Unregister(ProtocolType("test"))
	if registry.Has(ProtocolType("test")) {
		t.Error("expected protocol to be unregistered")
	}
}

func TestRegistry_FileTransfer(t *testing.T) {
	registry := NewRegistry()

	// Test RegisterFileTransfer
	if registry.HasFileTransfer(ProtocolSSH) {
		t.Error("expected no file transfer adapter registered")
	}

	// Register mock file transfer factory
	registry.RegisterFileTransfer(ProtocolSSH, func(config *ConnectionConfig) (FileTransferAdapter, error) {
		return nil, nil
	})

	if !registry.HasFileTransfer(ProtocolSSH) {
		t.Error("expected file transfer adapter to be registered")
	}
}

func TestRegistry_Tunnel(t *testing.T) {
	registry := NewRegistry()

	// Test RegisterTunnel
	if registry.HasTunnel(ProtocolSSH) {
		t.Error("expected no tunnel adapter registered")
	}

	// Register mock tunnel factory
	registry.RegisterTunnel(ProtocolSSH, func(config *ConnectionConfig) (TunnelAdapter, error) {
		return nil, nil
	})

	if !registry.HasTunnel(ProtocolSSH) {
		t.Error("expected tunnel adapter to be registered")
	}
}

func TestAdapterPool(t *testing.T) {
	registry := NewRegistry()

	// Register mock adapter
	registry.Register(ProtocolSSH, func(config *ConnectionConfig) (ProtocolAdapter, error) {
		return NewMockAdapter(), nil
	})

	pool := NewAdapterPool(registry)

	// Test Count (empty)
	if pool.Count() != 0 {
		t.Errorf("expected pool count = 0, got %d", pool.Count())
	}

	// Test Get
	device := &proxy.ProxiedDevice{
		ID:       "test-device",
		Address:  "192.168.1.1",
		Protocol: "ssh",
	}
	cred := &credentials.SSHPasswordCredential{}

	ctx := context.Background()
	adapter, err := pool.Get(ctx, device, cred)
	if err != nil {
		t.Errorf("unexpected error getting adapter: %v", err)
	}
	if adapter == nil {
		t.Error("expected adapter to be non-nil")
	}

	// Test Count (after get)
	if pool.Count() != 1 {
		t.Errorf("expected pool count = 1, got %d", pool.Count())
	}

	// Test ActiveCount
	if pool.ActiveCount() != 1 {
		t.Errorf("expected active count = 1, got %d", pool.ActiveCount())
	}

	// Test Release (no-op)
	pool.Release(device.ID)
	if pool.Count() != 1 {
		t.Errorf("expected pool count = 1 after release, got %d", pool.Count())
	}

	// Test Close
	err = pool.Close(ctx, device.ID)
	if err != nil {
		t.Errorf("unexpected error closing adapter: %v", err)
	}
	if pool.Count() != 0 {
		t.Errorf("expected pool count = 0 after close, got %d", pool.Count())
	}
}

func TestAdapterPool_CloseAll(t *testing.T) {
	registry := NewRegistry()

	registry.Register(ProtocolSSH, func(config *ConnectionConfig) (ProtocolAdapter, error) {
		return NewMockAdapter(), nil
	})

	pool := NewAdapterPool(registry)
	ctx := context.Background()

	// Add multiple devices
	for i := 0; i < 3; i++ {
		device := &proxy.ProxiedDevice{
			ID:       fmt.Sprintf("device-%d", i),
			Address:  "192.168.1.1",
			Protocol: "ssh",
		}
		_, err := pool.Get(ctx, device, &credentials.SSHPasswordCredential{})
		if err != nil {
			t.Fatalf("failed to get adapter: %v", err)
		}
	}

	if pool.Count() != 3 {
		t.Errorf("expected pool count = 3, got %d", pool.Count())
	}

	// Close all
	err := pool.CloseAll(ctx)
	if err != nil {
		t.Errorf("unexpected error closing all: %v", err)
	}

	if pool.Count() != 0 {
		t.Errorf("expected pool count = 0 after CloseAll, got %d", pool.Count())
	}
}

func TestMockAdapter(t *testing.T) {
	adapter := NewMockAdapter()
	ctx := context.Background()

	// Test Type
	if adapter.Type() != ProtocolType("mock") {
		t.Errorf("expected type = mock, got %s", adapter.Type())
	}

	// Test Connect
	device := &proxy.ProxiedDevice{ID: "test"}
	err := adapter.Connect(ctx, device, nil)
	if err != nil {
		t.Errorf("unexpected error connecting: %v", err)
	}
	if !adapter.IsConnected() {
		t.Error("expected adapter to be connected")
	}

	// Test Execute
	result, err := adapter.Execute(ctx, &ExecuteRequest{Command: "echo hello"})
	if err != nil {
		t.Errorf("unexpected error executing: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code = 0, got %d", result.ExitCode)
	}

	// Test HealthCheck
	health, err := adapter.HealthCheck(ctx)
	if err != nil {
		t.Errorf("unexpected error checking health: %v", err)
	}
	if !health.Healthy {
		t.Error("expected adapter to be healthy")
	}

	// Test Disconnect
	err = adapter.Disconnect(ctx)
	if err != nil {
		t.Errorf("unexpected error disconnecting: %v", err)
	}
	if adapter.IsConnected() {
		t.Error("expected adapter to be disconnected")
	}
}

func TestDefaultRegistry(t *testing.T) {
	// DefaultRegistry should be initialized
	if DefaultRegistry == nil {
		t.Fatal("DefaultRegistry should not be nil")
	}

	// Test that we can use the default registry
	registry := NewRegistry()
	registry.Register(ProtocolType("test"), func(config *ConnectionConfig) (ProtocolAdapter, error) {
		return NewMockAdapter(), nil
	})
}

func TestFileInfo(t *testing.T) {
	now := time.Now()
	info := FileInfo{
		Name:       "test.txt",
		Path:       "/path/to/test.txt",
		Size:       1024,
		Mode:       0644,
		ModTime:    now,
		IsDir:      false,
		IsSymlink:  true,
		LinkTarget: "/path/to/target",
		Owner:      "root",
		Group:      "root",
	}

	if info.Name != "test.txt" {
		t.Errorf("expected Name = test.txt, got %s", info.Name)
	}
	if info.Size != 1024 {
		t.Errorf("expected Size = 1024, got %d", info.Size)
	}
	if info.IsDir {
		t.Error("expected IsDir = false")
	}
	if !info.IsSymlink {
		t.Error("expected IsSymlink = true")
	}
}

func TestTunnel(t *testing.T) {
	closeCalled := false
	tunnel := &Tunnel{
		ID:            "test-tunnel",
		Type:          "local",
		LocalAddr:     "127.0.0.1:8080",
		RemoteAddr:    "192.168.1.1:80",
		Active:        true,
		BytesSent:     100,
		BytesReceived: 200,
		Close: func() error {
			closeCalled = true
			return nil
		},
	}

	if tunnel.ID != "test-tunnel" {
		t.Errorf("expected ID = test-tunnel, got %s", tunnel.ID)
	}
	if tunnel.Type != "local" {
		t.Errorf("expected Type = local, got %s", tunnel.Type)
	}
	if !tunnel.Active {
		t.Error("expected Active = true")
	}
	if tunnel.BytesSent != 100 {
		t.Errorf("expected BytesSent = 100, got %d", tunnel.BytesSent)
	}
	if tunnel.BytesReceived != 200 {
		t.Errorf("expected BytesReceived = 200, got %d", tunnel.BytesReceived)
	}

	err := tunnel.Close()
	if err != nil {
		t.Errorf("unexpected error closing tunnel: %v", err)
	}
	if !closeCalled {
		t.Error("expected Close() to be called")
	}
}

func TestForwardRequest(t *testing.T) {
	req := ForwardRequest{
		LocalHost:  "127.0.0.1",
		LocalPort:  8080,
		RemoteHost: "192.168.1.1",
		RemotePort: 80,
	}

	if req.LocalHost != "127.0.0.1" {
		t.Errorf("expected LocalHost = 127.0.0.1, got %s", req.LocalHost)
	}
	if req.LocalPort != 8080 {
		t.Errorf("expected LocalPort = 8080, got %d", req.LocalPort)
	}
	if req.RemoteHost != "192.168.1.1" {
		t.Errorf("expected RemoteHost = 192.168.1.1, got %s", req.RemoteHost)
	}
	if req.RemotePort != 80 {
		t.Errorf("expected RemotePort = 80, got %d", req.RemotePort)
	}
}

func TestAdapterMetrics(t *testing.T) {
	metrics := AdapterMetrics{
		ConnectionCount:  10,
		ConnectionErrors: 2,
		ExecutionCount:   100,
		ExecutionErrors:  5,
		BytesSent:        10000,
		BytesReceived:    20000,
		AverageLatency:   50 * time.Millisecond,
		LastActivity:     time.Now(),
	}

	if metrics.ConnectionCount != 10 {
		t.Errorf("expected ConnectionCount = 10, got %d", metrics.ConnectionCount)
	}
	if metrics.ExecutionErrors != 5 {
		t.Errorf("expected ExecutionErrors = 5, got %d", metrics.ExecutionErrors)
	}
}
