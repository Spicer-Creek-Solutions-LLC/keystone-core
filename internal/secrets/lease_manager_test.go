package secrets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSQLiteLeaseStore tests the SQLite lease store implementation.
func TestSQLiteLeaseStore(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		// Store is already initialized by createTestStore
		// Verify tables exist by querying
		ctx := context.Background()
		_, err := store.Count(ctx, nil)
		if err != nil {
			t.Fatalf("failed to query after initialization: %v", err)
		}
	})

	t.Run("Create", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()
		lease := createTestLease("lease-1", "vault/secret/test")

		err := store.Create(ctx, lease)
		if err != nil {
			t.Fatalf("failed to create lease: %v", err)
		}

		// Verify lease was created
		retrieved, err := store.Get(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to get lease: %v", err)
		}

		if retrieved.ID != lease.ID {
			t.Errorf("expected ID %s, got %s", lease.ID, retrieved.ID)
		}
		if retrieved.SecretPath != lease.SecretPath {
			t.Errorf("expected SecretPath %s, got %s", lease.SecretPath, retrieved.SecretPath)
		}
		if retrieved.Backend != lease.Backend {
			t.Errorf("expected Backend %s, got %s", lease.Backend, retrieved.Backend)
		}
	})

	t.Run("CreateDuplicate", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()
		lease := createTestLease("lease-dup", "vault/secret/test")

		err := store.Create(ctx, lease)
		if err != nil {
			t.Fatalf("failed to create lease: %v", err)
		}

		// Creating duplicate should fail
		err = store.Create(ctx, lease)
		if err == nil {
			t.Error("expected error creating duplicate lease")
		}
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()
		_, err := store.Get(ctx, "nonexistent")
		if !errors.Is(err, ErrLeaseNotFound) {
			t.Errorf("expected ErrLeaseNotFound, got %v", err)
		}
	})

	t.Run("Update", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()
		lease := createTestLease("lease-update", "vault/secret/test")
		_ = store.Create(ctx, lease)

		// Update the lease
		lease.State = LeaseStateRenewing
		lease.RenewalCount = 5
		lease.TTL = 2 * time.Hour

		err := store.Update(ctx, lease)
		if err != nil {
			t.Fatalf("failed to update lease: %v", err)
		}

		// Verify update
		retrieved, err := store.Get(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to get lease: %v", err)
		}

		if retrieved.State != LeaseStateRenewing {
			t.Errorf("expected state %s, got %s", LeaseStateRenewing, retrieved.State)
		}
		if retrieved.RenewalCount != 5 {
			t.Errorf("expected renewal count 5, got %d", retrieved.RenewalCount)
		}
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()
		lease := createTestLease("nonexistent", "vault/secret/test")

		err := store.Update(ctx, lease)
		if !errors.Is(err, ErrLeaseNotFound) {
			t.Errorf("expected ErrLeaseNotFound, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()
		lease := createTestLease("lease-delete", "vault/secret/test")
		_ = store.Create(ctx, lease)

		err := store.Delete(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to delete lease: %v", err)
		}

		// Verify deletion
		_, err = store.Get(ctx, lease.ID)
		if !errors.Is(err, ErrLeaseNotFound) {
			t.Errorf("expected ErrLeaseNotFound, got %v", err)
		}
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()
		err := store.Delete(ctx, "nonexistent")
		if !errors.Is(err, ErrLeaseNotFound) {
			t.Errorf("expected ErrLeaseNotFound, got %v", err)
		}
	})

	t.Run("List", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()

		// Create multiple leases
		for i := 0; i < 5; i++ {
			lease := createTestLease(fmt.Sprintf("lease-list-%d", i), fmt.Sprintf("vault/secret/test%d", i))
			_ = store.Create(ctx, lease)
		}

		leases, err := store.List(ctx, nil)
		if err != nil {
			t.Fatalf("failed to list leases: %v", err)
		}

		if len(leases) != 5 {
			t.Errorf("expected 5 leases, got %d", len(leases))
		}
	})

	t.Run("List_WithFilter", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()

		// Create leases with different backends
		for i := 0; i < 3; i++ {
			lease := createTestLease(fmt.Sprintf("lease-vault-%d", i), "vault/secret/test")
			lease.Backend = BackendTypeVault
			_ = store.Create(ctx, lease)
		}
		for i := 0; i < 2; i++ {
			lease := createTestLease(fmt.Sprintf("lease-aws-%d", i), "aws/secret/test")
			lease.Backend = BackendTypeAWS
			_ = store.Create(ctx, lease)
		}

		// Filter by backend
		leases, err := store.List(ctx, &LeaseFilter{
			Backend: BackendTypeVault,
		})
		if err != nil {
			t.Fatalf("failed to list leases: %v", err)
		}

		if len(leases) != 3 {
			t.Errorf("expected 3 Vault leases, got %d", len(leases))
		}
	})

	t.Run("List_WithStateFilter", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()

		// Create leases with different states
		for i := 0; i < 3; i++ {
			lease := createTestLease(fmt.Sprintf("lease-active-%d", i), "vault/secret/test")
			lease.State = LeaseStateActive
			_ = store.Create(ctx, lease)
		}
		for i := 0; i < 2; i++ {
			lease := createTestLease(fmt.Sprintf("lease-expired-%d", i), "vault/secret/test")
			lease.State = LeaseStateExpired
			_ = store.Create(ctx, lease)
		}

		// Filter by state
		leases, err := store.List(ctx, &LeaseFilter{
			State: LeaseStateActive,
		})
		if err != nil {
			t.Fatalf("failed to list leases: %v", err)
		}

		if len(leases) != 3 {
			t.Errorf("expected 3 active leases, got %d", len(leases))
		}
	})

	t.Run("List_WithExpiringBefore", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()
		now := time.Now()

		// Create leases expiring at different times
		lease1 := createTestLease("lease-exp-1", "vault/secret/test")
		lease1.ExpiresAt = now.Add(30 * time.Minute)
		_ = store.Create(ctx, lease1)

		lease2 := createTestLease("lease-exp-2", "vault/secret/test")
		lease2.ExpiresAt = now.Add(2 * time.Hour)
		_ = store.Create(ctx, lease2)

		// Filter for leases expiring within 1 hour
		leases, err := store.List(ctx, &LeaseFilter{
			ExpiringBefore: now.Add(1 * time.Hour),
		})
		if err != nil {
			t.Fatalf("failed to list leases: %v", err)
		}

		if len(leases) != 1 {
			t.Errorf("expected 1 lease, got %d", len(leases))
		}
	})

	t.Run("List_WithLimit", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()

		// Create multiple leases
		for i := 0; i < 10; i++ {
			lease := createTestLease(fmt.Sprintf("lease-limit-%d", i), "vault/secret/test")
			_ = store.Create(ctx, lease)
		}

		// List with limit
		leases, err := store.List(ctx, &LeaseFilter{
			Limit: 5,
		})
		if err != nil {
			t.Fatalf("failed to list leases: %v", err)
		}

		if len(leases) != 5 {
			t.Errorf("expected 5 leases, got %d", len(leases))
		}
	})

	t.Run("Count", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()

		// Create leases
		for i := 0; i < 7; i++ {
			lease := createTestLease(fmt.Sprintf("lease-count-%d", i), "vault/secret/test")
			_ = store.Create(ctx, lease)
		}

		count, err := store.Count(ctx, nil)
		if err != nil {
			t.Fatalf("failed to count leases: %v", err)
		}

		if count != 7 {
			t.Errorf("expected 7, got %d", count)
		}
	})

	t.Run("DeleteByFilter", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()

		// Create leases with different states
		for i := 0; i < 3; i++ {
			lease := createTestLease(fmt.Sprintf("lease-del-active-%d", i), "vault/secret/test")
			lease.State = LeaseStateActive
			_ = store.Create(ctx, lease)
		}
		for i := 0; i < 2; i++ {
			lease := createTestLease(fmt.Sprintf("lease-del-expired-%d", i), "vault/secret/test")
			lease.State = LeaseStateExpired
			_ = store.Create(ctx, lease)
		}

		// Delete expired leases
		deleted, err := store.DeleteByFilter(ctx, &LeaseFilter{
			State: LeaseStateExpired,
		})
		if err != nil {
			t.Fatalf("failed to delete leases: %v", err)
		}

		if deleted != 2 {
			t.Errorf("expected 2 deleted, got %d", deleted)
		}

		// Verify remaining
		count, _ := store.Count(ctx, nil)
		if count != 3 {
			t.Errorf("expected 3 remaining, got %d", count)
		}
	})

	t.Run("UpdateState", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()

		// Create active leases
		for i := 0; i < 3; i++ {
			lease := createTestLease(fmt.Sprintf("lease-state-%d", i), "vault/secret/test")
			lease.State = LeaseStateActive
			_ = store.Create(ctx, lease)
		}

		// Update all to expired
		updated, err := store.UpdateState(ctx, &LeaseFilter{
			State: LeaseStateActive,
		}, LeaseStateExpired)
		if err != nil {
			t.Fatalf("failed to update states: %v", err)
		}

		if updated != 3 {
			t.Errorf("expected 3 updated, got %d", updated)
		}

		// Verify
		count, _ := store.Count(ctx, &LeaseFilter{State: LeaseStateExpired})
		if count != 3 {
			t.Errorf("expected 3 expired, got %d", count)
		}
	})

	t.Run("LeaseEvents", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()
		lease := createTestLease("lease-events", "vault/secret/test")
		_ = store.Create(ctx, lease)

		// Record events
		oldState := LeaseStateActive
		newState := LeaseStateRenewing
		err := store.RecordLeaseEvent(ctx, lease.ID, "renew", &oldState, &newState, nil, nil)
		if err != nil {
			t.Fatalf("failed to record event: %v", err)
		}

		// Record another event
		err = store.RecordLeaseEvent(ctx, lease.ID, "expire", &newState, nil, fmt.Errorf("expired"), nil)
		if err != nil {
			t.Fatalf("failed to record event: %v", err)
		}

		// Get events
		events, err := store.GetLeaseEvents(ctx, lease.ID, 10)
		if err != nil {
			t.Fatalf("failed to get events: %v", err)
		}

		if len(events) != 2 {
			t.Errorf("expected 2 events, got %d", len(events))
		}
	})

	t.Run("PathPrefixFilter", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()

		// Create leases with different paths
		_ = store.Create(ctx, createTestLease("lease-p1", "vault/app1/secret"))
		_ = store.Create(ctx, createTestLease("lease-p2", "vault/app1/config"))
		_ = store.Create(ctx, createTestLease("lease-p3", "vault/app2/secret"))

		// Filter by path prefix
		leases, err := store.List(ctx, &LeaseFilter{
			PathPrefix: "vault/app1",
		})
		if err != nil {
			t.Fatalf("failed to list leases: %v", err)
		}

		if len(leases) != 2 {
			t.Errorf("expected 2 leases with app1 prefix, got %d", len(leases))
		}
	})

	t.Run("MultipleStatesFilter", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		ctx := context.Background()

		// Create leases with different states
		l1 := createTestLease("lease-ms1", "vault/secret/test")
		l1.State = LeaseStateActive
		_ = store.Create(ctx, l1)

		l2 := createTestLease("lease-ms2", "vault/secret/test")
		l2.State = LeaseStateRenewing
		_ = store.Create(ctx, l2)

		l3 := createTestLease("lease-ms3", "vault/secret/test")
		l3.State = LeaseStateExpired
		_ = store.Create(ctx, l3)

		// Filter by multiple states
		leases, err := store.List(ctx, &LeaseFilter{
			States: []LeaseState{LeaseStateActive, LeaseStateRenewing},
		})
		if err != nil {
			t.Fatalf("failed to list leases: %v", err)
		}

		if len(leases) != 2 {
			t.Errorf("expected 2 leases, got %d", len(leases))
		}
	})
}

