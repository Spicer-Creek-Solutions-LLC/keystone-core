package statemgmt

import (
	"context"
	"testing"
)

func TestNewRouteModule(t *testing.T) {
	m := NewRouteModule()
	if m == nil {
		t.Fatal("NewRouteModule returned nil")
	}
	if m.Name() != "route" {
		t.Errorf("expected name 'route', got '%s'", m.Name())
	}

	states := m.ValidStates()
	stateMap := make(map[string]bool)
	for _, s := range states {
		stateMap[s] = true
	}

	if !stateMap["present"] {
		t.Error("module should support 'present' state")
	}
	if !stateMap["absent"] {
		t.Error("module should support 'absent' state")
	}
}

func TestRouteModule_ParseConfig(t *testing.T) {
	m := NewRouteModule()

	tests := []struct {
		name        string
		decl        *StateDeclaration
		wantDest    string
		wantGW      string
		wantIface   string
		wantMetric  int
		wantTable   string
		wantErr     bool
	}{
		{
			name: "basic route via gateway",
			decl: &StateDeclaration{
				ID:     "10.0.0.0/8",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"gateway": "192.168.1.1",
				},
			},
			wantDest: "10.0.0.0/8",
			wantGW:   "192.168.1.1",
		},
		{
			name: "route via interface",
			decl: &StateDeclaration{
				ID:     "172.16.0.0/12",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"interface": "eth0",
				},
			},
			wantDest:  "172.16.0.0/12",
			wantIface: "eth0",
		},
		{
			name: "route with metric",
			decl: &StateDeclaration{
				ID:     "10.10.0.0/16",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"gateway": "10.0.0.1",
					"metric":  100,
				},
			},
			wantDest:   "10.10.0.0/16",
			wantGW:     "10.0.0.1",
			wantMetric: 100,
		},
		{
			name: "route with table (Linux)",
			decl: &StateDeclaration{
				ID:     "192.168.100.0/24",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"gateway": "192.168.1.1",
					"table":   "custom",
				},
			},
			wantDest:  "192.168.100.0/24",
			wantGW:    "192.168.1.1",
			wantTable: "custom",
		},
		{
			name: "default route",
			decl: &StateDeclaration{
				ID:     "default",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"gateway": "192.168.1.1",
				},
			},
			wantDest: "default",
			wantGW:   "192.168.1.1",
		},
		{
			name: "explicit destination parameter",
			decl: &StateDeclaration{
				ID:     "route-to-office",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"destination": "10.20.0.0/16",
					"gateway":     "192.168.1.1",
				},
			},
			wantDest: "10.20.0.0/16",
			wantGW:   "192.168.1.1",
		},
		{
			name: "missing gateway and interface",
			decl: &StateDeclaration{
				ID:         "10.0.0.0/8",
				State:      "present",
				Module:     "route",
				Parameters: map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "invalid destination",
			decl: &StateDeclaration{
				ID:     "not-a-network",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"gateway": "192.168.1.1",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseRouteConfig(tt.decl)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRouteConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if config.Destination != tt.wantDest {
				t.Errorf("Destination = %s, want %s", config.Destination, tt.wantDest)
			}
			if config.Gateway != tt.wantGW {
				t.Errorf("Gateway = %s, want %s", config.Gateway, tt.wantGW)
			}
			if config.Interface != tt.wantIface {
				t.Errorf("Interface = %s, want %s", config.Interface, tt.wantIface)
			}
			if config.Metric != tt.wantMetric {
				t.Errorf("Metric = %d, want %d", config.Metric, tt.wantMetric)
			}
			if config.Table != tt.wantTable {
				t.Errorf("Table = %s, want %s", config.Table, tt.wantTable)
			}
		})
	}
}

