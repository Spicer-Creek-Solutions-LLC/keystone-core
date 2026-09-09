// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// readSnapshot returns the parsed snapshot and its exact bytes. The bytes are
// what the allowlist hashes, so the binding is to the reviewed file rather than
// to a re-serialisation of it.
func readSnapshot(path string) (*Snapshot, []byte, error) {
	if path == "" {
		return nil, nil, fmt.Errorf("--snapshot <path> is required")
	}
	b, err := os.ReadFile(path) // #nosec G304 G703 -- operator-supplied --snapshot path
	if err != nil {
		return nil, nil, fmt.Errorf("read snapshot: %w", err)
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, nil, fmt.Errorf("parse snapshot: %w", err)
	}
	if s.Schema != "keystone-core/tracker-snapshot" {
		return nil, nil, fmt.Errorf("%s is not a tracker snapshot (schema %q)", path, s.Schema)
	}
	// A snapshot whose recorded hash no longer matches its records was edited
	// after it was taken, which makes it unreviewable.
	if got := hashRecords(s.Issues, s.Milestones); got != s.ResponseHash {
		return nil, nil, fmt.Errorf("snapshot %s records hash %s but its contents hash to %s; it was edited after capture",
			path, short(s.ResponseHash), short(got))
	}
	return &s, b, nil
}

func readAllowlist(path string) (*Allowlist, error) {
	if path == "" {
		return nil, fmt.Errorf("--allowlist <path> is required")
	}
	b, err := os.ReadFile(path) // #nosec G304 G703 -- operator-supplied --allowlist path
	if err != nil {
		return nil, fmt.Errorf("read allowlist: %w", err)
	}
	var a Allowlist
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("parse allowlist: %w", err)
	}
	if a.Schema != "keystone-core/tracker-allowlist" {
		return nil, fmt.Errorf("%s is not a tracker allowlist (schema %q)", path, a.Schema)
	}
	return &a, nil
}
