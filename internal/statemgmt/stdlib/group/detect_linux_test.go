// SPDX-License-Identifier: Apache-2.0

//go:build linux

package group

import (
	"context"
	"errors"
	"testing"
)

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
			name:    "groupadd wins over addgroup",
			present: []string{"groupadd", "addgroup"}, // Debian-like: both present
			wantFn:  func(p Provider) bool { _, ok := p.(shadowProvider); return ok },
			wantStr: "shadowProvider",
		},
		{
			name:    "addgroup fallback",
			present: []string{"addgroup"}, // BusyBox / Alpine
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
	if err := p.Add(ctx, "x", nil, false); !IsNoBackend(err) {
		t.Errorf("Add err = %v, want ErrNoBackend", err)
	}
	if err := p.Mod(ctx, "x", 1); !IsNoBackend(err) {
		t.Errorf("Mod err = %v, want ErrNoBackend", err)
	}
	if err := p.Del(ctx, "x"); !IsNoBackend(err) {
		t.Errorf("Del err = %v, want ErrNoBackend", err)
	}
}

func TestUndetectedProvider_LookupWorks(t *testing.T) {
	t.Parallel()
	p := undetectedProvider{}
	if _, err := p.Lookup("zzz-no-such-group-zzz"); err != nil {
		t.Errorf("Lookup should not error on a missing group; got %v", err)
	}
}
