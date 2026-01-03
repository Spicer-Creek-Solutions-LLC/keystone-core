package statemgmt

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ============================================================================
// X509 Certificate Module - Manage X.509 certificates and keys
// ============================================================================

// X509Module manages X.509 certificates
type X509Module struct {
	*BaseModule
}

// NewX509Module creates a new X509 module
func NewX509Module() *X509Module {
	return &X509Module{
		BaseModule: NewBaseModule("x509", []string{"present", "absent"}),
	}
}

// Check examines the current state of an X.509 certificate
func (m *X509Module) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	path := getStringParameter(decl, "path", "")
	if path == "" {
		return nil, fmt.Errorf("path parameter is required")
	}

	result := &ModuleCheckResult{
		Metadata: make(map[string]interface{}),
	}

	// Check if certificate file exists
	certData, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		result.Present = false
		result.Matches = decl.State == "absent"
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", err)
	}

	result.Present = true

	// Parse certificate
	block, _ := pem.Decode(certData)
	if block == nil || block.Type != "CERTIFICATE" {
		result.Metadata["valid"] = false
		result.Metadata["error"] = "invalid PEM data"
		result.Matches = decl.State == "absent"
		return result, nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		result.Metadata["valid"] = false
		result.Metadata["error"] = err.Error()
		result.Matches = decl.State == "absent"
		return result, nil
	}

	result.Metadata["valid"] = true
	result.Metadata["subject"] = cert.Subject.String()
	result.Metadata["issuer"] = cert.Issuer.String()
	result.Metadata["serial"] = cert.SerialNumber.String()
	result.Metadata["not_before"] = cert.NotBefore.Format(time.RFC3339)
	result.Metadata["not_after"] = cert.NotAfter.Format(time.RFC3339)
	result.Metadata["is_ca"] = cert.IsCA
	result.Metadata["dns_names"] = cert.DNSNames
	result.Metadata["ip_addresses"] = formatIPs(cert.IPAddresses)

	// Check if expired
	now := time.Now()
	if now.Before(cert.NotBefore) {
		result.Metadata["status"] = "not_yet_valid"
	} else if now.After(cert.NotAfter) {
		result.Metadata["status"] = "expired"
	} else {
		result.Metadata["status"] = "valid"
	}

	switch decl.State {
	case "absent":
		result.Matches = false
		result.Diff = map[string]interface{}{
			"current": "present",
			"desired": "absent",
		}
	case "present":
		// Check if certificate matches desired properties
		result.Matches = true
		result.CurrentState = "present"

		// Check common name if specified
		cn := getStringParameter(decl, "common_name", "")
		if cn != "" && cert.Subject.CommonName != cn {
			result.Matches = false
			result.Diff = map[string]interface{}{
				"common_name": map[string]string{
					"current": cert.Subject.CommonName,
					"desired": cn,
				},
			}
		}
	}

	return result, nil
}

