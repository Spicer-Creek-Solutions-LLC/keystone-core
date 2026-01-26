package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEtcdClient(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := DefaultEtcdConfig()
		client, err := NewEtcdClient(config, "")
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.False(t, client.IsConnected())
	})

	t.Run("nil config", func(t *testing.T) {
		client, err := NewEtcdClient(nil, "")
		assert.Error(t, err)
		assert.Nil(t, client)
	})

	t.Run("invalid config", func(t *testing.T) {
		config := &EtcdConfig{
			Mode: "invalid",
		}
		client, err := NewEtcdClient(config, "")
		assert.Error(t, err)
		assert.Nil(t, client)
	})
}

func TestEtcdClient_NotConnected(t *testing.T) {
	config := DefaultEtcdConfig()
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("get not connected", func(t *testing.T) {
		_, err := client.Get(ctx, "key")
		assert.ErrorIs(t, err, ErrEtcdNotConnected)
	})

	t.Run("put not connected", func(t *testing.T) {
		err := client.Put(ctx, "key", []byte("value"), 0)
		assert.ErrorIs(t, err, ErrEtcdNotConnected)
	})

	t.Run("delete not connected", func(t *testing.T) {
		err := client.Delete(ctx, "key")
		assert.ErrorIs(t, err, ErrEtcdNotConnected)
	})

	t.Run("list not connected", func(t *testing.T) {
		_, err := client.List(ctx, "prefix")
		assert.ErrorIs(t, err, ErrEtcdNotConnected)
	})

	t.Run("watch not connected", func(t *testing.T) {
		err := client.Watch(ctx, "prefix", func(key string, value []byte, deleted bool) {})
		assert.ErrorIs(t, err, ErrEtcdNotConnected)
	})

	t.Run("compare and swap not connected", func(t *testing.T) {
		_, err := client.CompareAndSwap(ctx, "key", nil, []byte("value"))
		assert.ErrorIs(t, err, ErrEtcdNotConnected)
	})

	t.Run("health not connected", func(t *testing.T) {
		err := client.Health(ctx)
		assert.ErrorIs(t, err, ErrEtcdNotConnected)
	})

	t.Run("keep alive not connected", func(t *testing.T) {
		_, err := client.KeepAlive(ctx)
		assert.ErrorIs(t, err, ErrEtcdNotConnected)
	})

	t.Run("put with lease not connected", func(t *testing.T) {
		err := client.PutWithLease(ctx, "key", []byte("value"))
		assert.ErrorIs(t, err, ErrEtcdNotConnected)
	})
}

func TestEtcdClient_Closed(t *testing.T) {
	config := DefaultEtcdConfig()
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	// Close the client
	err = client.Close()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("connect after close", func(t *testing.T) {
		err := client.Connect(ctx)
		assert.ErrorIs(t, err, ErrShutdown)
	})

	t.Run("get after close", func(t *testing.T) {
		_, err := client.Get(ctx, "key")
		assert.ErrorIs(t, err, ErrShutdown)
	})

	t.Run("put after close", func(t *testing.T) {
		err := client.Put(ctx, "key", []byte("value"), 0)
		assert.ErrorIs(t, err, ErrShutdown)
	})

	t.Run("close is idempotent", func(t *testing.T) {
		err := client.Close()
		assert.NoError(t, err)
	})
}

func TestEtcdClient_FullKey(t *testing.T) {
	config := DefaultEtcdConfig()
	config.KeyPrefix = "/test-prefix"
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	// Test the internal fullKey method
	key := client.fullKey("/my/key")
	assert.Equal(t, "/test-prefix/my/key", key)
}

func TestEtcdClient_Endpoints(t *testing.T) {
	t.Run("external mode", func(t *testing.T) {
		config := &EtcdConfig{
			Mode:           EtcdModeExternal,
			Endpoints:      []string{"etcd1:2379", "etcd2:2379"},
			DialTimeout:    5 * time.Second,
			RequestTimeout: 10 * time.Second,
			LeasesTTL:      15,
			KeyPrefix:      "/test",
		}
		client, err := NewEtcdClient(config, "")
		require.NoError(t, err)

		endpoints := client.Endpoints()
		assert.Equal(t, []string{"etcd1:2379", "etcd2:2379"}, endpoints)
	})

	t.Run("embedded mode", func(t *testing.T) {
		config := DefaultEtcdConfig()
		config.Embedded.ClientPort = 12379
		client, err := NewEtcdClient(config, "")
		require.NoError(t, err)

		endpoints := client.Endpoints()
		assert.Equal(t, []string{"localhost:12379"}, endpoints)
	})
}

func TestEtcdClient_Session(t *testing.T) {
	config := DefaultEtcdConfig()
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	// Session should be nil before creation
	assert.Nil(t, client.Session())
	assert.Equal(t, int64(0), int64(client.LeaseID()))
}

func TestTransaction(t *testing.T) {
	config := DefaultEtcdConfig()
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("transaction not connected", func(t *testing.T) {
		txn := client.Transaction(ctx)
		txn.Put("key", []byte("value"))
		success, err := txn.Commit()
		assert.Error(t, err)
		assert.False(t, success)
	})

	t.Run("transaction with conditions", func(t *testing.T) {
		txn := client.Transaction(ctx)
		txn.If("key", "=", "expected")
		txn.IfNotExists("new-key")
		txn.IfExists("existing-key")
		txn.Put("key", []byte("value"))
		txn.Delete("old-key")

		// Will fail because not connected
		success, err := txn.Commit()
		assert.Error(t, err)
		assert.False(t, success)
	})
}

func TestEtcdClient_WithRetry(t *testing.T) {
	config := DefaultEtcdConfig()
	config.MaxRetries = 3
	config.RetryInterval = 10 * time.Millisecond
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	attempts := 0
	err = client.WithRetry(ctx, func() error {
		attempts++
		return assert.AnError
	})

	assert.Error(t, err)
	assert.Equal(t, 3, attempts)
}

func TestEtcdClient_WithRetry_Success(t *testing.T) {
	config := DefaultEtcdConfig()
	config.MaxRetries = 3
	config.RetryInterval = 10 * time.Millisecond
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	ctx := context.Background()
	attempts := 0
	err = client.WithRetry(ctx, func() error {
		attempts++
		if attempts < 2 {
			return assert.AnError
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestEtcdClient_WithRetry_ContextCanceled(t *testing.T) {
	config := DefaultEtcdConfig()
	config.MaxRetries = 10
	config.RetryInterval = 100 * time.Millisecond
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	attempts := 0
	err = client.WithRetry(ctx, func() error {
		attempts++
		return assert.AnError
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, attempts)
}

func TestEtcdClient_Client(t *testing.T) {
	config := DefaultEtcdConfig()
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	// Client should be nil when not connected
	assert.Nil(t, client.Client())
}

func TestEtcdClient_DeletePrefix(t *testing.T) {
	config := DefaultEtcdConfig()
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	ctx := context.Background()
	err = client.DeletePrefix(ctx, "prefix")
	assert.ErrorIs(t, err, ErrEtcdNotConnected)
}

func TestEtcdClient_RevokeSession(t *testing.T) {
	config := DefaultEtcdConfig()
	client, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	ctx := context.Background()

	// Should fail because not connected
	err = client.RevokeSession(ctx)
	assert.ErrorIs(t, err, ErrEtcdNotConnected)
}
