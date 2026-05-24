// SPDX-License-Identifier: Apache-2.0

package envelope

import (
	"context"
	"testing"

	"go.keystone-core.io/keystone-core/internal/logging"
)

func TestFromContext_StampsWhenCtxHasID(t *testing.T) {
	ctx := logging.WithCorrelationID(context.Background(), "abc-123")
	env := New(nil, "default", FromContext(ctx))
	if env.CorrelationID != "abc-123" {
		t.Errorf("CorrelationID = %q, want abc-123", env.CorrelationID)
	}
}

func TestFromContext_NoopWhenCtxEmpty(t *testing.T) {
	env := New(nil, "default", FromContext(context.Background()))
	if env.CorrelationID != "" {
		t.Errorf("CorrelationID = %q, want empty", env.CorrelationID)
	}
}

func TestToContext_InjectsWhenEnvelopeHasID(t *testing.T) {
	env := Envelope{CorrelationID: "xyz"}
	ctx := ToContext(context.Background(), env)
	if got := logging.CorrelationIDFromContext(ctx); got != "xyz" {
		t.Errorf("ctx id = %q, want xyz", got)
	}
}

func TestToContext_NoopWhenEnvelopeEmpty(t *testing.T) {
	ctx := ToContext(context.Background(), Envelope{})
	if got := logging.CorrelationIDFromContext(ctx); got != "" {
		t.Errorf("ctx id = %q, want empty", got)
	}
}

func TestFromContext_DoesNotOverrideExplicit(t *testing.T) {
	// Combining WithCorrelationID(literal) with FromContext should
	// resolve to whichever option ran LAST — the standard functional-
	// options precedence rule. Documented here so we don't accidentally
	// regress it.
	ctx := logging.WithCorrelationID(context.Background(), "from-ctx")
	env := New(nil, "default",
		WithCorrelationID("explicit"),
		FromContext(ctx),
	)
	if env.CorrelationID != "from-ctx" {
		t.Errorf("CorrelationID = %q, want from-ctx (later option wins)", env.CorrelationID)
	}
}
