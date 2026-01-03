package statemgmt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ============================================================================
// X509 Module Tests
// ============================================================================

func TestNewX509Module(t *testing.T) {
	m := NewX509Module()

	if m.Name() != "x509" {
		t.Errorf("expected name 'x509', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestX509Module_Check_MissingPath(t *testing.T) {
	m := NewX509Module()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "x509",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "path parameter is required" {
		t.Errorf("expected path required error, got: %v", err)
	}
}

func TestX509Module_Check_NonExistent(t *testing.T) {
	m := NewX509Module()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "x509",
		State:  "absent",
		Parameters: map[string]interface{}{
			"path": "/nonexistent/cert.crt",
		},
	}

	result, err := m.Check(nil, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected Present to be false")
	}
	if !result.Matches {
		t.Error("expected Matches to be true for absent state on non-existent path")
	}
}

func TestX509Module_Test_Valid(t *testing.T) {
	m := NewX509Module()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "x509",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":        "/tmp/test.crt",
			"common_name": "test.example.com",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
}

func TestX509Module_Test_MissingCommonName(t *testing.T) {
	m := NewX509Module()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "x509",
		State:  "present",
		Parameters: map[string]interface{}{
			"path": "/tmp/test.crt",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if result.Success {
		t.Error("expected failure for missing common_name")
	}
}

func TestX509Module_Test_InvalidKeyType(t *testing.T) {
	m := NewX509Module()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "x509",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":        "/tmp/test.crt",
			"common_name": "test.example.com",
			"key_type":    "invalid",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if result.Success {
		t.Error("expected failure for invalid key_type")
	}
}

func TestX509Module_Integration_CreateAndRemove(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "test.crt")

	m := NewX509Module()

	// Create certificate
	createDecl := &StateDeclaration{
		ID:     "test-create",
		Module: "x509",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":         certPath,
			"common_name":  "test.example.com",
			"organization": "Test Org",
			"country":      "US",
			"validity_days": 30,
			"key_type":     "rsa",
			"key_size":     2048,
			"self_signed":  true,
		},
	}

	result, err := m.Apply(context.Background(), createDecl)
	if err != nil {
		t.Fatalf("Apply create failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
	if !result.Changed {
		t.Error("expected Changed to be true")
	}

	// Verify files exist
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Error("expected certificate file to exist")
	}
	keyPath := filepath.Join(tmpDir, "test.key")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("expected key file to exist")
	}

	// Check certificate
	checkResult, err := m.Check(context.Background(), createDecl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !checkResult.Present {
		t.Error("expected Present to be true")
	}
	if !checkResult.Matches {
		t.Error("expected Matches to be true")
	}
	if checkResult.Metadata["valid"] != true {
		t.Error("expected certificate to be valid")
	}

	// Remove certificate
	removeDecl := &StateDeclaration{
		ID:     "test-remove",
		Module: "x509",
		State:  "absent",
		Parameters: map[string]interface{}{
			"path": certPath,
		},
	}

	result, err = m.Apply(context.Background(), removeDecl)
	if err != nil {
		t.Fatalf("Apply remove failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
	if !result.Changed {
		t.Error("expected Changed to be true")
	}

	// Verify files removed
	if _, err := os.Stat(certPath); !os.IsNotExist(err) {
		t.Error("expected certificate file to be removed")
	}
}

func TestX509Module_Integration_ECDSA(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ecdsa.crt")

	m := NewX509Module()

	decl := &StateDeclaration{
		ID:     "test-ecdsa",
		Module: "x509",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":        certPath,
			"common_name": "ecdsa.example.com",
			"key_type":    "ecdsa",
			"key_size":    256,
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}

	// Verify ECDSA key
	keyPath := filepath.Join(tmpDir, "ecdsa.key")
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("Failed to read key: %v", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		t.Fatal("Failed to decode key PEM")
	}
	if block.Type != "EC PRIVATE KEY" {
		t.Errorf("expected EC PRIVATE KEY, got: %s", block.Type)
	}
}

func TestX509Module_Integration_Ed25519(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ed25519.crt")

	m := NewX509Module()

	decl := &StateDeclaration{
		ID:     "test-ed25519",
		Module: "x509",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":        certPath,
			"common_name": "ed25519.example.com",
			"key_type":    "ed25519",
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
}

func TestX509Module_Integration_WithSANs(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "san.crt")

	m := NewX509Module()

	decl := &StateDeclaration{
		ID:     "test-san",
		Module: "x509",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":        certPath,
			"common_name": "main.example.com",
			"san_names":   []interface{}{"alt1.example.com", "alt2.example.com"},
			"san_ips":     []interface{}{"10.0.0.1", "192.168.1.1"},
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}

	// Verify SANs in certificate
	certData, _ := os.ReadFile(certPath)
	block, _ := pem.Decode(certData)
	cert, _ := x509.ParseCertificate(block.Bytes)

	if len(cert.DNSNames) != 2 {
		t.Errorf("expected 2 DNS names, got %d", len(cert.DNSNames))
	}
	if len(cert.IPAddresses) != 2 {
		t.Errorf("expected 2 IP addresses, got %d", len(cert.IPAddresses))
	}
}

func TestX509Module_Check_ExistingCert(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "existing.crt")

	// Create a certificate manually
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "existing.example.com",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
	}

	certDER, _ := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	os.WriteFile(certPath, certPEM, 0644)

	m := NewX509Module()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "x509",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":        certPath,
			"common_name": "existing.example.com",
		},
	}

	result, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("expected Present to be true")
	}
	if result.Metadata["valid"] != true {
		t.Error("expected valid to be true")
	}
	if result.Metadata["status"] != "valid" {
		t.Errorf("expected status 'valid', got: %v", result.Metadata["status"])
	}
}

