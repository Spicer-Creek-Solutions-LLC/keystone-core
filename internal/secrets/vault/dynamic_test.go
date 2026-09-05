// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

func TestIssueDynamicSecret_DatabaseEngine(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, nil)

	srv.register("GET", "/v1/database/creds/app", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"lease_id":       "database/creds/app/lease-abc",
			"lease_duration": 3600,
			"renewable":      true,
			"data": map[string]any{
				"username": "v-app-1",
				"password": "tmp-pw",
			},
		})
	})

	out, err := b.IssueDynamicSecret(context.Background(), secrets.IssueDynamicSecretRequest{
		Path: "database/creds/app",
	})
	if err != nil {
		t.Fatalf("IssueDynamicSecret: %v", err)
	}
	if out.LeaseID != "database/creds/app/lease-abc" {
		t.Errorf("LeaseID = %q, want database/creds/app/lease-abc", out.LeaseID)
	}
	if out.LeaseDuration.Seconds() != 3600 {
		t.Errorf("LeaseDuration = %v, want 3600s", out.LeaseDuration)
	}
	if !out.Renewable {
		t.Errorf("Renewable = false, want true")
	}
	if out.Data["username"] != "v-app-1" {
		t.Errorf("Data lost: %#v", out.Data)
	}
}

func TestIssueDynamicSecret_PKIEngineWithParams(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, nil)

	var requestBody map[string]any
	srv.register("PUT", "/v1/pki/issue/server", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &requestBody)
		writeJSON(w, http.StatusOK, map[string]any{
			"lease_id":       "pki/issue/server/cert-1",
			"lease_duration": 86400,
			"renewable":      false,
			"data": map[string]any{
				"certificate":   "-----BEGIN CERTIFICATE-----\n...",
				"private_key":   "-----BEGIN PRIVATE KEY-----\n...",
				"serial_number": "ab:cd",
			},
		})
	})

	_, err := b.IssueDynamicSecret(context.Background(), secrets.IssueDynamicSecretRequest{
		Path: "pki/issue/server",
		Params: map[string]any{
			"common_name": "host.example.com",
			"alt_names":   "alt.example.com",
		},
	})
	if err != nil {
		t.Fatalf("IssueDynamicSecret: %v", err)
	}
	if requestBody["common_name"] != "host.example.com" {
		t.Errorf("common_name not propagated: %#v", requestBody)
	}
	if requestBody["alt_names"] != "alt.example.com" {
		t.Errorf("alt_names not propagated: %#v", requestBody)
	}
}

func TestIssueDynamicSecret_PathRequired(t *testing.T) {
	t.Parallel()
	b, _ := newKVFixture(t, nil)
	_, err := b.IssueDynamicSecret(context.Background(), secrets.IssueDynamicSecretRequest{})
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("err = %v, want ErrInvalidBackend", err)
	}
}

func TestRenewLease_HappyPath(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, nil)

	srv.register("PUT", "/v1/sys/leases/renew", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"lease_id":       "database/creds/app/abc",
			"lease_duration": 1800,
			"renewable":      true,
		})
	})

	info, err := b.RenewLease(context.Background(), secrets.RenewLeaseRequest{LeaseID: "database/creds/app/abc"})
	if err != nil {
		t.Fatalf("RenewLease: %v", err)
	}
	if info.ID != "database/creds/app/abc" {
		t.Errorf("ID = %q", info.ID)
	}
	if info.Duration.Seconds() != 1800 {
		t.Errorf("Duration = %v, want 1800s", info.Duration)
	}
}

func TestRenewLease_LeaseExpired(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, nil)

	srv.register("PUT", "/v1/sys/leases/renew", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"errors": []string{"lease has expired"},
		})
	})

	_, err := b.RenewLease(context.Background(), secrets.RenewLeaseRequest{LeaseID: "expired-lease"})
	if !errors.Is(err, secrets.ErrLeaseExpired) {
		t.Errorf("err = %v, want ErrLeaseExpired", err)
	}
}

func TestRenewLease_NotRenewable(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, nil)

	srv.register("PUT", "/v1/sys/leases/renew", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"errors": []string{"lease is not renewable"},
		})
	})

	_, err := b.RenewLease(context.Background(), secrets.RenewLeaseRequest{LeaseID: "x"})
	if !errors.Is(err, secrets.ErrLeaseNotRenewable) {
		t.Errorf("err = %v, want ErrLeaseNotRenewable", err)
	}
}

func TestRevokeLease_HappyPath(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, nil)

	revoked := false
	srv.register("PUT", "/v1/sys/leases/revoke", func(w http.ResponseWriter, _ *http.Request) {
		revoked = true
		writeJSON(w, http.StatusNoContent, nil)
	})

	if err := b.RevokeLease(context.Background(), secrets.RevokeLeaseRequest{LeaseID: "x"}); err != nil {
		t.Fatalf("RevokeLease: %v", err)
	}
	if !revoked {
		t.Errorf("revoke handler never called")
	}
}

func TestRevokeLease_IdempotentOnMissingLease(t *testing.T) {
	t.Parallel()
	b, srv := newKVFixture(t, nil)

	srv.register("PUT", "/v1/sys/leases/revoke", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"errors": []string{"lease not found"},
		})
	})

	if err := b.RevokeLease(context.Background(), secrets.RevokeLeaseRequest{LeaseID: "ghost"}); err != nil {
		t.Errorf("RevokeLease idempotent expectation: err = %v, want nil", err)
	}
}

func TestRevokeLease_PathRequired(t *testing.T) {
	t.Parallel()
	b, _ := newKVFixture(t, nil)
	err := b.RevokeLease(context.Background(), secrets.RevokeLeaseRequest{})
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("err = %v, want ErrInvalidBackend", err)
	}
	if !strings.Contains(err.Error(), "LeaseID is required") {
		t.Errorf("err = %q, want LeaseID-required message", err.Error())
	}
}

func TestFormatVaultDuration(t *testing.T) {
	t.Parallel()
	// 1h + 30m = 5400s
	if got := formatVaultDuration(5400_000_000_000); got != "5400s" {
		t.Errorf("formatVaultDuration = %q, want 5400s", got)
	}
}
