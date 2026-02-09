package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NotificationEvent represents a rotation notification event type.
type NotificationEvent string

// NotificationEvent constants define the events.
const (
	NotificationEventStart    NotificationEvent = "rotation_started"
	NotificationEventProgress NotificationEvent = "rotation_progress"
	NotificationEventComplete NotificationEvent = "rotation_completed"
	NotificationEventFailed   NotificationEvent = "rotation_failed"
	NotificationEventRollback NotificationEvent = "rotation_rollback"
)

// NotificationSeverity represents the severity level of a notification.
type NotificationSeverity string

// SeverityInfo constants define the severity levels.
const (
	SeverityInfo     NotificationSeverity = "info"
	SeverityWarning  NotificationSeverity = "warning"
	SeverityError    NotificationSeverity = "error"
	SeverityCritical NotificationSeverity = "critical"
)

// Notification represents a rotation notification.
type Notification struct {
	// Event is the type of notification event.
	Event NotificationEvent `json:"event"`

	// Severity is the notification severity.
	Severity NotificationSeverity `json:"severity"`

	// RotationID is the rotation identifier.
	RotationID string `json:"rotation_id"`

	// SecretPath is the secret being rotated.
	SecretPath string `json:"secret_path"`

	// Strategy is the rotation strategy.
	Strategy RotationStrategy `json:"strategy"`

	// State is the current rotation state.
	State RotationState `json:"state"`

	// Progress contains progress information.
	Progress *RotationProgress `json:"progress,omitempty"`

	// Error is any error message.
	Error string `json:"error,omitempty"`

	// Message is a human-readable message.
	Message string `json:"message"`

	// Timestamp is when the notification was created.
	Timestamp time.Time `json:"timestamp"`
}

// Notifier sends notifications for rotation events.
type Notifier interface {
	// Notify sends a notification.
	Notify(ctx context.Context, notification *Notification) error

	// Name returns the notifier name.
	Name() string
}

// SlackNotifier sends notifications to Slack.
type SlackNotifier struct {
	// WebhookURL is the Slack webhook URL.
	WebhookURL string

	// Channel is the Slack channel (optional, uses webhook default).
	Channel string

	// Username is the bot username (optional).
	Username string

	// IconEmoji is the bot icon emoji (optional).
	IconEmoji string

	// client is the HTTP client.
	client *http.Client
}

