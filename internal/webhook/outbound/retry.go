package outbound

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// retryDelivery retries a failed webhook delivery with exponential backoff.
// It records each attempt in the store and stops after maxRetries.
func retryDelivery(
	ctx context.Context,
	store SubscriptionStore,
	dispatcher *Dispatcher,
	sub *Subscription,
	payload []byte,
	event *events.Event,
	maxRetries int,
	initialBackoff time.Duration,
) {
	backoff := initialBackoff
	for attempt := 2; attempt <= maxRetries+1; attempt++ {
		delay := backoff + jitter(backoff)
		if err := wait.ForContext(ctx, delay); err != nil {
			return // context canceled
		}

		deliveryID := generateID()
		code, err := dispatcher.Deliver(ctx, sub, payload, deliveryID)

		status := DeliverySuccess
		errMsg := ""
		if err != nil {
			if attempt <= maxRetries {
				status = DeliveryRetrying
			} else {
				status = DeliveryFailed
			}
			errMsg = err.Error()
		}

		record := &DeliveryRecord{
			ID:             deliveryID,
			SubscriptionID: sub.ID,
			EventType:      string(event.Type),
			EventID:        event.ID,
			Status:         status,
			StatusCode:     code,
			Attempt:        attempt,
			Error:          errMsg,
			DeliveredAt:    time.Now(),
		}
		_ = store.RecordDelivery(ctx, record)

		if err == nil {
			return // success
		}

		backoff *= 2
	}
}

// jitter adds up to 10% random jitter to a duration.
func jitter(d time.Duration) time.Duration {
	tenth := d / 10
	if tenth <= 0 {
		return 0
	}
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	// Use only 7 bytes to stay within int64 positive range.
	n := int64(binary.LittleEndian.Uint64(buf[:]) >> 1) //nolint:gosec // right-shift ensures positive int64
	return time.Duration(n % int64(tenth))
}
