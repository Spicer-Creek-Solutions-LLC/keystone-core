//go:build !linux

package system

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) ReadBanner(context.Context, string) (string, error) {
	return "", wrapUnsupported("ReadBanner")
}
func (*otherProvider) WriteBanner(context.Context, string, string) error {
	return wrapUnsupported("WriteBanner")
}
func (*otherProvider) IsRebootNeeded(context.Context, string) (bool, error) {
	return false, wrapUnsupported("IsRebootNeeded")
}
func (*otherProvider) ScheduleReboot(context.Context, int) error {
	return wrapUnsupported("ScheduleReboot")
}
func (*otherProvider) ReadLocale(context.Context) (string, error) {
	return "", wrapUnsupported("ReadLocale")
}
func (*otherProvider) WriteLocale(context.Context, string) error {
	return wrapUnsupported("WriteLocale")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct{ op, os string }

func (e *unsupportedError) Error() string {
	return "system." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
