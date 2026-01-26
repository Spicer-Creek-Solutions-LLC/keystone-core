package cluster

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmbeddedEtcdServer(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := DefaultEtcdConfig()
		config.Mode = EtcdModeEmbedded

		server, err := NewEmbeddedEtcdServer(config, "test-member")
		require.NoError(t, err)
		assert.NotNil(t, server)
		assert.False(t, server.IsRunning())
	})

	t.Run("nil config", func(t *testing.T) {
		server, err := NewEmbeddedEtcdServer(nil, "test-member")
		assert.Error(t, err)
		assert.Nil(t, server)
		assert.Contains(t, err.Error(), "config is required")
	})

	t.Run("wrong mode", func(t *testing.T) {
		config := DefaultEtcdConfig()
		config.Mode = EtcdModeExternal

		server, err := NewEmbeddedEtcdServer(config, "test-member")
		assert.Error(t, err)
		assert.Nil(t, server)
		assert.Contains(t, err.Error(), "must be 'embedded'")
	})

	t.Run("nil embedded config", func(t *testing.T) {
		config := &EtcdConfig{
			Mode:     EtcdModeEmbedded,
			Embedded: nil,
		}

		server, err := NewEmbeddedEtcdServer(config, "test-member")
		assert.Error(t, err)
		assert.Nil(t, server)
		assert.Contains(t, err.Error(), "embedded etcd configuration is required")
	})
}

func TestEmbeddedEtcdServer_ClientEndpoint(t *testing.T) {
	config := DefaultEtcdConfig()
	config.Mode = EtcdModeEmbedded
	config.Embedded.ClientPort = 12379

	server, err := NewEmbeddedEtcdServer(config, "test-member")
	require.NoError(t, err)

	endpoint := server.ClientEndpoint()
	assert.Equal(t, "localhost:12379", endpoint)
}

func TestEmbeddedEtcdServer_DataDir(t *testing.T) {
	config := DefaultEtcdConfig()
	config.Mode = EtcdModeEmbedded

	server, err := NewEmbeddedEtcdServer(config, "test-member")
	require.NoError(t, err)

	// Before start, data dir should be empty
	assert.Empty(t, server.DataDir())
}

func TestEmbeddedEtcdServer_StopWithoutStart(t *testing.T) {
	config := DefaultEtcdConfig()
	config.Mode = EtcdModeEmbedded

	server, err := NewEmbeddedEtcdServer(config, "test-member")
	require.NoError(t, err)

	// Stop without start should be safe
	err = server.Stop()
	assert.NoError(t, err)
}

func TestEmbeddedEtcdServer_StartAndStop(t *testing.T) {
	// Skip in short mode - this test actually starts an embedded etcd server
	if testing.Short() {
		t.Skip("skipping embedded etcd test in short mode")
	}

	// Create a unique data directory
	tempDir, err := os.MkdirTemp("", "etcd-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := DefaultEtcdConfig()
	config.Mode = EtcdModeEmbedded
	config.Embedded.DataDir = filepath.Join(tempDir, "data")
	config.Embedded.ClientPort = 22379 // Use non-standard port to avoid conflicts
	config.Embedded.PeerPort = 22380

	server, err := NewEmbeddedEtcdServer(config, "test-member")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start the server
	err = server.Start(ctx)
	require.NoError(t, err, "failed to start embedded etcd server")
	assert.True(t, server.IsRunning())
	assert.NotEmpty(t, server.DataDir())

	// Stop the server
	err = server.Stop()
	require.NoError(t, err)
	assert.False(t, server.IsRunning())
}

func TestEmbeddedEtcdServer_StartAlreadyRunning(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping embedded etcd test in short mode")
	}

	tempDir, err := os.MkdirTemp("", "etcd-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := DefaultEtcdConfig()
	config.Mode = EtcdModeEmbedded
	config.Embedded.DataDir = filepath.Join(tempDir, "data")
	config.Embedded.ClientPort = 22381
	config.Embedded.PeerPort = 22382

	server, err := NewEmbeddedEtcdServer(config, "test-member")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start the server
	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// Start again should be a no-op (return nil)
	err = server.Start(ctx)
	assert.NoError(t, err)
	assert.True(t, server.IsRunning())
}

func TestEtcdClient_EmbeddedMode(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping embedded etcd test in short mode")
	}

	tempDir, err := os.MkdirTemp("", "etcd-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config := DefaultEtcdConfig()
	config.Mode = EtcdModeEmbedded
	config.Embedded.DataDir = filepath.Join(tempDir, "data")
	config.Embedded.ClientPort = 22383
	config.Embedded.PeerPort = 22384

	client, err := NewEtcdClient(config, "test-member")
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Connect should start the embedded server automatically
	err = client.Connect(ctx)
	require.NoError(t, err, "failed to connect to embedded etcd")

	assert.True(t, client.IsConnected())

	// Verify we can do basic operations
	err = client.Put(ctx, "/test/key", []byte("test-value"), 0)
	require.NoError(t, err)

	value, err := client.Get(ctx, "/test/key")
	require.NoError(t, err)
	assert.Equal(t, []byte("test-value"), value)

	// Clean up
	err = client.Delete(ctx, "/test/key")
	require.NoError(t, err)
}

func TestParseZapLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"warning", "warn"},
		{"error", "error"},
		{"fatal", "fatal"},
		{"unknown", "warn"}, // defaults to warn
		{"", "warn"},        // empty defaults to warn
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseZapLevel(tt.input)
			assert.Equal(t, tt.expected, level.String())
		})
	}
}
