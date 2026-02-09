package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/identity"
)

func TestIdentityCommandExists(t *testing.T) {
	cmd := newRootCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "identity" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected identity subcommand not found")
	}
}

func TestIdentitySubcommands(t *testing.T) {
	cmd := newIdentityCmd()

	expected := []string{"show", "renew", "status"}
	for _, name := range expected {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected identity subcommand %q not found", name)
		}
	}
}

func TestIdentityCommandHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "SPIFFE") {
		t.Errorf("expected help to contain 'SPIFFE', got: %s", output)
	}
	if !strings.Contains(output, "show") {
		t.Errorf("expected help to list 'show' subcommand, got: %s", output)
	}
	if !strings.Contains(output, "renew") {
		t.Errorf("expected help to list 'renew' subcommand, got: %s", output)
	}
	if !strings.Contains(output, "status") {
		t.Errorf("expected help to list 'status' subcommand, got: %s", output)
	}
}

func TestIdentityShowHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "show", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity show --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "SPIFFE") {
		t.Errorf("expected help to contain 'SPIFFE', got: %s", output)
	}
	if !strings.Contains(output, "data-dir") {
		t.Errorf("expected help to contain 'data-dir' flag, got: %s", output)
	}
}

func TestIdentityRenewHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "renew", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity renew --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "renewal") {
		t.Errorf("expected help to contain 'renewal', got: %s", output)
	}
	if !strings.Contains(output, "timeout") {
		t.Errorf("expected help to contain 'timeout' flag, got: %s", output)
	}
}

func TestIdentityStatusHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "status", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity status --help failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "health") {
		t.Errorf("expected help to contain 'health', got: %s", output)
	}
}

func TestIdentityShowWithValidCert(t *testing.T) {
	dir := t.TempDir()
	writeSelfSignedSVID(t, dir, "kscore.local", "/agent/test-agent-1", time.Now(), time.Now().Add(1*time.Hour))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "show", "--data-dir", dir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity show failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "spiffe://kscore.local/agent/test-agent-1") {
		t.Errorf("expected output to contain SPIFFE ID, got: %s", output)
	}
	if !strings.Contains(output, "Trust Domain: kscore.local") {
		t.Errorf("expected output to contain trust domain, got: %s", output)
	}
	if !strings.Contains(output, "/agent/test-agent-1") {
		t.Errorf("expected output to contain path, got: %s", output)
	}
	if !strings.Contains(output, "TTL:") {
		t.Errorf("expected output to contain TTL, got: %s", output)
	}
}

func TestIdentityShowMissingCert(t *testing.T) {
	dir := t.TempDir()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"identity", "show", "--data-dir", dir})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when cert is missing")
	}
	if !strings.Contains(err.Error(), "SVID") {
		t.Errorf("expected error to mention SVID, got: %v", err)
	}
}

func TestIdentityStatusValid(t *testing.T) {
	dir := t.TempDir()
	writeSelfSignedSVID(t, dir, "kscore.local", "/agent/web-1", time.Now(), time.Now().Add(2*time.Hour))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "status", "--data-dir", dir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity status failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Status:       VALID") {
		t.Errorf("expected VALID status, got: %s", output)
	}
	if !strings.Contains(output, "spiffe://kscore.local/agent/web-1") {
		t.Errorf("expected SPIFFE ID in output, got: %s", output)
	}
}

func TestIdentityStatusExpired(t *testing.T) {
	dir := t.TempDir()
	writeSelfSignedSVID(t, dir, "kscore.local", "/agent/old-agent", time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "status", "--data-dir", dir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity status failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Status:       EXPIRED") {
		t.Errorf("expected EXPIRED status, got: %s", output)
	}
}

func TestIdentityStatusExpiringSoon(t *testing.T) {
	dir := t.TempDir()
	// SVID with 2h lifetime, but 1h10m has already passed (past 50% threshold)
	writeSelfSignedSVID(t, dir, "kscore.local", "/agent/expiring", time.Now().Add(-70*time.Minute), time.Now().Add(50*time.Minute))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "status", "--data-dir", dir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity status failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "EXPIRING SOON") {
		t.Errorf("expected EXPIRING SOON status, got: %s", output)
	}
}

func TestIdentityStatusNoIdentity(t *testing.T) {
	dir := t.TempDir()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "status", "--data-dir", dir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity status failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NO IDENTITY") {
		t.Errorf("expected NO IDENTITY status, got: %s", output)
	}
}

func TestIdentityStatusCritical(t *testing.T) {
	dir := t.TempDir()
	// SVID with 10h lifetime, only 30m remaining (~5%)
	writeSelfSignedSVID(t, dir, "kscore.local", "/agent/critical", time.Now().Add(-570*time.Minute), time.Now().Add(30*time.Minute))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "status", "--data-dir", dir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity status failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "CRITICAL") {
		t.Errorf("expected CRITICAL status, got: %s", output)
	}
}

