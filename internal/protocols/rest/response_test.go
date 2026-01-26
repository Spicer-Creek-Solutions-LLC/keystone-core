package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func makeResponse(statusCode int, contentType string, body []byte, headers map[string]string) *Response {
	resp := &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	resp.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		resp.Header.Set(k, v)
	}

	r, _ := NewResponse(resp)
	return r
}

func TestNewResponse(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte("test body"))),
	}

	r, err := NewResponse(resp)
	if err != nil {
		t.Fatalf("NewResponse() error = %v", err)
	}

	if string(r.body) != "test body" {
		t.Errorf("body = %v, want 'test body'", string(r.body))
	}
}

func TestResponseBody(t *testing.T) {
	r := makeResponse(200, "text/plain", []byte("test body"), nil)

	if string(r.Body()) != "test body" {
		t.Errorf("Body() = %v, want 'test body'", string(r.Body()))
	}
}

func TestResponseString(t *testing.T) {
	r := makeResponse(200, "text/plain", []byte("test body"), nil)

	if r.String() != "test body" {
		t.Errorf("String() = %v, want 'test body'", r.String())
	}
}

func TestResponseJSON(t *testing.T) {
	body := `{"key":"value","number":42}`
	r := makeResponse(200, "application/json", []byte(body), nil)

	var result map[string]interface{}
	err := r.JSON(&result)
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("key = %v, want 'value'", result["key"])
	}
	if result["number"].(float64) != 42 {
		t.Errorf("number = %v, want 42", result["number"])
	}
}

func TestResponseJSONEmptyBody(t *testing.T) {
	r := makeResponse(200, "application/json", []byte{}, nil)

	var result map[string]interface{}
	err := r.JSON(&result)
	if err != nil {
		t.Errorf("JSON() error = %v, should handle empty body", err)
	}
}

func TestResponseJSONInvalid(t *testing.T) {
	r := makeResponse(200, "application/json", []byte("not json"), nil)

	var result map[string]interface{}
	err := r.JSON(&result)
	if err == nil {
		t.Error("JSON() should return error for invalid JSON")
	}
}

func TestResponseXML(t *testing.T) {
	body := `<root><key>value</key></root>`
	r := makeResponse(200, "application/xml", []byte(body), nil)

	var result struct {
		Key string `xml:"key"`
	}
	err := r.XML(&result)
	if err != nil {
		t.Fatalf("XML() error = %v", err)
	}

	if result.Key != "value" {
		t.Errorf("key = %v, want 'value'", result.Key)
	}
}

func TestResponseXMLEmptyBody(t *testing.T) {
	r := makeResponse(200, "application/xml", []byte{}, nil)

	var result struct{}
	err := r.XML(&result)
	if err != nil {
		t.Errorf("XML() error = %v, should handle empty body", err)
	}
}

func TestResponseStatusChecks(t *testing.T) {
	tests := []struct {
		status        int
		isSuccess     bool
		isRedirect    bool
		isClientError bool
		isServerError bool
		isError       bool
	}{
		{200, true, false, false, false, false},
		{201, true, false, false, false, false},
		{204, true, false, false, false, false},
		{299, true, false, false, false, false},
		{301, false, true, false, false, false},
		{302, false, true, false, false, false},
		{304, false, true, false, false, false},
		{400, false, false, true, false, true},
		{401, false, false, true, false, true},
		{403, false, false, true, false, true},
		{404, false, false, true, false, true},
		{499, false, false, true, false, true},
		{500, false, false, false, true, true},
		{502, false, false, false, true, true},
		{503, false, false, false, true, true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			r := makeResponse(tt.status, "", nil, nil)

			if r.IsSuccess() != tt.isSuccess {
				t.Errorf("IsSuccess() = %v, want %v", r.IsSuccess(), tt.isSuccess)
			}
			if r.IsRedirect() != tt.isRedirect {
				t.Errorf("IsRedirect() = %v, want %v", r.IsRedirect(), tt.isRedirect)
			}
			if r.IsClientError() != tt.isClientError {
				t.Errorf("IsClientError() = %v, want %v", r.IsClientError(), tt.isClientError)
			}
			if r.IsServerError() != tt.isServerError {
				t.Errorf("IsServerError() = %v, want %v", r.IsServerError(), tt.isServerError)
			}
			if r.IsError() != tt.isError {
				t.Errorf("IsError() = %v, want %v", r.IsError(), tt.isError)
			}
		})
	}
}

func TestResponseContentType(t *testing.T) {
	r := makeResponse(200, "application/json; charset=utf-8", nil, nil)

	if r.ContentType() != "application/json; charset=utf-8" {
		t.Errorf("ContentType() = %v, want 'application/json; charset=utf-8'", r.ContentType())
	}
}

