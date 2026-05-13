//go:build !linux

package firewall

import (
	"context"
	"runtime"
)

func defaultDetector() BackendDetector { return &otherDetector{} }

type otherDetector struct{}

func (*otherDetector) Detect(context.Context) (string, error) {
	return "", &unsupportedError{os: runtime.GOOS}
}

type unsupportedError struct{ os string }

func (e *unsupportedError) Error() string {
	return "firewall.Detect: " + ErrUnsupportedOS.Error() + " (got " + e.os + ")"
}
func (e *unsupportedError) Unwrap() error { return ErrUnsupportedOS }
