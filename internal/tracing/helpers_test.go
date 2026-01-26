package tracing

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func setupTestProvider(t *testing.T) *Provider {
	config := NewStdoutConfig("test-service")
	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	return provider
}

func TestStartSpan(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	newCtx, span := StartSpan(ctx, TracerControlPlane, "test-operation")

	if span == nil {
		t.Fatal("StartSpan returned nil span")
	}

	if newCtx == ctx {
		t.Error("StartSpan should return a new context")
	}

	span.End()
}

func TestStartSpanWithKind(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	_, span := StartSpanWithKind(ctx, TracerControlPlane, "test-operation", SpanKindServer,
		StringAttr("test.key", "test.value"),
	)

	if span == nil {
		t.Fatal("StartSpanWithKind returned nil span")
	}

	span.End()
}

func TestEndSpan(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "no error",
			err:  nil,
		},
		{
			name: "with error",
			err:  errors.New("test error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, span := StartSpan(ctx, TracerControlPlane, "test-operation")

			// This should not panic
			EndSpan(span, tt.err)
		})
	}
}

func TestEndSpanWithStatus(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	_, span := StartSpan(ctx, TracerControlPlane, "test-operation")

	// This should not panic
	EndSpanWithStatus(span, codes.Ok, "success")
}

func TestRecordError(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "nil error",
			err:  nil,
		},
		{
			name: "with error",
			err:  errors.New("test error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, span := StartSpan(ctx, TracerControlPlane, "test-operation")
			defer span.End()

			// This should not panic
			RecordError(span, tt.err)
		})
	}
}

func TestRecordErrorWithMessage(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	_, span := StartSpan(ctx, TracerControlPlane, "test-operation")
	defer span.End()

	err := errors.New("test error")
	RecordErrorWithMessage(span, err, "custom message")
}

func TestAddEvent(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	_, span := StartSpan(ctx, TracerControlPlane, "test-operation")
	defer span.End()

	AddEvent(span, "test-event",
		StringAttr("event.key", "event.value"),
	)
}

func TestSetAttributes(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	_, span := StartSpan(ctx, TracerControlPlane, "test-operation")
	defer span.End()

	SetAttributes(span,
		StringAttr("key1", "value1"),
		IntAttr("key2", 123),
		BoolAttr("key3", true),
	)
}

func TestSpanFromContext(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	newCtx, span := StartSpan(ctx, TracerControlPlane, "test-operation")
	defer span.End()

	extractedSpan := SpanFromContext(newCtx)
	if extractedSpan == nil {
		t.Error("SpanFromContext returned nil")
	}

	// Verify it's the same span
	if extractedSpan.SpanContext().SpanID() != span.SpanContext().SpanID() {
		t.Error("SpanFromContext returned different span")
	}
}

func TestContextWithSpan(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	_, span := StartSpan(ctx, TracerControlPlane, "test-operation")
	defer span.End()

	newCtx := ContextWithSpan(context.Background(), span)

	extractedSpan := SpanFromContext(newCtx)
	if extractedSpan.SpanContext().SpanID() != span.SpanContext().SpanID() {
		t.Error("ContextWithSpan did not preserve span")
	}
}

func TestTraceID(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	newCtx, span := StartSpan(ctx, TracerControlPlane, "test-operation")
	defer span.End()

	traceID := TraceID(newCtx)
	if traceID == "" {
		t.Error("TraceID returned empty string")
	}

	// Test with context without span
	traceID = TraceID(context.Background())
	if traceID != "" {
		t.Error("TraceID should return empty string for context without span")
	}
}

func TestSpanID(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	newCtx, span := StartSpan(ctx, TracerControlPlane, "test-operation")
	defer span.End()

	spanID := SpanID(newCtx)
	if spanID == "" {
		t.Error("SpanID returned empty string")
	}

	// Test with context without span
	spanID = SpanID(context.Background())
	if spanID != "" {
		t.Error("SpanID should return empty string for context without span")
	}
}