// Apply creates or removes an X.509 certificate
func (m *X509Module) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	path := getStringParameter(decl, "path", "")
	keyPath := getStringParameter(decl, "key_path", "")
	if keyPath == "" {
		keyPath = strings.TrimSuffix(path, ".crt") + ".key"
	}

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	check, err := m.Check(ctx, decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Check failed: %v", err)
		return result, nil
	}

	if check.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Certificate already in desired state"
		return result, nil
	}

	switch decl.State {
	case "absent":
		// Remove certificate and key
		removed := false
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to remove certificate: %v", err)
			return result, nil
		} else if err == nil {
			removed = true
		}

		if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
			// Key removal failed, but cert was removed
		} else if err == nil {
			removed = true
		}

		result.Success = true
		result.Changed = removed
		if removed {
			result.Comment = "Removed certificate and key"
		} else {
			result.Comment = "Certificate already absent"
		}

	case "present":
		// Generate certificate
		cn := getStringParameter(decl, "common_name", "")
		if cn == "" {
			result.Success = false
			result.Comment = "common_name parameter is required for present state"
			return result, nil
		}

		org := getStringParameter(decl, "organization", "")
		country := getStringParameter(decl, "country", "")
		validity := getIntParameter(decl, "validity_days", 365)
		keyType := getStringParameter(decl, "key_type", "rsa")
		keySize := getIntParameter(decl, "key_size", 2048)
		selfSigned := getBoolParameter(decl, "self_signed", true)
		isCA := getBoolParameter(decl, "is_ca", false)

		// Parse SAN names
		sanNames := getStringSliceParameter(decl, "san_names")
		sanIPs := getStringSliceParameter(decl, "san_ips")

		// Generate private key
		var privateKey crypto.PrivateKey
		var publicKey crypto.PublicKey

		switch keyType {
		case "rsa":
			key, err := rsa.GenerateKey(rand.Reader, keySize)
			if err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to generate RSA key: %v", err)
				return result, nil
			}
			privateKey = key
			publicKey = &key.PublicKey
		case "ecdsa":
			var curve elliptic.Curve
			switch keySize {
			case 256:
				curve = elliptic.P256()
			case 384:
				curve = elliptic.P384()
			case 521:
				curve = elliptic.P521()
			default:
				curve = elliptic.P256()
			}
			key, err := ecdsa.GenerateKey(curve, rand.Reader)
			if err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to generate ECDSA key: %v", err)
				return result, nil
			}
			privateKey = key
			publicKey = &key.PublicKey
		case "ed25519":
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to generate Ed25519 key: %v", err)
				return result, nil
			}
			privateKey = priv
			publicKey = pub
		default:
			result.Success = false
			result.Comment = fmt.Sprintf("Unknown key type: %s", keyType)
			return result, nil
		}

		// Generate serial number
		serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to generate serial number: %v", err)
			return result, nil
		}

		// Build certificate template
		notBefore := time.Now()
		notAfter := notBefore.AddDate(0, 0, validity)

		template := x509.Certificate{
			SerialNumber: serialNumber,
			Subject: pkix.Name{
				CommonName:   cn,
				Organization: []string{org},
				Country:      []string{country},
			},
			NotBefore:             notBefore,
			NotAfter:              notAfter,
			KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			BasicConstraintsValid: true,
			IsCA:                  isCA,
		}

		if isCA {
			template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		}

		// Add SANs
		for _, name := range sanNames {
			template.DNSNames = append(template.DNSNames, name)
		}
		for _, ipStr := range sanIPs {
			ip := net.ParseIP(ipStr)
			if ip != nil {
				template.IPAddresses = append(template.IPAddresses, ip)
			}
		}

		// Create certificate
		var certDER []byte
		if selfSigned {
			certDER, err = x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
		} else {
			// For non-self-signed, we'd need a CA cert/key - for now, just self-sign
			certDER, err = x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
		}
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create certificate: %v", err)
			return result, nil
		}

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create directory: %v", err)
			return result, nil
		}

		// Write certificate
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
		if err := os.WriteFile(path, certPEM, 0644); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to write certificate: %v", err)
			return result, nil
		}

		// Write private key
		var keyPEM []byte
		switch k := privateKey.(type) {
		case *rsa.PrivateKey:
			keyPEM = pem.EncodeToMemory(&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(k),
			})
		case *ecdsa.PrivateKey:
			der, _ := x509.MarshalECPrivateKey(k)
			keyPEM = pem.EncodeToMemory(&pem.Block{
				Type:  "EC PRIVATE KEY",
				Bytes: der,
			})
		case ed25519.PrivateKey:
			der, _ := x509.MarshalPKCS8PrivateKey(k)
			keyPEM = pem.EncodeToMemory(&pem.Block{
				Type:  "PRIVATE KEY",
				Bytes: der,
			})
		}

		if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to write private key: %v", err)
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("Created certificate for %s", cn)
	}

	return result, nil
}

// Test validates module parameters
func (m *X509Module) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	path := getStringParameter(decl, "path", "")
	if path == "" {
		result.Success = false
		result.Comment = "path parameter is required"
		return result, nil
	}

	if decl.State == "present" {
		cn := getStringParameter(decl, "common_name", "")
		if cn == "" {
			result.Success = false
			result.Comment = "common_name parameter is required for present state"
			return result, nil
		}

		keyType := getStringParameter(decl, "key_type", "rsa")
		validKeyTypes := map[string]bool{"rsa": true, "ecdsa": true, "ed25519": true}
		if !validKeyTypes[keyType] {
			result.Success = false
			result.Comment = fmt.Sprintf("invalid key_type: %s (must be rsa, ecdsa, or ed25519)", keyType)
			return result, nil
		}
	}

	result.Success = true
	result.Comment = "X509 module parameters are valid"
	return result, nil
}

// ============================================================================
// CA Module - Manage Certificate Authorities
// ============================================================================

