package outbound

import "time"

// Subscription is an operator-declared outbound webhook destination —
// one row in the §4.14 `subscriptions` table. Secret is the HMAC key
// the [Dispatcher] (task 13) signs payloads with. v1.0 stores it
// plaintext (the §4.14 contract); the REST/CLI surface (task 16)
// masks it in responses.
type Subscription struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	Secret     string            `json:"secret,omitempty"`
	Events     []string          `json:"events,omitempty"`
	Enabled    bool              `json:"enabled"`
	Headers    map[string]string `json:"headers,omitempty"`
	MaxRetries int               `json:"max_retries"`
	TimeoutSec int               `json:"timeout_sec"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// DeliveryStatus is the lifecycle of a single [DeliveryRecord]: a row
// enters as `pending`, may transition to `retrying`, and ends at
// either `success` or `failed` (history is retained on failure).
type DeliveryStatus string

const (
	DeliveryPending  DeliveryStatus = "pending"
	DeliveryRetrying DeliveryStatus = "retrying"
	DeliverySuccess  DeliveryStatus = "success"
	DeliveryFailed   DeliveryStatus = "failed"
)

// Valid reports whether s is one of the four §4.14 statuses. Returned
// records are validated on Save; this helper lets the REST handler
// (task 16) reject unknown statuses in PATCH bodies if any.
func (s DeliveryStatus) Valid() bool {
	switch s {
	case DeliveryPending, DeliveryRetrying, DeliverySuccess, DeliveryFailed:
		return true
	default:
		return false
	}
}

// DeliveryRecord is one delivery attempt — a row in the §4.14
// `deliveries` table. The [Dispatcher] (task 13) and [RetryQueue]
// (task 14) upsert the same row across attempts; on exhaustion the
// row remains with `failed` for audit (§4.14).
type DeliveryRecord struct {
	ID             string         `json:"id"`
	SubscriptionID string         `json:"subscription_id"`
	EventType      string         `json:"event_type"`
	EventID        string         `json:"event_id"`
	Status         DeliveryStatus `json:"status"`
	StatusCode     int            `json:"status_code"`
	Attempt        int            `json:"attempt"`
	Error          string         `json:"error,omitempty"`
	DeliveredAt    time.Time      `json:"delivered_at"`
}
