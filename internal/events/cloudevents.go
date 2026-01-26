package events

import (
	"encoding/json"
	"fmt"
	"time"
)

// CloudEvent represents a CloudEvents 1.0 compliant event
// Spec: https://github.com/cloudevents/spec/blob/v1.0/spec.md
type CloudEvent struct {
	// REQUIRED attributes
	ID              string    `json:"id"`
	Source          string    `json:"source"`
	SpecVersion     string    `json:"specversion"`
	Type            string    `json:"type"`

	// OPTIONAL attributes
	DataContentType string                 `json:"datacontenttype,omitempty"`
	DataSchema      string                 `json:"dataschema,omitempty"`
	Subject         string                 `json:"subject,omitempty"`
	Time            *time.Time             `json:"time,omitempty"`
	Data            interface{}            `json:"data,omitempty"`

	// Extension attributes (custom fields)
	Extensions      map[string]interface{} `json:"-"`
}

// MarshalJSON implements custom JSON marshaling to include extensions
func (ce *CloudEvent) MarshalJSON() ([]byte, error) {
	type Alias CloudEvent
	aux := struct {
		*Alias
	}{
		Alias: (*Alias)(ce),
	}

	// Marshal the base struct
	baseJSON, err := json.Marshal(aux)
	if err != nil {
		return nil, err
	}

	// If no extensions, return base JSON
	if len(ce.Extensions) == 0 {
		return baseJSON, nil
	}

	// Merge extensions into the JSON
	var base map[string]interface{}
	if err := json.Unmarshal(baseJSON, &base); err != nil {
		return nil, err
	}

	for k, v := range ce.Extensions {
		base[k] = v
	}

	return json.Marshal(base)
}

// UnmarshalJSON implements custom JSON unmarshaling to extract extensions
func (ce *CloudEvent) UnmarshalJSON(data []byte) error {
	type Alias CloudEvent
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(ce),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Extract all fields
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Remove standard CloudEvents attributes
	standardAttrs := map[string]bool{
		"id":              true,
		"source":          true,
		"specversion":     true,
		"type":            true,
		"datacontenttype": true,
		"dataschema":      true,
		"subject":         true,
		"time":            true,
		"data":            true,
	}

	// Everything else is an extension
	ce.Extensions = make(map[string]interface{})
	for k, v := range raw {
		if !standardAttrs[k] {
			ce.Extensions[k] = v
		}
	}

	return nil
}

// ToCloudEvent converts a Keystone Core Event to a CloudEvent
func ToCloudEvent(event *Event) *CloudEvent {
	ce := &CloudEvent{
		ID:              event.ID,
		Source:          event.Source,
		SpecVersion:     "1.0",
		Type:            string(event.Type),
		DataContentType: "application/json",
		Time:            &event.Time,
		Data:            event.Data,
		Extensions:      make(map[string]interface{}),
	}

	// Add severity as extension
	ce.Extensions["severity"] = string(event.Severity)

	// Add correlation ID if present
	if event.CorrelationID != "" {
		ce.Extensions["correlationid"] = event.CorrelationID
	}

	// Add tags as extensions with "tag_" prefix
	for k, v := range event.Tags {
		ce.Extensions["tag_"+k] = v
	}

	return ce
}

// FromCloudEvent converts a CloudEvent to a Keystone Core Event
func FromCloudEvent(ce *CloudEvent) (*Event, error) {
	if ce.SpecVersion != "1.0" {
		return nil, fmt.Errorf("unsupported CloudEvents version: %s", ce.SpecVersion)
	}

	event := &Event{
		ID:     ce.ID,
		Source: ce.Source,
		Type:   EventType(ce.Type),
		Tags:   make(map[string]string),
		Data:   make(map[string]interface{}),
	}

	// Set time
	if ce.Time != nil {
		event.Time = *ce.Time
	} else {
		event.Time = time.Now()
	}

	// Extract severity from extensions
	if sev, ok := ce.Extensions["severity"].(string); ok {
		event.Severity = Severity(sev)
	} else {
		event.Severity = SeverityInfo
	}

	// Extract correlation ID from extensions
	if cid, ok := ce.Extensions["correlationid"].(string); ok {
		event.CorrelationID = cid
	}

	// Extract tags from extensions (with "tag_" prefix)
	for k, v := range ce.Extensions {
		if len(k) > 4 && k[:4] == "tag_" {
			if strVal, ok := v.(string); ok {
				event.Tags[k[4:]] = strVal
			}
		}
	}

	// Set data from CloudEvent data
	if ce.Data != nil {
		if dataMap, ok := ce.Data.(map[string]interface{}); ok {
			event.Data = dataMap
		} else {
			// If data is not a map, store it under "payload" key
			event.Data["payload"] = ce.Data
		}
	}

	// Set subject if present
	if ce.Subject != "" {
		event.Subject = ce.Subject
	}

	return event, nil
}

// CloudEventPublisher wraps an EventPublisher to emit CloudEvents
type CloudEventPublisher struct {
	publisher EventPublisher
	formatter func(*Event) (*CloudEvent, error)
}

// NewCloudEventPublisher creates a publisher that emits CloudEvents
func NewCloudEventPublisher(publisher EventPublisher) *CloudEventPublisher {
	return &CloudEventPublisher{
		publisher: publisher,
		formatter: func(e *Event) (*CloudEvent, error) {
			return ToCloudEvent(e), nil
		},
	}
}

