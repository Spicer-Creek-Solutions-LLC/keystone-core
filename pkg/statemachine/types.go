package statemachine

import (
	"context"

	"github.com/shawnbutts/keystone-core/internal/logging"
)

// Logger is used for reporting state machine errors and callback panics.
type Logger = logging.Logger

// Field represents a structured log field.
type Field = logging.Field

// ErrorHandler receives errors and panics surfaced by the state machine.
type ErrorHandler func(ctx context.Context, err error)
