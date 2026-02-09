package rotation

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"

	"crypto/x509"

	"github.com/shawnbutts/keystone-core/internal/credentials"

	"golang.org/x/crypto/ssh"
)

// SSHProvider rotates SSH password and key credentials on devices.
type SSHProvider struct {
	// PasswordLength is the length of generated passwords (default 32).
	PasswordLength int
	// KeyType is the SSH key type to generate (default ed25519).
	KeyType string
	// Commander executes commands on devices (for applying changes).
	Commander SSHCommander
}

// SSHCommander executes SSH commands on devices.
type SSHCommander interface {
	RunCommand(ctx context.Context, device DeviceInfo, cred credentials.Credential, command string) (string, error)
	TestConnection(ctx context.Context, device DeviceInfo, cred credentials.Credential) error
}

// NewSSHProvider creates a new SSH rotation provider.
func NewSSHProvider(commander SSHCommander) *SSHProvider {
	return &SSHProvider{
		PasswordLength: 32,
		KeyType:        "ed25519",
		Commander:      commander,
	}
}

// SupportsType implements Provider.
func (p *SSHProvider) SupportsType(credType credentials.CredentialType) bool {
	return credType == credentials.CredentialTypeSSHPassword || credType == credentials.CredentialTypeSSHKey
}

// ValidateOld implements Provider.
func (p *SSHProvider) ValidateOld(ctx context.Context, device DeviceInfo, cred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no SSH commander configured")
	}
	return p.Commander.TestConnection(ctx, device, cred)
}

// Generate implements Provider.
func (p *SSHProvider) Generate(_ context.Context, _ DeviceInfo, oldCred credentials.Credential) (credentials.Credential, error) {
	switch old := oldCred.(type) {
	case *credentials.SSHPasswordCredential:
		return p.generatePassword(old)
	case *credentials.SSHKeyCredential:
		return p.generateKey(old)
	default:
		return nil, fmt.Errorf("unsupported credential type for SSH rotation: %T", oldCred)
	}
}

// Apply implements Provider.
func (p *SSHProvider) Apply(ctx context.Context, device DeviceInfo, oldCred, newCred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no SSH commander configured")
	}

	switch newC := newCred.(type) {
	case *credentials.SSHPasswordCredential:
		cmd := fmt.Sprintf("echo '%s:%s' | chpasswd", newC.Username, newC.Password)
		_, err := p.Commander.RunCommand(ctx, device, oldCred, cmd)
		return err
	case *credentials.SSHKeyCredential:
		pubKey, err := ssh.NewPublicKey(ed25519.PublicKey(newC.PublicKey))
		if err != nil {
			return fmt.Errorf("failed to parse public key: %w", err)
		}
		authorizedKey := string(ssh.MarshalAuthorizedKey(pubKey))
		cmd := fmt.Sprintf("mkdir -p ~/.ssh && echo '%s' >> ~/.ssh/authorized_keys", authorizedKey)
		_, err = p.Commander.RunCommand(ctx, device, oldCred, cmd)
		return err
	default:
		return fmt.Errorf("unsupported credential type: %T", newCred)
	}
}

// Verify implements Provider.
func (p *SSHProvider) Verify(ctx context.Context, device DeviceInfo, newCred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no SSH commander configured")
	}
	return p.Commander.TestConnection(ctx, device, newCred)
}

// Rollback implements Provider.
func (p *SSHProvider) Rollback(ctx context.Context, device DeviceInfo, oldCred, newCred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no SSH commander configured")
	}

	switch oldC := oldCred.(type) {
	case *credentials.SSHPasswordCredential:
		cmd := fmt.Sprintf("echo '%s:%s' | chpasswd", oldC.Username, oldC.Password)
		_, err := p.Commander.RunCommand(ctx, device, oldCred, cmd)
		return err
	case *credentials.SSHKeyCredential:
		// For key rollback, we remove the new key from authorized_keys
		if newKey, ok := newCred.(*credentials.SSHKeyCredential); ok && len(newKey.PublicKey) > 0 {
			pubKey, err := ssh.NewPublicKey(ed25519.PublicKey(newKey.PublicKey))
			if err == nil {
				authorizedKey := string(ssh.MarshalAuthorizedKey(pubKey))
				cmd := fmt.Sprintf("sed -i '/%s/d' ~/.ssh/authorized_keys", authorizedKey[:20])
				_, _ = p.Commander.RunCommand(ctx, device, oldCred, cmd)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported credential type: %T", oldCred)
	}
}

// Cleanup implements Provider.
func (p *SSHProvider) Cleanup(_ context.Context, _ DeviceInfo, _ credentials.Credential) error {
	return nil // No cleanup needed for SSH
}

func (p *SSHProvider) generatePassword(old *credentials.SSHPasswordCredential) (*credentials.SSHPasswordCredential, error) {
	password, err := generateRandomPassword(p.PasswordLength)
	if err != nil {
		return nil, fmt.Errorf("failed to generate password: %w", err)
	}

	return &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   old.CredentialID,
			CredentialType: credentials.CredentialTypeSSHPassword,
			Description:    old.Description,
			Metadata:       old.Metadata,
		},
		Username: old.Username,
		Password: password,
	}, nil
}

func (p *SSHProvider) generateKey(old *credentials.SSHKeyCredential) (*credentials.SSHKeyCredential, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ed25519 key: %w", err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	return &credentials.SSHKeyCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   old.CredentialID,
			CredentialType: credentials.CredentialTypeSSHKey,
			Description:    old.Description,
			Metadata:       old.Metadata,
		},
		Username:   old.Username,
		PrivateKey: privPEM,
		PublicKey:  []byte(pub),
	}, nil
}

func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}

var _ Provider = (*SSHProvider)(nil)
