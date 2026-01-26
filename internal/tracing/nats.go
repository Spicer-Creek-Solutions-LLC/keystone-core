package tracing

import (
	"context"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// NATSCarrier adapts NATS message headers to OpenTelemetry's TextMapCarrier
// This allows trace context to be propagated through NATS messages
type NATSCarrier struct {
	msg *nats.Msg
}

// NewNATSCarrier creates a new NATS carrier for the given message
func NewNATSCarrier(msg *nats.Msg) *NATSCarrier {
	if msg.Header == nil {
		msg.Header = nats.Header{}
	}
	return &NATSCarrier{msg: msg}
}

// Get retrieves a value from the carrier
func (c *NATSCarrier) Get(key string) string {
	return c.msg.Header.Get(key)
}

// Set stores a value in the carrier
func (c *NATSCarrier) Set(key, value string) {
	c.msg.Header.Set(key, value)
}

// Keys returns all keys in the carrier
func (c *NATSCarrier) Keys() []string {
	keys := make([]string, 0, len(c.msg.Header))
	for k := range c.msg.Header {
		keys = append(keys, k)
	}
	return keys
}

// InjectTraceContext injects the trace context from ctx into the NATS message
// This should be called before publishing a message
func InjectTraceContext(ctx context.Context, msg *nats.Msg) {
	carrier := NewNATSCarrier(msg)
	propagator := propagation.TraceContext{}
	propagator.Inject(ctx, carrier)
}

// ExtractTraceContext extracts the trace context from the NATS message into a new context
// This should be called when receiving a message
func ExtractTraceContext(ctx context.Context, msg *nats.Msg) context.Context {
	carrier := NewNATSCarrier(msg)
	propagator := propagation.TraceContext{}
	return propagator.Extract(ctx, carrier)
}

// PublishWithTrace publishes a NATS message with trace context injected
// It also creates a span for the publish operation
func PublishWithTrace(ctx context.Context, nc *nats.Conn, subject string, data []byte) error {
	ctx, span := StartNATSPublishSpan(ctx, subject)
	defer span.End()

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}

	InjectTraceContext(ctx, msg)

	err := nc.PublishMsg(msg)
	if err != nil {
		RecordError(span, err)
		return err
	}

	return nil
}

// PublishMsgWithTrace publishes a NATS message with trace context injected
// Use this when you already have a nats.Msg constructed
func PublishMsgWithTrace(ctx context.Context, nc *nats.Conn, msg *nats.Msg) error {
	ctx, span := StartNATSPublishSpan(ctx, msg.Subject)
	defer span.End()

	InjectTraceContext(ctx, msg)

	err := nc.PublishMsg(msg)
	if err != nil {
		RecordError(span, err)
		return err
	}

	return nil
}

// RequestWithTrace makes a NATS request with trace context
func RequestWithTrace(ctx context.Context, nc *nats.Conn, subject string, data []byte) (*nats.Msg, error) {
	ctx, span := StartSpanWithKind(ctx, TracerNATS, SpanNATSRequest, SpanKindClient,
		StringAttr(AttrNATSSubject, subject),
	)
	defer span.End()

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}

	InjectTraceContext(ctx, msg)

	reply, err := nc.RequestMsg(msg, nats.DefaultTimeout)
	if err != nil {
		RecordError(span, err)
		return nil, err
	}

	// Extract trace context from reply (if any)
	if reply.Header != nil {
		ctx = ExtractTraceContext(ctx, reply)
	}

	return reply, nil
}

// HandleMessageWithTrace wraps a NATS message handler to extract trace context
// and create a span for message processing
func HandleMessageWithTrace(tracerName, spanName string, handler func(context.Context, *nats.Msg)) func(*nats.Msg) {
	return func(msg *nats.Msg) {
		// Extract trace context from message
		ctx := context.Background()
		ctx = ExtractTraceContext(ctx, msg)

		// Start a span for message processing
		ctx, span := StartNATSSubscribeSpan(ctx, msg.Subject)
		defer span.End()

		// Call the actual handler
		handler(ctx, msg)
	}
}

// SubscribeWithTrace subscribes to a NATS subject with automatic trace context extraction
func SubscribeWithTrace(nc *nats.Conn, subject string, handler func(context.Context, *nats.Msg)) (*nats.Subscription, error) {
	wrappedHandler := HandleMessageWithTrace(TracerNATS, SpanNATSSubscribe, handler)
	return nc.Subscribe(subject, wrappedHandler)
}

// QueueSubscribeWithTrace subscribes to a NATS subject with a queue group and automatic trace context extraction
func QueueSubscribeWithTrace(nc *nats.Conn, subject, queue string, handler func(context.Context, *nats.Msg)) (*nats.Subscription, error) {
	wrappedHandler := HandleMessageWithTrace(TracerNATS, SpanNATSSubscribe, handler)
	return nc.QueueSubscribe(subject, queue, wrappedHandler)
}

// JetStreamPublishWithTrace publishes to a JetStream stream with trace context
func JetStreamPublishWithTrace(ctx context.Context, js nats.JetStreamContext, subject string, data []byte) (*nats.PubAck, error) {
	ctx, span := StartNATSPublishSpan(ctx, subject,
		StringAttr(AttrNATSStream, "jetstream"),
	)
	defer span.End()

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}

	InjectTraceContext(ctx, msg)

	ack, err := js.PublishMsg(msg)
	if err != nil {
		RecordError(span, err)
		return nil, err
	}

	// Record stream and sequence info
	if ack != nil {
		SetAttributes(span,
			StringAttr("nats.stream", ack.Stream),
			Int64Attr("nats.sequence", int64(ack.Sequence)),
		)
	}

	return ack, nil
}

// JetStreamSubscribeWithTrace subscribes to a JetStream subject with automatic trace context extraction
func JetStreamSubscribeWithTrace(js nats.JetStreamContext, subject string, handler func(context.Context, *nats.Msg), opts ...nats.SubOpt) (*nats.Subscription, error) {
	wrappedHandler := HandleMessageWithTrace(TracerNATS, SpanNATSSubscribe, func(ctx context.Context, msg *nats.Msg) {
		// Add JetStream-specific metadata
		meta, err := msg.Metadata()
		if err == nil {
			span := trace.SpanFromContext(ctx)
			SetAttributes(span,
				StringAttr(AttrNATSStream, meta.Stream),
				StringAttr(AttrNATSConsumer, meta.Consumer),
				Int64Attr("nats.sequence.stream", int64(meta.Sequence.Stream)),
				Int64Attr("nats.sequence.consumer", int64(meta.Sequence.Consumer)),
			)
		}

		handler(ctx, msg)
	})

	return js.Subscribe(subject, wrappedHandler, opts...)
}

// PropagateTraceHeaders is a utility to copy trace headers between messages
// Useful when forwarding or transforming messages
func PropagateTraceHeaders(from, to *nats.Msg) {
	if from.Header == nil {
		return
	}

	if to.Header == nil {
		to.Header = nats.Header{}
	}

	// Copy all traceparent and tracestate headers
	if traceparent := from.Header.Get("traceparent"); traceparent != "" {
		to.Header.Set("traceparent", traceparent)
	}
	if tracestate := from.Header.Get("tracestate"); tracestate != "" {
		to.Header.Set("tracestate", tracestate)
	}
}
