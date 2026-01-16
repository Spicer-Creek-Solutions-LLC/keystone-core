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

func (m *NATSStatusProvider) IsConnected() bool {
	m.mu.Lock()
	m.StatusCallHits++
	m.mu.Unlock()
	return m.Connected
}

func (m *NATSStatusProvider) ConnectedURLs() []string {
	m.mu.Lock()
	m.StatusCallHits++
	m.mu.Unlock()
	return append([]string(nil), m.URLs...)
}

func (m *NATSStatusProvider) JetStreamAvailable() bool {
	m.mu.Lock()
	m.StatusCallHits++
	m.mu.Unlock()
	return m.JetStream
}

func (m *NATSStatusProvider) LastPublishTime() time.Time {
	m.mu.Lock()
	m.StatusCallHits++
	m.mu.Unlock()
	return m.LastPublish
}

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

func (m *NATSController) RestartEmbedded(ctx context.Context) error {
	_ = ctx
	m.mu.Lock()
	m.RestartCalls++
	m.mu.Unlock()
	return m.RestartErr
}

func (m *NATSController) Reconnect(ctx context.Context) error {
	_ = ctx
	m.mu.Lock()
	m.ReconnectCalls++
	m.mu.Unlock()
	return m.ReconnectErr
}

func (m *NATSController) Drain(ctx context.Context) error {
	_ = ctx
	m.mu.Lock()
	m.DrainCalls++
	m.mu.Unlock()
	return m.DrainErr
}

func (m *NATSController) Failover(ctx context.Context, targetURLs []string) error {
	_ = ctx
	m.mu.Lock()
	m.FailoverCalls++
	m.LastFailover = append([]string(nil), targetURLs...)
	m.mu.Unlock()
	return m.FailoverErr
}

func (m *NATSController) IsEmbedded() bool {
	return m.Embedded
}
