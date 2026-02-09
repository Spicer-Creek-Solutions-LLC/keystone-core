// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package statemgmt

import (
	"context"
	"fmt"
	"time"
)

// WinServiceModule implements Windows service management
// This is a stub for non-Windows platforms
type WinServiceModule struct {
	*BaseModule
}

// NewWinServiceModule creates a new Windows service module
func NewWinServiceModule() *WinServiceModule {
	return &WinServiceModule{
		BaseModule: NewBaseModule("win_service", []string{
			"running", "stopped", "enabled", "disabled",
			"present", "absent",
		}),
	}
}

// Check checks the current state of a Windows service
// On non-Windows platforms, this returns an error
func (m *WinServiceModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	return nil, fmt.Errorf("win_service module is only available on Windows")
}

// Apply applies the Windows service state
// On non-Windows platforms, this returns an error
func (m *WinServiceModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	return &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Error:     fmt.Errorf("win_service module is only available on Windows"),
		Comment:   "win_service module is only available on Windows",
		StartTime: startTime,
		EndTime:   time.Now(),
		Duration:  time.Since(startTime),
	}, nil
}

// Test tests if the service is in the desired state
// On non-Windows platforms, this returns an error
func (m *WinServiceModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	return false, fmt.Errorf("win_service module is only available on Windows")
}

func init() {
	_ = RegisterModule(NewWinServiceModule()) //nolint:errcheck // module registration in init
}
