// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package kmod

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) Loaded(string) (bool, error)            { return false, nil }
func (*otherProvider) PersistExists(string) (bool, error)     { return false, nil }
func (*otherProvider) Load(context.Context, string) error     { return wrapUnsupported("Load") }
func (*otherProvider) Unload(context.Context, string) error   { return wrapUnsupported("Unload") }
func (*otherProvider) AddPersist(string) error                { return wrapUnsupported("AddPersist") }
func (*otherProvider) RemovePersist(string) error             { return wrapUnsupported("RemovePersist") }

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "kernel_module." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