func TestResponseIsJSON(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"application/vnd.api+json", true},
		{"text/plain", false},
		{"application/xml", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			r := makeResponse(200, tt.contentType, nil, nil)
			if r.IsJSON() != tt.want {
				t.Errorf("IsJSON() = %v, want %v", r.IsJSON(), tt.want)
			}
		})
	}
}

func TestResponseIsXML(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"application/xml", true},
		{"text/xml", true},
		{"application/rss+xml", true},
		{"application/json", false},
		{"text/plain", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			r := makeResponse(200, tt.contentType, nil, nil)
			if r.IsXML() != tt.want {
				t.Errorf("IsXML() = %v, want %v", r.IsXML(), tt.want)
			}
		})
	}
}

func TestResponseGetHeader(t *testing.T) {
	r := makeResponse(200, "", nil, map[string]string{
		"X-Custom":     "custom-value",
		"X-Request-Id": "12345",
	})

	if r.GetHeader("X-Custom") != "custom-value" {
		t.Errorf("GetHeader(X-Custom) = %v, want 'custom-value'", r.GetHeader("X-Custom"))
	}
	if r.GetHeader("X-Request-Id") != "12345" {
		t.Errorf("GetHeader(X-Request-Id) = %v, want '12345'", r.GetHeader("X-Request-Id"))
	}
	if r.GetHeader("X-Missing") != "" {
		t.Errorf("GetHeader(X-Missing) = %v, want ''", r.GetHeader("X-Missing"))
	}
}

func TestResponseGetHeaders(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}
	resp.Header.Add("Set-Cookie", "a=1")
	resp.Header.Add("Set-Cookie", "b=2")

	r, _ := NewResponse(resp)

	cookies := r.GetHeaders("Set-Cookie")
	if len(cookies) != 2 {
		t.Errorf("GetHeaders(Set-Cookie) returned %d values, want 2", len(cookies))
	}
}

func TestResponseError(t *testing.T) {
	t.Run("success returns nil", func(t *testing.T) {
		r := makeResponse(200, "", nil, nil)
		if r.Error() != nil {
			t.Errorf("Error() = %v, want nil", r.Error())
		}
	})

	t.Run("error with JSON message", func(t *testing.T) {
		body := `{"error":"Not found"}`
		r := makeResponse(404, "application/json", []byte(body), nil)
		err := r.Error()
		if err == nil {
			t.Error("Error() = nil, want error")
		}
		if err.Error() != "HTTP 404: Not found" {
			t.Errorf("Error() = %v, want 'HTTP 404: Not found'", err.Error())
		}
	})

	t.Run("error with message field", func(t *testing.T) {
		body := `{"message":"Access denied"}`
		r := makeResponse(403, "application/json", []byte(body), nil)
		err := r.Error()
		if err.Error() != "HTTP 403: Access denied" {
			t.Errorf("Error() = %v, want 'HTTP 403: Access denied'", err.Error())
		}
	})

	t.Run("error with description field", func(t *testing.T) {
		body := `{"description":"Invalid request"}`
		r := makeResponse(400, "application/json", []byte(body), nil)
		err := r.Error()
		if err.Error() != "HTTP 400: Invalid request" {
			t.Errorf("Error() = %v, want 'HTTP 400: Invalid request'", err.Error())
		}
	})

	t.Run("error with detail field", func(t *testing.T) {
		body := `{"detail":"Something went wrong"}`
		r := makeResponse(500, "application/json", []byte(body), nil)
		err := r.Error()
		if err.Error() != "HTTP 500: Something went wrong" {
			t.Errorf("Error() = %v, want 'HTTP 500: Something went wrong'", err.Error())
		}
	})

	t.Run("error without message", func(t *testing.T) {
		r := makeResponse(500, "text/plain", []byte("error"), nil)
		err := r.Error()
		if err == nil {
			t.Error("Error() = nil, want error")
		}
	})
}

func TestResponseJSONMap(t *testing.T) {
	body := `{"key":"value","number":42}`
	r := makeResponse(200, "application/json", []byte(body), nil)

	result, err := r.JSONMap()
	if err != nil {
		t.Fatalf("JSONMap() error = %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("key = %v, want 'value'", result["key"])
	}
}

func TestResponseJSONArray(t *testing.T) {
	body := `[1,2,3]`
	r := makeResponse(200, "application/json", []byte(body), nil)

	result, err := r.JSONArray()
	if err != nil {
		t.Fatalf("JSONArray() error = %v", err)
	}

	if len(result) != 3 {
		t.Errorf("len(result) = %d, want 3", len(result))
	}
}

func TestResponseJSONPath(t *testing.T) {
	body := `{
		"user": {
			"name": "John",
			"emails": ["a@b.com", "c@d.com"]
		},
		"items": [
			{"id": 1},
			{"id": 2}
		]
	}`
	r := makeResponse(200, "application/json", []byte(body), nil)

	tests := []struct {
		path string
		want interface{}
	}{
		{"user.name", "John"},
		{"user.emails[0]", "a@b.com"},
		{"user.emails[1]", "c@d.com"},
		{"items[0].id", float64(1)},
		{"items[1].id", float64(2)},
		{"", nil}, // Empty path returns entire object
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result, err := r.JSONPath(tt.path)
			if err != nil {
				if tt.path != "" { // Empty path test
					t.Errorf("JSONPath(%q) error = %v", tt.path, err)
				}
				return
			}

			if tt.path == "" {
				// Empty path should return full object
				return
			}

			if result != tt.want {
				t.Errorf("JSONPath(%q) = %v, want %v", tt.path, result, tt.want)
			}
		})
	}
}