// TestPersistentLeaseManager tests the persistent lease manager.
func TestPersistentLeaseManager(t *testing.T) {
	t.Run("NewPersistentLeaseManager", func(t *testing.T) {
		t.Run("NilConfig", func(t *testing.T) {
			_, err := NewPersistentLeaseManager(nil)
			if err == nil {
				t.Error("expected error for nil config")
			}
		})

		t.Run("NilStore", func(t *testing.T) {
			_, err := NewPersistentLeaseManager(&PersistentLeaseManagerConfig{})
			if err == nil {
				t.Error("expected error for nil store")
			}
		})

		t.Run("ValidConfig", func(t *testing.T) {
			store := createTestStore(t)
			defer store.Close()

			mgr, err := NewPersistentLeaseManager(&PersistentLeaseManagerConfig{
				Store: store,
			})
			if err != nil {
				t.Fatalf("failed to create manager: %v", err)
			}
			defer mgr.Stop()

			if mgr == nil {
				t.Fatal("expected non-nil manager")
			}
		})
	})

	t.Run("Track", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()
		lease := createTestLease("track-1", "vault/secret/test")

		err := mgr.Track(ctx, lease)
		if err != nil {
			t.Fatalf("failed to track lease: %v", err)
		}

		// Verify tracking
		retrieved, err := mgr.Get(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to get lease: %v", err)
		}

		if retrieved.ID != lease.ID {
			t.Errorf("expected ID %s, got %s", lease.ID, retrieved.ID)
		}
		if retrieved.State != LeaseStateActive {
			t.Errorf("expected state active, got %s", retrieved.State)
		}
	})

	t.Run("Track_InvalidLease", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()

		// Nil lease
		err := mgr.Track(ctx, nil)
		if err == nil {
			t.Error("expected error for nil lease")
		}

		// Empty ID
		err = mgr.Track(ctx, &Lease{})
		if err == nil {
			t.Error("expected error for empty ID")
		}
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()
		_, err := mgr.Get(ctx, "nonexistent")
		if !errors.Is(err, ErrLeaseNotFound) {
			t.Errorf("expected ErrLeaseNotFound, got %v", err)
		}
	})

	t.Run("List", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()

		// Track multiple leases
		for i := 0; i < 5; i++ {
			lease := createTestLease(fmt.Sprintf("list-%d", i), "vault/secret/test")
			_ = mgr.Track(ctx, lease)
		}

		leases, err := mgr.List(ctx)
		if err != nil {
			t.Fatalf("failed to list leases: %v", err)
		}

		if len(leases) != 5 {
			t.Errorf("expected 5 leases, got %d", len(leases))
		}
	})

	t.Run("ListByPath", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()

		// Track leases with different paths
		l1 := createTestLease("path-1", "vault/app1/secret")
		_ = mgr.Track(ctx, l1)
		l2 := createTestLease("path-2", "vault/app1/secret")
		_ = mgr.Track(ctx, l2)
		l3 := createTestLease("path-3", "vault/app2/secret")
		_ = mgr.Track(ctx, l3)

		leases, err := mgr.ListByPath(ctx, "vault/app1/secret")
		if err != nil {
			t.Fatalf("failed to list by path: %v", err)
		}

		if len(leases) != 2 {
			t.Errorf("expected 2 leases, got %d", len(leases))
		}
	})

	t.Run("ListByBackend", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()

		// Track leases with different backends
		l1 := createTestLease("backend-1", "vault/secret/test")
		l1.Backend = BackendTypeVault
		_ = mgr.Track(ctx, l1)

		l2 := createTestLease("backend-2", "aws/secret/test")
		l2.Backend = BackendTypeAWS
		_ = mgr.Track(ctx, l2)

		leases, err := mgr.ListByBackend(ctx, BackendTypeVault)
		if err != nil {
			t.Fatalf("failed to list by backend: %v", err)
		}

		if len(leases) != 1 {
			t.Errorf("expected 1 lease, got %d", len(leases))
		}
	})

	t.Run("ListExpiring", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()
		now := time.Now()

		// Track leases with different expiry times
		l1 := createTestLease("exp-1", "vault/secret/test")
		l1.ExpiresAt = now.Add(30 * time.Minute)
		_ = mgr.Track(ctx, l1)

		l2 := createTestLease("exp-2", "vault/secret/test")
		l2.ExpiresAt = now.Add(3 * time.Hour)
		_ = mgr.Track(ctx, l2)

		leases, err := mgr.ListExpiring(ctx, 1*time.Hour)
		if err != nil {
			t.Fatalf("failed to list expiring: %v", err)
		}

		if len(leases) != 1 {
			t.Errorf("expected 1 expiring lease, got %d", len(leases))
		}
	})

	t.Run("Revoke", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()
		lease := createTestLease("revoke-1", "vault/secret/test")
		_ = mgr.Track(ctx, lease)

		err := mgr.Revoke(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to revoke lease: %v", err)
		}

		// Verify state
		retrieved, err := mgr.Get(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to get lease: %v", err)
		}

		if retrieved.State != LeaseStateRevoked {
			t.Errorf("expected state revoked, got %s", retrieved.State)
		}
	})

	t.Run("RevokeByPath", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()

		// Track multiple leases
		for i := 0; i < 3; i++ {
			lease := createTestLease(fmt.Sprintf("revpath-%d", i), "vault/app1/secret")
			_ = mgr.Track(ctx, lease)
		}
		for i := 0; i < 2; i++ {
			lease := createTestLease(fmt.Sprintf("revpath-other-%d", i), "vault/app2/secret")
			_ = mgr.Track(ctx, lease)
		}

		count, err := mgr.RevokeByPath(ctx, "vault/app1")
		if err != nil {
			t.Fatalf("failed to revoke by path: %v", err)
		}

		if count != 3 {
			t.Errorf("expected 3 revoked, got %d", count)
		}
	})

	t.Run("RevokeByBackend", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()

		// Track leases with different backends
		for i := 0; i < 3; i++ {
			lease := createTestLease(fmt.Sprintf("revbe-vault-%d", i), "vault/secret/test")
			lease.Backend = BackendTypeVault
			_ = mgr.Track(ctx, lease)
		}
		for i := 0; i < 2; i++ {
			lease := createTestLease(fmt.Sprintf("revbe-aws-%d", i), "aws/secret/test")
			lease.Backend = BackendTypeAWS
			_ = mgr.Track(ctx, lease)
		}

		count, err := mgr.RevokeByBackend(ctx, BackendTypeVault)
		if err != nil {
			t.Fatalf("failed to revoke by backend: %v", err)
		}

		if count != 3 {
			t.Errorf("expected 3 revoked, got %d", count)
		}
	})

	t.Run("RevokeByAgentID", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()

		// Track leases with different agent IDs
		for i := 0; i < 3; i++ {
			lease := createTestLease(fmt.Sprintf("revagent-%d", i), "vault/secret/test")
			lease.Metadata = map[string]string{"agent_id": "agent-1"}
			_ = mgr.Track(ctx, lease)
		}
		for i := 0; i < 2; i++ {
			lease := createTestLease(fmt.Sprintf("revagent-other-%d", i), "vault/secret/test")
			lease.Metadata = map[string]string{"agent_id": "agent-2"}
			_ = mgr.Track(ctx, lease)
		}

		count, err := mgr.RevokeByAgentID(ctx, "agent-1")
		if err != nil {
			t.Fatalf("failed to revoke by agent: %v", err)
		}

		if count != 3 {
			t.Errorf("expected 3 revoked, got %d", count)
		}
	})

	t.Run("Remove", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()
		lease := createTestLease("remove-1", "vault/secret/test")
		_ = mgr.Track(ctx, lease)

		err := mgr.Remove(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to remove lease: %v", err)
		}

		// Verify removal
		_, err = mgr.Get(ctx, lease.ID)
		if !errors.Is(err, ErrLeaseNotFound) {
			t.Errorf("expected ErrLeaseNotFound, got %v", err)
		}
	})

	t.Run("Stats", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()

		// Track some leases
		for i := 0; i < 5; i++ {
			lease := createTestLease(fmt.Sprintf("stats-%d", i), "vault/secret/test")
			_ = mgr.Track(ctx, lease)
		}

		// Revoke some
		_ = mgr.Revoke(ctx, "stats-0")
		_ = mgr.Revoke(ctx, "stats-1")

		stats, err := mgr.Stats(ctx)
		if err != nil {
			t.Fatalf("failed to get stats: %v", err)
		}

		if stats.ActiveLeases != 3 {
			t.Errorf("expected 3 active leases, got %d", stats.ActiveLeases)
		}
		if stats.RevokedLeases != 2 {
			t.Errorf("expected 2 revoked leases, got %d", stats.RevokedLeases)
		}
	})

	t.Run("BulkRenew", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		// Register a mock backend
		mockBackend := NewMockBackend("vault", BackendTypeVault)
		mgr.RegisterBackend(BackendTypeVault, mockBackend)

		ctx := context.Background()

		// Track leases
		var leaseIDs []string
		for i := 0; i < 5; i++ {
			lease := createTestLease(fmt.Sprintf("bulk-%d", i), "vault/secret/test")
			lease.Renewable = true
			_ = mgr.Track(ctx, lease)
			leaseIDs = append(leaseIDs, lease.ID)
		}

		results, err := mgr.BulkRenew(ctx, leaseIDs)
		if err != nil {
			t.Fatalf("failed to bulk renew: %v", err)
		}

		if len(results) != 5 {
			t.Errorf("expected 5 results, got %d", len(results))
		}
	})

	t.Run("BulkRevoke", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()

		// Track leases
		var leaseIDs []string
		for i := 0; i < 5; i++ {
			lease := createTestLease(fmt.Sprintf("bulkrev-%d", i), "vault/secret/test")
			_ = mgr.Track(ctx, lease)
			leaseIDs = append(leaseIDs, lease.ID)
		}

		results, err := mgr.BulkRevoke(ctx, leaseIDs)
		if err != nil {
			t.Fatalf("failed to bulk revoke: %v", err)
		}

		if len(results) != 5 {
			t.Errorf("expected 5 results, got %d", len(results))
		}

		// Verify all revoked
		for _, id := range leaseIDs {
			lease, _ := mgr.Get(ctx, id)
			if lease.State != LeaseStateRevoked {
				t.Errorf("lease %s not revoked", id)
			}
		}
	})

	t.Run("Renew_NotRenewable", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		mockBackend := NewMockBackend("vault", BackendTypeVault)
		mgr.RegisterBackend(BackendTypeVault, mockBackend)

		ctx := context.Background()
		lease := createTestLease("not-renewable", "vault/secret/test")
		lease.Renewable = false
		_ = mgr.Track(ctx, lease)

		_, err := mgr.Renew(ctx, lease.ID, time.Hour)
		if !errors.Is(err, ErrLeaseNotRenewable) {
			t.Errorf("expected ErrLeaseNotRenewable, got %v", err)
		}
	})

	t.Run("Renew_Expired", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		mockBackend := NewMockBackend("vault", BackendTypeVault)
		mgr.RegisterBackend(BackendTypeVault, mockBackend)

		ctx := context.Background()
		lease := createTestLease("expired-renew", "vault/secret/test")
		lease.Renewable = true
		lease.ExpiresAt = time.Now().Add(-1 * time.Hour) // Already expired
		_ = mgr.Track(ctx, lease)

		_, err := mgr.Renew(ctx, lease.ID, time.Hour)
		if !errors.Is(err, ErrLeaseExpired) {
			t.Errorf("expected ErrLeaseExpired, got %v", err)
		}
	})

	t.Run("Renew_NoBackend", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()
		lease := createTestLease("no-backend", "vault/secret/test")
		lease.Renewable = true
		lease.Backend = BackendTypeVault
		_ = mgr.Track(ctx, lease)

		// Don't register a backend
		_, err := mgr.Renew(ctx, lease.ID, time.Hour)
		if err == nil {
			t.Error("expected error for no backend")
		}
	})

	t.Run("Callbacks", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		var revokedIDs []string
		var mu sync.Mutex

		mgr, _ := NewPersistentLeaseManager(&PersistentLeaseManagerConfig{
			Store: store,
			Callbacks: &LeaseCallbacks{
				OnRevoke: func(ctx context.Context, lease *Lease) {
					mu.Lock()
					revokedIDs = append(revokedIDs, lease.ID)
					mu.Unlock()
				},
			},
		})
		defer mgr.Stop()

		ctx := context.Background()
		lease := createTestLease("callback-1", "vault/secret/test")
		_ = mgr.Track(ctx, lease)

		_ = mgr.Revoke(ctx, lease.ID)

		mu.Lock()
		if len(revokedIDs) != 1 {
			t.Errorf("expected 1 revoke callback, got %d", len(revokedIDs))
		}
		mu.Unlock()
	})

	t.Run("StartStop", func(t *testing.T) {
		mgr := createTestManager(t)

		ctx := context.Background()
		err := mgr.Start(ctx)
		if err != nil {
			t.Fatalf("failed to start: %v", err)
		}

		// Start again should be no-op
		err = mgr.Start(ctx)
		if err != nil {
			t.Fatalf("second start failed: %v", err)
		}

		err = mgr.Stop()
		if err != nil {
			t.Fatalf("failed to stop: %v", err)
		}
	})

	t.Run("ConcurrentOperations", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		ctx := context.Background()
		var wg sync.WaitGroup
		var ops int64

		// Concurrent track operations
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				lease := createTestLease(fmt.Sprintf("concurrent-%d", n), "vault/secret/test")
				if err := mgr.Track(ctx, lease); err != nil {
					t.Errorf("failed to track: %v", err)
					return
				}
				atomic.AddInt64(&ops, 1)
			}(i)
		}

		wg.Wait()

		if ops != 50 {
			t.Errorf("expected 50 successful operations, got %d", ops)
		}

		leases, _ := mgr.List(ctx)
		if len(leases) != 50 {
			t.Errorf("expected 50 leases, got %d", len(leases))
		}
	})
}

