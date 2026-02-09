package restconf

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscribe_Events(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/restconf/streams/NETCONF", r.URL.Path)
		assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		fmt.Fprintf(w, "id: 1\nevent: push-update\ndata: {\"ietf-interfaces:interfaces\":{}}\n\n")
		flusher.Flush()

		fmt.Fprintf(w, "id: 2\nevent: push-update\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub, err := a.Subscribe(ctx, "NETCONF")
	require.NoError(t, err)
	defer sub.Close()

	// Read first event
	select {
	case ev := <-sub.Events():
		assert.Equal(t, "1", ev.ID)
		assert.Equal(t, "push-update", ev.Event)
		assert.Contains(t, string(ev.Data), "ietf-interfaces")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for first event")
	}

	// Read second event
	select {
	case ev := <-sub.Events():
		assert.Equal(t, "2", ev.ID)
		assert.Contains(t, string(ev.Data), "ok")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for second event")
	}
}

func TestSubscribe_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	_, err := a.Subscribe(context.Background(), "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestSubscribe_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	_, err := a.Subscribe(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stream error")
}

func TestSubscribe_Close(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		// Send one event then keep connection open
		fmt.Fprintf(w, "id: 1\nevent: test\ndata: hello\n\n")
		flusher.Flush()

		// Block until client disconnects
		<-r.Context().Done()
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	ctx := context.Background()

	sub, err := a.Subscribe(ctx, "test-stream")
	require.NoError(t, err)

	// Read the event
	select {
	case ev := <-sub.Events():
		assert.Equal(t, "1", ev.ID)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Close the subscription
	sub.Close()

	// Events channel should drain and close
	select {
	case _, ok := <-sub.Events():
		if ok {
			// May get buffered events, that's fine
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for events channel to close")
	}
}

func TestSubscribe_MultilineData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		// Multiline data field
		fmt.Fprintf(w, "id: 1\ndata: line1\ndata: line2\ndata: line3\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub, err := a.Subscribe(ctx, "test")
	require.NoError(t, err)
	defer sub.Close()

	select {
	case ev := <-sub.Events():
		assert.Equal(t, "line1\nline2\nline3", string(ev.Data))
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for multiline event")
	}
}

func TestSubscribe_CommentsIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		// Comment lines should be ignored
		fmt.Fprintf(w, ": this is a comment\nid: 1\ndata: actual-data\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub, err := a.Subscribe(ctx, "test")
	require.NoError(t, err)
	defer sub.Close()

	select {
	case ev := <-sub.Events():
		assert.Equal(t, "1", ev.ID)
		assert.Equal(t, "actual-data", string(ev.Data))
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestStreamSubscription_Channels(t *testing.T) {
	events := make(chan StreamEvent, 1)
	errs := make(chan error, 1)
	_, cancel := context.WithCancel(context.Background())

	sub := &StreamSubscription{
		stream: "test",
		events: events,
		errs:   errs,
		cancel: cancel,
	}

	assert.NotNil(t, sub.Events())
	assert.NotNil(t, sub.Errors())
}
