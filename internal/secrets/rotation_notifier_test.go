package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlackNotifier(t *testing.T) {
	t.Run("Name", func(t *testing.T) {
		notifier := NewSlackNotifier("https://hooks.slack.com/test")
		if notifier.Name() != "slack" {
			t.Errorf("expected name slack, got %s", notifier.Name())
		}
	})

	t.Run("Notify_Success", func(t *testing.T) {
		var receivedMessage SlackMessage

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("expected application/json content type")
			}

			if err := json.NewDecoder(r.Body).Decode(&receivedMessage); err != nil {
				t.Errorf("failed to decode body: %v", err)
			}

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		notifier := NewSlackNotifier(server.URL)
		notifier.Channel = "#alerts"

		notification := &Notification{
			Event:      NotificationEventStart,
			Severity:   SeverityInfo,
			RotationID: "rot-123",
			SecretPath: "vault/secret/db",
			Strategy:   RotationStrategyBlueGreen,
			State:      RotationStateInProgress,
			Message:    "Secret rotation started",
			Timestamp:  time.Now(),
			Progress: &RotationProgress{
				TotalTargets:   10,
				UpdatedTargets: 0,
				Percentage:     0,
			},
		}

		err := notifier.Notify(context.Background(), notification)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedMessage.Channel != "#alerts" {
			t.Errorf("expected channel #alerts, got %s", receivedMessage.Channel)
		}
		if len(receivedMessage.Attachments) != 1 {
			t.Errorf("expected 1 attachment, got %d", len(receivedMessage.Attachments))
		}
	})

	t.Run("Notify_WithError", func(t *testing.T) {
		var receivedMessage SlackMessage

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&receivedMessage)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		notifier := NewSlackNotifier(server.URL)

		notification := &Notification{
			Event:      NotificationEventFailed,
			Severity:   SeverityError,
			RotationID: "rot-123",
			SecretPath: "vault/secret/db",
			Strategy:   RotationStrategyBlueGreen,
			State:      RotationStateFailed,
			Message:    "Secret rotation failed",
			Error:      "connection timeout",
			Timestamp:  time.Now(),
		}

		err := notifier.Notify(context.Background(), notification)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify error field is included
		if len(receivedMessage.Attachments) != 1 {
			t.Errorf("expected 1 attachment")
		}

		hasErrorField := false
		for _, field := range receivedMessage.Attachments[0].Fields {
			if field.Title == "Error" {
				hasErrorField = true
				break
			}
		}
		if !hasErrorField {
			t.Error("expected error field in attachment")
		}
	})

	t.Run("Notify_ServerError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		notifier := NewSlackNotifier(server.URL)

		notification := &Notification{
			Event:      NotificationEventStart,
			RotationID: "rot-123",
			SecretPath: "vault/secret/db",
			Message:    "Test",
			Timestamp:  time.Now(),
		}

		err := notifier.Notify(context.Background(), notification)
		if err == nil {
			t.Error("expected error for server error response")
		}
	})

	t.Run("Colors", func(t *testing.T) {
		notifier := NewSlackNotifier("http://test")

		tests := []struct {
			severity NotificationSeverity
			expected string
		}{
			{SeverityInfo, "#36a64f"},
			{SeverityWarning, "#ffc107"},
			{SeverityError, "#dc3545"},
			{SeverityCritical, "#721c24"},
		}

		for _, tt := range tests {
			color := notifier.getColor(tt.severity)
			if color != tt.expected {
				t.Errorf("severity %s: expected color %s, got %s", tt.severity, tt.expected, color)
			}
		}
	})

	t.Run("Emojis", func(t *testing.T) {
		notifier := NewSlackNotifier("http://test")

		tests := []struct {
			event    NotificationEvent
			expected string
		}{
			{NotificationEventStart, "🔄"},
			{NotificationEventProgress, "⏳"},
			{NotificationEventComplete, "✅"},
			{NotificationEventFailed, "❌"},
			{NotificationEventRollback, "⏪"},
		}

		for _, tt := range tests {
			emoji := notifier.getEmoji(tt.event)
			if emoji != tt.expected {
				t.Errorf("event %s: expected emoji %s, got %s", tt.event, tt.expected, emoji)
			}
		}
	})
}

func TestPagerDutyNotifier(t *testing.T) {
	t.Run("Name", func(t *testing.T) {
		notifier := NewPagerDutyNotifier("test-routing-key")
		if notifier.Name() != "pagerduty" {
			t.Errorf("expected name pagerduty, got %s", notifier.Name())
		}
	})

	t.Run("Notify_Success", func(t *testing.T) {
		var receivedEvent PagerDutyEvent

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}

			if err := json.NewDecoder(r.Body).Decode(&receivedEvent); err != nil {
				t.Errorf("failed to decode body: %v", err)
			}

			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		notifier := NewPagerDutyNotifier("test-routing-key")
		notifier.Endpoint = server.URL

		notification := &Notification{
			Event:      NotificationEventFailed,
			Severity:   SeverityError,
			RotationID: "rot-123",
			SecretPath: "vault/secret/db",
			Strategy:   RotationStrategyBlueGreen,
			State:      RotationStateFailed,
			Message:    "Secret rotation failed",
			Error:      "connection timeout",
			Timestamp:  time.Now(),
		}

		err := notifier.Notify(context.Background(), notification)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedEvent.RoutingKey != "test-routing-key" {
			t.Errorf("wrong routing key: %s", receivedEvent.RoutingKey)
		}
		if receivedEvent.EventAction != "trigger" {
			t.Errorf("expected trigger action, got %s", receivedEvent.EventAction)
		}
		if receivedEvent.Payload.Severity != "error" {
			t.Errorf("expected error severity, got %s", receivedEvent.Payload.Severity)
		}
	})

	t.Run("Notify_Resolve", func(t *testing.T) {
		var receivedEvent PagerDutyEvent

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&receivedEvent)
			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		notifier := NewPagerDutyNotifier("test-routing-key")
		notifier.Endpoint = server.URL

		notification := &Notification{
			Event:      NotificationEventComplete,
			Severity:   SeverityInfo,
			RotationID: "rot-123",
			SecretPath: "vault/secret/db",
			Message:    "Secret rotation completed",
			Timestamp:  time.Now(),
		}

		err := notifier.Notify(context.Background(), notification)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedEvent.EventAction != "resolve" {
			t.Errorf("expected resolve action for complete event, got %s", receivedEvent.EventAction)
		}
	})

	t.Run("DedupKey", func(t *testing.T) {
		var receivedEvent PagerDutyEvent

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&receivedEvent)
			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		notifier := NewPagerDutyNotifier("test-routing-key")
		notifier.Endpoint = server.URL

		notification := &Notification{
			Event:      NotificationEventStart,
			RotationID: "rot-xyz-123",
			SecretPath: "vault/secret/db",
			Message:    "Test",
			Timestamp:  time.Now(),
		}

		_ = notifier.Notify(context.Background(), notification)

		expectedDedupKey := "rotation-rot-xyz-123"
		if receivedEvent.DedupKey != expectedDedupKey {
			t.Errorf("expected dedup key %s, got %s", expectedDedupKey, receivedEvent.DedupKey)
		}
	})
}

