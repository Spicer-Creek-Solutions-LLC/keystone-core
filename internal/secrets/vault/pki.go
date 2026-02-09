// Package vault provides a HashiCorp Vault backend for the secrets broker.
package vault

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// CertificateType represents the type of certificate to issue.
type CertificateType string

// CertificateTypeServer constants define the supported types.
const (
	CertificateTypeServer     CertificateType = "server"
	CertificateTypeClient     CertificateType = "client"
	CertificateTypeCodeSign   CertificateType = "code_signing"
	CertificateTypeEmail      CertificateType = "email"
	CertificateTypeServerAuth CertificateType = "server_auth"
	CertificateTypeClientAuth CertificateType = "client_auth"
)

// PKIConfig configures a PKI secret engine mount.
type PKIConfig struct {
	// MountPath is the mount path for the PKI engine (default: "pki").
	MountPath string `json:"mount_path,omitempty"`

	// DefaultTTL is the default TTL for issued certificates.
	DefaultTTL time.Duration `json:"default_ttl,omitempty"`

	// MaxTTL is the maximum TTL for issued certificates.
	MaxTTL time.Duration `json:"max_ttl,omitempty"`
}

// PKIEngine provides methods for working with Vault's PKI secret engine.
type PKIEngine struct {
	client *Client
	config *PKIConfig
}

// NewPKIEngine creates a new PKI engine client.
func NewPKIEngine(client *Client, config *PKIConfig) *PKIEngine {
	if config == nil {
		config = &PKIConfig{}
	}
	if config.MountPath == "" {
		config.MountPath = "pki"
	}
	return &PKIEngine{
		client: client,
		config: config,
	}
}

// Certificate represents an issued certificate.
type Certificate struct {
	// Certificate is the PEM-encoded certificate.
	Certificate string `json:"certificate"`

	// PrivateKey is the PEM-encoded private key.
	PrivateKey string `json:"private_key"`

	// PrivateKeyType is the type of private key (rsa, ec, ed25519).
	PrivateKeyType string `json:"private_key_type"`

	// CAChain is the PEM-encoded CA certificate chain.
	CAChain []string `json:"ca_chain"`

	// IssuingCA is the PEM-encoded issuing CA certificate.
	IssuingCA string `json:"issuing_ca"`

	// SerialNumber is the certificate serial number.
	SerialNumber string `json:"serial_number"`

	// Expiration is when the certificate expires.
	Expiration time.Time `json:"expiration"`

	// LeaseID is the Vault lease ID.
	LeaseID string `json:"lease_id,omitempty"`

	// LeaseDuration is the lease duration.
	LeaseDuration time.Duration `json:"lease_duration,omitempty"`

	// Renewable indicates if the lease is renewable.
	Renewable bool `json:"renewable,omitempty"`
}

// CertificateRequest specifies parameters for certificate issuance.
type CertificateRequest struct {
	// Role is the PKI role to use for issuance.
	Role string `json:"role"`

	// CommonName is the CN for the certificate.
	CommonName string `json:"common_name"`

	// AltNames are subject alternative names (comma-separated DNS names).
	AltNames string `json:"alt_names,omitempty"`

	// IPSANs are IP subject alternative names (comma-separated IPs).
	IPSANs string `json:"ip_sans,omitempty"`

	// URISANs are URI subject alternative names (comma-separated URIs).
	URISANs string `json:"uri_sans,omitempty"`

	// OtherSANs are custom subject alternative names (semicolon-delimited).
	OtherSANs string `json:"other_sans,omitempty"`

	// TTL is the requested TTL for the certificate.
	TTL time.Duration `json:"ttl,omitempty"`

	// Format is the output format (pem, der, pem_bundle).
	Format string `json:"format,omitempty"`

	// PrivateKeyFormat is the private key format (der, pkcs8).
	PrivateKeyFormat string `json:"private_key_format,omitempty"`

	// ExcludeCNFromSANs excludes CN from SANs.
	ExcludeCNFromSANs bool `json:"exclude_cn_from_sans,omitempty"`

	// NotAfter specifies explicit expiration time (RFC3339).
	NotAfter string `json:"not_after,omitempty"`
}

