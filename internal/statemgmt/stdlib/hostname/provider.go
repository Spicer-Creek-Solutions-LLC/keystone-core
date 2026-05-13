package hostname

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms.
var ErrUnsupportedOS = errors.New("hostname: unsupported OS for v0.1 (Linux only)")

// Provider abstracts the OS-level hostname operations.
type Provider interface {
	// Current returns the static hostname (/etc/hostname). set=false
	// when the file is absent (e.g., a fresh container that only has
	// a transient hostname).
	Current() (hostname string, set bool, err error)
	// Set persists + applies the hostname.
	Set(ctx context.Context, hostname string) error
}

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
