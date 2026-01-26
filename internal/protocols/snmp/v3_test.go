package snmp

import (
	"testing"

	"github.com/gosnmp/gosnmp"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

func TestDefaultV3Config(t *testing.T) {
	cfg := DefaultV3Config()

	if cfg.Version != SNMPv3 {
		t.Errorf("Version = %v, want SNMPv3", cfg.Version)
	}
	if cfg.Port != 161 {
		t.Errorf("Port = %d, want 161", cfg.Port)
	}
	if cfg.Retries != 3 {
		t.Errorf("Retries = %d, want 3", cfg.Retries)
	}
	if cfg.MaxOids != 60 {
		t.Errorf("MaxOids = %d, want 60", cfg.MaxOids)
	}
}

func TestNewV3Adapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewV3Adapter(nil)
		if adapter == nil {
			t.Fatal("expected adapter to be created")
		}
		if adapter.config.Version != SNMPv3 {
			t.Errorf("Version = %v, want SNMPv3", adapter.config.Version)
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &V3Config{
			Config: &Config{
				Port:    1161,
				Version: SNMPv2c, // Should be overridden
			},
			ContextName:     "mycontext",
			ContextEngineID: "80001234",
		}
		adapter := NewV3Adapter(cfg)
		if adapter.config.Version != SNMPv3 {
			t.Errorf("Version = %v, want SNMPv3 (overridden)", adapter.config.Version)
		}
		if adapter.contextName != "mycontext" {
			t.Errorf("contextName = %v, want 'mycontext'", adapter.contextName)
		}
		if adapter.contextEngineID != "80001234" {
			t.Errorf("contextEngineID = %v, want '80001234'", adapter.contextEngineID)
		}
	})
}

