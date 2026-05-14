package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestBackendCapability_StringRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cap  BackendCapability
		text string
	}{
		{CapKV, "kv"},
		{CapList, "list"},
		{CapDynamic, "dynamic"},
		{CapLeaseRenew, "lease_renew"},
		{CapLeaseRevoke, "lease_revoke"},
		{CapTransit, "transit"},
	}
	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			t.Parallel()
			if got := tc.cap.String(); got != tc.text {
				t.Errorf("String() = %q, want %q", got, tc.text)
			}
			parsed, err := ParseBackendCapability(tc.text)
			if err != nil {
				t.Fatalf("ParseBackendCapability(%q): %v", tc.text, err)
			}
			if parsed != tc.cap {
				t.Errorf("ParseBackendCapability(%q) = %v, want %v", tc.text, parsed, tc.cap)
			}
		})
	}
}

func TestBackendCapability_ParseUnknown(t *testing.T) {
	t.Parallel()
	if _, err := ParseBackendCapability("bogus"); !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
	if CapKVUnknown.String() != "unknown" {
		t.Errorf("CapKVUnknown.String() = %q, want %q", CapKVUnknown.String(), "unknown")
	}
}

func TestBackendCapability_ParseAliases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want BackendCapability
	}{
		{"lease-renew", CapLeaseRenew},
		{"lease-revoke", CapLeaseRevoke},
		{"  KV  ", CapKV},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParseBackendCapability(tc.in)
			if err != nil {
				t.Fatalf("ParseBackendCapability(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseBackendCapability(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestHasCapability(t *testing.T) {
	t.Parallel()
	set := []BackendCapability{CapKV, CapList, CapDynamic}
	if !HasCapability(set, CapKV) {
		t.Errorf("HasCapability(CapKV) = false, want true")
	}
	if HasCapability(set, CapTransit) {
		t.Errorf("HasCapability(CapTransit) = true, want false")
	}
	if HasCapability(nil, CapKV) {
		t.Errorf("HasCapability on nil set returned true")
	}
}

func TestBackendCapability_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	// BackendCapability doesn't implement Marshaler — the enum
	// stringification is for log + audit lines, not on-the-wire
	// serialisation. This test pins that contract so a future change
	// is intentional.
	b, err := json.Marshal(CapTransit)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != "6" {
		t.Errorf("BackendCapability marshals as numeric (%s); update tests if the contract changes", b)
	}
}

// noopBackend pins the [SecretBackend] interface shape and lets task 3
// (the broker) onboard a fake without rewriting the interface. Every
// method returns [ErrNotImplementedYet] so the broker's "route to a
// backend that can't serve this op" path has something concrete to
// test against.
type noopBackend struct{ name string }

func (b *noopBackend) Name() string                       { return b.name }
func (b *noopBackend) Capabilities() []BackendCapability  { return nil }
func (b *noopBackend) Start(_ context.Context) error      { return nil }
func (b *noopBackend) Stop(_ context.Context) error       { return nil }
func (b *noopBackend) Health(_ context.Context) error     { return ErrBackendNotStarted }
func (b *noopBackend) GetSecret(_ context.Context, _ GetSecretRequest) (*Secret, error) {
	return nil, ErrNotImplementedYet
}
func (b *noopBackend) WriteSecret(_ context.Context, _ WriteSecretRequest) (*Secret, error) {
	return nil, ErrNotImplementedYet
}
func (b *noopBackend) ListSecrets(_ context.Context, _ ListSecretsRequest) (*ListSecretsResponse, error) {
	return nil, ErrNotImplementedYet
}
func (b *noopBackend) DeleteSecret(_ context.Context, _ DeleteSecretRequest) error {
	return ErrNotImplementedYet
}
func (b *noopBackend) IssueDynamicSecret(_ context.Context, _ IssueDynamicSecretRequest) (*Secret, error) {
	return nil, ErrNotImplementedYet
}
func (b *noopBackend) RenewLease(_ context.Context, _ RenewLeaseRequest) (*LeaseInfo, error) {
	return nil, ErrNotImplementedYet
}
func (b *noopBackend) RevokeLease(_ context.Context, _ RevokeLeaseRequest) error {
	return ErrNotImplementedYet
}

func TestSecretBackend_NoopConformsToInterface(t *testing.T) {
	t.Parallel()

	var b SecretBackend = &noopBackend{name: "noop"}

	if b.Name() != "noop" {
		t.Errorf("Name() = %q, want %q", b.Name(), "noop")
	}
	if caps := b.Capabilities(); caps != nil {
		t.Errorf("Capabilities() = %v, want nil", caps)
	}

	ctx := context.Background()
	if err := b.Start(ctx); err != nil {
		t.Errorf("Start: %v", err)
	}
	if err := b.Health(ctx); !errors.Is(err, ErrBackendNotStarted) {
		t.Errorf("Health err = %v, want wraps ErrBackendNotStarted", err)
	}

	if _, err := b.GetSecret(ctx, GetSecretRequest{Path: "x"}); !errors.Is(err, ErrNotImplementedYet) {
		t.Errorf("GetSecret err = %v, want wraps ErrNotImplementedYet", err)
	}
	if _, err := b.WriteSecret(ctx, WriteSecretRequest{Path: "x"}); !errors.Is(err, ErrNotImplementedYet) {
		t.Errorf("WriteSecret err = %v, want wraps ErrNotImplementedYet", err)
	}
	if _, err := b.ListSecrets(ctx, ListSecretsRequest{}); !errors.Is(err, ErrNotImplementedYet) {
		t.Errorf("ListSecrets err = %v, want wraps ErrNotImplementedYet", err)
	}
	if err := b.DeleteSecret(ctx, DeleteSecretRequest{Path: "x"}); !errors.Is(err, ErrNotImplementedYet) {
		t.Errorf("DeleteSecret err = %v, want wraps ErrNotImplementedYet", err)
	}
	if _, err := b.IssueDynamicSecret(ctx, IssueDynamicSecretRequest{}); !errors.Is(err, ErrNotImplementedYet) {
		t.Errorf("IssueDynamicSecret err = %v, want wraps ErrNotImplementedYet", err)
	}
	if _, err := b.RenewLease(ctx, RenewLeaseRequest{LeaseID: "x"}); !errors.Is(err, ErrNotImplementedYet) {
		t.Errorf("RenewLease err = %v, want wraps ErrNotImplementedYet", err)
	}
	if err := b.RevokeLease(ctx, RevokeLeaseRequest{LeaseID: "x"}); !errors.Is(err, ErrNotImplementedYet) {
		t.Errorf("RevokeLease err = %v, want wraps ErrNotImplementedYet", err)
	}

	if err := b.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
}
