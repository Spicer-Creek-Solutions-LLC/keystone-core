package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Correlation ID header names
const (
	// HeaderCorrelationID is the standard correlation ID header
	HeaderCorrelationID = "X-Correlation-ID"

	// HeaderRequestID is an alternative request ID header
	HeaderRequestID = "X-Request-ID"

	// HeaderTraceID is the trace ID header (W3C Trace Context)
	HeaderTraceID = "X-Trace-ID"

	// ContextKeyCorrelationID is the context key for correlation ID
	ContextKeyCorrelationID = contextKey("correlation_id")

	// ContextKeyRequestID is the context key for request ID
	ContextKeyRequestID = contextKey("request_id")

	// AttrCorrelationID is the attribute key for correlation ID
	AttrCorrelationID = "kscore.correlation_id"

	// AttrRequestID is the attribute key for request ID
	AttrRequestID = "kscore.request_id"
)

// contextKey is a type for context keys to avoid collisions
type contextKey string

// CorrelationConfig configures correlation ID handling
type CorrelationConfig struct {
	// GenerateIfMissing generates a new correlation ID if not present
	GenerateIfMissing bool `json:"generate_if_missing"`

	// PropagateToChildren propagates correlation ID to child spans
	PropagateToChildren bool `json:"propagate_to_children"`

	// IncludeInLogs includes correlation ID in log output
	IncludeInLogs bool `json:"include_in_logs"`

	// IncludeInMetrics includes correlation ID in metrics (caution: high cardinality)
	IncludeInMetrics bool `json:"include_in_metrics"`

	// HeaderNames are the headers to check for correlation ID (in order of precedence)
	HeaderNames []string `json:"header_names,omitempty"`

	// IDLength is the length of generated correlation IDs in bytes
	IDLength int `json:"id_length,omitempty"`

	// UseTraceIDAsCorrelation uses the trace ID as correlation ID
	UseTraceIDAsCorrelation bool `json:"use_trace_id_as_correlation"`
}

// DefaultCorrelationConfig returns sensible defaults
func DefaultCorrelationConfig() *CorrelationConfig {
	return &CorrelationConfig{
		GenerateIfMissing:       true,
		PropagateToChildren:     true,
		IncludeInLogs:           true,
		IncludeInMetrics:        false, // High cardinality
		HeaderNames:             []string{HeaderCorrelationID, HeaderRequestID, HeaderTraceID},
		IDLength:                16,
		UseTraceIDAsCorrelation: false,
	}
}

// CorrelationIDGenerator generates correlation IDs
type CorrelationIDGenerator struct {
	config *CorrelationConfig
	pool   sync.Pool
}

// NewCorrelationIDGenerator creates a new correlation ID generator
func NewCorrelationIDGenerator(config *CorrelationConfig) *CorrelationIDGenerator {
	if config == nil {
		config = DefaultCorrelationConfig()
	}
	if config.IDLength <= 0 {
		config.IDLength = 16
	}

	return &CorrelationIDGenerator{
		config: config,
		pool: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, config.IDLength)
				return &buf
			},
		},
	}
}

// Generate generates a new correlation ID
func (g *CorrelationIDGenerator) Generate() string {
	buf := g.pool.Get().(*[]byte)
	defer g.pool.Put(buf)

	if _, err := rand.Read(*buf); err != nil {
		// Fallback to timestamp-based ID
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000")))
	}

	return hex.EncodeToString(*buf)
}

// CorrelationContext holds correlation information
type CorrelationContext struct {
	// CorrelationID is the main correlation identifier
	CorrelationID string `json:"correlation_id"`

	// RequestID is an optional request-specific ID
	RequestID string `json:"request_id,omitempty"`

	// TraceID is the OpenTelemetry trace ID
	TraceID string `json:"trace_id,omitempty"`

	// SpanID is the current span ID
	SpanID string `json:"span_id,omitempty"`

	// ParentSpanID is the parent span ID
	ParentSpanID string `json:"parent_span_id,omitempty"`

	// Source identifies where the request originated
	Source string `json:"source,omitempty"`

	// StartTime is when the correlation context was created
	StartTime time.Time `json:"start_time"`

	// Metadata holds additional context information
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NewCorrelationContext creates a new correlation context
func NewCorrelationContext(correlationID string) *CorrelationContext {
	return &CorrelationContext{
		CorrelationID: correlationID,
		StartTime:     time.Now(),
		Metadata:      make(map[string]string),
	}
}

// WithRequestID adds a request ID
func (cc *CorrelationContext) WithRequestID(requestID string) *CorrelationContext {
	cc.RequestID = requestID
	return cc
}

// WithSource adds source information
func (cc *CorrelationContext) WithSource(source string) *CorrelationContext {
	cc.Source = source
	return cc
}

// WithMetadata adds metadata
func (cc *CorrelationContext) WithMetadata(key, value string) *CorrelationContext {
	if cc.Metadata == nil {
		cc.Metadata = make(map[string]string)
	}
	cc.Metadata[key] = value
	return cc
}

// UpdateFromSpan updates trace/span IDs from a span context
func (cc *CorrelationContext) UpdateFromSpan(ctx context.Context) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		cc.TraceID = span.SpanContext().TraceID().String()
		cc.SpanID = span.SpanContext().SpanID().String()
	}
}