// SlackMessage represents a Slack webhook message.
type SlackMessage struct {
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	Text        string            `json:"text,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// SlackAttachment represents a Slack message attachment.
type SlackAttachment struct {
	Color      string       `json:"color"`
	Title      string       `json:"title"`
	Text       string       `json:"text"`
	Fields     []SlackField `json:"fields,omitempty"`
	Footer     string       `json:"footer,omitempty"`
	Timestamp  int64        `json:"ts,omitempty"`
	MarkdownIn []string     `json:"mrkdwn_in,omitempty"`
}

// SlackField represents a field in a Slack attachment.
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// NewSlackNotifier creates a new Slack notifier.
func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		WebhookURL: webhookURL,
		Username:   "Keystone Secrets",
		IconEmoji:  ":key:",
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns the notifier name.
func (s *SlackNotifier) Name() string {
	return "slack"
}

// Notify sends a notification to Slack.
func (s *SlackNotifier) Notify(ctx context.Context, n *Notification) error {
	color := s.getColor(n.Severity)
	emoji := s.getEmoji(n.Event)

	attachment := SlackAttachment{
		Color:      color,
		Title:      fmt.Sprintf("%s Secret Rotation %s", emoji, n.Event),
		Text:       n.Message,
		Timestamp:  n.Timestamp.Unix(),
		MarkdownIn: []string{"text", "fields"},
		Fields: []SlackField{
			{Title: "Rotation ID", Value: n.RotationID, Short: true},
			{Title: "Secret Path", Value: n.SecretPath, Short: true},
			{Title: "Strategy", Value: string(n.Strategy), Short: true},
			{Title: "State", Value: string(n.State), Short: true},
		},
	}

	if n.Progress != nil {
		attachment.Fields = append(attachment.Fields,
			SlackField{
				Title: "Progress",
				Value: fmt.Sprintf("%d/%d targets (%d%%)",
					n.Progress.UpdatedTargets,
					n.Progress.TotalTargets,
					n.Progress.Percentage),
				Short: true,
			},
		)
	}

	if n.Error != "" {
		attachment.Fields = append(attachment.Fields,
			SlackField{Title: "Error", Value: fmt.Sprintf("```%s```", n.Error), Short: false},
		)
	}

	msg := SlackMessage{
		Channel:     s.Channel,
		Username:    s.Username,
		IconEmoji:   s.IconEmoji,
		Attachments: []SlackAttachment{attachment},
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (s *SlackNotifier) getColor(severity NotificationSeverity) string {
	switch severity {
	case SeverityInfo:
		return "#36a64f" // Green
	case SeverityWarning:
		return "#ffc107" // Yellow
	case SeverityError:
		return "#dc3545" // Red
	case SeverityCritical:
		return "#721c24" // Dark Red
	default:
		return "#6c757d" // Gray
	}
}

func (s *SlackNotifier) getEmoji(event NotificationEvent) string {
	switch event {
	case NotificationEventStart:
		return "🔄"
	case NotificationEventProgress:
		return "⏳"
	case NotificationEventComplete:
		return "✅"
	case NotificationEventFailed:
		return "❌"
	case NotificationEventRollback:
		return "⏪"
	default:
		return "🔑"
	}
}

// PagerDutyNotifier sends notifications to PagerDuty.
type PagerDutyNotifier struct {
	// RoutingKey is the PagerDuty Events API v2 routing key.
	RoutingKey string

	// Endpoint is the PagerDuty Events API endpoint.
	Endpoint string

	// Source is the source identifier for events.
	Source string

	// client is the HTTP client.
	client *http.Client
}

// PagerDutyEvent represents a PagerDuty Events API v2 event.
type PagerDutyEvent struct {
	RoutingKey  string           `json:"routing_key"`
	EventAction string           `json:"event_action"`
	DedupKey    string           `json:"dedup_key,omitempty"`
	Payload     PagerDutyPayload `json:"payload"`
	Links       []PagerDutyLink  `json:"links,omitempty"`
	Images      []PagerDutyImage `json:"images,omitempty"`
}

// PagerDutyPayload represents the payload of a PagerDuty event.
type PagerDutyPayload struct {
	Summary       string                 `json:"summary"`
	Severity      string                 `json:"severity"`
	Source        string                 `json:"source"`
	Timestamp     string                 `json:"timestamp,omitempty"`
	Component     string                 `json:"component,omitempty"`
	Group         string                 `json:"group,omitempty"`
	Class         string                 `json:"class,omitempty"`
	CustomDetails map[string]interface{} `json:"custom_details,omitempty"`
}

// PagerDutyLink represents a link in a PagerDuty event.
type PagerDutyLink struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

// PagerDutyImage represents an image in a PagerDuty event.
type PagerDutyImage struct {
	Src  string `json:"src"`
	Href string `json:"href,omitempty"`
	Alt  string `json:"alt,omitempty"`
}

// NewPagerDutyNotifier creates a new PagerDuty notifier.
func NewPagerDutyNotifier(routingKey string) *PagerDutyNotifier {
	return &PagerDutyNotifier{
		RoutingKey: routingKey,
		Endpoint:   "https://events.pagerduty.com/v2/enqueue",
		Source:     "keystone-secrets",
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns the notifier name.
func (p *PagerDutyNotifier) Name() string {
	return "pagerduty"
}

// Notify sends a notification to PagerDuty.
func (p *PagerDutyNotifier) Notify(ctx context.Context, n *Notification) error {
	eventAction := p.getEventAction(n.Event)
	severity := p.getSeverity(n.Severity)

	event := PagerDutyEvent{
		RoutingKey:  p.RoutingKey,
		EventAction: eventAction,
		DedupKey:    fmt.Sprintf("rotation-%s", n.RotationID),
		Payload: PagerDutyPayload{
			Summary:   n.Message,
			Severity:  severity,
			Source:    p.Source,
			Timestamp: n.Timestamp.Format(time.RFC3339),
			Component: "secrets-rotation",
			Group:     n.SecretPath,
			Class:     string(n.Event),
			CustomDetails: map[string]interface{}{
				"rotation_id": n.RotationID,
				"secret_path": n.SecretPath,
				"strategy":    n.Strategy,
				"state":       n.State,
			},
		},
	}

	if n.Progress != nil {
		event.Payload.CustomDetails["progress"] = map[string]interface{}{
			"total_targets":   n.Progress.TotalTargets,
			"updated_targets": n.Progress.UpdatedTargets,
			"failed_targets":  n.Progress.FailedTargets,
			"percentage":      n.Progress.Percentage,
		}
	}

	if n.Error != "" {
		event.Payload.CustomDetails["error"] = n.Error
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pagerduty returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (p *PagerDutyNotifier) getEventAction(event NotificationEvent) string {
	switch event {
	case NotificationEventStart, NotificationEventProgress:
		return "trigger"
	case NotificationEventComplete:
		return "resolve"
	case NotificationEventFailed, NotificationEventRollback:
		return "trigger"
	default:
		return "trigger"
	}
}

func (p *PagerDutyNotifier) getSeverity(severity NotificationSeverity) string {
	switch severity {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	case SeverityCritical:
		return "critical"
	default:
		return "info"
	}
}

// NotificationManager manages multiple notifiers and sends notifications.
type NotificationManager struct {
	notifiers []Notifier
}

// NewNotificationManager creates a new notification manager.
func NewNotificationManager() *NotificationManager {
	return &NotificationManager{
		notifiers: make([]Notifier, 0),
	}
}

// AddNotifier adds a notifier to the manager.
func (nm *NotificationManager) AddNotifier(notifier Notifier) {
	nm.notifiers = append(nm.notifiers, notifier)
}

// RemoveNotifier removes a notifier by name.
func (nm *NotificationManager) RemoveNotifier(name string) {
	for i, n := range nm.notifiers {
		if n.Name() == name {
			nm.notifiers = append(nm.notifiers[:i], nm.notifiers[i+1:]...)
			return
		}
	}
}

// Notify sends a notification to all registered notifiers.
func (nm *NotificationManager) Notify(ctx context.Context, n *Notification) []error {
	var errors []error

	for _, notifier := range nm.notifiers {
		if err := notifier.Notify(ctx, n); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", notifier.Name(), err))
		}
	}

	return errors
}

// NotifyAsync sends a notification to all notifiers asynchronously.
func (nm *NotificationManager) NotifyAsync(ctx context.Context, n *Notification) {
	for _, notifier := range nm.notifiers {
		go func(notif Notifier) {
			_ = notif.Notify(ctx, n)
		}(notifier)
	}
}

// CreateNotification creates a notification from a managed rotation.
func CreateNotification(rotation *ManagedRotation, event NotificationEvent, message string) *Notification {
	severity := SeverityInfo
	switch event {
	case NotificationEventFailed:
		severity = SeverityError
	case NotificationEventRollback:
		severity = SeverityWarning
	default:
	}

	n := &Notification{
		Event:      event,
		Severity:   severity,
		RotationID: rotation.Rotation.ID,
		SecretPath: rotation.Rotation.SecretPath,
		Strategy:   rotation.Rotation.Strategy,
		State:      rotation.State(),
		Message:    message,
		Timestamp:  time.Now(),
	}

	progress := rotation.GetProgress()
	n.Progress = &progress

	if err := rotation.Error(); err != nil {
		n.Error = err.Error()
	}

	return n
}

// Ensure implementations satisfy the interface.
var (
	_ Notifier = (*SlackNotifier)(nil)
	_ Notifier = (*PagerDutyNotifier)(nil)
)
