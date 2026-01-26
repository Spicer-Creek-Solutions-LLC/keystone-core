package statemachine

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidTransition is returned when a transition is not defined for the
	// current state and event combination.
	ErrInvalidTransition = errors.New("invalid transition")

	// ErrGuardFailed is returned when a transition's guard condition returns false.
	ErrGuardFailed = errors.New("guard condition failed")

	// ErrMachineClosed is returned when attempting to use a closed state machine.
	ErrMachineClosed = errors.New("state machine is closed")

	// ErrConcurrentTransition is returned when another transition completed first.
	ErrConcurrentTransition = errors.New("concurrent transition detected")

	// ErrNoInitialState is returned when building a machine without an initial state.
	ErrNoInitialState = errors.New("no initial state specified")

	// ErrDuplicateTransition is returned when attempting to define the same
	// transition twice during machine building.
	ErrDuplicateTransition = errors.New("duplicate transition definition")
)

// TransitionError provides detailed information about a failed transition.
type TransitionError struct {
	// From is the state the machine was in when the transition was attempted.
	From any

	// Event is the event that triggered the transition attempt.
	Event any

	// Reason is the underlying error explaining why the transition failed.
	Reason error

	// Message provides additional context about the failure.
	Message string
}

// Error implements the error interface.
func (e *TransitionError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("transition failed from %v via %v: %s: %v",
			e.From, e.Event, e.Message, e.Reason)
	}
	return fmt.Sprintf("transition failed from %v via %v: %v",
		e.From, e.Event, e.Reason)
}

// Unwrap returns the underlying error.
func (e *TransitionError) Unwrap() error {
	return e.Reason
}

// Is reports whether target matches this error.
func (e *TransitionError) Is(target error) bool {
	return errors.Is(e.Reason, target)
}

// NewTransitionError creates a new TransitionError.
func NewTransitionError(from, event any, reason error, message string) *TransitionError {
	return &TransitionError{
		From:    from,
		Event:   event,
		Reason:  reason,
		Message: message,
	}
}

// BuildError represents an error that occurred while building a state machine.
type BuildError struct {
	// Reason is the underlying error.
	Reason error

	// Message provides additional context about the build failure.
	Message string
}

// Error implements the error interface.
func (e *BuildError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("failed to build state machine: %s: %v", e.Message, e.Reason)
	}
	return fmt.Sprintf("failed to build state machine: %v", e.Reason)
}

// Unwrap returns the underlying error.
func (e *BuildError) Unwrap() error {
	return e.Reason
}

// NewBuildError creates a new BuildError.
func NewBuildError(reason error, message string) *BuildError {
	return &BuildError{
		Reason:  reason,
		Message: message,
	}
}
