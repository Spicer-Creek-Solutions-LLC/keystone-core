// Package statemgmt provides state management modules.
package statemgmt

import (
	"testing"

	"github.com/shawnbutts/keystone-core/internal/platform"
)

// =============================================================================
// PackageModule Tests
// =============================================================================

func TestNewPackageModule(t *testing.T) {
	m := NewPackageModule()
	if m == nil {
		t.Fatal("NewPackageModule returned nil")
	}
	if m.Name() != "package" {
		t.Errorf("expected name 'package', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 4 {
		t.Errorf("expected 4 states (installed, removed, latest, purged), got %d", len(states))
	}
	expectedStates := map[string]bool{
		"installed": true,
		"removed":   true,
		"latest":    true,
		"purged":    true,
	}
	for _, s := range states {
		if !expectedStates[s] {
			t.Errorf("unexpected state: %s", s)
		}
	}
}

func TestConvertPlatformPM(t *testing.T) {
	tests := []struct {
		input    platform.PackageManager
		expected PackageManager
	}{
		{platform.PackageManagerAPT, PMApt},
		{platform.PackageManagerYum, PMYum},
		{platform.PackageManagerDNF, PMDNF},
		{platform.PackageManagerZypper, PMZypper},
		{platform.PackageManagerPacman, PMPacman},
		{platform.PackageManagerAPK, PMApk},
		{platform.PackageManagerBrew, PMBrew},
		{platform.PackageManagerChocolatey, PMChoco},
		{platform.PackageManagerWinget, PMWinget},
		{platform.PackageManagerUnknown, PMUnknown},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := convertPlatformPM(tt.input)
			if result != tt.expected {
				t.Errorf("convertPlatformPM(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPackageManager_String(t *testing.T) {
	// Test that PackageManager constants have expected string values
	tests := []struct {
		pm       PackageManager
		expected string
	}{
		{PMApt, "apt"},
		{PMYum, "yum"},
		{PMDNF, "dnf"},
		{PMApk, "apk"},
		{PMBrew, "brew"},
		{PMPacman, "pacman"},
		{PMZypper, "zypper"},
		{PMChoco, "chocolatey"},
		{PMWinget, "winget"},
		{PMUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.pm) != tt.expected {
				t.Errorf("PackageManager string = %s, want %s", string(tt.pm), tt.expected)
			}
		})
	}
}