func TestResponseJSONPathErrors(t *testing.T) {
	body := `{"user":{"name":"John"}}`
	r := makeResponse(200, "application/json", []byte(body), nil)

	tests := []struct {
		name string
		path string
	}{
		{"key not found", "missing.key"},
		{"invalid array index", "user.name[0]"},
		{"navigate into string", "user.name.invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.JSONPath(tt.path)
			if err == nil {
				t.Errorf("JSONPath(%q) should return error", tt.path)
			}
		})
	}
}

func TestResponseJSONPathArrayIndexOutOfBounds(t *testing.T) {
	body := `{"items":[1,2,3]}`
	r := makeResponse(200, "application/json", []byte(body), nil)

	_, err := r.JSONPath("items[99]")
	if err == nil {
		t.Error("JSONPath should return error for out of bounds index")
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"", nil},
		{"key", []string{"key"}},
		{"a.b.c", []string{"a", "b", "c"}},
		{"a[0]", []string{"a", "0"}},
		{"a[0].b", []string{"a", "0", "b"}},
		{"a.b[0].c[1]", []string{"a", "b", "0", "c", "1"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := splitPath(tt.path)
			if len(got) != len(tt.want) {
				t.Errorf("splitPath(%q) = %v, want %v", tt.path, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitPath(%q)[%d] = %v, want %v", tt.path, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestResponsePretty(t *testing.T) {
	t.Run("JSON pretty print", func(t *testing.T) {
		body := `{"key":"value"}`
		r := makeResponse(200, "application/json", []byte(body), nil)

		pretty := r.Pretty()
		if pretty == body {
			t.Error("Pretty() should format JSON")
		}
		// Should contain newlines for formatting
		if len(pretty) <= len(body) {
			t.Error("Pretty JSON should be longer due to formatting")
		}
	})

	t.Run("XML returns as-is", func(t *testing.T) {
		body := `<root><key>value</key></root>`
		r := makeResponse(200, "application/xml", []byte(body), nil)

		pretty := r.Pretty()
		if pretty != body {
			t.Errorf("Pretty() = %v, want %v (unchanged for XML)", pretty, body)
		}
	})

	t.Run("text returns as-is", func(t *testing.T) {
		body := `plain text`
		r := makeResponse(200, "text/plain", []byte(body), nil)

		pretty := r.Pretty()
		if pretty != body {
			t.Errorf("Pretty() = %v, want %v", pretty, body)
		}
	})

	t.Run("invalid JSON returns as-is", func(t *testing.T) {
		body := `not json`
		r := makeResponse(200, "application/json", []byte(body), nil)

		pretty := r.Pretty()
		if pretty != body {
			t.Errorf("Pretty() = %v, want %v (unchanged for invalid JSON)", pretty, body)
		}
	})
}

func TestResponseSize(t *testing.T) {
	body := []byte("test body content")
	r := makeResponse(200, "text/plain", body, nil)

	if r.Size() != len(body) {
		t.Errorf("Size() = %d, want %d", r.Size(), len(body))
	}
}

func TestResponseGetPagination(t *testing.T) {
	t.Run("from headers", func(t *testing.T) {
		r := makeResponse(200, "application/json", []byte("[]"), map[string]string{
			"X-Page":        "2",
			"X-Per-Page":    "10",
			"X-Total":       "100",
			"X-Total-Pages": "10",
		})

		info := r.GetPagination()
		if info.Page != 2 {
			t.Errorf("Page = %d, want 2", info.Page)
		}
		if info.PerPage != 10 {
			t.Errorf("PerPage = %d, want 10", info.PerPage)
		}
		if info.TotalItems != 100 {
			t.Errorf("TotalItems = %d, want 100", info.TotalItems)
		}
		if info.TotalPages != 10 {
			t.Errorf("TotalPages = %d, want 10", info.TotalPages)
		}
	})

	t.Run("from Link header", func(t *testing.T) {
		r := makeResponse(200, "application/json", []byte("[]"), map[string]string{
			"Link": `<https://api.example.com/items?page=3>; rel="next", <https://api.example.com/items?page=1>; rel="prev"`,
		})

		info := r.GetPagination()
		if info.NextURL != "https://api.example.com/items?page=3" {
			t.Errorf("NextURL = %v, want 'https://api.example.com/items?page=3'", info.NextURL)
		}
		if info.PrevURL != "https://api.example.com/items?page=1" {
			t.Errorf("PrevURL = %v, want 'https://api.example.com/items?page=1'", info.PrevURL)
		}
	})

	t.Run("from JSON body", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"page":        2,
			"per_page":    25,
			"total":       100,
			"total_pages": 4,
			"has_more":    true,
			"next":        "https://api.example.com/items?page=3",
			"previous":    "https://api.example.com/items?page=1",
		})
		r := makeResponse(200, "application/json", body, nil)

		info := r.GetPagination()
		if info.Page != 2 {
			t.Errorf("Page = %d, want 2", info.Page)
		}
		if info.PerPage != 25 {
			t.Errorf("PerPage = %d, want 25", info.PerPage)
		}
		if info.HasMore != true {
			t.Error("HasMore = false, want true")
		}
		if info.NextURL != "https://api.example.com/items?page=3" {
			t.Errorf("NextURL = %v", info.NextURL)
		}
	})

	t.Run("HasMore inferred from NextURL", func(t *testing.T) {
		r := makeResponse(200, "application/json", []byte("[]"), map[string]string{
			"Link": `<https://api.example.com/items?page=2>; rel="next"`,
		})

		info := r.GetPagination()
		if !info.HasMore {
			t.Error("HasMore should be inferred from NextURL")
		}
	})

	t.Run("HasMore inferred from page/total", func(t *testing.T) {
		r := makeResponse(200, "application/json", []byte("[]"), map[string]string{
			"X-Page":        "2",
			"X-Total-Pages": "5",
		})

		info := r.GetPagination()
		if !info.HasMore {
			t.Error("HasMore should be inferred from Page < TotalPages")
		}
	})
}

func TestParseLinkHeader(t *testing.T) {
	tests := []struct {
		header string
		rel    string
		want   string
	}{
		{
			`<https://api.example.com/items?page=3>; rel="next"`,
			"next",
			"https://api.example.com/items?page=3",
		},
		{
			`<https://api.example.com/items?page=1>; rel="prev", <https://api.example.com/items?page=3>; rel="next"`,
			"prev",
			"https://api.example.com/items?page=1",
		},
		{
			`<https://api.example.com/items?page=1>; rel="first"`,
			"next",
			"",
		},
		{
			"",
			"next",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			got := parseLinkHeader(tt.header, tt.rel)
			if got != tt.want {
				t.Errorf("parseLinkHeader(%q, %q) = %q, want %q", tt.header, tt.rel, got, tt.want)
			}
		})
	}
}

