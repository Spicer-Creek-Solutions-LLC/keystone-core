package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracerProvider creates and manages tracers
type TracerProvider interface {
	// Tracer returns a tracer for the given instrumentation scope
	Tracer(name string, opts ...trace.TracerOption) trace.Tracer

	// Shutdown flushes any remaining spans and shuts down the provider
	Shutdown(ctx context.Context) error
}

// Config defines the tracing configuration
type Config struct {
	// Enabled controls whether tracing is enabled
	Enabled bool

	// ServiceName is the name of the service being traced
	ServiceName string

	// ServiceVersion is the version of the service
	ServiceVersion string

	// Environment is the deployment environment (dev, staging, prod)
	Environment string

	// Sampling configuration
	Sampling SamplingConfig

	// Exporters to use
	Exporters []ExporterConfig

	// Resource attributes to add to all spans
	ResourceAttributes map[string]string
}

// SamplingConfig defines trace sampling behavior
type SamplingConfig struct {
	// Type is the sampling strategy type
	Type SamplingType

	// Rate is the sampling rate (0.0 to 1.0)
	// Only used for probabilistic sampling
	Rate float64

	// AlwaysSample is a list of span names to always sample
	AlwaysSample []string

	// NeverSample is a list of span names to never sample
	NeverSample []string
}

// SamplingType represents different sampling strategies
type SamplingType string

const (
	// SamplingAlwaysOn samples all traces
	SamplingAlwaysOn SamplingType = "always_on"

	// SamplingAlwaysOff samples no traces
	SamplingAlwaysOff SamplingType = "always_off"

	// SamplingProbabilistic samples traces based on a probability
	SamplingProbabilistic SamplingType = "probabilistic"

	// SamplingParentBased inherits sampling decision from parent span
	SamplingParentBased SamplingType = "parent_based"

	// SamplingRateLimiting limits the number of traces per second
	SamplingRateLimiting SamplingType = "rate_limiting"
)

// ExporterConfig defines configuration for a trace exporter
type ExporterConfig struct {
	// Type is the exporter type
	Type ExporterType

	// Endpoint is the exporter endpoint (e.g., "localhost:4317")
	Endpoint string

	// Insecure controls whether to use insecure connections
	Insecure bool

	// Headers are additional headers to send with traces
	Headers map[string]string

	// Timeout for sending traces
	Timeout time.Duration

	// Compression to use (gzip, none)
	Compression string

	// BatchTimeout is the maximum time to wait before sending a batch
	BatchTimeout time.Duration

	// MaxExportBatchSize is the maximum batch size
	MaxExportBatchSize int

	// MaxQueueSize is the maximum queue size
	MaxQueueSize int
}

// ExporterType represents different trace exporter types
type ExporterType string

const (
	// ExporterOTLP exports to OTLP (OpenTelemetry Protocol) endpoints
	// Compatible with Jaeger, Tempo, etc.
	ExporterOTLP ExporterType = "otlp"

	// ExporterOTLPHTTP exports to OTLP over HTTP
	ExporterOTLPHTTP ExporterType = "otlp_http"

	// ExporterZipkin exports to Zipkin
	ExporterZipkin ExporterType = "zipkin"

	// ExporterStdout exports to stdout (for debugging)
	ExporterStdout ExporterType = "stdout"

	// ExporterNone disables export
	ExporterNone ExporterType = "none"
)

// SpanKind represents the kind of span
type SpanKind = trace.SpanKind

const (
	// SpanKindInternal represents an internal operation
	SpanKindInternal = trace.SpanKindInternal

	// SpanKindServer represents a server handling a request
	SpanKindServer = trace.SpanKindServer

	// SpanKindClient represents a client making a request
	SpanKindClient = trace.SpanKindClient

	// SpanKindProducer represents a producer sending a message
	SpanKindProducer = trace.SpanKindProducer

	// SpanKindConsumer represents a consumer receiving a message
	SpanKindConsumer = trace.SpanKindConsumer
)

// SpanStatus represents the status of a span
type SpanStatus = codes.Code

const (
	// StatusUnset indicates the span status is unset
	StatusUnset = codes.Unset

	// StatusError indicates the span encountered an error
	StatusError = codes.Error

	// StatusOK indicates the span completed successfully
	StatusOK = codes.Ok
)

