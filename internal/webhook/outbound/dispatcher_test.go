package outbound

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatcher_Deliver_Success(t *testing.T) {
	var receivedBody []byte
	var receivedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(5 * time.Second)
	sub := &Subscription{
		URL:    srv.URL,
		Secret: "my-secret",
		Headers: map[string]string{
			"X-Custom": "custom-value",
		},
	}
	payload := []byte(`{"event":"test"}`)

	code, err := d.Deliver(context.Background(), sub, payload, "del-123")
	require.NoError(t, err)
	assert.Equal(t, 200, code)
	assert.Equal(t, payload, receivedBody)
	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
	assert.Equal(t, "del-123", receivedHeaders.Get(WebhookIDHeader))
	assert.NotEmpty(t, receivedHeaders.Get(WebhookTimestampHeader))
	assert.NotEmpty(t, receivedHeaders.Get(SignatureHeader))
	assert.Equal(t, "custom-value", receivedHeaders.Get("X-Custom"))

	// Verify signature
	sig := receivedHeaders.Get(SignatureHeader)
	assert.True(t, Verify([]byte("my-secret"), payload, sig))
}

func TestDispatcher_Deliver_NoSecret(t *testing.T) {
	var receivedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(5 * time.Second)
	sub := &Subscription{URL: srv.URL}

	code, err := d.Deliver(context.Background(), sub, []byte("{}"), "del-456")
	require.NoError(t, err)
	assert.Equal(t, 200, code)
	assert.Empty(t, receivedHeaders.Get(SignatureHeader))
}

func TestDispatcher_Deliver_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := NewDispatcher(5 * time.Second)
	sub := &Subscription{URL: srv.URL}

	code, err := d.Deliver(context.Background(), sub, []byte("{}"), "del-789")
	assert.Error(t, err)
	assert.Equal(t, 500, code)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestDispatcher_Deliver_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(10 * time.Second)
	sub := &Subscription{URL: srv.URL}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := d.Deliver(ctx, sub, []byte("{}"), "del-cancel")
	assert.Error(t, err)
}

func TestDispatcher_Deliver_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(50 * time.Millisecond)
	sub := &Subscription{URL: srv.URL}

	_, err := d.Deliver(context.Background(), sub, []byte("{}"), "del-timeout")
	assert.Error(t, err)
}

func TestNewDispatcher_DefaultTimeout(t *testing.T) {
	d := NewDispatcher(0)
	assert.Equal(t, 10*time.Second, d.client.Timeout)
}
