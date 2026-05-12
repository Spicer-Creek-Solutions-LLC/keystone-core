//go:build !linux

package cron

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) Read(context.Context, string) (string, error) { return "", nil }
func (*otherProvider) Write(context.Context, string, string) error  { return wrapUnsupported("Write") }

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "cron." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
