// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package statemgmt

import (
	"context"
	"fmt"
	"time"
)

// WinPackageModule implements Windows package management
// This is a stub for non-Windows platforms
type WinPackageModule struct {
	*BaseModule
}

// NewWinPackageModule creates a new Windows package module
func NewWinPackageModule() *WinPackageModule {
	return &WinPackageModule{
		BaseModule: NewBaseModule("win_package", []string{
			"installed", "removed", "latest",
		}),
	}
}

// Check checks the current state of a Windows package
// On non-Windows platforms, this returns an error
func (m *WinPackageModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	return nil, fmt.Errorf("win_package module is only available on Windows")
}

// Apply applies the Windows package state
// On non-Windows platforms, this returns an error
func (m *WinPackageModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	return &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Error:     fmt.Errorf("win_package module is only available on Windows"),
		Comment:   "win_package module is only available on Windows",
		StartTime: startTime,
		EndTime:   time.Now(),
		Duration:  time.Since(startTime),
	}, nil
}

// Test tests if the package is in the desired state
// On non-Windows platforms, this returns an error
func (m *WinPackageModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	return false, fmt.Errorf("win_package module is only available on Windows")
}

func init() {
	_ = RegisterModule(NewWinPackageModule()) //nolint:errcheck // module registration in init
}
