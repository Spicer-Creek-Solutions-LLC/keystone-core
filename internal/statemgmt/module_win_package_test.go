// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

func TestNewWinPackageModule(t *testing.T) {
	m := NewWinPackageModule()
	if m == nil {
		t.Fatal("NewWinPackageModule returned nil")
	}
	if m.Name() != "win_package" {
		t.Errorf("Name() = %s, want win_package", m.Name())
	}
}

func TestWinPackageModule_States(t *testing.T) {
	m := NewWinPackageModule()

	states := m.ValidStates()
	expectedStates := []string{"installed", "removed", "latest"}

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

func TestWinPackageModule_Check_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinPackageModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestPackage",
		State: "installed",
		Parameters: map[string]interface{}{
			"name":   "TestPackage",
			"source": "chocolatey",
		},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
}

func TestWinPackageModule_Apply_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinPackageModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestPackage",
		State: "installed",
		Parameters: map[string]interface{}{
			"name":   "TestPackage",
			"source": "chocolatey",
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

func TestWinPackageModule_Test_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows")
	}

	m := NewWinPackageModule()
	ctx := context.Background()
	decl := &StateDeclaration{
		ID:    "TestPackage",
		State: "installed",
	}

	_, err := m.Test(ctx, decl)
	if err == nil {
		t.Error("Expected error on non-Windows platform")
	}
}

// Windows-specific tests
func TestWinPackageModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinPackageModule()
	ctx := context.Background()

	// Test with a non-existent package via Chocolatey
	decl := &StateDeclaration{
		ID:    "NonExistentPackageXYZ12345",
		State: "installed",
		Parameters: map[string]interface{}{
			"name":   "NonExistentPackageXYZ12345",
			"source": "chocolatey",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		// Chocolatey might not be installed
		t.Logf("Check returned error (Chocolatey may not be installed): %v", err)
		return
	}

	if result == nil {
		t.Fatal("Check returned nil result")
	}

	if result.Present {
		t.Error("Expected package to be absent")
	}
}

func TestWinPackageModule_Check_Removed_NonExistent_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinPackageModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:    "NonExistentPackageXYZ12345",
		State: "removed",
		Parameters: map[string]interface{}{
			"name":   "NonExistentPackageXYZ12345",
			"source": "chocolatey",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		// Chocolatey might not be installed
		t.Logf("Check returned error (Chocolatey may not be installed): %v", err)
		return
	}

	if result.Present {
		t.Error("Expected Present to be false")
	}
	if !result.Matches {
		t.Error("Expected Matches to be true for removed state on non-existent package")
	}
}

func TestWinPackageModule_Check_InstalledProgram_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	m := NewWinPackageModule()
	ctx := context.Background()

	// Test with a common Windows program that should be installed
	decl := &StateDeclaration{
		ID:    "Windows PowerShell",
		State: "installed",
		Parameters: map[string]interface{}{
			"name":   "Windows PowerShell",
			"source": "msi", // Will check registry for installed programs
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result == nil {
		t.Fatal("Check returned nil result")
	}

	t.Logf("Package present: %v, state: %s", result.Present, result.CurrentState)
	if result.Present {
		t.Logf("Package metadata: %+v", result.Metadata)
	}
}
