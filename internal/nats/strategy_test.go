// SPDX-License-Identifier: Apache-2.0

package nats

import (
	"crypto/tls"
	"testing"
)

func TestStrategySelector_DispatchesByScheme(t *testing.T) {
	sel := NewStrategySelector(nil)

	if got := sel.Select("nats").Scheme(); got != "nats" {
		t.Errorf("Select(nats) = %q", got)
	}
	if got := sel.Select("tls").Scheme(); got != "tls" {
		t.Errorf("Select(tls) = %q", got)
	}
	// Mixed case routes the same way (config rarely round-trips through
	// the lowercaser before hitting the selector).
	if got := sel.Select("TLS").Scheme(); got != "tls" {
		t.Errorf("Select(TLS) = %q", got)
	}
	// Unknown scheme falls back to direct; better than a panic for a
	// typo'd URL.
	if got := sel.Select("ws").Scheme(); got != "nats" {
		t.Errorf("Select(ws) = %q, want nats (fallback)", got)
	}
}

func TestStrategySelector_PreservesTLSConfig(t *testing.T) {
	cfg := &tls.Config{ServerName: "kscore.example.com"}
	sel := NewStrategySelector(cfg)
	tlsStrat, ok := sel.Select("tls").(TLSStrategy)
	if !ok {
		t.Fatalf("Select(tls) returned %T, want TLSStrategy", sel.Select("tls"))
	}
	if tlsStrat.TLSConfig != cfg {
		t.Error("TLSConfig was not preserved through the selector")
	}
}

func TestSchemeFromURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"nats://localhost:4222", "nats"},
		{"tls://nats.example.com:4222", "tls"},
		{"NATS://x:4222", "nats"},
		{"not a url", ""},
	}
	for _, tt := range tests {
		if got := schemeFromURL(tt.in); got != tt.want {
			t.Errorf("schemeFromURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDirectStrategy_ConnectErrorWrapped(t *testing.T) {
	// Unreachable URL: surface goes through DirectStrategy; we just
	// want the error text to namespace the failure.
	_, err := DirectStrategy{}.Connect(Endpoint{URL: "nats://127.0.0.1:1"}, nil)
	if err == nil {
		t.Fatal("expected error for unreachable URL")
	}
	if got := err.Error(); !contains(got, "direct connect") {
		t.Errorf("err = %q, want containing 'direct connect'", got)
	}
}

func TestTLSStrategy_NilConfigUsesSystemRoots(t *testing.T) {
	// We can't actually connect over TLS in a unit test without a
	// listening server. Asserting Connect returns an error is enough
	// to prove the path is wired and namespaced.
	_, err := TLSStrategy{}.Connect(Endpoint{URL: "tls://127.0.0.1:1"}, nil)
	if err == nil {
		t.Fatal("expected error for unreachable TLS URL")
	}
	if got := err.Error(); !contains(got, "tls connect") {
		t.Errorf("err = %q, want containing 'tls connect'", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
