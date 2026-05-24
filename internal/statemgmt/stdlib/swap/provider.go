// SPDX-License-Identifier: Apache-2.0

package swap

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (v1.0 manages
// swap via /proc/swaps + mkswap(8)/swapon(8)/swapoff(8)).
var ErrUnsupportedOS = errors.New("swap: unsupported OS for v0.1 (Linux only)")

// ErrNoSwapTools is returned by mutating operations on a host where
// the mkswap / swapon / swapoff binaries are not on PATH.
var ErrNoSwapTools = errors.New("swap: the mkswap/swapon/swapoff binaries were not found on PATH")

// ErrSwapfileSizeRequired is returned when a `state: on` declaration
// needs to create a not-yet-existing swapfile but no `size:` was
// given.
var ErrSwapfileSizeRequired = errors.New("swap: a swapfile at this path does not exist and no size was given (set size: to create it)")

func IsUnsupportedOS(err error) bool        { return errors.Is(err, ErrUnsupportedOS) }
func IsNoSwapTools(err error) bool          { return errors.Is(err, ErrNoSwapTools) }
func IsSwapfileSizeRequired(err error) bool { return errors.Is(err, ErrSwapfileSizeRequired) }

// SwapInfo is the live state of a swap source. Active==false → the
// other fields are meaningless.
type SwapInfo struct {
	Source   string
	Active   bool
	Priority int
}

// Provider abstracts the swap operations. Production reads
// /proc/swaps and shells out to mkswap/swapon/swapoff (and dd for
// swapfile creation); tests inject a fake.
type Provider interface {
	// Lookup reports whether source is an active swap area.
	Lookup(ctx context.Context, source string) (*SwapInfo, error)
	// MakeSwap initialises path as a swap area (mkswap).
	MakeSwap(ctx context.Context, path string) error
	// SwapOn activates path. priority < 0 means "no explicit
	// priority" (the swapon default).
	SwapOn(ctx context.Context, path string, priority int) error
	// SwapOff deactivates path.
	SwapOff(ctx context.Context, path string) error
	// CreateSwapfile creates a non-sparse file of sizeBytes at path
	// with mode 0600 (suitable for mkswap).
	CreateSwapfile(ctx context.Context, path string, sizeBytes int64) error
}

// commandRunner runs the swap binaries. It returns combined
// stdout+stderr and, on a non-zero exit, an error wrapping the exit
// code and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