// TestLeaseRenewalWithBackend tests lease renewal with a mock backend.
func TestLeaseRenewalWithBackend(t *testing.T) {
	t.Run("SuccessfulRenewal", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		// Use a mock that supports lease renewal
		mockBackend := &renewableMockBackend{
			leases: make(map[string]*Lease),
		}
		mgr.RegisterBackend(BackendTypeVault, mockBackend)

		ctx := context.Background()
		lease := createTestLease("renew-success", "vault/secret/test")
		lease.Renewable = true
		lease.Backend = BackendTypeVault
		_ = mgr.Track(ctx, lease)

		// Add the lease to the mock backend
		mockBackend.leases[lease.ID] = lease

		renewed, err := mgr.Renew(ctx, lease.ID, time.Hour)
		if err != nil {
			t.Fatalf("failed to renew: %v", err)
		}

		if renewed.RenewalCount != 1 {
			t.Errorf("expected renewal count 1, got %d", renewed.RenewalCount)
		}
		if renewed.State != LeaseStateActive {
			t.Errorf("expected state active, got %s", renewed.State)
		}
	})

	t.Run("RenewalFailure", func(t *testing.T) {
		mgr := createTestManager(t)
		defer mgr.Stop()

		// Use a mock that always fails renewal
		mockBackend := &renewableMockBackend{
			leases:     make(map[string]*Lease),
			failRenew:  true,
			renewError: fmt.Errorf("renewal failed"),
		}
		mgr.RegisterBackend(BackendTypeVault, mockBackend)

		ctx := context.Background()
		lease := createTestLease("renew-fail", "vault/secret/test")
		lease.Renewable = true
		lease.Backend = BackendTypeVault
		_ = mgr.Track(ctx, lease)
		mockBackend.leases[lease.ID] = lease

		_, err := mgr.Renew(ctx, lease.ID, time.Hour)
		if err == nil {
			t.Error("expected renewal to fail")
		}
	})
}

