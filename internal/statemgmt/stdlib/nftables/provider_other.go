//go:build !linux

package nftables

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) ListRuleHandles(context.Context, string, string, string) ([]RuleHandle, error) {
	return nil, wrapUnsupported("ListRuleHandles")
}
func (*otherProvider) AddRule(context.Context, string, string, string, int, []string) error {
	return wrapUnsupported("AddRule")
}
func (*otherProvider) DeleteRule(context.Context, string, string, string, int) error {
	return wrapUnsupported("DeleteRule")
}
func (*otherProvider) SaveRuleset(context.Context, string) error {
	return wrapUnsupported("SaveRuleset")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "nftables." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
