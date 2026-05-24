// SPDX-License-Identifier: Apache-2.0

package versioning_test

import (
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/api/versioning"
)

func TestStatus_String(t *testing.T) {
	tests := []struct {
		s    versioning.Status
		want string
	}{
		{versioning.StatusAlpha, "alpha"},
		{versioning.StatusBeta, "beta"},
		{versioning.StatusCurrent, "current"},
		{versioning.StatusSupported, "supported"},
		{versioning.StatusDeprecated, "deprecated"},
		{versioning.StatusRetired, "retired"},
		{versioning.StatusUnspecified, "unspecified"},
		{versioning.Status(99), "unspecified"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.s.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	tests := []struct {
		in      string
		want    versioning.Status
		wantErr bool
	}{
		{"alpha", versioning.StatusAlpha, false},
		{"beta", versioning.StatusBeta, false},
		{"current", versioning.StatusCurrent, false},
		{"supported", versioning.StatusSupported, false},
		{"deprecated", versioning.StatusDeprecated, false},
		{"retired", versioning.StatusRetired, false},
		{"", versioning.StatusUnspecified, false},
		{"sunset", versioning.StatusUnspecified, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := versioning.ParseStatus(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr=%v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistry_RegisterLookup(t *testing.T) {
	r := versioning.NewRegistry()
	if _, ok := r.Lookup("/missing"); ok {
		t.Error("empty registry should not find /missing")
	}

	want := versioning.Endpoint{
		Method: "/svc/M",
		Status: versioning.StatusCurrent,
	}
	r.Register(want)
	got, ok := r.Lookup("/svc/M")
	if !ok {
		t.Fatal("Lookup after Register: not found")
	}
	if got.Method != want.Method || got.Status != want.Status {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestRegistry_EffectiveStatus(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		e    versioning.Endpoint
		want versioning.Status
	}{
		{
			name: "current",
			e:    versioning.Endpoint{Method: "/m", Status: versioning.StatusCurrent},
			want: versioning.StatusCurrent,
		},
		{
			name: "deprecated, sunset future",
			e: versioning.Endpoint{
				Method:   "/m",
				Status:   versioning.StatusDeprecated,
				SunsetAt: now.Add(30 * 24 * time.Hour),
			},
			want: versioning.StatusDeprecated,
		},
		{
			name: "deprecated, sunset past => retired override",
			e: versioning.Endpoint{
				Method:   "/m",
				Status:   versioning.StatusDeprecated,
				SunsetAt: now.Add(-1 * time.Hour),
			},
			want: versioning.StatusRetired,
		},
		{
			name: "explicit retired",
			e:    versioning.Endpoint{Method: "/m", Status: versioning.StatusRetired},
			want: versioning.StatusRetired,
		},
		{
			name: "current with sunset in past => retired override",
			e: versioning.Endpoint{
				Method:   "/m",
				Status:   versioning.StatusCurrent,
				SunsetAt: now.Add(-time.Hour),
			},
			want: versioning.StatusRetired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := versioning.NewRegistry()
			r.SetClock(func() time.Time { return now })
			r.Register(tt.e)
			if got := r.EffectiveStatus("/m"); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("untracked => unspecified", func(t *testing.T) {
		r := versioning.NewRegistry()
		if got := r.EffectiveStatus("/never-registered"); got != versioning.StatusUnspecified {
			t.Errorf("got %v, want unspecified", got)
		}
	})
}

func TestRegistry_IsRetired(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := versioning.NewRegistry()
	r.SetClock(func() time.Time { return now })

	r.Register(versioning.Endpoint{Method: "/current", Status: versioning.StatusCurrent})
	r.Register(versioning.Endpoint{
		Method:   "/expired",
		Status:   versioning.StatusDeprecated,
		SunsetAt: now.Add(-time.Hour),
	})
	r.Register(versioning.Endpoint{Method: "/explicit", Status: versioning.StatusRetired})

	if r.IsRetired("/current") {
		t.Error("current should not be retired")
	}
	if !r.IsRetired("/expired") {
		t.Error("expired should be retired (sunset override)")
	}
	if !r.IsRetired("/explicit") {
		t.Error("explicit should be retired")
	}
	if r.IsRetired("/untracked") {
		t.Error("untracked methods are not retired")
	}
}

// Concurrent Register + Lookup must not race; the registry is a
// shared resource and the auth + handler chains read from many
// goroutines.
func TestRegistry_Concurrency(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{Method: "/seed", Status: versioning.StatusCurrent})

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			r.Register(versioning.Endpoint{
				Method: "/m",
				Status: versioning.StatusCurrent,
			})
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		_, _ = r.Lookup("/m")
		_ = r.IsRetired("/m")
		_ = r.EffectiveStatus("/seed")
	}
	<-done
}
