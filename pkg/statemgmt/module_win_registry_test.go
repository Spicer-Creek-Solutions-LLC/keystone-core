// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

func TestNewWinRegistryModule(t *testing.T) {
	m := NewWinRegistryModule()
	if m == nil {
		t.Fatal("NewWinRegistryModule returned nil")
	}
	if m.Name() != "win_registry" {
		t.Errorf("Name() = %s, want win_registry", m.Name())
	}
}

func TestWinRegistryModule_States(t *testing.T) {
	m := NewWinRegistryModule()

	states := m.ValidStates()
	expectedStates := []string{"present", "absent"}

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

func TestWinRegistryModule_Check_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinRegistryModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "HKLM\\SOFTWARE\\Test",
		State: "present",
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
}

func TestWinRegistryModule_Apply_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinRegistryModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "HKLM\\SOFTWARE\\Test",
		State: "present",
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

func TestWinRegistryModule_Test_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinRegistryModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "HKLM\\SOFTWARE\\Test",
		State: "present",
	}

	_, err := m.Test(ctx, decl)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
}

// Windows-specific tests
func TestWinRegistryModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinRegistryModule()
	ctx := context.Background()

	// Test with a known Windows registry key
	decl := &StateDeclaration{
		ID:    "HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion",
		State: "present",
		Parameters: map[string]interface{}{
			"key": "HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result == nil {
		t.Fatal("Check returned nil result")
	}

	if !result.Present {
		t.Error("Expected key to be present")
	}

	t.Logf("Key state: %s, present: %v, matches: %v",
		result.CurrentState,
		result.Present,
		result.Matches)
}

func TestWinRegistryModule_Check_Value_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinRegistryModule()
	ctx := context.Background()

	// Test with a known Windows registry value
	decl := &StateDeclaration{
		ID:    "CurrentVersion",
		State: "present",
		Parameters: map[string]interface{}{
			"key":  "HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
			"name": "CurrentBuild",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result == nil {
		t.Fatal("Check returned nil result")
	}

	if !result.Present {
		t.Log("Key not present (may be expected in some environments)")
	}

	if result.Metadata["value_exists"] == true {
		t.Logf("Value type: %v, data: %v",
			result.Metadata["value_type"],
			result.Metadata["value_data"])
	}
}

func TestWinRegistryModule_Check_NonExistent_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinRegistryModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:    "NonExistentKeyXYZ12345",
		State: "present",
		Parameters: map[string]interface{}{
			"key": "HKLM\\SOFTWARE\\NonExistentKeyXYZ12345",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("Expected Present to be false for non-existent key")
	}
	if result.CurrentState != "absent" {
		t.Errorf("Expected CurrentState to be 'absent', got %s", result.CurrentState)
	}
	if result.Matches {
		t.Error("Expected Matches to be false for non-existent key with present state")
	}
}

func TestWinRegistryModule_Check_Absent_NonExistent_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinRegistryModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:    "NonExistentKeyXYZ12345",
		State: "absent",
		Parameters: map[string]interface{}{
			"key": "HKLM\\SOFTWARE\\NonExistentKeyXYZ12345",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("Expected Present to be false")
	}
	if !result.Matches {
		t.Error("Expected Matches to be true for absent state on non-existent key")
	}
}
