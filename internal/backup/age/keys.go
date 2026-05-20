package age

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	upstream "filippo.io/age"
)

// LoadIdentityFile parses a file containing one or more age
// identities (the AGE-SECRET-KEY-... lines emitted by age-keygen).
// Comments and blank lines are ignored by the upstream parser.
//
// Used by restore-capable nodes — typically NOT the kscore-server
// itself, which holds only recipients (see [LoadRecipientsFile]).
func LoadIdentityFile(path string) ([]upstream.Identity, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied identity file path
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("age: identity file not found: %s", path)
		}
		return nil, fmt.Errorf("age: open identity %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	ids, err := upstream.ParseIdentities(f)
	if err != nil {
		return nil, fmt.Errorf("age: parse identity %q: %w", path, err)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("age: %q contains no identities", path)
	}
	return ids, nil
}

// LoadRecipientsFile parses one or more age1... public-key lines.
// Used by backup-only nodes (e.g. kscore-server) that hold no
// secrets material.
func LoadRecipientsFile(path string) ([]upstream.Recipient, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied recipients file path
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("age: recipients file not found: %s", path)
		}
		return nil, fmt.Errorf("age: open recipients %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	recs, err := upstream.ParseRecipients(f)
	if err != nil {
		return nil, fmt.Errorf("age: parse recipients %q: %w", path, err)
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("age: %q contains no recipients", path)
	}
	return recs, nil
}

// RecipientsFromIdentities derives the public-key Recipient from
// each X25519 identity in ids. Used by tests + restore-capable nodes
// that want to round-trip without a separate recipients file. Returns
// an error if an identity does not expose a [upstream.X25519Identity]
// concrete type (e.g. scrypt identities have no recipient form).
func RecipientsFromIdentities(ids []upstream.Identity) ([]upstream.Recipient, error) {
	if len(ids) == 0 {
		return nil, errors.New("age: RecipientsFromIdentities: empty identity list")
	}
	out := make([]upstream.Recipient, 0, len(ids))
	for i, id := range ids {
		x, ok := id.(*upstream.X25519Identity)
		if !ok {
			return nil, fmt.Errorf("age: identity[%d] (%T) is not an X25519Identity", i, id)
		}
		out = append(out, x.Recipient())
	}
	return out, nil
}