// IssueCertificate issues a new certificate.
func (p *PKIEngine) IssueCertificate(ctx context.Context, req *CertificateRequest) (*Certificate, error) {
	if req == nil || req.Role == "" {
		return nil, fmt.Errorf("role is required")
	}
	if req.CommonName == "" {
		return nil, fmt.Errorf("common_name is required")
	}

	path := fmt.Sprintf("%s/issue/%s", p.config.MountPath, req.Role)

	data := map[string]interface{}{
		"common_name": req.CommonName,
	}

	if req.AltNames != "" {
		data["alt_names"] = req.AltNames
	}
	if req.IPSANs != "" {
		data["ip_sans"] = req.IPSANs
	}
	if req.URISANs != "" {
		data["uri_sans"] = req.URISANs
	}
	if req.OtherSANs != "" {
		data["other_sans"] = req.OtherSANs
	}
	if req.TTL > 0 {
		data["ttl"] = fmt.Sprintf("%ds", int(req.TTL.Seconds()))
	}
	if req.Format != "" {
		data["format"] = req.Format
	}
	if req.PrivateKeyFormat != "" {
		data["private_key_format"] = req.PrivateKeyFormat
	}
	if req.ExcludeCNFromSANs {
		data["exclude_cn_from_sans"] = true
	}
	if req.NotAfter != "" {
		data["not_after"] = req.NotAfter
	}

	resp, err := p.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("failed to issue certificate: %w", err)
	}

	return p.parseCertificate(resp)
}

// SignCSR signs a certificate signing request.
func (p *PKIEngine) SignCSR(ctx context.Context, role, csr, commonName string, ttl time.Duration) (*Certificate, error) {
	if role == "" {
		return nil, fmt.Errorf("role is required")
	}
	if csr == "" {
		return nil, fmt.Errorf("csr is required")
	}

	path := fmt.Sprintf("%s/sign/%s", p.config.MountPath, role)

	data := map[string]interface{}{
		"csr": csr,
	}

	if commonName != "" {
		data["common_name"] = commonName
	}
	if ttl > 0 {
		data["ttl"] = fmt.Sprintf("%ds", int(ttl.Seconds()))
	}

	resp, err := p.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("failed to sign CSR: %w", err)
	}

	return p.parseCertificate(resp)
}

// SignVerbatim signs a CSR without role constraints.
func (p *PKIEngine) SignVerbatim(ctx context.Context, csr string, ttl time.Duration) (*Certificate, error) {
	if csr == "" {
		return nil, fmt.Errorf("csr is required")
	}

	path := fmt.Sprintf("%s/sign-verbatim", p.config.MountPath)

	data := map[string]interface{}{
		"csr": csr,
	}

	if ttl > 0 {
		data["ttl"] = fmt.Sprintf("%ds", int(ttl.Seconds()))
	}

	resp, err := p.client.Write(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("failed to sign CSR verbatim: %w", err)
	}

	return p.parseCertificate(resp)
}

// RevokeCertificate revokes a certificate by serial number.
func (p *PKIEngine) RevokeCertificate(ctx context.Context, serialNumber string) error {
	if serialNumber == "" {
		return fmt.Errorf("serial_number is required")
	}

	path := fmt.Sprintf("%s/revoke", p.config.MountPath)

	data := map[string]interface{}{
		"serial_number": serialNumber,
	}

	_, err := p.client.Write(ctx, path, data)
	if err != nil {
		return fmt.Errorf("failed to revoke certificate: %w", err)
	}

	return nil
}

