// SPDX-License-Identifier: Apache-2.0

package envelope

import (
	"context"

	"go.keystone-core.io/keystone-core/internal/logging"
)

// FromContext stamps the envelope's CorrelationID from the context's
// logging.CorrelationIDFromContext value. No-op when ctx carries no
// ID — the resulting envelope's CorrelationID stays empty (which the
// existing Envelope.Validate accepts; CorrelationID is optional).
//
// Use this at publish sites instead of envelope.WithCorrelationID
// (literal) so HTTP/gRPC entry-point IDs propagate end-to-end without
// per-call-site plumbing.
func FromContext(ctx context.Context) Option {
	id := logging.CorrelationIDFromContext(ctx)
	return func(e *Envelope) {
		if id != "" {
			e.CorrelationID = id
		}
	}
}

// ToContext returns ctx annotated with env.CorrelationID. No-op when
// the envelope's CorrelationID is empty. Subscribers call this on the
// envelope they received so downstream logs / spans correlate back to
// the original request.
func ToContext(ctx context.Context, env Envelope) context.Context {
	if env.CorrelationID == "" {
		return ctx
	}
	return logging.WithCorrelationID(ctx, env.CorrelationID)
}