func TestWithSpan(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	tests := []struct {
		name    string
		fn      func(context.Context, trace.Span) error
		wantErr bool
	}{
		{
			name: "success",
			fn: func(ctx context.Context, span trace.Span) error {
				SetAttributes(span, StringAttr("test", "value"))
				return nil
			},
			wantErr: false,
		},
		{
			name: "error",
			fn: func(ctx context.Context, span trace.Span) error {
				return errors.New("test error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WithSpan(context.Background(), TracerControlPlane, "test-operation", tt.fn)
			if (err != nil) != tt.wantErr {
				t.Errorf("WithSpan() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWithSpanAsync(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	done := make(chan bool)

	WithSpanAsync(context.Background(), TracerControlPlane, "test-operation", func(ctx context.Context, span trace.Span) {
		SetAttributes(span, StringAttr("async", "true"))
		done <- true
	})

	// Wait for async operation to complete
	<-done
}

func TestHelperFunctions(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()

	t.Run("StartControlPlaneSpan", func(t *testing.T) {
		_, span := StartControlPlaneSpan(ctx, "test-op")
		if span == nil {
			t.Error("StartControlPlaneSpan returned nil")
		}
		span.End()
	})

	t.Run("StartAgentSpan", func(t *testing.T) {
		_, span := StartAgentSpan(ctx, "test-op")
		if span == nil {
			t.Error("StartAgentSpan returned nil")
		}
		span.End()
	})

	t.Run("StartStateSpan", func(t *testing.T) {
		_, span := StartStateSpan(ctx, "test-op")
		if span == nil {
			t.Error("StartStateSpan returned nil")
		}
		span.End()
	})

	t.Run("StartExecutionSpan", func(t *testing.T) {
		_, span := StartExecutionSpan(ctx, "test-op")
		if span == nil {
			t.Error("StartExecutionSpan returned nil")
		}
		span.End()
	})

	t.Run("StartEventSpan", func(t *testing.T) {
		_, span := StartEventSpan(ctx, "test-op")
		if span == nil {
			t.Error("StartEventSpan returned nil")
		}
		span.End()
	})

	t.Run("StartPolicySpan", func(t *testing.T) {
		_, span := StartPolicySpan(ctx, "test-op")
		if span == nil {
			t.Error("StartPolicySpan returned nil")
		}
		span.End()
	})

	t.Run("StartGitOpsSpan", func(t *testing.T) {
		_, span := StartGitOpsSpan(ctx, "test-op")
		if span == nil {
			t.Error("StartGitOpsSpan returned nil")
		}
		span.End()
	})

	t.Run("StartNATSPublishSpan", func(t *testing.T) {
		_, span := StartNATSPublishSpan(ctx, "test.subject")
		if span == nil {
			t.Error("StartNATSPublishSpan returned nil")
		}
		span.End()
	})

	t.Run("StartNATSSubscribeSpan", func(t *testing.T) {
		_, span := StartNATSSubscribeSpan(ctx, "test.subject")
		if span == nil {
			t.Error("StartNATSSubscribeSpan returned nil")
		}
		span.End()
	})
}

func TestRecordMetric(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	_, span := StartSpan(ctx, TracerControlPlane, "test-operation")
	defer span.End()

	tests := []struct {
		name  string
		value interface{}
	}{
		{"int", 123},
		{"int64", int64(456)},
		{"float64", 78.9},
		{"string", "test"},
		{"bool", true},
		{"other", struct{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RecordMetric(span, tt.name, tt.value)
		})
	}
}

func TestRecordSuccess(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	_, span := StartSpan(ctx, TracerControlPlane, "test-operation")
	defer span.End()

	RecordSuccess(span, "operation completed successfully")
}

func TestIsTracingEnabled(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	t.Run("with span", func(t *testing.T) {
		ctx, span := StartSpan(context.Background(), TracerControlPlane, "test-operation")
		defer span.End()

		if !IsTracingEnabled(ctx) {
			t.Error("IsTracingEnabled should return true for context with span")
		}
	})

	t.Run("without span", func(t *testing.T) {
		if IsTracingEnabled(context.Background()) {
			t.Error("IsTracingEnabled should return false for context without span")
		}
	})
}

func TestAttributeHelpers(t *testing.T) {
	provider := setupTestProvider(t)
	defer provider.Shutdown(context.Background())

	ctx := context.Background()
	_, span := StartSpan(ctx, TracerControlPlane, "test-operation")
	defer span.End()

	t.Run("AgentAttrs", func(t *testing.T) {
		attrs := AgentAttrs("agent-123", "host-1", "web", "linux")
		if len(attrs) != 4 {
			t.Errorf("AgentAttrs returned %d attributes, want 4", len(attrs))
		}
		SetAttributes(span, attrs...)
	})

	t.Run("JobAttrs", func(t *testing.T) {
		attrs := JobAttrs("job-456", "echo test", "role:web", "completed")
		if len(attrs) != 4 {
			t.Errorf("JobAttrs returned %d attributes, want 4", len(attrs))
		}
		SetAttributes(span, attrs...)
	})

	t.Run("StateAttrs", func(t *testing.T) {
		attrs := StateAttrs("state-789", "/etc/nginx.conf", "file", "ensure")
		if len(attrs) != 4 {
			t.Errorf("StateAttrs returned %d attributes, want 4", len(attrs))
		}
		SetAttributes(span, attrs...)
	})

	t.Run("EventAttrs", func(t *testing.T) {
		attrs := EventAttrs("state.drift", "state-mgr", "warning", "corr-123")
		if len(attrs) != 4 {
			t.Errorf("EventAttrs returned %d attributes, want 4", len(attrs))
		}
		SetAttributes(span, attrs...)
	})

	t.Run("PolicyAttrs", func(t *testing.T) {
		attrs := PolicyAttrs("pol-001", "opa", "violated", 3)
		if len(attrs) != 4 {
			t.Errorf("PolicyAttrs returned %d attributes, want 4", len(attrs))
		}
		SetAttributes(span, attrs...)
	})
}
