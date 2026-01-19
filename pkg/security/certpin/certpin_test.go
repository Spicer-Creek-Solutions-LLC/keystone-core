package certpin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// generateTestCert creates a self-signed test certificate
func generateTestCert(commonName string) (*x509.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(certDER)
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultMode != ModeEnforce {
		t.Errorf("DefaultMode = %s, want enforce", cfg.DefaultMode)
	}
	if cfg.CacheExpiry != 5*time.Minute {
		t.Errorf("CacheExpiry = %v, want 5m", cfg.CacheExpiry)
	}
	if cfg.AllowSystemRoots {
		t.Error("AllowSystemRoots should be false by default")
	}
}

func TestNewPinner(t *testing.T) {
	// With nil config
	p := NewPinner(nil)
	if p == nil {
		t.Fatal("NewPinner returned nil")
	}
	if p.config.DefaultMode != ModeEnforce {
		t.Error("Should use default config when nil")
	}

	// With custom config
	cfg := &Config{DefaultMode: ModeReportOnly}
	p = NewPinner(cfg)
	if p.config.DefaultMode != ModeReportOnly {
		t.Error("Should use provided config")
	}
}

func TestPinner_AddService(t *testing.T) {
	p := NewPinner(nil)

	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com", "api.example.com"},
		Mode:  ModeEnforce,
	}

	p.AddService(svc)

	// Check both hosts are indexed
	if p.GetService("example.com") == nil {
		t.Error("Should find service for example.com")
	}
	if p.GetService("api.example.com") == nil {
		t.Error("Should find service for api.example.com")
	}
}

func TestPinner_RemoveService(t *testing.T) {
	p := NewPinner(nil)

	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeEnforce,
	}
	p.AddService(svc)

	// Verify added
	if p.GetService("example.com") == nil {
		t.Fatal("Service should exist before removal")
	}

	// Remove
	p.RemoveService("test-service")

	// Verify removed
	if p.GetService("example.com") != nil {
		t.Error("Service should be removed")
	}
}

func TestPinner_GetService_Subdomain(t *testing.T) {
	p := NewPinner(nil)

	svc := &ServiceConfig{
		Name:              "test-service",
		Hosts:             []string{"example.com"},
		Mode:              ModeEnforce,
		IncludeSubdomains: true,
	}
	p.AddService(svc)

	// Exact match
	if p.GetService("example.com") == nil {
		t.Error("Should find exact match")
	}

	// Subdomain match
	if p.GetService("api.example.com") == nil {
		t.Error("Should find subdomain match")
	}

	// Deep subdomain
	if p.GetService("deep.api.example.com") == nil {
		t.Error("Should find deep subdomain match")
	}

	// Different domain
	if p.GetService("other.com") != nil {
		t.Error("Should not match different domain")
	}
}

func TestPinner_Verify_NoService(t *testing.T) {
	p := NewPinner(nil)

	cert, err := generateTestCert("unknown.com")
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}

	// No service configured - should pass
	err = p.Verify("unknown.com", []*x509.Certificate{cert})
	if err != nil {
		t.Errorf("Should pass when no service configured: %v", err)
	}
}

func TestPinner_Verify_Disabled(t *testing.T) {
	p := NewPinner(nil)

	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeDisabled,
	}
	p.AddService(svc)

	cert, err := generateTestCert("example.com")
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}

	// Disabled mode - should pass
	err = p.Verify("example.com", []*x509.Certificate{cert})
	if err != nil {
		t.Errorf("Should pass when disabled: %v", err)
	}
}

func TestPinner_Verify_Success(t *testing.T) {
	cert, err := generateTestCert("example.com")
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}

	pin := NewPin(cert, PinTypeSPKI, "test pin")

	p := NewPinner(nil)
	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeEnforce,
		Pins:  []*Pin{pin},
	}
	p.AddService(svc)

	err = p.Verify("example.com", []*x509.Certificate{cert})
	if err != nil {
		t.Errorf("Should pass with matching pin: %v", err)
	}
}

