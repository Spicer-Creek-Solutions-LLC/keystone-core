// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

func TestNewWinFeatureModule(t *testing.T) {
	m := NewWinFeatureModule()
	if m == nil {
		t.Fatal("NewWinFeatureModule returned nil")
	}
	if m.Name() != "win_feature" {
		t.Errorf("Name() = %s, want win_feature", m.Name())
	}
}

func TestWinFeatureModule_States(t *testing.T) {
	m := NewWinFeatureModule()

	states := m.ValidStates()
	expectedStates := []string{"installed", "removed", "enabled", "disabled"}

	for _, expected := range expectedStates {
		found := false
		for _, state := range states {
			if state == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected state %s not found in valid states", expected)
		}
	}
}

func TestWinFeatureModule_Check_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinFeatureModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestFeature",
		State: "installed",
		Parameters: map[string]interface{}{
			"name": "TestFeature",
		},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
}

func TestWinFeatureModule_Apply_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinFeatureModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestFeature",
		State: "installed",
		Parameters: map[string]interface{}{
			"name": "TestFeature",
		},
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

func TestWinFeatureModule_Test_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinFeatureModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestFeature",
		State: "installed",
	}

	_, err := m.Test(ctx, decl)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
}

// Windows-specific tests
func TestWinFeatureModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinFeatureModule()
	ctx := context.Background()

	// Test with a non-existent feature
	decl := &StateDeclaration{
		ID:    "NonExistentFeatureXYZ12345",
		State: "installed",
		Parameters: map[string]interface{}{
			"name": "NonExistentFeatureXYZ12345",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result == nil {
		t.Fatal("Check returned nil result")
	}

	// Non-existent feature should not be present
	if result.Present {
		t.Error("Expected feature to be absent")
	}
}

func TestWinFeatureModule_Check_Removed_NonExistent_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinFeatureModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:    "NonExistentFeatureXYZ12345",
		State: "removed",
		Parameters: map[string]interface{}{
			"name": "NonExistentFeatureXYZ12345",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	// Non-existent feature should match "removed" state
	if !result.Matches {
		t.Error("Expected Matches to be true for removed state on non-existent feature")
	}
}

func TestWinFeatureModule_Check_KnownOptionalFeature_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinFeatureModule()
	ctx := context.Background()

	// Test with a known optional feature (TelnetClient is commonly available)
	decl := &StateDeclaration{
		ID:    "TelnetClient",
		State: "installed",
		Parameters: map[string]interface{}{
			"name": "TelnetClient",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result == nil {
		t.Fatal("Check returned nil result")
	}

	t.Logf("Feature present: %v, state: %s", result.Present, result.CurrentState)
	if result.Present {
		t.Logf("Feature metadata: %+v", result.Metadata)
	}
}
