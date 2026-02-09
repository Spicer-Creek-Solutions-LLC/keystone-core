// Package mocks provides configurable mock implementations for testing
// including NATS, file storage, agent store, and policy components.
package mocks

import (
	"context"
	"sync"
	"time"
)

// NATSStatusProvider is a configurable mock for NATS status checks.
type NATSStatusProvider struct {
	Connected      bool
	URLs           []string
	JetStream      bool
	LastPublish    time.Time
	LastSubscribe  time.Time
	StatusCallHits int
	mu             sync.Mutex
}

// IsConnected returns whether the NATS connection is established.
func (m *NATSStatusProvider) IsConnected() bool {
	m.mu.Lock()
	m.StatusCallHits++
	m.mu.Unlock()
	return m.Connected
}

// ConnectedURLs returns the connected NATS server URLs.
func (m *NATSStatusProvider) ConnectedURLs() []string {
	m.mu.Lock()
	m.StatusCallHits++
	m.mu.Unlock()
	return append([]string(nil), m.URLs...)
}

// JetStreamAvailable returns whether JetStream is available.
func (m *NATSStatusProvider) JetStreamAvailable() bool {
	m.mu.Lock()
	m.StatusCallHits++
	m.mu.Unlock()
	return m.JetStream
}

// LastPublishTime returns the time of the last publish.
func (m *NATSStatusProvider) LastPublishTime() time.Time {
	m.mu.Lock()
	m.StatusCallHits++
	m.mu.Unlock()
	return m.LastPublish
}

// LastSubscribeTime returns the time of the last subscribe.
func (m *NATSStatusProvider) LastSubscribeTime() time.Time {
	m.mu.Lock()
	m.StatusCallHits++
	m.mu.Unlock()
	return m.LastSubscribe
}

// NATSController is a configurable mock for NATS control operations.
type NATSController struct {
	Embedded bool

	RestartErr   error
	ReconnectErr error
	DrainErr     error
	FailoverErr  error

	RestartCalls   int
	ReconnectCalls int
	DrainCalls     int
	FailoverCalls  int
	LastFailover   []string
	mu             sync.Mutex
}

// RestartEmbedded restarts the embedded NATS server.
func (m *NATSController) RestartEmbedded(ctx context.Context) error {
	_ = ctx
	m.mu.Lock()
	m.RestartCalls++
	m.mu.Unlock()
	return m.RestartErr
}

// Reconnect reconnects to NATS.
func (m *NATSController) Reconnect(ctx context.Context) error {
	_ = ctx
	m.mu.Lock()
	m.ReconnectCalls++
	m.mu.Unlock()
	return m.ReconnectErr
}

// Drain drains the NATS connection.
func (m *NATSController) Drain(ctx context.Context) error {
	_ = ctx
	m.mu.Lock()
	m.DrainCalls++
	m.mu.Unlock()
	return m.DrainErr
}

// Failover triggers a failover.
func (m *NATSController) Failover(ctx context.Context, targetURLs []string) error {
	_ = ctx
	m.mu.Lock()
	m.FailoverCalls++
	m.LastFailover = append([]string(nil), targetURLs...)
	m.mu.Unlock()
	return m.FailoverErr
}

// IsEmbedded returns whether NATS is running in embedded mode.
func (m *NATSController) IsEmbedded() bool {
	return m.Embedded
}