func TestPinner_Verify_Failure(t *testing.T) {
	cert1, _ := generateTestCert("example.com")
	cert2, _ := generateTestCert("example.com")

	// Pin for cert1
	pin := NewPin(cert1, PinTypeSPKI, "test pin")

	p := NewPinner(nil)
	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeEnforce,
		Pins:  []*Pin{pin},
	}
	p.AddService(svc)

	// Verify with cert2 (different key)
	err := p.Verify("example.com", []*x509.Certificate{cert2})
	if err == nil {
		t.Error("Should fail with non-matching pin")
	}
}

func TestPinner_Verify_ReportOnly(t *testing.T) {
	cert1, _ := generateTestCert("example.com")
	cert2, _ := generateTestCert("example.com")

	pin := NewPin(cert1, PinTypeSPKI, "test pin")

	var violationReported bool
	cfg := &Config{
		OnViolation: func(report *ViolationReport) {
			violationReported = true
		},
	}

	p := NewPinner(cfg)
	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeReportOnly,
		Pins:  []*Pin{pin},
	}
	p.AddService(svc)

	// Should not error in report-only mode
	err := p.Verify("example.com", []*x509.Certificate{cert2})
	if err != nil {
		t.Errorf("Should not error in report-only mode: %v", err)
	}

	if !violationReported {
		t.Error("Violation should be reported")
	}
}

func TestPinner_Verify_ExpiredPin(t *testing.T) {
	cert, _ := generateTestCert("example.com")

	expired := time.Now().Add(-1 * time.Hour)
	pin := NewPin(cert, PinTypeSPKI, "expired pin")
	pin.ExpiresAt = &expired

	p := NewPinner(nil)
	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeEnforce,
		Pins:  []*Pin{pin},
	}
	p.AddService(svc)

	err := p.Verify("example.com", []*x509.Certificate{cert})
	if err == nil {
		t.Error("Should fail with expired pin")
	}
}

func TestPinner_Verify_BackupPin(t *testing.T) {
	cert, _ := generateTestCert("example.com")
	backupCert, _ := generateTestCert("backup.example.com")

	primaryPin := NewPin(backupCert, PinTypeSPKI, "primary pin")
	backupPin := NewPin(cert, PinTypeSPKI, "backup pin")
	backupPin.IsBackup = true

	p := NewPinner(nil)
	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeEnforce,
		Pins:  []*Pin{primaryPin, backupPin},
	}
	p.AddService(svc)

	// Should match backup pin
	err := p.Verify("example.com", []*x509.Certificate{cert})
	if err != nil {
		t.Errorf("Should pass with backup pin: %v", err)
	}
}

func TestPinner_Cache(t *testing.T) {
	cert, _ := generateTestCert("example.com")
	pin := NewPin(cert, PinTypeSPKI, "test pin")

	p := NewPinner(&Config{CacheExpiry: 1 * time.Hour})
	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeEnforce,
		Pins:  []*Pin{pin},
	}
	p.AddService(svc)

	// First verification
	err := p.Verify("example.com", []*x509.Certificate{cert})
	if err != nil {
		t.Fatalf("First verification failed: %v", err)
	}

	stats := p.Stats().Snapshot()
	if stats.CacheHits != 0 {
		t.Error("Should have 0 cache hits on first verification")
	}

	// Second verification should use cache
	err = p.Verify("example.com", []*x509.Certificate{cert})
	if err != nil {
		t.Fatalf("Second verification failed: %v", err)
	}

	stats = p.Stats().Snapshot()
	if stats.CacheHits != 1 {
		t.Errorf("Should have 1 cache hit, got %d", stats.CacheHits)
	}
}

func TestPinner_ClearCache(t *testing.T) {
	cert, _ := generateTestCert("example.com")
	pin := NewPin(cert, PinTypeSPKI, "test pin")

	p := NewPinner(&Config{CacheExpiry: 1 * time.Hour})
	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeEnforce,
		Pins:  []*Pin{pin},
	}
	p.AddService(svc)

	// First verification
	p.Verify("example.com", []*x509.Certificate{cert})

	// Clear cache
	p.ClearCache()

	// Should not get cache hit
	p.Verify("example.com", []*x509.Certificate{cert})

	stats := p.Stats().Snapshot()
	if stats.CacheHits != 0 {
		t.Error("Should have 0 cache hits after clear")
	}
}

