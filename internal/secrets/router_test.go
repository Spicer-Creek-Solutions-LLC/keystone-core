// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"errors"
	"strings"
	"testing"
)

func TestNewRouter_ValidatesEach(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		routes  []Route
		wantSub string
	}{
		{
			name:    "empty prefix rejected",
			routes:  []Route{{Prefix: "", Backend: "file"}},
			wantSub: "prefix is required",
		},
		{
			name:    "leading slash rejected",
			routes:  []Route{{Prefix: "/kv/", Backend: "file"}},
			wantSub: "must not start with",
		},
		{
			name:    "whitespace in prefix rejected",
			routes:  []Route{{Prefix: "kv app/", Backend: "file"}},
			wantSub: "whitespace or non-printable",
		},
		{
			name:    "tab in prefix rejected",
			routes:  []Route{{Prefix: "kv\t/", Backend: "file"}},
			wantSub: "whitespace or non-printable",
		},
		{
			name:    "control byte in prefix rejected",
			routes:  []Route{{Prefix: "kv\x01/", Backend: "file"}},
			wantSub: "whitespace or non-printable",
		},
		{
			name:    "empty backend rejected",
			routes:  []Route{{Prefix: "kv/", Backend: ""}},
			wantSub: "backend is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewRouter(tc.routes)
			if err == nil {
				t.Fatalf("NewRouter() = nil err, want %q", tc.wantSub)
			}
			if !errors.Is(err, ErrInvalidBackend) {
				t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestNewRouter_DuplicatePrefix(t *testing.T) {
	t.Parallel()

	t.Run("single duplicate", func(t *testing.T) {
		t.Parallel()
		_, err := NewRouter([]Route{
			{Prefix: "kv/", Backend: "file"},
			{Prefix: "kv/", Backend: "vault"},
		})
		if err == nil {
			t.Fatalf("NewRouter() = nil err, want duplicate error")
		}
		if !errors.Is(err, ErrInvalidBackend) {
			t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
		}
		if !strings.Contains(err.Error(), `"kv/"`) {
			t.Errorf("err = %q, want contains %q", err.Error(), `"kv/"`)
		}
		if !strings.Contains(err.Error(), "duplicate prefix") {
			t.Errorf("err = %q, want contains %q", err.Error(), "duplicate prefix")
		}
	})

	t.Run("multiple duplicates listed in one error", func(t *testing.T) {
		t.Parallel()
		_, err := NewRouter([]Route{
			{Prefix: "kv/", Backend: "file"},
			{Prefix: "kv/", Backend: "vault"},
			{Prefix: "secret/", Backend: "vault"},
			{Prefix: "secret/", Backend: "file"},
		})
		if err == nil {
			t.Fatalf("NewRouter() = nil err, want duplicate error")
		}
		if !strings.Contains(err.Error(), `"kv/"`) {
			t.Errorf("err = %q, want %q in message", err.Error(), `"kv/"`)
		}
		if !strings.Contains(err.Error(), `"secret/"`) {
			t.Errorf("err = %q, want %q in message", err.Error(), `"secret/"`)
		}
	})
}

func TestNewRouter_HappyPath(t *testing.T) {
	t.Parallel()

	r, err := NewRouter([]Route{
		{Prefix: "kv/", Backend: "file"},
		{Prefix: "secret/", Backend: "vault"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if r.Len() != 2 {
		t.Errorf("Len() = %d, want 2", r.Len())
	}
}

func TestRouter_Lookup(t *testing.T) {
	t.Parallel()

	r, err := NewRouter([]Route{
		{Prefix: "secret/", Backend: "vault"},
		{Prefix: "secret/data/", Backend: "vault-kv2"},
		{Prefix: "kv/", Backend: "file"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		wantBackend string
		wantPrefix  string
		wantOK      bool
	}{
		{
			name:        "longest prefix wins (secret/data/ over secret/)",
			path:        "secret/data/foo",
			wantBackend: "vault-kv2",
			wantPrefix:  "secret/data/",
			wantOK:      true,
		},
		{
			name:        "falls back to shorter prefix when longer does not match",
			path:        "secret/static/foo",
			wantBackend: "vault",
			wantPrefix:  "secret/",
			wantOK:      true,
		},
		{
			name:        "kv routes to file backend",
			path:        "kv/app/db",
			wantBackend: "file",
			wantPrefix:  "kv/",
			wantOK:      true,
		},
		{
			name:   "no match returns zero route and false",
			path:   "other/path",
			wantOK: false,
		},
		{
			name:   "empty path no match",
			path:   "",
			wantOK: false,
		},
		{
			name:        "exact prefix match counts",
			path:        "kv/",
			wantBackend: "file",
			wantPrefix:  "kv/",
			wantOK:      true,
		},
		{
			name:   "path shorter than any prefix no match",
			path:   "kv",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := r.Lookup(tc.path)
			if ok != tc.wantOK {
				t.Fatalf("Lookup(%q) ok = %v, want %v", tc.path, ok, tc.wantOK)
			}
			if !ok {
				if got != (Route{}) {
					t.Errorf("Lookup(%q) returned %#v on miss, want zero Route", tc.path, got)
				}
				return
			}
			if got.Backend != tc.wantBackend {
				t.Errorf("Lookup(%q) backend = %q, want %q", tc.path, got.Backend, tc.wantBackend)
			}
			if got.Prefix != tc.wantPrefix {
				t.Errorf("Lookup(%q) prefix = %q, want %q", tc.path, got.Prefix, tc.wantPrefix)
			}
		})
	}
}

func TestRouter_Lookup_TrailingSlashFootgun(t *testing.T) {
	t.Parallel()

	// A prefix without trailing `/` matches *any* path starting with
	// that literal string, including `secretstore/...`. This is the
	// documented Vault-style semantic — operators avoid it by
	// terminating prefixes at a segment boundary. The test pins the
	// behavior so a future "auto-segment" refactor is intentional.
	r, err := NewRouter([]Route{
		{Prefix: "secret", Backend: "vault"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	for _, path := range []string{"secret/foo", "secretstore/foo", "secret"} {
		got, ok := r.Lookup(path)
		if !ok {
			t.Errorf("Lookup(%q) ok = false, want true (literal-prefix semantic)", path)
			continue
		}
		if got.Backend != "vault" {
			t.Errorf("Lookup(%q) backend = %q, want vault", path, got.Backend)
		}
	}
}

func TestRouter_SortDeterministic(t *testing.T) {
	t.Parallel()

	// Input order is intentionally scrambled — Routes() must come back
	// in (len DESC, lex ASC) order regardless of how it went in.
	r, err := NewRouter([]Route{
		{Prefix: "b/", Backend: "x"},
		{Prefix: "secret/data/", Backend: "vault-kv2"},
		{Prefix: "a/", Backend: "y"},
		{Prefix: "secret/", Backend: "vault"},
		{Prefix: "kv/", Backend: "file"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	wantOrder := []string{"secret/data/", "secret/", "kv/", "a/", "b/"}
	got := r.Routes()
	if len(got) != len(wantOrder) {
		t.Fatalf("Routes() len = %d, want %d", len(got), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got[i].Prefix != want {
			t.Errorf("Routes()[%d].Prefix = %q, want %q", i, got[i].Prefix, want)
		}
	}
}

func TestRouter_RoutesDefensiveCopy(t *testing.T) {
	t.Parallel()

	r, err := NewRouter([]Route{
		{Prefix: "kv/", Backend: "file"},
		{Prefix: "secret/", Backend: "vault"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	snapshot := r.Routes()
	snapshot[0].Backend = "hijacked"
	snapshot[0].Prefix = "evil/"

	// Re-fetch and assert the router is unmodified.
	fresh := r.Routes()
	for _, route := range fresh {
		if route.Backend == "hijacked" || route.Prefix == "evil/" {
			t.Fatalf("mutating Routes() result leaked into router state: %#v", fresh)
		}
	}
	if got, ok := r.Lookup("kv/foo"); !ok || got.Backend != "file" {
		t.Errorf("Lookup after Routes() mutation = (%#v, %v), want kv/→file", got, ok)
	}
}

func TestRouter_Empty(t *testing.T) {
	t.Parallel()

	r, err := NewRouter(nil)
	if err != nil {
		t.Fatalf("NewRouter(nil): %v", err)
	}
	if r.Len() != 0 {
		t.Errorf("Len() = %d, want 0", r.Len())
	}
	if got, ok := r.Lookup("kv/foo"); ok {
		t.Errorf("Lookup on empty router returned %#v, want miss", got)
	}
	if routes := r.Routes(); len(routes) != 0 {
		t.Errorf("Routes() on empty router = %#v, want empty", routes)
	}
}
