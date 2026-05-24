// SPDX-License-Identifier: Apache-2.0

package verification

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxHTTPSnippet caps the response body retained in Result.Data.
const maxHTTPSnippet = 4 << 10 // 4 KiB

// defaultHTTPTimeout bounds a single HTTP probe when the caller does
// not inject a client. The task-6 engine layers a per-step ctx
// deadline on top.
const defaultHTTPTimeout = 10 * time.Second

// HTTPVerifier probes an HTTP endpoint. Config:
//
//	url                  (required) target URL
//	method               (default GET)
//	headers              (map[string]string)
//	body                 (string) request body
//	expect_status        (int) required status; 0 ⇒ any 2xx passes
//	expect_body_contains (string) substring the body must contain
//
// Client is injectable for tests (httptest); a nil Client uses a
// default with [defaultHTTPTimeout].
type HTTPVerifier struct {
	Client *http.Client
}

// Type implements [Verifier].
func (HTTPVerifier) Type() string { return "http" }

func (v HTTPVerifier) client() *http.Client {
	if v.Client != nil {
		return v.Client
	}
	return &http.Client{Timeout: defaultHTTPTimeout}
}

// Verify implements [Verifier]. Success when the response status
// matches expect_status (or is any 2xx when unset) and, if
// expect_body_contains is set, the body contains it.
func (v HTTPVerifier) Verify(ctx context.Context, step Step) Result {
	start := time.Now()

	url, err := cfgString(step.Config, "url")
	if err != nil {
		return failf(start, err, "http: %v", err)
	}
	method := strings.ToUpper(cfgStringOpt(step.Config, "method", http.MethodGet))
	expectStatus, err := cfgIntOpt(step.Config, "expect_status", 0)
	if err != nil {
		return failf(start, err, "http: %v", err)
	}
	wantBody := cfgStringOpt(step.Config, "expect_body_contains", "")
	headers, err := cfgStringMap(step.Config, "headers")
	if err != nil {
		return failf(start, err, "http: %v", err)
	}

	var bodyReader io.Reader
	if b := cfgStringOpt(step.Config, "body", ""); b != "" {
		bodyReader = strings.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return failf(start, fmt.Errorf("%w: build request: %v", ErrConfig, err), "http: build request: %v", err)
	}
	for k, h := range headers {
		req.Header.Set(k, h)
	}

	resp, err := v.client().Do(req)
	if err != nil {
		return failf(start, err, "http: request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxHTTPSnippet))
	data := map[string]any{
		"status":       resp.StatusCode,
		"body_snippet": string(raw),
	}

	if expectStatus != 0 {
		if resp.StatusCode != expectStatus {
			r := failf(start, nil, "http: status %d, want %d", resp.StatusCode, expectStatus)
			r.Data = data
			return r
		}
	} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		r := failf(start, nil, "http: status %d is not 2xx", resp.StatusCode)
		r.Data = data
		return r
	}

	if wantBody != "" && !strings.Contains(string(raw), wantBody) {
		r := failf(start, nil, "http: body does not contain %q", wantBody)
		r.Data = data
		return r
	}

	return Result{
		Success:  true,
		Message:  fmt.Sprintf("http: %s %s → %d", method, url, resp.StatusCode),
		Data:     data,
		Duration: time.Since(start),
	}
}
