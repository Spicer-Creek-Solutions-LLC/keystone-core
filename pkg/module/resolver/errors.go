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

// ActionableError is an error that includes a hint for how to fix it
type ActionableError interface {
	error
	// Hint returns actionable suggestion for fixing the error
	Hint() string
	// Command returns a suggested command to run (if applicable)
	Command() string
	// DocURL returns a documentation URL for more information
	DocURL() string
}

// ModuleNotFoundError indicates a module was not found with actionable hints
type ModuleNotFoundError struct {
	// Module is the module name that was not found
	Module string

	// Registry is the registry that was searched
	Registry string

	// Suggestions are similar module names that might be what the user wanted
	Suggestions []string
}

func (e *ModuleNotFoundError) Error() string {
	msg := fmt.Sprintf("module %q not found in registry", e.Module)
	if e.Registry != "" {
		msg = fmt.Sprintf("module %q not found in registry %s", e.Module, e.Registry)
	}
	return msg
}

func (e *ModuleNotFoundError) Hint() string {
	hints := []string{
		"Check the module name for typos",
		"Verify the module exists in the registry: kscore module search " + e.Module,
	}
	if len(e.Suggestions) > 0 {
		hints = append(hints, fmt.Sprintf("Did you mean: %s?", strings.Join(e.Suggestions, ", ")))
	}
	hints = append(hints, "If using a private registry, ensure you are authenticated: kscore registry login")
	return strings.Join(hints, "\n  - ")
}

func (e *ModuleNotFoundError) Command() string {
	return "kscore module search " + e.Module
}

func (e *ModuleNotFoundError) DocURL() string {
	return "https://docs.keystonecore.io/modules/finding-modules"
}

// VersionNotFoundError indicates a specific version was not found
type VersionNotFoundError struct {
	// Module is the module name
	Module string

	// Version is the requested version
	Version string

	// AvailableVersions are versions that are available
	AvailableVersions []string

	// LatestVersion is the latest available version
	LatestVersion string
}

func (e *VersionNotFoundError) Error() string {
	return fmt.Sprintf("version %s of module %q not found", e.Version, e.Module)
}

func (e *VersionNotFoundError) Hint() string {
	hints := []string{
		"Check that the version exists: kscore module versions " + e.Module,
	}
	if e.LatestVersion != "" {
		hints = append(hints, fmt.Sprintf("Latest available version is %s", e.LatestVersion))
	}
	if len(e.AvailableVersions) > 0 && len(e.AvailableVersions) <= 5 {
		hints = append(hints, fmt.Sprintf("Available versions: %s", strings.Join(e.AvailableVersions, ", ")))
	}
	hints = append(hints, "Use a version constraint instead of exact version: ^"+e.Version[:strings.Index(e.Version+".", ".")])
	return strings.Join(hints, "\n  - ")
}

func (e *VersionNotFoundError) Command() string {
	return "kscore module versions " + e.Module
}

func (e *VersionNotFoundError) DocURL() string {
	return "https://docs.keystonecore.io/modules/versioning"
}

// CircularDependencyError represents a circular dependency error with the cycle path
type CircularDependencyError struct {
	// Cycle is the list of modules forming the cycle
	Cycle []string
}

// Error implements the error interface
func (e *CircularDependencyError) Error() string {
	return fmt.Sprintf("circular dependency detected: %s", strings.Join(e.Cycle, " -> "))
}

func (e *CircularDependencyError) Hint() string {
	hints := []string{
		"Circular dependencies prevent modules from being loaded in the correct order",
		"Review the dependency chain: " + strings.Join(e.Cycle, " -> "),
		"Consider restructuring modules to break the cycle",
		"Use 'kscore module graph' to visualize the full dependency tree",
	}
	if len(e.Cycle) >= 2 {
		hints = append(hints, fmt.Sprintf("Check if %s really needs to depend on %s", e.Cycle[len(e.Cycle)-2], e.Cycle[len(e.Cycle)-1]))
	}
	return strings.Join(hints, "\n  - ")
}

func (e *CircularDependencyError) Command() string {
	return "kscore module graph"
}

func (e *CircularDependencyError) DocURL() string {
	return "https://docs.keystonecore.io/modules/dependencies#circular-dependencies"
}

