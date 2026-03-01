package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"google.golang.org/grpc/connectivity"
)

// ConnState represents a connection state.
type ConnState int

const (
	ConnConnected ConnState = iota
	ConnDisconnected
	ConnReconnecting
)

// ConnectionHealth tracks health of all connections.
type ConnectionHealth struct {
	GRPC ConnState
	NATS ConnState
}

// NewConnectionHealth creates connection health with default disconnected state.
func NewConnectionHealth() *ConnectionHealth {
	return &ConnectionHealth{
		GRPC: ConnDisconnected,
		NATS: ConnDisconnected,
	}
}

// UpdateFromGRPC updates the gRPC connection state from the grpc.ClientConn state.
func (h *ConnectionHealth) UpdateFromGRPC(state connectivity.State) {
	switch state {
	case connectivity.Ready:
		h.GRPC = ConnConnected
	case connectivity.Connecting, connectivity.TransientFailure:
		h.GRPC = ConnReconnecting
	default:
		h.GRPC = ConnDisconnected
	}
}

// View renders the connection health indicators.
func (h *ConnectionHealth) View() string {
	return fmt.Sprintf("gRPC:%s NATS:%s",
		renderDot(h.GRPC),
		renderDot(h.NATS),
	)
}

func renderDot(state ConnState) string {
	switch state {
	case ConnConnected:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("●")
	case ConnReconnecting:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("●")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("●")
	}
}