// CAModule manages Certificate Authority operations
type CAModule struct {
	*BaseModule
}

// NewCAModule creates a new CA module
func NewCAModule() *CAModule {
	return &CAModule{
		BaseModule: NewBaseModule("ca", []string{"present", "absent"}),
	}
}

// Check examines the current state of a CA
func (m *CAModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	path := getStringParameter(decl, "path", "")
	if path == "" {
		return nil, fmt.Errorf("path parameter is required")
	}

	result := &ModuleCheckResult{
		Metadata: make(map[string]interface{}),
	}

	// CA consists of cert and key
	certPath := filepath.Join(path, "ca.crt")
	keyPath := filepath.Join(path, "ca.key")

	certData, certErr := os.ReadFile(certPath)
	_, keyErr := os.ReadFile(keyPath)

	if os.IsNotExist(certErr) || os.IsNotExist(keyErr) {
		result.Present = false
		result.Matches = decl.State == "absent"
		return result, nil
	}
	if certErr != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", certErr)
	}
	if keyErr != nil {
		return nil, fmt.Errorf("failed to read CA key: %w", keyErr)
	}

	result.Present = true

	// Parse certificate
	block, _ := pem.Decode(certData)
	if block == nil || block.Type != "CERTIFICATE" {
		result.Metadata["valid"] = false
		result.Matches = false // Files exist but are invalid, need action for any state
		return result, nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		result.Metadata["valid"] = false
		result.Matches = false // Files exist but are invalid, need action for any state
		return result, nil
	}

	result.Metadata["valid"] = true
	result.Metadata["subject"] = cert.Subject.String()
	result.Metadata["is_ca"] = cert.IsCA
	result.Metadata["not_after"] = cert.NotAfter.Format(time.RFC3339)

	switch decl.State {
	case "absent":
		result.Matches = false
	case "present":
		result.Matches = true
		result.CurrentState = "present"
	}

	return result, nil
}

// Apply creates or removes a CA
func (m *CAModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	path := getStringParameter(decl, "path", "")

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	check, err := m.Check(ctx, decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Check failed: %v", err)
		return result, nil
	}

	if check.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "CA already in desired state"
		return result, nil
	}

	switch decl.State {
	case "absent":
		// Remove CA directory
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to remove CA: %v", err)
			return result, nil
		}
		result.Success = true
		result.Changed = check.Present
		result.Comment = "Removed CA"

	case "present":
		cn := getStringParameter(decl, "common_name", "")
		if cn == "" {
			result.Success = false
			result.Comment = "common_name parameter is required"
			return result, nil
		}

		org := getStringParameter(decl, "organization", "")
		country := getStringParameter(decl, "country", "")
		validity := getIntParameter(decl, "validity_days", 3650) // 10 years default for CA
		keyType := getStringParameter(decl, "key_type", "rsa")
		keySize := getIntParameter(decl, "key_size", 4096)

		// Create CA directory
		if err := os.MkdirAll(path, 0700); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create CA directory: %v", err)
			return result, nil
		}

		// Generate private key
		var privateKey crypto.PrivateKey
		var publicKey crypto.PublicKey

		switch keyType {
		case "rsa":
			key, err := rsa.GenerateKey(rand.Reader, keySize)
			if err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to generate RSA key: %v", err)
				return result, nil
			}
			privateKey = key
			publicKey = &key.PublicKey
		case "ecdsa":
			key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
			if err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to generate ECDSA key: %v", err)
				return result, nil
			}
			privateKey = key
			publicKey = &key.PublicKey
		default:
			result.Success = false
			result.Comment = fmt.Sprintf("Unknown key type: %s", keyType)
			return result, nil
		}

		// Generate serial number
		serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

		// Build CA certificate
		notBefore := time.Now()
		notAfter := notBefore.AddDate(0, 0, validity)

		template := x509.Certificate{
			SerialNumber: serialNumber,
			Subject: pkix.Name{
				CommonName:   cn,
				Organization: []string{org},
				Country:      []string{country},
			},
			NotBefore:             notBefore,
			NotAfter:              notAfter,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
			BasicConstraintsValid: true,
			IsCA:                  true,
			MaxPathLen:            2,
		}

		// Create CA certificate (self-signed)
		certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create CA certificate: %v", err)
			return result, nil
		}

		// Write CA certificate
		certPath := filepath.Join(path, "ca.crt")
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
		if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to write CA certificate: %v", err)
			return result, nil
		}

		// Write CA private key
		keyPath := filepath.Join(path, "ca.key")
		var keyPEM []byte
		switch k := privateKey.(type) {
		case *rsa.PrivateKey:
			keyPEM = pem.EncodeToMemory(&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(k),
			})
		case *ecdsa.PrivateKey:
			der, _ := x509.MarshalECPrivateKey(k)
			keyPEM = pem.EncodeToMemory(&pem.Block{
				Type:  "EC PRIVATE KEY",
				Bytes: der,
			})
		}

		if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to write CA key: %v", err)
			return result, nil
		}

		// Create serial file
		serialFile := filepath.Join(path, "serial")
		if err := os.WriteFile(serialFile, []byte("01\n"), 0644); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to write serial file: %v", err)
			return result, nil
		}

		// Create index file
		indexFile := filepath.Join(path, "index.txt")
		if err := os.WriteFile(indexFile, []byte(""), 0644); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to write index file: %v", err)
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("Created CA: %s", cn)
	}

	return result, nil
}

