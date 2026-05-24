// SPDX-License-Identifier: Apache-2.0

package iptables

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (iptables is
// Linux-only).
var ErrUnsupportedOS = errors.New("iptables: unsupported OS for v0.1 (Linux only)")

// ErrNoIptables is returned when the binary for the requested family
// (iptables / ip6tables, and the matching *-save tool for `save:`)
// is not on PATH.
var ErrNoIptables = errors.New("iptables: the iptables/ip6tables binary was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoIptables(err error) bool    { return errors.Is(err, ErrNoIptables) }

// Provider abstracts the iptables operations. Production shells out
// to iptables/ip6tables (and iptables-save/ip6tables-save); tests
// inject a fake. `family` selects the IPv4 vs IPv6 binary per call.
type Provider interface {
	// HasRule reports whether the rule exists in the chain
	// (iptables -t <table> -C <chain> <rule...>). Any non-zero exit
	// from -C is taken as "rule absent" — a structural problem (a
	// missing chain) surfaces from AddRule/DeleteRule instead.
	HasRule(ctx context.Context, family, table, chain string, rule []string) (bool, error)
	// AddRule appends the rule (position == 0) or inserts it at
	// position (>= 1).
	AddRule(ctx context.Context, family, table, chain string, position int, rule []string) error
	// DeleteRule deletes one matching rule.
	DeleteRule(ctx context.Context, family, table, chain string, rule []string) error
	// Save writes the current ruleset (iptables-save) to path.
	Save(ctx context.Context, family, path string) error
}

// commandRunner runs iptables / ip6tables / *-save. It returns
// combined stdout+stderr and, on a non-zero exit, an error wrapping
// the exit code and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
