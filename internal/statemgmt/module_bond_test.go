package statemgmt

import (
	"context"
	"testing"
)

func TestNewBondModule(t *testing.T) {
	m := NewBondModule()
	if m == nil {
		t.Fatal("NewBondModule returned nil")
	}
	if m.Name() != "bond" {
		t.Errorf("expected module name 'bond', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 2 {
		t.Errorf("expected 2 valid states, got %d", len(states))
	}
}

func TestBondModule_ParseConfig(t *testing.T) {
	m := NewBondModule()

	tests := []struct {
		name        string
		decl        *StateDeclaration
		expectError bool
		validate    func(*BondConfig) error
	}{
		{
			name: "basic bond config",
			decl: &StateDeclaration{
				ID:     "bond0",
				State:  "present",
				Module: "bond",
				Parameters: map[string]interface{}{
					"slaves": []interface{}{"eth0", "eth1"},
					"mode":   "active-backup",
				},
			},
			expectError: false,
			validate: func(c *BondConfig) error {
				if c.Name != "bond0" {
					t.Errorf("expected name 'bond0', got '%s'", c.Name)
				}
				if len(c.Slaves) != 2 {
					t.Errorf("expected 2 slaves, got %d", len(c.Slaves))
				}
				if c.Mode != "active-backup" {
					t.Errorf("expected mode 'active-backup', got '%s'", c.Mode)
				}
				if c.MIIMon != 100 {
					t.Errorf("expected default miimon 100, got %d", c.MIIMon)
				}
				return nil
			},
		},
		{
			name: "bond with LACP",
			decl: &StateDeclaration{
				ID:     "bond0",
				State:  "present",
				Module: "bond",
				Parameters: map[string]interface{}{
					"slaves":           []interface{}{"eth0", "eth1"},
					"mode":             "802.3ad",
					"lacp_rate":        "fast",
					"xmit_hash_policy": "layer3+4",
					"miimon":           50,
				},
			},
			expectError: false,
			validate: func(c *BondConfig) error {
				if c.Mode != "802.3ad" {
					t.Errorf("expected mode '802.3ad', got '%s'", c.Mode)
				}
				if c.LACPRate != "fast" {
					t.Errorf("expected lacp_rate 'fast', got '%s'", c.LACPRate)
				}
				if c.XmitHashPolicy != "layer3+4" {
					t.Errorf("expected xmit_hash_policy 'layer3+4', got '%s'", c.XmitHashPolicy)
				}
				if c.MIIMon != 50 {
					t.Errorf("expected miimon 50, got %d", c.MIIMon)
				}
				return nil
			},
		},
		{
			name: "bond with addresses",
			decl: &StateDeclaration{
				ID:     "bond0",
				State:  "present",
				Module: "bond",
				Parameters: map[string]interface{}{
					"slaves":    []interface{}{"eth0", "eth1"},
					"mode":      "balance-rr",
					"addresses": []interface{}{"10.0.0.1/24"},
					"gateway":   "10.0.0.254",
					"dns":       []interface{}{"8.8.8.8"},
					"mtu":       9000,
				},
			},
			expectError: false,
			validate: func(c *BondConfig) error {
				if len(c.Addresses) != 1 {
					t.Errorf("expected 1 address, got %d", len(c.Addresses))
				}
				if c.Gateway != "10.0.0.254" {
					t.Errorf("expected gateway '10.0.0.254', got '%s'", c.Gateway)
				}
				if c.MTU != 9000 {
					t.Errorf("expected MTU 9000, got %d", c.MTU)
				}
				return nil
			},
		},
		{
			name: "missing slaves",
			decl: &StateDeclaration{
				ID:     "bond0",
				State:  "present",
				Module: "bond",
				Parameters: map[string]interface{}{
					"mode": "active-backup",
				},
			},
			expectError: true,
		},
		{
			name: "missing mode",
			decl: &StateDeclaration{
				ID:     "bond0",
				State:  "present",
				Module: "bond",
				Parameters: map[string]interface{}{
					"slaves": []interface{}{"eth0", "eth1"},
				},
			},
			expectError: true,
		},
		{
			name: "invalid mode",
			decl: &StateDeclaration{
				ID:     "bond0",
				State:  "present",
				Module: "bond",
				Parameters: map[string]interface{}{
					"slaves": []interface{}{"eth0", "eth1"},
					"mode":   "invalid-mode",
				},
			},
			expectError: true,
		},
		{
			name: "numeric mode",
			decl: &StateDeclaration{
				ID:     "bond0",
				State:  "present",
				Module: "bond",
				Parameters: map[string]interface{}{
					"slaves": []interface{}{"eth0", "eth1"},
					"mode":   "4",
				},
			},
			expectError: false,
			validate: func(c *BondConfig) error {
				if c.Mode != "4" {
					t.Errorf("expected mode '4', got '%s'", c.Mode)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseBondConfig(tt.decl)
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

func TestBondModule_Check_NonexistentInterface(t *testing.T) {
	m := NewBondModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nonexistent_bond",
		State:  "absent",
		Module: "bond",
		Parameters: map[string]interface{}{
			"slaves": []interface{}{"eth0", "eth1"},
			"mode":   "active-backup",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected bond to not be present")
	}
	if !result.Matches {
		t.Error("expected state to match for absent interface")
	}
	if result.CurrentState != "absent" {
		t.Errorf("expected current state 'absent', got '%s'", result.CurrentState)
	}
}

func TestBondModule_Test(t *testing.T) {
	m := NewBondModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nonexistent_bond",
		State:  "absent",
		Module: "bond",
		Parameters: map[string]interface{}{
			"slaves": []interface{}{"eth0", "eth1"},
			"mode":   "active-backup",
		},
	}

	matches, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if !matches {
		t.Error("expected Test to return true for absent bond that doesn't exist")
	}
}

func TestStringSlicesEqualUnordered(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected bool
	}{
		{
			name:     "empty slices",
			a:        []string{},
			b:        []string{},
			expected: true,
		},
		{
			name:     "same order",
			a:        []string{"eth0", "eth1"},
			b:        []string{"eth0", "eth1"},
			expected: true,
		},
		{
			name:     "different order",
			a:        []string{"eth0", "eth1"},
			b:        []string{"eth1", "eth0"},
			expected: true,
		},
		{
			name:     "different length",
			a:        []string{"eth0", "eth1"},
			b:        []string{"eth0"},
			expected: false,
		},
		{
			name:     "different content",
			a:        []string{"eth0", "eth1"},
			b:        []string{"eth0", "eth2"},
			expected: false,
		},
		{
			name:     "duplicates same",
			a:        []string{"eth0", "eth0"},
			b:        []string{"eth0", "eth0"},
			expected: true,
		},
		{
			name:     "duplicates different",
			a:        []string{"eth0", "eth0"},
			b:        []string{"eth0", "eth1"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringSlicesEqualUnordered(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("stringSlicesEqualUnordered(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestValidBondModes(t *testing.T) {
	validModes := []string{
		"balance-rr", "active-backup", "balance-xor", "broadcast",
		"802.3ad", "balance-tlb", "balance-alb",
		"0", "1", "2", "3", "4", "5", "6",
	}

	for _, mode := range validModes {
		if _, ok := validBondModes[mode]; !ok {
			t.Errorf("expected mode '%s' to be valid", mode)
		}
	}

	invalidModes := []string{"invalid", "lacp", "7", "-1"}
	for _, mode := range invalidModes {
		if _, ok := validBondModes[mode]; ok {
			t.Errorf("expected mode '%s' to be invalid", mode)
		}
	}
}
