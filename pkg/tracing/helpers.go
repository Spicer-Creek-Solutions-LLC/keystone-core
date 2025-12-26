package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// StartSpan starts a new span with the given name and options
// Returns the span and a context containing the span
func StartSpan(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.Tracer(tracerName)
	return tracer.Start(ctx, spanName, opts...)
}

// StartSpanWithKind starts a new span with the given name and kind
func StartSpanWithKind(ctx context.Context, tracerName, spanName string, kind SpanKind, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	opts := []trace.SpanStartOption{
		trace.WithSpanKind(kind),
	}
	if len(attrs) > 0 {
		opts = append(opts, trace.WithAttributes(attrs...))
	}
	return StartSpan(ctx, tracerName, spanName, opts...)
}

// EndSpan ends the span and records an error if err is not nil
func EndSpan(span trace.Span, err error) {
	if err != nil {
		RecordError(span, err)
	}
	span.End()
}

// EndSpanWithStatus ends the span with the given status
func EndSpanWithStatus(span trace.Span, status codes.Code, description string) {
	span.SetStatus(status, description)
	span.End()
}

// RecordError records an error on the span
func RecordError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// RecordErrorWithMessage records an error with a custom message
func RecordErrorWithMessage(span trace.Span, err error, message string) {
	if err == nil {
		return
	}
	span.RecordError(err, trace.WithAttributes(
		attribute.String("error.message", message),
	))
	span.SetStatus(codes.Error, message)
}

// AddEvent adds an event to the span
func AddEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttributes sets attributes on the span
func SetAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

// SpanFromContext extracts the span from the context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// ContextWithSpan returns a new context with the span
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

// TraceID returns the trace ID from the context as a string
func TraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().TraceID().String()
}

// SpanID returns the span ID from the context as a string
func SpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().SpanID().String()
}

// WithSpan executes a function within a span
// Automatically handles span creation, error recording, and cleanup
func WithSpan(ctx context.Context, tracerName, spanName string, fn func(context.Context, trace.Span) error, opts ...trace.SpanStartOption) error {
	ctx, span := StartSpan(ctx, tracerName, spanName, opts...)
	defer span.End()

	err := fn(ctx, span)
	if err != nil {
		RecordError(span, err)
	}

	return err
}

// WithSpanAsync executes a function asynchronously within a span
// Useful for background tasks that should be traced
func WithSpanAsync(ctx context.Context, tracerName, spanName string, fn func(context.Context, trace.Span), opts ...trace.SpanStartOption) {
	ctx, span := StartSpan(ctx, tracerName, spanName, opts...)

	go func() {
		defer span.End()
		fn(ctx, span)
	}()
}

// Tracer names for different TitanAnvil components
const (
	TracerControlPlane  = "titananvil.controlplane"
	TracerAgent         = "titananvil.agent"
	TracerState         = "titananvil.state"
	TracerExecution     = "titananvil.execution"
	TracerEvents        = "titananvil.events"
	TracerPolicy        = "titananvil.policy"
	TracerGitOps        = "titananvil.gitops"
	TracerNATS          = "titananvil.nats"
)

// Common span operation names
const (
	// Control Plane operations
	SpanAPIRequest         = "api.request"
	SpanAgentConnect       = "agent.connect"
	SpanAgentDisconnect    = "agent.disconnect"
	SpanAgentHeartbeat     = "agent.heartbeat"
	SpanCommandDispatch    = "command.dispatch"
	SpanCommandExecute     = "command.execute"

	// State management operations
	SpanStateExecute       = "state.execute"
	SpanStateApply         = "state.apply"
	SpanStateCheck         = "state.check"
	SpanStateDrift         = "state.drift"
	SpanStateModule        = "state.module"

	// Event operations
	SpanEventPublish       = "event.publish"
	SpanEventSubscribe     = "event.subscribe"
	SpanEventProcess       = "event.process"
	SpanReactorExecute     = "reactor.execute"

	// Policy operations
	SpanPolicyEvaluate     = "policy.evaluate"
	SpanPolicyEnforce      = "policy.enforce"
	SpanPolicyRemediate    = "policy.remediate"

	// GitOps operations
	SpanGitOpsWebhook      = "gitops.webhook"
	SpanGitOpsVerify       = "gitops.verify"
	SpanGitOpsRollback     = "gitops.rollback"
	SpanGitOpsSync         = "gitops.sync"

	// NATS operations
	SpanNATSPublish        = "nats.publish"
	SpanNATSSubscribe      = "nats.subscribe"
	SpanNATSRequest        = "nats.request"
)

// Helper functions for common operations

// StartControlPlaneSpan starts a span for control plane operations
func StartControlPlaneSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return StartSpanWithKind(ctx, TracerControlPlane, operation, SpanKindServer, attrs...)
}

// StartAgentSpan starts a span for agent operations
func StartAgentSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return StartSpanWithKind(ctx, TracerAgent, operation, SpanKindInternal, attrs...)
}

// StartStateSpan starts a span for state management operations
func StartStateSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return StartSpanWithKind(ctx, TracerState, operation, SpanKindInternal, attrs...)
}

// StartExecutionSpan starts a span for execution operations
func StartExecutionSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return StartSpanWithKind(ctx, TracerExecution, operation, SpanKindInternal, attrs...)
}

// StartEventSpan starts a span for event operations
func StartEventSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return StartSpanWithKind(ctx, TracerEvents, operation, SpanKindInternal, attrs...)
}

// StartPolicySpan starts a span for policy operations
func StartPolicySpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return StartSpanWithKind(ctx, TracerPolicy, operation, SpanKindInternal, attrs...)
}

// StartGitOpsSpan starts a span for GitOps operations
func StartGitOpsSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return StartSpanWithKind(ctx, TracerGitOps, operation, SpanKindInternal, attrs...)
}

// StartNATSPublishSpan starts a span for NATS publish operations
func StartNATSPublishSpan(ctx context.Context, subject string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	allAttrs := append([]attribute.KeyValue{
		StringAttr(AttrNATSSubject, subject),
	}, attrs...)
	return StartSpanWithKind(ctx, TracerNATS, SpanNATSPublish, SpanKindProducer, allAttrs...)
}

// StartNATSSubscribeSpan starts a span for NATS subscribe operations
func StartNATSSubscribeSpan(ctx context.Context, subject string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	allAttrs := append([]attribute.KeyValue{
		StringAttr(AttrNATSSubject, subject),
	}, attrs...)
	return StartSpanWithKind(ctx, TracerNATS, SpanNATSSubscribe, SpanKindConsumer, allAttrs...)
}

// RecordMetric records a metric value as a span event
func RecordMetric(span trace.Span, name string, value interface{}) {
	var attr attribute.KeyValue
	switch v := value.(type) {
	case int:
		attr = IntAttr("value", v)
	case int64:
		attr = Int64Attr("value", v)
	case float64:
		attr = Float64Attr("value", v)
	case string:
		attr = StringAttr("value", v)
	case bool:
		attr = BoolAttr("value", v)
	default:
		attr = StringAttr("value", fmt.Sprintf("%v", v))
	}

	AddEvent(span, fmt.Sprintf("metric.%s", name), attr)
}

// RecordSuccess marks the span as successful
func RecordSuccess(span trace.Span, message string) {
	span.SetStatus(codes.Ok, message)
}

// IsTracingEnabled returns true if a valid span exists in the context
func IsTracingEnabled(ctx context.Context) bool {
	span := trace.SpanFromContext(ctx)
	return span.SpanContext().IsValid()
}