func TestNotificationManager(t *testing.T) {
	t.Run("AddNotifier", func(t *testing.T) {
		manager := NewNotificationManager()

		slack := NewSlackNotifier("http://test")
		manager.AddNotifier(slack)

		pagerduty := NewPagerDutyNotifier("test-key")
		manager.AddNotifier(pagerduty)

		// No direct way to check count, but can verify by calling Notify
	})

	t.Run("RemoveNotifier", func(t *testing.T) {
		manager := NewNotificationManager()

		slack := NewSlackNotifier("http://test")
		manager.AddNotifier(slack)

		manager.RemoveNotifier("slack")

		// Verify by checking no errors when notifying (no notifiers)
		notification := &Notification{
			Event:      NotificationEventStart,
			RotationID: "rot-123",
			Message:    "Test",
			Timestamp:  time.Now(),
		}

		errors := manager.Notify(context.Background(), notification)
		if len(errors) != 0 {
			t.Errorf("expected 0 errors after removing notifier, got %d", len(errors))
		}
	})

	t.Run("Notify_AllNotifiers", func(t *testing.T) {
		notifyCount := 0

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			notifyCount++
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		manager := NewNotificationManager()

		slack := NewSlackNotifier(server.URL)
		manager.AddNotifier(slack)

		notification := &Notification{
			Event:      NotificationEventStart,
			RotationID: "rot-123",
			SecretPath: "vault/secret/db",
			Message:    "Test",
			Timestamp:  time.Now(),
		}

		errors := manager.Notify(context.Background(), notification)
		if len(errors) != 0 {
			t.Errorf("unexpected errors: %v", errors)
		}
		if notifyCount != 1 {
			t.Errorf("expected 1 notification, got %d", notifyCount)
		}
	})

	t.Run("Notify_PartialFailure", func(t *testing.T) {
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer successServer.Close()

		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer failServer.Close()

		manager := NewNotificationManager()
		manager.AddNotifier(NewSlackNotifier(successServer.URL))
		manager.AddNotifier(NewSlackNotifier(failServer.URL))

		notification := &Notification{
			Event:      NotificationEventStart,
			RotationID: "rot-123",
			SecretPath: "vault/secret/db",
			Message:    "Test",
			Timestamp:  time.Now(),
		}

		errors := manager.Notify(context.Background(), notification)
		if len(errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(errors))
		}
	})
}

func TestCreateNotification(t *testing.T) {
	t.Run("FromRotation", func(t *testing.T) {
		targets := createTestTargets(10)
		config := &RotationConfig{Strategy: RotationStrategyBlueGreen}
		rotation := NewManagedRotation("rot-123", "vault/secret/db", config, targets, nil)
		_ = rotation.Start()

		notification := CreateNotification(rotation, NotificationEventStart, "Rotation started")

		if notification.Event != NotificationEventStart {
			t.Errorf("wrong event: %s", notification.Event)
		}
		if notification.RotationID != "rot-123" {
			t.Errorf("wrong rotation ID: %s", notification.RotationID)
		}
		if notification.SecretPath != "vault/secret/db" {
			t.Errorf("wrong secret path: %s", notification.SecretPath)
		}
		if notification.Strategy != RotationStrategyBlueGreen {
			t.Errorf("wrong strategy: %s", notification.Strategy)
		}
		if notification.State != RotationStateInProgress {
			t.Errorf("wrong state: %s", notification.State)
		}
		if notification.Progress == nil {
			t.Error("expected progress to be set")
		}
		if notification.Progress.TotalTargets != 10 {
			t.Errorf("wrong total targets: %d", notification.Progress.TotalTargets)
		}
	})

	t.Run("FailedRotation", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()
		_ = rotation.Fail(nil)

		notification := CreateNotification(rotation, NotificationEventFailed, "Rotation failed")

		if notification.Severity != SeverityError {
			t.Errorf("expected error severity for failed event, got %s", notification.Severity)
		}
	})

	t.Run("RollbackEvent", func(t *testing.T) {
		rotation := createTestRotation(5)

		notification := CreateNotification(rotation, NotificationEventRollback, "Rolling back")

		if notification.Severity != SeverityWarning {
			t.Errorf("expected warning severity for rollback event, got %s", notification.Severity)
		}
	})
}
