package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMembershipManager(t *testing.T) {
	t.Run("valid config and etcd", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig)
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)
		assert.NotNil(t, manager)
	})

	t.Run("nil config", func(t *testing.T) {
		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig)
		require.NoError(t, err)

		manager, err := NewMembershipManager(nil, etcdClient)
		assert.Error(t, err)
		assert.Nil(t, manager)
	})

	t.Run("nil etcd client", func(t *testing.T) {
		config := DefaultConfig()
		manager, err := NewMembershipManager(config, nil)
		assert.Error(t, err)
		assert.Nil(t, manager)
	})
}

func TestMembershipManager_LocalMember(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	// Before start, no local member
	assert.Nil(t, manager.LocalMember())
}

func TestMembershipManager_MemberCount(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	assert.Equal(t, 0, manager.MemberCount())
	assert.Equal(t, 0, manager.HealthyMemberCount())
}

func TestMembershipManager_GetMember(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	// Member not found
	member, err := manager.GetMember("non-existent")
	assert.ErrorIs(t, err, ErrMemberNotFound)
	assert.Nil(t, member)
}

func TestMembershipManager_ListMembers(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	members := manager.ListMembers()
	assert.Empty(t, members)
}

func TestMembershipManager_GetHealthyMembers(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	members := manager.GetHealthyMembers()
	assert.Empty(t, members)
}

func TestMembershipManager_HasQuorum(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	// Empty cluster has no quorum
	assert.False(t, manager.HasQuorum())
}

func TestMembershipManager_GetLeader(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	// No leader initially
	assert.Nil(t, manager.GetLeader())
}

func TestMembershipManager_SetLeader(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	// Set leader with no members - should not panic
	manager.SetLeader("member-1")
}

func TestMembershipManager_Observers(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	eventReceived := make(chan MembershipEvent, 1)
	observer := func(event MembershipEvent) {
		select {
		case eventReceived <- event:
		default:
		}
	}

	manager.AddObserver(observer)
	manager.RemoveObserver(observer)
}

func TestMembershipManager_GetClusterInfo(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	info := manager.GetClusterInfo()
	assert.NotNil(t, info)
	assert.Equal(t, "test-cluster", info.Name)
	assert.Equal(t, ClusterStatusUnhealthy, info.Status) // No quorum
	assert.Empty(t, info.LeaderID)
	assert.Empty(t, info.Members)
	assert.Equal(t, 0, info.MemberCount)
	assert.Equal(t, 0, info.HealthyCount)
	assert.False(t, info.HasQuorum)
}

func TestMembershipManager_CreateLocalMember(t *testing.T) {
	t.Run("with member id", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.MemberID = "custom-member-id"
		config.MemberName = "custom-member-name"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig)
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		// Create local member manually (normally done in Start)
		member, err := manager.createLocalMember()
		require.NoError(t, err)
		assert.Equal(t, "custom-member-id", member.ID)
		assert.Equal(t, "custom-member-name", member.Name)
		assert.Equal(t, "127.0.0.1", member.Address)
		assert.Equal(t, MemberStatusHealthy, member.Status)
		assert.False(t, member.IsLeader)
	})

	t.Run("without member id generates uuid", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig)
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		member, err := manager.createLocalMember()
		require.NoError(t, err)
		assert.NotEmpty(t, member.ID)
		assert.Len(t, member.ID, 36) // UUID format
	})
}

func TestMembershipManager_StopWithoutStart(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	// Stop without start should be safe
	ctx := testContextWithTimeout(t)
	err = manager.Stop(ctx)
	assert.NoError(t, err)
}

func TestMembershipManager_QuorumCalculation(t *testing.T) {
	tests := []struct {
		name            string
		healthyCount    int
		memberCount     int
		configuredQuorum int
		expectedHasQuorum bool
	}{
		{"empty cluster", 0, 0, 0, false},
		{"single member", 1, 1, 0, true},
		{"3 members all healthy", 3, 3, 0, true},
		{"3 members 2 healthy", 2, 3, 0, true},
		{"3 members 1 healthy", 1, 3, 0, false},
		{"5 members 3 healthy", 3, 5, 0, true},
		{"5 members 2 healthy", 2, 5, 0, false},
		{"custom quorum met", 2, 3, 2, true},
		{"custom quorum not met", 1, 3, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.Enabled = true
			config.ClusterName = "test-cluster"
			config.AdvertiseAddress = "127.0.0.1"
			config.QuorumSize = tt.configuredQuorum

			etcdConfig := DefaultEtcdConfig()
			etcdClient, err := NewEtcdClient(etcdConfig)
			require.NoError(t, err)

			manager, err := NewMembershipManager(config, etcdClient)
			require.NoError(t, err)

			// Manually set up members
			manager.mu.Lock()
			for i := 0; i < tt.memberCount; i++ {
				status := MemberStatusUnhealthy
				if i < tt.healthyCount {
					status = MemberStatusHealthy
				}
				manager.members[string(rune('A'+i))] = &Member{
					ID:     string(rune('A' + i)),
					Status: status,
				}
			}
			manager.mu.Unlock()

			assert.Equal(t, tt.expectedHasQuorum, manager.HasQuorum())
		})
	}
}