// SetFormatter sets a custom CloudEvent formatter
func (p *CloudEventPublisher) SetFormatter(formatter func(*Event) (*CloudEvent, error)) {
	p.formatter = formatter
}

func (p *CloudEventPublisher) Publish(event *Event) error {
	_, err := p.formatter(event)
	if err != nil {
		return fmt.Errorf("failed to format CloudEvent: %w", err)
	}

	// Convert CloudEvent back to Event for publishing
	// In a real implementation, this would publish to a CloudEvents-compatible transport
	return p.publisher.Publish(event)
}

func (p *CloudEventPublisher) PublishAsync(event *Event) error {
	_, err := p.formatter(event)
	if err != nil {
		return fmt.Errorf("failed to format CloudEvent: %w", err)
	}

	// Convert CloudEvent back to Event for publishing
	return p.publisher.PublishAsync(event)
}

func (p *CloudEventPublisher) Close() error {
	return p.publisher.Close()
}

// CloudEventSubscriber wraps an EventSubscriber to receive CloudEvents
type CloudEventSubscriber struct {
	subscriber EventSubscriber
	parser     func(*CloudEvent) (*Event, error)
}

// NewCloudEventSubscriber creates a subscriber that receives CloudEvents
func NewCloudEventSubscriber(subscriber EventSubscriber) *CloudEventSubscriber {
	return &CloudEventSubscriber{
		subscriber: subscriber,
		parser:     FromCloudEvent,
	}
}

// SetParser sets a custom CloudEvent parser
func (s *CloudEventSubscriber) SetParser(parser func(*CloudEvent) (*Event, error)) {
	s.parser = parser
}

// Subscribe subscribes to events and converts them from CloudEvents
func (s *CloudEventSubscriber) Subscribe(subject string, handler EventHandler) (*Subscription, error) {
	// Wrap handler to convert CloudEvents
	wrappedHandler := func(event *Event) error {
		// In a real implementation, this would receive CloudEvents from transport
		// and convert them to Keystone Core events
		return handler(event)
	}

	return s.subscriber.Subscribe(subject, wrappedHandler)
}

// SubscribeQueue subscribes to events with a queue group
func (s *CloudEventSubscriber) SubscribeQueue(subject string, queue string, handler EventHandler) (*Subscription, error) {
	wrappedHandler := func(event *Event) error {
		return handler(event)
	}

	return s.subscriber.SubscribeQueue(subject, queue, wrappedHandler)
}

func (s *CloudEventSubscriber) Close() error {
	return s.subscriber.Close()
}

// HTTPCloudEventHandler provides HTTP endpoint for receiving CloudEvents
type HTTPCloudEventHandler struct {
	handler EventHandler
	parser  func(*CloudEvent) (*Event, error)
}

// NewHTTPCloudEventHandler creates a handler for HTTP CloudEvents
func NewHTTPCloudEventHandler(handler EventHandler) *HTTPCloudEventHandler {
	return &HTTPCloudEventHandler{
		handler: handler,
		parser:  FromCloudEvent,
	}
}

// SetParser sets a custom CloudEvent parser
func (h *HTTPCloudEventHandler) SetParser(parser func(*CloudEvent) (*Event, error)) {
	h.parser = parser
}

// HandleCloudEvent processes a CloudEvent received via HTTP
func (h *HTTPCloudEventHandler) HandleCloudEvent(ce *CloudEvent) error {
	event, err := h.parser(ce)
	if err != nil {
		return fmt.Errorf("failed to parse CloudEvent: %w", err)
	}

	return h.handler(event)
}

// ValidateCloudEvent validates a CloudEvent according to the spec
func ValidateCloudEvent(ce *CloudEvent) error {
	if ce.ID == "" {
		return fmt.Errorf("id is required")
	}
	if ce.Source == "" {
		return fmt.Errorf("source is required")
	}
	if ce.SpecVersion == "" {
		return fmt.Errorf("specversion is required")
	}
	if ce.Type == "" {
		return fmt.Errorf("type is required")
	}

	// Validate specversion
	if ce.SpecVersion != "1.0" {
		return fmt.Errorf("unsupported specversion: %s (only 1.0 supported)", ce.SpecVersion)
	}

	return nil
}

// CloudEventBatch represents a batch of CloudEvents
type CloudEventBatch struct {
	Events []*CloudEvent `json:"events"`
}

// ToCloudEventBatch converts multiple Keystone Core events to a CloudEvent batch
func ToCloudEventBatch(events []*Event) *CloudEventBatch {
	batch := &CloudEventBatch{
		Events: make([]*CloudEvent, len(events)),
	}

	for i, event := range events {
		batch.Events[i] = ToCloudEvent(event)
	}

	return batch
}

// FromCloudEventBatch converts a CloudEvent batch to Keystone Core events
func FromCloudEventBatch(batch *CloudEventBatch) ([]*Event, error) {
	events := make([]*Event, len(batch.Events))

	for i, ce := range batch.Events {
		event, err := FromCloudEvent(ce)
		if err != nil {
			return nil, fmt.Errorf("failed to convert event %d: %w", i, err)
		}
		events[i] = event
	}

	return events, nil
}