// RevokeCertificateByPEM revokes a certificate by its PEM content.
func (p *PKIEngine) RevokeCertificateByPEM(ctx context.Context, certificate string) error {
	if certificate == "" {
		return fmt.Errorf("certificate is required")
	}

	path := fmt.Sprintf("%s/revoke", p.config.MountPath)

	data := map[string]interface{}{
		"certificate": certificate,
	}

	_, err := p.client.Write(ctx, path, data)
	if err != nil {
		return fmt.Errorf("failed to revoke certificate: %w", err)
	}

	return nil
}

// GetCRL retrieves the certificate revocation list.
func (p *PKIEngine) GetCRL(ctx context.Context) ([]byte, error) {
	path := fmt.Sprintf("%s/crl", p.config.MountPath)

	resp, err := p.client.Read(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get CRL: %w", err)
	}

	if data, ok := resp["data"].(map[string]interface{}); ok {
		if crl, ok := data["crl"].(string); ok {
			return []byte(crl), nil
		}
	}

	return nil, fmt.Errorf("invalid CRL response")
}

// GetCA retrieves the CA certificate.
func (p *PKIEngine) GetCA(ctx context.Context) (string, error) {
	path := fmt.Sprintf("%s/ca/pem", p.config.MountPath)

	resp, err := p.client.Read(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to get CA: %w", err)
	}

	if data, ok := resp["data"].(map[string]interface{}); ok {
		if ca, ok := data["certificate"].(string); ok {
			return ca, nil
		}
	}

	return "", fmt.Errorf("invalid CA response")
}

// GetCAChain retrieves the full CA certificate chain.
func (p *PKIEngine) GetCAChain(ctx context.Context) (string, error) {
	path := fmt.Sprintf("%s/ca_chain", p.config.MountPath)

	resp, err := p.client.Read(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to get CA chain: %w", err)
	}

	if data, ok := resp["data"].(map[string]interface{}); ok {
		if chain, ok := data["certificate"].(string); ok {
			return chain, nil
		}
	}

	return "", fmt.Errorf("invalid CA chain response")
}

// ListCertificates lists all issued certificates.
func (p *PKIEngine) ListCertificates(ctx context.Context) ([]string, error) {
	path := fmt.Sprintf("%s/certs", p.config.MountPath)
	return p.client.List(ctx, path)
}

// GetCertificate retrieves a certificate by serial number.
func (p *PKIEngine) GetCertificate(ctx context.Context, serialNumber string) (*Certificate, error) {
	if serialNumber == "" {
		return nil, fmt.Errorf("serial_number is required")
	}

	path := fmt.Sprintf("%s/cert/%s", p.config.MountPath, serialNumber)

	resp, err := p.client.Read(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get certificate: %w", err)
	}

	return p.parseCertificateRead(resp)
}

// ListRoles lists all PKI roles.
func (p *PKIEngine) ListRoles(ctx context.Context) ([]string, error) {
	path := fmt.Sprintf("%s/roles", p.config.MountPath)
	return p.client.List(ctx, path)
}

