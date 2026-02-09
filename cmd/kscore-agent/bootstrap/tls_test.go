package bootstrap

import "testing"

func TestGenerateTLSBundle(t *testing.T) {
	bundle, err := GenerateTLSBundle([]string{"example.com", "127.0.0.1"})
	if err != nil {
		t.Fatalf("GenerateTLSBundle returned error: %v", err)
	}
	if len(bundle.CertPEM) == 0 || len(bundle.KeyPEM) == 0 || len(bundle.CAPEM) == 0 {
		t.Fatal("expected non-empty cert bundle")
	}
	if string(bundle.CertPEM[:27]) != "-----BEGIN CERTIFICATE-----" {
		t.Fatal("expected certificate PEM header")
	}
	if string(bundle.KeyPEM[:31]) != "-----BEGIN RSA PRIVATE KEY-----" {
		t.Fatal("expected RSA key PEM header")
	}
}

func TestResolveTLSPaths(t *testing.T) {
	cfg := &Config{GenerateCerts: true}
	cert, key, ca := resolveTLSPaths(cfg)
	if cert != defaultTLSCertPath || key != defaultTLSKeyPath || ca != defaultTLSCAPath {
		t.Fatal("expected default tls paths when generating certs")
	}
}
