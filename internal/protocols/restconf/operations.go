package restconf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/protocols/rest"
)

// GetData retrieves YANG data from the specified path.
func (a *Adapter) GetData(ctx context.Context, path string, opts *protocols.RestconfQueryOptions) ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected {
		return nil, fmt.Errorf("not connected")
	}

	u := a.buildDataURL(path, opts)
	body, _, _, err := a.doRequest(ctx, http.MethodGet, u, nil, string(a.config.Encoding))
	return body, err
}

// PostData creates a new data resource at the specified path.
func (a *Adapter) PostData(ctx context.Context, path string, data []byte) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected {
		return fmt.Errorf("not connected")
	}

	u := a.buildDataURL(path, nil)
	_, _, _, err := a.doRequest(ctx, http.MethodPost, u, data, string(a.config.Encoding))
	return err
}

// PutData creates or replaces a data resource at the specified path.
func (a *Adapter) PutData(ctx context.Context, path string, data []byte) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected {
		return fmt.Errorf("not connected")
	}

	u := a.buildDataURL(path, nil)
	_, _, _, err := a.doRequest(ctx, http.MethodPut, u, data, string(a.config.Encoding))
	return err
}

// PatchData merges data into an existing resource at the specified path.
func (a *Adapter) PatchData(ctx context.Context, path string, data []byte) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected {
		return fmt.Errorf("not connected")
	}

	u := a.buildDataURL(path, nil)
	_, _, _, err := a.doRequest(ctx, http.MethodPatch, u, data, string(a.config.Encoding))
	return err
}

// DeleteData removes a data resource at the specified path.
func (a *Adapter) DeleteData(ctx context.Context, path string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected {
		return fmt.Errorf("not connected")
	}

	u := a.buildDataURL(path, nil)
	_, _, _, err := a.doRequest(ctx, http.MethodDelete, u, nil, string(a.config.Encoding))
	return err
}

// InvokeOperation calls a YANG RPC or action (RFC 8040 Section 3.6).
func (a *Adapter) InvokeOperation(ctx context.Context, operation string, input []byte) ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected {
		return nil, fmt.Errorf("not connected")
	}

	u := a.rootPath + "/operations/" + strings.TrimPrefix(operation, "/")
	body, _, _, err := a.doRequest(ctx, http.MethodPost, u, input, string(a.config.Encoding))
	return body, err
}

// buildDataURL constructs a RESTCONF data URL with optional query parameters.
func (a *Adapter) buildDataURL(path string, opts *protocols.RestconfQueryOptions) string {
	u := a.rootPath + "/data/" + strings.TrimPrefix(path, "/")

	if opts == nil {
		return u
	}

	params := url.Values{}
	if opts.Depth > 0 {
		params.Set("depth", strconv.Itoa(opts.Depth))
	}
	if opts.Fields != "" {
		params.Set("fields", opts.Fields)
	}
	if opts.Content != "" {
		params.Set("content", opts.Content)
	}
	if opts.WithDefaults != "" {
		params.Set("with-defaults", opts.WithDefaults)
	}
	if opts.Filter != "" {
		params.Set("filter", opts.Filter)
	}

	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	return u
}

// doRequest executes an HTTP request with auth, media type handling, and error parsing.
func (a *Adapter) doRequest(ctx context.Context, method, path string, body []byte, acceptType string) (respBody []byte, headers http.Header, status int, err error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	// Split path and query string since rest.Client.NewRequest treats the
	// entire argument as a URL path component.
	reqPath := path
	var rawQuery string
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		reqPath = path[:idx]
		rawQuery = path[idx+1:]
	}

	req, err := a.client.NewRequest(ctx, method, reqPath, bodyReader)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("create request: %w", err)
	}
	if rawQuery != "" {
		req.URL.RawQuery = rawQuery
	}

	if acceptType != "" {
		req.Header.Set("Accept", acceptType)
	}
	if body != nil {
		req.Header.Set("Content-Type", string(a.config.Encoding))
	}

	if a.auth != nil {
		if err := a.auth.Authenticate(req); err != nil {
			return nil, nil, 0, fmt.Errorf("authenticate: %w", err)
		}
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	r, err := rest.NewResponse(resp)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("read response: %w", err)
	}

	if IsErrorResponse(r.StatusCode) {
		rcErr, parseErr := ParseErrorResponse(r.Body(), r.Header.Get("Content-Type"))
		if parseErr == nil && rcErr.HasError() {
			return nil, r.Header, r.StatusCode, rcErr
		}
		return nil, r.Header, r.StatusCode, fmt.Errorf("HTTP %d: %s", r.StatusCode, r.Body())
	}

	return r.Body(), r.Header, r.StatusCode, nil
}

