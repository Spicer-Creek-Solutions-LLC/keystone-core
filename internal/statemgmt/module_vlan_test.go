package statemgmt

import (
	"context"
	"testing"
)

func TestNewVLANModule(t *testing.T) {
	m := NewVLANModule()
	if m == nil {
		t.Fatal("NewVLANModule returned nil")
	}
	if m.Name() != "vlan" {
		t.Errorf("expected module name 'vlan', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 2 {
		t.Errorf("expected 2 valid states, got %d", len(states))
	}
	hasPresent := false
	hasAbsent := false
	for _, s := range states {
		if s == "present" {
			hasPresent = true
		}
		if s == "absent" {
			hasAbsent = true
		}
	}
	if !hasPresent || !hasAbsent {
		t.Error("expected 'present' and 'absent' states")
	}
}

func TestVLANModule_ParseConfig(t *testing.T) {
	m := NewVLANModule()

	tests := []struct {
		name        string
		decl        *StateDeclaration
		expectError bool
		validate    func(*VLANConfig) error
	}{
		{
			name: "basic VLAN config",
			decl: &StateDeclaration{
				ID:     "eth0.100",
				State:  "present",
				Module: "vlan",
				Parameters: map[string]interface{}{
					"parent": "eth0",
					"id":     100,
				},
			},
			expectError: false,
			validate: func(c *VLANConfig) error {
				if c.Name != "eth0.100" {
					t.Errorf("expected name 'eth0.100', got '%s'", c.Name)
				}
				if c.Parent != "eth0" {
					t.Errorf("expected parent 'eth0', got '%s'", c.Parent)
				}
				if c.ID != 100 {
					t.Errorf("expected ID 100, got %d", c.ID)
				}
				return nil
			},
		},
		{
			name: "VLAN with addresses",
			decl: &StateDeclaration{
				ID:     "vlan100",
				State:  "present",
				Module: "vlan",
				Parameters: map[string]interface{}{
					"parent":    "eth0",
					"vlan_id":   100,
					"addresses": []interface{}{"192.168.100.1/24", "192.168.100.2/24"},
					"gateway":   "192.168.100.254",
					"dns":       []interface{}{"8.8.8.8", "8.8.4.4"},
					"mtu":       1500,
				},
			},
			expectError: false,
			validate: func(c *VLANConfig) error {
				if len(c.Addresses) != 2 {
					t.Errorf("expected 2 addresses, got %d", len(c.Addresses))
				}
				if c.Gateway != "192.168.100.254" {
					t.Errorf("expected gateway '192.168.100.254', got '%s'", c.Gateway)
				}
				if len(c.DNS) != 2 {
					t.Errorf("expected 2 DNS servers, got %d", len(c.DNS))
				}
				if c.MTU != 1500 {
					t.Errorf("expected MTU 1500, got %d", c.MTU)
				}
				return nil
			},
		},
		{
			name: "missing parent",
			decl: &StateDeclaration{
				ID:     "vlan100",
				State:  "present",
				Module: "vlan",
				Parameters: map[string]interface{}{
					"id": 100,
				},
			},
			expectError: true,
		},
		{
			name: "missing VLAN ID",
			decl: &StateDeclaration{
				ID:     "vlan100",
				State:  "present",
				Module: "vlan",
				Parameters: map[string]interface{}{
					"parent": "eth0",
				},
			},
			expectError: true,
		},
		{
			name: "VLAN ID out of range (low)",
			decl: &StateDeclaration{
				ID:     "vlan0",
				State:  "present",
				Module: "vlan",
				Parameters: map[string]interface{}{
					"parent": "eth0",
					"id":     0,
				},
			},
			expectError: true,
		},
		{
			name: "VLAN ID out of range (high)",
			decl: &StateDeclaration{
				ID:     "vlan9999",
				State:  "present",
				Module: "vlan",
				Parameters: map[string]interface{}{
					"parent": "eth0",
					"id":     9999,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseVLANConfig(tt.decl)
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

func TestVLANModule_Check_NonexistentInterface(t *testing.T) {
	m := NewVLANModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nonexistent.999",
		State:  "absent",
		Module: "vlan",
		Parameters: map[string]interface{}{
			"parent": "nonexistent",
			"id":     999,
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected VLAN to not be present")
	}
	if !result.Matches {
		t.Error("expected state to match for absent interface")
	}
	if result.CurrentState != "absent" {
		t.Errorf("expected current state 'absent', got '%s'", result.CurrentState)
	}
}

func TestVLANModule_Test(t *testing.T) {
	m := NewVLANModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nonexistent.999",
		State:  "absent",
		Module: "vlan",
		Parameters: map[string]interface{}{
			"parent": "nonexistent",
			"id":     999,
		},
	}

	matches, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if !matches {
		t.Error("expected Test to return true for absent VLAN that doesn't exist")
	}
}
