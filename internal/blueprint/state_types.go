// Package blueprint provides state-related types for blueprint execution.
// These types mirror pkg/statemgmt types to avoid import cycles.
package blueprint

// StateFile represents a state file with blueprint includes resolved.
// This is the blueprint package's view of a state file, separate from statemgmt.StateFile.
type StateFile struct {
	// Path is the original state file path.
	Path string

	// Includes are file includes from the original state file.
	Includes []string

	// BlueprintIncludes are blueprint includes to resolve.
	BlueprintIncludes []Include

	// Variables are variable definitions.
	Variables map[string]interface{}

	// States are state declarations organized by module.
	States map[string][]StateDeclaration

	// Metadata contains file metadata.
	Metadata *BlueprintMetadata
}

// Note: Include is defined in loader.go

// StateDeclaration represents a single state declaration.
type StateDeclaration struct {
	// ID is the unique identifier for this state.
	ID string

	// Module is the state module name (e.g., "file", "package").
	Module string

	// State is the desired state (e.g., "present", "absent").
	State string

	// Parameters are module-specific parameters.
	Parameters map[string]interface{}

	// Order defines execution order.
	Order int

	// FailHard stops execution on failure.
	FailHard bool

	// Unless is a condition that skips this state if true.
	Unless string

	// OnlyIf is a condition that must be true to execute.
	OnlyIf string

	// Requisites define dependencies between states.
	Requisites Requisites
}

// Requisites define relationships between states.
type Requisites struct {
	// Require specifies states that must complete before this one.
	Require []StateReference

	// RequireIn specifies states that require this one.
	RequireIn []StateReference

	// Watch specifies states to watch for changes.
	Watch []StateReference

	// WatchIn specifies states that watch this one.
	WatchIn []StateReference

	// Prereq specifies prerequisite states.
	Prereq []StateReference

	// PrereqIn specifies states for which this is a prereq.
	PrereqIn []StateReference

	// Onchanges specifies states that trigger this on change.
	Onchanges []StateReference

	// OnchangesIn specifies states triggered by changes to this.
	OnchangesIn []StateReference
}

// StateReference references another state.
type StateReference struct {
	// Module is the state module.
	Module string

	// ID is the state identifier.
	ID string
}

// BlueprintMetadata contains state file metadata.
//
//nolint:revive // Cannot rename to Metadata as there's already a Metadata struct in this package
type BlueprintMetadata struct {
	// Name is the state file name.
	Name string

	// Description describes the state file.
	Description string

	// Version is the state file version.
	Version string

	// Author is the author name.
	Author string

	// Tags are categorization tags.
	Tags []string
}

// StateFileConverter converts between blueprint and statemgmt state representations.
// This interface is implemented by callers who need to integrate with statemgmt.
type StateFileConverter interface {
	// ToStateFile converts a StateFile to the target state file type.
	ToStateFile(bsf *StateFile) (interface{}, error)

	// FromStateFile converts a target state file to StateFile.
	FromStateFile(sf interface{}) (*StateFile, error)
}
