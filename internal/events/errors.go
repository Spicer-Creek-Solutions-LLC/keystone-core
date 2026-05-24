package events

import "errors"

// ErrInvalidEvent is the family root for [Event] / [EventType] /
// [Severity] / [Category] validation rejections. Constructors wrap
// context with `fmt.Errorf("%w: ...", ErrInvalidEvent)` so call
// sites match with [errors.Is].
var ErrInvalidEvent = errors.New("events: invalid event")

// ErrEventNotFound is the store-agnostic miss returned by
// [EventStore.Get] (task 2) and the gRPC `GetEvent` handler (task 6).
// SQL backends translate "no rows" into this so the policy / API
// layer matches a single sentinel.
var ErrEventNotFound = errors.New("events: event not found")

// ErrInvalidFilter is returned when an [EventQuery] DSL parse fails
// (task 2) or when a CEL filter expression (task 5) fails to compile.
// The wrapped error carries the parser / compiler diagnostic for
// operator-facing reporting.
var ErrInvalidFilter = errors.New("events: invalid filter")

// ErrPublisherNotStarted is returned by [EventPublisher] methods
// invoked before `Start` or after `Close` (task 3).
var ErrPublisherNotStarted = errors.New("events: publisher not started")

// ErrSubscriberNotStarted is returned by [EventSubscriber] methods
// invoked before `Start` or after `Close` (task 4).
var ErrSubscriberNotStarted = errors.New("events: subscriber not started")