// GetRole retrieves a PKI role configuration.
func (p *PKIEngine) GetRole(ctx context.Context, name string) (*PKIRole, error) {
	if name == "" {
		return nil, fmt.Errorf("role name is required")
	}

	path := fmt.Sprintf("%s/roles/%s", p.config.MountPath, name)

	resp, err := p.client.Read(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	return p.parseRole(resp)
}

// PKIRole represents a PKI role configuration.
type PKIRole struct {
	// Name is the role name.
	Name string `json:"name"`

	// TTL is the default TTL.
	TTL time.Duration `json:"ttl"`

	// MaxTTL is the maximum TTL.
	MaxTTL time.Duration `json:"max_ttl"`

	// AllowLocalhost allows localhost in CNs.
	AllowLocalhost bool `json:"allow_localhost"`

	// AllowedDomains are allowed domains for certificates.
	AllowedDomains []string `json:"allowed_domains"`

	// AllowBareDomains allows bare domains.
	AllowBareDomains bool `json:"allow_bare_domains"`

	// AllowSubdomains allows subdomains.
	AllowSubdomains bool `json:"allow_subdomains"`

	// AllowGlobDomains allows glob patterns in domains.
	AllowGlobDomains bool `json:"allow_glob_domains"`

	// AllowAnyName allows any CN.
	AllowAnyName bool `json:"allow_any_name"`

	// EnforceHostnames enforces hostname validation.
	EnforceHostnames bool `json:"enforce_hostnames"`

	// AllowIPSANs allows IP SANs.
	AllowIPSANs bool `json:"allow_ip_sans"`

	// AllowedURISANs are allowed URI SANs patterns.
	AllowedURISANs []string `json:"allowed_uri_sans"`

	// ServerFlag sets server flag in certificates.
	ServerFlag bool `json:"server_flag"`

	// ClientFlag sets client flag in certificates.
	ClientFlag bool `json:"client_flag"`

	// CodeSigningFlag sets code signing flag.
	CodeSigningFlag bool `json:"code_signing_flag"`

	// EmailProtectionFlag sets email protection flag.
	EmailProtectionFlag bool `json:"email_protection_flag"`

	// KeyType is the key type (rsa, ec, ed25519).
	KeyType string `json:"key_type"`

	// KeyBits is the key size in bits.
	KeyBits int `json:"key_bits"`

	// KeyUsage is the key usage list.
	KeyUsage []string `json:"key_usage"`

	// ExtKeyUsage is the extended key usage list.
	ExtKeyUsage []string `json:"ext_key_usage"`

	// RequireCN requires common name.
	RequireCN bool `json:"require_cn"`

	// BasicConstraintsValidForNonCA sets basic constraints.
	BasicConstraintsValidForNonCA bool `json:"basic_constraints_valid_for_non_ca"`

	// NotBeforeDuration is the duration before issuance time to set NotBefore.
	NotBeforeDuration time.Duration `json:"not_before_duration"`
}

// CreateRole creates a PKI role.
func (p *PKIEngine) CreateRole(ctx context.Context, name string, role *PKIRole) error {
	if name == "" {
		return fmt.Errorf("role name is required")
	}

	path := fmt.Sprintf("%s/roles/%s", p.config.MountPath, name)

	data := make(map[string]interface{})

	if role != nil {
		if role.TTL > 0 {
			data["ttl"] = fmt.Sprintf("%ds", int(role.TTL.Seconds()))
		}
		if role.MaxTTL > 0 {
			data["max_ttl"] = fmt.Sprintf("%ds", int(role.MaxTTL.Seconds()))
		}
		if len(role.AllowedDomains) > 0 {
			data["allowed_domains"] = role.AllowedDomains
		}
		if len(role.AllowedURISANs) > 0 {
			data["allowed_uri_sans"] = role.AllowedURISANs
		}

		data["allow_localhost"] = role.AllowLocalhost
		data["allow_bare_domains"] = role.AllowBareDomains
		data["allow_subdomains"] = role.AllowSubdomains
		data["allow_glob_domains"] = role.AllowGlobDomains
		data["allow_any_name"] = role.AllowAnyName
		data["enforce_hostnames"] = role.EnforceHostnames
		data["allow_ip_sans"] = role.AllowIPSANs
		data["server_flag"] = role.ServerFlag
		data["client_flag"] = role.ClientFlag
		data["code_signing_flag"] = role.CodeSigningFlag
		data["email_protection_flag"] = role.EmailProtectionFlag
		data["require_cn"] = role.RequireCN

		if role.KeyType != "" {
			data["key_type"] = role.KeyType
		}
		if role.KeyBits > 0 {
			data["key_bits"] = role.KeyBits
		}
		if len(role.KeyUsage) > 0 {
			data["key_usage"] = role.KeyUsage
		}
		if len(role.ExtKeyUsage) > 0 {
			data["ext_key_usage"] = role.ExtKeyUsage
		}
	}

	_, err := p.client.Write(ctx, path, data)
	return err
}

// DeleteRole deletes a PKI role.
func (p *PKIEngine) DeleteRole(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("role name is required")
	}

	path := fmt.Sprintf("%s/roles/%s", p.config.MountPath, name)
	_, err := p.client.Delete(ctx, path)
	return err
}