// executeCommand parses a command string and dispatches to the appropriate operation.
// Supports both RESTCONF commands and raw HTTP methods.
func (a *Adapter) executeCommand(ctx context.Context, command string) ([]byte, error) {
	parts := strings.SplitN(strings.TrimSpace(command), " ", 3)
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("empty command")
	}

	op := strings.ToLower(parts[0])
	switch op {
	case "get-data":
		if len(parts) < 2 {
			return nil, fmt.Errorf("get-data requires path")
		}
		return a.doGetData(ctx, parts[1], nil)

	case "post-data":
		if len(parts) < 3 {
			return nil, fmt.Errorf("post-data requires path and body")
		}
		return nil, a.doPostData(ctx, parts[1], []byte(parts[2]))

	case "put-data":
		if len(parts) < 3 {
			return nil, fmt.Errorf("put-data requires path and body")
		}
		return nil, a.doPutData(ctx, parts[1], []byte(parts[2]))

	case "patch-data":
		if len(parts) < 3 {
			return nil, fmt.Errorf("patch-data requires path and body")
		}
		return nil, a.doPatchData(ctx, parts[1], []byte(parts[2]))

	case "delete-data":
		if len(parts) < 2 {
			return nil, fmt.Errorf("delete-data requires path")
		}
		return nil, a.doDeleteData(ctx, parts[1])

	case "invoke":
		if len(parts) < 2 {
			return nil, fmt.Errorf("invoke requires operation name")
		}
		var input []byte
		if len(parts) > 2 {
			input = []byte(parts[2])
		}
		return a.doInvokeOperation(ctx, parts[1], input)

	case "yang-library":
		return a.doYANGLibrary(ctx)

	case "modules":
		return a.doModules(ctx)

	case "get", "post", "put", "patch", "delete", "head", "options":
		return a.doRawHTTP(ctx, strings.ToUpper(parts[0]), parts)

	default:
		return nil, fmt.Errorf("unknown RESTCONF operation: %s", op)
	}
}

// Internal operation methods (no locking, called from locked contexts via Execute).

func (a *Adapter) doGetData(ctx context.Context, path string, opts *protocols.RestconfQueryOptions) ([]byte, error) {
	u := a.buildDataURL(path, opts)
	body, _, _, err := a.doRequest(ctx, http.MethodGet, u, nil, string(a.config.Encoding))
	return body, err
}

func (a *Adapter) doPostData(ctx context.Context, path string, data []byte) error {
	u := a.buildDataURL(path, nil)
	_, _, _, err := a.doRequest(ctx, http.MethodPost, u, data, string(a.config.Encoding))
	return err
}

func (a *Adapter) doPutData(ctx context.Context, path string, data []byte) error {
	u := a.buildDataURL(path, nil)
	_, _, _, err := a.doRequest(ctx, http.MethodPut, u, data, string(a.config.Encoding))
	return err
}

func (a *Adapter) doPatchData(ctx context.Context, path string, data []byte) error {
	u := a.buildDataURL(path, nil)
	_, _, _, err := a.doRequest(ctx, http.MethodPatch, u, data, string(a.config.Encoding))
	return err
}

func (a *Adapter) doDeleteData(ctx context.Context, path string) error {
	u := a.buildDataURL(path, nil)
	_, _, _, err := a.doRequest(ctx, http.MethodDelete, u, nil, string(a.config.Encoding))
	return err
}

func (a *Adapter) doInvokeOperation(ctx context.Context, operation string, input []byte) ([]byte, error) {
	u := a.rootPath + "/operations/" + strings.TrimPrefix(operation, "/")
	body, _, _, err := a.doRequest(ctx, http.MethodPost, u, input, string(a.config.Encoding))
	return body, err
}

func (a *Adapter) doYANGLibrary(ctx context.Context) ([]byte, error) {
	u := a.rootPath + "/yang-library-version"
	body, _, _, err := a.doRequest(ctx, http.MethodGet, u, nil, string(ContentTypeYANGJSON))
	return body, err
}

func (a *Adapter) doModules(ctx context.Context) ([]byte, error) {
	u := a.rootPath + "/data/ietf-yang-library:modules-state"
	body, _, _, err := a.doRequest(ctx, http.MethodGet, u, nil, string(ContentTypeYANGJSON))
	return body, err
}

func (a *Adapter) doRawHTTP(ctx context.Context, method string, parts []string) ([]byte, error) {
	path := "/"
	if len(parts) > 1 {
		path = parts[1]
	}
	var body []byte
	if len(parts) > 2 {
		body = []byte(parts[2])
	}
	result, _, _, err := a.doRequest(ctx, method, path, body, string(a.config.Encoding))
	return result, err
}
