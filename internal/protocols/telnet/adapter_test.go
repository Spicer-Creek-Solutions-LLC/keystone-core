package telnet

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// Compile-time interface compliance.
var _ protocols.ProtocolAdapter = (*Adapter)(nil)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	if config.Port != 23 {
		t.Errorf("expected Port=23, got %d", config.Port)
	}
	if config.Prompt != "# " {
		t.Errorf("expected Prompt='# ', got %q", config.Prompt)
	}
	if config.TermType != "vt100" {
		t.Errorf("expected TermType=vt100, got %q", config.TermType)
	}
	if config.Rows != 24 {
		t.Errorf("expected Rows=24, got %d", config.Rows)
	}
	if config.Cols != 80 {
		t.Errorf("expected Cols=80, got %d", config.Cols)
	}
	if len(config.LoginPrompts) == 0 {
		t.Error("expected non-empty LoginPrompts")
	}
	if len(config.PasswordPrompts) == 0 {
		t.Error("expected non-empty PasswordPrompts")
	}
	if config.Security == nil || !config.Security.DeprecationWarning {
		t.Error("expected Security.DeprecationWarning=true")
	}
	if config.ConnectionConfig == nil {
		t.Error("expected non-nil ConnectionConfig")
	}
}

func TestNewAdapter_Defaults(t *testing.T) {
	adapter := NewAdapter(nil, nil)
	if adapter.config == nil {
		t.Fatal("expected non-nil config")
	}
	if adapter.metrics == nil {
		t.Fatal("expected non-nil metrics")
	}
}

func TestNewAdapter_CustomConfig(t *testing.T) {
	config := &Config{
		Port:     2323,
		Prompt:   "> ",
		TermType: "xterm",
	}
	adapter := NewAdapter(config, nil)
	if adapter.config.Port != 2323 {
		t.Errorf("expected Port=2323, got %d", adapter.config.Port)
	}
	if adapter.config.ConnectionConfig == nil {
		t.Error("expected ConnectionConfig to be set to default")
	}
}

func TestAdapter_Type(t *testing.T) {
	adapter := NewAdapter(nil, nil)
	if adapter.Type() != protocols.ProtocolTelnet {
		t.Errorf("expected type telnet, got %s", adapter.Type())
	}
}

func TestAdapter_IsConnected_NotConnected(t *testing.T) {
	adapter := NewAdapter(nil, nil)
	if adapter.IsConnected() {
		t.Error("expected not connected")
	}
}

func TestAdapter_Metrics(t *testing.T) {
	adapter := NewAdapter(nil, nil)
	m := adapter.Metrics()
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.ConnectionCount != 0 {
		t.Errorf("expected ConnectionCount=0, got %d", m.ConnectionCount)
	}
}

func TestAdapter_Connect_UnsupportedCredential(t *testing.T) {
	adapter := NewAdapter(nil, nil)
	device := &proxy.ProxiedDevice{
		ID:      "test-device",
		Address: "10.0.0.1",
	}
	// Use a non-SSHPasswordCredential
	cred := &credentials.SSHKeyCredential{}
	err := adapter.Connect(context.Background(), device, cred)
	if err == nil {
		t.Error("expected error for unsupported credential type")
	}
}

func TestAdapter_Connect_DeniedByAllowlist(t *testing.T) {
	config := DefaultConfig()
	config.Security = &SecurityConfig{
		AllowedNetworks: []string{"192.168.0.0/16"},
	}
	adapter := NewAdapter(config, nil)
	device := &proxy.ProxiedDevice{
		ID:      "test-device",
		Address: "10.0.0.1",
	}
	cred := &credentials.SSHPasswordCredential{Username: "admin", Password: "pass"}
	err := adapter.Connect(context.Background(), device, cred)
	if err == nil {
		t.Error("expected error for denied IP")
	}
}

func TestAdapter_Execute_NotConnected(t *testing.T) {
	adapter := NewAdapter(nil, nil)
	_, err := adapter.Execute(context.Background(), &protocols.ExecuteRequest{Command: "test"})
	if err == nil {
		t.Error("expected error for not connected")
	}
}

func TestAdapter_Disconnect_NotConnected(t *testing.T) {
	adapter := NewAdapter(nil, nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("disconnect on non-connected should not error, got %v", err)
	}
}

func TestAdapter_HealthCheck_NotConnected(t *testing.T) {
	adapter := NewAdapter(nil, nil)
	result, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy when not connected")
	}
	if result.Status != "not connected" {
		t.Errorf("expected status 'not connected', got %q", result.Status)
	}
}

