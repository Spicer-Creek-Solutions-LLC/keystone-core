package security

import (
	"crypto/x509"
	"os"
	"testing"
	"time"
)

func TestGenerateCA(t *testing.T) {
	ca, err := GenerateCA("Test CA", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	if ca == nil {
		t.Fatal("CA should not be nil")
	}

	if ca.Certificate == nil {
		t.Fatal("CA certificate should not be nil")
	}

	if ca.PrivateKey == nil {
		t.Fatal("CA private key should not be nil")
	}

	// Verify certificate properties
	cert := ca.Certificate

	if cert.Subject.CommonName != "Test CA" {
		t.Errorf("Expected CommonName 'Test CA', got '%s'", cert.Subject.CommonName)
	}

	if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != "TitanAnvil" {
		t.Error("Expected Organization 'TitanAnvil'")
	}

	if !cert.IsCA {
		t.Error("Certificate should be marked as CA")
	}

	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("Certificate should have CertSign key usage")
	}

	if cert.NotBefore.After(time.Now()) {
		t.Error("NotBefore should be in the past")
	}

	expectedExpiry := time.Now().Add(365 * 24 * time.Hour)
	if cert.NotAfter.Before(expectedExpiry.Add(-1*time.Minute)) || cert.NotAfter.After(expectedExpiry.Add(1*time.Minute)) {
		t.Errorf("NotAfter should be approximately 365 days from now, got %v", cert.NotAfter)
	}
}

func TestGenerateServerCert(t *testing.T) {
	// First generate a CA
	ca, err := GenerateCA("Test CA", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// Generate server certificate
	dnsNames := []string{"localhost", "example.com", "*.example.com"}
	cert, key, err := ca.GenerateServerCert("server.example.com", dnsNames, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate server cert: %v", err)
	}

	if cert == nil {
		t.Fatal("Server certificate should not be nil")
	}

	if key == nil {
		t.Fatal("Server private key should not be nil")
	}

	// Verify certificate properties
	if cert.Subject.CommonName != "server.example.com" {
		t.Errorf("Expected CommonName 'server.example.com', got '%s'", cert.Subject.CommonName)
	}

	if cert.IsCA {
		t.Error("Server certificate should not be marked as CA")
	}

	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("Server certificate should have DigitalSignature key usage")
	}

	// Verify ExtKeyUsage contains ServerAuth
	hasServerAuth := false
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
			break
		}
	}
	if !hasServerAuth {
		t.Error("Server certificate should have ServerAuth extended key usage")
	}

	// Verify DNS names
	if len(cert.DNSNames) != len(dnsNames) {
		t.Errorf("Expected %d DNS names, got %d", len(dnsNames), len(cert.DNSNames))
	}

	for i, name := range dnsNames {
		if i >= len(cert.DNSNames) || cert.DNSNames[i] != name {
			t.Errorf("Expected DNS name %s at index %d, got %v", name, i, cert.DNSNames)
		}
	}

	// Verify the certificate is signed by the CA
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate)

	opts := x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "localhost",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	if _, err := cert.Verify(opts); err != nil {
		t.Errorf("Server certificate should be verifiable by CA: %v", err)
	}
}

func TestGenerateClientCert(t *testing.T) {
	// First generate a CA
	ca, err := GenerateCA("Test CA", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// Generate client certificate
	cert, key, err := ca.GenerateClientCert("client@example.com", 90*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate client cert: %v", err)
	}

	if cert == nil {
		t.Fatal("Client certificate should not be nil")
	}

	if key == nil {
		t.Fatal("Client private key should not be nil")
	}

	// Verify certificate properties
	if cert.Subject.CommonName != "client@example.com" {
		t.Errorf("Expected CommonName 'client@example.com', got '%s'", cert.Subject.CommonName)
	}

	if cert.IsCA {
		t.Error("Client certificate should not be marked as CA")
	}

	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("Client certificate should have DigitalSignature key usage")
	}

	// Verify ExtKeyUsage contains ClientAuth
	hasClientAuth := false
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageClientAuth {
			hasClientAuth = true
			break
		}
	}
	if !hasClientAuth {
		t.Error("Client certificate should have ClientAuth extended key usage")
	}

	// Verify the certificate is signed by the CA
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate)

	opts := x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	if _, err := cert.Verify(opts); err != nil {
		t.Errorf("Client certificate should be verifiable by CA: %v", err)
	}
}

