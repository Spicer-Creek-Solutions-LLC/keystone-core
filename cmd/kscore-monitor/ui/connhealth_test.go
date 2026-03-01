package ui

import (
	"strings"
	"testing"

	"google.golang.org/grpc/connectivity"
)

func TestConnectionHealthDefault(t *testing.T) {
	h := NewConnectionHealth()
	if h.GRPC != ConnDisconnected {
		t.Errorf("expected gRPC disconnected, got %d", h.GRPC)
	}
	if h.NATS != ConnDisconnected {
		t.Errorf("expected NATS disconnected, got %d", h.NATS)
	}
}

func TestConnectionHealthUpdateFromGRPC(t *testing.T) {
	h := NewConnectionHealth()

	h.UpdateFromGRPC(connectivity.Ready)
	if h.GRPC != ConnConnected {
		t.Error("expected connected after Ready")
	}

	h.UpdateFromGRPC(connectivity.Connecting)
	if h.GRPC != ConnReconnecting {
		t.Error("expected reconnecting after Connecting")
	}

	h.UpdateFromGRPC(connectivity.TransientFailure)
	if h.GRPC != ConnReconnecting {
		t.Error("expected reconnecting after TransientFailure")
	}

	h.UpdateFromGRPC(connectivity.Shutdown)
	if h.GRPC != ConnDisconnected {
		t.Error("expected disconnected after Shutdown")
	}
}

func TestConnectionHealthView(t *testing.T) {
	h := NewConnectionHealth()
	h.GRPC = ConnConnected
	h.NATS = ConnDisconnected

	view := h.View()
	if !strings.Contains(view, "gRPC:") {
		t.Errorf("expected gRPC label, got %q", view)
	}
	if !strings.Contains(view, "NATS:") {
		t.Errorf("expected NATS label, got %q", view)
	}
}