func TestResponseGetRateLimit(t *testing.T) {
	t.Run("X-RateLimit headers", func(t *testing.T) {
		r := makeResponse(200, "application/json", []byte("{}"), map[string]string{
			"X-RateLimit-Limit":     "1000",
			"X-RateLimit-Remaining": "999",
			"X-RateLimit-Reset":     "1609459200",
		})

		info := r.GetRateLimit()
		if info.Limit != 1000 {
			t.Errorf("Limit = %d, want 1000", info.Limit)
		}
		if info.Remaining != 999 {
			t.Errorf("Remaining = %d, want 999", info.Remaining)
		}
		if info.Reset != 1609459200 {
			t.Errorf("Reset = %d, want 1609459200", info.Reset)
		}
	})

	t.Run("RateLimit headers (alternative)", func(t *testing.T) {
		r := makeResponse(200, "application/json", []byte("{}"), map[string]string{
			"RateLimit-Limit":     "500",
			"RateLimit-Remaining": "450",
			"RateLimit-Reset":     "1609459200",
		})

		info := r.GetRateLimit()
		if info.Limit != 500 {
			t.Errorf("Limit = %d, want 500", info.Limit)
		}
		if info.Remaining != 450 {
			t.Errorf("Remaining = %d, want 450", info.Remaining)
		}
	})

	t.Run("Retry-After header", func(t *testing.T) {
		r := makeResponse(429, "application/json", []byte("{}"), map[string]string{
			"Retry-After": "60",
		})

		info := r.GetRateLimit()
		if info.RetryAfter != 60 {
			t.Errorf("RetryAfter = %d, want 60", info.RetryAfter)
		}
	})

	t.Run("no rate limit headers", func(t *testing.T) {
		r := makeResponse(200, "application/json", []byte("{}"), nil)

		info := r.GetRateLimit()
		if info.Limit != 0 || info.Remaining != 0 {
			t.Errorf("expected zero values, got Limit=%d, Remaining=%d", info.Limit, info.Remaining)
		}
	})
}