func TestRouteModule_Check_NonexistentRoute(t *testing.T) {
	m := NewRouteModule()
	ctx := context.Background()

	// Use an unlikely destination that won't exist
	decl := &StateDeclaration{
		ID:     "203.0.113.0/24", // TEST-NET-3 per RFC 5737
		State:  "present",
		Module: "route",
		Parameters: map[string]interface{}{
			"gateway": "192.168.1.1",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	// Route should not exist
	if result.Present {
		t.Error("expected Present=false for nonexistent route")
	}
	if result.CurrentState != "absent" {
		t.Errorf("expected CurrentState='absent', got '%s'", result.CurrentState)
	}
	if result.Matches {
		t.Error("expected Matches=false for nonexistent route when state is 'present'")
	}
}

func TestRouteModule_Check_AbsentState(t *testing.T) {
	m := NewRouteModule()
	ctx := context.Background()

	// Check that a nonexistent route matches 'absent' state
	decl := &StateDeclaration{
		ID:     "203.0.113.0/24",
		State:  "absent",
		Module: "route",
		Parameters: map[string]interface{}{
			"gateway": "192.168.1.1",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if result.Present {
		t.Error("expected Present=false for nonexistent route")
	}
	if !result.Matches {
		t.Error("expected Matches=true when route doesn't exist and state is 'absent'")
	}
}

func TestRouteModule_Test(t *testing.T) {
	m := NewRouteModule()
	ctx := context.Background()

	// Test with nonexistent route wanting absent state
	decl := &StateDeclaration{
		ID:     "203.0.113.0/24",
		State:  "absent",
		Module: "route",
		Parameters: map[string]interface{}{
			"gateway": "192.168.1.1",
		},
	}

	matches, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}

	if !matches {
		t.Error("expected Test() to return true for absent route with 'absent' state")
	}
}

func TestRouteModule_ParseConfig_HostRoute(t *testing.T) {
	m := NewRouteModule()

	// A host route (single IP) should be valid
	decl := &StateDeclaration{
		ID:     "192.168.100.10",
		State:  "present",
		Module: "route",
		Parameters: map[string]interface{}{
			"gateway": "192.168.1.1",
		},
	}

	config, err := m.parseRouteConfig(decl)
	if err != nil {
		t.Fatalf("parseRouteConfig() error = %v for host route", err)
	}

	if config.Destination != "192.168.100.10" {
		t.Errorf("Destination = %s, want 192.168.100.10", config.Destination)
	}
}

func TestRouteModule_ParseConfig_ZeroRoute(t *testing.T) {
	m := NewRouteModule()

	// 0.0.0.0/0 should be valid (same as default)
	decl := &StateDeclaration{
		ID:     "0.0.0.0/0",
		State:  "present",
		Module: "route",
		Parameters: map[string]interface{}{
			"gateway": "192.168.1.1",
		},
	}

	config, err := m.parseRouteConfig(decl)
	if err != nil {
		t.Fatalf("parseRouteConfig() error = %v for 0.0.0.0/0", err)
	}

	if config.Destination != "0.0.0.0/0" {
		t.Errorf("Destination = %s, want 0.0.0.0/0", config.Destination)
	}
}

func TestRouteModule_Apply_AlreadyAbsent(t *testing.T) {
	m := NewRouteModule()
	ctx := context.Background()

	// Apply absent state to a route that doesn't exist
	decl := &StateDeclaration{
		ID:     "203.0.113.0/24",
		State:  "absent",
		Module: "route",
		Parameters: map[string]interface{}{
			"gateway": "192.168.1.1",
		},
	}

	result, err := m.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Changed {
		t.Error("expected Changed=false for already absent route")
	}
	if result.Comment != "Already in desired state" {
		t.Errorf("unexpected Comment: %s", result.Comment)
	}
}

func TestRouteConfig_Validation(t *testing.T) {
	m := NewRouteModule()

	tests := []struct {
		name    string
		decl    *StateDeclaration
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with gateway",
			decl: &StateDeclaration{
				ID:     "10.0.0.0/8",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"gateway": "192.168.1.1",
				},
			},
			wantErr: false,
		},
		{
			name: "valid with interface",
			decl: &StateDeclaration{
				ID:     "10.0.0.0/8",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"interface": "eth0",
				},
			},
			wantErr: false,
		},
		{
			name: "valid with both",
			decl: &StateDeclaration{
				ID:     "10.0.0.0/8",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"gateway":   "192.168.1.1",
					"interface": "eth0",
				},
			},
			wantErr: false,
		},
		{
			name: "missing gateway and interface",
			decl: &StateDeclaration{
				ID:         "10.0.0.0/8",
				State:      "present",
				Module:     "route",
				Parameters: map[string]interface{}{},
			},
			wantErr: true,
			errMsg:  "must specify gateway or interface",
		},
		{
			name: "invalid CIDR",
			decl: &StateDeclaration{
				ID:     "invalid-cidr",
				State:  "present",
				Module: "route",
				Parameters: map[string]interface{}{
					"gateway": "192.168.1.1",
				},
			},
			wantErr: true,
			errMsg:  "invalid destination",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.parseRouteConfig(tt.decl)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRouteConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("error message = %v, want to contain %s", err, tt.errMsg)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
