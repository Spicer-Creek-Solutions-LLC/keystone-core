package steps

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.keystone-core.io/keystone-core/internal/runbook"
)

// maxAPIBody caps the response body the api step reads into outputs.
const maxAPIBody = 1 << 20 // 1 MiB

// apiStep performs an HTTP request. config: url (required), method
// (default GET), headers (map), body (string). When config.expect_status
// is set and the response status differs, the step fails. Outputs:
// status, body, body_truncated.
func (d Deps) apiStep(ctx context.Context, sc runbook.StepContext) (runbook.StepOutput, error) {
	url, err := cfgString(sc.Config, "url")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	method := strings.ToUpper(cfgStringOpt(sc.Config, "method", http.MethodGet))
	var bodyReader io.Reader
	if b := cfgStringOpt(sc.Config, "body", ""); b != "" {
		bodyReader = strings.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return runbook.StepOutput{}, fmt.Errorf("%w: build request: %v", ErrStepConfig, err)
	}
	headers, err := cfgStringMap(sc.Config, "headers")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := d.httpClient().Do(req)
	if err != nil {
		return runbook.StepOutput{}, fmt.Errorf("steps: api request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	limited := io.LimitReader(resp.Body, maxAPIBody+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return runbook.StepOutput{}, fmt.Errorf("steps: api read body: %w", err)
	}
	truncated := len(raw) > maxAPIBody
	if truncated {
		raw = raw[:maxAPIBody]
	}

	out := runbook.StepOutput{Outputs: map[string]any{
		"status":         resp.StatusCode,
		"body":           string(raw),
		"body_truncated": truncated,
	}}

	if v, ok := sc.Config["expect_status"]; ok {
		want, ok := toInt(v)
		if !ok {
			return out, fmt.Errorf("%w: expect_status must be an integer, got %T", ErrStepConfig, v)
		}
		if resp.StatusCode != want {
			return out, fmt.Errorf("steps: api: status %d, expected %d", resp.StatusCode, want)
		}
	}
	return out, nil
}

// toInt accepts the int/int64/float64 shapes YAML/JSON decoding and
// template rendering can produce.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