// TidyCertificates removes expired certificates and revoked entries.
func (p *PKIEngine) TidyCertificates(ctx context.Context, tidyCertStore, tidyRevokedCerts bool, safetyBuffer time.Duration) error {
	path := fmt.Sprintf("%s/tidy", p.config.MountPath)

	data := map[string]interface{}{
		"tidy_cert_store":    tidyCertStore,
		"tidy_revoked_certs": tidyRevokedCerts,
	}

	if safetyBuffer > 0 {
		data["safety_buffer"] = fmt.Sprintf("%ds", int(safetyBuffer.Seconds()))
	}

	_, err := p.client.Write(ctx, path, data)
	return err
}

// parseCertificate parses a certificate issuance response.
func (p *PKIEngine) parseCertificate(resp map[string]interface{}) (*Certificate, error) {
	if resp == nil {
		return nil, secrets.ErrSecretNotFound
	}

	cert := &Certificate{}

	if data, ok := resp["data"].(map[string]interface{}); ok {
		if certificate, ok := data["certificate"].(string); ok {
			cert.Certificate = certificate
		}
		if privateKey, ok := data["private_key"].(string); ok {
			cert.PrivateKey = privateKey
		}
		if privateKeyType, ok := data["private_key_type"].(string); ok {
			cert.PrivateKeyType = privateKeyType
		}
		if issuingCA, ok := data["issuing_ca"].(string); ok {
			cert.IssuingCA = issuingCA
		}
		if serialNumber, ok := data["serial_number"].(string); ok {
			cert.SerialNumber = serialNumber
		}
		if expiration, ok := data["expiration"].(float64); ok {
			cert.Expiration = time.Unix(int64(expiration), 0)
		}
		if caChain, ok := data["ca_chain"].([]interface{}); ok {
			for _, ca := range caChain {
				if s, ok := ca.(string); ok {
					cert.CAChain = append(cert.CAChain, s)
				}
			}
		}
	}

	// Parse lease information
	if leaseID, ok := resp["lease_id"].(string); ok {
		cert.LeaseID = leaseID
	}
	if leaseDuration, ok := resp["lease_duration"].(float64); ok {
		cert.LeaseDuration = time.Duration(leaseDuration) * time.Second
	}
	if renewable, ok := resp["renewable"].(bool); ok {
		cert.Renewable = renewable
	}

	return cert, nil
}

// parseCertificateRead parses a certificate read response.
func (p *PKIEngine) parseCertificateRead(resp map[string]interface{}) (*Certificate, error) {
	if resp == nil {
		return nil, secrets.ErrSecretNotFound
	}

	cert := &Certificate{}

	if data, ok := resp["data"].(map[string]interface{}); ok {
		if certificate, ok := data["certificate"].(string); ok {
			cert.Certificate = certificate
		}
		// When reading, there's no private key
	}

	// Parse certificate to get serial and expiration
	if cert.Certificate != "" {
		if parsed, err := ParseCertificatePEM(cert.Certificate); err == nil {
			cert.SerialNumber = fmt.Sprintf("%x", parsed.SerialNumber)
			cert.Expiration = parsed.NotAfter
		}
	}

	return cert, nil
}

