package rotation

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

// CertificateRotationProvider rotates TLS certificates for gNMI and other
// certificate-based device credentials.
type CertificateRotationProvider struct {
	// CertValidity is the validity period for generated certificates (default 365 days).
	CertValidity time.Duration
	// Commander installs and verifies certificates on devices.
	Commander CertificateCommander
}

// CertificateCommander manages certificates on devices.
type CertificateCommander interface {
	InstallCertificate(ctx context.Context, device DeviceInfo, cert, key []byte) error
	VerifyCertificate(ctx context.Context, device DeviceInfo, cert []byte) error
	RemoveCertificate(ctx context.Context, device DeviceInfo, cert []byte) error
}

// NewCertificateRotationProvider creates a new certificate rotation provider.
func NewCertificateRotationProvider(commander CertificateCommander) *CertificateRotationProvider {
	return &CertificateRotationProvider{
		CertValidity: 365 * 24 * time.Hour,
		Commander:    commander,
	}
}

func (p *CertificateRotationProvider) SupportsType(credType credentials.CredentialType) bool {
	return credType == credentials.CredentialTypeGNMI
}

func (p *CertificateRotationProvider) ValidateOld(ctx context.Context, device DeviceInfo, cred credentials.Credential) error {
	gnmi, ok := cred.(*credentials.GNMICredential)
	if !ok {
		return fmt.Errorf("expected GNMICredential, got %T", cred)
	}
	if p.Commander == nil {
		return errors.New("no certificate commander configured")
	}
	return p.Commander.VerifyCertificate(ctx, device, gnmi.ClientCert)
}

func (p *CertificateRotationProvider) Generate(_ context.Context, device DeviceInfo, oldCred credentials.Credential) (credentials.Credential, error) {
	old, ok := oldCred.(*credentials.GNMICredential)
	if !ok {
		return nil, fmt.Errorf("expected GNMICredential, got %T", oldCred)
	}

	// Generate a new ECDSA P-256 key pair
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   device.Host,
			Organization: []string{"Keystone Core"},
		},
		NotBefore:             now,
		NotAfter:              now.Add(p.CertValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Self-sign (in production, a CA would sign this)
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	privKeyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privKeyDER})

	return &credentials.GNMICredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   old.CredentialID,
			CredentialType: credentials.CredentialTypeGNMI,
			Description:    old.Description,
			Expires:        now.Add(p.CertValidity),
			Metadata:       old.Metadata,
		},
		Username:   old.Username,
		Password:   old.Password,
		CACert:     old.CACert,
		ClientCert: certPEM,
		ClientKey:  keyPEM,
		SkipVerify: old.SkipVerify,
	}, nil
}

func (p *CertificateRotationProvider) Apply(ctx context.Context, device DeviceInfo, _, newCred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no certificate commander configured")
	}
	gnmi, ok := newCred.(*credentials.GNMICredential)
	if !ok {
		return fmt.Errorf("expected GNMICredential, got %T", newCred)
	}
	return p.Commander.InstallCertificate(ctx, device, gnmi.ClientCert, gnmi.ClientKey)
}

func (p *CertificateRotationProvider) Verify(ctx context.Context, device DeviceInfo, newCred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no certificate commander configured")
	}
	gnmi, ok := newCred.(*credentials.GNMICredential)
	if !ok {
		return fmt.Errorf("expected GNMICredential, got %T", newCred)
	}
	return p.Commander.VerifyCertificate(ctx, device, gnmi.ClientCert)
}

func (p *CertificateRotationProvider) Rollback(ctx context.Context, device DeviceInfo, oldCred, newCred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no certificate commander configured")
	}

	// Remove the new cert
	if gnmi, ok := newCred.(*credentials.GNMICredential); ok {
		_ = p.Commander.RemoveCertificate(ctx, device, gnmi.ClientCert)
	}

	// Reinstall the old cert
	oldGNMI, ok := oldCred.(*credentials.GNMICredential)
	if !ok {
		return fmt.Errorf("expected GNMICredential, got %T", oldCred)
	}
	return p.Commander.InstallCertificate(ctx, device, oldGNMI.ClientCert, oldGNMI.ClientKey)
}

func (p *CertificateRotationProvider) Cleanup(ctx context.Context, device DeviceInfo, oldCred credentials.Credential) error {
	if p.Commander == nil {
		return nil
	}
	gnmi, ok := oldCred.(*credentials.GNMICredential)
	if !ok {
		return nil
	}
	return p.Commander.RemoveCertificate(ctx, device, gnmi.ClientCert)
}

var _ RotationProvider = (*CertificateRotationProvider)(nil)
