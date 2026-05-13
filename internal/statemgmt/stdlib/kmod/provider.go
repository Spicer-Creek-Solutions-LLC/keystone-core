package kmod

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (no
// kernel-module concept).
var ErrUnsupportedOS = errors.New("kernel_module: unsupported OS for v0.1 (Linux only)")

// Provider abstracts the OS-level kernel-module operations.
type Provider interface {
	// Loaded reports whether the named module is currently loaded
	// (present in /proc/modules).
	Loaded(name string) (bool, error)
	// Load runs `modprobe <name>`.
	Load(ctx context.Context, name string) error
	// Unload runs `modprobe -r <name>`.
	Unload(ctx context.Context, name string) error
	// PersistExists reports whether the keystone-managed
	// /etc/modules-load.d entry for name is present.
	PersistExists(name string) (bool, error)
	// AddPersist writes that entry.
	AddPersist(name string) error
	// RemovePersist deletes that entry.
	RemovePersist(name string) error
}

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
