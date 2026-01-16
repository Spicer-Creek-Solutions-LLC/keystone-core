package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/IBM/sarama"
)

// KafkaConfig holds Kafka connection configuration
type KafkaConfig struct {
	Brokers       []string
	Topic         string
	ClientID      string
	Version       string
	UseCloudEvent bool

	// Producer config
	RequiredAcks      sarama.RequiredAcks
	CompressionType   sarama.CompressionCodec
	MaxMessageBytes   int

	// Consumer config
	GroupID           string
	SessionTimeout    time.Duration
	HeartbeatInterval time.Duration
	RebalanceStrategy string

	// TLS/SASL config
	TLSEnabled   bool
	SASLEnabled  bool
	SASLUser     string
	SASLPassword string
	SASLMechanism string
}

// DefaultKafkaConfig returns a default Kafka configuration
func DefaultKafkaConfig() *KafkaConfig {
	return &KafkaConfig{
		Brokers:           []string{"localhost:9092"},
		Topic:             "kscore.events",
		ClientID:          "kscore",
		Version:           "2.8.0",
		UseCloudEvent:     true,
		RequiredAcks:      sarama.WaitForLocal,
		CompressionType:   sarama.CompressionSnappy,
		MaxMessageBytes:   1000000,
		SessionTimeout:    10 * time.Second,
		HeartbeatInterval: 3 * time.Second,
		RebalanceStrategy: "range",
	}
}

// KafkaPublisher publishes events to Kafka
type KafkaPublisher struct {
	config   *KafkaConfig
	producer sarama.SyncProducer
	async    sarama.AsyncProducer
	mu       sync.RWMutex
	closed   bool
}

// NewKafkaPublisher creates a new Kafka publisher
func NewKafkaPublisher(config *KafkaConfig) (*KafkaPublisher, error) {
	if config == nil {
		config = DefaultKafkaConfig()
	}

	saramaConfig := sarama.NewConfig()
	saramaConfig.ClientID = config.ClientID

	// Parse Kafka version
	version, err := sarama.ParseKafkaVersion(config.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Kafka version: %w", err)
	}
	saramaConfig.Version = version

	// Producer config
	saramaConfig.Producer.RequiredAcks = config.RequiredAcks
	saramaConfig.Producer.Compression = config.CompressionType
	saramaConfig.Producer.MaxMessageBytes = config.MaxMessageBytes
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Return.Errors = true

	// TLS/SASL config
	if config.TLSEnabled {
		saramaConfig.Net.TLS.Enable = true
	}
	if config.SASLEnabled {
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = config.SASLUser
		saramaConfig.Net.SASL.Password = config.SASLPassword
		saramaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		if config.SASLMechanism != "" {
			saramaConfig.Net.SASL.Mechanism = sarama.SASLMechanism(config.SASLMechanism)
		}
	}

	// Create sync producer
	producer, err := sarama.NewSyncProducer(config.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	// Create async producer
	asyncProducer, err := sarama.NewAsyncProducer(config.Brokers, saramaConfig)
	if err != nil {
		producer.Close()
		return nil, fmt.Errorf("failed to create async Kafka producer: %w", err)
	}

	kp := &KafkaPublisher{
		config:   config,
		producer: producer,
		async:    asyncProducer,
	}

	// Start error handling for async producer
	go kp.handleAsyncErrors()

	return kp, nil
}

// handleAsyncErrors processes async producer errors
func (p *KafkaPublisher) handleAsyncErrors() {
	for err := range p.async.Errors() {
		log.Printf("events kafka async producer error: %v", err)
	}
}

// Publish publishes an event synchronously
func (p *KafkaPublisher) Publish(event *Event) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return fmt.Errorf("publisher is closed")
	}

	msg, err := p.buildMessage(event)
	if err != nil {
		return err
	}

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	// Store partition/offset in event metadata for tracking
	_ = partition
	_ = offset

	return nil
}

// PublishAsync publishes an event asynchronously
func (p *KafkaPublisher) PublishAsync(event *Event) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return fmt.Errorf("publisher is closed")
	}

	msg, err := p.buildMessage(event)
	if err != nil {
		return err
	}

	select {
	case p.async.Input() <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout publishing event to Kafka")
	}
}

// buildMessage creates a Kafka message from an event
func (p *KafkaPublisher) buildMessage(event *Event) (*sarama.ProducerMessage, error) {
	var value []byte
	var err error

	if p.config.UseCloudEvent {
		// Convert to CloudEvent
		ce := ToCloudEvent(event)
		value, err = json.Marshal(ce)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal CloudEvent: %w", err)
		}
	} else {
		// Use native Keystone Core event format
		value, err = json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal event: %w", err)
		}
	}

	msg := &sarama.ProducerMessage{
		Topic: p.config.Topic,
		Key:   sarama.StringEncoder(event.ID),
		Value: sarama.ByteEncoder(value),
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("event-type"),
				Value: []byte(event.Type),
			},
			{
				Key:   []byte("event-source"),
				Value: []byte(event.Source),
			},
			{
				Key:   []byte("event-severity"),
				Value: []byte(event.Severity),
			},
		},
	}

	if p.config.UseCloudEvent {
		msg.Headers = append(msg.Headers, sarama.RecordHeader{
			Key:   []byte("content-type"),
			Value: []byte("application/cloudevents+json"),
		})
	}

	return msg, nil
}

// Close closes the Kafka publisher
func (p *KafkaPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	var errs []error
	if err := p.producer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("sync producer close: %w", err))
	}
	if err := p.async.Close(); err != nil {
		errs = append(errs, fmt.Errorf("async producer close: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing Kafka publisher: %v", errs)
	}

	return nil
}

