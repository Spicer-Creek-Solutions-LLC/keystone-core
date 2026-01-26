package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildTLSConfig_NoTLS(t *testing.T) {
	cfg := &Config{}

	// When TLS is not enabled, buildTLSConfig should still work
	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tlsConfig.MinVersion != 0x0304 { // TLS 1.3
		t.Errorf("expected MinVersion TLS 1.3, got %x", tlsConfig.MinVersion)
	}

	if tlsConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false by default")
	}
}

func TestBuildTLSConfig_SkipVerify(t *testing.T) {
	t.Setenv("KSCORE_ALLOW_INSECURE_TLS", "1")
	cfg := &Config{
		TLSSkipVerify: true,
	}

	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !tlsConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true when TLSSkipVerify is set")
	}
}

func TestBuildTLSConfig_SkipVerifyBlocked(t *testing.T) {
	cfg := &Config{
		TLSSkipVerify: true,
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Fatal("expected error when tls skip verify is set without KSCORE_ALLOW_INSECURE_TLS")
	}
}

func TestBuildTLSConfig_MinVersionOverride(t *testing.T) {
	cfg := &Config{
		TLSMinVersion: "1.2",
	}

	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tlsConfig.MinVersion != 0x0303 { // TLS 1.2
		t.Errorf("expected MinVersion TLS 1.2, got %x", tlsConfig.MinVersion)
	}
}

func TestBuildTLSConfig_MinVersionInvalid(t *testing.T) {
	cfg := &Config{
		TLSMinVersion: "1.1",
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported TLS minimum version")
	}
}

func TestBuildTLSConfig_ServerName(t *testing.T) {
	cfg := &Config{
		TLSServerName: "test.example.com",
	}

	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tlsConfig.ServerName != "test.example.com" {
		t.Errorf("expected ServerName 'test.example.com', got '%s'", tlsConfig.ServerName)
	}
}

func TestBuildTLSConfig_CACert(t *testing.T) {
	// Create a temporary CA certificate
	tmpDir := t.TempDir()
	caCertPath := filepath.Join(tmpDir, "ca.crt")

	caCert, _, err := generateTestCert(true)
	if err != nil {
		t.Fatalf("failed to generate test CA cert: %v", err)
	}

	if err := os.WriteFile(caCertPath, caCert, 0600); err != nil {
		t.Fatalf("failed to write CA cert: %v", err)
	}

	cfg := &Config{
		TLS:       true,
		TLSCACert: caCertPath,
	}

	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tlsConfig.RootCAs == nil {
		t.Error("RootCAs should be set when CA cert is provided")
	}
}

func TestBuildTLSConfig_CACert_NotFound(t *testing.T) {
	cfg := &Config{
		TLS:       true,
		TLSCACert: "/nonexistent/ca.crt",
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error for nonexistent CA cert file")
	}
}

func TestBuildTLSConfig_CACert_Invalid(t *testing.T) {
	tmpDir := t.TempDir()
	caCertPath := filepath.Join(tmpDir, "ca.crt")

	// Write invalid PEM data
	if err := os.WriteFile(caCertPath, []byte("not a valid certificate"), 0600); err != nil {
		t.Fatalf("failed to write invalid cert: %v", err)
	}

	cfg := &Config{
		TLS:       true,
		TLSCACert: caCertPath,
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid CA cert")
	}
}

func TestBuildTLSConfig_ClientCert(t *testing.T) {
	// Create temporary client certificate and key
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "client.crt")
	keyPath := filepath.Join(tmpDir, "client.key")

	cert, key, err := generateTestCert(false)
	if err != nil {
		t.Fatalf("failed to generate test cert: %v", err)
	}

	if err := os.WriteFile(certPath, cert, 0600); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}

	cfg := &Config{
		TLS:     true,
		TLSCert: certPath,
		TLSKey:  keyPath,
	}

	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(tlsConfig.Certificates))
	}
}

func TestBuildTLSConfig_ClientCert_CertOnly(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "client.crt")

	cert, _, err := generateTestCert(false)
	if err != nil {
		t.Fatalf("failed to generate test cert: %v", err)
	}

	if err := os.WriteFile(certPath, cert, 0600); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}

	cfg := &Config{
		TLS:     true,
		TLSCert: certPath,
		// TLSKey intentionally not set
	}

	_, err = buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error when only cert is provided without key")
	}
}

func TestBuildTLSConfig_ClientCert_KeyOnly(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "client.key")

	_, key, err := generateTestCert(false)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}

	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}

	cfg := &Config{
		TLS:    true,
		TLSKey: keyPath,
		// TLSCert intentionally not set
	}

	_, err = buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error when only key is provided without cert")
	}
}

func TestBuildTLSConfig_ClientCert_InvalidFiles(t *testing.T) {
	cfg := &Config{
		TLS:     true,
		TLSCert: "/nonexistent/client.crt",
		TLSKey:  "/nonexistent/client.key",
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error for nonexistent cert/key files")
	}
}

// generateTestCert generates a self-signed certificate for testing
func generateTestCert(isCA bool) (certPEM, keyPEM []byte, err error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}
