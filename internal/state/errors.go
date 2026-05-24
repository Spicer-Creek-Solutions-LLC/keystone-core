// SPDX-License-Identifier: Apache-2.0

// Package state defines the persistence-layer interfaces and types shared
// by the SQLite and PostgreSQL backends.
package state

import "errors"

// Sentinel errors returned by Store implementations.
var (
	// ErrNotFound is returned when a Get*/Update*/Delete* call references
	// a record that does not exist.
	ErrNotFound = errors.New("state: not found")

	// ErrDuplicate is returned by Create* paths when a unique
	// constraint would be violated by the insert. Backends MUST wrap
	// this sentinel so callers can branch with errors.Is rather than
	// string-matching driver-specific error messages.
	ErrDuplicate = errors.New("state: duplicate record")
)
