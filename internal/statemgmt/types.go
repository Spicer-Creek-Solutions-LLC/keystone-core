package statemgmt

import "time"

// Declaration is a single fully-resolved state declaration that a
// Module receives at Check / Apply / Test time.
//
// By the time a Module sees one, the runner has parsed the YAML,
// rendered templates, validated parameters, resolved requisites, and
// ordered the DAG. None of those concerns belong to the Module — they
// are the runner's job. A Module's contract is local: given this
// Declaration, what is the current state, how do I converge to the
// declared state, and did the convergence succeed?
//
// ID uniquely identifies the declaration within a state run and is
// what other declarations reference via require / watch / etc. The
// convention is "<category>:<name>" (e.g. "files:/etc/nginx/nginx.conf",
// "packages:nginx"); the runner enforces uniqueness.
type Declaration struct {
	ID     string         // unique within a state run, e.g. "files:/etc/nginx/nginx.conf"
	Module string         // module name registered in the Registry, e.g. "file"
	State  string         // declared state, must be one of Module.ValidStates()
	Name   string         // resource identifier (often a path or other natural key)
	Params map[string]any // module-specific parameters
}

// ModuleCheckResult is what Module.Check returns. Matches=true means
// the live system already satisfies the Declaration and Apply should
// be a no-op; Matches=false means convergence is required and Diff
// describes the gap in human-readable form.
type ModuleCheckResult struct {
	Matches bool
	Diff    string
}

// StateResult is what Module.Apply returns. Changed reports whether
// the system was actually modified — required for idempotency
// reporting (a second Apply of the same Declaration must produce
// Changed=false). Comment is a short operator-facing description.
type StateResult struct {
	Success  bool
	Changed  bool
	Diff     string
	Comment  string
	Duration time.Duration
}
