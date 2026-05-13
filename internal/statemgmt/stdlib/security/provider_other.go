//go:build !linux

package security

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) GetPersistentMode(context.Context) (string, error) {
	return "", wrapUnsupported("GetPersistentMode")
}
func (*otherProvider) GetRuntimeMode(context.Context) (string, error) {
	return "", wrapUnsupported("GetRuntimeMode")
}
func (*otherProvider) SetPersistentMode(context.Context, string) error {
	return wrapUnsupported("SetPersistentMode")
}
func (*otherProvider) SetRuntimeMode(context.Context, string) error {
	return wrapUnsupported("SetRuntimeMode")
}
func (*otherProvider) GetBoolean(context.Context, string) (bool, error) {
	return false, wrapUnsupported("GetBoolean")
}
func (*otherProvider) SetBoolean(context.Context, string, bool) error {
	return wrapUnsupported("SetBoolean")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct{ op, os string }

func (e *unsupportedError) Error() string {
	return "security." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
