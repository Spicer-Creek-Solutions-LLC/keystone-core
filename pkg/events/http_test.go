package events

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultHTTPReceiverConfig(t *testing.T) {
	config := DefaultHTTPReceiverConfig()

	if config.Port == 0 {
		t.Error("Expected default port to be set")
	}

	if config.Path == "" {
		t.Error("Expected default path to be set")
	}

	if !config.AcceptCloudEvents {
		t.Error("Expected AcceptCloudEvents to be true")
	}

	if !config.AcceptNative {
		t.Error("Expected AcceptNative to be true")
	}
}

func TestNewHTTPReceiver(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	mockPub := &MockPublisher{}

	receiver, err := NewHTTPReceiver(config, mockPub)
	if err != nil {
		t.Fatalf("NewHTTPReceiver failed: %v", err)
	}

	if receiver.config.Port != config.Port {
		t.Error("Config not set correctly")
	}
}

func TestNewHTTPReceiver_NilPublisher(t *testing.T) {
	config := DefaultHTTPReceiverConfig()

	_, err := NewHTTPReceiver(config, nil)
	if err == nil {
		t.Error("Expected error with nil publisher")
	}
}

func TestHTTPReceiver_HandleEvent_Native(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	config.SignatureRequired = false

	var publishedEvent *Event
	mockPub := &MockPublisher{
		PublishAsyncFunc: func(event *Event) error {
			publishedEvent = event
			return nil
		},
	}

	receiver, _ := NewHTTPReceiver(config, mockPub)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Data("key", "value").
		Build()

	body, _ := json.Marshal(event)

	req := httptest.NewRequest("POST", "/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	receiver.handleEvent(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", w.Code)
	}

	if publishedEvent == nil {
		t.Fatal("Expected event to be published")
	}

	if publishedEvent.ID != event.ID {
		t.Errorf("Expected event ID=%s, got %s", event.ID, publishedEvent.ID)
	}
}

func TestHTTPReceiver_HandleEvent_CloudEvents(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	config.SignatureRequired = false

	var publishedEvent *Event
	mockPub := &MockPublisher{
		PublishAsyncFunc: func(event *Event) error {
			publishedEvent = event
			return nil
		},
	}

	receiver, _ := NewHTTPReceiver(config, mockPub)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Data("key", "value").
		Build()

	ce := ToCloudEvent(event)
	body, _ := json.Marshal(ce)

	req := httptest.NewRequest("POST", "/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")

	w := httptest.NewRecorder()
	receiver.handleEvent(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", w.Code)
	}

	if publishedEvent == nil {
		t.Fatal("Expected event to be published")
	}

	if publishedEvent.Type != event.Type {
		t.Errorf("Expected event Type=%s, got %s", event.Type, publishedEvent.Type)
	}
}

func TestHTTPReceiver_HandleEvent_WithSignature(t *testing.T) {
	secret := "test-secret"
	config := DefaultHTTPReceiverConfig()
	config.Secret = secret
	config.SignatureRequired = true

	var publishedEvent *Event
	mockPub := &MockPublisher{
		PublishAsyncFunc: func(event *Event) error {
			publishedEvent = event
			return nil
		},
	}

	receiver, _ := NewHTTPReceiver(config, mockPub)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	body, _ := json.Marshal(event)

	// Calculate signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TitanAnvil-Signature", signature)

	w := httptest.NewRecorder()
	receiver.handleEvent(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", w.Code)
	}

	if publishedEvent == nil {
		t.Error("Expected event to be published")
	}
}

