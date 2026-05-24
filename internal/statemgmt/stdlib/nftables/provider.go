// SPDX-License-Identifier: Apache-2.0

package nftables

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (nftables is
// Linux-only).
var ErrUnsupportedOS = errors.New("nftables: unsupported OS for v0.1 (Linux only)")

// ErrNoNft is returned when the `nft` binary is not on PATH.
var ErrNoNft = errors.New("nftables: the nft binary was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoNft(err error) bool         { return errors.Is(err, ErrNoNft) }

// RuleHandle is one rule of a chain: its canonical text (as `nft list
// chain` prints it, with the trailing " # handle N" comment stripped
// and surrounding whitespace trimmed) plus that handle.
type RuleHandle struct {
	Text   string
	Handle int
}

// Provider abstracts the nft operations. Production shells out to
// `nft`; tests inject a fake.
type Provider interface {
	// ListRuleHandles returns the rules of the given chain. It
	// returns (nil, nil) — not an error — when the table or chain
	// does not exist, so an `absent` declaration converges and a
	// `present` one gets a clear error from AddRule.
	ListRuleHandles(ctx context.Context, family, table, chain string) ([]RuleHandle, error)
	// AddRule appends the rule (index < 0, `nft add rule`) or inserts
	// it at the 0-based index (`nft insert rule … index N`).
	AddRule(ctx context.Context, family, table, chain string, index int, rule []string) error
	// DeleteRule deletes the rule with the given handle (`nft delete
	// rule … handle N`).
	DeleteRule(ctx context.Context, family, table, chain string, handle int) error
	// SaveRuleset writes `nft list ruleset` output to path.
	SaveRuleset(ctx context.Context, path string) error
}

// commandRunner runs `nft`. It returns combined stdout+stderr and, on
// a non-zero exit, an error wrapping the exit code and trimmed
// output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
