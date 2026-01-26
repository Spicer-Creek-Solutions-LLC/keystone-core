package statemgmt

import (
	"time"
)

// StateFile represents a parsed state configuration file
type StateFile struct {
	// Path to the state file
	Path string

	// Includes are other state files to include (simple file paths)
	Includes []string

	// BlueprintIncludes are blueprint includes with parameters
	BlueprintIncludes []BlueprintInclude

	// Variables for templating
	Variables map[string]interface{}

	// States contains all state declarations organized by module type
	States map[string][]StateDeclaration

	// Metadata about the state file
	Metadata StateMetadata
}

// BlueprintInclude represents a blueprint include directive in a state file.
type BlueprintInclude struct {
	// Blueprint is the blueprint reference (e.g., "blueprints/community/web-app-stack")
	Blueprint string

	// Version is the version constraint (e.g., "^1.0.0", ">=1.2.0")
	Version string

	// As is the namespace for this instance (for multiple instances of same blueprint)
	As string

	// Entrypoint specifies a specific entry point within the blueprint
	Entrypoint string

	// Features enables/disables optional blueprint features
	Features map[string]bool

	// Parameters are the values for the blueprint's parameters
	Parameters map[string]interface{}
}

// IsBlueprint returns true (satisfies interface for polymorphic include handling).
func (i *BlueprintInclude) IsBlueprint() bool {
	return true
}

// StateMetadata contains metadata about a state file
type StateMetadata struct {
	// Name is a descriptive name for this state file
	Name string

	// Description describes the purpose of this state
	Description string

	// Version is the state file format version
	Version string

	// Tags for organizing states
	Tags []string
}

// StateDeclaration represents a single state declaration
type StateDeclaration struct {
	// ID uniquely identifies this state (e.g., "/etc/nginx/nginx.conf" for a file)
	ID string

	// Module is the state module type (e.g., "file", "package", "service")
	Module string

	// State is the desired state (e.g., "present", "absent", "running", "stopped")
	State string

	// Parameters contains module-specific parameters
	Parameters map[string]interface{}

	// Requisites define dependencies between states
	Requisites Requisites

	// OnChanges defines states to run when this state changes
	OnChanges []StateReference

	// Watch defines states that should trigger this state when they change
	Watch []StateReference

	// Unless is a condition that prevents execution if true
	Unless string

	// OnlyIf is a condition that only allows execution if true
	OnlyIf string

	// Order specifies explicit ordering (lower numbers run first)
	Order int

	// FailHard causes state application to stop if this state fails
	FailHard bool

	// Retry configuration
	Retry *RetryConfig
}

// Requisites defines dependencies between states
type Requisites struct {
	// Require states must succeed before this state runs
	Require []StateReference

	// RequireIn makes this state a requirement for other states
	RequireIn []StateReference

	// Watch states trigger this state when they change
	Watch []StateReference

	// WatchIn makes this state watch other states
	WatchIn []StateReference

	// Prereq states must run before this state during dry-run
	Prereq []StateReference

	// PrereqIn makes this state a prereq for other states
	PrereqIn []StateReference

	// Onchanges only run this state if referenced states change
	Onchanges []StateReference

	// OnchangesIn makes this state run on changes to other states
	OnchangesIn []StateReference
}

// StateReference references another state
type StateReference struct {
	// Module is the state module type (e.g., "file", "package")
	Module string

	// ID is the state identifier
	ID string
}

// RetryConfig defines retry behavior for a state
type RetryConfig struct {
	// Attempts is the number of retry attempts
	Attempts int

	// Delay is the delay between retries
	Delay time.Duration

	// BackoffMultiplier increases delay after each retry
	BackoffMultiplier float64

	// MaxDelay caps the retry delay
	MaxDelay time.Duration
}

// CompiledState represents a state file after compilation and dependency resolution
type CompiledState struct {
	// StateFile is the original parsed state
	StateFile *StateFile

	// ExecutionOrder is the resolved execution order
	ExecutionOrder []*StateDeclaration

	// DependencyGraph represents state dependencies
	DependencyGraph *DependencyGraph

	// CompiledAt is when this state was compiled
	CompiledAt time.Time
}

// DependencyGraph represents state dependencies
type DependencyGraph struct {
	// Nodes maps state IDs to their declarations
	Nodes map[string]*StateDeclaration

	// Edges maps state IDs to their dependencies
	Edges map[string][]string

	// Levels contains states grouped by dependency level
	Levels [][]string
}

// StateResult represents the result of applying a state
type StateResult struct {
	// StateID is the ID of the state
	StateID string

	// Module is the state module type
	Module string

	// Success indicates if the state succeeded
	Success bool

	// Changed indicates if the state made changes
	Changed bool

	// Comment describes what happened
	Comment string

	// Changes details what changed
	Changes map[string]interface{}

	// Duration is how long the state took
	Duration time.Duration

	// Error if the state failed
	Error error

	// StartTime when the state started
	StartTime time.Time

	// EndTime when the state ended
	EndTime time.Time
}

// StateRun represents a complete state application run
type StateRun struct {
	// RunID uniquely identifies this run
	RunID string

	// StateFiles are the state files being applied
	StateFiles []string

	// Target is the targeting expression for which agents to apply to
	Target string

	// DryRun indicates if this is a dry-run (preview only)
	DryRun bool

	// StartTime when the run started
	StartTime time.Time

	// EndTime when the run ended
	EndTime time.Time

	// Results contains results for each state
	Results []*StateResult

	// Summary contains aggregate statistics
	Summary *RunSummary

	// AgentID is the agent this run executed on
	AgentID string
}

// RunSummary contains aggregate statistics for a state run
type RunSummary struct {
	// Total number of states
	Total int

	// Succeeded states
	Succeeded int

	// Failed states
	Failed int

	// Changed states
	Changed int

	// Unchanged states
	Unchanged int

	// Duration of the entire run
	Duration time.Duration

	// Success is true if all states succeeded
	Success bool
}

// ValidationError represents a state validation error
type ValidationError struct {
	// StateID that has the error
	StateID string

	// Module type
	Module string

	// Field that has the error
	Field string

	// Message describes the error
	Message string

	// Line number in the state file (if available)
	Line int
}

func (e *ValidationError) Error() string {
	if e.Line > 0 {
		return e.Message + " (line " + string(rune(e.Line)) + ")"
	}
	return e.Message
}

// StateFileFormat defines supported state file formats
type StateFileFormat string

const (
	// FormatYAML is YAML format
	FormatYAML StateFileFormat = "yaml"

	// FormatJSON is JSON format
	FormatJSON StateFileFormat = "json"
)

// SourceType defines how to retrieve file sources
type SourceType string

const (
	// SourceTypeFile is a local file path
	SourceTypeFile SourceType = "file"

	// SourceTypeHTTP is an HTTP(S) URL
	SourceTypeHTTP SourceType = "http"

	// SourceTypeTemplate is a template file
	SourceTypeTemplate SourceType = "template"

	// SourceTypeInline is inline content
	SourceTypeInline SourceType = "inline"
)
