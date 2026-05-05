// Package apierror defines the project-wide error model.
//
// REST endpoints return Response as a JSON body. gRPC services use
// status.Error(codes.X, msg) for the same condition. FromGRPC/AsGRPC
// transcribe between the two; StatusCode returns the canonical HTTP
// code for any Response.
package apierror

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Response is the standard JSON error body for REST endpoints.
type Response struct {
	Error   string         `json:"error"`             // canonical name (gRPC code String())
	Message string         `json:"message"`           // human-readable
	Details map[string]any `json:"details,omitempty"` // optional key/value details
}

// New constructs a Response from a gRPC code and message.
func New(code codes.Code, msg string) *Response {
	return &Response{
		Error:   code.String(),
		Message: msg,
	}
}

// WithDetails attaches a key/value to Details.
func (r *Response) WithDetails(key string, value any) *Response {
	if r.Details == nil {
		r.Details = make(map[string]any)
	}
	r.Details[key] = value
	return r
}

// StatusCode returns the canonical HTTP status code.
func (r *Response) StatusCode() int {
	return HTTPStatusFromCode(codeFromString(r.Error))
}

// AsGRPC returns the gRPC-error equivalent.
func (r *Response) AsGRPC() error {
	return status.Error(codeFromString(r.Error), r.Message)
}

// FromGRPC converts a gRPC error into a Response. Returns nil if err is nil.
func FromGRPC(err error) *Response {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return New(codes.Unknown, err.Error())
	}
	return New(st.Code(), st.Message())
}

// HTTPStatusFromCode maps a gRPC code to its HTTP status equivalent.
func HTTPStatusFromCode(c codes.Code) int {
	switch c {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return 499 // nginx convention: client closed request
	case codes.Unknown:
		return http.StatusInternalServerError
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusBadRequest
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DataLoss:
		return http.StatusInternalServerError
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

// CodeFromHTTPStatus maps an HTTP status to a gRPC code.
func CodeFromHTTPStatus(httpCode int) codes.Code {
	switch httpCode {
	case http.StatusOK:
		return codes.OK
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusConflict:
		return codes.Aborted
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	case 499:
		return codes.Canceled
	case http.StatusInternalServerError:
		return codes.Internal
	case http.StatusNotImplemented:
		return codes.Unimplemented
	case http.StatusServiceUnavailable:
		return codes.Unavailable
	case http.StatusGatewayTimeout:
		return codes.DeadlineExceeded
	default:
		return codes.Unknown
	}
}

// codeByName reverses codes.Code.String() for lookup from JSON-deserialized Responses.
var codeByName = func() map[string]codes.Code {
	m := make(map[string]codes.Code, 17)
	for c := codes.Code(0); c <= codes.Unauthenticated; c++ {
		m[c.String()] = c
	}
	return m
}()

func codeFromString(s string) codes.Code {
	if c, ok := codeByName[s]; ok {
		return c
	}
	return codes.Unknown
}