func TestIdentityShowExpiredCert(t *testing.T) {
	dir := t.TempDir()
	writeSelfSignedSVID(t, dir, "example.com", "/agent/expired-agent", time.Now().Add(-2*time.Hour), time.Now().Add(-1*time.Hour))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "show", "--data-dir", dir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity show failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "expired") {
		t.Errorf("expected output to indicate expired, got: %s", output)
	}
}

func TestIdentityStatusTrustBundlePresent(t *testing.T) {
	dir := t.TempDir()
	writeSelfSignedSVID(t, dir, "kscore.local", "/agent/bundle-test", time.Now(), time.Now().Add(1*time.Hour))

	// Write a dummy trust bundle file
	bundlePath := filepath.Join(dir, trustBundleFile)
	if err := os.WriteFile(bundlePath, []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "status", "--data-dir", dir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity status failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Trust Bundle: present") {
		t.Errorf("expected trust bundle present, got: %s", output)
	}
}

func TestIdentityStatusTrustBundleMissing(t *testing.T) {
	dir := t.TempDir()
	writeSelfSignedSVID(t, dir, "kscore.local", "/agent/no-bundle", time.Now(), time.Now().Add(1*time.Hour))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"identity", "status", "--data-dir", dir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("identity status failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Trust Bundle: missing") {
		t.Errorf("expected trust bundle missing, got: %s", output)
	}
}

func TestLoadSVIDFromDisk(t *testing.T) {
	dir := t.TempDir()
	writeSelfSignedSVID(t, dir, "test.local", "/agent/load-test", time.Now(), time.Now().Add(1*time.Hour))

	svid, err := loadSVIDFromDisk(dir)
	if err != nil {
		t.Fatalf("loadSVIDFromDisk failed: %v", err)
	}

	if svid.SPIFFEID.TrustDomain != "test.local" {
		t.Errorf("expected trust domain 'test.local', got %q", svid.SPIFFEID.TrustDomain)
	}
	if svid.SPIFFEID.Path != "/agent/load-test" {
		t.Errorf("expected path '/agent/load-test', got %q", svid.SPIFFEID.Path)
	}
	if len(svid.Certificates) == 0 {
		t.Error("expected at least one certificate")
	}
}

func TestLoadSVIDFromDiskNoCert(t *testing.T) {
	dir := t.TempDir()

	_, err := loadSVIDFromDisk(dir)
	if err == nil {
		t.Fatal("expected error when cert file is missing")
	}
}

func TestLoadSVIDFromDiskInvalidPEM(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, svidCertFile)
	if err := os.WriteFile(certPath, []byte("not a PEM"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadSVIDFromDisk(dir)
	if err == nil {
		t.Fatal("expected error with invalid PEM data")
	}
}

func TestExtractAgentID(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "standard agent path",
			path:     "/agent/web-server-1",
			expected: "web-server-1",
		},
		{
			name:     "short path",
			path:     "/svc",
			expected: "/svc",
		},
		{
			name:     "agent prefix only",
			path:     "/agent/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := extractAgentID(identity.SPIFFEID{TrustDomain: "test.local", Path: tt.path})
			if id != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, id)
			}
		})
	}
}

func TestGetAttestationTypeDefault(t *testing.T) {
	original := os.Getenv("KSCORE_ATTESTATION_TYPE")
	os.Unsetenv("KSCORE_ATTESTATION_TYPE")
	defer func() {
		if original != "" {
			os.Setenv("KSCORE_ATTESTATION_TYPE", original)
		}
	}()

	result := getAttestationType()
	if result != "join_token" {
		t.Errorf("expected default attestation type 'join_token', got %q", result)
	}
}

func TestGetAttestationTypeFromEnv(t *testing.T) {
	original := os.Getenv("KSCORE_ATTESTATION_TYPE")
	os.Setenv("KSCORE_ATTESTATION_TYPE", "aws_iid")
	defer func() {
		if original != "" {
			os.Setenv("KSCORE_ATTESTATION_TYPE", original)
		} else {
			os.Unsetenv("KSCORE_ATTESTATION_TYPE")
		}
	}()

	result := getAttestationType()
	if result != "aws_iid" {
		t.Errorf("expected attestation type 'aws_iid', got %q", result)
	}
}

func TestIdentityRenewMissingCert(t *testing.T) {
	dir := t.TempDir()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"identity", "renew", "--data-dir", dir})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when cert is missing for renew")
	}
}

func writeSelfSignedSVID(t *testing.T, dir, trustDomain, path string, notBefore, notAfter time.Time) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	spiffeURI := &url.URL{
		Scheme: "spiffe",
		Host:   trustDomain,
		Path:   path,
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-agent",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{spiffeURI},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	certPath := filepath.Join(dir, svidCertFile)
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
}
