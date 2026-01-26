package statemgmt

import (
	"context"
	"testing"
)

func TestNewBridgeModule(t *testing.T) {
	m := NewBridgeModule()
	if m == nil {
		t.Fatal("NewBridgeModule returned nil")
	}
	if m.Name() != "bridge" {
		t.Errorf("expected module name 'bridge', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 2 {
		t.Errorf("expected 2 valid states, got %d", len(states))
	}
}

func TestBridgeModule_ParseConfig(t *testing.T) {
	m := NewBridgeModule()

	tests := []struct {
		name        string
		decl        *StateDeclaration
		expectError bool
		validate    func(*BridgeConfig) error
	}{
		{
			name: "basic bridge config",
			decl: &StateDeclaration{
				ID:     "br0",
				State:  "present",
				Module: "bridge",
				Parameters: map[string]interface{}{
					"ports": []interface{}{"eth0", "eth1"},
				},
			},
			expectError: false,
			validate: func(c *BridgeConfig) error {
				if c.Name != "br0" {
					t.Errorf("expected name 'br0', got '%s'", c.Name)
				}
				if len(c.Ports) != 2 {
					t.Errorf("expected 2 ports, got %d", len(c.Ports))
				}
				if c.STP != false {
					t.Error("expected STP to be false by default")
				}
				if c.ForwardDelay != 15 {
					t.Errorf("expected default forward_delay 15, got %d", c.ForwardDelay)
				}
				return nil
			},
		},
		{
			name: "bridge with STP",
			decl: &StateDeclaration{
				ID:     "br0",
				State:  "present",
				Module: "bridge",
				Parameters: map[string]interface{}{
					"ports":         []interface{}{"eth0", "eth1"},
					"stp":           true,
					"forward_delay": 4,
					"hello_time":    1,
					"max_age":       10,
				},
			},
			expectError: false,
			validate: func(c *BridgeConfig) error {
				if !c.STP {
					t.Error("expected STP to be true")
				}
				if c.ForwardDelay != 4 {
					t.Errorf("expected forward_delay 4, got %d", c.ForwardDelay)
				}
				if c.HelloTime != 1 {
					t.Errorf("expected hello_time 1, got %d", c.HelloTime)
				}
				if c.MaxAge != 10 {
					t.Errorf("expected max_age 10, got %d", c.MaxAge)
				}
				return nil
			},
		},
		{
			name: "bridge with addresses",
			decl: &StateDeclaration{
				ID:     "br0",
				State:  "present",
				Module: "bridge",
				Parameters: map[string]interface{}{
					"ports":     []interface{}{"eth0"},
					"addresses": []interface{}{"192.168.1.1/24"},
					"gateway":   "192.168.1.254",
					"dns":       []interface{}{"8.8.8.8"},
					"mtu":       1500,
				},
			},
			expectError: false,
			validate: func(c *BridgeConfig) error {
				if len(c.Addresses) != 1 {
					t.Errorf("expected 1 address, got %d", len(c.Addresses))
				}
				if c.Gateway != "192.168.1.254" {
					t.Errorf("expected gateway '192.168.1.254', got '%s'", c.Gateway)
				}
				if c.MTU != 1500 {
					t.Errorf("expected MTU 1500, got %d", c.MTU)
				}
				return nil
			},
		},
		{
			name: "bridge with interfaces (alias for ports)",
			decl: &StateDeclaration{
				ID:     "br0",
				State:  "present",
				Module: "bridge",
				Parameters: map[string]interface{}{
					"interfaces": []interface{}{"eth0", "eth1"},
				},
			},
			expectError: false,
			validate: func(c *BridgeConfig) error {
				if len(c.Ports) != 2 {
					t.Errorf("expected 2 ports, got %d", len(c.Ports))
				}
				return nil
			},
		},
		{
			name: "bridge with no ports",
			decl: &StateDeclaration{
				ID:         "br0",
				State:      "present",
				Module:     "bridge",
				Parameters: map[string]interface{}{},
			},
			expectError: false,
			validate: func(c *BridgeConfig) error {
				if len(c.Ports) != 0 {
					t.Errorf("expected 0 ports, got %d", len(c.Ports))
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseBridgeConfig(tt.decl)
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tt.validate != nil {
				tt.validate(config)
			}
		})
	}
}

func TestBridgeModule_Check_NonexistentInterface(t *testing.T) {
	m := NewBridgeModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nonexistent_bridge",
		State:  "absent",
		Module: "bridge",
		Parameters: map[string]interface{}{
			"ports": []interface{}{"eth0"},
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected bridge to not be present")
	}
	if !result.Matches {
		t.Error("expected state to match for absent interface")
	}
	if result.CurrentState != "absent" {
		t.Errorf("expected current state 'absent', got '%s'", result.CurrentState)
	}
}

func TestBridgeModule_Test(t *testing.T) {
	m := NewBridgeModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nonexistent_bridge",
		State:  "absent",
		Module: "bridge",
		Parameters: map[string]interface{}{
			"ports": []interface{}{"eth0"},
		},
	}

	matches, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if !matches {
		t.Error("expected Test to return true for absent bridge that doesn't exist")
	}
}

func TestBridgeConfig_DefaultValues(t *testing.T) {
	m := NewBridgeModule()

	decl := &StateDeclaration{
		ID:         "br0",
		State:      "present",
		Module:     "bridge",
		Parameters: map[string]interface{}{},
	}

	config, err := m.parseBridgeConfig(decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.ForwardDelay != 15 {
		t.Errorf("expected default forward_delay 15, got %d", config.ForwardDelay)
	}
	if config.HelloTime != 2 {
		t.Errorf("expected default hello_time 2, got %d", config.HelloTime)
	}
	if config.MaxAge != 20 {
		t.Errorf("expected default max_age 20, got %d", config.MaxAge)
	}
	if config.AgeingTime != 300 {
		t.Errorf("expected default ageing_time 300, got %d", config.AgeingTime)
	}
	if config.STP != false {
		t.Error("expected default STP to be false")
	}
}
