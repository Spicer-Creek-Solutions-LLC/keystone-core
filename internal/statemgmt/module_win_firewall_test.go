// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

func TestNewWinFirewallModule(t *testing.T) {
	m := NewWinFirewallModule()
	if m == nil {
		t.Fatal("NewWinFirewallModule returned nil")
	}
	if m.Name() != "win_firewall" {
		t.Errorf("Name() = %s, want win_firewall", m.Name())
	}
}

func TestWinFirewallModule_States(t *testing.T) {
	m := NewWinFirewallModule()

	states := m.ValidStates()
	expectedStates := []string{"present", "absent", "enabled", "disabled"}

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

func TestWinFirewallModule_Check_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinFirewallModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestRule",
		State: "present",
		Parameters: map[string]interface{}{
			"name":      "TestRule",
			"direction": "Inbound",
			"action":    "Allow",
		},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
}

func TestWinFirewallModule_Apply_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinFirewallModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestRule",
		State: "present",
		Parameters: map[string]interface{}{
			"name":      "TestRule",
			"direction": "Inbound",
			"action":    "Allow",
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

func TestWinFirewallModule_Test_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinFirewallModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestRule",
		State: "present",
	}

	_, err := m.Test(ctx, decl)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
}

// Windows-specific tests
func TestWinFirewallModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinFirewallModule()
	ctx := context.Background()

	// Test with a non-existent rule
	decl := &StateDeclaration{
		ID:    "NonExistentRuleXYZ12345",
		State: "present",
		Parameters: map[string]interface{}{
			"name": "NonExistentRuleXYZ12345",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result == nil {
		t.Fatal("Check returned nil result")
	}

	if result.Present {
		t.Error("Expected rule to be absent")
	}
	if result.CurrentState != "absent" {
		t.Errorf("Expected CurrentState to be 'absent', got %s", result.CurrentState)
	}
}

func TestWinFirewallModule_Check_Absent_NonExistent_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinFirewallModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:    "NonExistentRuleXYZ12345",
		State: "absent",
		Parameters: map[string]interface{}{
			"name": "NonExistentRuleXYZ12345",
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
		t.Error("Expected Matches to be true for absent state on non-existent rule")
	}
}

func TestWinFirewallModule_Check_KnownRule_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinFirewallModule()
	ctx := context.Background()

	// Test with a known Windows Firewall rule (Remote Desktop)
	decl := &StateDeclaration{
		ID:    "RemoteDesktop-UserMode-In-TCP",
		State: "present",
		Parameters: map[string]interface{}{
			"name": "RemoteDesktop-UserMode-In-TCP",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result == nil {
		t.Fatal("Check returned nil result")
	}

	// This rule may or may not exist depending on the Windows version
	t.Logf("Rule present: %v, state: %s", result.Present, result.CurrentState)
	if result.Present {
		t.Logf("Rule metadata: %+v", result.Metadata)
	}
}