// Common attribute keys used across Keystone Core
const (
	// Agent attributes
	AttrAgentID       = "kscore.agent.id"
	AttrAgentHostname = "kscore.agent.hostname"
	AttrAgentRole     = "kscore.agent.role"
	AttrAgentOS       = "kscore.agent.os"

	// Command/Job attributes
	AttrJobID        = "kscore.job.id"
	AttrJobCommand   = "kscore.job.command"
	AttrJobTarget    = "kscore.job.target"
	AttrJobStatus    = "kscore.job.status"
	AttrJobExitCode  = "kscore.job.exit_code"
	AttrJobDuration  = "kscore.job.duration_ms"

	// State management attributes
	AttrStateID           = "kscore.state.id"
	AttrStateResource     = "kscore.state.resource"
	AttrStateModule       = "kscore.state.module"
	AttrStateAction       = "kscore.state.action"
	AttrStateChanged      = "kscore.state.changed"
	AttrStateDrift        = "kscore.state.drift"
	AttrStateDriftSeverity = "kscore.state.drift_severity"

	// Event attributes
	AttrEventType         = "kscore.event.type"
	AttrEventSource       = "kscore.event.source"
	AttrEventSeverity     = "kscore.event.severity"
	AttrEventCorrelationID = "kscore.event.correlation_id"

	// Policy attributes
	AttrPolicyID       = "kscore.policy.id"
	AttrPolicyType     = "kscore.policy.type"
	AttrPolicyResult   = "kscore.policy.result"
	AttrPolicyViolations = "kscore.policy.violations"

	// GitOps attributes
	AttrGitOpsSource      = "kscore.gitops.source"
	AttrGitOpsApplication = "kscore.gitops.application"
	AttrGitOpsRevision    = "kscore.gitops.revision"
	AttrGitOpsStatus      = "kscore.gitops.status"

	// NATS attributes
	AttrNATSSubject = "kscore.nats.subject"
	AttrNATSStream  = "kscore.nats.stream"
	AttrNATSConsumer = "kscore.nats.consumer"
)

// Helper functions for creating attributes

// StringAttr creates a string attribute
func StringAttr(key, value string) attribute.KeyValue {
	return attribute.String(key, value)
}

// IntAttr creates an int attribute
func IntAttr(key string, value int) attribute.KeyValue {
	return attribute.Int(key, value)
}

// Int64Attr creates an int64 attribute
func Int64Attr(key string, value int64) attribute.KeyValue {
	return attribute.Int64(key, value)
}

// BoolAttr creates a bool attribute
func BoolAttr(key string, value bool) attribute.KeyValue {
	return attribute.Bool(key, value)
}

// Float64Attr creates a float64 attribute
func Float64Attr(key string, value float64) attribute.KeyValue {
	return attribute.Float64(key, value)
}

// StringSliceAttr creates a string slice attribute
func StringSliceAttr(key string, value []string) attribute.KeyValue {
	return attribute.StringSlice(key, value)
}

// AgentAttrs creates common agent attributes
func AgentAttrs(id, hostname, role, os string) []attribute.KeyValue {
	return []attribute.KeyValue{
		StringAttr(AttrAgentID, id),
		StringAttr(AttrAgentHostname, hostname),
		StringAttr(AttrAgentRole, role),
		StringAttr(AttrAgentOS, os),
	}
}

// JobAttrs creates common job attributes
func JobAttrs(jobID, command, target, status string) []attribute.KeyValue {
	return []attribute.KeyValue{
		StringAttr(AttrJobID, jobID),
		StringAttr(AttrJobCommand, command),
		StringAttr(AttrJobTarget, target),
		StringAttr(AttrJobStatus, status),
	}
}

// StateAttrs creates common state attributes
func StateAttrs(stateID, resource, module, action string) []attribute.KeyValue {
	return []attribute.KeyValue{
		StringAttr(AttrStateID, stateID),
		StringAttr(AttrStateResource, resource),
		StringAttr(AttrStateModule, module),
		StringAttr(AttrStateAction, action),
	}
}

// EventAttrs creates common event attributes
func EventAttrs(eventType, source, severity, correlationID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		StringAttr(AttrEventType, eventType),
		StringAttr(AttrEventSource, source),
		StringAttr(AttrEventSeverity, severity),
		StringAttr(AttrEventCorrelationID, correlationID),
	}
}

// PolicyAttrs creates common policy attributes
func PolicyAttrs(policyID, policyType, result string, violations int) []attribute.KeyValue {
	return []attribute.KeyValue{
		StringAttr(AttrPolicyID, policyID),
		StringAttr(AttrPolicyType, policyType),
		StringAttr(AttrPolicyResult, result),
		IntAttr(AttrPolicyViolations, violations),
	}
}
