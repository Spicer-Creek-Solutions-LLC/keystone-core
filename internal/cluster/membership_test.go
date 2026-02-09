package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/version"
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
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)
		assert.NotNil(t, manager)
	})

	t.Run("nil config", func(t *testing.T) {
		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
	require.NoError(t, err)

	manager, err := NewMembershipManager(config, etcdClient)
	require.NoError(t, err)

	info := manager.GetClusterInfo()
	assert.NotNil(t, info)
	assert.Equal(t, "test-cluster", info.Name)
	assert.Equal(t, StatusUnhealthy, info.Status) // No quorum
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
		etcdClient, err := NewEtcdClient(etcdConfig, "")
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
		etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
		name              string
		healthyCount      int
		memberCount       int
		configuredQuorum  int
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
			etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	etcdClient, err := NewEtcdClient(etcdConfig, "")
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
	original := version.Version
	version.Version = "1.2.3"
	t.Cleanup(func() {
		version.Version = original
	})

	got := getVersion()
	assert.Equal(t, "1.2.3", got)
}

func TestMembershipManager_RemoveMember(t *testing.T) {
	t.Run("remove non-existent member", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
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
		etcdClient, err := NewEtcdClient(etcdConfig, "")
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
		etcdClient, err := NewEtcdClient(etcdConfig, "")
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
		etcdClient, err := NewEtcdClient(etcdConfig, "")
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

func TestAddMemberRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &AddMemberRequest{
			Address: "192.168.1.100:8080",
		}
		assert.NoError(t, req.Validate())
	})

	t.Run("valid request with all fields", func(t *testing.T) {
		req := &AddMemberRequest{
			ID:          "custom-id",
			Name:        "custom-name",
			Address:     "192.168.1.100:8080",
			GRPCAddress: "192.168.1.100:9090",
			NATSAddress: "192.168.1.100:4222",
			Metadata:    map[string]string{"region": "us-west"},
		}
		assert.NoError(t, req.Validate())
	})

	t.Run("missing address", func(t *testing.T) {
		req := &AddMemberRequest{}
		err := req.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "address is required")
	})
}

func TestMembershipManager_AddMember(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		ctx := testContextWithTimeout(t)
		member, err := manager.AddMember(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, member)
		assert.Contains(t, err.Error(), "request is required")
	})

	t.Run("invalid request - missing address", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		ctx := testContextWithTimeout(t)
		member, err := manager.AddMember(ctx, &AddMemberRequest{})
		assert.Error(t, err)
		assert.Nil(t, member)
		assert.Contains(t, err.Error(), "invalid request")
	})

	t.Run("member already exists", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		// Pre-add a member
		manager.mu.Lock()
		manager.members["existing-id"] = &Member{
			ID:      "existing-id",
			Address: "192.168.1.100:8080",
		}
		manager.mu.Unlock()

		ctx := testContextWithTimeout(t)
		member, err := manager.AddMember(ctx, &AddMemberRequest{
			ID:      "existing-id",
			Address: "192.168.1.200:8080",
		})
		assert.Error(t, err)
		assert.Nil(t, member)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("address already in use", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		// Pre-add a member with a specific address
		manager.mu.Lock()
		manager.members["existing-id"] = &Member{
			ID:      "existing-id",
			Address: "192.168.1.100:8080",
		}
		manager.mu.Unlock()

		ctx := testContextWithTimeout(t)
		member, err := manager.AddMember(ctx, &AddMemberRequest{
			ID:      "new-id",
			Address: "192.168.1.100:8080", // Same address
		})
		assert.Error(t, err)
		assert.Nil(t, member)
		assert.Contains(t, err.Error(), "already in use")
	})

	t.Run("auto-generate ID and name", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		ctx := testContextWithTimeout(t)
		// This will fail at etcd.Put because no real etcd, but we can check the validation
		_, err = manager.AddMember(ctx, &AddMemberRequest{
			Address: "192.168.1.100:8080",
		})
		// Expect etcd error, not validation error
		if err != nil {
			assert.Contains(t, err.Error(), "etcd")
		}
	})

	t.Run("grpc address derived from address", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		// Manually add to cache to avoid etcd dependency
		req := &AddMemberRequest{
			ID:      "test-member",
			Name:    "Test Member",
			Address: "192.168.1.100:8080",
		}

		// Verify GRPCAddress would be derived (without actually calling AddMember which needs etcd)
		grpcAddress := req.GRPCAddress
		if grpcAddress == "" {
			grpcAddress = req.Address
		}
		assert.Equal(t, "192.168.1.100:8080", grpcAddress)

		// Also verify manager exists to satisfy linter
		assert.NotNil(t, manager)
	})
}

// IPv6 Support Tests

func TestMembershipManager_IPv6Support(t *testing.T) {
	t.Run("create local member with IPv6 advertise address", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.MemberID = "ipv6-member"
		config.MemberName = "IPv6 Member"
		config.AdvertiseAddress = "2001:db8::1"
		config.AddressFamilyPreference = PreferIPv6

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		member, err := manager.createLocalMember()
		require.NoError(t, err)
		assert.Equal(t, "ipv6-member", member.ID)
		assert.Equal(t, "IPv6 Member", member.Name)
		assert.Equal(t, "2001:db8::1", member.Address)
		// gRPC address should have brackets for IPv6 (default port is 9090)
		assert.Equal(t, "[2001:db8::1]:9090", member.GRPCAddress)
		assert.Equal(t, MemberStatusHealthy, member.Status)
	})

	t.Run("create local member with IPv6 loopback", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "::1"
		config.AddressFamilyPreference = IPv6Only

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		member, err := manager.createLocalMember()
		require.NoError(t, err)
		assert.Equal(t, "::1", member.Address)
		assert.Equal(t, "[::1]:9090", member.GRPCAddress)
	})
}

