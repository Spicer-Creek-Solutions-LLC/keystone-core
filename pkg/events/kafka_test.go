package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

func TestDefaultKafkaConfig(t *testing.T) {
	config := DefaultKafkaConfig()

	if len(config.Brokers) == 0 {
		t.Error("Expected default brokers to be set")
	}

	if config.Topic == "" {
		t.Error("Expected default topic to be set")
	}

	if !config.UseCloudEvent {
		t.Error("Expected UseCloudEvent to be true by default")
	}
}

func TestKafkaPublisher_Publish(t *testing.T) {
	// Create mock sync producer
	mockProducer := mocks.NewSyncProducer(t, nil)
	mockProducer.ExpectSendMessageAndSucceed()

	// Create mock async producer
	mockAsyncProducer := mocks.NewAsyncProducer(t, nil)

	config := DefaultKafkaConfig()

	publisher := &KafkaPublisher{
		config:   config,
		producer: mockProducer,
		async:    mockAsyncProducer,
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Data("key", "value").
		Build()

	err := publisher.Publish(event)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Verify mock expectations
	if err := mockProducer.Close(); err != nil {
		t.Errorf("Mock producer close failed: %v", err)
	}
}

func TestKafkaPublisher_PublishAsync(t *testing.T) {
	// Create mock sync producer
	mockProducer := mocks.NewSyncProducer(t, nil)

	// Create mock async producer
	mockAsyncProducer := mocks.NewAsyncProducer(t, nil)
	mockAsyncProducer.ExpectInputAndSucceed()

	config := DefaultKafkaConfig()

	publisher := &KafkaPublisher{
		config:   config,
		producer: mockProducer,
		async:    mockAsyncProducer,
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Data("key", "value").
		Build()

	err := publisher.PublishAsync(event)
	if err != nil {
		t.Fatalf("PublishAsync failed: %v", err)
	}

	// Verify mock expectations
	if err := mockAsyncProducer.Close(); err != nil {
		t.Errorf("Mock async producer close failed: %v", err)
	}
}

func TestKafkaPublisher_BuildMessage_CloudEvent(t *testing.T) {
	config := DefaultKafkaConfig()
	config.UseCloudEvent = true

	publisher := &KafkaPublisher{
		config: config,
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityWarning).
		Tag("env", "prod").
		Data("key", "value").
		Build()

	msg, err := publisher.buildMessage(event)
	if err != nil {
		t.Fatalf("buildMessage failed: %v", err)
	}

	if msg.Topic != config.Topic {
		t.Errorf("Expected topic=%s, got %s", config.Topic, msg.Topic)
	}

	if string(msg.Key.(sarama.StringEncoder)) != event.ID {
		t.Errorf("Expected key=%s, got %s", event.ID, msg.Key)
	}

	// Verify it's a CloudEvent
	var ce CloudEvent
	if err := json.Unmarshal(msg.Value.(sarama.ByteEncoder), &ce); err != nil {
		t.Fatalf("Failed to unmarshal CloudEvent: %v", err)
	}

	if ce.ID != event.ID {
		t.Errorf("Expected CloudEvent ID=%s, got %s", event.ID, ce.ID)
	}

	if ce.Type != string(event.Type) {
		t.Errorf("Expected CloudEvent Type=%s, got %s", event.Type, ce.Type)
	}

	// Verify headers
	foundContentType := false
	for _, header := range msg.Headers {
		if string(header.Key) == "content-type" &&
		   string(header.Value) == "application/cloudevents+json" {
			foundContentType = true
			break
		}
	}

	if !foundContentType {
		t.Error("Expected content-type header for CloudEvent")
	}
}

func TestKafkaPublisher_BuildMessage_Native(t *testing.T) {
	config := DefaultKafkaConfig()
	config.UseCloudEvent = false

	publisher := &KafkaPublisher{
		config: config,
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Data("key", "value").
		Build()

	msg, err := publisher.buildMessage(event)
	if err != nil {
		t.Fatalf("buildMessage failed: %v", err)
	}

	// Verify it's a native event
	var parsedEvent Event
	if err := json.Unmarshal(msg.Value.(sarama.ByteEncoder), &parsedEvent); err != nil {
		t.Fatalf("Failed to unmarshal native event: %v", err)
	}

	if parsedEvent.ID != event.ID {
		t.Errorf("Expected event ID=%s, got %s", event.ID, parsedEvent.ID)
	}

	if parsedEvent.Type != event.Type {
		t.Errorf("Expected event Type=%s, got %s", event.Type, parsedEvent.Type)
	}

	// Verify headers
	foundContentType := false
	for _, header := range msg.Headers {
		if string(header.Key) == "content-type" {
			foundContentType = true
			break
		}
	}

	if foundContentType {
		// Native format shouldn't have CloudEvent content-type
		for _, header := range msg.Headers {
			if string(header.Key) == "content-type" &&
			   string(header.Value) == "application/cloudevents+json" {
				t.Error("Expected no CloudEvent content-type for native format")
			}
		}
	}
}

func TestKafkaPublisher_Close(t *testing.T) {
	mockProducer := mocks.NewSyncProducer(t, nil)
	mockAsyncProducer := mocks.NewAsyncProducer(t, nil)

	config := DefaultKafkaConfig()

	publisher := &KafkaPublisher{
		config:   config,
		producer: mockProducer,
		async:    mockAsyncProducer,
	}

	err := publisher.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !publisher.closed {
		t.Error("Expected publisher to be marked as closed")
	}

	// Second close should not error
	err = publisher.Close()
	if err != nil {
		t.Errorf("Second close should not error: %v", err)
	}
}

func TestKafkaPublisher_PublishAfterClose(t *testing.T) {
	mockProducer := mocks.NewSyncProducer(t, nil)
	mockAsyncProducer := mocks.NewAsyncProducer(t, nil)

	config := DefaultKafkaConfig()

	publisher := &KafkaPublisher{
		config:   config,
		producer: mockProducer,
		async:    mockAsyncProducer,
	}

	publisher.Close()

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := publisher.Publish(event)
	if err == nil {
		t.Error("Expected error publishing after close")
	}

	err = publisher.PublishAsync(event)
	if err == nil {
		t.Error("Expected error publishing async after close")
	}
}

func TestKafkaConsumerHandler_ParseMessage_CloudEvent(t *testing.T) {
	config := DefaultKafkaConfig()
	config.UseCloudEvent = true

	subscriber := &KafkaSubscriber{
		config: config,
	}

	handler := &kafkaConsumerHandler{
		subscriber: subscriber,
	}

	// Create a CloudEvent message
	event := NewEvent(EventTypeJobStart).
		Source("/test").
		Severity(SeverityInfo).
		Data("job_id", "123").
		Build()

	ce := ToCloudEvent(event)
	ceJSON, _ := json.Marshal(ce)

	message := &sarama.ConsumerMessage{
		Value: ceJSON,
		Headers: []*sarama.RecordHeader{
			{
				Key:   []byte("content-type"),
				Value: []byte("application/cloudevents+json"),
			},
		},
	}

	parsed, err := handler.parseMessage(message)
	if err != nil {
		t.Fatalf("parseMessage failed: %v", err)
	}

	if parsed.ID != event.ID {
		t.Errorf("Expected ID=%s, got %s", event.ID, parsed.ID)
	}

	if parsed.Type != event.Type {
		t.Errorf("Expected Type=%s, got %s", event.Type, parsed.Type)
	}
}

func TestKafkaConsumerHandler_ParseMessage_Native(t *testing.T) {
	config := DefaultKafkaConfig()
	config.UseCloudEvent = false

	subscriber := &KafkaSubscriber{
		config: config,
	}

	handler := &kafkaConsumerHandler{
		subscriber: subscriber,
	}

	// Create a native event message
	event := NewEvent(EventTypeJobStart).
		Source("/test").
		Severity(SeverityInfo).
		Data("job_id", "123").
		Build()

	eventJSON, _ := json.Marshal(event)

	message := &sarama.ConsumerMessage{
		Value: eventJSON,
		Headers: []*sarama.RecordHeader{
			{
				Key:   []byte("event-type"),
				Value: []byte(event.Type),
			},
		},
	}

	parsed, err := handler.parseMessage(message)
	if err != nil {
		t.Fatalf("parseMessage failed: %v", err)
	}

	if parsed.ID != event.ID {
		t.Errorf("Expected ID=%s, got %s", event.ID, parsed.ID)
	}

	if parsed.Type != event.Type {
		t.Errorf("Expected Type=%s, got %s", event.Type, parsed.Type)
	}
}

func TestKafkaSubscriber_Subscribe(t *testing.T) {
	config := DefaultKafkaConfig()
	config.GroupID = "test-group"

	// Create mock consumer group
	mockConsumerGroup := &mockConsumerGroup{
		errors: make(chan error, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())

	subscriber := &KafkaSubscriber{
		config:        config,
		consumerGroup: mockConsumerGroup,
		handlers:      make(map[string]EventHandler),
		ctx:           ctx,
		cancel:        cancel,
	}

	handled := false
	handler := func(event *Event) error {
		handled = true
		return nil
	}

	sub, err := subscriber.Subscribe("test-topic", handler)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	if sub.Subject != "test-topic" {
		t.Errorf("Expected subject=test-topic, got %s", sub.Subject)
	}

	// Wait a bit for consumer to start
	time.Sleep(100 * time.Millisecond)

	// Clean up
	subscriber.Close()

	// Note: handled won't be true without a real message, but we verified subscription works
	_ = handled
}

func TestKafkaSubscriber_SubscribeAfterClose(t *testing.T) {
	config := DefaultKafkaConfig()

	mockConsumerGroup := &mockConsumerGroup{
		errors: make(chan error, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())

	subscriber := &KafkaSubscriber{
		config:        config,
		consumerGroup: mockConsumerGroup,
		handlers:      make(map[string]EventHandler),
		ctx:           ctx,
		cancel:        cancel,
		closed:        true,
	}

	_, err := subscriber.Subscribe("test-topic", func(e *Event) error { return nil })
	if err == nil {
		t.Error("Expected error subscribing after close")
	}
}

// Mock consumer group for testing
type mockConsumerGroup struct {
	errors chan error
}

func (m *mockConsumerGroup) Consume(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) error {
	// Simulate consume blocking until context is cancelled
	<-ctx.Done()
	return ctx.Err()
}

func (m *mockConsumerGroup) Errors() <-chan error {
	return m.errors
}

func (m *mockConsumerGroup) Close() error {
	close(m.errors)
	return nil
}

func (m *mockConsumerGroup) Pause(partitions map[string][]int32) {}
func (m *mockConsumerGroup) Resume(partitions map[string][]int32) {}
func (m *mockConsumerGroup) PauseAll() {}
func (m *mockConsumerGroup) ResumeAll() {}

// Benchmark Kafka message building
func BenchmarkKafkaPublisher_BuildMessage_CloudEvent(b *testing.B) {
	config := DefaultKafkaConfig()
	config.UseCloudEvent = true

	publisher := &KafkaPublisher{
		config: config,
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Tag("env", "prod").
		Data("key", "value").
		Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		publisher.buildMessage(event)
	}
}

func BenchmarkKafkaPublisher_BuildMessage_Native(b *testing.B) {
	config := DefaultKafkaConfig()
	config.UseCloudEvent = false

	publisher := &KafkaPublisher{
		config: config,
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Tag("env", "prod").
		Data("key", "value").
		Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		publisher.buildMessage(event)
	}
}
