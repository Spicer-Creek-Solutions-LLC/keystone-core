//go:build !linux

package pkg

import (
	"context"
	"runtime"
)

// otherProvider returns ErrUnsupportedOS from every mutating
// method on non-Linux. Lookup mirrors undetectedProvider's
// "everything not installed" stance so state=absent decls don't
// produce spurious drift events on a developer macOS machine.
type otherProvider struct{}

func defaultProvider() Provider { return &otherProvider{} }

func (*otherProvider) Lookup(name string) (*PkgInfo, error) {
	return &PkgInfo{Name: name, Installed: false}, nil
}

func (*otherProvider) Install(_ context.Context, _, _ string) error {
	return wrapUnsupported("Install")
}

func (*otherProvider) Remove(_ context.Context, _ string) error {
	return wrapUnsupported("Remove")
}

func wrapUnsupported(op string) error {
	return &unsupportedError{op: op, os: runtime.GOOS}
}

type unsupportedError struct {
	op string
	os string
}

func (e *unsupportedError) Error() string {
	return "pkg." + e.op + ": " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}

func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
