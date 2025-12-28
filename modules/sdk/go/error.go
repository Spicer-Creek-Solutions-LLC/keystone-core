package kscoresdk

import "fmt"

// ErrorType represents different error categories
type ErrorType string

const (
	// ErrorTypeCapabilityDenied indicates a capability was not granted
	ErrorTypeCapabilityDenied ErrorType = "capability_denied"
	// ErrorTypeFileSystem indicates a filesystem error
	ErrorTypeFileSystem ErrorType = "filesystem"
	// ErrorTypeHTTP indicates an HTTP error
	ErrorTypeHTTP ErrorType = "http"
	// ErrorTypeExec indicates a command execution error
	ErrorTypeExec ErrorType = "exec"
	// ErrorTypeSerialization indicates a serialization error
	ErrorTypeSerialization ErrorType = "serialization"
	// ErrorTypeOther indicates a generic error
	ErrorTypeOther ErrorType = "other"
)

// Error represents a Keystone Core module error
type Error struct {
	Type    ErrorType
	Message string
}

// Error implements the error interface
func (e *Error) Error() string {
	switch e.Type {
	case ErrorTypeCapabilityDenied:
		return fmt.Sprintf("Capability denied: %s", e.Message)
	case ErrorTypeFileSystem:
		return fmt.Sprintf("Filesystem error: %s", e.Message)
	case ErrorTypeHTTP:
		return fmt.Sprintf("HTTP error: %s", e.Message)
	case ErrorTypeExec:
		return fmt.Sprintf("Exec error: %s", e.Message)
	case ErrorTypeSerialization:
		return fmt.Sprintf("Serialization error: %s", e.Message)
	default:
		return e.Message
	}
}

// NewCapabilityDeniedError creates a capability denied error
func NewCapabilityDeniedError(capability string) *Error {
	return &Error{
		Type:    ErrorTypeCapabilityDenied,
		Message: fmt.Sprintf("Capability '%s' not granted", capability),
	}
}

// NewFileSystemError creates a filesystem error
func NewFileSystemError(message string) *Error {
	return &Error{
		Type:    ErrorTypeFileSystem,
		Message: message,
	}
}

// NewHTTPError creates an HTTP error
func NewHTTPError(message string) *Error {
	return &Error{
		Type:    ErrorTypeHTTP,
		Message: message,
	}
}

// NewExecError creates an execution error
func NewExecError(message string) *Error {
	return &Error{
		Type:    ErrorTypeExec,
		Message: message,
	}
}

// NewSerializationError creates a serialization error
func NewSerializationError(message string) *Error {
	return &Error{
		Type:    ErrorTypeSerialization,
		Message: message,
	}
}

// NewError creates a generic error
func NewError(message string) *Error {
	return &Error{
		Type:    ErrorTypeOther,
		Message: message,
	}
}