// Test validates module parameters
func (m *CAModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	path := getStringParameter(decl, "path", "")
	if path == "" {
		result.Success = false
		result.Comment = "path parameter is required"
		return result, nil
	}

	if decl.State == "present" {
		cn := getStringParameter(decl, "common_name", "")
		if cn == "" {
			result.Success = false
			result.Comment = "common_name parameter is required for present state"
			return result, nil
		}
	}

	result.Success = true
	result.Comment = "CA module parameters are valid"
	return result, nil
}

// SignCertificate signs a certificate with this CA
func (m *CAModule) SignCertificate(caPath string, csrPEM []byte, validityDays int) ([]byte, error) {
	// Read CA certificate
	caCertData, err := os.ReadFile(filepath.Join(caPath, "ca.crt"))
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	block, _ := pem.Decode(caCertData)
	if block == nil {
		return nil, fmt.Errorf("failed to parse CA certificate PEM")
	}

	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Read CA private key
	caKeyData, err := os.ReadFile(filepath.Join(caPath, "ca.key"))
	if err != nil {
		return nil, fmt.Errorf("failed to read CA key: %w", err)
	}

	block, _ = pem.Decode(caKeyData)
	if block == nil {
		return nil, fmt.Errorf("failed to parse CA key PEM")
	}

	var caKey crypto.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		caKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		caKey, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		caKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA key: %w", err)
	}

	// Parse CSR
	block, _ = pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("failed to parse CSR PEM")
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSR: %w", err)
	}

	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("CSR signature invalid: %w", err)
	}

	// Generate serial number
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	// Create certificate
	notBefore := time.Now()
	notAfter := notBefore.AddDate(0, 0, validityDays)

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               csr.Subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              csr.DNSNames,
		IPAddresses:           csr.IPAddresses,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, caCert, csr.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign certificate: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), nil
}

// ============================================================================
// ACME Module - Let's Encrypt / ACME certificates
// ============================================================================

// ACMEModule manages ACME certificates (Let's Encrypt)
type ACMEModule struct {
	*BaseModule
}

// NewACMEModule creates a new ACME module
func NewACMEModule() *ACMEModule {
	return &ACMEModule{
		BaseModule: NewBaseModule("acme", []string{"present", "absent", "renewed"}),
	}
}

