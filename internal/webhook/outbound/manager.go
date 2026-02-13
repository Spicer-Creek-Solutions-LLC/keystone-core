package outbound

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// ManagerConfig configures the outbound webhook manager.
type ManagerConfig struct {
	MaxRetries   int
	RetryBackoff time.Duration
}

// Manager subscribes to the event bus and dispatches matching events
// to outbound webhook subscriptions.
type Manager struct {
	store      SubscriptionStore
	dispatcher *Dispatcher
	subscriber events.EventSubscriber
	config     ManagerConfig
	logger     *slog.Logger

	subscription *events.Subscription
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewManager creates a new outbound webhook manager.
func NewManager(store SubscriptionStore, dispatcher *Dispatcher, subscriber events.EventSubscriber, cfg ManagerConfig) *Manager {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = time.Second
	}
	return &Manager{
		store:      store,
		dispatcher: dispatcher,
		subscriber: subscriber,
		config:     cfg,
		logger:     slog.Default().With("component", "outbound-webhooks"),
	}
}

// Start subscribes to the event bus and begins dispatching.
func (m *Manager) Start(ctx context.Context) error {
	mgrCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	sub, err := m.subscriber.SubscribeQueue(">", "outbound-webhooks", func(event *events.Event) error {
		m.handleEvent(mgrCtx, event)
		return nil
	})
	if err != nil {
		cancel()
		return fmt.Errorf("subscribe to event bus: %w", err)
	}
	m.subscription = sub
	return nil
}

// Stop unsubscribes from the event bus and waits for in-flight deliveries to complete.
func (m *Manager) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.subscription != nil {
		m.subscription.Unsubscribe()
	}
	m.wg.Wait()
	return nil
}

func (m *Manager) handleEvent(ctx context.Context, event *events.Event) {
	subs, err := m.store.ListEnabled(ctx)
	if err != nil {
		m.logger.Error("Failed to load enabled subscriptions", "error", err)
		return
	}

	payload, err := json.Marshal(event)
	if err != nil {
		m.logger.Error("Failed to marshal event", "error", err, "event_id", event.ID)
		return
	}

	eventType := string(event.Type)
	for _, sub := range subs {
		if !matchesFilter(eventType, sub.Events) {
			continue
		}
		m.wg.Add(1)
		go func(s *Subscription) {
			defer m.wg.Done()
			m.deliverToSubscription(ctx, s, payload, event)
		}(sub)
	}
}

func (m *Manager) deliverToSubscription(ctx context.Context, sub *Subscription, payload []byte, event *events.Event) {
	deliveryID := generateID()
	maxRetries := sub.MaxRetries
	if maxRetries <= 0 {
		maxRetries = m.config.MaxRetries
	}

	code, err := m.dispatcher.Deliver(ctx, sub, payload, deliveryID)

	status := DeliverySuccess
	errMsg := ""
	if err != nil {
		status = DeliveryFailed
		errMsg = err.Error()
	}

	record := &DeliveryRecord{
		ID:             deliveryID,
		SubscriptionID: sub.ID,
		EventType:      string(event.Type),
		EventID:        event.ID,
		Status:         status,
		StatusCode:     code,
		Attempt:        1,
		Error:          errMsg,
		DeliveredAt:    time.Now(),
	}
	if recordErr := m.store.RecordDelivery(ctx, record); recordErr != nil {
		m.logger.Error("Failed to record delivery", "error", recordErr, "delivery_id", deliveryID)
	}

	if err != nil && maxRetries > 0 {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			retryDelivery(ctx, m.store, m.dispatcher, sub, payload, event, maxRetries, m.config.RetryBackoff)
		}()
	}
}

// matchesFilter checks whether an event type matches any of the subscription's filter patterns.
func matchesFilter(eventType string, filters []string) bool {
	for _, pattern := range filters {
		if matched, _ := path.Match(pattern, eventType); matched {
			return true
		}
	}
	return false
}

// generateID produces a unique delivery ID.
func generateID() string {
	return fmt.Sprintf("del_%d", time.Now().UnixNano())
}
