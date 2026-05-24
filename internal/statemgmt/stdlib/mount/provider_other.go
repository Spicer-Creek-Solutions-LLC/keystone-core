// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package mount

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) Lookup(_ context.Context, mountPoint string) (*MountInfo, error) {
	return &MountInfo{MountPoint: mountPoint, Mounted: false}, nil
}
func (*otherProvider) Mount(context.Context, string, string, string, string) error {
	return wrapUnsupported("Mount")
}
func (*otherProvider) Unmount(context.Context, string) error { return wrapUnsupported("Unmount") }

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "mount." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