// ============================================================================
// CA Module Tests
// ============================================================================

func TestNewCAModule(t *testing.T) {
	m := NewCAModule()

	if m.Name() != "ca" {
		t.Errorf("expected name 'ca', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestCAModule_Check_MissingPath(t *testing.T) {
	m := NewCAModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "ca",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "path parameter is required" {
		t.Errorf("expected path required error, got: %v", err)
	}
}

func TestCAModule_Check_NonExistent(t *testing.T) {
	m := NewCAModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "ca",
		State:  "absent",
		Parameters: map[string]interface{}{
			"path": "/nonexistent/ca",
		},
	}

	result, err := m.Check(nil, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected Present to be false")
	}
	if !result.Matches {
		t.Error("expected Matches to be true for absent state")
	}
}

func TestCAModule_Test_Valid(t *testing.T) {
	m := NewCAModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "ca",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":        "/tmp/test-ca",
			"common_name": "Test CA",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
}

func TestCAModule_Test_MissingCommonName(t *testing.T) {
	m := NewCAModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "ca",
		State:  "present",
		Parameters: map[string]interface{}{
			"path": "/tmp/test-ca",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if result.Success {
		t.Error("expected failure for missing common_name")
	}
}

func TestCAModule_Integration_CreateAndRemove(t *testing.T) {
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "test-ca")

	m := NewCAModule()

	// Create CA
	createDecl := &StateDeclaration{
		ID:     "test-create",
		Module: "ca",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":         caPath,
			"common_name":  "Test Root CA",
			"organization": "Test Org",
			"country":      "US",
			"validity_days": 365,
		},
	}

	result, err := m.Apply(context.Background(), createDecl)
	if err != nil {
		t.Fatalf("Apply create failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
	if !result.Changed {
		t.Error("expected Changed to be true")
	}

	// Verify CA files exist
	certPath := filepath.Join(caPath, "ca.crt")
	keyPath := filepath.Join(caPath, "ca.key")
	serialPath := filepath.Join(caPath, "serial")
	indexPath := filepath.Join(caPath, "index.txt")

	for _, path := range []string{certPath, keyPath, serialPath, indexPath} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", path)
		}
	}

	// Check CA
	checkResult, err := m.Check(context.Background(), createDecl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !checkResult.Present {
		t.Error("expected Present to be true")
	}
	if checkResult.Metadata["is_ca"] != true {
		t.Error("expected is_ca to be true")
	}

	// Remove CA
	removeDecl := &StateDeclaration{
		ID:     "test-remove",
		Module: "ca",
		State:  "absent",
		Parameters: map[string]interface{}{
			"path": caPath,
		},
	}

	result, err = m.Apply(context.Background(), removeDecl)
	if err != nil {
		t.Fatalf("Apply remove failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
	if !result.Changed {
		t.Error("expected Changed to be true")
	}

	// Verify CA removed
	if _, err := os.Stat(caPath); !os.IsNotExist(err) {
		t.Error("expected CA directory to be removed")
	}
}

func TestCAModule_SignCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "test-ca")

	m := NewCAModule()

	// Create CA first
	createDecl := &StateDeclaration{
		ID:     "test-ca",
		Module: "ca",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":        caPath,
			"common_name": "Test CA",
		},
	}

	_, err := m.Apply(context.Background(), createDecl)
	if err != nil {
		t.Fatalf("Failed to create CA: %v", err)
	}

	// Generate a CSR
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	csrPEM, err := GenerateCSR(privateKey, "server.example.com", []string{"www.example.com"}, nil)
	if err != nil {
		t.Fatalf("Failed to generate CSR: %v", err)
	}

	// Sign the certificate
	certPEM, err := m.SignCertificate(caPath, csrPEM, 30)
	if err != nil {
		t.Fatalf("Failed to sign certificate: %v", err)
	}

	// Parse and verify the certificate
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("Failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	if cert.Subject.CommonName != "server.example.com" {
		t.Errorf("expected CN 'server.example.com', got: %s", cert.Subject.CommonName)
	}
	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != "www.example.com" {
		t.Errorf("expected DNS name 'www.example.com', got: %v", cert.DNSNames)
	}
}