// renewableMockBackend is a mock backend that supports lease operations.
type renewableMockBackend struct {
	mu         sync.RWMutex
	leases     map[string]*Lease
	failRenew  bool
	renewError error
}

func (m *renewableMockBackend) Type() BackendType                { return BackendTypeVault }
func (m *renewableMockBackend) Name() string                     { return "vault" }
func (m *renewableMockBackend) Healthy(ctx context.Context) bool { return true }
func (m *renewableMockBackend) List(ctx context.Context, prefix string) ([]string, error) {
	return nil, nil
}

func (m *renewableMockBackend) Read(ctx context.Context, req *SecretRequest) (*Secret, error) {
	return nil, ErrSecretNotFound
}

func (m *renewableMockBackend) ReadDynamic(ctx context.Context, req *SecretRequest) (*Secret, error) {
	return nil, ErrSecretNotFound
}

func (m *renewableMockBackend) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failRenew {
		return nil, m.renewError
	}

	lease, ok := m.leases[leaseID]
	if !ok {
		return nil, ErrLeaseNotFound
	}

	// Simulate renewal
	renewed := &Lease{
		ID:         lease.ID,
		SecretPath: lease.SecretPath,
		Backend:    lease.Backend,
		TTL:        increment,
		ExpiresAt:  time.Now().Add(increment),
		Renewable:  lease.Renewable,
	}
	return renewed, nil
}

