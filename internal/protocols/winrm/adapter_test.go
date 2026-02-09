package winrm

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Port != 5985 {
		t.Errorf("Port = %d, want 5985", cfg.Port)
	}
	if cfg.HTTPS != false {
		t.Errorf("HTTPS = %v, want false", cfg.HTTPS)
	}
	if cfg.Insecure != false {
		t.Errorf("Insecure = %v, want false", cfg.Insecure)
	}
	if cfg.UseNTLM != true {
		t.Errorf("UseNTLM = %v, want true", cfg.UseNTLM)
	}
	if cfg.OperationTimeout != 60*time.Second {
		t.Errorf("OperationTimeout = %v, want 60s", cfg.OperationTimeout)
	}
	if cfg.DefaultShell != "powershell" {
		t.Errorf("DefaultShell = %v, want 'powershell'", cfg.DefaultShell)
	}
	if cfg.ConnectionConfig == nil {
		t.Error("ConnectionConfig should not be nil")
	}
}

func TestNewAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewAdapter(nil)
		if adapter == nil {
			t.Fatal("expected adapter to be created")
		}
		if adapter.config == nil {
			t.Error("config should not be nil")
		}
		if adapter.config.Port != 5985 {
			t.Errorf("Port = %d, want 5985", adapter.config.Port)
		}
		if adapter.metrics == nil {
			t.Error("metrics should be initialized")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &Config{
			Port:             5986,
			HTTPS:            true,
			Insecure:         true,
			UseNTLM:          false,
			UseKerberos:      true,
			DefaultShell:     "cmd",
			ConnectionConfig: nil, // Should be set to default
		}
		adapter := NewAdapter(cfg)
		if adapter.config.Port != 5986 {
			t.Errorf("Port = %d, want 5986", adapter.config.Port)
		}
		if adapter.config.HTTPS != true {
			t.Errorf("HTTPS = %v, want true", adapter.config.HTTPS)
		}
		if adapter.config.ConnectionConfig == nil {
			t.Error("ConnectionConfig should be set to default")
		}
	})
}

func TestAdapterType(t *testing.T) {
	adapter := NewAdapter(nil)
	if adapter.Type() != protocols.ProtocolWinRM {
		t.Errorf("Type() = %v, want ProtocolWinRM", adapter.Type())
	}
}

func TestAdapterIsConnected(t *testing.T) {
	adapter := NewAdapter(nil)

	// New adapter should not be connected
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestAdapterClient(t *testing.T) {
	adapter := NewAdapter(nil)

	// New adapter should have nil client
	if adapter.Client() != nil {
		t.Error("new adapter should have nil client")
	}
}

func TestAdapterMetrics(t *testing.T) {
	adapter := NewAdapter(nil)

	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("metrics should not be nil")
	}
	if metrics.ConnectionCount != 0 {
		t.Errorf("ConnectionCount = %d, want 0", metrics.ConnectionCount)
	}
	if metrics.ExecutionCount != 0 {
		t.Errorf("ExecutionCount = %d, want 0", metrics.ExecutionCount)
	}
}

func TestUtf16LEEncode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []byte
	}{
		{
			name:  "ASCII",
			input: "Hello",
			want:  []byte{0x48, 0x00, 0x65, 0x00, 0x6c, 0x00, 0x6c, 0x00, 0x6f, 0x00},
		},
		{
			name:  "empty string",
			input: "",
			want:  []byte{},
		},
		{
			name:  "single char",
			input: "A",
			want:  []byte{0x41, 0x00},
		},
		{
			name:  "numbers",
			input: "123",
			want:  []byte{0x31, 0x00, 0x32, 0x00, 0x33, 0x00},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := utf16LEEncode(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("utf16LEEncode(%q) length = %d, want %d", tt.input, len(got), len(tt.want))
				return
			}
			for i, b := range got {
				if b != tt.want[i] {
					t.Errorf("utf16LEEncode(%q)[%d] = %02x, want %02x", tt.input, i, b, tt.want[i])
				}
			}
		})
	}
}

