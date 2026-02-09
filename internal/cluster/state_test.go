package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStateStore(t *testing.T) {
	t.Run("valid etcd client", func(t *testing.T) {
		config := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(config, "")
		require.NoError(t, err)

		store, err := NewStateStore(etcdClient)
		require.NoError(t, err)
		assert.NotNil(t, store)
	})

	t.Run("nil etcd client", func(t *testing.T) {
		store, err := NewStateStore(nil)
		assert.Error(t, err)
		assert.Nil(t, store)
	})
}

func TestNewConfigStore(t *testing.T) {
	t.Run("valid etcd client", func(t *testing.T) {
		config := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(config, "")
		require.NoError(t, err)

		store, err := NewConfigStore(etcdClient)
		require.NoError(t, err)
		assert.NotNil(t, store)
	})

	t.Run("nil etcd client", func(t *testing.T) {
		store, err := NewConfigStore(nil)
		assert.Error(t, err)
		assert.Nil(t, store)
	})
}

func TestNewShardStore(t *testing.T) {
	t.Run("valid etcd client", func(t *testing.T) {
		config := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(config, "")
		require.NoError(t, err)

		store, err := NewShardStore(etcdClient)
		require.NoError(t, err)
		assert.NotNil(t, store)
	})

	t.Run("nil etcd client", func(t *testing.T) {
		store, err := NewShardStore(nil)
		assert.Error(t, err)
		assert.Nil(t, store)
	})
}

func TestNewCoordinationStore(t *testing.T) {
	t.Run("valid etcd client", func(t *testing.T) {
		config := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(config, "")
		require.NoError(t, err)

		store, err := NewCoordinationStore(etcdClient)
		require.NoError(t, err)
		assert.NotNil(t, store)
	})

	t.Run("nil etcd client", func(t *testing.T) {
		store, err := NewCoordinationStore(nil)
		assert.Error(t, err)
		assert.Nil(t, store)
	})
}

func TestDistributedLock(t *testing.T) {
	config := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	lock := NewDistributedLock(etcdClient, "test-lock")
	assert.NotNil(t, lock)
	assert.False(t, lock.IsLocked())
}

func TestDistributedLock_NotConnected(t *testing.T) {
	config := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	lock := NewDistributedLock(etcdClient, "test-lock")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	t.Run("lock not connected", func(t *testing.T) {
		err := lock.Lock(ctx)
		assert.Error(t, err)
	})

	t.Run("try lock not connected", func(t *testing.T) {
		success, err := lock.TryLock(ctx)
		assert.Error(t, err)
		assert.False(t, success)
	})

	t.Run("unlock when not locked", func(t *testing.T) {
		err := lock.Unlock(ctx)
		assert.NoError(t, err)
	})
}

func TestCounter(t *testing.T) {
	config := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	counter := NewCounter(etcdClient, "test-counter")
	assert.NotNil(t, counter)
}

func TestCounter_NotConnected(t *testing.T) {
	config := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	counter := NewCounter(etcdClient, "test-counter")
	ctx := context.Background()

	t.Run("get not connected", func(t *testing.T) {
		_, err := counter.Get(ctx)
		assert.Error(t, err)
	})

	t.Run("increment not connected", func(t *testing.T) {
		_, err := counter.Increment(ctx)
		assert.Error(t, err)
	})

	t.Run("decrement not connected", func(t *testing.T) {
		_, err := counter.Decrement(ctx)
		assert.Error(t, err)
	})

	t.Run("add not connected", func(t *testing.T) {
		_, err := counter.Add(ctx, 5)
		assert.Error(t, err)
	})

	t.Run("set not connected", func(t *testing.T) {
		err := counter.Set(ctx, 100)
		assert.Error(t, err)
	})

	t.Run("reset not connected", func(t *testing.T) {
		err := counter.Reset(ctx)
		assert.Error(t, err)
	})
}

