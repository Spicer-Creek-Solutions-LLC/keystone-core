package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

// PackageSigner signs bootstrap package manifests using the internal signing infrastructure.
type PackageSigner struct {
	signer signing.Signer
}

// NewPackageSigner creates a signer from a PEM-encoded private key.
func NewPackageSigner(privateKeyPEM []byte) (*PackageSigner, error) {
	s, err := signing.NewKeySigner(&signing.KeySignerConfig{
		PrivateKeyPEM: privateKeyPEM,
		HashAlgorithm: signing.HashSHA256,
		Format:        signing.FormatBase64,
	})
	if err != nil {
		return nil, fmt.Errorf("creating signer: %w", err)
	}
	return &PackageSigner{signer: s}, nil
}

// NewPackageSignerFromFile creates a signer from a private key file path.
func NewPackageSignerFromFile(keyPath string) (*PackageSigner, error) {
	s, err := signing.NewKeySigner(&signing.KeySignerConfig{
		PrivateKeyPath: keyPath,
		HashAlgorithm:  signing.HashSHA256,
		Format:         signing.FormatBase64,
	})
	if err != nil {
		return nil, fmt.Errorf("creating signer from file: %w", err)
	}
	return &PackageSigner{signer: s}, nil
}

// SignManifest signs the manifest.json file and writes a detached signature
// to signatures/manifest.json.sig within the package directory.
func (ps *PackageSigner) SignManifest(ctx context.Context, packageDir string) error {
	manifestPath := filepath.Join(packageDir, "manifest.json")
	sigDir := filepath.Join(packageDir, "signatures")

	if err := os.MkdirAll(sigDir, 0o750); err != nil {
		return fmt.Errorf("creating signatures directory: %w", err)
	}

	result, err := ps.signer.SignFile(ctx, manifestPath)
	if err != nil {
		return fmt.Errorf("signing manifest: %w", err)
	}

	sigPath := filepath.Join(sigDir, "manifest.json.sig")
	//nolint:gosec // G306: signature files need to be readable for verification
	if err := os.WriteFile(sigPath, []byte(result.SignatureBase64), 0o644); err != nil {
		return fmt.Errorf("writing signature: %w", err)
	}

	return nil
}

// WritePublicKey writes the signer's public key to signatures/cosign.pub.
func (ps *PackageSigner) WritePublicKey(packageDir string) error {
	pubKey, err := ps.signer.PublicKey()
	if err != nil {
		return fmt.Errorf("getting public key: %w", err)
	}

	sigDir := filepath.Join(packageDir, "signatures")
	if err := os.MkdirAll(sigDir, 0o750); err != nil {
		return fmt.Errorf("creating signatures directory: %w", err)
	}

	pubPath := filepath.Join(sigDir, "cosign.pub")
	//nolint:gosec // G306: public keys need to be readable for verification
	if err := os.WriteFile(pubPath, pubKey, 0o644); err != nil {
		return fmt.Errorf("writing public key: %w", err)
	}

	return nil
}
