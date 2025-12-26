package resolver

import (
	"errors"
	"fmt"
	"strings"
)

// Common resolver errors
var (
	// ErrModuleNotFound indicates a module was not found in the registry
	ErrModuleNotFound = errors.New("module not found")

	// ErrVersionNotFound indicates a specific version was not found
	ErrVersionNotFound = errors.New("version not found")

	// ErrNoVersionsAvailable indicates no versions are available
	ErrNoVersionsAvailable = errors.New("no versions available")

	// ErrCircularDependency indicates a circular dependency was detected
	ErrCircularDependency = errors.New("circular dependency detected")

	// ErrConstraintUnsatisfiable indicates no version satisfies all constraints
	ErrConstraintUnsatisfiable = errors.New("constraint unsatisfiable")

	// ErrInvalidConstraint indicates a constraint string is invalid
	ErrInvalidConstraint = errors.New("invalid version constraint")

	// ErrInvalidVersion indicates a version string is invalid
	ErrInvalidVersion = errors.New("invalid version")

	// ErrMaxDepthExceeded indicates the maximum dependency depth was exceeded
	ErrMaxDepthExceeded = errors.New("maximum dependency depth exceeded")

	// ErrCacheCorrupted indicates the module cache is corrupted
	ErrCacheCorrupted = errors.New("cache corrupted")

	// ErrCacheReadonly indicates attempting to write to a read-only cache
	ErrCacheReadonly = errors.New("cache is read-only")

	// ErrLockFileInvalid indicates a lock file is invalid
	ErrLockFileInvalid = errors.New("lock file invalid")

	// ErrLockFileMismatch indicates a lock file doesn't match the manifest
	ErrLockFileMismatch = errors.New("lock file mismatch")
)

// CircularDependencyError represents a circular dependency error with the cycle path
type CircularDependencyError struct {
	// Cycle is the list of modules forming the cycle
	Cycle []string
}

// Error implements the error interface
func (e *CircularDependencyError) Error() string {
	return fmt.Sprintf("circular dependency detected: %s", strings.Join(e.Cycle, " -> "))
}

// ConstraintError represents a constraint-related error
type ConstraintError struct {
	// Module is the module name
	Module string

	// Constraint is the constraint string
	Constraint string

	// Reason is the error reason
	Reason string
}

// Error implements the error interface
func (e *ConstraintError) Error() string {
	return fmt.Sprintf("constraint error for %s (%s): %s", e.Module, e.Constraint, e.Reason)
}

// ConflictError represents a version conflict error
type ConflictError struct {
	// Module is the module name
	Module string

	// Constraints are the conflicting constraints
	Constraints []string

	// Requesters are the modules that requested each constraint
	Requesters []string
}

// Error implements the error interface
func (e *ConflictError) Error() string {
	if len(e.Requesters) > 0 {
		conflicts := make([]string, len(e.Constraints))
		for i := range e.Constraints {
			conflicts[i] = fmt.Sprintf("%s (required by %s)", e.Constraints[i], e.Requesters[i])
		}
		return fmt.Sprintf("version conflict for %s: %s", e.Module, strings.Join(conflicts, ", "))
	}
	return fmt.Sprintf("version conflict for %s: %s", e.Module, strings.Join(e.Constraints, ", "))
}

// RegistryError represents a registry-related error
type RegistryError struct {
	// Operation is the operation that failed
	Operation string

	// Module is the module name
	Module string

	// Version is the version (if applicable)
	Version string

	// Err is the underlying error
	Err error
}

// Error implements the error interface
func (e *RegistryError) Error() string {
	if e.Version != "" {
		return fmt.Sprintf("registry error during %s for %s@%s: %v", e.Operation, e.Module, e.Version, e.Err)
	}
	return fmt.Sprintf("registry error during %s for %s: %v", e.Operation, e.Module, e.Err)
}

// Unwrap returns the underlying error
func (e *RegistryError) Unwrap() error {
	return e.Err
}

// VerificationError represents a module verification error
type VerificationError struct {
	// Module is the module name
	Module string

	// Version is the version
	Version string

	// Reason is the verification failure reason
	Reason string

	// Err is the underlying error
	Err error
}

// Error implements the error interface
func (e *VerificationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("verification failed for %s@%s: %s (%v)", e.Module, e.Version, e.Reason, e.Err)
	}
	return fmt.Sprintf("verification failed for %s@%s: %s", e.Module, e.Version, e.Reason)
}

// Unwrap returns the underlying error
func (e *VerificationError) Unwrap() error {
	return e.Err
}

// CacheError represents a cache-related error
type CacheError struct {
	// Operation is the operation that failed
	Operation string

	// Module is the module name
	Module string

	// Path is the cache path (if applicable)
	Path string

	// Err is the underlying error
	Err error
}

// Error implements the error interface
func (e *CacheError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("cache error during %s for %s at %s: %v", e.Operation, e.Module, e.Path, e.Err)
	}
	return fmt.Sprintf("cache error during %s for %s: %v", e.Operation, e.Module, e.Err)
}

// Unwrap returns the underlying error
func (e *CacheError) Unwrap() error {
	return e.Err
}
