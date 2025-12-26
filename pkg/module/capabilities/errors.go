package capabilities

import "errors"

var (
	// ErrNilCapability indicates a nil capability was provided
	ErrNilCapability = errors.New("capability cannot be nil")

	// ErrEmptyCapabilityName indicates an empty capability name
	ErrEmptyCapabilityName = errors.New("capability name cannot be empty")

	// ErrCapabilityAlreadyRegistered indicates a capability is already registered
	ErrCapabilityAlreadyRegistered = errors.New("capability already registered")

	// ErrCapabilityNotFound indicates a capability was not found
	ErrCapabilityNotFound = errors.New("capability not found")

	// ErrCapabilityDenied indicates a capability invocation was denied
	ErrCapabilityDenied = errors.New("capability denied")

	// ErrInvalidPath indicates an invalid file path
	ErrInvalidPath = errors.New("invalid path")

	// ErrPathNotAllowed indicates a path is not in the allowed list
	ErrPathNotAllowed = errors.New("path not allowed")

	// ErrPathDenied indicates a path is in the denied list
	ErrPathDenied = errors.New("path denied")

	// ErrDomainNotAllowed indicates a domain is not in the allowed list
	ErrDomainNotAllowed = errors.New("domain not allowed")

	// ErrDomainDenied indicates a domain is in the denied list
	ErrDomainDenied = errors.New("domain denied")

	// ErrCommandNotAllowed indicates a command is not in the allowed list
	ErrCommandNotAllowed = errors.New("command not allowed")

	// ErrRateLimitExceeded indicates a rate limit was exceeded
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// ErrTimeout indicates an operation timed out
	ErrTimeout = errors.New("operation timed out")

	// ErrMaxSizeExceeded indicates a maximum size was exceeded
	ErrMaxSizeExceeded = errors.New("maximum size exceeded")

	// ErrInvalidConfiguration indicates invalid capability configuration
	ErrInvalidConfiguration = errors.New("invalid configuration")
)