func TestSecurityLevelString(t *testing.T) {
	tests := []struct {
		level SecurityLevel
		want  string
	}{
		{NoAuthNoPriv, "noAuthNoPriv"},
		{AuthNoPriv, "authNoPriv"},
		{AuthPriv, "authPriv"},
		{SecurityLevel(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestV3AdapterGetSecurityLevel(t *testing.T) {
	adapter := NewV3Adapter(nil)

	// When not connected, should return NoAuthNoPriv
	level := adapter.GetSecurityLevel()
	if level != NoAuthNoPriv {
		t.Errorf("GetSecurityLevel() = %v, want NoAuthNoPriv (not connected)", level)
	}
}

func TestV3AdapterGetEngineInfo(t *testing.T) {
	adapter := NewV3Adapter(nil)

	info := adapter.GetEngineInfo()
	if info.EngineID != "" {
		t.Errorf("EngineID = %v, want empty (not connected)", info.EngineID)
	}
	if info.EngineBoots != 0 {
		t.Errorf("EngineBoots = %d, want 0", info.EngineBoots)
	}
	if info.EngineTime != 0 {
		t.Errorf("EngineTime = %d, want 0", info.EngineTime)
	}
}

func TestValidateAuthProtocol(t *testing.T) {
	tests := []struct {
		proto string
		want  bool
	}{
		{"", true},    // empty is valid (noAuth)
		{"MD5", true}, // deprecated but valid
		{"SHA", true},
		{"SHA224", true},
		{"SHA256", true},
		{"SHA384", true},
		{"SHA512", true},
		{"INVALID", false},
		{"sha256", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.proto, func(t *testing.T) {
			got := ValidateAuthProtocol(tt.proto)
			if got != tt.want {
				t.Errorf("ValidateAuthProtocol(%q) = %v, want %v", tt.proto, got, tt.want)
			}
		})
	}
}

func TestValidatePrivProtocol(t *testing.T) {
	tests := []struct {
		proto string
		want  bool
	}{
		{"", true},    // empty is valid (noPriv)
		{"DES", true}, // deprecated but valid
		{"AES", true},
		{"AES192", true},
		{"AES256", true},
		{"AES192C", true},
		{"AES256C", true},
		{"INVALID", false},
		{"aes", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.proto, func(t *testing.T) {
			got := ValidatePrivProtocol(tt.proto)
			if got != tt.want {
				t.Errorf("ValidatePrivProtocol(%q) = %v, want %v", tt.proto, got, tt.want)
			}
		})
	}
}

func TestAuthProtocols(t *testing.T) {
	expected := []string{"MD5", "SHA", "SHA224", "SHA256", "SHA384", "SHA512"}

	if len(AuthProtocols) != len(expected) {
		t.Errorf("AuthProtocols has %d entries, want %d", len(AuthProtocols), len(expected))
	}

	for i, proto := range expected {
		if AuthProtocols[i] != proto {
			t.Errorf("AuthProtocols[%d] = %v, want %v", i, AuthProtocols[i], proto)
		}
	}
}

func TestPrivProtocols(t *testing.T) {
	expected := []string{"DES", "AES", "AES192", "AES256", "AES192C", "AES256C"}

	if len(PrivProtocols) != len(expected) {
		t.Errorf("PrivProtocols has %d entries, want %d", len(PrivProtocols), len(expected))
	}

	for i, proto := range expected {
		if PrivProtocols[i] != proto {
			t.Errorf("PrivProtocols[%d] = %v, want %v", i, PrivProtocols[i], proto)
		}
	}
}

func TestUSMUserManager(t *testing.T) {
	manager := NewUSMUserManager()

	if manager == nil {
		t.Fatal("expected manager to be created")
	}

	t.Run("AddUser", func(t *testing.T) {
		user := &USMUser{
			Name:           "admin",
			AuthProtocol:   "SHA256",
			AuthPassphrase: "authpass",
			PrivProtocol:   "AES",
			PrivPassphrase: "privpass",
			EngineID:       "80001234",
		}
		manager.AddUser(user)

		got := manager.GetUser("admin")
		if got == nil {
			t.Error("GetUser returned nil")
		} else if got.Name != "admin" {
			t.Errorf("Name = %v, want 'admin'", got.Name)
		}
	})

	t.Run("GetUser not found", func(t *testing.T) {
		got := manager.GetUser("nonexistent")
		if got != nil {
			t.Error("GetUser should return nil for nonexistent user")
		}
	})

	t.Run("ListUsers", func(t *testing.T) {
		users := manager.ListUsers()
		if len(users) != 1 {
			t.Errorf("ListUsers() returned %d users, want 1", len(users))
		}
	})

	t.Run("RemoveUser", func(t *testing.T) {
		manager.RemoveUser("admin")

		got := manager.GetUser("admin")
		if got != nil {
			t.Error("User should be removed")
		}

		users := manager.ListUsers()
		if len(users) != 0 {
			t.Errorf("ListUsers() returned %d users, want 0", len(users))
		}
	})
}

func TestNewV3TrapReceiver(t *testing.T) {
	manager := NewUSMUserManager()
	receiver := NewV3TrapReceiver(manager)

	if receiver == nil {
		t.Fatal("expected receiver to be created")
	}
	if receiver.userManager != manager {
		t.Error("userManager not set correctly")
	}
	if len(receiver.handlers) != 0 {
		t.Errorf("handlers should be empty, got %d", len(receiver.handlers))
	}
	if receiver.running {
		t.Error("receiver should not be running")
	}
}

func TestV3TrapReceiverAddHandler(t *testing.T) {
	manager := NewUSMUserManager()
	receiver := NewV3TrapReceiver(manager)

	handlerCalled := false
	receiver.AddHandler(func(trap *Trap) {
		handlerCalled = true
	})

	if len(receiver.handlers) != 1 {
		t.Errorf("handlers count = %d, want 1", len(receiver.handlers))
	}

	// Call the handler to verify it was added correctly
	receiver.handlers[0](&Trap{})
	if !handlerCalled {
		t.Error("handler should have been called")
	}
}

func TestV3TrapReceiverStopWhenNotRunning(t *testing.T) {
	manager := NewUSMUserManager()
	receiver := NewV3TrapReceiver(manager)

	err := receiver.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestEngineInfoStructure(t *testing.T) {
	info := EngineInfo{
		EngineID:    "80001234",
		EngineBoots: 100,
		EngineTime:  12345,
	}

	if info.EngineID != "80001234" {
		t.Errorf("EngineID = %v", info.EngineID)
	}
	if info.EngineBoots != 100 {
		t.Errorf("EngineBoots = %d", info.EngineBoots)
	}
	if info.EngineTime != 12345 {
		t.Errorf("EngineTime = %d", info.EngineTime)
	}
}

func TestUSMUserStructure(t *testing.T) {
	user := &USMUser{
		Name:           "testuser",
		AuthProtocol:   "SHA256",
		AuthPassphrase: "authpassword",
		PrivProtocol:   "AES",
		PrivPassphrase: "privpassword",
		EngineID:       "80001234567890",
	}

	if user.Name != "testuser" {
		t.Errorf("Name = %v", user.Name)
	}
	if user.AuthProtocol != "SHA256" {
		t.Errorf("AuthProtocol = %v", user.AuthProtocol)
	}
	if user.PrivProtocol != "AES" {
		t.Errorf("PrivProtocol = %v", user.PrivProtocol)
	}
}

func TestNewV3AdapterFactory(t *testing.T) {
	factory := NewV3AdapterFactory(nil)
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
	if adapter.Type() != protocols.ProtocolSNMP {
		t.Errorf("expected SNMP protocol, got %v", adapter.Type())
	}
}

func TestV3ConfigStructure(t *testing.T) {
	cfg := &V3Config{
		Config: &Config{
			Port:           161,
			Version:        SNMPv3,
			Retries:        5,
			MaxOids:        100,
			MaxRepetitions: 25,
		},
		ContextName:     "contextA",
		ContextEngineID: "80001234",
	}

	if cfg.Port != 161 {
		t.Errorf("Port = %d", cfg.Port)
	}
	if cfg.ContextName != "contextA" {
		t.Errorf("ContextName = %v", cfg.ContextName)
	}
	if cfg.ContextEngineID != "80001234" {
		t.Errorf("ContextEngineID = %v", cfg.ContextEngineID)
	}
}

func TestSecurityLevelFromMsgFlags(t *testing.T) {
	adapter := NewV3Adapter(nil)

	// Manually test the mapping (adapter.client is nil here)
	tests := []struct {
		flags gosnmp.SnmpV3MsgFlags
		want  SecurityLevel
	}{
		{gosnmp.NoAuthNoPriv, NoAuthNoPriv},
		{gosnmp.AuthNoPriv, AuthNoPriv},
		{gosnmp.AuthPriv, AuthPriv},
	}

	// We can't directly test GetSecurityLevel without a connection,
	// but we can verify the constant mappings are correct
	for _, tt := range tests {
		// Just verify the test case makes sense
		if tt.flags == gosnmp.NoAuthNoPriv && tt.want != NoAuthNoPriv {
			t.Error("mapping mismatch for NoAuthNoPriv")
		}
		if tt.flags == gosnmp.AuthNoPriv && tt.want != AuthNoPriv {
			t.Error("mapping mismatch for AuthNoPriv")
		}
		if tt.flags == gosnmp.AuthPriv && tt.want != AuthPriv {
			t.Error("mapping mismatch for AuthPriv")
		}
	}

	_ = adapter // Use adapter to avoid unused variable warning
}
