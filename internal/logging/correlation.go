// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
)

// HTTPHeader is the HTTP request/response header carrying the
// correlation ID. The middleware in pkg/api/server consults this on
// inbound requests and echoes it on responses; HTTP clients hand-rolling
// requests should set the same name when they want their downstream
// logs correlated with this caller's request.
const HTTPHeader = "X-Correlation-ID"

// GRPCMetadataKey is the gRPC metadata key carrying the correlation
// ID. Lower-cased per gRPC's metadata convention (HTTP/2 header
// canonicalisation requires lowercase). The interceptor in
// pkg/api/server reads inbound metadata at this key and sets the
// same key on outbound trailers.
const GRPCMetadataKey = "x-correlation-id"

type correlationKey struct{}

// WithCorrelationID returns ctx annotated with the given correlation ID.
// Pass the returned ctx to slog.*Context calls so the ID is emitted on
// every record.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey{}, id)
}

// CorrelationIDFromContext returns the correlation ID from ctx, or "".
func CorrelationIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(correlationKey{}).(string); ok {
		return v
	}
	return ""
}

// NewCorrelationID returns an opaque, random 32-char hex correlation ID.
// Suitable for marking the start of a new request or operation.
func NewCorrelationID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read on a sufficient OS never errors in practice
	return hex.EncodeToString(b[:])
}

// correlationHandler wraps a slog.Handler so that any record logged with
// a context carrying a correlation ID gets it attached as "correlation_id".
//
// Note: per slog semantics, record-level attrs added inside Handle respect
// any active group from WithGroup. So a logger created via
// `logger.WithGroup("svc")` will emit `correlation_id` nested under
// `svc`, not at the top level. WithAttrs has no such effect.
type correlationHandler struct {
	slog.Handler
}

func (h *correlationHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := CorrelationIDFromContext(ctx); id != "" {
		r.AddAttrs(slog.String("correlation_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *correlationHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &correlationHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *correlationHandler) WithGroup(name string) slog.Handler {
	return &correlationHandler{Handler: h.Handler.WithGroup(name)}
}
