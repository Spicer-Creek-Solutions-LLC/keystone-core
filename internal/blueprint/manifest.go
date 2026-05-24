// SPDX-License-Identifier: Apache-2.0

package blueprint

// Parameter type names accepted in a ParamSpec.Type. They map 1:1 to
// JSON Schema primitive type names.
const (
	TypeString  = "string"
	TypeInteger = "integer"
	TypeNumber  = "number"
	TypeBoolean = "boolean"
	TypeArray   = "array"
	TypeObject  = "object"
)

// SourceSecret is the ParamSpec.Source value that marks a parameter
// as fetched from the secret broker at apply time (Epic 15 task 5;
// this package only flags it).
const SourceSecret = "secret"

// Manifest is the parsed shape of a blueprint.yaml.
type Manifest struct {
	Metadata      Metadata              `yaml:"metadata"`
	Compatibility Compatibility         `yaml:"compatibility"`
	Dependencies  Dependencies          `yaml:"dependencies"`
	Features      map[string]Feature    `yaml:"features"`
	Entrypoints   Entrypoints           `yaml:"entrypoints"`
	Parameters    map[string]ParamSpec  `yaml:"parameters"`
	Outputs       map[string]OutputSpec `yaml:"outputs"`
	Hooks         Hooks                 `yaml:"hooks"`

	// SourcePath is the absolute directory the manifest was loaded
	// from. Not serialised; set by Load.
	SourcePath string `yaml:"-"`
}

// Metadata identifies a blueprint.
type Metadata struct {
	Name        string            `yaml:"name"`
	Version     string            `yaml:"version"`
	Description string            `yaml:"description"`
	Labels      map[string]string `yaml:"labels"`
}

// Compatibility constrains where a blueprint may be applied.
type Compatibility struct {
	MinKeystoneVersion string   `yaml:"min_keystone_version"`
	Platforms          []string `yaml:"platforms"`
}

// Dependencies declares inter-blueprint edges. Requires is a hard
// edge (the dependency must be present, and is ordered before this
// blueprint). RequiresBefore is a soft ordering edge: if the named
// blueprint is present it is applied first, but its absence is not an
// error.
type Dependencies struct {
	Requires       []string `yaml:"requires"`
	RequiresBefore []string `yaml:"requires_before"`
}

// Feature is one entry of the `features:` block: an optional,
// named bundle of states included only when the feature is enabled.
// The conditional state inclusion itself is Epic 15 task 4.
type Feature struct {
	Description string   `yaml:"description"`
	Default     bool     `yaml:"default"`
	States      []string `yaml:"states"`
}

// Entrypoints names the state collections a blueprint exposes.
type Entrypoints struct {
	Default  string            `yaml:"default"`
	Rollback string            `yaml:"rollback"`
	Named    map[string]string `yaml:"named"`
}

// ParamSpec describes one declared parameter. Type/Enum/Min/Max/
// Pattern feed the assembled JSON Schema; Sensitive/Source are
// Keystone annotations (not emitted into the schema).
type ParamSpec struct {
	Type        string   `yaml:"type"`
	Description string   `yaml:"description"`
	Default     any      `yaml:"default"`
	Required    bool     `yaml:"required"`
	Enum        []any    `yaml:"enum"`
	Sensitive   bool     `yaml:"sensitive"`
	Source      string   `yaml:"source"`
	Min         *float64 `yaml:"min"`
	Max         *float64 `yaml:"max"`
	Pattern     string   `yaml:"pattern"`
}

// OutputSpec documents a value the blueprint exports after apply.
// Value is a template string resolved by the executor (task 5).
type OutputSpec struct {
	Description string `yaml:"description"`
	Value       string `yaml:"value"`
}

// Hooks lists runbook names run around apply/rollback. They execute
// as runbooks (Epic 15 task 5); this package only carries the names.
type Hooks struct {
	PreApply     []string `yaml:"pre_apply"`
	PostApply    []string `yaml:"post_apply"`
	PreRollback  []string `yaml:"pre_rollback"`
	PostRollback []string `yaml:"post_rollback"`
}
