// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestTokenRenewer_NextWait(t *testing.T) {
	t.Parallel()
	r := &tokenRenewer{earlyFrac: 0.5}
	cases := []struct {
		ttlSec int
		want   time.Duration
	}{
		{0, time.Second}, // clamped to 1s minimum
		{1, time.Second},
		{10, 5 * time.Second}, // 10 * 0.5 = 5
		{100, 50 * time.Second},
	}
	for _, tc := range cases {
		if got := r.nextWait(tc.ttlSec); got != tc.want {
			t.Errorf("nextWait(%d) = %v, want %v", tc.ttlSec, got, tc.want)
		}
	}
}

func TestTokenRenewer_NextWaitFractionOutOfRange(t *testing.T) {
	t.Parallel()
	r := &tokenRenewer{earlyFrac: 0}
	// 0 frac → fall back to DefaultTokenRenewalEarlyFraction (0.5)
	if got := r.nextWait(10); got != 5*time.Second {
		t.Errorf("nextWait with frac=0 = %v, want 5s (default fallback)", got)
	}
	r.earlyFrac = 2.0
	if got := r.nextWait(10); got != 5*time.Second {
		t.Errorf("nextWait with frac=2 = %v, want 5s (default fallback)", got)
	}
}

func TestTokenRenewer_StartSuccessRenews(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	var renewCount int32
	var mu sync.Mutex
	srv.register("PUT", "/v1/auth/token/renew-self", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		renewCount++
		mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"auth": map[string]any{
				"client_token":   "s.renewed",
				"lease_duration": 2, // 2s so the next tick is ~1s away
				"renewable":      true,
			},
		})
	})

	client := newAPIClient(t, srv.addr())
	client.SetToken("s.dev")
	r := newTokenRenewer(client, Config{
		TokenRenewalEarlyFraction: 0.5,
		Logger:                    nil,
		Clock:                     time.Now,
	}.withDefaults())

	tickCh := make(chan struct{}, 5)
	r.OnTick = func(ok bool, _ error) {
		if ok {
			select {
			case tickCh <- struct{}{}:
			default:
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.start(ctx, 2); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = r.stop(stopCtx)
	}()

	// Wait for at least one successful renewal tick.
	select {
	case <-tickCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("no renewal tick in 5s")
	}

	if !r.isHealthy() {
		t.Errorf("renewer not healthy after successful renew")
	}

	mu.Lock()
	count := renewCount
	mu.Unlock()
	if count == 0 {
		t.Errorf("renew handler never called")
	}
}

func TestTokenRenewer_FailureMarksUnhealthy(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("PUT", "/v1/auth/token/renew-self", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]any{"errors": []string{"denied"}})
	})

	client := newAPIClient(t, srv.addr())
	client.SetToken("s.dev")
	r := newTokenRenewer(client, Config{
		TokenRenewalEarlyFraction: 0.5,
	}.withDefaults())

	tickCh := make(chan bool, 1)
	r.OnTick = func(ok bool, _ error) {
		select {
		case tickCh <- ok:
		default:
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.start(ctx, 2); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = r.stop(stopCtx)
	}()

	select {
	case ok := <-tickCh:
		if ok {
			t.Errorf("tick reported success, want failure")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no tick in 5s")
	}

	if r.isHealthy() {
		t.Errorf("renewer should be unhealthy after failed renew")
	}
}

func TestTokenRenewer_Lifecycle(t *testing.T) {
	t.Parallel()
	r := newTokenRenewer(nil, Config{}.withDefaults())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := r.start(ctx, 60); err == nil {
		// Without a real Vault client this will panic on the first
		// renew-self call. The lifecycle test only cares about
		// double-start + stop semantics, so cancel right away.
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 1*time.Second)
		_ = r.stop(stopCtx)
		stopCancel()
	}

	// Double-start should reject.
	r2 := newTokenRenewer(nil, Config{}.withDefaults())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	if err := r2.start(ctx2, 60); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if err := r2.start(ctx2, 60); err == nil {
		t.Errorf("double start = nil err")
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 1*time.Second)
	_ = r2.stop(stopCtx)
	stopCancel()

	// Double-stop is idempotent.
	stopCtx2, stopCancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	if err := r2.stop(stopCtx2); err != nil {
		t.Errorf("double stop: %v", err)
	}
	stopCancel2()
}

func TestErrString_NilSafe(t *testing.T) {
	t.Parallel()
	if got := errString(nil); got != "" {
		t.Errorf("errString(nil) = %q", got)
	}
}
