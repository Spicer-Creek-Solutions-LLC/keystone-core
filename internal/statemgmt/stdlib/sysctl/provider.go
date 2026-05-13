package sysctl

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (sysctl
// semantics differ on BSD/macOS; v1.0 is Linux-only).
var ErrUnsupportedOS = errors.New("sysctl: unsupported OS for v0.1 (Linux only)")

// ErrKeyNotFound is returned by the module's Check/Apply when the
// declared sysctl key doesn't exist in /proc/sys (e.g., a module
// providing it isn't loaded). Surfaced so the operator can wire a
// require: [kernel_module: ...] relationship.
var ErrKeyNotFound = errors.New("sysctl: key not present in /proc/sys")

// Provider abstracts the OS-level sysctl operations. Production
// uses the Linux impl; tests inject a fake.
type Provider interface {
	// Get returns the current runtime value. exists=false when the
	// key is absent from /proc/sys.
	Get(key string) (value string, exists bool, err error)
	// Set writes the runtime value (sysctl -w key=value).
	Set(ctx context.Context, key, value string) error
	// ReadPersist returns the value recorded in the keystone-managed
	// /etc/sysctl.d file for key. exists=false when the file is
	// absent or doesn't carry that key.
	ReadPersist(key string) (value string, exists bool, err error)
	// WritePersist atomically writes the keystone-managed file for
	// key with the given value.
	WritePersist(key, value string) error
}

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsKeyNotFound(err error) bool   { return errors.Is(err, ErrKeyNotFound) }
