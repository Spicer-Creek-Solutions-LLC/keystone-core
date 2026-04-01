// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

func TestNewWinServiceModule(t *testing.T) {
	m := NewWinServiceModule()
	if m == nil {
		t.Fatal("NewWinServiceModule returned nil")
	}
	if m.Name() != "win_service" {
		t.Errorf("Name() = %s, want win_service", m.Name())
	}
}

func TestWinServiceModule_States(t *testing.T) {
	m := NewWinServiceModule()

	states := m.ValidStates()
	expectedStates := []string{"running", "stopped", "enabled", "disabled", "present", "absent"}

	for _, expected := range expectedStates {
		found := false
		for _, state := range states {
			if state == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected state %s not found in supported states", expected)
		}
	}
}

func TestWinServiceModule_Check_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinServiceModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestService",
		State: "running",
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
}

func TestWinServiceModule_Apply_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinServiceModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestService",
		State: "running",
	}

	result, err := m.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if result.Success {
		t.Error("Expected Success to be false on non-Windows")
	}
	if result.Error == nil {
		t.Error("Expected Error to be set on non-Windows")
	}
}

func TestWinServiceModule_Test_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinServiceModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestService",
		State: "running",
	}

	_, err := m.Test(ctx, decl)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
}

// Windows-specific tests
func TestWinServiceModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinServiceModule()
	ctx := context.Background()

	// Test with a known Windows service (Spooler is commonly available)
	decl := &StateDeclaration{
		ID:    "Spooler",
		State: "running",
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result == nil {
		t.Fatal("Check returned nil result")
	}

	if !result.Present {
		t.Log("Spooler service not present (may be expected in some environments)")
	}

	// Check metadata
	if result.Metadata["running"] == nil {
		t.Error("Expected 'running' in metadata")
	}
	if result.Metadata["enabled"] == nil {
		t.Error("Expected 'enabled' in metadata")
	}
	if result.Metadata["start_type"] == nil {
		t.Error("Expected 'start_type' in metadata")
	}

	t.Logf("Service state: %s, running: %v, enabled: %v",
		result.CurrentState,
		result.Metadata["running"],
		result.Metadata["enabled"])
}

func TestWinServiceModule_Check_NonExistent_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinServiceModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:    "NonExistentServiceXYZ12345",
		State: "running",
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("Expected Present to be false for non-existent service")
	}
	if result.CurrentState != "absent" {
		t.Errorf("Expected CurrentState to be 'absent', got %s", result.CurrentState)
	}
	if result.Matches {
		t.Error("Expected Matches to be false for non-existent service with running state")
	}
}

func TestWinServiceModule_Check_Absent_NonExistent_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinServiceModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:    "NonExistentServiceXYZ12345",
		State: "absent",
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("Expected Present to be false")
	}
	if !result.Matches {
		t.Error("Expected Matches to be true for absent state on non-existent service")
	}
}

func TestWinServiceModule_Apply_DryRun_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinServiceModule()
	ctx := context.Background()

	// Test with a service that doesn't exist (should fail gracefully)
	decl := &StateDeclaration{
		ID:    "NonExistentServiceXYZ12345",
		State: "absent",
	}

	result, err := m.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	// Should succeed because service is already absent
	if !result.Success {
		t.Errorf("Expected success for absent state on non-existent service: %v", result.Error)
	}
}

// Test helper functions (these work on all platforms)
func TestWinServiceModule_Helpers(t *testing.T) {
	m := NewWinServiceModule()
	_ = m // Use module to verify it's constructable

	// Test parameter helpers work with nil Parameters
	decl := &StateDeclaration{
		ID:    "test",
		State: "running",
	}

	// These should not panic with nil Parameters
	if hasParameter(decl, "test") {
		t.Error("hasParameter should return false for nil Parameters")
	}
	if getIntParameter(decl, "test", 10) != 10 {
		t.Error("getIntParameter should return default for nil Parameters")
	}
	if getStringSliceParameter(decl, "test") != nil {
		t.Error("getStringSliceParameter should return nil for nil Parameters")
	}
}

func TestWinServiceModule_Helpers_WithParams(t *testing.T) {
	decl := &StateDeclaration{
		ID:    "test",
		State: "running",
		Parameters: map[string]interface{}{
			"intval":    42,
			"int64val":  int64(100),
			"floatval":  123.45,
			"stringval": "test",
			"sliceval":  []string{"a", "b", "c"},
			"ifaceval":  []interface{}{"x", "y"},
		},
	}

	if !hasParameter(decl, "intval") {
		t.Error("hasParameter should return true for existing key")
	}
	if hasParameter(decl, "nonexistent") {
		t.Error("hasParameter should return false for non-existing key")
	}

	if getIntParameter(decl, "intval", 0) != 42 {
		t.Error("getIntParameter should return int value")
	}
	if getIntParameter(decl, "int64val", 0) != 100 {
		t.Error("getIntParameter should convert int64")
	}
	if getIntParameter(decl, "floatval", 0) != 123 {
		t.Error("getIntParameter should convert float64")
	}
	if getIntParameter(decl, "nonexistent", 99) != 99 {
		t.Error("getIntParameter should return default for non-existing")
	}

	slice := getStringSliceParameter(decl, "sliceval")
	if len(slice) != 3 || slice[0] != "a" {
		t.Error("getStringSliceParameter should return string slice")
	}

	iface := getStringSliceParameter(decl, "ifaceval")
	if len(iface) != 2 || iface[0] != "x" {
		t.Error("getStringSliceParameter should convert []interface{}")
	}
}
