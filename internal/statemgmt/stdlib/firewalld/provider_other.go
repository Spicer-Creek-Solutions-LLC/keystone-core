// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package firewalld

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) Has(context.Context, string, Item) (bool, error) {
	return false, wrapUnsupported("Has")
}
func (*otherProvider) Add(context.Context, string, Item) error {
	return wrapUnsupported("Add")
}
func (*otherProvider) Remove(context.Context, string, Item) error {
	return wrapUnsupported("Remove")
}
func (*otherProvider) Reload(context.Context) error { return wrapUnsupported("Reload") }

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "firewalld." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
