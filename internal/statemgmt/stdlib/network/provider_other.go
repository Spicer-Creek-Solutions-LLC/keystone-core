// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package network

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) GetInterface(context.Context, string) (*InterfaceState, error) {
	return nil, wrapUnsupported("GetInterface")
}
func (*otherProvider) AddAddress(context.Context, string, string) error {
	return wrapUnsupported("AddAddress")
}
func (*otherProvider) DelAddress(context.Context, string, string) error {
	return wrapUnsupported("DelAddress")
}
func (*otherProvider) SetMTU(context.Context, string, int) error {
	return wrapUnsupported("SetMTU")
}
func (*otherProvider) SetLinkUp(context.Context, string, bool) error {
	return wrapUnsupported("SetLinkUp")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct{ op, os string }

func (e *unsupportedError) Error() string {
	return "network." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
