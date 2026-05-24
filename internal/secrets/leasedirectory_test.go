// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"fmt"
	"sync"
	"testing"
)

func TestInMemoryLeaseDirectory_RecordLookupForget(t *testing.T) {
	t.Parallel()

	d := NewInMemoryLeaseDirectory()
	if d.Len() != 0 {
		t.Fatalf("fresh directory Len = %d, want 0", d.Len())
	}

	if _, ok := d.Lookup("missing"); ok {
		t.Errorf("Lookup on empty directory returned hit")
	}

	d.Record("lease-1", LeaseRecord{Backend: "vault", Path: "database/creds/app"})
	d.Record("lease-2", LeaseRecord{Backend: "vault", Path: "database/creds/other"})

	if d.Len() != 2 {
		t.Errorf("Len after two records = %d, want 2", d.Len())
	}

	got, ok := d.Lookup("lease-1")
	if !ok {
		t.Fatalf("Lookup(lease-1) returned miss after Record")
	}
	if got.Backend != "vault" || got.Path != "database/creds/app" {
		t.Errorf("Lookup(lease-1) = %#v, want {vault, database/creds/app}", got)
	}

	d.Forget("lease-1")
	if d.Len() != 1 {
		t.Errorf("Len after Forget = %d, want 1", d.Len())
	}
	if _, ok := d.Lookup("lease-1"); ok {
		t.Errorf("Lookup(lease-1) after Forget returned hit")
	}

	// Forget on unknown is a no-op.
	d.Forget("never-existed")
	if d.Len() != 1 {
		t.Errorf("Len after Forget(unknown) = %d, want 1", d.Len())
	}
}

func TestInMemoryLeaseDirectory_OverwriteSameID(t *testing.T) {
	t.Parallel()

	d := NewInMemoryLeaseDirectory()
	d.Record("lease-1", LeaseRecord{Backend: "vault", Path: "p1"})
	d.Record("lease-1", LeaseRecord{Backend: "file", Path: "p2"})

	got, _ := d.Lookup("lease-1")
	if got.Backend != "file" || got.Path != "p2" {
		t.Errorf("overwrite did not replace: got %#v, want {file, p2}", got)
	}
}

func TestInMemoryLeaseDirectory_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	d := NewInMemoryLeaseDirectory()
	const n = 50

	var wg sync.WaitGroup
	wg.Add(n * 3)

	// N writers + N readers + N forgetters all concurrently. Under
	// -race this must stay clean.
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			d.Record(fmt.Sprintf("lease-%d", i), LeaseRecord{Backend: "vault", Path: fmt.Sprintf("p-%d", i)})
		}()
		go func() {
			defer wg.Done()
			_, _ = d.Lookup(fmt.Sprintf("lease-%d", i))
		}()
		go func() {
			defer wg.Done()
			d.Forget(fmt.Sprintf("never-%d", i))
		}()
	}
	wg.Wait()
}
