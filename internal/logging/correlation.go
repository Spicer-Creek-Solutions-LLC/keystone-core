package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
)

// contextKey is the type for context keys
type contextKey string

const (
	// CorrelationIDKey is the context key for correlation IDs
	CorrelationIDKey contextKey = "correlation_id"
)

var (
	// Global counter for correlation IDs
	correlationCounter uint64
)

// GenerateCorrelationID generates a new correlation ID
func GenerateCorrelationID() string {
	// Use random bytes for uniqueness
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to counter-based ID
		return fmt.Sprintf("corr-%016x", atomic.AddUint64(&correlationCounter, 1))
	}
	return fmt.Sprintf("corr-%s", hex.EncodeToString(b))
}

// ContextWithCorrelationID adds a correlation ID to a context
func ContextWithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, id)
}

// CorrelationIDFromContext extracts the correlation ID from a context
func CorrelationIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(CorrelationIDKey).(string)
	return id, ok
}

// EnsureCorrelationID ensures a context has a correlation ID, generating one if needed
func EnsureCorrelationID(ctx context.Context) (newCtx context.Context, id string) {
	if existingID, ok := CorrelationIDFromContext(ctx); ok {
		return ctx, existingID
	}

	id = GenerateCorrelationID()
	return ContextWithCorrelationID(ctx, id), id
}
