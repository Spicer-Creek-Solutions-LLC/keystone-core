//go:build !linux

package service

import (
	"context"
	"runtime"
)

type otherProvider struct{}

// The linux variant of defaultProvider takes a systemdRunDir to
// satisfy the test-mutable-without-global requirement; the non-
// linux variant ignores it.
func defaultProvider(_ string) Provider { return &otherProvider{} }

// defaultSystemdRunDir is unused on non-linux builds but kept for
// API parity so callers don't need build tags.
const defaultSystemdRunDir = "/run/systemd/system"

func (*otherProvider) Lookup(name string) (*ServiceInfo, error) {
	return &ServiceInfo{Name: name, Exists: false}, nil
}
func (*otherProvider) Start(_ context.Context, _ string) error   { return wrapUnsupported("Start") }
func (*otherProvider) Stop(_ context.Context, _ string) error    { return wrapUnsupported("Stop") }
func (*otherProvider) Enable(_ context.Context, _ string) error  { return wrapUnsupported("Enable") }
func (*otherProvider) Disable(_ context.Context, _ string) error { return wrapUnsupported("Disable") }

func wrapUnsupported(op string) error {
	return &unsupportedError{op: op, os: runtime.GOOS}
}

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "service." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}

func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