func TestAdapter_ConnectAndExecute(t *testing.T) {
	// Create a fake Telnet server using a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)

		// Send login prompt
		conn.Write([]byte("Login: "))

		// Read username
		conn.Read(buf)

		// Send password prompt
		conn.Write([]byte("Password: "))

		// Read password
		conn.Read(buf)

		// Send welcome + prompt
		conn.Write([]byte("Welcome\r\n# "))

		// Read command
		conn.Read(buf)

		// Send command output + prompt
		conn.Write([]byte("show version\r\nVersion 1.0\r\n# "))
	}()

	// Parse port
	var port int
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	config := DefaultConfig()
	config.Port = port
	config.Security = &SecurityConfig{DeprecationWarning: false}
	config.Timeout = 5 * time.Second

	adapter := NewAdapter(config, nil)
	device := &proxy.ProxiedDevice{
		ID:      "test-device",
		Address: "127.0.0.1",
	}
	cred := &credentials.SSHPasswordCredential{Username: "admin", Password: "secret"}

	ctx := context.Background()

	// Connect
	if err := adapter.Connect(ctx, device, cred); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	if !adapter.IsConnected() {
		t.Error("expected connected")
	}

	// Execute
	result, err := adapter.Execute(ctx, &protocols.ExecuteRequest{Command: "show version"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected ExitCode=0, got %d", result.ExitCode)
	}

	// Health check
	health, err := adapter.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck error: %v", err)
	}
	if !health.Healthy {
		t.Error("expected healthy")
	}

	// Metrics
	m := adapter.Metrics()
	if m.ConnectionCount != 1 {
		t.Errorf("expected ConnectionCount=1, got %d", m.ConnectionCount)
	}
	if m.ExecutionCount != 1 {
		t.Errorf("expected ExecutionCount=1, got %d", m.ExecutionCount)
	}

	// Disconnect
	if err := adapter.Disconnect(ctx); err != nil {
		t.Errorf("Disconnect error: %v", err)
	}
	if adapter.IsConnected() {
		t.Error("expected disconnected")
	}
}

func TestAdapter_DevicePortOverride(t *testing.T) {
	config := DefaultConfig()
	config.Security = &SecurityConfig{DeprecationWarning: false}
	config.Timeout = 1 * time.Second

	adapter := NewAdapter(config, nil)
	device := &proxy.ProxiedDevice{
		ID:      "test-device",
		Address: "127.0.0.1",
		Port:    19999, // Non-listening port
	}
	cred := &credentials.SSHPasswordCredential{Username: "admin", Password: "pass"}

	// Should fail to connect (no listener on that port) but should use device.Port
	err := adapter.Connect(context.Background(), device, cred)
	if err == nil {
		t.Error("expected connection error to non-listening port")
	}
}

func TestAdapter_SessionExpiry(t *testing.T) {
	// Create a fake server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		conn.Write([]byte("Login: "))
		conn.Read(buf)
		conn.Write([]byte("Password: "))
		conn.Read(buf)
		conn.Write([]byte("OK\r\n# "))

		// Keep connection alive
		for {
			_, err := conn.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	var port int
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	config := DefaultConfig()
	config.Port = port
	config.Security = &SecurityConfig{
		DeprecationWarning: false,
		MaxSessionDuration: 1 * time.Millisecond,
	}
	config.Timeout = 5 * time.Second

	adapter := NewAdapter(config, nil)
	device := &proxy.ProxiedDevice{
		ID:      "test-device",
		Address: "127.0.0.1",
	}
	cred := &credentials.SSHPasswordCredential{Username: "admin", Password: "pass"}

	ctx := context.Background()
	if err := adapter.Connect(ctx, device, cred); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	// Wait for session to expire
	time.Sleep(5 * time.Millisecond)

	_, err = adapter.Execute(ctx, &protocols.ExecuteRequest{Command: "test"})
	if err == nil {
		t.Error("expected session expired error")
	}

	// Health check should also show expired
	health, _ := adapter.HealthCheck(ctx)
	if health.Healthy {
		t.Error("expected unhealthy due to session expiry")
	}
}

func TestNewAdapterFactory(t *testing.T) {
	factory := NewAdapterFactory(nil)
	adapter, err := factory(protocols.DefaultConnectionConfig())
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if adapter.Type() != protocols.ProtocolTelnet {
		t.Errorf("expected type telnet, got %s", adapter.Type())
	}
}

func TestNewAdapterFactory_CustomConfig(t *testing.T) {
	config := &Config{Port: 2323}
	factory := NewAdapterFactory(config)
	connConfig := protocols.DefaultConnectionConfig()
	adapter, err := factory(connConfig)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	telnetAdapter := adapter.(*Adapter)
	if telnetAdapter.config.Port != 2323 {
		t.Errorf("expected Port=2323, got %d", telnetAdapter.config.Port)
	}
}

func TestRegistryRegistration(t *testing.T) {
	if !protocols.DefaultRegistry.Has(protocols.ProtocolTelnet) {
		t.Error("expected Telnet to be registered in DefaultRegistry")
	}
}

func TestAdapter_Execute_WithArgs(t *testing.T) {
	// Create a fake server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		conn.Write([]byte("Login: "))
		conn.Read(buf)
		conn.Write([]byte("Password: "))
		conn.Read(buf)
		conn.Write([]byte("\r\n# "))

		// Read command
		n, _ := conn.Read(buf)
		cmd := string(buf[:n])
		// Echo back command + output + prompt
		conn.Write([]byte(cmd + "result\r\n# "))
	}()

	var port int
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	config := DefaultConfig()
	config.Port = port
	config.Security = &SecurityConfig{DeprecationWarning: false}
	config.Timeout = 5 * time.Second

	adapter := NewAdapter(config, nil)
	device := &proxy.ProxiedDevice{ID: "test", Address: "127.0.0.1"}
	cred := &credentials.SSHPasswordCredential{Username: "admin", Password: "pass"}

	ctx := context.Background()
	if err := adapter.Connect(ctx, device, cred); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer adapter.Disconnect(ctx)

	result, err := adapter.Execute(ctx, &protocols.ExecuteRequest{
		Command: "ping",
		Args:    []string{"-c", "3", "10.0.0.1"},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected ExitCode=0, got %d", result.ExitCode)
	}
}
