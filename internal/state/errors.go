// Package state defines the persistence-layer interfaces and types shared
// by the SQLite and PostgreSQL backends.
package state

import "errors"

// Sentinel errors returned by Store implementations.
var (
	// ErrNotFound is returned when a Get*/Update*/Delete* call references
	// a record that does not exist.
	ErrNotFound = errors.New("state: not found")

	// ErrNotImplemented is returned by stub methods on backends whose real
	// implementation is still pending. Real backends never return this.
	ErrNotImplemented = errors.New("state: not implemented")
)
