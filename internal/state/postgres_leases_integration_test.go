// SPDX-License-Identifier: Apache-2.0

//go:build integration

package state

import (
	"errors"
	"testing"
	"time"
)

func TestPg_LeaseCRUD(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	in := sampleLease("pg-l-1", "vault", "database/creds/app")
	if err := s.CreateLease(ctx, in); err != nil {
		t.Fatalf("CreateLease: %v", err)
	}

	got, err := s.GetLease(ctx, "pg-l-1")
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if got.Backend != "vault" || got.SecretPath != "database/creds/app" {
		t.Errorf("round-trip mismatch: %#v", got)
	}
	if got.Metadata["role"] != "app" {
		t.Errorf("metadata lost: %#v", got.Metadata)
	}
}

func TestPg_LeaseDuplicate(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	in := sampleLease("pg-dup", "vault", "p")
	if err := s.CreateLease(ctx, in); err != nil {
		t.Fatalf("first CreateLease: %v", err)
	}
	if err := s.CreateLease(ctx, in); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate err = %v, want ErrDuplicate", err)
	}
}

func TestPg_LeaseList_Filters(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	a := sampleLease("pg-a", "vault", "database/creds/app")
	b := sampleLease("pg-b", "file", "kv/x")
	b.State = "expired"
	for _, l := range []*LeaseStoreRecord{a, b} {
		if err := s.CreateLease(ctx, l); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	got, _ := s.ListLeases(ctx, LeaseFilter{Backend: "vault"})
	if len(got) != 1 || got[0].ID != "pg-a" {
		t.Errorf("backend filter: %#v", got)
	}

	got, _ = s.ListLeases(ctx, LeaseFilter{State: "expired"})
	if len(got) != 1 || got[0].ID != "pg-b" {
		t.Errorf("state filter: %#v", got)
	}
}

func TestPg_LeaseDeleteExpired(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	now := time.Now().UTC().Truncate(time.Second)

	expired := sampleLease("pg-expired", "vault", "p")
	expired.ExpiresAt = now.Add(-time.Hour)
	expired.State = "expired"

	if err := s.CreateLease(ctx, expired); err != nil {
		t.Fatalf("seed: %v", err)
	}

	n, err := s.DeleteExpiredLeases(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpiredLeases: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}
}
