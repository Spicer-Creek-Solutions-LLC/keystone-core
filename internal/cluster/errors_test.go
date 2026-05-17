package cluster

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsLeaseNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"rpctypes lease not found", rpctypes.ErrLeaseNotFound, true},
		{"grpc NotFound status", status.Error(codes.NotFound, "requested lease not found"), true},
		{"grpc Unavailable", status.Error(codes.Unavailable, "down"), false},
		{"plain error", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLeaseNotFound(tt.err); got != tt.want {
				t.Fatalf("isLeaseNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestTranslateError(t *testing.T) {
	if translateError(nil) != nil {
		t.Fatal("translateError(nil) must be nil")
	}

	// Context errors pass through unwrapped so caller select
	// loops keep matching them.
	if got := translateError(context.Canceled); !errors.Is(got, context.Canceled) {
		t.Fatalf("context.Canceled not passed through: %v", got)
	}
	wrapped := fmt.Errorf("op: %w", context.DeadlineExceeded)
	if got := translateError(wrapped); !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("DeadlineExceeded not passed through: %v", got)
	}

	// Lease-not-found is classified onto the sentinel.
	if got := translateError(status.Error(codes.NotFound, "lease")); !errors.Is(got, ErrLeaseNotFound) {
		t.Fatalf("lease-not-found not mapped: %v", got)
	}

	// Unclassified errors are returned (still inspectable).
	sentinel := errors.New("some etcd error")
	if got := translateError(sentinel); !errors.Is(got, sentinel) {
		t.Fatalf("unclassified error not preserved: %v", got)
	}
}
