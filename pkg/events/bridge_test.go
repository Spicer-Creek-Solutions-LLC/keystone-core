package events

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewBridge(t *testing.T) {
	config := &BridgeConfig{
		Name:        "test-bridge",
		Description: "Test bridge",
		Enabled:     true,
		SourceType:  "nats",
		DestinationType: "kafka",
	}

	bridge, err := NewBridge(config)
	if err != nil {
		t.Fatalf("NewBridge failed: %v", err)
	}

	if bridge.config.Name != "test-bridge" {
		t.Errorf("Expected name=test-bridge, got %s", bridge.config.Name)
	}

	if bridge.config.BufferSize == 0 {
		t.Error("Expected default buffer size to be set")
	}

	if bridge.config.MaxRetries == 0 {
		t.Error("Expected default max retries to be set")
	}
}

func TestNewBridge_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config *BridgeConfig
		errMsg string
	}{
		{
			name:   "nil config",
			config: nil,
			errMsg: "config is required",
		},
		{
			name: "empty name",
			config: &BridgeConfig{
				Name: "",
			},
			errMsg: "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBridge(tt.config)
			if err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

func TestBridge_StartStop(t *testing.T) {
	config := &BridgeConfig{
		Name:    "test-bridge",
		Enabled: true,
	}

	bridge, _ := NewBridge(config)

	// Create mock subscriber and publisher
	mockSub := &MockSubscriber{}
	mockPub := &MockPublisher{}

	bridge.SetSubscriber(mockSub).SetPublisher(mockPub)

	// Start bridge
	err := bridge.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !bridge.IsRunning() {
		t.Error("Expected bridge to be running")
	}

	// Stop bridge
	err = bridge.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if bridge.IsRunning() {
		t.Error("Expected bridge to be stopped")
	}
}

func TestBridge_StartWithoutDependencies(t *testing.T) {
	config := &BridgeConfig{
		Name:    "test-bridge",
		Enabled: true,
	}

	bridge, _ := NewBridge(config)

	// Try to start without subscriber
	err := bridge.Start()
	if err == nil {
		t.Error("Expected error starting without subscriber")
	}

	// Add subscriber but no publisher
	mockSub := &MockSubscriber{}
	bridge.SetSubscriber(mockSub)

	err = bridge.Start()
	if err == nil {
		t.Error("Expected error starting without publisher")
	}
}

func TestBridge_EventForwarding(t *testing.T) {
	config := &BridgeConfig{
		Name:    "test-bridge",
		Enabled: true,
	}

	bridge, _ := NewBridge(config)

	// Track published events
	var publishedEvents []*Event
	var mu sync.Mutex

	mockSub := &MockSubscriber{
		SubscribeFunc: func(subject string, handler EventHandler) (*Subscription, error) {
			// Simulate receiving an event
			go func() {
				time.Sleep(50 * time.Millisecond)
				event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
				handler(event)
			}()
			return &Subscription{ID: "test-sub", Subject: subject}, nil
		},
	}

	mockPub := &MockPublisher{
		PublishFunc: func(event *Event) error {
			mu.Lock()
			publishedEvents = append(publishedEvents, event)
			mu.Unlock()
			return nil
		},
	}

	bridge.SetSubscriber(mockSub).SetPublisher(mockPub)

	err := bridge.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for event to be forwarded
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	eventCount := len(publishedEvents)
	mu.Unlock()

	if eventCount != 1 {
		t.Errorf("Expected 1 forwarded event, got %d", eventCount)
	}

	metrics := bridge.GetMetrics()
	if metrics.EventsReceived != 1 {
		t.Errorf("Expected 1 received event, got %d", metrics.EventsReceived)
	}

	if metrics.EventsForwarded != 1 {
		t.Errorf("Expected 1 forwarded event, got %d", metrics.EventsForwarded)
	}

	bridge.Stop()
}

func TestBridge_EventFiltering(t *testing.T) {
	// Filter to only forward agent.connect events
	filter := &ComparisonExpr{
		Field:    "type",
		Operator: OpEqual,
		Value:    string(EventTypeAgentConnect),
	}

	config := &BridgeConfig{
		Name:    "test-bridge",
		Enabled: true,
		Filter:  filter,
	}

	bridge, _ := NewBridge(config)

	var publishedCount int
	var mu sync.Mutex

	mockSub := &MockSubscriber{
		SubscribeFunc: func(subject string, handler EventHandler) (*Subscription, error) {
			go func() {
				time.Sleep(50 * time.Millisecond)
				// Send matching event
				handler(NewEvent(EventTypeAgentConnect).Source("/test").Build())
				// Send non-matching event
				handler(NewEvent(EventTypeJobStart).Source("/test").Build())
			}()
			return &Subscription{ID: "test-sub", Subject: subject}, nil
		},
	}

	mockPub := &MockPublisher{
		PublishFunc: func(event *Event) error {
			mu.Lock()
			publishedCount++
			mu.Unlock()
			return nil
		},
	}

	bridge.SetSubscriber(mockSub).SetPublisher(mockPub)
	bridge.Start()

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := publishedCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("Expected 1 forwarded event (filtered), got %d", count)
	}

	metrics := bridge.GetMetrics()
	if metrics.EventsFiltered != 1 {
		t.Errorf("Expected 1 filtered event, got %d", metrics.EventsFiltered)
	}

	bridge.Stop()
}

func TestBridge_EventTransformation(t *testing.T) {
	// Transform to add a tag
	transform := func(event *Event) (*Event, error) {
		event.Tags["transformed"] = "true"
		return event, nil
	}

	config := &BridgeConfig{
		Name:      "test-bridge",
		Enabled:   true,
		Transform: transform,
	}

	bridge, _ := NewBridge(config)

	var transformedEvent *Event
	var mu sync.Mutex

	mockSub := &MockSubscriber{
		SubscribeFunc: func(subject string, handler EventHandler) (*Subscription, error) {
			go func() {
				time.Sleep(50 * time.Millisecond)
				event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
				handler(event)
			}()
			return &Subscription{ID: "test-sub", Subject: subject}, nil
		},
	}

	mockPub := &MockPublisher{
		PublishFunc: func(event *Event) error {
			mu.Lock()
			transformedEvent = event
			mu.Unlock()
			return nil
		},
	}

	bridge.SetSubscriber(mockSub).SetPublisher(mockPub)
	bridge.Start()

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if transformedEvent == nil {
		t.Fatal("Expected transformed event")
	}
	hasTag := transformedEvent.Tags["transformed"] == "true"
	mu.Unlock()

	if !hasTag {
		t.Error("Expected event to have 'transformed' tag")
	}

	bridge.Stop()
}

func TestBridge_TransformError(t *testing.T) {
	// Transform that returns an error
	transform := func(event *Event) (*Event, error) {
		return nil, fmt.Errorf("transform error")
	}

	config := &BridgeConfig{
		Name:      "test-bridge",
		Enabled:   true,
		Transform: transform,
		OnError:   ErrorBehaviorContinue,
	}

	bridge, _ := NewBridge(config)

	var publishedCount int
	var mu sync.Mutex

	mockSub := &MockSubscriber{
		SubscribeFunc: func(subject string, handler EventHandler) (*Subscription, error) {
			go func() {
				time.Sleep(50 * time.Millisecond)
				event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
				handler(event)
			}()
			return &Subscription{ID: "test-sub", Subject: subject}, nil
		},
	}

	mockPub := &MockPublisher{
		PublishFunc: func(event *Event) error {
			mu.Lock()
			publishedCount++
			mu.Unlock()
			return nil
		},
	}

	bridge.SetSubscriber(mockSub).SetPublisher(mockPub)
	bridge.Start()

	time.Sleep(200 * time.Millisecond)

	metrics := bridge.GetMetrics()
	if metrics.TransformErrors != 1 {
		t.Errorf("Expected 1 transform error, got %d", metrics.TransformErrors)
	}

	// Should still forward original event
	mu.Lock()
	count := publishedCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("Expected 1 forwarded event (original), got %d", count)
	}

	bridge.Stop()
}

