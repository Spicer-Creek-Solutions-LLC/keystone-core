// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package agent

import "errors"

// ErrNotWindows is returned when Windows service functions are called on non-Windows
var ErrNotWindows = errors.New("windows service functions are only available on Windows")

// RunAsService is not available on non-Windows platforms
func RunAsService(agent *Agent) error {
	return ErrNotWindows
}

// IsWindowsService returns false on non-Windows platforms
func IsWindowsService() bool {
	return false
}