// ConstraintError represents a constraint-related error
type ConstraintError struct {
	// Module is the module name
	Module string

	// Constraint is the constraint string
	Constraint string

	// Reason is the error reason
	Reason string

	// ValidExamples are examples of valid constraint syntax
	ValidExamples []string
}

// Error implements the error interface
func (e *ConstraintError) Error() string {
	return fmt.Sprintf("constraint error for %s (%s): %s", e.Module, e.Constraint, e.Reason)
}

func (e *ConstraintError) Hint() string {
	hints := []string{
		"Version constraints use semantic versioning (semver)",
	}

	examples := e.ValidExamples
	if len(examples) == 0 {
		examples = []string{
			"^1.0.0  - Compatible with 1.x.x (>=1.0.0 <2.0.0)",
			"~1.0.0  - Approximately 1.0.x (>=1.0.0 <1.1.0)",
			">=1.0.0 - Version 1.0.0 or higher",
			"1.0.0   - Exact version 1.0.0",
			"*       - Any version",
		}
	}
	hints = append(hints, "Valid constraint examples:")
	for _, ex := range examples {
		hints = append(hints, "  "+ex)
	}

	return strings.Join(hints, "\n  - ")
}

func (e *ConstraintError) Command() string {
	return ""
}

func (e *ConstraintError) DocURL() string {
	return "https://docs.keystonecore.io/modules/versioning#constraints"
}

