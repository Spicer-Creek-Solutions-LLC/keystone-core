package gnmi

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/security"
)

func TestBuildTLSConfig_SkipVerify(t *testing.T) {
	cred := &credentials.GNMICredential{
		SkipVerify: true,
	}

	cfg, err := buildTLSConfig(cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=true")
	}
}

func TestBuildTLSConfig_MinVersion(t *testing.T) {
	cred := &credentials.GNMICredential{}
	cfg, err := buildTLSConfig(cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("expected MinVersion=TLS1.3, got %d", cfg.MinVersion)
	}
}

func TestBuildTLSConfig_CACert(t *testing.T) {
	ca, err := security.GenerateCA("test-ca", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate CA: %v", err)
	}

	cred := &credentials.GNMICredential{
		CACert: encodeCert(ca.Certificate),
	}

	cfg, err := buildTLSConfig(cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Error("expected RootCAs to be set")
	}
}

func TestBuildTLSConfig_InvalidCACert(t *testing.T) {
	cred := &credentials.GNMICredential{
		CACert: []byte("not-a-valid-pem"),
	}

	_, err := buildTLSConfig(cred)
	if err == nil {
		t.Error("expected error for invalid CA cert")
	}
}

func TestBuildTLSConfig_MTLS(t *testing.T) {
	ca, err := security.GenerateCA("test-ca", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate CA: %v", err)
	}

	clientCert, clientKey, err := ca.GenerateClientCert("test-client", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate client cert: %v", err)
	}

	cred := &credentials.GNMICredential{
		CACert:     encodeCert(ca.Certificate),
		ClientCert: encodeCert(clientCert),
		ClientKey:  encodeKey(clientKey),
	}

	cfg, err := buildTLSConfig(cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Error("expected RootCAs to be set")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("expected 1 client certificate, got %d", len(cfg.Certificates))
	}
}

func TestBuildTLSConfig_InvalidClientCert(t *testing.T) {
	cred := &credentials.GNMICredential{
		ClientCert: []byte("not-a-cert"),
		ClientKey:  []byte("not-a-key"),
	}

	_, err := buildTLSConfig(cred)
	if err == nil {
		t.Error("expected error for invalid client cert")
	}
}

func TestBuildTLSConfig_Empty(t *testing.T) {
	cred := &credentials.GNMICredential{}

	cfg, err := buildTLSConfig(cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RootCAs != nil {
		t.Error("expected nil RootCAs for empty cred")
	}
	if len(cfg.Certificates) != 0 {
		t.Error("expected no client certificates for empty cred")
	}
}
