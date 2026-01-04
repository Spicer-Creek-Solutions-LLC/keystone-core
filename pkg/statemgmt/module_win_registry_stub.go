// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package statemgmt

import (
	"context"
	"fmt"
	"time"
)

// WinRegistryModule implements Windows registry management
// This is a stub for non-Windows platforms
type WinRegistryModule struct {
	*BaseModule
}

// NewWinRegistryModule creates a new Windows registry module
func NewWinRegistryModule() *WinRegistryModule {
	return &WinRegistryModule{
		BaseModule: NewBaseModule("win_registry", []string{
			"present", "absent",
		}),
	}
}

// Check checks the current state of a Windows registry key/value
// On non-Windows platforms, this returns an error
func (m *WinRegistryModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	return nil, fmt.Errorf("win_registry module is only available on Windows")
}

// Apply applies the Windows registry state
// On non-Windows platforms, this returns an error
func (m *WinRegistryModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	return &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Error:     fmt.Errorf("win_registry module is only available on Windows"),
		Comment:   "win_registry module is only available on Windows",
		StartTime: startTime,
		EndTime:   time.Now(),
		Duration:  time.Since(startTime),
	}, nil
}

// Test tests if the registry key/value is in the desired state
// On non-Windows platforms, this returns an error
func (m *WinRegistryModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	return false, fmt.Errorf("win_registry module is only available on Windows")
}

func init() {
	RegisterModule(NewWinRegistryModule())
}