func TestStateStore_NotConnected(t *testing.T) {
	config := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	store, err := NewStateStore(etcdClient)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("get not connected", func(t *testing.T) {
		_, err := store.Get(ctx, "key")
		assert.Error(t, err)
	})

	t.Run("set not connected", func(t *testing.T) {
		err := store.Set(ctx, "key", []byte("value"), 0)
		assert.Error(t, err)
	})

	t.Run("delete not connected", func(t *testing.T) {
		err := store.Delete(ctx, "key")
		assert.Error(t, err)
	})

	t.Run("list not connected", func(t *testing.T) {
		_, err := store.List(ctx, "prefix")
		assert.Error(t, err)
	})

	t.Run("compare and swap not connected", func(t *testing.T) {
		_, err := store.CompareAndSwap(ctx, "key", nil, []byte("value"))
		assert.Error(t, err)
	})
}

func TestConfigStore_NotConnected(t *testing.T) {
	config := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	store, err := NewConfigStore(etcdClient)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("get config not connected", func(t *testing.T) {
		_, err := store.GetConfig(ctx, "key")
		assert.Error(t, err)
	})

	t.Run("set config not connected", func(t *testing.T) {
		err := store.SetConfig(ctx, "key", []byte("value"))
		assert.Error(t, err)
	})

	t.Run("delete config not connected", func(t *testing.T) {
		err := store.DeleteConfig(ctx, "key")
		assert.Error(t, err)
	})

	t.Run("list configs not connected", func(t *testing.T) {
		_, err := store.ListConfigs(ctx, "prefix")
		assert.Error(t, err)
	})
}

func TestShardStore_NotConnected(t *testing.T) {
	config := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	store, err := NewShardStore(etcdClient)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("get assignment not connected", func(t *testing.T) {
		_, err := store.GetAssignment(ctx, "agent-1")
		assert.Error(t, err)
	})

	t.Run("set assignment not connected", func(t *testing.T) {
		assignment := &ShardAssignment{
			AgentID:    "agent-1",
			MemberID:   "member-1",
			AssignedAt: time.Now().UTC(),
		}
		err := store.SetAssignment(ctx, assignment)
		assert.Error(t, err)
	})

	t.Run("delete assignment not connected", func(t *testing.T) {
		err := store.DeleteAssignment(ctx, "agent-1")
		assert.Error(t, err)
	})

	t.Run("list assignments not connected", func(t *testing.T) {
		_, err := store.ListAssignments(ctx)
		assert.Error(t, err)
	})

	t.Run("list assignments for member not connected", func(t *testing.T) {
		_, err := store.ListAssignmentsForMember(ctx, "member-1")
		assert.Error(t, err)
	})

	t.Run("compare and swap assignment not connected", func(t *testing.T) {
		assignment := &ShardAssignment{
			AgentID:    "agent-1",
			MemberID:   "member-1",
			AssignedAt: time.Now().UTC(),
		}
		_, err := store.CompareAndSwapAssignment(ctx, assignment)
		assert.Error(t, err)
	})
}

func TestCoordinationStore_NotConnected(t *testing.T) {
	config := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(config, "")
	require.NoError(t, err)

	store, err := NewCoordinationStore(etcdClient)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	t.Run("barrier not connected", func(t *testing.T) {
		err := store.Barrier(ctx, "test-barrier", "member-1", 3)
		assert.Error(t, err)
	})

	t.Run("elect not connected", func(t *testing.T) {
		_, err := store.Elect(ctx, "test-election", "member-1")
		assert.Error(t, err)
	})

	t.Run("get elected not connected", func(t *testing.T) {
		_, err := store.GetElected(ctx, "test-election")
		assert.Error(t, err)
	})

	t.Run("resign not connected", func(t *testing.T) {
		err := store.Resign(ctx, "test-election", "member-1")
		assert.Error(t, err)
	})
}

func TestKeyPrefixes(t *testing.T) {
	// Verify key prefixes are properly defined
	assert.Equal(t, "/members/", memberKeyPrefix)
	assert.Equal(t, "/cluster/meta", memberMetaKey)
	assert.Equal(t, "/state/", stateKeyPrefix)
	assert.Equal(t, "/config/", configKeyPrefix)
	assert.Equal(t, "/leader/", leaderKeyPrefix)
	assert.Equal(t, "/shards/", shardKeyPrefix)
	assert.Equal(t, "/locks/", lockKeyPrefix)
	assert.Equal(t, "/coordination/", coordinationPrefix)
}
