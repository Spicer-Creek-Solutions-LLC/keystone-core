// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"errors"
	"testing"
	"time"
)

func sampleLease(id, backend, path string) *LeaseStoreRecord {
	now := time.Now().UTC().Truncate(time.Second)
	return &LeaseStoreRecord{
		ID:         id,
		Backend:    backend,
		SecretPath: path,
		IssuedAt:   now,
		ExpiresAt:  now.Add(time.Hour),
		Duration:   time.Hour,
		Renewable:  true,
		State:      "active",
		Strategy:   "lazy",
		IssuedFor:  "spiffe://kscore.local/agent/" + id,
		Metadata:   map[string]string{"role": "app", "env": "trial"},
	}
}

func TestSQLiteStore_CreateLease_RoundTrip(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	in := sampleLease("l-1", "vault", "database/creds/app")
	if err := s.CreateLease(ctx, in); err != nil {
		t.Fatalf("CreateLease: %v", err)
	}
	got, err := s.GetLease(ctx, "l-1")
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if got.Backend != "vault" || got.SecretPath != "database/creds/app" {
		t.Errorf("round-trip mismatch: %#v", got)
	}
	if !got.Renewable {
		t.Errorf("Renewable lost: %#v", got)
	}
	if got.Metadata["role"] != "app" || got.Metadata["env"] != "trial" {
		t.Errorf("metadata lost: %#v", got.Metadata)
	}
	if got.IssuedAt.Unix() != in.IssuedAt.Unix() {
		t.Errorf("IssuedAt mismatch: got %v want %v", got.IssuedAt, in.IssuedAt)
	}
}

func TestSQLiteStore_CreateLease_Duplicate(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	in := sampleLease("dup", "vault", "p")
	if err := s.CreateLease(ctx, in); err != nil {
		t.Fatalf("first CreateLease: %v", err)
	}
	err := s.CreateLease(ctx, in)
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("second CreateLease err = %v, want ErrDuplicate", err)
	}
}

func TestSQLiteStore_CreateLease_Validation(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		mutate  func(*LeaseStoreRecord)
		wantSub string
	}{
		{"nil", nil, "nil record"},
		{"empty id", func(r *LeaseStoreRecord) { r.ID = "" }, "ID is required"},
		{"empty backend", func(r *LeaseStoreRecord) { r.Backend = "" }, "Backend is required"},
		{"empty path", func(r *LeaseStoreRecord) { r.SecretPath = "" }, "SecretPath is required"},
		{"zero issued", func(r *LeaseStoreRecord) { r.IssuedAt = time.Time{} }, "IssuedAt is required"},
		{"zero expires", func(r *LeaseStoreRecord) { r.ExpiresAt = time.Time{} }, "ExpiresAt is required"},
		{"empty state", func(r *LeaseStoreRecord) { r.State = "" }, "State is required"},
		{"empty strategy", func(r *LeaseStoreRecord) { r.Strategy = "" }, "Strategy is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec *LeaseStoreRecord
			if tc.mutate != nil {
				rec = sampleLease("v", "vault", "p")
				tc.mutate(rec)
			}
			err := s.CreateLease(ctx, rec)
			if err == nil {
				t.Fatalf("CreateLease = nil err, want validation rejection")
			}
		})
	}
}

func TestSQLiteStore_GetLease_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	if _, err := s.GetLease(context.Background(), "ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_UpdateLease(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	in := sampleLease("u-1", "vault", "p")
	if err := s.CreateLease(ctx, in); err != nil {
		t.Fatalf("CreateLease: %v", err)
	}

	in.RenewCount = 3
	in.LastRenewedAt = time.Now().UTC().Truncate(time.Second)
	in.ExpiresAt = in.ExpiresAt.Add(time.Hour)
	in.Duration = 2 * time.Hour
	in.State = "active"

	if err := s.UpdateLease(ctx, in); err != nil {
		t.Fatalf("UpdateLease: %v", err)
	}
	got, _ := s.GetLease(ctx, "u-1")
	if got.RenewCount != 3 {
		t.Errorf("RenewCount = %d, want 3", got.RenewCount)
	}
	if got.LastRenewedAt.Unix() != in.LastRenewedAt.Unix() {
		t.Errorf("LastRenewedAt not persisted: got %v want %v", got.LastRenewedAt, in.LastRenewedAt)
	}
	if got.Duration != 2*time.Hour {
		t.Errorf("Duration = %v, want 2h", got.Duration)
	}
}

