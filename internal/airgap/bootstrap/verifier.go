package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

// PackageVerifier verifies bootstrap package manifest signatures and file checksums.
type PackageVerifier struct {
	trustedKeys [][]byte // PEM-encoded public keys
}

// NewPackageVerifier creates a verifier with the given trusted public keys.
func NewPackageVerifier(trustedKeys [][]byte) *PackageVerifier {
	return &PackageVerifier{trustedKeys: trustedKeys}
}

// VerifyManifestSignature checks that the manifest.json has a valid detached
// signature from one of the trusted keys.
func (pv *PackageVerifier) VerifyManifestSignature(ctx context.Context, packageDir string) error {
	manifestPath := filepath.Join(packageDir, "manifest.json")
	sigPath := filepath.Join(packageDir, "signatures", "manifest.json.sig")

	if _, err := os.Stat(sigPath); os.IsNotExist(err) {
		return fmt.Errorf("signature file not found: %s", sigPath)
	}

	for _, pubKeyPEM := range pv.trustedKeys {
		verifier, err := signing.NewKeyVerifier(&signing.KeyVerifierConfig{
			PublicKeyPEM:  pubKeyPEM,
			HashAlgorithm: signing.HashSHA256,
		})
		if err != nil {
			continue
		}

		valid, err := verifier.VerifyFile(ctx, manifestPath, sigPath)
		if err != nil {
			continue
		}
		if valid {
			return nil
		}
	}

	return fmt.Errorf("manifest signature could not be verified against any trusted key")
}

// VerifyChecksums validates that every file listed in the manifest matches
// its recorded SHA256 hash.
func (pv *PackageVerifier) VerifyChecksums(packageDir string, manifest *Manifest) error {
	// Verify component checksums
	for _, c := range manifest.Components {
		if err := verifyEntryChecksum(packageDir, c.Path, c.SHA256); err != nil {
			return fmt.Errorf("component %q: %w", c.Name, err)
		}
	}

	// Verify content checksums
	for _, entries := range []struct {
		label string
		items []ContentEntry
	}{
		{"module", manifest.Modules},
		{"blueprint", manifest.Blueprints},
		{"policy", manifest.Policies},
		{"config_template", manifest.ConfigTemplates},
	} {
		for _, e := range entries.items {
			if err := verifyEntryChecksum(packageDir, e.Path, e.SHA256); err != nil {
				return fmt.Errorf("%s %q: %w", entries.label, e.Name, err)
			}
		}
	}

	if manifest.Docs != nil {
		if err := verifyEntryChecksum(packageDir, manifest.Docs.Path, manifest.Docs.SHA256); err != nil {
			return fmt.Errorf("docs: %w", err)
		}
	}

	// Verify overall checksum
	if manifest.Checksum != "" {
		actual, err := CalculateChecksum(packageDir)
		if err != nil {
			return fmt.Errorf("calculating package checksum: %w", err)
		}
		if actual != manifest.Checksum {
			return fmt.Errorf("package checksum mismatch: expected %s, got %s", manifest.Checksum, actual)
		}
	}

	return nil
}

func verifyEntryChecksum(dir, relPath, expectedSHA256 string) error {
	if expectedSHA256 == "" {
		return nil
	}

	fullPath := filepath.Join(dir, relPath)
	actual, err := HashFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", relPath)
		}
		return fmt.Errorf("hashing %s: %w", relPath, err)
	}

	if actual != expectedSHA256 {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", relPath, expectedSHA256, actual)
	}
	return nil
}
