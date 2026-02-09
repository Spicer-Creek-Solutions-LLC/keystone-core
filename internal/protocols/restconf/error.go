package restconf

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
)

// Error represents a single RESTCONF error (RFC 8040 Section 7.1).
type Error struct {
	ErrorType    string `json:"error-type" xml:"error-type"`
	ErrorTag     string `json:"error-tag" xml:"error-tag"`
	ErrorAppTag  string `json:"error-app-tag,omitempty" xml:"error-app-tag,omitempty"`
	ErrorPath    string `json:"error-path,omitempty" xml:"error-path,omitempty"`
	ErrorMessage string `json:"error-message,omitempty" xml:"error-message,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "restconf %s: %s", e.ErrorType, e.ErrorTag)
	if e.ErrorMessage != "" {
		fmt.Fprintf(&b, ": %s", e.ErrorMessage)
	}
	if e.ErrorPath != "" {
		fmt.Fprintf(&b, " (path: %s)", e.ErrorPath)
	}
	return b.String()
}

// ResponseError wraps the ietf-restconf:errors response envelope.
type ResponseError struct {
	Errors []Error
}

// Error implements the error interface.
func (e *ResponseError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d restconf errors:", len(e.Errors))
	for _, err := range e.Errors {
		fmt.Fprintf(&b, "\n  - %s", err.Error())
	}
	return b.String()
}

// HasError returns true if any error has severity "error" (not just warning).
func (e *ResponseError) HasError() bool {
	return len(e.Errors) > 0
}

// JSON envelope for parsing.
type jsonErrorsEnvelope struct {
	IETFErrors struct {
		Error []Error `json:"error"`
	} `json:"ietf-restconf:errors"`
}

// XML envelope for parsing.
type xmlErrorsEnvelope struct {
	XMLName xml.Name   `xml:"errors"`
	Error   []xmlError `xml:"error"`
}

type xmlError struct {
	ErrorType    string `xml:"error-type"`
	ErrorTag     string `xml:"error-tag"`
	ErrorAppTag  string `xml:"error-app-tag"`
	ErrorPath    string `xml:"error-path"`
	ErrorMessage string `xml:"error-message"`
}

// ParseErrorResponse parses a RESTCONF error response from JSON or XML.
func ParseErrorResponse(body []byte, contentType string) (*ResponseError, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty error response body")
	}

	if strings.Contains(contentType, "xml") {
		return parseXMLErrors(body)
	}
	return parseJSONErrors(body)
}

func parseJSONErrors(body []byte) (*ResponseError, error) {
	var env jsonErrorsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse json error response: %w", err)
	}
	if len(env.IETFErrors.Error) == 0 {
		return nil, fmt.Errorf("no errors found in response")
	}
	return &ResponseError{Errors: env.IETFErrors.Error}, nil
}

func parseXMLErrors(body []byte) (*ResponseError, error) {
	var env xmlErrorsEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse xml error response: %w", err)
	}
	if len(env.Error) == 0 {
		return nil, fmt.Errorf("no errors found in response")
	}
	errs := make([]Error, len(env.Error))
	for i, xe := range env.Error {
		errs[i] = Error(xe)
	}
	return &ResponseError{Errors: errs}, nil
}

// IsErrorResponse returns true if the HTTP status indicates a RESTCONF error.
func IsErrorResponse(statusCode int) bool {
	return statusCode >= 400
}
