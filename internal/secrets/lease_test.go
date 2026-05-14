package secrets

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestLeaseState_StringRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state LeaseState
		text  string
	}{
		{LeaseStatePending, "pending"},
		{LeaseStateActive, "active"},
		{LeaseStateRenewing, "renewing"},
		{LeaseStateExpired, "expired"},
		{LeaseStateRevoked, "revoked"},
	}

	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			t.Parallel()
			if got := tc.state.String(); got != tc.text {
				t.Errorf("String() = %q, want %q", got, tc.text)
			}
			parsed, err := ParseLeaseState(tc.text)
			if err != nil {
				t.Fatalf("ParseLeaseState(%q) returned err: %v", tc.text, err)
			}
			if parsed != tc.state {
				t.Errorf("ParseLeaseState(%q) = %v, want %v", tc.text, parsed, tc.state)
			}
		})
	}
}

func TestLeaseState_Unknown(t *testing.T) {
	t.Parallel()
	if got := LeaseStateUnknown.String(); got != "unknown" {
		t.Errorf("LeaseStateUnknown.String() = %q, want %q", got, "unknown")
	}
	if _, err := ParseLeaseState("bogus"); !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("ParseLeaseState(bogus) err does not wrap ErrInvalidBackend: %v", err)
	}
}

func TestLeaseState_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	in := LeaseStateActive
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != `"active"` {
		t.Errorf("Marshal = %s, want %q", b, "active")
	}

	var out LeaseState
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("round trip lost value: got %v want %v", out, in)
	}

	// Empty text decodes to Unknown.
	var empty LeaseState
	if err := empty.UnmarshalText(nil); err != nil {
		t.Fatalf("UnmarshalText(nil): %v", err)
	}
	if empty != LeaseStateUnknown {
		t.Errorf("empty text -> %v, want LeaseStateUnknown", empty)
	}
}

func TestRenewStrategy_Threshold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		strategy RenewStrategy
		want     float64
	}{
		{RenewStrategyEager, 0.5},
		{RenewStrategyLazy, 0.9},
		{RenewStrategyOnDemand, 0},
		{RenewStrategyUnknown, 0},
	}
	for _, tc := range tests {
		t.Run(tc.strategy.String(), func(t *testing.T) {
			t.Parallel()
			if got := tc.strategy.Threshold(); got != tc.want {
				t.Errorf("Threshold() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRenewStrategy_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	for _, s := range []RenewStrategy{RenewStrategyEager, RenewStrategyLazy, RenewStrategyOnDemand} {
		b, err := s.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%v): %v", s, err)
		}
		var out RenewStrategy
		if err := out.UnmarshalText(b); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", b, err)
		}
		if out != s {
			t.Errorf("round-trip lost value: got %v want %v", out, s)
		}
	}

	// Empty bytes decode to Unknown.
	var s RenewStrategy
	if err := s.UnmarshalText(nil); err != nil {
		t.Errorf("UnmarshalText(nil): %v", err)
	}
	if s != RenewStrategyUnknown {
		t.Errorf("empty text -> %v, want RenewStrategyUnknown", s)
	}
}

