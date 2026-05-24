// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package at

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) ListJobs(context.Context) ([]string, error) { return nil, nil }
func (*otherProvider) JobScript(context.Context, string) (string, error) {
	return "", wrapUnsupported("JobScript")
}
func (*otherProvider) Submit(context.Context, string, string, string) error {
	return wrapUnsupported("Submit")
}
func (*otherProvider) Remove(context.Context, string) error { return wrapUnsupported("Remove") }

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "at." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