// ============================================================================
// ACME Module Tests
// ============================================================================

func TestNewACMEModule(t *testing.T) {
	m := NewACMEModule()

	if m.Name() != "acme" {
		t.Errorf("expected name 'acme', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent", "renewed"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestACMEModule_Check_MissingPath(t *testing.T) {
	m := NewACMEModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "acme",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "path parameter is required" {
		t.Errorf("expected path required error, got: %v", err)
	}
}

func TestACMEModule_Check_MissingDomain(t *testing.T) {
	m := NewACMEModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "acme",
		State:  "present",
		Parameters: map[string]interface{}{
			"path": "/tmp/certs",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "domain parameter is required for state present" {
		t.Errorf("expected domain required error, got: %v", err)
	}
}

func TestACMEModule_Check_NonExistent(t *testing.T) {
	m := NewACMEModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "acme",
		State:  "absent",
		Parameters: map[string]interface{}{
			"path":   "/nonexistent/certs",
			"domain": "example.com",
		},
	}

	result, err := m.Check(nil, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected Present to be false")
	}
	if !result.Matches {
		t.Error("expected Matches to be true for absent state")
	}
}

func TestACMEModule_Test_Valid(t *testing.T) {
	m := NewACMEModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "acme",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":      "/tmp/certs",
			"domain":    "example.com",
			"challenge": "http-01",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
}

func TestACMEModule_Test_InvalidChallenge(t *testing.T) {
	m := NewACMEModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "acme",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":      "/tmp/certs",
			"domain":    "example.com",
			"challenge": "invalid",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if result.Success {
		t.Error("expected failure for invalid challenge type")
	}
}

func TestACMEModule_Check_ExistingCert(t *testing.T) {
	tmpDir := t.TempDir()
	domain := "test.example.com"
	certPath := filepath.Join(tmpDir, domain+".crt")
	keyPath := filepath.Join(tmpDir, domain+".key")

	// Create a certificate manually
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(90 * 24 * time.Hour), // 90 days
		DNSNames:    []string{domain},
		IPAddresses: []net.IP{net.ParseIP("10.0.0.1")},
	}

	certDER, _ := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	os.WriteFile(certPath, certPEM, 0644)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	os.WriteFile(keyPath, keyPEM, 0600)

	m := NewACMEModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "acme",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":   tmpDir,
			"domain": domain,
		},
	}

	result, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("expected Present to be true")
	}
	if result.Metadata["valid"] != true {
		t.Error("expected valid to be true")
	}
	if result.Metadata["needs_renewal"] != false {
		t.Error("expected needs_renewal to be false (cert has 90 days)")
	}
}

