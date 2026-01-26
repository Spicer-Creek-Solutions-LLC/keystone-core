package errors

import "fmt"

// Kind represents a category of CLI error.
type Kind string

const (
	KindInvalidArgument Kind = "invalid_argument"
	KindNotFound        Kind = "not_found"
	KindConflict        Kind = "conflict"
	KindUnavailable     Kind = "unavailable"
	KindInternal        Kind = "internal"
)

// Error wraps an underlying error with a category and message.
type Error struct {
	Kind    Kind
	Message string
	Err     error
}

func (e *Error) Error() string {
	switch {
	case e.Err != nil && e.Message != "":
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	case e.Err != nil:
		return e.Err.Error()
	default:
		return e.Message
	}
}

func (e *Error) Unwrap() error {
	return e.Err
}

// New creates a categorized error with message.
func New(kind Kind, message string) *Error {
	return &Error{
		Kind:    kind,
		Message: message,
	}
}

// Wrap creates a categorized error that wraps err.
func Wrap(kind Kind, err error, message string) *Error {
	return &Error{
		Kind:    kind,
		Message: message,
		Err:     err,
	}
}