func TestSaveAndLoadCertificatePEM(t *testing.T) {
	// Generate a CA for testing
	ca, err := GenerateCA("Test CA", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "test-cert-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Save certificate
	if err := SaveCertificatePEM(ca.Certificate, tmpFile.Name()); err != nil {
		t.Fatalf("Failed to save certificate: %v", err)
	}

	// Load certificate
	loaded, err := LoadCertificatePEM(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load certificate: %v", err)
	}

	// Verify loaded certificate matches original
	if loaded.Subject.CommonName != ca.Certificate.Subject.CommonName {
		t.Errorf("Loaded certificate CommonName mismatch: expected %s, got %s",
			ca.Certificate.Subject.CommonName, loaded.Subject.CommonName)
	}

	if !loaded.Equal(ca.Certificate) {
		t.Error("Loaded certificate should equal original certificate")
	}
}

func TestSaveAndLoadPrivateKeyPEM(t *testing.T) {
	// Generate a CA for testing
	ca, err := GenerateCA("Test CA", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "test-key-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Save private key
	if err := SavePrivateKeyPEM(ca.PrivateKey, tmpFile.Name()); err != nil {
		t.Fatalf("Failed to save private key: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to stat key file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected key file permissions 0600, got %o", info.Mode().Perm())
	}

	// Load private key
	loaded, err := LoadPrivateKeyPEM(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load private key: %v", err)
	}

	// Verify loaded key matches original (compare public keys)
	if !ca.PrivateKey.PublicKey.Equal(&loaded.PublicKey) {
		t.Error("Loaded private key should match original")
	}
}

func TestLoadCertificatePEM_InvalidFile(t *testing.T) {
	// Try to load non-existent file
	_, err := LoadCertificatePEM("/nonexistent/path/cert.pem")
	if err == nil {
		t.Error("Expected error when loading non-existent file")
	}

	// Try to load invalid PEM
	tmpFile, err := os.CreateTemp("", "invalid-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.WriteString("This is not a valid PEM file")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	_, err = LoadCertificatePEM(tmpFile.Name())
	if err == nil {
		t.Error("Expected error when loading invalid PEM")
	}
}

func TestLoadPrivateKeyPEM_InvalidFile(t *testing.T) {
	// Try to load non-existent file
	_, err := LoadPrivateKeyPEM("/nonexistent/path/key.pem")
	if err == nil {
		t.Error("Expected error when loading non-existent file")
	}

	// Try to load invalid PEM
	tmpFile, err := os.CreateTemp("", "invalid-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.WriteString("This is not a valid PEM file")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	_, err = LoadPrivateKeyPEM(tmpFile.Name())
	if err == nil {
		t.Error("Expected error when loading invalid PEM")
	}
}

func TestCertificateChain(t *testing.T) {
	// Test a complete certificate chain: CA -> Server Cert
	ca, err := GenerateCA("Root CA", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	serverCert, _, err := ca.GenerateServerCert("server.local", []string{"server.local"}, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate server cert: %v", err)
	}

	clientCert, _, err := ca.GenerateClientCert("client@local", 90*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate client cert: %v", err)
	}

	// Verify both certs are signed by the same CA
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate)

	// Verify server cert
	serverOpts := x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "server.local",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if _, err := serverCert.Verify(serverOpts); err != nil {
		t.Errorf("Server cert verification failed: %v", err)
	}

	// Verify client cert
	clientOpts := x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	if _, err := clientCert.Verify(clientOpts); err != nil {
		t.Errorf("Client cert verification failed: %v", err)
	}
}

func TestMultipleCAsIndependent(t *testing.T) {
	// Generate two independent CAs
	ca1, err := GenerateCA("CA 1", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate CA 1: %v", err)
	}

	ca2, err := GenerateCA("CA 2", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate CA 2: %v", err)
	}

	// Generate cert signed by CA1
	cert, _, err := ca1.GenerateServerCert("server.local", []string{"server.local"}, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}

	// Try to verify with CA2 (should fail)
	roots := x509.NewCertPool()
	roots.AddCert(ca2.Certificate)

	opts := x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "server.local",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	if _, err := cert.Verify(opts); err == nil {
		t.Error("Certificate signed by CA1 should not verify with CA2")
	}

	// Verify with correct CA (should succeed)
	roots2 := x509.NewCertPool()
	roots2.AddCert(ca1.Certificate)
	opts.Roots = roots2

	if _, err := cert.Verify(opts); err != nil {
		t.Errorf("Certificate should verify with correct CA: %v", err)
	}
}
