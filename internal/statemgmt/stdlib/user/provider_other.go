//go:build !linux

package user

import (
	"context"
	"runtime"
)

type otherProvider struct{ osLookup }

func defaultProvider() Provider { return otherProvider{} }

func (otherProvider) Add(_ context.Context, _ AddOptions) error {
	return wrapUnsupported("Add")
}
func (otherProvider) Mod(_ context.Context, _ ModOptions) error {
	return wrapUnsupported("Mod")
}
func (otherProvider) Del(_ context.Context, _ string, _ bool) error {
	return wrapUnsupported("Del")
}
func (otherProvider) SetGroups(_ context.Context, _ string, _ []string) error {
	return wrapUnsupported("SetGroups")
}

func wrapUnsupported(op string) error {
	return &unsupportedError{op: op, os: runtime.GOOS}
}

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "user." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}

func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
