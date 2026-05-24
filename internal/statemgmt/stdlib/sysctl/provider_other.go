// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package sysctl

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) Get(string) (string, bool, error)          { return "", false, nil }
func (*otherProvider) ReadPersist(string) (string, bool, error)  { return "", false, nil }
func (*otherProvider) Set(context.Context, string, string) error { return wrapUnsupported("Set") }
func (*otherProvider) WritePersist(string, string) error         { return wrapUnsupported("WritePersist") }

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "sysctl." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