func TestMembershipManager_MemberHealthCheck(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"
	config.HeartbeatTimeout = 30 * time.Second

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	// Set up a local member so we can test health checks
	member, err := manager.createLocalMember()
	require.NoError(t, err)

	manager.mu.Lock()
	manager.localMember = member
	manager.members[member.ID] = member

	// Add another member that's stale
	staleMember := &Member{
		ID:            "stale-member",
		Status:        MemberStatusHealthy,
		LastHeartbeat: time.Now().Add(-60 * time.Second), // Stale
	}
	manager.members["stale-member"] = staleMember
	manager.mu.Unlock()

	// Run health check
	manager.checkMemberHealth()

	// Stale member should now be unhealthy
	manager.mu.RLock()
	assert.Equal(t, MemberStatusUnhealthy, manager.members["stale-member"].Status)
	manager.mu.RUnlock()
}

func TestMembershipManager_MemberRecovery(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"
	config.HeartbeatTimeout = 30 * time.Second

	etcdConfig := DefaultEtcdConfig()
	etcdClient, err := NewEtcdClient(etcdConfig)
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	// Set up a local member
	member, err := manager.createLocalMember()
	require.NoError(t, err)

	manager.mu.Lock()
	manager.localMember = member
	manager.members[member.ID] = member

	// Add a member that was unhealthy but now has recent heartbeat
	recoveredMember := &Member{
		ID:            "recovered-member",
		Status:        MemberStatusUnhealthy,
		LastHeartbeat: time.Now(), // Fresh heartbeat
	}
	manager.members["recovered-member"] = recoveredMember
	manager.mu.Unlock()

	// Run health check
	manager.checkMemberHealth()

	// Recovered member should now be healthy
	manager.mu.RLock()
	assert.Equal(t, MemberStatusHealthy, manager.members["recovered-member"].Status)
	manager.mu.RUnlock()
}

func testContextWithTimeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestGetVersion(t *testing.T) {
	version := getVersion()
	assert.NotEmpty(t, version)
	assert.Contains(t, version, "0.11.0")
}

func TestMembershipManager_RemoveMember(t *testing.T) {
	t.Run("remove non-existent member", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig)
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		ctx := testContextWithTimeout(t)
		err = manager.RemoveMember(ctx, "non-existent", true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("cannot remove local member", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.MemberID = "local-member"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig)
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		// Set local member
		manager.mu.Lock()
		manager.localMember = &Member{ID: "local-member"}
		manager.members["local-member"] = &Member{ID: "local-member", Status: MemberStatusHealthy}
		manager.mu.Unlock()

		ctx := testContextWithTimeout(t)
		err = manager.RemoveMember(ctx, "local-member", true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot remove local member")
	})

	t.Run("cannot remove healthy member without force", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig)
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		// Add a healthy member
		manager.mu.Lock()
		manager.members["healthy-member"] = &Member{
			ID:     "healthy-member",
			Status: MemberStatusHealthy,
		}
		manager.mu.Unlock()

		ctx := testContextWithTimeout(t)
		err = manager.RemoveMember(ctx, "healthy-member", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not unhealthy")
		assert.Contains(t, err.Error(), "use force=true")
	})

	t.Run("can remove unhealthy member without force", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig)
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		// Add an unhealthy member
		manager.mu.Lock()
		manager.members["unhealthy-member"] = &Member{
			ID:     "unhealthy-member",
			Status: MemberStatusUnhealthy,
		}
		manager.mu.Unlock()

		ctx := testContextWithTimeout(t)
		// This will fail because etcd isn't running, but it will pass the validation checks
		err = manager.RemoveMember(ctx, "unhealthy-member", false)
		// We expect an error because etcd.Delete will fail (no real etcd)
		// The important thing is it passed the validation checks
		if err != nil {
			assert.Contains(t, err.Error(), "etcd")
		}
	})
}
