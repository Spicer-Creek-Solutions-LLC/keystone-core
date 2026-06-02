// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/url"
	"testing"
	"time"
)

// selfSignedForeignServerCert mints a self-signed SERVER cert (CA =
// self) carrying a `server/*` SPIFFE URI SAN from a DIFFERENT trust
// domain. The provider's bundle won't anchor it, so a
// BuildClientTLSConfig client rejects it at chain verification.
func selfSignedForeignServerCert(t *testing.T, key *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	foreignID, _ := ServerID("foreign.org", "control-plane")
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		Subject:      pkix.Name{CommonName: "rogue-server"},
		URIs:         []*url.URL{foreignID.URI()},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: parsed}
}

// ---- BuildClientTLSConfig --------------------------------------

func TestBuildClientTLSConfig_NilProvider(t *testing.T) {
	t.Parallel()
	_, cancel, err := BuildClientTLSConfig(context.Background(), nil, nil)
	t.Cleanup(cancel)
	if err == nil || !errors.Is(err, ErrTLSConfig) {
		t.Errorf("err = %v, want ErrTLSConfig", err)
	}
}

func TestBuildClientTLSConfig_ProviderNotRunning(t *testing.T) {
	t.Parallel()
	store := NewInMemoryJoinTokenStore()
	caStorage, _ := NewFileCAStorage(t.TempDir())
	p, _ := NewEmbeddedProvider(EmbeddedProviderConfig{
		CAConfig:       shortLifetimeCAConfigTLS(),
		Storage:        caStorage,
		JoinTokenStore: store,
	})
	// Don't Start — Health should fail.
	_, cancel, err := BuildClientTLSConfig(context.Background(), p, nil)
	t.Cleanup(cancel)
	if err == nil || !errors.Is(err, ErrTLSConfig) {
		t.Errorf("err = %v, want ErrTLSConfig", err)
	}
}

// TestBuildClientTLSConfig_MutualHandshake is the core path: a
// strict-mTLS control-plane server + a BuildClientTLSConfig client,
// both anchored on the same provider, complete a mutual handshake.
// This exercises BOTH directions — the server verifies the client's
// SVID via its ClientCAs pool, and the client verifies the server's
// SVID via the SPIFFE VerifyPeerCertificate.
func TestBuildClientTLSConfig_MutualHandshake(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)

	serverCfg, scancel, err := BuildServerTLSConfig(context.Background(), p, ServerRoleControlPlane,
		&ServerTLSOptions{ClientAuth: tls.RequireAndVerifyClientCert})
	if err != nil {
		t.Fatalf("BuildServerTLSConfig: %v", err)
	}
	t.Cleanup(scancel)

	clientCfg, ccancel, err := BuildClientTLSConfig(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig: %v", err)
	}
	t.Cleanup(ccancel)

	if err := runHandshake(t, serverCfg, clientCfg); err != nil {
		t.Fatalf("mutual handshake: %v", err)
	}
}

// TestBuildClientTLSConfig_RejectsForeignServer confirms the client
// refuses a server whose cert does not chain to the trust bundle.
func TestBuildClientTLSConfig_RejectsForeignServer(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)

	clientCfg, ccancel, err := BuildClientTLSConfig(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig: %v", err)
	}
	t.Cleanup(ccancel)

	foreignKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	foreignServer := selfSignedForeignServerCert(t, foreignKey)
	serverCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{foreignServer},
	}

	if err := runHandshake(t, serverCfg, clientCfg); err == nil {
		t.Fatal("client accepted a server cert that does not chain to the trust bundle")
	}
}

// TestBuildClientTLSConfig_RejectsNonServerPeer confirms the SPIFFE
// guard: a provider-issued cert that chains to the bundle but carries
// an agent (not server) identity is rejected by the client.
func TestBuildClientTLSConfig_RejectsNonServerPeer(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)

	clientCfg, ccancel, err := BuildClientTLSConfig(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig: %v", err)
	}
	t.Cleanup(ccancel)

	// A real provider-issued agent SVID: chains to the bundle (so the
	// chain check passes), but its URI SAN is agent/* — the SPIFFE
	// guard must reject it. The CA stamps both server+client EKU, so
	// it is usable as a server cert here.
	agentSVID := mintAgentClientCert(t, p, "agent-posing-as-server")
	serverCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{agentSVID},
	}

	if err := runHandshake(t, serverCfg, clientCfg); err == nil {
		t.Fatal("client accepted an agent identity as a coordination server peer")
	}
}

// TestBuildClientTLSConfig_RefreshAfterRotation confirms the watcher
// reissues the presented client cert under the new signing CA.
func TestBuildClientTLSConfig_RefreshAfterRotation(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)

	clientCfg, cancel, err := BuildClientTLSConfig(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig: %v", err)
	}
	t.Cleanup(cancel)

	cert1, err := clientCfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	if err != nil {
		t.Fatalf("GetClientCertificate before rotation: %v", err)
	}
	signing1 := cert1.Certificate[1] // [leaf, signingCA]

	if err := p.RotateSigningCA(context.Background()); err != nil {
		t.Fatalf("RotateSigningCA: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cert2, err := clientCfg.GetClientCertificate(&tls.CertificateRequestInfo{})
		if err != nil {
			t.Fatalf("GetClientCertificate post-rotation: %v", err)
		}
		if string(cert2.Certificate[1]) != string(signing1) {
			return // watcher caught up — cert anchored under the new signing CA
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("client cert never refreshed after rotation within 2s")
}

func TestBuildClientTLSConfig_CancelStopsWatcher(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)
	_, cancel, err := BuildClientTLSConfig(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("BuildClientTLSConfig: %v", err)
	}
	cancel()
	cancel() // idempotent
}
