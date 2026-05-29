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
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// maxAttempts is the total number of tries (1 initial + retries) for a
	// request that hits a rate limit (429) or a transient server error.
	maxAttempts = 5
	// maxRetryWait caps how long a single Retry-After / backoff sleep can be,
	// so a misbehaving server can't wedge the tool indefinitely.
	maxRetryWait = 60 * time.Second
	// rateLimitMargin is added to a parsed rate-limit window before retrying, so
	// the retry lands just past the window boundary rather than on it.
	rateLimitMargin = 15 * time.Second
)

// rateLimitWindowRe matches the window in Forgejo's issue-creation rate-limit
// message, e.g. `posted 5 issues in under 5 minutes`. Forgejo does not send a
// Retry-After header for this limit, but it states the window in the body.
var rateLimitWindowRe = regexp.MustCompile(`in under (\d+) minute`)

// parseRateLimitWindow extracts the rate-limit window from a 429 response body.
// Returns false when the body carries no recognisable window.
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

// client is a minimal Forgejo (Gitea-compatible) REST client scoped to one repo.
type client struct {
	base     string // e.g. https://codeberg.org
	repo     string // owner/name
	token    string
	http     *http.Client
	throttle time.Duration       // optional pause before each mutating request (POST/PATCH/DELETE)
	maxWait  time.Duration       // budget for waiting out server-stated rate-limit windows; 0 = fail fast
	sleep    func(time.Duration) // pluggable for tests; defaults to time.Sleep
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

// retryableStatus reports whether an HTTP status warrants a retry: 429 (rate
// limited) or a transient upstream/server error.
func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}

// retryWait picks how long to sleep before the next attempt: honour a
// Retry-After header if the server sent one, otherwise exponential backoff with
// jitter. Capped by maxRetryWait.
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
	base := time.Duration(1<<uint(attempt)) * 500 * time.Millisecond // 0.5s, 1s, 2s, 4s, …
	// #nosec G404 -- retry-backoff jitter; cryptographic randomness is not needed or wanted here.
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
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyBytes = b
	}
	u := c.base + "/api/v1/repos/" + c.repo + path
	mutating := method != http.MethodGet && method != http.MethodHead

	var lastErr error
	var rateWaited time.Duration // cumulative time spent waiting out rate-limit windows this call
	transient := 0               // count of transient (network / 5xx / headerless-429) retries
	for {
		if mutating && c.throttle > 0 {
			c.sleep(c.throttle)
		}
		var rdr io.Reader
		if bodyBytes != nil {
			rdr = bytes.NewReader(bodyBytes)
		}
		// #nosec G704 -- the target host is the operator-supplied --host flag; this is a CLI admin tool, not a server handling untrusted input.
		req, err := http.NewRequest(method, u, rdr)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "token "+c.token)
		req.Header.Set("Accept", "application/json")
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		// #nosec G704 -- request URL derives from the operator-supplied --host flag (see above).
		resp, err := c.http.Do(req)
		if err != nil {
			// network-level failure: retry a couple of times with backoff.
			lastErr = err
			if transient < maxAttempts-1 {
				wait := retryWait(nil, transient)
				fmt.Fprintf(os.Stderr, "trackerctl: %s %s failed (%v); retrying in %s (attempt %d/%d)\n", method, u, err, wait.Round(time.Millisecond), transient+2, maxAttempts)
				c.sleep(wait)
				transient++
				continue
			}
			return lastErr
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Windowed rate limit: Forgejo states the window in the 429 body (no
		// Retry-After header). When given a --max-wait budget, sleep the window
		// out and retry — this is progress, not a transient failure, so it does
		// not consume the bounded transient-retry budget.
		if resp.StatusCode == http.StatusTooManyRequests && c.maxWait > 0 {
			if win, ok := parseRateLimitWindow(data); ok {
				wait := win + rateLimitMargin
				if rateWaited+wait <= c.maxWait {
					fmt.Fprintf(os.Stderr, "trackerctl: rate limited (%s window) — waiting %s then retrying %s %s (waited %s/%s)\n", win, wait.Round(time.Second), method, u, (rateWaited + wait).Round(time.Second), c.maxWait)
					c.sleep(wait)
					rateWaited += wait
					transient = 0
					continue
				}
				return fmt.Errorf("%s %s: rate limited, would exceed --max-wait %s: %s", method, u, c.maxWait, strings.TrimSpace(string(data)))
			}
		}

		if retryableStatus(resp.StatusCode) && transient < maxAttempts-1 {
			wait := retryWait(resp, transient)
			reason := "rate limited"
			if resp.StatusCode != http.StatusTooManyRequests {
				reason = fmt.Sprintf("server returned %s", resp.Status)
			}
			fmt.Fprintf(os.Stderr, "trackerctl: %s — backing off %s then retrying %s %s (attempt %d/%d)\n", reason, wait.Round(time.Millisecond), method, u, transient+2, maxAttempts)
			c.sleep(wait)
			transient++
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("%s %s: %s: %s", method, u, resp.Status, strings.TrimSpace(string(data)))
		}
		if out != nil && len(data) > 0 {
			return json.Unmarshal(data, out)
		}
		return nil
	}
}

