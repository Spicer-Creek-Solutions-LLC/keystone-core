// Package outbound provides persistent outbound webhook subscriptions that deliver
// internal Keystone Core events to external HTTP endpoints with HMAC signing,
// retry logic, and delivery tracking.
package outbound

import (
	"context"
	"time"
)

// Subscription represents an outbound webhook subscription.
type Subscription struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	Secret     string            `json:"secret,omitempty"`
	Events     []string          `json:"events"`
	Enabled    bool              `json:"enabled"`
	Headers    map[string]string `json:"headers,omitempty"`
	MaxRetries int               `json:"max_retries"`
	TimeoutSec int               `json:"timeout_secs"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// DeliveryStatus represents the status of a webhook delivery attempt.
type DeliveryStatus string

// Delivery status values.
const (
	DeliveryPending  DeliveryStatus = "pending"
	DeliverySuccess  DeliveryStatus = "success"
	DeliveryFailed   DeliveryStatus = "failed"
	DeliveryRetrying DeliveryStatus = "retrying"
)

// DeliveryRecord records a single delivery attempt.
type DeliveryRecord struct {
	ID             string         `json:"id"`
	SubscriptionID string         `json:"subscription_id"`
	EventType      string         `json:"event_type"`
	EventID        string         `json:"event_id"`
	Status         DeliveryStatus `json:"status"`
	StatusCode     int            `json:"status_code,omitempty"`
	Attempt        int            `json:"attempt"`
	Error          string         `json:"error,omitempty"`
	DeliveredAt    time.Time      `json:"delivered_at"`
}

// SubscriptionStore persists outbound webhook subscriptions and delivery records.
type SubscriptionStore interface {
	Create(ctx context.Context, sub *Subscription) error
	Get(ctx context.Context, id string) (*Subscription, error)
	List(ctx context.Context) ([]*Subscription, error)
	Update(ctx context.Context, sub *Subscription) error
	Delete(ctx context.Context, id string) error
	ListEnabled(ctx context.Context) ([]*Subscription, error)

	RecordDelivery(ctx context.Context, record *DeliveryRecord) error
	ListDeliveries(ctx context.Context, subscriptionID string, limit int) ([]*DeliveryRecord, error)
	DeleteOldDeliveries(ctx context.Context, olderThan time.Duration) (int64, error)
}