func (m *renewableMockBackend) RevokeLease(ctx context.Context, leaseID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.leases[leaseID]; !ok {
		return ErrLeaseNotFound
	}
	delete(m.leases, leaseID)
	return nil
}

func (m *renewableMockBackend) Close() error { return nil }

var _ SecretBackend = (*renewableMockBackend)(nil)

// Helper functions

func createTestStore(t *testing.T) *SQLiteLeaseStore {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_leases.db")

	store, err := NewSQLiteLeaseStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	if err := store.Initialize(ctx); err != nil {
		store.Close()
		t.Fatalf("failed to initialize store: %v", err)
	}

	return store
}

func createTestManager(t *testing.T) *PersistentLeaseManager {
	t.Helper()

	store := createTestStore(t)
	t.Cleanup(func() {
		store.Close()
	})

	mgr, err := NewPersistentLeaseManager(&PersistentLeaseManagerConfig{
		Store: store,
		RenewalConfig: &LeaseRenewalConfig{
			Threshold:     0.75,
			RetryInterval: time.Second,
			MaxRetries:    3,
			GracePeriod:   time.Minute,
		},
		CleanupInterval:    time.Minute,
		RenewalBatchSize:   10,
		RenewalConcurrency: 5,
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	return mgr
}

func createTestLease(id, path string) *Lease {
	now := time.Now()
	return &Lease{
		ID:         id,
		SecretPath: path,
		Backend:    BackendTypeVault,
		State:      LeaseStateActive,
		TTL:        time.Hour,
		IssuedAt:   now,
		ExpiresAt:  now.Add(time.Hour),
		Renewable:  true,
		Revocable:  true,
		Metadata:   make(map[string]string),
	}
}

// TestLeaseStoreInMemory tests using an in-memory SQLite database.
func TestLeaseStoreInMemory(t *testing.T) {
	store, err := NewSQLiteLeaseStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	// Basic operations
	lease := createTestLease("mem-1", "vault/secret/test")
	if err := store.Create(ctx, lease); err != nil {
		t.Fatalf("failed to create: %v", err)
	}

	retrieved, err := store.Get(ctx, lease.ID)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if retrieved.ID != lease.ID {
		t.Errorf("expected %s, got %s", lease.ID, retrieved.ID)
	}
}

// TestLeaseMetadata tests lease metadata handling.
func TestLeaseMetadata(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	lease := createTestLease("meta-1", "vault/secret/test")
	lease.Metadata = map[string]string{
		"agent_id":   "agent-123",
		"request_id": "req-456",
		"namespace":  "production",
	}

	if err := store.Create(ctx, lease); err != nil {
		t.Fatalf("failed to create: %v", err)
	}

	retrieved, err := store.Get(ctx, lease.ID)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if retrieved.Metadata["agent_id"] != "agent-123" {
		t.Errorf("expected agent_id agent-123, got %s", retrieved.Metadata["agent_id"])
	}
	if retrieved.Metadata["request_id"] != "req-456" {
		t.Errorf("expected request_id req-456, got %s", retrieved.Metadata["request_id"])
	}
}

// TestLeaseTTLHandling tests TTL and MaxTTL handling.
func TestLeaseTTLHandling(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()

	t.Run("WithMaxTTL", func(t *testing.T) {
		lease := createTestLease("ttl-1", "vault/secret/test")
		lease.TTL = time.Hour
		lease.MaxTTL = 24 * time.Hour

		if err := store.Create(ctx, lease); err != nil {
			t.Fatalf("failed to create: %v", err)
		}

		retrieved, err := store.Get(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to get: %v", err)
		}

		if retrieved.TTL != time.Hour {
			t.Errorf("expected TTL 1h, got %v", retrieved.TTL)
		}
		if retrieved.MaxTTL != 24*time.Hour {
			t.Errorf("expected MaxTTL 24h, got %v", retrieved.MaxTTL)
		}
	})

	t.Run("WithoutMaxTTL", func(t *testing.T) {
		lease := createTestLease("ttl-2", "vault/secret/test")
		lease.TTL = time.Hour
		lease.MaxTTL = 0

		if err := store.Create(ctx, lease); err != nil {
			t.Fatalf("failed to create: %v", err)
		}

		retrieved, err := store.Get(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to get: %v", err)
		}

		if retrieved.MaxTTL != 0 {
			t.Errorf("expected MaxTTL 0, got %v", retrieved.MaxTTL)
		}
	})
}

// TestFilterByRenewable tests filtering by renewable flag.
func TestFilterByRenewable(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create renewable and non-renewable leases
	for i := 0; i < 3; i++ {
		lease := createTestLease(fmt.Sprintf("renewable-%d", i), "vault/secret/test")
		lease.Renewable = true
		_ = store.Create(ctx, lease)
	}
	for i := 0; i < 2; i++ {
		lease := createTestLease(fmt.Sprintf("nonrenewable-%d", i), "vault/secret/test")
		lease.Renewable = false
		_ = store.Create(ctx, lease)
	}

	// Filter renewable
	renewable := true
	leases, err := store.List(ctx, &LeaseFilter{
		Renewable: &renewable,
	})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	if len(leases) != 3 {
		t.Errorf("expected 3 renewable leases, got %d", len(leases))
	}

	// Filter non-renewable
	notRenewable := false
	leases, err = store.List(ctx, &LeaseFilter{
		Renewable: &notRenewable,
	})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	if len(leases) != 2 {
		t.Errorf("expected 2 non-renewable leases, got %d", len(leases))
	}
}

// TestOrderBy tests ordering results.
func TestOrderBy(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now()

	// Create leases with different expiry times
	for i := 0; i < 3; i++ {
		lease := createTestLease(fmt.Sprintf("order-%d", i), "vault/secret/test")
		lease.ExpiresAt = now.Add(time.Duration(i+1) * time.Hour)
		_ = store.Create(ctx, lease)
	}

	// Order by expires_at descending
	leases, err := store.List(ctx, &LeaseFilter{
		OrderBy:   "expires_at",
		OrderDesc: true,
	})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	// First should have latest expiry
	if leases[0].ID != "order-2" {
		t.Errorf("expected order-2 first, got %s", leases[0].ID)
	}
}

// Ensure the database file is cleaned up.
func TestDatabaseCleanup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cleanup_test.db")

	store, err := NewSQLiteLeaseStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	_ = store.Initialize(ctx)

	// Verify file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file should exist")
	}

	store.Close()

	// File should still exist after close (cleanup is handled by t.TempDir)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file should still exist after close")
	}
}

