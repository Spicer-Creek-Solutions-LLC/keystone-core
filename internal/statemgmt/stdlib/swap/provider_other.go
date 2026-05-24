// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package swap

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) Lookup(_ context.Context, source string) (*SwapInfo, error) {
	return &SwapInfo{Source: source, Active: false}, nil
}
func (*otherProvider) MakeSwap(context.Context, string) error    { return wrapUnsupported("MakeSwap") }
func (*otherProvider) SwapOn(context.Context, string, int) error { return wrapUnsupported("SwapOn") }
func (*otherProvider) SwapOff(context.Context, string) error     { return wrapUnsupported("SwapOff") }
func (*otherProvider) CreateSwapfile(context.Context, string, int64) error {
	return wrapUnsupported("CreateSwapfile")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "swap." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
