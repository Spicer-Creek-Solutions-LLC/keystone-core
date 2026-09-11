// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// maxAttempts is the total number of tries (1 initial + retries) for a
	// request that hits a rate limit or a transient server error.
	maxAttempts = 5
	// maxRetryWait caps a single backoff sleep so a misbehaving server cannot
	// wedge the tool indefinitely.
	maxRetryWait = 60 * time.Second
	// rateLimitMargin lands the retry just past a stated window boundary
	// rather than on it.
	rateLimitMargin = 15 * time.Second
)

// rateLimitWindowRe matches the window Forgejo states in the body of a
// rate-limit response. Forgejo sends no Retry-After header for these limits, so
// the body is the only source — and it does not phrase them consistently:
//
//	issue creation:   "posted 5 issues in under 5 minutes"
//	comment creation: "posted 16 comments in 5 minutes"
//
// The optional "under" matters. A pattern written for the first wording silently
// fails to match the second, which leaves --max-wait inert and turns a routine
// pause into a stopped run. That is exactly what happened on the first R07
// apply, 16 comments in.
var rateLimitWindowRe = regexp.MustCompile(`in (?:under )?(\d+) minute`)

func parseRateLimitWindow(body []byte) (time.Duration, bool) {
	m := rateLimitWindowRe.FindSubmatch(body)
	if m == nil {
		return 0, false
	}
	mins, err := strconv.Atoi(string(m[1]))
	if err != nil || mins <= 0 {
		return 0, false
	}
	return time.Duration(mins) * time.Minute, true
}

// client is a minimal Forgejo REST client scoped to one repository.
type client struct {
	base     string
	repo     string
	token    string
	http     *http.Client
	throttle time.Duration
	maxWait  time.Duration
	sleep    func(time.Duration)

	// calls records every request the client made, so a dry run can report
	// exactly what it would have done and a test can assert that nothing
	// outside the allowlist was touched.
	calls []Call
}

// Call is one HTTP request the client issued or would have issued.
type Call struct {
	Method string
	Path   string
	Body   string
}

func newClient(host, repo, token string, throttle, maxWait time.Duration) *client {
	return &client{
		base:     strings.TrimRight(host, "/"),
		repo:     repo,
		token:    token,
		http:     &http.Client{Timeout: 30 * time.Second},
		throttle: throttle,
		maxWait:  maxWait,
		sleep:    time.Sleep,
	}
}

// retryableStatus reports whether a status warrants another attempt for the
// given request.
//
// 500 is retryable for everything except posting a comment. A 500 is ambiguous
// — the write may have landed before the response failed — and for adding a
// label or closing an issue that ambiguity is harmless, because both are
// idempotent. Re-posting a comment is not: it would leave two identical
// retirement notices on a public issue. Codeberg returned exactly this, an
// empty-bodied 500 on a label POST, partway through the R07 apply.
func retryableStatus(code int, method, path string) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	case http.StatusInternalServerError:
		return !isCommentPost(method, path)
	}
	return false
}

func isCommentPost(method, path string) bool {
	return method == http.MethodPost && strings.HasSuffix(strings.SplitN(path, "?", 2)[0], "/comments")
}

func retryWait(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if ra := strings.TrimSpace(resp.Header.Get("Retry-After")); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs >= 0 {
				return capDuration(time.Duration(secs) * time.Second)
			}
			if t, err := http.ParseTime(ra); err == nil {
				if d := time.Until(t); d > 0 {
					return capDuration(d)
				}
				return 0
			}
		}
	}
	base := time.Duration(1<<uint(attempt)) * 500 * time.Millisecond
	// #nosec G404 -- retry-backoff jitter; cryptographic randomness is neither
	// needed nor wanted here.
	jitter := time.Duration(rand.Int63n(int64(500 * time.Millisecond)))
	return capDuration(base + jitter)
}

func capDuration(d time.Duration) time.Duration {
	if d > maxRetryWait {
		return maxRetryWait
	}
	return d
}

func (c *client) do(method, path string, body, out any) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}
	c.calls = append(c.calls, Call{Method: method, Path: path, Body: string(bodyBytes)})

	if method != http.MethodGet && c.throttle > 0 {
		c.sleep(c.throttle)
	}

	var waited time.Duration
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// #nosec G704 -- the base URL is the operator-supplied --host and the
		// path is built from this tool's own route constants plus issue numbers
		// taken from a reviewed allowlist. checkForge additionally refuses to
		// act unless the host resolves to the exact repository the manifest
		// names, which is a stronger guard than URL shape.
		req, err := http.NewRequest(method, c.base+"/api/v1"+path, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "token "+c.token)
		req.Header.Set("Content-Type", "application/json")

		// #nosec G704 -- as above.
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			c.sleep(retryWait(nil, attempt))
			continue
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			c.sleep(retryWait(resp, attempt))
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			// Forgejo states the window in the body rather than a header. With a
			// budget, sleep it out; without one, fail fast so an operator is not
			// left wondering whether a long apply is hung or waiting.
			if win, ok := parseRateLimitWindow(data); ok {
				wait := win + rateLimitMargin
				if c.maxWait == 0 || waited+wait > c.maxWait {
					return fmt.Errorf("%s %s: rate limited for %s, beyond the --max-wait budget", method, path, win)
				}
				waited += wait
				c.sleep(wait)
				continue
			}
		}
		if retryableStatus(resp.StatusCode, method, path) {
			lastErr = fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, truncate(string(data), 200))
			c.sleep(retryWait(resp, attempt))
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, truncate(string(data), 300))
		}
		if out != nil && len(data) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("decode %s %s: %w", method, path, err)
			}
		}
		return nil
	}
	return fmt.Errorf("%s %s: gave up after %d attempts: %w", method, path, maxAttempts, lastErr)
}

// getPaged walks a paginated collection until a page comes back empty. fetch
// returns the number of items on the page it read.
func (c *client) getPaged(path string, extra url.Values, fetch func(page int) (int, error)) error {
	for page := 1; ; page++ {
		n, err := fetch(page)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if page > 500 {
			return fmt.Errorf("%s: refusing to page past %d pages", path, page)
		}
	}
}

func pageQuery(extra url.Values, page int) string {
	q := url.Values{}
	for k, v := range extra {
		q[k] = v
	}
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", "50")
	return "?" + q.Encode()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
