package extract

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// --- IP ---------------------------------------------------------------------

func TestIP_HTTP_RemoteAddr_IPv4(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "1.2.3.4:5678"
	k, ok := IP(IPConfig{}).HTTP(r)
	if !ok || k != "1.2.3.4" {
		t.Errorf("got (%q, %v), want (1.2.3.4, true)", k, ok)
	}
}

func TestIP_HTTP_RemoteAddr_IPv6(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "[::1]:5678"
	k, ok := IP(IPConfig{}).HTTP(r)
	if !ok || k != "::1" {
		t.Errorf("got (%q, %v), want (::1, true)", k, ok)
	}
}

func TestIP_HTTP_RemoteAddr_NoPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "1.2.3.4"
	k, ok := IP(IPConfig{}).HTTP(r)
	if !ok || k != "1.2.3.4" {
		t.Errorf("got (%q, %v), want (1.2.3.4, true)", k, ok)
	}
}

func TestIP_HTTP_RemoteAddrEmpty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = ""
	k, ok := IP(IPConfig{}).HTTP(r)
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

func TestIP_HTTP_TrustForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("X-Forwarded-For", "  9.9.9.9 , 10.0.0.1 , 10.0.0.2  ")
	r.RemoteAddr = "1.2.3.4:5678"
	k, ok := IP(IPConfig{TrustForwardedFor: true}).HTTP(r)
	if !ok || k != "9.9.9.9" {
		t.Errorf("got (%q, %v), want (9.9.9.9, true)", k, ok)
	}
}

func TestIP_HTTP_UntrustedForwardedFor_IgnoredByDefault(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("X-Forwarded-For", "9.9.9.9")
	r.RemoteAddr = "1.2.3.4:5678"
	k, ok := IP(IPConfig{}).HTTP(r)
	if !ok || k != "1.2.3.4" {
		t.Errorf("got (%q, %v), want (1.2.3.4, true)", k, ok)
	}
}

func TestIP_HTTP_ForwardedForEmpty_FallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("X-Forwarded-For", "  ,  ")
	r.RemoteAddr = "1.2.3.4:5678"
	k, ok := IP(IPConfig{TrustForwardedFor: true}).HTTP(r)
	if !ok || k != "1.2.3.4" {
		t.Errorf("got (%q, %v), want (1.2.3.4, true)", k, ok)
	}
}

func TestIP_GRPC(t *testing.T) {
	addr, _ := net.ResolveTCPAddr("tcp", "1.2.3.4:9000")
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: addr})
	k, ok := IP(IPConfig{}).GRPC(ctx)
	if !ok || k != "1.2.3.4" {
		t.Errorf("got (%q, %v), want (1.2.3.4, true)", k, ok)
	}
}

func TestIP_GRPC_NoPeer(t *testing.T) {
	k, ok := IP(IPConfig{}).GRPC(context.Background())
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

func TestIP_GRPC_PeerAddrNil(t *testing.T) {
	ctx := peer.NewContext(context.Background(), &peer.Peer{})
	k, ok := IP(IPConfig{}).GRPC(ctx)
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

// --- APIKey -----------------------------------------------------------------

func TestAPIKey_HTTP_Bearer(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer super-secret-key")
	k, ok := APIKey().HTTP(r)
	if !ok {
		t.Fatal("want ok")
	}
	if k != auth.HashAPIKey("super-secret-key") {
		t.Errorf("key mismatch: got %q, want %q", k, auth.HashAPIKey("super-secret-key"))
	}
	if k == "super-secret-key" {
		t.Error("cleartext leaked into key — must be hashed")
	}
}

func TestAPIKey_HTTP_BareToken(t *testing.T) {
	// kscorectl scripts historically send the bare token without
	// the Bearer prefix; preserve that compat.
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "raw-token")
	k, ok := APIKey().HTTP(r)
	if !ok || k != auth.HashAPIKey("raw-token") {
		t.Errorf("got (%q, %v)", k, ok)
	}
}

func TestAPIKey_HTTP_BearerCaseInsensitive(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "bearer mixedcase")
	k, ok := APIKey().HTTP(r)
	if !ok || k != auth.HashAPIKey("mixedcase") {
		t.Errorf("got (%q, %v)", k, ok)
	}
}

func TestAPIKey_HTTP_NoHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	k, ok := APIKey().HTTP(r)
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

