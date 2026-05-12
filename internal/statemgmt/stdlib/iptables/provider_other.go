//go:build !linux

package iptables

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) HasRule(context.Context, string, string, string, []string) (bool, error) {
	return false, wrapUnsupported("HasRule")
}
func (*otherProvider) AddRule(context.Context, string, string, string, int, []string) error {
	return wrapUnsupported("AddRule")
}
func (*otherProvider) DeleteRule(context.Context, string, string, string, []string) error {
	return wrapUnsupported("DeleteRule")
}
func (*otherProvider) Save(context.Context, string, string) error { return wrapUnsupported("Save") }

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "iptables." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