func TestUtf16EncodePair(t *testing.T) {
	// Test surrogate pair encoding for emoji (outside BMP)
	// 🎉 (U+1F389) should produce surrogate pair
	emoji := '\U0001F389' // 🎉
	r1, r2 := utf16EncodePair(emoji)

	// High surrogate should be in range 0xD800-0xDBFF
	if r1 < 0xD800 || r1 > 0xDBFF {
		t.Errorf("high surrogate %04X out of range [D800, DBFF]", r1)
	}

	// Low surrogate should be in range 0xDC00-0xDFFF
	if r2 < 0xDC00 || r2 > 0xDFFF {
		t.Errorf("low surrogate %04X out of range [DC00, DFFF]", r2)
	}
}

func TestEncodePowerShellCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "simple command",
			command: "Get-Process",
		},
		{
			name:    "command with args",
			command: "Get-Process -Name notepad",
		},
		{
			name:    "command with special chars",
			command: "Write-Host 'Hello, World!'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := encodePowerShellCommand(tt.command)
			if encoded == "" {
				t.Error("encoded command should not be empty")
			}
			// Base64 encoded string should be valid
			if len(encoded)%4 != 0 {
				t.Error("encoded command should be valid base64 (length multiple of 4)")
			}
		})
	}
}

func TestNewAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewAdapterFactory(nil)
		if factory == nil {
			t.Fatal("expected factory to be created")
		}

		connConfig := protocols.DefaultConnectionConfig()
		adapter, err := factory(connConfig)
		if err != nil {
			t.Errorf("factory failed: %v", err)
		}
		if adapter == nil {
			t.Error("expected adapter from factory")
		}
		if adapter.Type() != protocols.ProtocolWinRM {
			t.Errorf("expected WinRM protocol, got %v", adapter.Type())
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &Config{
			Port:  5986,
			HTTPS: true,
		}
		factory := NewAdapterFactory(cfg)
		if factory == nil {
			t.Fatal("expected factory to be created")
		}

		connConfig := protocols.DefaultConnectionConfig()
		adapter, err := factory(connConfig)
		if err != nil {
			t.Errorf("factory failed: %v", err)
		}
		if adapter == nil {
			t.Error("expected adapter from factory")
		}
	})
}

func TestConfigStructure(t *testing.T) {
	cfg := &Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		Port:             5986,
		HTTPS:            true,
		Insecure:         false,
		CACert:           []byte("cacert"),
		CertPEM:          []byte("certpem"),
		KeyPEM:           []byte("keypem"),
		UseNTLM:          true,
		UseKerberos:      false,
		OperationTimeout: 120 * time.Second,
		DefaultShell:     "cmd",
	}

	if cfg.Port != 5986 {
		t.Errorf("Port = %d", cfg.Port)
	}
	if cfg.HTTPS != true {
		t.Errorf("HTTPS = %v", cfg.HTTPS)
	}
	if string(cfg.CACert) != "cacert" {
		t.Errorf("CACert = %v", cfg.CACert)
	}
	if cfg.OperationTimeout != 120*time.Second {
		t.Errorf("OperationTimeout = %v", cfg.OperationTimeout)
	}
}

func TestAdapterNotConnected(t *testing.T) {
	adapter := NewAdapter(nil)

	t.Run("RunPowerShell not connected", func(t *testing.T) {
		_, _, _, err := adapter.RunPowerShell(context.Background(), "Get-Process")
		if err == nil {
			t.Error("expected error when not connected")
		}
		if err.Error() != "not connected" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("RunCmd not connected", func(t *testing.T) {
		_, _, _, err := adapter.RunCmd(context.Background(), "dir")
		if err == nil {
			t.Error("expected error when not connected")
		}
		if err.Error() != "not connected" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestAdapterDisconnect(t *testing.T) {
	adapter := NewAdapter(nil)

	// Disconnect on unconnected adapter should succeed
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}

	// Adapter should report disconnected
	if adapter.IsConnected() {
		t.Error("adapter should be disconnected")
	}
}

func TestAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewAdapter(nil)

	result, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Healthy {
		t.Error("should not be healthy when not connected")
	}
	if result.Status != "not connected" {
		t.Errorf("Status = %v, want 'not connected'", result.Status)
	}
}