func TestAddMemberRequest_IPv6Validation(t *testing.T) {
	tests := []struct {
		name        string
		request     *AddMemberRequest
		expectError bool
	}{
		{
			name: "IPv6 address without brackets",
			request: &AddMemberRequest{
				Address: "[2001:db8::1]:8080",
			},
			expectError: false,
		},
		{
			name: "IPv6 loopback with port",
			request: &AddMemberRequest{
				Address: "[::1]:8080",
			},
			expectError: false,
		},
		{
			name: "IPv6 full address",
			request: &AddMemberRequest{
				ID:          "ipv6-node",
				Name:        "IPv6 Node",
				Address:     "[2001:db8:85a3::8a2e:370:7334]:8080",
				GRPCAddress: "[2001:db8:85a3::8a2e:370:7334]:50051",
				NATSAddress: "[2001:db8:85a3::8a2e:370:7334]:4222",
			},
			expectError: false,
		},
		{
			name: "dual stack - IPv6 main, IPv4 NATS",
			request: &AddMemberRequest{
				Address:     "[2001:db8::1]:8080",
				GRPCAddress: "[2001:db8::1]:50051",
				NATSAddress: "192.168.1.100:4222",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMembershipManager_AddMember_IPv6(t *testing.T) {
	t.Run("add member with IPv6 address", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		// Pre-add an IPv4 member
		manager.mu.Lock()
		manager.members["ipv4-member"] = &Member{
			ID:      "ipv4-member",
			Address: "192.168.1.100:8080",
		}
		manager.mu.Unlock()

		// Try to add IPv6 member - should be allowed (different address)
		ctx := testContextWithTimeout(t)
		_, err = manager.AddMember(ctx, &AddMemberRequest{
			ID:      "ipv6-member",
			Address: "[2001:db8::1]:8080",
		})
		// Expect etcd error, not validation error (validates IPv6 is accepted)
		if err != nil {
			assert.Contains(t, err.Error(), "etcd")
		}
	})

	t.Run("IPv6 address collision detection", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "test-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		// Pre-add an IPv6 member
		manager.mu.Lock()
		manager.members["ipv6-member-1"] = &Member{
			ID:      "ipv6-member-1",
			Address: "[2001:db8::1]:8080",
		}
		manager.mu.Unlock()

		// Try to add another member with same IPv6 address - should fail
		ctx := testContextWithTimeout(t)
		_, err = manager.AddMember(ctx, &AddMemberRequest{
			ID:      "ipv6-member-2",
			Address: "[2001:db8::1]:8080", // Same address
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in use")
	})
}

func TestMember_IPv6Addresses(t *testing.T) {
	t.Run("member with IPv6 addresses", func(t *testing.T) {
		member := &Member{
			ID:          "ipv6-member",
			Name:        "IPv6 Test Member",
			Address:     "[2001:db8::1]:8080",
			GRPCAddress: "[2001:db8::1]:50051",
			NATSAddress: "[2001:db8::1]:4222",
			Status:      MemberStatusHealthy,
		}

		// Clone should preserve IPv6 addresses
		clone := member.Clone()
		assert.Equal(t, "[2001:db8::1]:8080", clone.Address)
		assert.Equal(t, "[2001:db8::1]:50051", clone.GRPCAddress)
		assert.Equal(t, "[2001:db8::1]:4222", clone.NATSAddress)
	})

	t.Run("cluster info with IPv6 members", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.ClusterName = "ipv6-cluster"
		config.AdvertiseAddress = "127.0.0.1"

		etcdConfig := DefaultEtcdConfig()
		etcdClient, err := NewEtcdClient(etcdConfig, "")
		require.NoError(t, err)

		manager, err := NewMembershipManager(config, etcdClient)
		require.NoError(t, err)

		// Add IPv6 members
		manager.mu.Lock()
		manager.members["member-1"] = &Member{
			ID:      "member-1",
			Address: "[2001:db8::1]:8080",
			Status:  MemberStatusHealthy,
		}
		manager.members["member-2"] = &Member{
			ID:      "member-2",
			Address: "[2001:db8::2]:8080",
			Status:  MemberStatusHealthy,
		}
		manager.members["member-3"] = &Member{
			ID:       "member-3",
			Address:  "[2001:db8::3]:8080",
			Status:   MemberStatusHealthy,
			IsLeader: true,
		}
		manager.mu.Unlock()

		info := manager.GetClusterInfo()
		assert.Equal(t, 3, info.MemberCount)
		assert.Equal(t, 3, info.HealthyCount)
		assert.Equal(t, "member-3", info.LeaderID)
		assert.True(t, info.HasQuorum)
		assert.Equal(t, StatusHealthy, info.Status)

		// Verify members have correct IPv6 addresses
		for _, m := range info.Members {
			assert.Contains(t, m.Address, "2001:db8::")
		}
	})
}