// CorrelationPropagator handles correlation ID propagation
type CorrelationPropagator struct {
	config    *CorrelationConfig
	generator *CorrelationIDGenerator
}

// NewCorrelationPropagator creates a new correlation propagator
func NewCorrelationPropagator(config *CorrelationConfig) *CorrelationPropagator {
	if config == nil {
		config = DefaultCorrelationConfig()
	}

	return &CorrelationPropagator{
		config:    config,
		generator: NewCorrelationIDGenerator(config),
	}
}

// Extract extracts correlation ID from headers
func (p *CorrelationPropagator) Extract(headers http.Header) string {
	for _, name := range p.config.HeaderNames {
		if value := headers.Get(name); value != "" {
			return value
		}
	}
	return ""
}

// ExtractFromMap extracts correlation ID from a string map (e.g., NATS headers)
func (p *CorrelationPropagator) ExtractFromMap(headers map[string]string) string {
	for _, name := range p.config.HeaderNames {
		if value, ok := headers[name]; ok && value != "" {
			return value
		}
	}
	return ""
}

// Inject injects correlation ID into headers
func (p *CorrelationPropagator) Inject(headers http.Header, correlationID string) {
	if correlationID != "" {
		headers.Set(HeaderCorrelationID, correlationID)
	}
}

// InjectToMap injects correlation ID into a string map
func (p *CorrelationPropagator) InjectToMap(headers map[string]string, correlationID string) {
	if correlationID != "" {
		headers[HeaderCorrelationID] = correlationID
	}
}

// GetOrGenerate gets existing correlation ID or generates a new one
func (p *CorrelationPropagator) GetOrGenerate(headers http.Header) string {
	correlationID := p.Extract(headers)
	if correlationID == "" && p.config.GenerateIfMissing {
		correlationID = p.generator.Generate()
	}
	return correlationID
}

// GetOrGenerateFromMap gets existing correlation ID or generates a new one from map
func (p *CorrelationPropagator) GetOrGenerateFromMap(headers map[string]string) string {
	correlationID := p.ExtractFromMap(headers)
	if correlationID == "" && p.config.GenerateIfMissing {
		correlationID = p.generator.Generate()
	}
	return correlationID
}

// Generate generates a new correlation ID
func (p *CorrelationPropagator) Generate() string {
	return p.generator.Generate()
}

// Context functions for storing/retrieving correlation IDs

// WithCorrelationID adds a correlation ID to the context
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, ContextKeyCorrelationID, correlationID)
}

// CorrelationIDFromContext extracts correlation ID from context
func CorrelationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(ContextKeyCorrelationID).(string); ok {
		return id
	}
	return ""
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ContextKeyRequestID, requestID)
}

// RequestIDFromContext extracts request ID from context
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(ContextKeyRequestID).(string); ok {
		return id
	}
	return ""
}

// WithCorrelationContext adds a full correlation context
func WithCorrelationContext(ctx context.Context, cc *CorrelationContext) context.Context {
	ctx = WithCorrelationID(ctx, cc.CorrelationID)
	if cc.RequestID != "" {
		ctx = WithRequestID(ctx, cc.RequestID)
	}
	return ctx
}

// CorrelationContextFromContext builds a CorrelationContext from context
func CorrelationContextFromContext(ctx context.Context) *CorrelationContext {
	cc := &CorrelationContext{
		CorrelationID: CorrelationIDFromContext(ctx),
		RequestID:     RequestIDFromContext(ctx),
		StartTime:     time.Now(),
		Metadata:      make(map[string]string),
	}
	cc.UpdateFromSpan(ctx)
	return cc
}

// HTTP Middleware