func TestComputeSPKIPin(t *testing.T) {
	cert, _ := generateTestCert("example.com")

	pin := ComputeSPKIPin(cert)
	if pin == "" {
		t.Error("Pin should not be empty")
	}

	// Should be consistent
	pin2 := ComputeSPKIPin(cert)
	if pin != pin2 {
		t.Error("Pin should be deterministic")
	}
}

func TestComputeCertificatePin(t *testing.T) {
	cert, _ := generateTestCert("example.com")

	pin := ComputeCertificatePin(cert)
	if pin == "" {
		t.Error("Pin should not be empty")
	}

	// SPKI pin should be different from certificate pin
	spkiPin := ComputeSPKIPin(cert)
	if pin == spkiPin {
		t.Error("Certificate pin should differ from SPKI pin")
	}
}

func TestNewPin(t *testing.T) {
	cert, _ := generateTestCert("example.com")

	pin := NewPin(cert, PinTypeSPKI, "test comment")

	if pin.Hash == "" {
		t.Error("Hash should not be empty")
	}
	if pin.Type != PinTypeSPKI {
		t.Errorf("Type = %s, want spki", pin.Type)
	}
	if pin.Comment != "test comment" {
		t.Errorf("Comment = %s, want 'test comment'", pin.Comment)
	}
}

func TestNewPinFromHash(t *testing.T) {
	pin := NewPinFromHash("abc123", PinTypeSPKI, "test comment")

	if pin.Hash != "abc123" {
		t.Errorf("Hash = %s, want abc123", pin.Hash)
	}
	if pin.Type != PinTypeSPKI {
		t.Errorf("Type = %s, want spki", pin.Type)
	}
}

func TestPinner_VerifyCallback(t *testing.T) {
	cert, _ := generateTestCert("example.com")
	pin := NewPin(cert, PinTypeSPKI, "test pin")

	p := NewPinner(nil)
	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeEnforce,
		Pins:  []*Pin{pin},
	}
	p.AddService(svc)

	callback := p.VerifyCallback("example.com")

	// Test with verified chains
	err := callback(nil, [][]*x509.Certificate{{cert}})
	if err != nil {
		t.Errorf("Callback should pass: %v", err)
	}

	// Test with raw certs
	err = callback([][]byte{cert.Raw}, nil)
	if err != nil {
		t.Errorf("Callback should pass with raw certs: %v", err)
	}
}

func TestPinner_TLSConfig(t *testing.T) {
	p := NewPinner(nil)

	tlsConfig := p.TLSConfig("example.com")

	if tlsConfig.ServerName != "example.com" {
		t.Errorf("ServerName = %s, want example.com", tlsConfig.ServerName)
	}
	if tlsConfig.VerifyPeerCertificate == nil {
		t.Error("VerifyPeerCertificate should be set")
	}
	if tlsConfig.MinVersion != 0x0303 { // TLS 1.2
		t.Errorf("MinVersion = %x, want TLS 1.2", tlsConfig.MinVersion)
	}
}

func TestStats_RecordVerification(t *testing.T) {
	s := NewStats()

	s.RecordVerification("example.com", true, false)
	s.RecordVerification("example.com", true, true)
	s.RecordVerification("other.com", false, false)

	if s.TotalVerifications != 3 {
		t.Errorf("TotalVerifications = %d, want 3", s.TotalVerifications)
	}
	if s.SuccessfulVerifications != 2 {
		t.Errorf("SuccessfulVerifications = %d, want 2", s.SuccessfulVerifications)
	}
	if s.FailedVerifications != 1 {
		t.Errorf("FailedVerifications = %d, want 1", s.FailedVerifications)
	}
	if s.CacheHits != 1 {
		t.Errorf("CacheHits = %d, want 1", s.CacheHits)
	}
	if s.CacheMisses != 2 {
		t.Errorf("CacheMisses = %d, want 2", s.CacheMisses)
	}

	hostStats := s.ByHost["example.com"]
	if hostStats.Verifications != 2 {
		t.Errorf("example.com verifications = %d, want 2", hostStats.Verifications)
	}
}

