// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package disk

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) GetFilesystem(context.Context, string) (string, error) {
	return "", wrapUnsupported("GetFilesystem")
}
func (*otherProvider) MakeFilesystem(context.Context, string, string, []string) error {
	return wrapUnsupported("MakeFilesystem")
}
func (*otherProvider) WipeFilesystem(context.Context, string) error {
	return wrapUnsupported("WipeFilesystem")
}
func (*otherProvider) FilesystemFillsDevice(context.Context, string, string) (bool, error) {
	return false, wrapUnsupported("FilesystemFillsDevice")
}
func (*otherProvider) ResizeFilesystem(context.Context, string, string) error {
	return wrapUnsupported("ResizeFilesystem")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct{ op, os string }

func (e *unsupportedError) Error() string {
	return "disk." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
