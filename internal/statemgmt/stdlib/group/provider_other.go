// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package group

import (
	"context"
	"runtime"
)

// otherProvider returns ErrUnsupportedOS from every mutating
// method. Lookup is inherited from osLookup and works fine on
// macOS / BSD / Windows.
type otherProvider struct{ osLookup }

func defaultProvider() Provider { return otherProvider{} }

func (otherProvider) Add(_ context.Context, _ string, _ *int, _ bool) error {
	return wrapUnsupported("Add")
}
func (otherProvider) Mod(_ context.Context, _ string, _ int) error {
	return wrapUnsupported("Mod")
}
func (otherProvider) Del(_ context.Context, _ string) error {
	return wrapUnsupported("Del")
}

func wrapUnsupported(op string) error {
	// Wraps the package sentinel so callers can errors.Is for the
	// general "unsupported" condition, while still surfacing the
	// op + OS that produced it.
	return &unsupportedError{op: op, os: runtime.GOOS}
}

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "group." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}

func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
