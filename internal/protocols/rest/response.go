// Package rest provides a REST/HTTP protocol adapter for proxy agents.
package rest

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Response wraps an HTTP response with helper methods.
type Response struct {
	*http.Response
	body []byte
}

// NewResponse creates a new Response from an HTTP response.
func NewResponse(resp *http.Response) (*Response, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	resp.Body.Close()

	return &Response{
		Response: resp,
		body:     body,
	}, nil
}

// Body returns the response body as bytes.
func (r *Response) Body() []byte {
	return r.body
}

// String returns the response body as a string.
func (r *Response) String() string {
	return string(r.body)
}

// JSON unmarshals the response body as JSON.
func (r *Response) JSON(v interface{}) error {
	if len(r.body) == 0 {
		return nil
	}
	return json.Unmarshal(r.body, v)
}

// XML unmarshals the response body as XML.
func (r *Response) XML(v interface{}) error {
	if len(r.body) == 0 {
		return nil
	}
	return xml.Unmarshal(r.body, v)
}

// IsSuccess returns true if the status code indicates success (2xx).
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsRedirect returns true if the status code indicates redirect (3xx).
func (r *Response) IsRedirect() bool {
	return r.StatusCode >= 300 && r.StatusCode < 400
}

// IsClientError returns true if the status code indicates client error (4xx).
func (r *Response) IsClientError() bool {
	return r.StatusCode >= 400 && r.StatusCode < 500
}

// IsServerError returns true if the status code indicates server error (5xx).
func (r *Response) IsServerError() bool {
	return r.StatusCode >= 500
}

// IsError returns true if the status code indicates any error (4xx or 5xx).
func (r *Response) IsError() bool {
	return r.StatusCode >= 400
}

// ContentType returns the response Content-Type header.
func (r *Response) ContentType() string {
	return r.Header.Get("Content-Type")
}

// IsJSON returns true if the response has a JSON content type.
func (r *Response) IsJSON() bool {
	ct := r.ContentType()
	return strings.Contains(ct, "application/json") || strings.Contains(ct, "+json")
}

// IsXML returns true if the response has an XML content type.
func (r *Response) IsXML() bool {
	ct := r.ContentType()
	return strings.Contains(ct, "application/xml") || strings.Contains(ct, "text/xml") || strings.Contains(ct, "+xml")
}

// GetHeader returns a response header value.
func (r *Response) GetHeader(key string) string {
	return r.Header.Get(key)
}

// GetHeaders returns all values for a header.
func (r *Response) GetHeaders(key string) []string {
	return r.Header.Values(key)
}

// Error returns an error if the response indicates failure.
func (r *Response) Error() error {
	if r.IsSuccess() {
		return nil
	}

	// Try to parse error from body
	if r.IsJSON() {
		var errResp struct {
			Error       string `json:"error"`
			Message     string `json:"message"`
			Description string `json:"description"`
			Detail      string `json:"detail"`
		}
		if json.Unmarshal(r.body, &errResp) == nil {
			msg := errResp.Error
			if msg == "" {
				msg = errResp.Message
			}
			if msg == "" {
				msg = errResp.Description
			}
			if msg == "" {
				msg = errResp.Detail
			}
			if msg != "" {
				return fmt.Errorf("HTTP %d: %s", r.StatusCode, msg)
			}
		}
	}

	return fmt.Errorf("HTTP %d: %s", r.StatusCode, r.Status)
}