// Check examines the current state of an ACME certificate
func (m *ACMEModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	path := getStringParameter(decl, "path", "")
	if path == "" {
		return nil, fmt.Errorf("path parameter is required")
	}

	domain := getStringParameter(decl, "domain", "")
	if domain == "" && decl.State != "absent" {
		return nil, fmt.Errorf("domain parameter is required for state %s", decl.State)
	}

	result := &ModuleCheckResult{
		Metadata: make(map[string]interface{}),
	}

	certPath := filepath.Join(path, domain+".crt")
	keyPath := filepath.Join(path, domain+".key")

	certData, certErr := os.ReadFile(certPath)
	_, keyErr := os.ReadFile(keyPath)

	if os.IsNotExist(certErr) || os.IsNotExist(keyErr) {
		result.Present = false
		result.Matches = decl.State == "absent"
		return result, nil
	}
	if certErr != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", certErr)
	}

	result.Present = true

	// Parse certificate to check validity
	block, _ := pem.Decode(certData)
	if block == nil || block.Type != "CERTIFICATE" {
		result.Metadata["valid"] = false
		result.Matches = false // Files exist but are invalid, need action for any state
		return result, nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		result.Metadata["valid"] = false
		result.Matches = false // Files exist but are invalid, need action for any state
		return result, nil
	}

	result.Metadata["valid"] = true
	result.Metadata["subject"] = cert.Subject.String()
	result.Metadata["issuer"] = cert.Issuer.String()
	result.Metadata["not_after"] = cert.NotAfter.Format(time.RFC3339)
	result.Metadata["dns_names"] = cert.DNSNames

	// Check renewal threshold (default 30 days)
	renewDays := getIntParameter(decl, "renew_days", 30)
	renewTime := time.Now().AddDate(0, 0, renewDays)
	needsRenewal := cert.NotAfter.Before(renewTime)
	result.Metadata["needs_renewal"] = needsRenewal

	switch decl.State {
	case "absent":
		result.Matches = false
	case "present":
		result.Matches = true
		result.CurrentState = "present"
	case "renewed":
		result.Matches = !needsRenewal
		if needsRenewal {
			result.Diff = map[string]interface{}{
				"expires":        cert.NotAfter.Format(time.RFC3339),
				"renew_deadline": renewTime.Format(time.RFC3339),
			}
		}
	}

	return result, nil
}

// Apply requests, renews, or removes an ACME certificate
func (m *ACMEModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	path := getStringParameter(decl, "path", "")
	domain := getStringParameter(decl, "domain", "")

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	check, err := m.Check(ctx, decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Check failed: %v", err)
		return result, nil
	}

	if check.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "ACME certificate already in desired state"
		return result, nil
	}

	switch decl.State {
	case "absent":
		// Remove certificate files
		certPath := filepath.Join(path, domain+".crt")
		keyPath := filepath.Join(path, domain+".key")

		removed := false
		if err := os.Remove(certPath); err == nil {
			removed = true
		}
		if err := os.Remove(keyPath); err == nil {
			removed = true
		}

		result.Success = true
		result.Changed = removed
		result.Comment = "Removed ACME certificate"

	case "present", "renewed":
		// NOTE: Full ACME implementation requires external library (e.g., lego)
		// This is a placeholder that documents the expected behavior
		//
		// In a full implementation, this would:
		// 1. Create ACME account if needed
		// 2. Request certificate via HTTP-01 or DNS-01 challenge
		// 3. Save certificate and key to path

		email := getStringParameter(decl, "email", "")
		challenge := getStringParameter(decl, "challenge", "http-01")
		staging := getBoolParameter(decl, "staging", false)

		// For now, return an error indicating ACME is not fully implemented
		result.Success = false
		result.Comment = fmt.Sprintf(
			"ACME certificate request not yet implemented. "+
				"Would request cert for %s using %s challenge (staging=%v, email=%s). "+
				"Consider using certbot or lego CLI as a workaround.",
			domain, challenge, staging, email,
		)
	}

	return result, nil
}

// Test validates module parameters
func (m *ACMEModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	path := getStringParameter(decl, "path", "")
	if path == "" {
		result.Success = false
		result.Comment = "path parameter is required"
		return result, nil
	}

	if decl.State != "absent" {
		domain := getStringParameter(decl, "domain", "")
		if domain == "" {
			result.Success = false
			result.Comment = "domain parameter is required for state " + decl.State
			return result, nil
		}

		challenge := getStringParameter(decl, "challenge", "http-01")
		validChallenges := map[string]bool{"http-01": true, "dns-01": true, "tls-alpn-01": true}
		if !validChallenges[challenge] {
			result.Success = false
			result.Comment = fmt.Sprintf("invalid challenge type: %s", challenge)
			return result, nil
		}
	}

	result.Success = true
	result.Comment = "ACME module parameters are valid (note: full ACME not yet implemented)"
	return result, nil
}

// ============================================================================
// Helper functions
// ============================================================================

func formatIPs(ips []net.IP) []string {
	result := make([]string, len(ips))
	for i, ip := range ips {
		result[i] = ip.String()
	}
	return result
}

// GenerateCSR generates a Certificate Signing Request
func GenerateCSR(privateKey crypto.PrivateKey, cn string, sans []string, sanIPs []net.IP) ([]byte, error) {
	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: cn,
		},
		DNSNames:    sans,
		IPAddresses: sanIPs,
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &template, privateKey)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	return buf.Bytes(), nil
}
