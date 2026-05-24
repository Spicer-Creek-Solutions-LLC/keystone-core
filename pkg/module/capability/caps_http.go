// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gobwas/glob"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// HTTPCap is the shared scoped HTTP capability (http.get / http.post
// differ only by method). Enforces: request host ∈ domain
// allowlist; request/response body ≤ limits; rate limit; per-call
// timeout.
type HTTPCap struct {
	method  string
	domains []glob.Glob
	maxReq  int64
	maxResp int64
	timeout time.Duration
	limiter *tokenBucket
	host    HTTPHost
}

func newHTTPCap(method string, cfg manifest.CapabilityConfig, host HTTPHost) (*HTTPCap, error) {
	// Domains are host patterns ("example.com", "*.example.com") —
	// no path separator, so `*` may span dots.
	doms := make([]glob.Glob, 0, len(cfg.Domains))
	for _, d := range cfg.Domains {
		g, err := glob.Compile(d)
		if err != nil {
			return nil, fmt.Errorf("http: invalid domain glob %q: %w", d, err)
		}
		doms = append(doms, g)
	}
	maxReq, err := sizeLimit(cfg.MaxRequestSize)
	if err != nil {
		return nil, fmt.Errorf("http max_request_size: %w", err)
	}
	maxResp, err := sizeLimit(cfg.MaxResponseSize)
	if err != nil {
		return nil, fmt.Errorf("http max_response_size: %w", err)
	}
	lim, err := newRateLimiter(cfg.RateLimit)
	if err != nil {
		return nil, fmt.Errorf("http rate_limit: %w", err)
	}
	var to time.Duration
	if cfg.Timeout != "" {
		if to, err = time.ParseDuration(cfg.Timeout); err != nil {
			return nil, fmt.Errorf("http timeout: %w", err)
		}
	}
	return &HTTPCap{
		method: method, domains: doms, maxReq: maxReq, maxResp: maxResp,
		timeout: to, limiter: lim, host: host,
	}, nil
}

func (c *HTTPCap) domainAllowed(hostName string) bool {
	for _, g := range c.domains {
		if g.Match(hostName) {
			return true
		}
	}
	return false
}

// Call performs the request to url with the optional body, enforcing
// every scope. Returns the response body (already size-checked).
func (c *HTTPCap) Call(ctx context.Context, url string, body []byte) ([]byte, int, error) {
	if c.host == nil {
		return nil, 0, fmt.Errorf("http: %w", ErrHostUnavailable)
	}
	if !c.limiter.allow() {
		return nil, 0, fmt.Errorf("%w: http", ErrRateLimited)
	}
	if err := withinSize(int64(len(body)), c.maxReq); err != nil {
		return nil, 0, err
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, c.method, url, rdr)
	if err != nil {
		return nil, 0, err
	}
	if !c.domainAllowed(req.URL.Hostname()) {
		return nil, 0, fmt.Errorf("%w: %q", ErrDomainDenied, req.URL.Hostname())
	}
	resp, err := c.host.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	limit := c.maxResp
	var rd io.Reader = resp.Body
	if limit > 0 {
		// Read one extra byte to detect an over-limit body.
		rd = io.LimitReader(resp.Body, limit+1)
	}
	data, err := io.ReadAll(rd)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if err := withinSize(int64(len(data)), limit); err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// NewHTTPGet / NewHTTPPost are the public constructors.
func NewHTTPGet(cfg manifest.CapabilityConfig, host HTTPHost) (*HTTPCap, error) {
	return newHTTPCap(http.MethodGet, cfg, host)
}

func NewHTTPPost(cfg manifest.CapabilityConfig, host HTTPHost) (*HTTPCap, error) {
	return newHTTPCap(http.MethodPost, cfg, host)
}
