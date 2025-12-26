package manifest

import "errors"

var (
	// ErrInvalidSchemaVersion indicates an unsupported schema version
	ErrInvalidSchemaVersion = errors.New("invalid or unsupported schema version")

	// ErrMissingName indicates the module name is missing
	ErrMissingName = errors.New("module name is required")

	// ErrMissingVersion indicates the module version is missing
	ErrMissingVersion = errors.New("module version is required")

	// ErrInvalidDependency indicates a dependency is malformed
	ErrInvalidDependency = errors.New("invalid dependency: module and version required")

	// ErrNoRuntime indicates no runtime (Starlark or WASM) is specified
	ErrNoRuntime = errors.New("at least one runtime (starlark or wasm) must be specified")

	// ErrInvalidYAML indicates the YAML is malformed
	ErrInvalidYAML = errors.New("invalid YAML format")
)