// KafkaSubscriber subscribes to events from Kafka
type KafkaSubscriber struct {
	config        *KafkaConfig
	consumerGroup sarama.ConsumerGroup
	handlers      map[string]EventHandler
	mu            sync.RWMutex
	closed        bool
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// NewKafkaSubscriber creates a new Kafka subscriber
func NewKafkaSubscriber(config *KafkaConfig) (*KafkaSubscriber, error) {
	if config == nil {
		config = DefaultKafkaConfig()
	}

	if config.GroupID == "" {
		config.GroupID = "kscore-consumer"
	}

	saramaConfig := sarama.NewConfig()
	saramaConfig.ClientID = config.ClientID

	// Parse Kafka version
	version, err := sarama.ParseKafkaVersion(config.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Kafka version: %w", err)
	}
	saramaConfig.Version = version

	// Consumer config
	saramaConfig.Consumer.Group.Session.Timeout = config.SessionTimeout
	saramaConfig.Consumer.Group.Heartbeat.Interval = config.HeartbeatInterval
	saramaConfig.Consumer.Return.Errors = true
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest

	// Rebalance strategy
	switch config.RebalanceStrategy {
	case "sticky":
		saramaConfig.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategySticky()
	case "roundrobin":
		saramaConfig.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	default:
		saramaConfig.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRange()
	}

	// TLS/SASL config
	if config.TLSEnabled {
		saramaConfig.Net.TLS.Enable = true
	}
	if config.SASLEnabled {
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = config.SASLUser
		saramaConfig.Net.SASL.Password = config.SASLPassword
		saramaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		if config.SASLMechanism != "" {
			saramaConfig.Net.SASL.Mechanism = sarama.SASLMechanism(config.SASLMechanism)
		}
	}

	// Create consumer group
	consumerGroup, err := sarama.NewConsumerGroup(config.Brokers, config.GroupID, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka consumer group: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	ks := &KafkaSubscriber{
		config:        config,
		consumerGroup: consumerGroup,
		handlers:      make(map[string]EventHandler),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Start error handling
	go ks.handleErrors()

	return ks, nil
}

// handleErrors processes consumer group errors
func (s *KafkaSubscriber) handleErrors() {
	for err := range s.consumerGroup.Errors() {
		log.Printf("events kafka consumer group error: %v", err)
	}
}

// Subscribe subscribes to events matching a subject pattern
// For Kafka, subject is treated as a topic filter
func (s *KafkaSubscriber) Subscribe(subject string, handler EventHandler) (*Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("subscriber is closed")
	}

	// Store handler
	subID := fmt.Sprintf("kafka-%d", len(s.handlers))
	s.handlers[subID] = handler

	// Start consumer if this is the first subscription
	if len(s.handlers) == 1 {
		s.wg.Add(1)
		go s.consume()
	}

	return &Subscription{
		ID:      subID,
		Subject: subject,
	}, nil
}

// SubscribeQueue subscribes to events with a queue group
// For Kafka, this is similar to Subscribe (consumer groups handle load balancing)
func (s *KafkaSubscriber) SubscribeQueue(subject string, queue string, handler EventHandler) (*Subscription, error) {
	// In Kafka, the consumer group handles queue semantics
	return s.Subscribe(subject, handler)
}

// consume consumes messages from Kafka
func (s *KafkaSubscriber) consume() {
	defer s.wg.Done()

	consumerHandler := &kafkaConsumerHandler{
		subscriber: s,
	}

	for {
		// Check if context is cancelled
		if s.ctx.Err() != nil {
			return
		}

		// Consume will block until the context is cancelled or an error occurs
		err := s.consumerGroup.Consume(s.ctx, []string{s.config.Topic}, consumerHandler)
		if err != nil {
			log.Printf("events kafka consume error: %v", err)

			// If context is cancelled, exit
			if s.ctx.Err() != nil {
				return
			}

			// Otherwise, retry after a delay
			time.Sleep(1 * time.Second)
		}
	}
}

// Close closes the Kafka subscriber
func (s *KafkaSubscriber) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	s.cancel()
	s.wg.Wait()

	return s.consumerGroup.Close()
}

// kafkaConsumerHandler implements sarama.ConsumerGroupHandler
type kafkaConsumerHandler struct {
	subscriber *KafkaSubscriber
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (h *kafkaConsumerHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (h *kafkaConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages()
func (h *kafkaConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			// Parse event from message
			event, err := h.parseMessage(message)
			if err != nil {
				log.Printf("events kafka parse error: %v", err)
				session.MarkMessage(message, "")
				continue
			}

			// Call all handlers
			h.subscriber.mu.RLock()
			for _, handler := range h.subscriber.handlers {
				if err := handler(event); err != nil {
					log.Printf("events kafka handler error: %v", err)
				}
			}
			h.subscriber.mu.RUnlock()

			// Mark message as processed
			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

// parseMessage parses a Kafka message into an Event
func (h *kafkaConsumerHandler) parseMessage(message *sarama.ConsumerMessage) (*Event, error) {
	// Check if it's a CloudEvent
	isCloudEvent := false
	for _, header := range message.Headers {
		if string(header.Key) == "content-type" &&
		   string(header.Value) == "application/cloudevents+json" {
			isCloudEvent = true
			break
		}
	}

	if isCloudEvent || h.subscriber.config.UseCloudEvent {
		// Parse as CloudEvent
		var ce CloudEvent
		if err := json.Unmarshal(message.Value, &ce); err != nil {
			return nil, fmt.Errorf("failed to unmarshal CloudEvent: %w", err)
		}
		return FromCloudEvent(&ce)
	}

	// Parse as native Keystone Core event
	var event Event
	if err := json.Unmarshal(message.Value, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return &event, nil
}