func TestACMEModule_Check_NeedsRenewal(t *testing.T) {
	tmpDir := t.TempDir()
	domain := "renew.example.com"
	certPath := filepath.Join(tmpDir, domain+".crt")
	keyPath := filepath.Join(tmpDir, domain+".key")

	// Create a certificate that expires in 10 days
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore: time.Now().Add(-80 * 24 * time.Hour),
		NotAfter:  time.Now().Add(10 * 24 * time.Hour), // 10 days left
	}

	certDER, _ := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	os.WriteFile(certPath, certPEM, 0644)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	os.WriteFile(keyPath, keyPEM, 0600)

	m := NewACMEModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "acme",
		State:  "renewed",
		Parameters: map[string]interface{}{
			"path":       tmpDir,
			"domain":     domain,
			"renew_days": 30,
		},
	}

	result, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("expected Present to be true")
	}
	if result.Metadata["needs_renewal"] != true {
		t.Error("expected needs_renewal to be true (10 days < 30 day threshold)")
	}
	if result.Matches {
		t.Error("expected Matches to be false (needs renewal)")
	}
}

func TestACMEModule_Apply_Absent(t *testing.T) {
	tmpDir := t.TempDir()
	domain := "remove.example.com"
	certPath := filepath.Join(tmpDir, domain+".crt")
	keyPath := filepath.Join(tmpDir, domain+".key")

	// Create dummy files
	os.WriteFile(certPath, []byte("cert"), 0644)
	os.WriteFile(keyPath, []byte("key"), 0600)

	m := NewACMEModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "acme",
		State:  "absent",
		Parameters: map[string]interface{}{
			"path":   tmpDir,
			"domain": domain,
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
	if !result.Changed {
		t.Error("expected Changed to be true")
	}

	// Verify files removed
	if _, err := os.Stat(certPath); !os.IsNotExist(err) {
		t.Error("expected certificate to be removed")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Error("expected key to be removed")
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestGenerateCSR(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	csrPEM, err := GenerateCSR(
		privateKey,
		"test.example.com",
		[]string{"www.example.com", "mail.example.com"},
		[]net.IP{net.ParseIP("10.0.0.1")},
	)
	if err != nil {
		t.Fatalf("GenerateCSR failed: %v", err)
	}

	// Parse the CSR
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Fatal("expected CERTIFICATE REQUEST PEM block")
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse CSR: %v", err)
	}

	if csr.Subject.CommonName != "test.example.com" {
		t.Errorf("expected CN 'test.example.com', got: %s", csr.Subject.CommonName)
	}
	if len(csr.DNSNames) != 2 {
		t.Errorf("expected 2 DNS names, got: %d", len(csr.DNSNames))
	}
	if len(csr.IPAddresses) != 1 {
		t.Errorf("expected 1 IP address, got: %d", len(csr.IPAddresses))
	}
}

func TestFormatIPs(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("10.0.0.1"),
		net.ParseIP("192.168.1.1"),
		net.ParseIP("::1"),
	}

	result := formatIPs(ips)

	if len(result) != 3 {
		t.Errorf("expected 3 IPs, got: %d", len(result))
	}
	if result[0] != "10.0.0.1" {
		t.Errorf("expected '10.0.0.1', got: %s", result[0])
	}
}