// parseRole parses a role response.
func (p *PKIEngine) parseRole(resp map[string]interface{}) (*PKIRole, error) {
	if resp == nil {
		return nil, secrets.ErrSecretNotFound
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid role response")
	}

	role := &PKIRole{}

	if ttl, ok := data["ttl"].(float64); ok {
		role.TTL = time.Duration(ttl) * time.Second
	}
	if maxTTL, ok := data["max_ttl"].(float64); ok {
		role.MaxTTL = time.Duration(maxTTL) * time.Second
	}
	if allowLocalhost, ok := data["allow_localhost"].(bool); ok {
		role.AllowLocalhost = allowLocalhost
	}
	if allowBareDomains, ok := data["allow_bare_domains"].(bool); ok {
		role.AllowBareDomains = allowBareDomains
	}
	if allowSubdomains, ok := data["allow_subdomains"].(bool); ok {
		role.AllowSubdomains = allowSubdomains
	}
	if allowGlobDomains, ok := data["allow_glob_domains"].(bool); ok {
		role.AllowGlobDomains = allowGlobDomains
	}
	if allowAnyName, ok := data["allow_any_name"].(bool); ok {
		role.AllowAnyName = allowAnyName
	}
	if enforceHostnames, ok := data["enforce_hostnames"].(bool); ok {
		role.EnforceHostnames = enforceHostnames
	}
	if allowIPSANs, ok := data["allow_ip_sans"].(bool); ok {
		role.AllowIPSANs = allowIPSANs
	}
	if serverFlag, ok := data["server_flag"].(bool); ok {
		role.ServerFlag = serverFlag
	}
	if clientFlag, ok := data["client_flag"].(bool); ok {
		role.ClientFlag = clientFlag
	}
	if codeSigningFlag, ok := data["code_signing_flag"].(bool); ok {
		role.CodeSigningFlag = codeSigningFlag
	}
	if emailProtectionFlag, ok := data["email_protection_flag"].(bool); ok {
		role.EmailProtectionFlag = emailProtectionFlag
	}
	if requireCN, ok := data["require_cn"].(bool); ok {
		role.RequireCN = requireCN
	}
	if keyType, ok := data["key_type"].(string); ok {
		role.KeyType = keyType
	}
	if keyBits, ok := data["key_bits"].(float64); ok {
		role.KeyBits = int(keyBits)
	}

	role.AllowedDomains = parseStringSlice(data, "allowed_domains")
	role.AllowedURISANs = parseStringSlice(data, "allowed_uri_sans")
	role.KeyUsage = parseStringSlice(data, "key_usage")
	role.ExtKeyUsage = parseStringSlice(data, "ext_key_usage")

	return role, nil
}

// ToSecret converts a Certificate to a Secret.
func (c *Certificate) ToSecret(path string) *secrets.Secret {
	secret := &secrets.Secret{
		Path:    path,
		Backend: secrets.BackendTypeVault,
		Type:    secrets.SecretTypePKI,
		Data: map[string]interface{}{
			"certificate":      c.Certificate,
			"private_key":      c.PrivateKey,
			"private_key_type": c.PrivateKeyType,
			"issuing_ca":       c.IssuingCA,
			"serial_number":    c.SerialNumber,
			"ca_chain":         c.CAChain,
		},
		Metadata: map[string]string{
			"serial_number": c.SerialNumber,
		},
		ExpiresAt: c.Expiration,
		Renewable: c.Renewable,
	}

	if c.LeaseID != "" {
		secret.Lease = &secrets.Lease{
			ID:         c.LeaseID,
			SecretPath: path,
			Backend:    secrets.BackendTypeVault,
			State:      secrets.LeaseStateActive,
			TTL:        c.LeaseDuration,
			IssuedAt:   time.Now(),
			ExpiresAt:  time.Now().Add(c.LeaseDuration),
			Renewable:  c.Renewable,
			Revocable:  true,
		}
	}

	return secret
}

// ParseCertificatePEM parses a PEM-encoded certificate.
func ParseCertificatePEM(pemData string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	return x509.ParseCertificate(block.Bytes)
}

// IsCertificateExpiring checks if a certificate is expiring within the given duration.
func IsCertificateExpiring(cert *x509.Certificate, within time.Duration) bool {
	if cert == nil {
		return true
	}
	return time.Now().Add(within).After(cert.NotAfter)
}

// GetCertificateExpiration returns the expiration time of a PEM certificate.
func GetCertificateExpiration(pemData string) (time.Time, error) {
	cert, err := ParseCertificatePEM(pemData)
	if err != nil {
		return time.Time{}, err
	}
	return cert.NotAfter, nil
}
