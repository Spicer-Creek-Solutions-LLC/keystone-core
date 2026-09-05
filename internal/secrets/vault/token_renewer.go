// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// ErrRenewerNotRunning is the sentinel a renewer method returns when
// called before [tokenRenewer.start] or after [tokenRenewer.stop].
var errRenewerNotRunning = errors.New("vault: token renewer not running")

// tokenRenewer keeps the Vault auth token alive by re-renewing
// shortly before expiry. Shape mirrors Epic 09's `CARotator` — a
// single goroutine + a `sync.Once`-guarded Stop. The renewer is
// passive (no re-login on failure); a permanent failure flips
// [tokenRenewer.healthy] to false so the backend's `Health` reports
// unhealthy and operators can intervene.
//
// v0.x ROADMAP entry "Vault auto re-authentication on token expiry"
// tracks the self-healing variant.
type tokenRenewer struct {
	client    *vaultapi.Client
	earlyFrac float64
	logger    *slog.Logger
	clock     func() time.Time

	// OnTick is a test hook fired after each renewal attempt
	// (regardless of outcome).
	OnTick func(ok bool, err error)

	mu      sync.Mutex
	started bool
	stopped bool
	healthy bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func newTokenRenewer(client *vaultapi.Client, cfg Config) *tokenRenewer {
	return &tokenRenewer{
		client:    client,
		earlyFrac: cfg.TokenRenewalEarlyFraction,
		logger:    cfg.Logger,
		clock:     cfg.Clock,
		healthy:   true,
	}
}

// start spawns the renewal goroutine. ttlSec is the auth token's
// initial TTL in seconds; the loop computes the next renewal cadence
// from it. One-shot — second call returns an error.
func (r *tokenRenewer) start(ctx context.Context, ttlSec int) error {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return fmt.Errorf("%w: cannot start after stop", errRenewerNotRunning)
	}
	if r.started {
		r.mu.Unlock()
		return fmt.Errorf("%w: already started", errRenewerNotRunning)
	}
	r.started = true
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.mu.Unlock()

	go r.run(ctx, ttlSec)
	return nil
}

// stop cancels the renewer and waits for the goroutine to exit.
// Idempotent.
func (r *tokenRenewer) stop(ctx context.Context) error {
	r.mu.Lock()
	if r.stopped || !r.started {
		r.stopped = true
		r.mu.Unlock()
		return nil
	}
	r.stopped = true
	stopCh := r.stopCh
	doneCh := r.doneCh
	r.mu.Unlock()

	close(stopCh)
	select {
	case <-doneCh:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: stop deadline: %v", secrets.ErrInvalidBackend, ctx.Err())
	}
}

// isHealthy reports whether the most recent renewal succeeded.
// Surfaced via [Backend.Health].
func (r *tokenRenewer) isHealthy() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.healthy
}

func (r *tokenRenewer) markHealth(ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.healthy = ok
}

// run is the renewer loop. Computes the next tick from the most
// recently observed TTL and the configured early-fraction; sleeps;
// renews; repeats. Exits on stopCh or ctx cancel.
func (r *tokenRenewer) run(ctx context.Context, ttlSec int) {
	defer close(r.doneCh)
	nextTTL := ttlSec
	for {
		wait := r.nextWait(nextTTL)
		timer := time.NewTimer(wait)

		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-r.stopCh:
			timer.Stop()
			return
		case <-timer.C:
		}

		ok, ttl, err := r.renewOnce(ctx)
		r.markHealth(ok)
		if r.OnTick != nil {
			r.OnTick(ok, err)
		}
		if ok && ttl > 0 {
			nextTTL = ttl
		} else if !ok {
			r.logger.LogAttrs(ctx, slog.LevelWarn, "vault: token renewal failed",
				slog.String("err", errString(err)),
			)
			// Keep the loop alive but pace future attempts on a
			// modest fixed cadence so a misbehaving Vault doesn't
			// get hammered.
			nextTTL = int((30 * time.Second).Seconds())
		}
	}
}

// nextWait returns how long to sleep before the next renewal attempt
// given the latest observed TTL. The formula is
// `ttlSec * earlyFrac` clamped to [1s, ttlSec - 1s] so we never
// sleep zero (busy loop) or past expiry (token dies before renewal).
func (r *tokenRenewer) nextWait(ttlSec int) time.Duration {
	if ttlSec <= 1 {
		return time.Second
	}
	frac := r.earlyFrac
	if frac <= 0 || frac >= 1 {
		frac = DefaultTokenRenewalEarlyFraction
	}
	target := float64(ttlSec) * frac
	if target < 1 {
		target = 1
	}
	maxWait := float64(ttlSec) - 1
	if target > maxWait {
		target = maxWait
	}
	return time.Duration(target * float64(time.Second))
}

// renewOnce performs one renew-self call. Returns (success, newTTLSec, err).
func (r *tokenRenewer) renewOnce(ctx context.Context) (bool, int, error) {
	secret, err := r.client.Auth().Token().RenewSelfWithContext(ctx, 0)
	if err != nil {
		return false, 0, err
	}
	if secret == nil || secret.Auth == nil {
		return false, 0, errors.New("renew-self returned no auth")
	}
	return true, secret.Auth.LeaseDuration, nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