// JSONMap parses the response body as a map.
func (r *Response) JSONMap() (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := r.JSON(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// JSONArray parses the response body as an array.
func (r *Response) JSONArray() ([]interface{}, error) {
	var result []interface{}
	if err := r.JSON(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// JSONPath extracts a value from the JSON response using a simple path.
// Path format: "key.nested.array[0].value"
func (r *Response) JSONPath(path string) (interface{}, error) {
	var data interface{}
	if err := r.JSON(&data); err != nil {
		return nil, err
	}

	return jsonPath(data, path)
}

// jsonPath extracts a value using a simple path notation.
func jsonPath(data interface{}, path string) (interface{}, error) {
	if path == "" {
		return data, nil
	}

	parts := splitPath(path)
	current := data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[part]
			if !ok {
				return nil, fmt.Errorf("key not found: %s", part)
			}
		case []interface{}:
			// Parse array index
			var idx int
			if _, err := fmt.Sscanf(part, "%d", &idx); err != nil {
				return nil, fmt.Errorf("invalid array index: %s", part)
			}
			if idx < 0 || idx >= len(v) {
				return nil, fmt.Errorf("index out of bounds: %d", idx)
			}
			current = v[idx]
		default:
			return nil, fmt.Errorf("cannot navigate into %T", current)
		}
	}

	return current, nil
}

// splitPath splits a path into parts, handling array notation.
func splitPath(path string) []string {
	var parts []string
	var current strings.Builder

	for i := 0; i < len(path); i++ {
		c := path[i]
		switch c {
		case '.':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		case '[':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		case ']':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(c)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// Pretty returns a pretty-printed version of the response body.
func (r *Response) Pretty() string {
	if r.IsJSON() {
		var data interface{}
		if err := json.Unmarshal(r.body, &data); err == nil {
			pretty, err := json.MarshalIndent(data, "", "  ")
			if err == nil {
				return string(pretty)
			}
		}
	}

	if r.IsXML() {
		// For XML, just return as-is (could implement pretty printing)
		return string(r.body)
	}

	return string(r.body)
}

// Size returns the size of the response body.
func (r *Response) Size() int {
	return len(r.body)
}

// PaginationInfo contains pagination information from a response.
type PaginationInfo struct {
	// Page is the current page number.
	Page int
	// PerPage is the number of items per page.
	PerPage int
	// TotalPages is the total number of pages.
	TotalPages int
	// TotalItems is the total number of items.
	TotalItems int
	// NextPage is the next page number (0 if no next page).
	NextPage int
	// PrevPage is the previous page number (0 if no previous page).
	PrevPage int
	// NextURL is the URL for the next page.
	NextURL string
	// PrevURL is the URL for the previous page.
	PrevURL string
	// HasMore indicates if there are more pages.
	HasMore bool
}

// GetPagination extracts pagination information from the response.
func (r *Response) GetPagination() *PaginationInfo {
	info := &PaginationInfo{}

	// Try to parse from headers (common patterns)
	if page := r.Header.Get("X-Page"); page != "" {
		fmt.Sscanf(page, "%d", &info.Page)
	}
	if perPage := r.Header.Get("X-Per-Page"); perPage != "" {
		fmt.Sscanf(perPage, "%d", &info.PerPage)
	}
	if total := r.Header.Get("X-Total"); total != "" {
		fmt.Sscanf(total, "%d", &info.TotalItems)
	}
	if totalPages := r.Header.Get("X-Total-Pages"); totalPages != "" {
		fmt.Sscanf(totalPages, "%d", &info.TotalPages)
	}

	// Parse Link header for next/prev URLs
	link := r.Header.Get("Link")
	if link != "" {
		info.NextURL = parseLinkHeader(link, "next")
		info.PrevURL = parseLinkHeader(link, "prev")
	}

	// Try to extract from JSON body
	if r.IsJSON() {
		var data map[string]interface{}
		if json.Unmarshal(r.body, &data) == nil {
			// Common pagination field names
			if page, ok := data["page"].(float64); ok {
				info.Page = int(page)
			}
			if perPage, ok := data["per_page"].(float64); ok {
				info.PerPage = int(perPage)
			}
			if total, ok := data["total"].(float64); ok {
				info.TotalItems = int(total)
			}
			if totalPages, ok := data["total_pages"].(float64); ok {
				info.TotalPages = int(totalPages)
			}
			if hasMore, ok := data["has_more"].(bool); ok {
				info.HasMore = hasMore
			}
			if nextURL, ok := data["next"].(string); ok {
				info.NextURL = nextURL
			}
			if prevURL, ok := data["previous"].(string); ok {
				info.PrevURL = prevURL
			}
		}
	}

	// Calculate HasMore if not explicitly set
	if !info.HasMore && info.NextURL != "" {
		info.HasMore = true
	}
	if !info.HasMore && info.TotalPages > 0 && info.Page < info.TotalPages {
		info.HasMore = true
	}

	return info
}

// parseLinkHeader parses a Link header and extracts the URL for a relation.
func parseLinkHeader(header, rel string) string {
	parts := strings.Split(header, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="`+rel+`"`) {
			continue
		}

		// Extract URL from <url>; rel="..."
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start >= 0 && end > start {
			return part[start+1 : end]
		}
	}
	return ""
}

// RateLimit contains rate limit information from a response.
type RateLimit struct {
	// Limit is the maximum number of requests allowed.
	Limit int
	// Remaining is the number of requests remaining.
	Remaining int
	// Reset is the time when the rate limit resets.
	Reset int64
	// RetryAfter is the number of seconds to wait before retrying.
	RetryAfter int
}

// GetRateLimit extracts rate limit information from the response.
func (r *Response) GetRateLimit() *RateLimit {
	info := &RateLimit{}

	// Common rate limit headers
	if limit := r.Header.Get("X-RateLimit-Limit"); limit != "" {
		fmt.Sscanf(limit, "%d", &info.Limit)
	}
	if remaining := r.Header.Get("X-RateLimit-Remaining"); remaining != "" {
		fmt.Sscanf(remaining, "%d", &info.Remaining)
	}
	if reset := r.Header.Get("X-RateLimit-Reset"); reset != "" {
		fmt.Sscanf(reset, "%d", &info.Reset)
	}
	if retryAfter := r.Header.Get("Retry-After"); retryAfter != "" {
		fmt.Sscanf(retryAfter, "%d", &info.RetryAfter)
	}

	// Alternative header names
	if info.Limit == 0 {
		if limit := r.Header.Get("RateLimit-Limit"); limit != "" {
			fmt.Sscanf(limit, "%d", &info.Limit)
		}
	}
	if info.Remaining == 0 {
		if remaining := r.Header.Get("RateLimit-Remaining"); remaining != "" {
			fmt.Sscanf(remaining, "%d", &info.Remaining)
		}
	}
	if info.Reset == 0 {
		if reset := r.Header.Get("RateLimit-Reset"); reset != "" {
			fmt.Sscanf(reset, "%d", &info.Reset)
		}
	}

	return info
}
