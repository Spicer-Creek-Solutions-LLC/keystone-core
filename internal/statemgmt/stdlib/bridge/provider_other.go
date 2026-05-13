//go:build !linux

package bridge

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) GetLink(context.Context, string) (*LinkInfo, error) {
	return nil, wrapUnsupported("GetLink")
}
func (*otherProvider) CreateBridge(context.Context, BridgeSpec) error {
	return wrapUnsupported("CreateBridge")
}
func (*otherProvider) DeleteLink(context.Context, string) error {
	return wrapUnsupported("DeleteLink")
}
func (*otherProvider) SetMaster(context.Context, string, string) error {
	return wrapUnsupported("SetMaster")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct{ op, os string }

func (e *unsupportedError) Error() string {
	return "bridge." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