func TestStats_RecordViolation(t *testing.T) {
	s := NewStats()

	s.RecordViolation("example.com")
	s.RecordViolation("example.com")
	s.RecordViolation("other.com")

	if s.Violations != 3 {
		t.Errorf("Violations = %d, want 3", s.Violations)
	}

	if s.ByHost["example.com"].Violations != 2 {
		t.Errorf("example.com violations = %d, want 2", s.ByHost["example.com"].Violations)
	}
}

func TestStats_Snapshot(t *testing.T) {
	s := NewStats()

	s.RecordVerification("example.com", true, false)
	s.RecordViolation("example.com")

	snapshot := s.Snapshot()

	// Modify original
	s.RecordVerification("other.com", true, false)

	// Snapshot should be unchanged
	if snapshot.TotalVerifications != 1 {
		t.Error("Snapshot should be independent copy")
	}
	if len(snapshot.ByHost) != 1 {
		t.Error("Snapshot should have 1 host")
	}
}

func TestViolationReport(t *testing.T) {
	cert1, _ := generateTestCert("example.com")
	cert2, _ := generateTestCert("example.com")

	pin := NewPin(cert1, PinTypeSPKI, "test pin")

	var receivedReport *ViolationReport
	cfg := &Config{
		OnViolation: func(report *ViolationReport) {
			receivedReport = report
		},
	}

	p := NewPinner(cfg)
	svc := &ServiceConfig{
		Name:              "test-service",
		Hosts:             []string{"example.com"},
		Mode:              ModeReportOnly,
		Pins:              []*Pin{pin},
		IncludeSubdomains: true,
	}
	p.AddService(svc)

	p.Verify("example.com", []*x509.Certificate{cert2})

	if receivedReport == nil {
		t.Fatal("Should receive violation report")
	}
	if receivedReport.Hostname != "example.com" {
		t.Errorf("Hostname = %s, want example.com", receivedReport.Hostname)
	}
	if receivedReport.EnforcementMode != ModeReportOnly {
		t.Errorf("EnforcementMode = %s, want report_only", receivedReport.EnforcementMode)
	}
	if !receivedReport.IncludeSubdomains {
		t.Error("IncludeSubdomains should be true")
	}
	if len(receivedReport.ServedCertificateChain) != 1 {
		t.Errorf("Should have 1 served cert, got %d", len(receivedReport.ServedCertificateChain))
	}
	if len(receivedReport.KnownPins) != 1 {
		t.Errorf("Should have 1 known pin, got %d", len(receivedReport.KnownPins))
	}
}

func TestPinner_CertificatePinType(t *testing.T) {
	cert, _ := generateTestCert("example.com")

	pin := NewPin(cert, PinTypeCertificate, "cert pin")

	p := NewPinner(nil)
	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"example.com"},
		Mode:  ModeEnforce,
		Pins:  []*Pin{pin},
	}
	p.AddService(svc)

	err := p.Verify("example.com", []*x509.Certificate{cert})
	if err != nil {
		t.Errorf("Should pass with certificate pin: %v", err)
	}
}

func TestPinner_ChainVerification(t *testing.T) {
	// Create a chain: leaf <- intermediate <- root
	leafCert, _ := generateTestCert("leaf.example.com")
	intermediateCert, _ := generateTestCert("intermediate.example.com")
	rootCert, _ := generateTestCert("root.example.com")

	// Pin the intermediate
	pin := NewPin(intermediateCert, PinTypeSPKI, "intermediate pin")

	p := NewPinner(nil)
	svc := &ServiceConfig{
		Name:  "test-service",
		Hosts: []string{"leaf.example.com"},
		Mode:  ModeEnforce,
		Pins:  []*Pin{pin},
	}
	p.AddService(svc)

	// Should pass because intermediate is in the chain
	chain := []*x509.Certificate{leafCert, intermediateCert, rootCert}
	err := p.Verify("leaf.example.com", chain)
	if err != nil {
		t.Errorf("Should pass with intermediate pin in chain: %v", err)
	}
}
