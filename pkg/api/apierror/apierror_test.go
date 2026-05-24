// SPDX-License-Identifier: Apache-2.0

package apierror

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNew(t *testing.T) {
	r := New(codes.NotFound, "thing not found")
	if r.Error != "NotFound" {
		t.Errorf("Error = %q, want NotFound", r.Error)
	}
	if r.Message != "thing not found" {
		t.Errorf("Message = %q", r.Message)
	}
	if r.Details != nil {
		t.Errorf("Details = %v, want nil", r.Details)
	}
}

func TestWithDetails(t *testing.T) {
	r := New(codes.InvalidArgument, "bad").
		WithDetails("field", "name").
		WithDetails("max", 64)
	if r.Details["field"] != "name" {
		t.Errorf("Details[field] = %v", r.Details["field"])
	}
	if r.Details["max"] != 64 {
		t.Errorf("Details[max] = %v", r.Details["max"])
	}
}

func TestStatusCode(t *testing.T) {
	r := New(codes.NotFound, "x")
	if got := r.StatusCode(); got != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", got, http.StatusNotFound)
	}
}

func TestStatusCode_UnknownErrorString(t *testing.T) {
	// Simulates a JSON-deserialized Response with a corrupted Error field.
	r := &Response{Error: "Garbage", Message: "x"}
	if got := r.StatusCode(); got != http.StatusInternalServerError {
		t.Errorf("StatusCode for unknown error = %d, want 500", got)
	}
}

func TestAsGRPC(t *testing.T) {
	r := New(codes.PermissionDenied, "no access")
	err := r.AsGRPC()
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("AsGRPC did not produce a status error")
	}
	if st.Code() != codes.PermissionDenied {
		t.Errorf("Code = %v, want PermissionDenied", st.Code())
	}
	if st.Message() != "no access" {
		t.Errorf("Message = %q", st.Message())
	}
}

func TestFromGRPC_Nil(t *testing.T) {
	if r := FromGRPC(nil); r != nil {
		t.Errorf("FromGRPC(nil) = %v, want nil", r)
	}
}

func TestFromGRPC_StatusError(t *testing.T) {
	err := status.Error(codes.Unavailable, "down")
	r := FromGRPC(err)
	if r == nil {
		t.Fatal("FromGRPC returned nil")
	}
	if r.Error != "Unavailable" {
		t.Errorf("Error = %q, want Unavailable", r.Error)
	}
	if r.Message != "down" {
		t.Errorf("Message = %q", r.Message)
	}
}

func TestFromGRPC_NonStatusError(t *testing.T) {
	r := FromGRPC(errors.New("plain error"))
	if r == nil {
		t.Fatal("FromGRPC returned nil")
	}
	if r.Error != codes.Unknown.String() {
		t.Errorf("Error = %q, want Unknown", r.Error)
	}
	if r.Message != "plain error" {
		t.Errorf("Message = %q", r.Message)
	}
}

func TestRoundTrip(t *testing.T) {
	original := New(codes.AlreadyExists, "duplicate")
	gerr := original.AsGRPC()
	got := FromGRPC(gerr)
	if got.Error != original.Error {
		t.Errorf("Error: %q != %q", got.Error, original.Error)
	}
	if got.Message != original.Message {
		t.Errorf("Message: %q != %q", got.Message, original.Message)
	}
	// Note: Details don't survive AsGRPC (gRPC status doesn't carry our map).
	// Rich details can be added later via google.rpc.errdetails.
}

func TestHTTPStatusFromCode(t *testing.T) {
	tests := []struct {
		code codes.Code
		want int
	}{
		{codes.OK, http.StatusOK},
		{codes.Canceled, 499},
		{codes.Unknown, http.StatusInternalServerError},
		{codes.InvalidArgument, http.StatusBadRequest},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout},
		{codes.NotFound, http.StatusNotFound},
		{codes.AlreadyExists, http.StatusConflict},
		{codes.PermissionDenied, http.StatusForbidden},
		{codes.ResourceExhausted, http.StatusTooManyRequests},
		{codes.FailedPrecondition, http.StatusBadRequest},
		{codes.Aborted, http.StatusConflict},
		{codes.OutOfRange, http.StatusBadRequest},
		{codes.Unimplemented, http.StatusNotImplemented},
		{codes.Internal, http.StatusInternalServerError},
		{codes.Unavailable, http.StatusServiceUnavailable},
		{codes.DataLoss, http.StatusInternalServerError},
		{codes.Unauthenticated, http.StatusUnauthorized},
		{codes.Code(99), http.StatusInternalServerError}, // unknown code → default 500
	}
	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			if got := HTTPStatusFromCode(tt.code); got != tt.want {
				t.Errorf("HTTPStatusFromCode(%v) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}

func TestCodeFromHTTPStatus(t *testing.T) {
	tests := []struct {
		http int
		want codes.Code
	}{
		{http.StatusOK, codes.OK},
		{http.StatusBadRequest, codes.InvalidArgument},
		{http.StatusUnauthorized, codes.Unauthenticated},
		{http.StatusForbidden, codes.PermissionDenied},
		{http.StatusNotFound, codes.NotFound},
		{http.StatusConflict, codes.Aborted},
		{http.StatusTooManyRequests, codes.ResourceExhausted},
		{499, codes.Canceled},
		{http.StatusInternalServerError, codes.Internal},
		{http.StatusNotImplemented, codes.Unimplemented},
		{http.StatusServiceUnavailable, codes.Unavailable},
		{http.StatusGatewayTimeout, codes.DeadlineExceeded},
		{418, codes.Unknown}, // not in canonical mapping
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("http_%d", tt.http), func(t *testing.T) {
			if got := CodeFromHTTPStatus(tt.http); got != tt.want {
				t.Errorf("CodeFromHTTPStatus(%d) = %v, want %v", tt.http, got, tt.want)
			}
		})
	}
}

func TestJSONMarshal(t *testing.T) {
	r := New(codes.NotFound, "missing")
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"error":"NotFound","message":"missing"}`
	if string(b) != want {
		t.Errorf("Marshal = %s, want %s", b, want)
	}
}

func TestJSONMarshal_WithDetails(t *testing.T) {
	r := New(codes.InvalidArgument, "bad").WithDetails("field", "x")
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"error":"InvalidArgument","message":"bad","details":{"field":"x"}}`
	if string(b) != want {
		t.Errorf("Marshal = %s, want %s", b, want)
	}
}

func TestJSONUnmarshal(t *testing.T) {
	const input = `{"error":"NotFound","message":"missing"}`
	var r Response
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatal(err)
	}
	if r.Error != "NotFound" || r.Message != "missing" {
		t.Errorf("Unmarshal: %+v", r)
	}
	// StatusCode must still work after unmarshal (it parses Error string back to codes.Code).
	if got := r.StatusCode(); got != http.StatusNotFound {
		t.Errorf("StatusCode after unmarshal = %d, want %d", got, http.StatusNotFound)
	}
}
