// SPDX-License-Identifier: Apache-2.0

//go:build linux

package user

import (
	"context"
	"errors"
	"testing"
)

// fakeLookPath returns success only for the binaries in present.
func fakeLookPath(present ...string) func(string) (string, error) {
	set := make(map[string]struct{}, len(present))
	for _, p := range present {
		set[p] = struct{}{}
	}
	return func(bin string) (string, error) {
		if _, ok := set[bin]; ok {
			return "/usr/sbin/" + bin, nil
		}
		return "", errors.New("not found")
	}
}

func TestDetectProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		present []string
		wantFn  func(Provider) bool
		wantStr string
	}{
		{
			name:    "useradd wins over adduser",
			present: []string{"useradd", "adduser"}, // Debian-like: both present
			wantFn:  func(p Provider) bool { _, ok := p.(shadowProvider); return ok },
			wantStr: "shadowProvider",
		},
		{
			name:    "adduser fallback",
			present: []string{"adduser"}, // BusyBox / Alpine
			wantFn:  func(p Provider) bool { _, ok := p.(*busyboxProvider); return ok },
			wantStr: "*busyboxProvider",
		},
		{
			name:    "neither detected",
			present: nil,
			wantFn:  func(p Provider) bool { _, ok := p.(undetectedProvider); return ok },
			wantStr: "undetectedProvider",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := detectProvider(fakeLookPath(tt.present...))
			if !tt.wantFn(p) {
				t.Errorf("detectProvider() = %T, want %s", p, tt.wantStr)
			}
		})
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}

func TestUndetectedProvider_MutationsReturnNoBackend(t *testing.T) {
	t.Parallel()
	p := undetectedProvider{}
	ctx := context.Background()
	if err := p.Add(ctx, AddOptions{Name: "x"}); !IsNoBackend(err) {
		t.Errorf("Add err = %v, want ErrNoBackend", err)
	}
	if err := p.Mod(ctx, ModOptions{Name: "x"}); !IsNoBackend(err) {
		t.Errorf("Mod err = %v, want ErrNoBackend", err)
	}
	if err := p.Del(ctx, "x", false); !IsNoBackend(err) {
		t.Errorf("Del err = %v, want ErrNoBackend", err)
	}
	if err := p.SetGroups(ctx, "x", nil); !IsNoBackend(err) {
		t.Errorf("SetGroups err = %v, want ErrNoBackend", err)
	}
}

// TestUndetectedProvider_LookupWorks confirms the read path still
// functions when no backend is detected (osLookup via NSS).
func TestUndetectedProvider_LookupWorks(t *testing.T) {
	t.Parallel()
	p := undetectedProvider{}
	if _, err := p.Lookup("zzz-no-such-user-zzz"); err != nil {
		t.Errorf("Lookup should not error on a missing user; got %v", err)
	}
}
