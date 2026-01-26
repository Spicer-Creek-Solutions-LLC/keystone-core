package events

import (
	"context"
	"fmt"
	"sync"
)

// Enricher enriches an event with additional context
type Enricher interface {
	// Enrich adds context to an event
	Enrich(ctx context.Context, event *Event) error

	// Name returns the enricher's name for logging/metrics
	Name() string
}

// EnrichmentPipeline processes events through a series of enrichers
type EnrichmentPipeline struct {
	enrichers []Enricher
	mu        sync.RWMutex

	// Optional error handler
	errorHandler func(enricherName string, event *Event, err error)
}

// NewEnrichmentPipeline creates a new enrichment pipeline
func NewEnrichmentPipeline(enrichers ...Enricher) *EnrichmentPipeline {
	return &EnrichmentPipeline{
		enrichers: enrichers,
	}
}

// AddEnricher adds an enricher to the pipeline
func (p *EnrichmentPipeline) AddEnricher(enricher Enricher) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enrichers = append(p.enrichers, enricher)
}

// RemoveEnricher removes an enricher by name
func (p *EnrichmentPipeline) RemoveEnricher(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, enricher := range p.enrichers {
		if enricher.Name() == name {
			p.enrichers = append(p.enrichers[:i], p.enrichers[i+1:]...)
			return true
		}
	}
	return false
}

// Enrich processes an event through all enrichers
func (p *EnrichmentPipeline) Enrich(ctx context.Context, event *Event) error {
	p.mu.RLock()
	enrichers := make([]Enricher, len(p.enrichers))
	copy(enrichers, p.enrichers)
	p.mu.RUnlock()

	for _, enricher := range enrichers {
		if err := enricher.Enrich(ctx, event); err != nil {
			if p.errorHandler != nil {
				p.errorHandler(enricher.Name(), event, err)
			}
			// Continue with other enrichers even if one fails
		}
	}

	return nil
}

