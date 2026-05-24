// SPDX-License-Identifier: Apache-2.0

package statemgmt

// StateFile is one parsed state document. The parser produces this
// from raw YAML bytes; the runner consumes it after templating,
// validation, and dependency resolution. See PROJECT-DETAILS §4.8.
//
// Declarations preserve source-file order. Order is not the resolver's
// dependency signal — requisites are — but is a stable tie-breaker
// the topological sort and error messages rely on.
type StateFile struct {
	Metadata     Metadata
	Includes     []string
	Variables    map[string]any
	Declarations []*Declaration
}

// Metadata is the optional bookkeeping block at the top of a state
// file. Both fields are advisory in v1.0 — they show up in run
// history and CLI output but the runner does not enforce semver on
// Version or uniqueness on Name across files.
type Metadata struct {
	Name    string
	Version string
}
