// Package statemgmt provides state management modules.
package statemgmt

import (
	"testing"

	"github.com/shawnbutts/keystone-core/internal/platform"
)

// =============================================================================
// ServiceModule Tests
// =============================================================================

func TestNewServiceModule(t *testing.T) {
	m := NewServiceModule()
	if m == nil {
		t.Fatal("NewServiceModule returned nil")
	}
	if m.Name() != "service" {
		t.Errorf("expected name 'service', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 5 {
		t.Errorf("expected 5 states (running, stopped, enabled, disabled, dead), got %d", len(states))
	}
	expectedStates := map[string]bool{
		"running":  true,
		"stopped":  true,
		"enabled":  true,
		"disabled": true,
		"dead":     true,
	}
	for _, s := range states {
		if !expectedStates[s] {
			t.Errorf("unexpected state: %s", s)
		}
	}
}

func TestConvertPlatformInitSystem(t *testing.T) {
	tests := []struct {
		input    platform.InitSystem
		expected ServiceManager
	}{
		{platform.InitSystemd, SMSystemd},
		{platform.InitUpstart, SMUpstart},
		{platform.InitSysV, SMInitD},
		{platform.InitOpenRC, SMOpenRC},
		{platform.InitLaunchd, SMLaunchd},
		{platform.InitWindowsService, SMWindowsService},
		{platform.InitUnknown, SMUnknown},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := convertPlatformInitSystem(tt.input)
			if result != tt.expected {
				t.Errorf("convertPlatformInitSystem(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestServiceManager_String(t *testing.T) {
	// Test that ServiceManager constants have expected string values
	tests := []struct {
		sm       ServiceManager
		expected string
	}{
		{SMSystemd, "systemd"},
		{SMInitD, "init.d"},
		{SMLaunchd, "launchd"},
		{SMOpenRC, "openrc"},
		{SMUpstart, "upstart"},
		{SMWindowsService, "windows_service"},
		{SMUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.sm) != tt.expected {
				t.Errorf("ServiceManager string = %s, want %s", string(tt.sm), tt.expected)
			}
		})
	}
}
