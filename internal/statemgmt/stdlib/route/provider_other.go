// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package route

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) GetRoute(context.Context, RouteQuery) (*RouteEntry, error) {
	return nil, wrapUnsupported("GetRoute")
}
func (*otherProvider) ReplaceRoute(context.Context, RouteSpec) error {
	return wrapUnsupported("ReplaceRoute")
}
func (*otherProvider) DelRoute(context.Context, RouteQuery) error {
	return wrapUnsupported("DelRoute")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct{ op, os string }

func (e *unsupportedError) Error() string {
	return "route." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
