// SPDX-License-Identifier: Apache-2.0

package outbound

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultDispatchTimeout is the per-delivery deadline when neither
// [Subscription.TimeoutSec] nor [HTTPDispatcher.DefaultTimeout] is
// set. Matches the §4.14 default (10s).
const DefaultDispatchTimeout = 10 * time.Second

// maxErrorBodyBytes caps the response body included in a non-2xx
// error message — receivers occasionally return large debug payloads
// the operator doesn't need verbatim in logs.
const maxErrorBodyBytes = 1024

// signatureHeader is the GitHub-compatible HMAC header (§4.14: same
// scheme, different name). The receiver-side helper that validates
// it lands with task 17.
const signatureHeader = "X-Keystone-Signature"

// deliveryHeader carries the per-delivery id so receivers can dedup
// or correlate replays.
const deliveryHeader = "X-Keystone-Delivery"

// HTTPDispatcher posts an event payload to one [Subscription] with
// HMAC-SHA256 signing and the per-subscription timeout. Implements
// the task-12 [Dispatcher] interface.
//
// Empty [Subscription.Secret] disables signing (no signature header)
// — matches GitHub semantics; signing with an empty key would be
// useless to the receiver.
type HTTPDispatcher struct {
	// HTTPClient is the client used for every delivery. Nil falls
	// back to a default with sane transport defaults.
	HTTPClient *http.Client
	// DefaultTimeout is used when [Subscription.TimeoutSec] is 0.
	// Zero → [DefaultDispatchTimeout].
	DefaultTimeout time.Duration
}

func (d *HTTPDispatcher) client() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return &http.Client{Transport: http.DefaultTransport}
}

// Deliver implements the task-12 [Dispatcher].
func (d *HTTPDispatcher) Deliver(ctx context.Context, sub *Subscription, payload []byte, deliveryID string) (int, error) {
	timeout := time.Duration(sub.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = d.DefaultTimeout
	}
	if timeout <= 0 {
		timeout = DefaultDispatchTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(cctx, http.MethodPost, sub.URL, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("dispatcher: build request: %w", err)
	}
	applyHeaders(req, sub, payload, deliveryID)

	resp, err := d.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	return resp.StatusCode, fmt.Errorf("dispatcher: http %d: %s", resp.StatusCode, string(body))
}

// applyHeaders writes operator-supplied headers first, then sets the
// dispatcher's own (so our Content-Type / signature / delivery-id
// take precedence over a misconfigured Headers entry).
func applyHeaders(req *http.Request, sub *Subscription, payload []byte, deliveryID string) {
	for k, v := range sub.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(deliveryHeader, deliveryID)
	if sub.Secret != "" {
		req.Header.Set(signatureHeader, Sign([]byte(sub.Secret), payload))
	}
}

// compile-time assertion: HTTPDispatcher satisfies the task-12 seam.
var _ Dispatcher = (*HTTPDispatcher)(nil)