// ConflictError represents a version conflict error
type ConflictError struct {
	// Module is the module name
	Module string

	// Constraints are the conflicting constraints
	Constraints []string

	// Requesters are the modules that requested each constraint
	Requesters []string

	// SuggestedResolution is a suggested version that might resolve the conflict
	SuggestedResolution string
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

func (e *ConflictError) Hint() string {
	hints := []string{
		"Multiple modules require incompatible versions of " + e.Module,
	}

	if len(e.Requesters) > 0 {
		hints = append(hints, "Conflicting requirements:")
		for i := range e.Constraints {
			hints = append(hints, fmt.Sprintf("  %s requires %s", e.Requesters[i], e.Constraints[i]))
		}
	}

	hints = append(hints,
		"Possible solutions:",
		"  1. Update dependent modules to use compatible versions",
		"  2. Use 'kscore module why "+e.Module+"' to see why each version is required",
		"  3. Add an explicit version override in your manifest",
	)

	if e.SuggestedResolution != "" {
		hints = append(hints, fmt.Sprintf("  4. Try pinning to version %s which may satisfy all constraints", e.SuggestedResolution))
	}

	return strings.Join(hints, "\n  - ")
}

func (e *ConflictError) Command() string {
	return "kscore module why " + e.Module
}

func (e *ConflictError) DocURL() string {
	return "https://docs.keystonecore.io/modules/dependencies#resolving-conflicts"
}

// RegistryError represents a registry-related error
type RegistryError struct {
	// Operation is the operation that failed
	Operation string

	// Module is the module name
	Module string

	// Version is the version (if applicable)
	Version string

	// RegistryURL is the registry URL
	RegistryURL string

	// StatusCode is the HTTP status code (if applicable)
	StatusCode int

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

func (e *RegistryError) Hint() string {
	hints := []string{}

	switch e.StatusCode {
	case 401:
		hints = append(hints, "Authentication required. Log in to the registry:")
		hints = append(hints, "  kscore registry login "+e.RegistryURL)
	case 403:
		hints = append(hints, "Access denied. Check your permissions for this module")
		hints = append(hints, "Verify your authentication token is valid: kscore registry whoami")
	case 404:
		hints = append(hints, "Module or version not found. Verify the module name and version")
		hints = append(hints, "Search for available modules: kscore module search "+e.Module)
	case 429:
		hints = append(hints, "Rate limited. Wait a moment and try again")
		hints = append(hints, "Consider using a local cache: kscore config set module.cache.enabled true")
	case 500, 502, 503:
		hints = append(hints, "Registry server error. The registry may be temporarily unavailable")
		hints = append(hints, "Check registry status or try again later")
		hints = append(hints, "Use offline mode if you have cached modules: kscore module --offline")
	default:
		hints = append(hints, "Check your network connection")
		hints = append(hints, "Verify the registry URL is correct: "+e.RegistryURL)
		hints = append(hints, "Try using a different registry: kscore config set module.registry <url>")
	}

	return strings.Join(hints, "\n  - ")
}

func (e *RegistryError) Command() string {
	if e.StatusCode == 401 || e.StatusCode == 403 {
		return "kscore registry login"
	}
	return ""
}

func (e *RegistryError) DocURL() string {
	return "https://docs.keystonecore.io/modules/registries"
}

// VerificationError represents a module verification error
type VerificationError struct {
	// Module is the module name
	Module string

	// Version is the version
	Version string

	// Reason is the verification failure reason
	Reason string

	// ExpectedHash is the expected hash
	ExpectedHash string

	// ActualHash is the actual hash
	ActualHash string

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

func (e *VerificationError) Hint() string {
	hints := []string{
		"Module verification ensures the downloaded content matches the expected hash",
	}

	if e.ExpectedHash != "" && e.ActualHash != "" {
		hints = append(hints, fmt.Sprintf("Expected: %s", e.ExpectedHash))
		hints = append(hints, fmt.Sprintf("Actual:   %s", e.ActualHash))
	}

	hints = append(hints,
		"Possible causes:",
		"  1. The module was tampered with or corrupted during download",
		"  2. The registry has an outdated or incorrect hash",
		"  3. A network proxy modified the content",
		"  4. The local cache is corrupted",
		"",
		"Try these solutions:",
		"  1. Clear the module cache: kscore module cache clean "+e.Module,
		"  2. Re-download the module: kscore module download "+e.Module+"@"+e.Version,
		"  3. If you trust the source, skip verification (not recommended):",
		"     kscore module download --skip-verify "+e.Module+"@"+e.Version,
	)

	return strings.Join(hints, "\n  - ")
}

func (e *VerificationError) Command() string {
	return "kscore module cache clean " + e.Module
}

func (e *VerificationError) DocURL() string {
	return "https://docs.keystonecore.io/modules/security#verification"
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

func (e *CacheError) Hint() string {
	hints := []string{}

	switch e.Operation {
	case "read":
		hints = append(hints, "Failed to read from the module cache")
		hints = append(hints, "The cached module may be corrupted or incomplete")
		hints = append(hints, "Try clearing the cache: kscore module cache clean")
	case "write":
		hints = append(hints, "Failed to write to the module cache")
		hints = append(hints, "Possible causes:")
		hints = append(hints, "  - Insufficient disk space")
		hints = append(hints, "  - Permission denied on cache directory")
		hints = append(hints, "  - Cache directory doesn't exist")
		hints = append(hints, "Check cache location: kscore config get module.cache.dir")
	case "delete":
		hints = append(hints, "Failed to delete from the module cache")
		hints = append(hints, "The file may be in use by another process")
	default:
		hints = append(hints, "An error occurred with the module cache")
	}

	if e.Path != "" {
		hints = append(hints, "Cache path: "+e.Path)
	}

	hints = append(hints,
		"",
		"General cache solutions:",
		"  1. Clear the entire cache: kscore module cache clean --all",
		"  2. Change cache location: kscore config set module.cache.dir /new/path",
		"  3. Check disk space: df -h (Linux/Mac) or dir (Windows)",
	)

	return strings.Join(hints, "\n  - ")
}

func (e *CacheError) Command() string {
	return "kscore module cache clean"
}

func (e *CacheError) DocURL() string {
	return "https://docs.keystonecore.io/modules/cache"
}

// FormatActionableError formats an error with its hint if it's an ActionableError
func FormatActionableError(err error) string {
	if err == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Error: ")
	sb.WriteString(err.Error())
	sb.WriteString("\n")

	if ae, ok := err.(ActionableError); ok {
		if hint := ae.Hint(); hint != "" {
			sb.WriteString("\nSuggestions:\n  - ")
			sb.WriteString(hint)
			sb.WriteString("\n")
		}

		if cmd := ae.Command(); cmd != "" {
			sb.WriteString("\nSuggested command:\n  $ ")
			sb.WriteString(cmd)
			sb.WriteString("\n")
		}

		if url := ae.DocURL(); url != "" {
			sb.WriteString("\nDocumentation:\n  ")
			sb.WriteString(url)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