func TestRenewStrategy_ParseAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want RenewStrategy
	}{
		{"eager", RenewStrategyEager},
		{"LAZY", RenewStrategyLazy},
		{"on_demand", RenewStrategyOnDemand},
		{"on-demand", RenewStrategyOnDemand},
		{"ondemand", RenewStrategyOnDemand},
		{"  eager  ", RenewStrategyEager},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParseRenewStrategy(tc.in)
			if err != nil {
				t.Fatalf("ParseRenewStrategy(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseRenewStrategy(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}

	if _, err := ParseRenewStrategy("nope"); !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("ParseRenewStrategy(nope) err does not wrap ErrInvalidBackend: %v", err)
	}
}

func TestLeaseInfo_Expired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		info LeaseInfo
		now  time.Time
		want bool
	}{
		{
			name: "before expiry",
			info: LeaseInfo{ExpiresAt: now.Add(time.Hour)},
			now:  now,
			want: false,
		},
		{
			name: "exact instant counts as live",
			info: LeaseInfo{ExpiresAt: now},
			now:  now,
			want: false,
		},
		{
			name: "one nanosecond past expiry",
			info: LeaseInfo{ExpiresAt: now.Add(-time.Nanosecond)},
			now:  now,
			want: true,
		},
		{
			name: "zero expiresat never expires",
			info: LeaseInfo{},
			now:  now,
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.info.Expired(tc.now); got != tc.want {
				t.Errorf("Expired() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLeaseInfo_TimeRemaining(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		info LeaseInfo
		now  time.Time
		want time.Duration
	}{
		{
			name: "half an hour left",
			info: LeaseInfo{ExpiresAt: now.Add(30 * time.Minute)},
			now:  now,
			want: 30 * time.Minute,
		},
		{
			name: "expired clamps to 0",
			info: LeaseInfo{ExpiresAt: now.Add(-time.Minute)},
			now:  now,
			want: 0,
		},
		{
			name: "zero expiresat returns 0",
			info: LeaseInfo{},
			now:  now,
			want: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.info.TimeRemaining(tc.now); got != tc.want {
				t.Errorf("TimeRemaining() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLeaseInfo_ShouldRenew(t *testing.T) {
	t.Parallel()

	issued := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	ttl := time.Hour

	// Helper: a renewable info with TTL=1h, expires at issued+ttl.
	mk := func() LeaseInfo {
		return LeaseInfo{
			IssuedAt:  issued,
			ExpiresAt: issued.Add(ttl),
			Duration:  ttl,
			Renewable: true,
			State:     LeaseStateActive,
		}
	}

	tests := []struct {
		name     string
		info     LeaseInfo
		strategy RenewStrategy
		now      time.Time
		want     bool
	}{
		{
			name:     "eager: just past 50% triggers",
			info:     mk(),
			strategy: RenewStrategyEager,
			now:      issued.Add(31 * time.Minute),
			want:     true,
		},
		{
			name:     "eager: at 49% holds",
			info:     mk(),
			strategy: RenewStrategyEager,
			now:      issued.Add(29 * time.Minute),
			want:     false,
		},
		{
			name:     "eager: exactly at 50% triggers",
			info:     mk(),
			strategy: RenewStrategyEager,
			now:      issued.Add(30 * time.Minute),
			want:     true,
		},
		{
			name:     "lazy: at 85% holds",
			info:     mk(),
			strategy: RenewStrategyLazy,
			now:      issued.Add(51 * time.Minute),
			want:     false,
		},
		{
			name:     "lazy: at 91% triggers",
			info:     mk(),
			strategy: RenewStrategyLazy,
			now:      issued.Add(55 * time.Minute), // ~91.6%
			want:     true,
		},
		{
			name:     "on_demand: never triggers, even past 99%",
			info:     mk(),
			strategy: RenewStrategyOnDemand,
			now:      issued.Add(59 * time.Minute),
			want:     false,
		},
		{
			name: "non-renewable: never triggers",
			info: func() LeaseInfo {
				l := mk()
				l.Renewable = false
				return l
			}(),
			strategy: RenewStrategyEager,
			now:      issued.Add(45 * time.Minute),
			want:     false,
		},
		{
			name:     "expired: never triggers",
			info:     mk(),
			strategy: RenewStrategyEager,
			now:      issued.Add(2 * time.Hour),
			want:     false,
		},
		{
			name: "zero duration: never triggers (defensive)",
			info: func() LeaseInfo {
				l := mk()
				l.Duration = 0
				return l
			}(),
			strategy: RenewStrategyEager,
			now:      issued.Add(30 * time.Minute),
			want:     false,
		},
		{
			name:     "unknown strategy: never triggers",
			info:     mk(),
			strategy: RenewStrategyUnknown,
			now:      issued.Add(45 * time.Minute),
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.info.ShouldRenew(tc.now, tc.strategy); got != tc.want {
				t.Errorf("ShouldRenew(%v, %v) = %v, want %v", tc.now.Sub(issued), tc.strategy, got, tc.want)
			}
		})
	}
}