// SetErrorHandler sets a custom error handler for enrichment failures
func (p *EnrichmentPipeline) SetErrorHandler(handler func(enricherName string, event *Event, err error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errorHandler = handler
}

// GetEnrichers returns a copy of the enrichers list
func (p *EnrichmentPipeline) GetEnrichers() []Enricher {
	p.mu.RLock()
	defer p.mu.RUnlock()

	enrichers := make([]Enricher, len(p.enrichers))
	copy(enrichers, p.enrichers)
	return enrichers
}

// ContextEnricher adds context from a context.Context to the event
type ContextEnricher struct {
	name string
	// Keys to extract from context (key -> field name mapping)
	contextKeys map[interface{}]string
}

// NewContextEnricher creates a new context enricher
// Each pair should be: contextKey (interface{}), fieldName (string)
func NewContextEnricher(name string, keys ...interface{}) *ContextEnricher {
	contextKeys := make(map[interface{}]string)

	// Parse key pairs: key, name, key, name, ...
	for i := 0; i < len(keys); i += 2 {
		if i+1 < len(keys) {
			key := keys[i]
			fieldName, ok := keys[i+1].(string)
			if ok {
				contextKeys[key] = fieldName
			}
		}
	}

	return &ContextEnricher{
		name:        name,
		contextKeys: contextKeys,
	}
}

func (e *ContextEnricher) Name() string {
	return e.name
}

func (e *ContextEnricher) Enrich(ctx context.Context, event *Event) error {
	if ctx == nil {
		return nil
	}

	for key, fieldName := range e.contextKeys {
		if value := ctx.Value(key); value != nil {
			event.Data[fieldName] = value
		}
	}

	return nil
}

// TagEnricher adds static tags to events
type TagEnricher struct {
	name string
	tags map[string]string
}

// NewTagEnricher creates a new tag enricher
func NewTagEnricher(name string, tags map[string]string) *TagEnricher {
	return &TagEnricher{
		name: name,
		tags: tags,
	}
}

func (e *TagEnricher) Name() string {
	return e.name
}

func (e *TagEnricher) Enrich(ctx context.Context, event *Event) error {
	for key, value := range e.tags {
		event.Tags[key] = value
	}
	return nil
}

// DataEnricher adds static data fields to events
type DataEnricher struct {
	name string
	data map[string]interface{}
}

// NewDataEnricher creates a new data enricher
func NewDataEnricher(name string, data map[string]interface{}) *DataEnricher {
	return &DataEnricher{
		name: name,
		data: data,
	}
}

func (e *DataEnricher) Name() string {
	return e.name
}

func (e *DataEnricher) Enrich(ctx context.Context, event *Event) error {
	for key, value := range e.data {
		event.Data[key] = value
	}
	return nil
}

// FunctionEnricher uses a custom function to enrich events
type FunctionEnricher struct {
	name     string
	enrichFn func(ctx context.Context, event *Event) error
}

// NewFunctionEnricher creates a new function-based enricher
func NewFunctionEnricher(name string, enrichFn func(ctx context.Context, event *Event) error) *FunctionEnricher {
	return &FunctionEnricher{
		name:     name,
		enrichFn: enrichFn,
	}
}

func (e *FunctionEnricher) Name() string {
	return e.name
}

func (e *FunctionEnricher) Enrich(ctx context.Context, event *Event) error {
	return e.enrichFn(ctx, event)
}

// ConditionalEnricher only enriches events matching a filter
type ConditionalEnricher struct {
	name     string
	filter   FilterExpression
	enricher Enricher
}

// NewConditionalEnricher creates a new conditional enricher
func NewConditionalEnricher(name string, filter FilterExpression, enricher Enricher) *ConditionalEnricher {
	return &ConditionalEnricher{
		name:     name,
		filter:   filter,
		enricher: enricher,
	}
}

func (e *ConditionalEnricher) Name() string {
	return e.name
}

func (e *ConditionalEnricher) Enrich(ctx context.Context, event *Event) error {
	if e.filter.Matches(event) {
		return e.enricher.Enrich(ctx, event)
	}
	return nil
}

// TimestampEnricher adds additional timestamp fields
type TimestampEnricher struct {
	name string
	// Fields to add (e.g., "enriched_at", "processed_at")
	fields []string
}

// NewTimestampEnricher creates a new timestamp enricher
func NewTimestampEnricher(name string, fields ...string) *TimestampEnricher {
	return &TimestampEnricher{
		name:   name,
		fields: fields,
	}
}

func (e *TimestampEnricher) Name() string {
	return e.name
}

func (e *TimestampEnricher) Enrich(ctx context.Context, event *Event) error {
	for _, field := range e.fields {
		event.Data[field] = event.Time
	}
	return nil
}

// HostnameEnricher adds hostname information
type HostnameEnricher struct {
	name     string
	hostname string
	field    string // Field name to use (default: "enriched_by_host")
}

// NewHostnameEnricher creates a new hostname enricher
func NewHostnameEnricher(name, hostname string) *HostnameEnricher {
	return &HostnameEnricher{
		name:     name,
		hostname: hostname,
		field:    "enriched_by_host",
	}
}

// SetField sets the field name for the hostname
func (e *HostnameEnricher) SetField(field string) *HostnameEnricher {
	e.field = field
	return e
}

func (e *HostnameEnricher) Name() string {
	return e.name
}

func (e *HostnameEnricher) Enrich(ctx context.Context, event *Event) error {
	event.Data[e.field] = e.hostname
	return nil
}

// SequenceNumberEnricher adds sequence numbers to events
type SequenceNumberEnricher struct {
	name     string
	field    string // Field name (default: "sequence")
	mu       sync.Mutex
	sequence uint64
}

// NewSequenceNumberEnricher creates a new sequence number enricher
func NewSequenceNumberEnricher(name string) *SequenceNumberEnricher {
	return &SequenceNumberEnricher{
		name:  name,
		field: "sequence",
	}
}

// SetField sets the field name for the sequence number
func (e *SequenceNumberEnricher) SetField(field string) *SequenceNumberEnricher {
	e.field = field
	return e
}

func (e *SequenceNumberEnricher) Name() string {
	return e.name
}

func (e *SequenceNumberEnricher) Enrich(ctx context.Context, event *Event) error {
	e.mu.Lock()
	e.sequence++
	seq := e.sequence
	e.mu.Unlock()

	event.Data[e.field] = seq
	return nil
}

// ChainEnrichers chains multiple enrichers into one
func ChainEnrichers(name string, enrichers ...Enricher) Enricher {
	return NewFunctionEnricher(name, func(ctx context.Context, event *Event) error {
		for _, enricher := range enrichers {
			if err := enricher.Enrich(ctx, event); err != nil {
				return fmt.Errorf("%s: %w", enricher.Name(), err)
			}
		}
		return nil
	})
}

// EnrichedPublisher wraps a publisher with an enrichment pipeline
type EnrichedPublisher struct {
	publisher EventPublisher
	pipeline  *EnrichmentPipeline
	ctx       context.Context
}

// NewEnrichedPublisher creates a publisher that enriches events before publishing
func NewEnrichedPublisher(publisher EventPublisher, pipeline *EnrichmentPipeline) *EnrichedPublisher {
	return &EnrichedPublisher{
		publisher: publisher,
		pipeline:  pipeline,
		ctx:       context.Background(),
	}
}

// SetContext sets the context for enrichment
func (p *EnrichedPublisher) SetContext(ctx context.Context) {
	p.ctx = ctx
}

func (p *EnrichedPublisher) Publish(event *Event) error {
	// Enrich event
	if err := p.pipeline.Enrich(p.ctx, event); err != nil {
		return fmt.Errorf("enrichment failed: %w", err)
	}

	// Publish enriched event
	return p.publisher.Publish(event)
}

func (p *EnrichedPublisher) PublishAsync(event *Event) error {
	// Enrich event
	if err := p.pipeline.Enrich(p.ctx, event); err != nil {
		return fmt.Errorf("enrichment failed: %w", err)
	}

	// Publish enriched event
	return p.publisher.PublishAsync(event)
}

func (p *EnrichedPublisher) Close() error {
	return p.publisher.Close()
}