// getPaged fetches a paginated collection, appending each page into out via the
// supplied accumulator until a short page is returned.
func (c *client) getPaged(path string, fetch func(page int) (int, error)) error {
	for page := 1; ; page++ {
		n, err := fetch(page)
		if err != nil {
			return err
		}
		if n < pageLimit {
			return nil
		}
	}
}

const pageLimit = 50

func pageQuery(extra url.Values, page int) string {
	v := url.Values{}
	for k, vs := range extra {
		v[k] = vs
	}
	v.Set("page", fmt.Sprint(page))
	v.Set("limit", fmt.Sprint(pageLimit))
	return "?" + v.Encode()
}

// --- labels ---

type label struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Exclusive   bool   `json:"exclusive"`
	IsArchived  bool   `json:"is_archived"`
}

type labelPayload struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Exclusive   bool   `json:"exclusive"`
}

func (c *client) listLabels() ([]label, error) {
	var all []label
	err := c.getPaged("/labels", func(page int) (int, error) {
		var batch []label
		if err := c.do(http.MethodGet, "/labels"+pageQuery(nil, page), nil, &batch); err != nil {
			return 0, err
		}
		all = append(all, batch...)
		return len(batch), nil
	})
	return all, err
}

func (c *client) createLabel(p labelPayload) (label, error) {
	var l label
	err := c.do(http.MethodPost, "/labels", p, &l)
	return l, err
}

func (c *client) editLabel(id int64, p labelPayload) (label, error) {
	var l label
	err := c.do(http.MethodPatch, fmt.Sprintf("/labels/%d", id), p, &l)
	return l, err
}

// --- milestones ---

type milestone struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	DueOn       string `json:"due_on,omitempty"`
}

type milestonePayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state,omitempty"`
	DueOn       string `json:"due_on,omitempty"`
}

func (c *client) listMilestones() ([]milestone, error) {
	var all []milestone
	q := url.Values{"state": {"all"}}
	err := c.getPaged("/milestones", func(page int) (int, error) {
		var batch []milestone
		if err := c.do(http.MethodGet, "/milestones"+pageQuery(q, page), nil, &batch); err != nil {
			return 0, err
		}
		all = append(all, batch...)
		return len(batch), nil
	})
	return all, err
}

func (c *client) createMilestone(p milestonePayload) (milestone, error) {
	var m milestone
	err := c.do(http.MethodPost, "/milestones", p, &m)
	return m, err
}

func (c *client) editMilestone(id int64, p milestonePayload) (milestone, error) {
	var m milestone
	err := c.do(http.MethodPatch, fmt.Sprintf("/milestones/%d", id), p, &m)
	return m, err
}

// --- issues ---

type issue struct {
	Number    int64      `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	Labels    []label    `json:"labels"`
	Milestone *milestone `json:"milestone"`
}

type issuePayload struct {
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Labels    []int64 `json:"labels,omitempty"`
	Milestone int64   `json:"milestone,omitempty"`
}

func (c *client) listIssues() ([]issue, error) {
	var all []issue
	q := url.Values{"state": {"all"}, "type": {"issues"}}
	err := c.getPaged("/issues", func(page int) (int, error) {
		var batch []issue
		if err := c.do(http.MethodGet, "/issues"+pageQuery(q, page), nil, &batch); err != nil {
			return 0, err
		}
		all = append(all, batch...)
		return len(batch), nil
	})
	return all, err
}

func (c *client) createIssue(p issuePayload) (issue, error) {
	var is issue
	err := c.do(http.MethodPost, "/issues", p, &is)
	return is, err
}

func (c *client) editIssueBody(index int64, body string) (issue, error) {
	var is issue
	err := c.do(http.MethodPatch, fmt.Sprintf("/issues/%d", index), map[string]string{"body": body}, &is)
	return is, err
}

// editIssueMilestone sets (milestoneID > 0) or clears (milestoneID == 0) an
// issue's milestone.
func (c *client) editIssueMilestone(index, milestoneID int64) error {
	return c.do(http.MethodPatch, fmt.Sprintf("/issues/%d", index), map[string]int64{"milestone": milestoneID}, nil)
}

func (c *client) addIssueLabels(index int64, ids []int64) error {
	return c.do(http.MethodPost, fmt.Sprintf("/issues/%d/labels", index), map[string][]int64{"labels": ids}, nil)
}

func (c *client) removeIssueLabel(index, labelID int64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/issues/%d/labels/%d", index, labelID), nil, nil)
}
