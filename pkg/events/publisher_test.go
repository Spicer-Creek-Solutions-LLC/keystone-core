package events

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestNATSServer starts an embedded NATS server for testing
func startTestNATSServer(t *testing.T) (*server.Server, *nats.Conn, nats.JetStreamContext) {
	t.Helper()

	// Create temp directory for JetStream storage
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // Random port
		JetStream: true,
		StoreDir:  t.TempDir(),
	}

	ns, err := server.NewServer(opts)
	require.NoError(t, err)

	go ns.Start()

	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server failed to start")
	}

	// Connect to the server
	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)

	// Get JetStream context
	js, err := nc.JetStream()
	require.NoError(t, err)

	// Cleanup
	t.Cleanup(func() {
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	return ns, nc, js
}

func TestNewJetStreamPublisher(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	require.NotNil(t, publisher)
	defer publisher.Close()

	// Verify stream was created
	stream, err := js.StreamInfo(StreamName)
	require.NoError(t, err)
	assert.Equal(t, StreamName, stream.Config.Name)
}

func TestNewJetStreamPublisher_NilJetStream(t *testing.T) {
	publisher, err := NewJetStreamPublisher(nil)
	assert.Error(t, err)
	assert.Nil(t, publisher)
	assert.Contains(t, err.Error(), "JetStream context is required")
}

func TestPublisher_Publish(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	event := NewEvent(EventTypeAgentConnect).
		Source("/agents/test-agent").
		Severity(SeverityInfo).
		Tag("env", "test").
		Data("agent_id", "test-123").
		Build()

	err = publisher.Publish(event)
	assert.NoError(t, err)

	// Verify event was published to stream
	stream, err := js.StreamInfo(StreamName)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), stream.State.Msgs)
}

func TestPublisher_PublishAsync(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	event := NewEvent(EventTypeJobStart).
		Source("/control-plane").
		Severity(SeverityInfo).
		Data("job_id", "job-456").
		Build()

	err = publisher.PublishAsync(event)
	assert.NoError(t, err)

	// Wait for async publish to complete
	select {
	case <-js.PublishAsyncComplete():
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for async publish")
	}

	// Verify event was published
	stream, err := js.StreamInfo(StreamName)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), stream.State.Msgs)
}

func TestPublisher_PublishNilEvent(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	err = publisher.Publish(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event is nil")
}

func TestPublisher_PublishAfterClose(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)

	// Close the publisher
	err = publisher.Close()
	assert.NoError(t, err)

	// Try to publish after close
	event := NewEvent(EventTypeSystemStartup).
		Source("/control-plane").
		Build()

	err = publisher.Publish(event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publisher is closed")
}

func TestPublisher_MultipleEvents(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	// Publish multiple events
	eventTypes := []EventType{
		EventTypeAgentConnect,
		EventTypeAgentHeartbeat,
		EventTypeJobStart,
		EventTypeStateApplyStart,
		EventTypeSystemStartup,
	}

	for _, eventType := range eventTypes {
		event := NewEvent(eventType).
			Source("/test").
			Build()

		err := publisher.Publish(event)
		assert.NoError(t, err)
	}

	// Verify all events were published
	stream, err := js.StreamInfo(StreamName)
	require.NoError(t, err)
	assert.Equal(t, uint64(len(eventTypes)), stream.State.Msgs)
}

func TestPublisher_SubjectGeneration(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)
	defer publisher.Close()

	tests := []struct {
		name          string
		eventType     EventType
		expectedSubject string
	}{
		{
			name:          "agent event",
			eventType:     EventTypeAgentConnect,
			expectedSubject: "agent.connect",
		},
		{
			name:          "state event",
			eventType:     EventTypeStateChange,
			expectedSubject: "state.change",
		},
		{
			name:          "system event",
			eventType:     EventTypeSystemShutdown,
			expectedSubject: "system.shutdown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := NewEvent(tt.eventType).
				Source("/test").
				Build()

			assert.Equal(t, tt.expectedSubject, event.Subject)
		})
	}
}

func TestPublisher_Close(t *testing.T) {
	_, _, js := startTestNATSServer(t)

	publisher, err := NewJetStreamPublisher(js)
	require.NoError(t, err)

	// Close should succeed
	err = publisher.Close()
	assert.NoError(t, err)

	// Second close should also succeed (idempotent)
	err = publisher.Close()
	assert.NoError(t, err)
}