func TestAPIKey_HTTP_EmptyBearer(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer ")
	k, ok := APIKey().HTTP(r)
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

func TestAPIKey_GRPC(t *testing.T) {
	md := metadata.Pairs("authorization", "Bearer grpc-key")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	k, ok := APIKey().GRPC(ctx)
	if !ok || k != auth.HashAPIKey("grpc-key") {
		t.Errorf("got (%q, %v)", k, ok)
	}
}

func TestAPIKey_GRPC_NoMetadata(t *testing.T) {
	k, ok := APIKey().GRPC(context.Background())
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

func TestAPIKey_GRPC_MetadataMissingHeader(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-other", "v"))
	k, ok := APIKey().GRPC(ctx)
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

// --- Header -----------------------------------------------------------------

func TestHeader_HTTP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("X-Tenant-Id", "tenant-7")
	k, ok := Header("X-Tenant-Id").HTTP(r)
	if !ok || k != "tenant-7" {
		t.Errorf("got (%q, %v)", k, ok)
	}
}

func TestHeader_HTTP_CaseInsensitiveLookup(t *testing.T) {
	// Go's net/http canonicalises header names; the operator
	// passing the wrong case should still resolve.
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("X-Tenant-Id", "tenant-7")
	k, ok := Header("x-tenant-id").HTTP(r)
	if !ok || k != "tenant-7" {
		t.Errorf("got (%q, %v)", k, ok)
	}
}

func TestHeader_HTTP_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	k, ok := Header("X-Tenant-Id").HTTP(r)
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

func TestHeader_HTTP_EmptyValue(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("X-Tenant-Id", "   ")
	k, ok := Header("X-Tenant-Id").HTTP(r)
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

func TestHeader_GRPC(t *testing.T) {
	md := metadata.Pairs("x-tenant-id", "tenant-7")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	k, ok := Header("X-Tenant-Id").GRPC(ctx)
	if !ok || k != "tenant-7" {
		t.Errorf("got (%q, %v)", k, ok)
	}
}

func TestHeader_GRPC_NoMetadata(t *testing.T) {
	k, ok := Header("X-Tenant-Id").GRPC(context.Background())
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

func TestHeader_GRPC_EmptyValue(t *testing.T) {
	md := metadata.Pairs("x-tenant-id", "")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	k, ok := Header("X-Tenant-Id").GRPC(ctx)
	if ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}

// --- Chain ------------------------------------------------------------------

func TestChain_FirstHitWins(t *testing.T) {
	// APIKey first; falls back to IP when no Authorization.
	chain := Chain{APIKey(), IP(IPConfig{})}

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "1.2.3.4:80"
	// No Authorization → IP hits.
	k, ok := chain.HTTP(r)
	if !ok || k != "1.2.3.4" {
		t.Errorf("no-auth got (%q, %v), want (1.2.3.4, true)", k, ok)
	}

	// With Authorization → APIKey hits, IP ignored.
	r.Header.Set("Authorization", "Bearer a-key")
	k, ok = chain.HTTP(r)
	if !ok || k != auth.HashAPIKey("a-key") {
		t.Errorf("with-auth got (%q, %v)", k, ok)
	}
}

func TestChain_GRPC_FirstHitWins(t *testing.T) {
	chain := Chain{APIKey(), Header("X-Tenant-Id")}

	// Authorization wins.
	md := metadata.Pairs("authorization", "Bearer gk", "x-tenant-id", "t1")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	k, ok := chain.GRPC(ctx)
	if !ok || k != auth.HashAPIKey("gk") {
		t.Errorf("got (%q, %v)", k, ok)
	}

	// Authorization absent → falls back to tenant header.
	md = metadata.Pairs("x-tenant-id", "t1")
	ctx = metadata.NewIncomingContext(context.Background(), md)
	k, ok = chain.GRPC(ctx)
	if !ok || k != "t1" {
		t.Errorf("fallback got (%q, %v)", k, ok)
	}
}

func TestChain_Empty(t *testing.T) {
	chain := Chain{}
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	if k, ok := chain.HTTP(r); ok || k != "" {
		t.Errorf("HTTP got (%q, %v), want (\"\", false)", k, ok)
	}
	if k, ok := chain.GRPC(context.Background()); ok || k != "" {
		t.Errorf("GRPC got (%q, %v), want (\"\", false)", k, ok)
	}
}

func TestChain_AllMiss(t *testing.T) {
	chain := Chain{APIKey(), Header("X-Tenant-Id")}
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	if k, ok := chain.HTTP(r); ok || k != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", k, ok)
	}
}