func TestHTTPReceiver_HandleEvent_InvalidSignature(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	config.Secret = "test-secret"
	config.SignatureRequired = true

	mockPub := &MockPublisher{}
	receiver, _ := NewHTTPReceiver(config, mockPub)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()
	body, _ := json.Marshal(event)

	req := httptest.NewRequest("POST", "/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TitanAnvil-Signature", "invalid-signature")

	w := httptest.NewRecorder()
	receiver.handleEvent(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestHTTPReceiver_HandleEvent_MethodNotAllowed(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	mockPub := &MockPublisher{}
	receiver, _ := NewHTTPReceiver(config, mockPub)

	req := httptest.NewRequest("GET", "/events", nil)
	w := httptest.NewRecorder()
	receiver.handleEvent(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestHTTPReceiver_HandleBatch_Native(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	config.SignatureRequired = false

	var publishedCount int
	mockPub := &MockPublisher{
		PublishAsyncFunc: func(event *Event) error {
			publishedCount++
			return nil
		},
	}

	receiver, _ := NewHTTPReceiver(config, mockPub)

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test1").Build(),
		NewEvent(EventTypeJobStart).Source("/test2").Build(),
		NewEvent(EventTypeStateChange).Source("/test3").Build(),
	}

	body, _ := json.Marshal(events)

	req := httptest.NewRequest("POST", "/events/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	receiver.handleBatch(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", w.Code)
	}

	if publishedCount != 3 {
		t.Errorf("Expected 3 events published, got %d", publishedCount)
	}
}

func TestHTTPReceiver_HandleBatch_CloudEvents(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	config.SignatureRequired = false

	var publishedCount int
	mockPub := &MockPublisher{
		PublishAsyncFunc: func(event *Event) error {
			publishedCount++
			return nil
		},
	}

	receiver, _ := NewHTTPReceiver(config, mockPub)

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test1").Build(),
		NewEvent(EventTypeJobStart).Source("/test2").Build(),
	}

	batch := ToCloudEventBatch(events)
	body, _ := json.Marshal(batch)

	req := httptest.NewRequest("POST", "/events/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")

	w := httptest.NewRecorder()
	receiver.handleBatch(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", w.Code)
	}

	if publishedCount != 2 {
		t.Errorf("Expected 2 events published, got %d", publishedCount)
	}
}

func TestHTTPReceiver_HandleHealth(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	mockPub := &MockPublisher{}
	receiver, _ := NewHTTPReceiver(config, mockPub)

	receiver.running = true

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	receiver.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]string
	json.NewDecoder(w.Body).Decode(&response)

	if response["status"] != "healthy" {
		t.Errorf("Expected status=healthy, got %s", response["status"])
	}
}

func TestHTTPReceiver_HandleMetrics(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	mockPub := &MockPublisher{}
	receiver, _ := NewHTTPReceiver(config, mockPub)

	receiver.eventsReceived = 100
	receiver.eventsFailed = 5
	receiver.lastEvent = time.Now()

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	receiver.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var metrics map[string]interface{}
	json.NewDecoder(w.Body).Decode(&metrics)

	if int64(metrics["events_received"].(float64)) != 100 {
		t.Error("Metrics not returned correctly")
	}
}

func TestHTTPSender_Send_Native(t *testing.T) {
	// Create test server
	received := false
	var receivedEvent *Event

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		json.NewDecoder(r.Body).Decode(&receivedEvent)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sender := NewHTTPSender(server.URL, "", false)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	err := sender.Send(event)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if !received {
		t.Error("Expected event to be received")
	}

	if receivedEvent.ID != event.ID {
		t.Errorf("Event ID mismatch: expected %s, got %s", event.ID, receivedEvent.ID)
	}
}

func TestHTTPSender_Send_CloudEvents(t *testing.T) {
	// Create test server
	received := false
	var receivedCE *CloudEvent

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		json.NewDecoder(r.Body).Decode(&receivedCE)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sender := NewHTTPSender(server.URL, "", true)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	err := sender.Send(event)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if !received {
		t.Error("Expected event to be received")
	}

	if receivedCE.ID != event.ID {
		t.Errorf("CloudEvent ID mismatch: expected %s, got %s", event.ID, receivedCE.ID)
	}
}

func TestHTTPSender_Send_WithSignature(t *testing.T) {
	secret := "test-secret"

	// Create test server
	signatureValid := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 0)
		r.Body.Read(body)

		signature := r.Header.Get("X-TitanAnvil-Signature")
		if signature != "" {
			signatureValid = true
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sender := NewHTTPSender(server.URL, secret, false)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := sender.Send(event)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if !signatureValid {
		t.Error("Expected signature to be included")
	}
}

