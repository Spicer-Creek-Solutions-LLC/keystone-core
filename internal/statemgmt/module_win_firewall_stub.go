// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package statemgmt

import (
	"context"
	"fmt"
	"time"
)

// WinFirewallModule implements Windows Firewall rule management
// This is a stub for non-Windows platforms
type WinFirewallModule struct {
	*BaseModule
}

// NewWinFirewallModule creates a new Windows firewall module
func NewWinFirewallModule() *WinFirewallModule {
	return &WinFirewallModule{
		BaseModule: NewBaseModule("win_firewall", []string{
			"present", "absent", "enabled", "disabled",
		}),
	}
}

// Check checks the current state of a Windows Firewall rule
// On non-Windows platforms, this returns an error
func (m *WinFirewallModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	return nil, fmt.Errorf("win_firewall module is only available on Windows")
}

// Apply applies the Windows Firewall rule state
// On non-Windows platforms, this returns an error
func (m *WinFirewallModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	return &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Error:     fmt.Errorf("win_firewall module is only available on Windows"),
		Comment:   "win_firewall module is only available on Windows",
		StartTime: startTime,
		EndTime:   time.Now(),
		Duration:  time.Since(startTime),
	}, nil
}

// Test tests if the firewall rule is in the desired state
// On non-Windows platforms, this returns an error
func (m *WinFirewallModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	return false, fmt.Errorf("win_firewall module is only available on Windows")
}

func init() {
	_ = RegisterModule(NewWinFirewallModule()) //nolint:errcheck // module registration in init
}
