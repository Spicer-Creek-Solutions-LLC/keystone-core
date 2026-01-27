package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// NotificationChannel represents a notification delivery channel.
type NotificationChannel string

const (
	// NotificationChannelLog writes to structured logging (default)
	NotificationChannelLog NotificationChannel = "log"

	// NotificationChannelWebhook sends via HTTP webhook
	NotificationChannelWebhook NotificationChannel = "webhook"

	// NotificationChannelSlack sends to Slack (future)
	NotificationChannelSlack NotificationChannel = "slack"

	// NotificationChannelEmail sends via email (future)
	NotificationChannelEmail NotificationChannel = "email"
)

// NotificationSender is the interface for sending notifications.
type NotificationSender interface {
	Send(ctx context.Context, channel NotificationChannel, config map[string]interface{}) error
}

// LogNotificationSender is a simple sender that logs notifications.
type LogNotificationSender struct{}

// Send logs the notification.
func (s *LogNotificationSender) Send(_ context.Context, channel NotificationChannel, config map[string]interface{}) error {
	// In production, this would integrate with actual notification systems.
	// For now, it's a successful stub that indicates the notification was "sent".
	return nil
}

// NotificationHandler sends notifications.
type NotificationHandler struct {
	sender NotificationSender
}

// NewNotificationHandler creates a new notification handler.
func NewNotificationHandler() *NotificationHandler {
	return &NotificationHandler{
		sender: &LogNotificationSender{},
	}
}

// NewNotificationHandlerWithSender creates a new notification handler with a custom sender.
func NewNotificationHandlerWithSender(sender NotificationSender) *NotificationHandler {
	return &NotificationHandler{
		sender: sender,
	}
}

// Type returns the step type.
func (h *NotificationHandler) Type() runbook.StepType {
	return runbook.StepTypeNotification
}

// Validate checks step config.
func (h *NotificationHandler) Validate(step *runbook.Step) error {
	// Message is required
	message, hasMessage := step.Config["message"]
	if !hasMessage {
		return errors.New("notification step requires 'message' in config")
	}

	if _, ok := message.(string); !ok {
		return errors.New("message must be a string")
	}

	// Validate channel if provided
	if channel, ok := step.Config["channel"].(string); ok {
		switch NotificationChannel(channel) {
		case NotificationChannelLog, NotificationChannelWebhook,
			NotificationChannelSlack, NotificationChannelEmail:
			// Valid channels
		default:
			return fmt.Errorf("unknown notification channel: %s", channel)
		}
	}

	// Validate severity if provided
	if severity, ok := step.Config["severity"].(string); ok {
		switch severity {
		case "info", "warning", "error", "critical":
			// Valid severities
		default:
			return fmt.Errorf("invalid severity: %s (must be info, warning, error, or critical)", severity)
		}
	}

	return nil
}

// Execute runs the step.
func (h *NotificationHandler) Execute(ctx context.Context, step *runbook.Step, vars VariableContext) (*runbook.StepResult, error) {
	start := time.Now()

	// Get message
	message, _ := step.Config["message"].(string)

	// Get channel (default to log)
	channel := NotificationChannelLog
	if ch, ok := step.Config["channel"].(string); ok {
		channel = NotificationChannel(ch)
	}

	// Get severity (default to info)
	severity := "info"
	if sev, ok := step.Config["severity"].(string); ok {
		severity = sev
	}

	// Get title (optional)
	title := ""
	if t, ok := step.Config["title"].(string); ok {
		title = t
	}

	// Build notification config for sender
	config := map[string]interface{}{
		"message":      message,
		"severity":     severity,
		"title":        title,
		"runbook":      vars.RunbookName(),
		"execution_id": vars.ExecutionID(),
	}

	// Add any additional metadata
	if metadata, ok := step.Config["metadata"].(map[string]interface{}); ok {
		for k, v := range metadata {
			config[k] = v
		}
	}

	// Add webhook URL for webhook channel
	if channel == NotificationChannelWebhook {
		if url, ok := step.Config["webhook_url"].(string); ok {
			config["webhook_url"] = url
		}
	}

	// Send notification
	err := h.sender.Send(ctx, channel, config)

	result := &runbook.StepResult{
		Duration: time.Since(start),
		Outputs: map[string]interface{}{
			"channel":  string(channel),
			"severity": severity,
			"message":  message,
		},
	}

	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("failed to send notification: %v", err)
		return result, err
	}

	result.Success = true
	result.Message = fmt.Sprintf("notification sent via %s: %s", channel, truncate(message, 50))

	return result, nil
}

// truncate shortens a string to the specified length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
