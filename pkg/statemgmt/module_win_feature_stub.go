// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package statemgmt

import (
	"context"
	"fmt"
	"time"
)

// WinFeatureModule implements Windows Features/Roles management
// This is a stub for non-Windows platforms
type WinFeatureModule struct {
	*BaseModule
}

// NewWinFeatureModule creates a new Windows feature module
func NewWinFeatureModule() *WinFeatureModule {
	return &WinFeatureModule{
		BaseModule: NewBaseModule("win_feature", []string{
			"installed", "removed", "enabled", "disabled",
		}),
	}
}

// Check checks the current state of a Windows Feature
// On non-Windows platforms, this returns an error
func (m *WinFeatureModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	return nil, fmt.Errorf("win_feature module is only available on Windows")
}

// Apply applies the Windows Feature state
// On non-Windows platforms, this returns an error
func (m *WinFeatureModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	return &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Error:     fmt.Errorf("win_feature module is only available on Windows"),
		Comment:   "win_feature module is only available on Windows",
		StartTime: startTime,
		EndTime:   time.Now(),
		Duration:  time.Since(startTime),
	}, nil
}

// Test tests if the feature is in the desired state
// On non-Windows platforms, this returns an error
func (m *WinFeatureModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	return false, fmt.Errorf("win_feature module is only available on Windows")
}

func init() {
	RegisterModule(NewWinFeatureModule())
}
