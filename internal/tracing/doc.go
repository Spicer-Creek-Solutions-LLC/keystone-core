// Package tracing builds the OpenTelemetry TracerProvider that the rest
// of Keystone Core writes spans through.
//
// New returns a [*Provider] wrapping an OTel SDK TracerProvider when
// the supplied TracingConfig has Enabled=true, or a noop wrapper
// otherwise. Either way the returned provider exposes:
//
//   - TracerProvider() trace.TracerProvider — the value to pass to
//     otel.SetTracerProvider in boot wiring.
//   - Shutdown(ctx) error — flushes the batch processor and closes the
//     exporter inside ctx's deadline.
//
// Samplers honour the v1.0 enum (always_on, always_off, probabilistic,
// parent_based, rate_limiting). Adaptive sampling is deferred to v2.x+
// (epic 17 Scope-out / ROADMAP "Adaptive sampling tied to error
// metrics").
//
// Exporters cover OTLP over gRPC and HTTP, Zipkin, and stdout. stdout
// is the v1.0 default — operators can opt in without standing up a
// collector first.
//
// Note: the upstream go.opentelemetry.io/otel/exporters/zipkin module
// is marked deprecated with planned removal in early 2027. Epic 17's
// v1.0 metric list still names Zipkin, so we ship it; a follow-up will
// switch operators to OTLP before the upstream removal lands.
//
// No globals: the package never touches otel.SetTracerProvider or
// otel.SetTextMapPropagator. Boot wiring (task 10) chooses when to
// install the provider process-wide.
package tracing