// CorrelationMiddleware creates HTTP middleware for correlation ID handling
func CorrelationMiddleware(config *CorrelationConfig) func(http.Handler) http.Handler {
	propagator := NewCorrelationPropagator(config)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract or generate correlation ID
			correlationID := propagator.GetOrGenerate(r.Header)

			// Add to context
			ctx := WithCorrelationID(r.Context(), correlationID)

			// Check for request ID
			requestID := r.Header.Get(HeaderRequestID)
			if requestID != "" && requestID != correlationID {
				ctx = WithRequestID(ctx, requestID)
			}

			// Add to response headers
			w.Header().Set(HeaderCorrelationID, correlationID)
			if requestID != "" {
				w.Header().Set(HeaderRequestID, requestID)
			}

			// Continue with enriched context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CorrelationRoundTripper wraps an http.RoundTripper to propagate correlation IDs
type CorrelationRoundTripper struct {
	Base       http.RoundTripper
	Propagator *CorrelationPropagator
}

// NewCorrelationRoundTripper creates a new correlation-propagating round tripper
func NewCorrelationRoundTripper(base http.RoundTripper, config *CorrelationConfig) *CorrelationRoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &CorrelationRoundTripper{
		Base:       base,
		Propagator: NewCorrelationPropagator(config),
	}
}

// RoundTrip propagates correlation ID to outgoing requests
func (t *CorrelationRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	// Clone request to avoid modifying original
	reqClone := r.Clone(r.Context())

	// Get correlation ID from context or generate
	correlationID := CorrelationIDFromContext(r.Context())
	if correlationID == "" && t.Propagator.config.GenerateIfMissing {
		correlationID = t.Propagator.Generate()
	}

	// Inject into request headers
	if correlationID != "" {
		t.Propagator.Inject(reqClone.Header, correlationID)
	}

	// Also inject request ID if present
	requestID := RequestIDFromContext(r.Context())
	if requestID != "" {
		reqClone.Header.Set(HeaderRequestID, requestID)
	}

	return t.Base.RoundTrip(reqClone)
}

// NATS helpers

// NATSCorrelationHeaders creates correlation headers for NATS messages
type NATSCorrelationHeaders struct {
	CorrelationID string
	RequestID     string
	TraceID       string
	SpanID        string
}

// ToMap converts headers to a string map
func (h *NATSCorrelationHeaders) ToMap() map[string]string {
	m := make(map[string]string)
	if h.CorrelationID != "" {
		m[HeaderCorrelationID] = h.CorrelationID
	}
	if h.RequestID != "" {
		m[HeaderRequestID] = h.RequestID
	}
	if h.TraceID != "" {
		m[HeaderTraceID] = h.TraceID
	}
	return m
}

// FromMap populates headers from a string map
func (h *NATSCorrelationHeaders) FromMap(m map[string]string) {
	h.CorrelationID = m[HeaderCorrelationID]
	h.RequestID = m[HeaderRequestID]
	h.TraceID = m[HeaderTraceID]
}

// ExtractNATSCorrelation extracts correlation context from NATS message headers
func ExtractNATSCorrelation(headers map[string]string, config *CorrelationConfig) *CorrelationContext {
	propagator := NewCorrelationPropagator(config)
	correlationID := propagator.GetOrGenerateFromMap(headers)

	cc := NewCorrelationContext(correlationID)
	if requestID, ok := headers[HeaderRequestID]; ok {
		cc.RequestID = requestID
	}
	if traceID, ok := headers[HeaderTraceID]; ok {
		cc.TraceID = traceID
	}

	return cc
}

// InjectNATSCorrelation injects correlation context into NATS message headers
func InjectNATSCorrelation(ctx context.Context, headers map[string]string) {
	correlationID := CorrelationIDFromContext(ctx)
	if correlationID != "" {
		headers[HeaderCorrelationID] = correlationID
	}

	requestID := RequestIDFromContext(ctx)
	if requestID != "" {
		headers[HeaderRequestID] = requestID
	}

	// Also add trace ID if available
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		headers[HeaderTraceID] = span.SpanContext().TraceID().String()
	}
}

// Log helpers

// CorrelationLogFields returns log fields for correlation context
func CorrelationLogFields(ctx context.Context) map[string]string {
	fields := make(map[string]string)

	if correlationID := CorrelationIDFromContext(ctx); correlationID != "" {
		fields["correlation_id"] = correlationID
	}
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		fields["request_id"] = requestID
	}

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		fields["trace_id"] = span.SpanContext().TraceID().String()
		fields["span_id"] = span.SpanContext().SpanID().String()
	}

	return fields
}
