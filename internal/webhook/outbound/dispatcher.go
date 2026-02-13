package outbound

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Dispatcher delivers webhook payloads to subscription URLs.
type Dispatcher struct {
	client *http.Client
}

// NewDispatcher creates a dispatcher with the given HTTP timeout.
func NewDispatcher(timeout time.Duration) *Dispatcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Dispatcher{
		client: &http.Client{Timeout: timeout},
	}
}

// Deliver sends payload to the subscription's URL with appropriate headers.
// Returns the HTTP status code and any error.
func (d *Dispatcher) Deliver(ctx context.Context, sub *Subscription, payload []byte, deliveryID string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.URL, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(WebhookIDHeader, deliveryID)
	req.Header.Set(WebhookTimestampHeader, FormatTimestamp(time.Now().Unix()))

	if sub.Secret != "" {
		sig := Sign([]byte(sub.Secret), payload)
		req.Header.Set(SignatureHeader, sig)
	}

	for k, v := range sub.Headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("deliver webhook: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, fmt.Errorf("endpoint returned HTTP %d", resp.StatusCode)
}
