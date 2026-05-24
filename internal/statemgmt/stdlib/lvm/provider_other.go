// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package lvm

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) HasPV(context.Context, string) (bool, error) {
	return false, wrapUnsupported("HasPV")
}
func (*otherProvider) CreatePV(context.Context, string) error { return wrapUnsupported("CreatePV") }
func (*otherProvider) RemovePV(context.Context, string) error { return wrapUnsupported("RemovePV") }
func (*otherProvider) HasVG(context.Context, string) (bool, error) {
	return false, wrapUnsupported("HasVG")
}
func (*otherProvider) CreateVG(context.Context, string, []string) error {
	return wrapUnsupported("CreateVG")
}
func (*otherProvider) RemoveVG(context.Context, string) error { return wrapUnsupported("RemoveVG") }
func (*otherProvider) HasLV(context.Context, string, string) (bool, error) {
	return false, wrapUnsupported("HasLV")
}
func (*otherProvider) CreateLV(context.Context, string, string, string, string) error {
	return wrapUnsupported("CreateLV")
}
func (*otherProvider) RemoveLV(context.Context, string, string) error {
	return wrapUnsupported("RemoveLV")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct{ op, os string }

func (e *unsupportedError) Error() string {
	return "lvm." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