func TestHTTPSender_SendBatch(t *testing.T) {
	// Create test server
	received := false
	var receivedEvents []*Event

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/batch" {
			received = true
			json.NewDecoder(r.Body).Decode(&receivedEvents)
			w.WriteHeader(http.StatusAccepted)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	sender := NewHTTPSender(server.URL, "", false)

	events := []*Event{
		NewEvent(EventTypeAgentConnect).Source("/test1").Build(),
		NewEvent(EventTypeJobStart).Source("/test2").Build(),
		NewEvent(EventTypeStateChange).Source("/test3").Build(),
	}

	err := sender.SendBatch(events)
	if err != nil {
		t.Fatalf("SendBatch failed: %v", err)
	}

	if !received {
		t.Error("Expected batch to be received")
	}

	if len(receivedEvents) != 3 {
		t.Errorf("Expected 3 events, got %d", len(receivedEvents))
	}
}

func TestHTTPSender_Send_ServerError(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
	}))
	defer server.Close()

	sender := NewHTTPSender(server.URL, "", false)

	event := NewEvent(EventTypeAgentConnect).Source("/test").Build()

	err := sender.Send(event)
	if err == nil {
		t.Error("Expected error from server")
	}
}

// Test integration between sender and receiver
func TestHTTPIntegration(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	config.SignatureRequired = false

	var publishedEvent *Event
	mockPub := &MockPublisher{
		PublishAsyncFunc: func(event *Event) error {
			publishedEvent = event
			return nil
		},
	}

	receiver, _ := NewHTTPReceiver(config, mockPub)

	// Create test server with receiver
	server := httptest.NewServer(http.HandlerFunc(receiver.handleEvent))
	defer server.Close()

	// Create sender
	sender := NewHTTPSender(server.URL, "", false)

	// Send event
	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Tag("env", "test").
		Data("key", "value").
		Build()

	err := sender.Send(event)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Wait a bit for async publishing
	time.Sleep(100 * time.Millisecond)

	if publishedEvent == nil {
		t.Fatal("Expected event to be published")
	}

	if publishedEvent.ID != event.ID {
		t.Errorf("Event ID mismatch: expected %s, got %s", event.ID, publishedEvent.ID)
	}

	if publishedEvent.Tags["env"] != "test" {
		t.Error("Event tags not preserved")
	}
}

func TestHTTPIntegration_CloudEvents(t *testing.T) {
	config := DefaultHTTPReceiverConfig()
	config.SignatureRequired = false

	var publishedEvent *Event
	mockPub := &MockPublisher{
		PublishAsyncFunc: func(event *Event) error {
			publishedEvent = event
			return nil
		},
	}

	receiver, _ := NewHTTPReceiver(config, mockPub)

	// Create test server with receiver
	server := httptest.NewServer(http.HandlerFunc(receiver.handleEvent))
	defer server.Close()

	// Create sender with CloudEvents
	sender := NewHTTPSender(server.URL, "", true)

	// Send event
	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityWarning).
		Tag("region", "us-west-2").
		Data("metric", "cpu").
		Build()

	err := sender.Send(event)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Wait a bit for async publishing
	time.Sleep(100 * time.Millisecond)

	if publishedEvent == nil {
		t.Fatal("Expected event to be published")
	}

	if publishedEvent.Type != event.Type {
		t.Errorf("Event Type mismatch: expected %s, got %s", event.Type, publishedEvent.Type)
	}

	if publishedEvent.Severity != event.Severity {
		t.Errorf("Event Severity mismatch: expected %s, got %s", event.Severity, publishedEvent.Severity)
	}

	if publishedEvent.Tags["region"] != "us-west-2" {
		t.Error("Event tags not preserved through CloudEvents")
	}
}

// Benchmark HTTP receiver
func BenchmarkHTTPReceiver_HandleEvent(b *testing.B) {
	config := DefaultHTTPReceiverConfig()
	config.SignatureRequired = false

	mockPub := &MockPublisher{
		PublishAsyncFunc: func(event *Event) error {
			return nil
		},
	}

	receiver, _ := NewHTTPReceiver(config, mockPub)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Data("key", "value").
		Build()

	body, _ := json.Marshal(event)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		receiver.handleEvent(w, req)
	}
}

// Benchmark HTTP sender
func BenchmarkHTTPSender_Send(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sender := NewHTTPSender(server.URL, "", false)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Data("key", "value").
		Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sender.Send(event)
	}
}