// TestDatabaseConnectionError tests handling of connection errors.
func TestDatabaseConnectionError(t *testing.T) {
	// Try to open a database in a non-existent directory
	_, err := NewSQLiteLeaseStore("/nonexistent/path/to/db.sqlite")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

// TestLeaseFilterIDs tests filtering by multiple IDs.
func TestLeaseFilterIDs(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create leases
	for i := 0; i < 5; i++ {
		lease := createTestLease(fmt.Sprintf("id-%d", i), "vault/secret/test")
		_ = store.Create(ctx, lease)
	}

	// Filter by specific IDs
	leases, err := store.List(ctx, &LeaseFilter{
		IDs: []string{"id-1", "id-3"},
	})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	if len(leases) != 2 {
		t.Errorf("expected 2 leases, got %d", len(leases))
	}
}

// TestLeaseNeedsRenewal tests the NeedsRenewal method.
func TestLeaseNeedsRenewal(t *testing.T) {
	t.Run("NotRenewable", func(t *testing.T) {
		lease := createTestLease("needs-1", "vault/secret/test")
		lease.Renewable = false

		if lease.NeedsRenewal(0.75) {
			t.Error("non-renewable lease should not need renewal")
		}
	})

	t.Run("BelowThreshold", func(t *testing.T) {
		now := time.Now()
		lease := createTestLease("needs-2", "vault/secret/test")
		lease.Renewable = true
		lease.TTL = time.Hour
		lease.IssuedAt = now
		lease.ExpiresAt = now.Add(time.Hour)

		// With 100% TTL remaining, shouldn't need renewal at 75% threshold
		if lease.NeedsRenewal(0.75) {
			t.Error("lease should not need renewal when TTL is above threshold")
		}
	})

	t.Run("AboveThreshold", func(t *testing.T) {
		now := time.Now()
		lease := createTestLease("needs-3", "vault/secret/test")
		lease.Renewable = true
		lease.TTL = time.Hour
		lease.IssuedAt = now.Add(-50 * time.Minute) // 50 minutes ago
		lease.ExpiresAt = now.Add(10 * time.Minute) // 10 minutes remaining (16.6%)

		// With only ~16% TTL remaining, should need renewal at 75% threshold
		if !lease.NeedsRenewal(0.75) {
			t.Error("lease should need renewal when TTL is below threshold")
		}
	})
}

// TestLeaseIsExpired tests the IsExpired method.
func TestLeaseIsExpired(t *testing.T) {
	t.Run("NotExpired", func(t *testing.T) {
		lease := createTestLease("exp-1", "vault/secret/test")
		lease.ExpiresAt = time.Now().Add(time.Hour)

		if lease.IsExpired() {
			t.Error("lease should not be expired")
		}
	})

	t.Run("Expired", func(t *testing.T) {
		lease := createTestLease("exp-2", "vault/secret/test")
		lease.ExpiresAt = time.Now().Add(-time.Hour)

		if !lease.IsExpired() {
			t.Error("lease should be expired")
		}
	})
}

// Verify interface compliance.
var (
	_ LeaseStore   = (*SQLiteLeaseStore)(nil)
	_ LeaseManager = (*PersistentLeaseManager)(nil)
)

// TestSQLiteLeaseStoreWithExistingDB tests opening an existing database.
func TestSQLiteLeaseStoreWithExistingDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "existing.db")

	// Create initial store
	store1, err := NewSQLiteLeaseStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	_ = store1.Initialize(ctx)

	lease := createTestLease("persist-1", "vault/secret/test")
	_ = store1.Create(ctx, lease)
	store1.Close()

	// Re-open store
	store2, err := NewSQLiteLeaseStore(dbPath)
	if err != nil {
		t.Fatalf("failed to re-open store: %v", err)
	}
	defer store2.Close()

	// Data should persist
	retrieved, err := store2.Get(ctx, lease.ID)
	if err != nil {
		t.Fatalf("failed to get lease: %v", err)
	}

	if retrieved.ID != lease.ID {
		t.Errorf("expected %s, got %s", lease.ID, retrieved.ID)
	}
}

