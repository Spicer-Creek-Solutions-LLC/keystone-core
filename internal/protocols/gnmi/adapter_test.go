package gnmi

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Port != DefaultPort {
		t.Errorf("expected port %d, got %d", DefaultPort, cfg.Port)
	}
	if cfg.Encoding != EncodingJSONIETF {
		t.Errorf("expected encoding %s, got %s", EncodingJSONIETF, cfg.Encoding)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", cfg.Timeout)
	}
}

func TestAdapter_Type(t *testing.T) {
	adapter := &Adapter{
		config:  DefaultConfig(),
		metrics: &protocols.AdapterMetrics{},
	}

	if adapter.Type() != protocols.ProtocolGNMI {
		t.Errorf("expected type %s, got %s", protocols.ProtocolGNMI, adapter.Type())
	}
}

func TestAdapter_InterfaceCompliance(t *testing.T) {
	var _ protocols.ProtocolAdapter = (*Adapter)(nil)
	var _ protocols.GNMIAdapter = (*Adapter)(nil)
}

func TestAdapter_Connect_WrongCredentialType(t *testing.T) {
	adapter := &Adapter{
		config:  DefaultConfig(),
		metrics: &protocols.AdapterMetrics{},
	}

	device := &proxy.ProxiedDevice{
		ID:      "test",
		Address: "localhost",
	}

	err := adapter.Connect(context.Background(), device, &credentials.SSHPasswordCredential{})
	if err == nil {
		t.Error("expected error for wrong credential type")
	}
}

func TestAdapter_Connect_AlreadyConnected(t *testing.T) {
	adapter := &Adapter{
		config:    DefaultConfig(),
		metrics:   &protocols.AdapterMetrics{},
		connected: true,
	}

	device := &proxy.ProxiedDevice{
		ID:      "test",
		Address: "localhost",
	}
	cred := &credentials.GNMICredential{SkipVerify: true}

	err := adapter.Connect(context.Background(), device, cred)
	if err == nil {
		t.Error("expected error when already connected")
	}
}

func TestAdapter_Execute_NotConnected(t *testing.T) {
	adapter := &Adapter{
		config:  DefaultConfig(),
		metrics: &protocols.AdapterMetrics{},
	}

	_, err := adapter.Execute(context.Background(), &protocols.ExecuteRequest{Command: "capabilities"})
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestAdapter_HealthCheck_NotConnected(t *testing.T) {
	adapter := &Adapter{
		config:  DefaultConfig(),
		metrics: &protocols.AdapterMetrics{},
	}

	result, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error when not connected")
	}
	if result.Healthy {
		t.Error("expected unhealthy when not connected")
	}
}

func TestAdapter_IsConnected_Default(t *testing.T) {
	adapter := &Adapter{
		config:  DefaultConfig(),
		metrics: &protocols.AdapterMetrics{},
	}

	if adapter.IsConnected() {
		t.Error("expected not connected by default")
	}
}

func TestAdapter_Disconnect_NotConnected(t *testing.T) {
	adapter := &Adapter{
		config:  DefaultConfig(),
		metrics: &protocols.AdapterMetrics{},
	}

	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("disconnect when not connected should not error: %v", err)
	}
}

func TestAdapter_Metrics(t *testing.T) {
	metrics := &protocols.AdapterMetrics{}
	adapter := &Adapter{
		config:  DefaultConfig(),
		metrics: metrics,
	}

	if adapter.Metrics() != metrics {
		t.Error("expected same metrics reference")
	}
}

func TestAdapter_ConnectAndDisconnect(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)
	device := makeTestDevice(mock.addr)
	cred := makeTestCredential(certs)

	adapter := &Adapter{
		config:  DefaultConfig(),
		metrics: &protocols.AdapterMetrics{},
	}

	ctx := context.Background()
	err := adapter.Connect(ctx, device, cred)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	if !adapter.IsConnected() {
		t.Error("expected connected after Connect")
	}
	if adapter.metrics.ConnectionCount != 1 {
		t.Errorf("expected ConnectionCount=1, got %d", adapter.metrics.ConnectionCount)
	}

	err = adapter.Disconnect(ctx)
	if err != nil {
		t.Errorf("failed to disconnect: %v", err)
	}
	if adapter.IsConnected() {
		t.Error("expected not connected after Disconnect")
	}
}

func TestNewAdapterFactory(t *testing.T) {
	factory := NewAdapterFactory(nil)

	adapter, err := factory(nil)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}

	if adapter.Type() != protocols.ProtocolGNMI {
		t.Errorf("expected type %s, got %s", protocols.ProtocolGNMI, adapter.Type())
	}
}

func TestNewGNMIAdapterFactory(t *testing.T) {
	factory := NewGNMIAdapterFactory(nil)

	adapter, err := factory(nil)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}

	if adapter.Type() != protocols.ProtocolGNMI {
		t.Errorf("expected type %s, got %s", protocols.ProtocolGNMI, adapter.Type())
	}
}

func TestRegistry_GNMI(t *testing.T) {
	if !protocols.DefaultRegistry.Has(protocols.ProtocolGNMI) {
		t.Error("expected gNMI adapter registered in default registry")
	}
	if !protocols.DefaultRegistry.HasGNMI(protocols.ProtocolGNMI) {
		t.Error("expected gNMI typed adapter registered in default registry")
	}
}

func TestAdapter_DevicePortOverride(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)
	device := makeTestDevice(mock.addr)
	cred := makeTestCredential(certs)

	adapter := &Adapter{
		config:  DefaultConfig(),
		metrics: &protocols.AdapterMetrics{},
	}

	ctx := context.Background()
	err := adapter.Connect(ctx, device, cred)
	if err != nil {
		t.Fatalf("failed to connect with device port override: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck
}
