// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package langpkg

import (
	"context"
	"runtime"
)

type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) HasPipPackage(context.Context, string) (bool, string, error) {
	return false, "", wrapUnsupported("HasPipPackage")
}
func (*otherProvider) InstallPipPackage(context.Context, string, string) error {
	return wrapUnsupported("InstallPipPackage")
}
func (*otherProvider) UninstallPipPackage(context.Context, string) error {
	return wrapUnsupported("UninstallPipPackage")
}
func (*otherProvider) HasNpmPackage(context.Context, string) (bool, string, error) {
	return false, "", wrapUnsupported("HasNpmPackage")
}
func (*otherProvider) InstallNpmPackage(context.Context, string, string) error {
	return wrapUnsupported("InstallNpmPackage")
}
func (*otherProvider) UninstallNpmPackage(context.Context, string) error {
	return wrapUnsupported("UninstallNpmPackage")
}
func (*otherProvider) HasGemPackage(context.Context, string) (bool, string, error) {
	return false, "", wrapUnsupported("HasGemPackage")
}
func (*otherProvider) InstallGemPackage(context.Context, string, string) error {
	return wrapUnsupported("InstallGemPackage")
}
func (*otherProvider) UninstallGemPackage(context.Context, string) error {
	return wrapUnsupported("UninstallGemPackage")
}

func wrapUnsupported(op string) error { return &unsupportedError{op: op, os: runtime.GOOS} }

type unsupportedError struct{ op, os string }

func (e *unsupportedError) Error() string {
	return "langpkg." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