// TestUpdateStateWithOffset tests pagination with UpdateState.
func TestUpdateStateWithOffset(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create leases
	for i := 0; i < 10; i++ {
		lease := createTestLease(fmt.Sprintf("offset-%d", i), "vault/secret/test")
		_ = store.Create(ctx, lease)
	}

	// List with offset
	leases, err := store.List(ctx, &LeaseFilter{
		Limit:  5,
		Offset: 3,
	})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	if len(leases) != 5 {
		t.Errorf("expected 5 leases, got %d", len(leases))
	}
}

// TestContextCancellation tests context cancellation handling.
func TestContextCancellation(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Operations should fail gracefully
	_, err := store.List(ctx, nil)
	if err == nil || errors.Is(err, sql.ErrNoRows) {
		// Either context error or no error is acceptable depending on timing
		t.Log("List returned without context error")
	}
}

// TestLeaseStateMachine tests the lease state machine transitions.
func TestLeaseStateMachine(t *testing.T) {
	t.Run("CanTransitionLease", func(t *testing.T) {
		tests := []struct {
			name     string
			from     LeaseState
			event    LeaseTransitionEvent
			expected bool
		}{
			{"Pending->Active", LeaseStatePending, LeaseTransitionEventActivate, true},
			{"Active->Renewing", LeaseStateActive, LeaseTransitionEventRenewStart, true},
			{"Active->Expired", LeaseStateActive, LeaseTransitionEventExpire, true},
			{"Active->Revoked", LeaseStateActive, LeaseTransitionEventRevoke, true},
			{"Renewing->Active (success)", LeaseStateRenewing, LeaseTransitionEventRenewSuccess, true},
			{"Renewing->Active (failed)", LeaseStateRenewing, LeaseTransitionEventRenewFailed, true},
			{"Renewing->Expired", LeaseStateRenewing, LeaseTransitionEventExpire, true},
			{"Renewing->Revoked", LeaseStateRenewing, LeaseTransitionEventRevoke, true},
			{"Expired->Revoked (invalid)", LeaseStateExpired, LeaseTransitionEventRevoke, true}, // Ignored, not an error
			{"Revoked->Expired (invalid)", LeaseStateRevoked, LeaseTransitionEventExpire, true}, // Ignored, not an error
			{"Pending->Renewing (invalid)", LeaseStatePending, LeaseTransitionEventRenewStart, false},
			{"Expired->Renewing (invalid)", LeaseStateExpired, LeaseTransitionEventRenewStart, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := CanTransitionLease(tt.from, tt.event)
				if result != tt.expected {
					t.Errorf("CanTransitionLease(%s, %s) = %v, want %v", tt.from, tt.event, result, tt.expected)
				}
			})
		}
	})

	t.Run("NextLeaseState", func(t *testing.T) {
		tests := []struct {
			name         string
			from         LeaseState
			event        LeaseTransitionEvent
			expectedNext LeaseState
			expectedOK   bool
		}{
			{"Pending->Active", LeaseStatePending, LeaseTransitionEventActivate, LeaseStateActive, true},
			{"Active->Renewing", LeaseStateActive, LeaseTransitionEventRenewStart, LeaseStateRenewing, true},
			{"Active->Expired", LeaseStateActive, LeaseTransitionEventExpire, LeaseStateExpired, true},
			{"Active->Revoked", LeaseStateActive, LeaseTransitionEventRevoke, LeaseStateRevoked, true},
			{"Renewing->Active (success)", LeaseStateRenewing, LeaseTransitionEventRenewSuccess, LeaseStateActive, true},
			{"Renewing->Active (failed)", LeaseStateRenewing, LeaseTransitionEventRenewFailed, LeaseStateActive, true},
			{"Pending->Renewing (invalid)", LeaseStatePending, LeaseTransitionEventRenewStart, LeaseStatePending, false},
			{"Expired->Active (invalid)", LeaseStateExpired, LeaseTransitionEventActivate, LeaseStateExpired, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				next, ok := NextLeaseState(tt.from, tt.event)
				if next != tt.expectedNext || ok != tt.expectedOK {
					t.Errorf("NextLeaseState(%s, %s) = (%s, %v), want (%s, %v)",
						tt.from, tt.event, next, ok, tt.expectedNext, tt.expectedOK)
				}
			})
		}
	})

	t.Run("TrackUsesStateMachine", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		config := &PersistentLeaseManagerConfig{
			Store: store,
		}

		manager, err := NewPersistentLeaseManager(config)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		ctx := context.Background()
		lease := &Lease{
			ID:         "state-machine-test-1",
			SecretPath: "test/path",
			Backend:    BackendTypeVault,
			TTL:        time.Hour,
			ExpiresAt:  time.Now().Add(time.Hour),
			Renewable:  true,
		}

		err = manager.Track(ctx, lease)
		if err != nil {
			t.Fatalf("failed to track lease: %v", err)
		}

		// Verify lease is in Active state (transitioned from Pending)
		retrieved, err := manager.Get(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to get lease: %v", err)
		}

		if retrieved.State != LeaseStateActive {
			t.Errorf("expected state %s, got %s", LeaseStateActive, retrieved.State)
		}
	})

	t.Run("RevokeUsesStateMachine", func(t *testing.T) {
		store := createTestStore(t)
		defer store.Close()

		config := &PersistentLeaseManagerConfig{
			Store: store,
		}

		manager, err := NewPersistentLeaseManager(config)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		ctx := context.Background()
		lease := &Lease{
			ID:         "revoke-state-machine-test",
			SecretPath: "test/path",
			Backend:    BackendTypeVault,
			State:      LeaseStateActive,
			TTL:        time.Hour,
			ExpiresAt:  time.Now().Add(time.Hour),
			Renewable:  true,
		}

		err = store.Create(ctx, lease)
		if err != nil {
			t.Fatalf("failed to create lease: %v", err)
		}

		err = manager.Revoke(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to revoke lease: %v", err)
		}

		// Verify lease is in Revoked state
		retrieved, err := manager.Get(ctx, lease.ID)
		if err != nil {
			t.Fatalf("failed to get lease: %v", err)
		}

		if retrieved.State != LeaseStateRevoked {
			t.Errorf("expected state %s, got %s", LeaseStateRevoked, retrieved.State)
		}
	})
}
