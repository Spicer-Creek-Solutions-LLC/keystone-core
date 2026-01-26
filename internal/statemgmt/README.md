# State Management Package Layout

This package stays in a single Go package (`statemgmt`) to keep imports stable,
but files are grouped by responsibility and naming prefixes.

## Core Pipeline
- `types.go` and `parser.go` define state file structures and parsing.
- `validator.go` validates declarations and schema rules.
- `executor.go` and `dependency.go` handle execution order and requisites.
- `diff.go` and `template.go` cover drift detection and templating.

## Module Implementations
Module files use the `module_*` prefix and are grouped by domain:
- `module_k8s_*` for Kubernetes resources
- `module_win_*` for Windows-only modules
- `module_*_test.go` for module-specific tests

If future changes require splitting into subpackages, the module registry should
be the first boundary to extract so callers keep using `statemgmt` through a
single entry point.
