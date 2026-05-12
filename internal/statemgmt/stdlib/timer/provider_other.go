//go:build !linux

package timer

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) ReadUnit(string) (string, bool, error) { return "", false, nil }
func (*otherProvider) WriteUnit(string, string) error        { return wrapUnsupported("WriteUnit") }
func (*otherProvider) RemoveUnit(string) error               { return wrapUnsupported("RemoveUnit") }
func (*otherProvider) DaemonReload(context.Context) error    { return wrapUnsupported("DaemonReload") }
func (*otherProvider) Status(context.Context, string) (*TimerStatus, error) {
	return nil, wrapUnsupported("Status")
}
func (*otherProvider) EnableNow(context.Context, string) error   { return wrapUnsupported("EnableNow") }
func (*otherProvider) DisableStop(context.Context, string) error { return wrapUnsupported("DisableStop") }

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "systemd_timer." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