func TestBridge_PublishRetry(t *testing.T) {
	config := &BridgeConfig{
		Name:         "test-bridge",
		Enabled:      true,
		MaxRetries:   3,
		RetryBackoff: 10 * time.Millisecond,
		OnError:      ErrorBehaviorContinue,
	}

	bridge, _ := NewBridge(config)

	attempts := 0
	var mu sync.Mutex

	mockSub := &MockSubscriber{
		SubscribeFunc: func(subject string, handler EventHandler) (*Subscription, error) {
			go func() {
				time.Sleep(50 * time.Millisecond)
				event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
				handler(event)
			}()
			return &Subscription{ID: "test-sub", Subject: subject}, nil
		},
	}

	mockPub := &MockPublisher{
		PublishFunc: func(event *Event) error {
			mu.Lock()
			attempts++
			mu.Unlock()
			return fmt.Errorf("publish error")
		},
	}

	bridge.SetSubscriber(mockSub).SetPublisher(mockPub)
	bridge.Start()

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	attemptCount := attempts
	mu.Unlock()

	// Should retry MaxRetries+1 times (initial + retries)
	expectedAttempts := config.MaxRetries + 1
	if attemptCount != expectedAttempts {
		t.Errorf("Expected %d publish attempts, got %d", expectedAttempts, attemptCount)
	}

	metrics := bridge.GetMetrics()
	if metrics.EventsFailed != 1 {
		t.Errorf("Expected 1 failed event, got %d", metrics.EventsFailed)
	}

	bridge.Stop()
}

