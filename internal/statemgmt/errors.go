// SPDX-License-Identifier: Apache-2.0

package statemgmt

import "errors"

// Sentinel errors returned by Registry. They are wrappable so callers
// can match with errors.Is.
var (
	// ErrModuleNotFound is returned by Registry.Get when no factory
	// is registered under the requested name.
	ErrModuleNotFound = errors.New("statemgmt: module not found")

	// ErrDuplicateModule is returned by Registry.Register when a
	// factory is already registered under the requested name.
	// Re-registration is rejected rather than silently overwritten
	// so init-time registration order cannot mask a name collision.
	ErrDuplicateModule = errors.New("statemgmt: module already registered")

	// ErrInvalidModuleName is returned by Registry.Register when the
	// supplied name is empty.
	ErrInvalidModuleName = errors.New("statemgmt: invalid module name")

	// ErrNilFactory is returned by Registry.Register when the
	// supplied factory is nil.
	ErrNilFactory = errors.New("statemgmt: nil module factory")
)
