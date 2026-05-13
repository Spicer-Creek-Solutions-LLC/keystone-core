//go:build !linux

package vlan

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) GetLink(context.Context, string) (*LinkInfo, error) {
	return nil, wrapUnsupported("GetLink")
}
func (*otherProvider) CreateVLAN(context.Context, VLANSpec) error {
	return wrapUnsupported("CreateVLAN")
}
func (*otherProvider) DeleteLink(context.Context, string) error {
	return wrapUnsupported("DeleteLink")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct{ op, os string }

func (e *unsupportedError) Error() string {
	return "vlan." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