func TestBridgeManager_AddRemove(t *testing.T) {
	manager := NewBridgeManager()

	config := &BridgeConfig{
		Name:    "test-bridge",
		Enabled: true,
	}

	bridge, _ := NewBridge(config)

	// Add bridge
	err := manager.AddBridge(bridge)
	if err != nil {
		t.Fatalf("AddBridge failed: %v", err)
	}

	// Try to add same bridge again
	err = manager.AddBridge(bridge)
	if err == nil {
		t.Error("Expected error adding duplicate bridge")
	}

	// Get bridge
	retrieved, err := manager.GetBridge("test-bridge")
	if err != nil {
		t.Fatalf("GetBridge failed: %v", err)
	}

	if retrieved.config.Name != "test-bridge" {
		t.Error("Retrieved bridge name mismatch")
	}

	// Remove bridge
	err = manager.RemoveBridge("test-bridge")
	if err != nil {
		t.Fatalf("RemoveBridge failed: %v", err)
	}

	// Try to get removed bridge
	_, err = manager.GetBridge("test-bridge")
	if err == nil {
		t.Error("Expected error getting removed bridge")
	}
}

func TestBridgeManager_ListBridges(t *testing.T) {
	manager := NewBridgeManager()

	bridge1, _ := NewBridge(&BridgeConfig{Name: "bridge1", Enabled: true})
	bridge2, _ := NewBridge(&BridgeConfig{Name: "bridge2", Enabled: true})

	manager.AddBridge(bridge1)
	manager.AddBridge(bridge2)

	bridges := manager.ListBridges()
	if len(bridges) != 2 {
		t.Errorf("Expected 2 bridges, got %d", len(bridges))
	}
}

func TestBridgeManager_StartStopBridge(t *testing.T) {
	manager := NewBridgeManager()

	config := &BridgeConfig{
		Name:    "test-bridge",
		Enabled: true,
	}

	bridge, _ := NewBridge(config)
	bridge.SetSubscriber(&MockSubscriber{})
	bridge.SetPublisher(&MockPublisher{})

	manager.AddBridge(bridge)

	// Start bridge
	err := manager.StartBridge("test-bridge")
	if err != nil {
		t.Fatalf("StartBridge failed: %v", err)
	}

	if !bridge.IsRunning() {
		t.Error("Expected bridge to be running")
	}

	// Stop bridge
	err = manager.StopBridge("test-bridge")
	if err != nil {
		t.Fatalf("StopBridge failed: %v", err)
	}

	if bridge.IsRunning() {
		t.Error("Expected bridge to be stopped")
	}
}

func TestBridgeManager_StartStopAll(t *testing.T) {
	manager := NewBridgeManager()

	bridge1, _ := NewBridge(&BridgeConfig{Name: "bridge1", Enabled: true})
	bridge1.SetSubscriber(&MockSubscriber{})
	bridge1.SetPublisher(&MockPublisher{})

	bridge2, _ := NewBridge(&BridgeConfig{Name: "bridge2", Enabled: true})
	bridge2.SetSubscriber(&MockSubscriber{})
	bridge2.SetPublisher(&MockPublisher{})

	bridge3, _ := NewBridge(&BridgeConfig{Name: "bridge3", Enabled: false})
	bridge3.SetSubscriber(&MockSubscriber{})
	bridge3.SetPublisher(&MockPublisher{})

	manager.AddBridge(bridge1)
	manager.AddBridge(bridge2)
	manager.AddBridge(bridge3)

	// Start all enabled bridges
	err := manager.StartAll()
	if err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}

	if !bridge1.IsRunning() {
		t.Error("Expected bridge1 to be running")
	}

	if !bridge2.IsRunning() {
		t.Error("Expected bridge2 to be running")
	}

	if bridge3.IsRunning() {
		t.Error("Expected bridge3 to not be running (disabled)")
	}

	// Stop all bridges
	err = manager.StopAll()
	if err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}

	if bridge1.IsRunning() {
		t.Error("Expected bridge1 to be stopped")
	}

	if bridge2.IsRunning() {
		t.Error("Expected bridge2 to be stopped")
	}
}

func TestBridgeManager_GetAllMetrics(t *testing.T) {
	manager := NewBridgeManager()

	bridge1, _ := NewBridge(&BridgeConfig{Name: "bridge1", Enabled: true})
	bridge2, _ := NewBridge(&BridgeConfig{Name: "bridge2", Enabled: true})

	manager.AddBridge(bridge1)
	manager.AddBridge(bridge2)

	metrics := manager.GetAllMetrics()

	if len(metrics) != 2 {
		t.Errorf("Expected metrics for 2 bridges, got %d", len(metrics))
	}

	if _, ok := metrics["bridge1"]; !ok {
		t.Error("Expected metrics for bridge1")
	}

	if _, ok := metrics["bridge2"]; !ok {
		t.Error("Expected metrics for bridge2")
	}
}