func TestSQLiteStore_UpdateLease_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	in := sampleLease("ghost", "vault", "p")
	if err := s.UpdateLease(context.Background(), in); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_ListLeases_Filters(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	leases := []*LeaseStoreRecord{
		sampleLease("a", "vault", "database/creds/app"),
		sampleLease("b", "vault", "database/creds/web"),
		sampleLease("c", "file", "kv/static"),
		sampleLease("d", "vault", "pki/issue/server"),
	}
	leases[2].State = "expired"
	leases[3].State = "revoked"
	revoked := time.Now().UTC().Truncate(time.Second)
	leases[3].RevokedAt = revoked

	for _, l := range leases {
		if err := s.CreateLease(ctx, l); err != nil {
			t.Fatalf("seed CreateLease(%s): %v", l.ID, err)
		}
	}

	t.Run("by backend", func(t *testing.T) {
		got, err := s.ListLeases(ctx, LeaseFilter{Backend: "vault"})
		if err != nil {
			t.Fatalf("ListLeases: %v", err)
		}
		// vault: a, b, d (d is revoked → excluded since IncludeRevoked=false)
		if len(got) != 2 {
			t.Errorf("vault backend (no revoked) entries = %d, want 2", len(got))
		}
	})

	t.Run("by backend including revoked", func(t *testing.T) {
		got, _ := s.ListLeases(ctx, LeaseFilter{Backend: "vault", IncludeRevoked: true})
		if len(got) != 3 {
			t.Errorf("vault backend with revoked = %d, want 3", len(got))
		}
	})

	t.Run("by state", func(t *testing.T) {
		got, _ := s.ListLeases(ctx, LeaseFilter{State: "expired"})
		if len(got) != 1 {
			t.Errorf("expired entries = %d, want 1", len(got))
		}
	})

	t.Run("by path prefix", func(t *testing.T) {
		got, _ := s.ListLeases(ctx, LeaseFilter{PathPrefix: "database/"})
		if len(got) != 2 {
			t.Errorf("database/ prefix entries = %d, want 2", len(got))
		}
	})

	t.Run("sort column allowlist", func(t *testing.T) {
		_, err := s.ListLeases(ctx, LeaseFilter{SortColumn: "drop_table"})
		if err == nil {
			t.Errorf("disallowed sort column accepted")
		}
	})
}

func TestSQLiteStore_DeleteLease(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	in := sampleLease("d-1", "vault", "p")
	_ = s.CreateLease(ctx, in)

	if err := s.DeleteLease(ctx, "d-1"); err != nil {
		t.Fatalf("DeleteLease: %v", err)
	}
	if _, err := s.GetLease(ctx, "d-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after Delete, Get err = %v, want ErrNotFound", err)
	}

	// Delete-on-missing returns ErrNotFound.
	if err := s.DeleteLease(ctx, "d-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete err = %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_DeleteExpiredLeases(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	// Active + still-time-left → keep.
	active := sampleLease("active", "vault", "p")
	active.ExpiresAt = now.Add(time.Hour)

	// Active but past expiry → KEEP (the scheduler may be mid-renewal).
	stuck := sampleLease("stuck", "vault", "p2")
	stuck.ExpiresAt = now.Add(-time.Hour)
	stuck.State = "active"

	// Expired + past expiry → drop.
	expired := sampleLease("expired", "vault", "p3")
	expired.ExpiresAt = now.Add(-time.Hour)
	expired.State = "expired"

	// Revoked + past expiry → drop.
	revoked := sampleLease("revoked", "vault", "p4")
	revoked.ExpiresAt = now.Add(-time.Hour)
	revoked.State = "revoked"
	revoked.RevokedAt = now.Add(-30 * time.Minute)

	for _, l := range []*LeaseStoreRecord{active, stuck, expired, revoked} {
		if err := s.CreateLease(ctx, l); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	n, err := s.DeleteExpiredLeases(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpiredLeases: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted = %d, want 2 (expired + revoked, not stuck or active)", n)
	}

	// stuck-active row must survive.
	if _, err := s.GetLease(ctx, "stuck"); err != nil {
		t.Errorf("stuck active lease was removed: %v", err)
	}
	if _, err := s.GetLease(ctx, "active"); err != nil {
		t.Errorf("future-expiry active lease was removed: %v", err)
	}
}
