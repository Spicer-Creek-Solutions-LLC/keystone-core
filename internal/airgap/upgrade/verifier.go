package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
	"github.com/shawnbutts/keystone-core/internal/signing"
	"github.com/shawnbutts/keystone-core/pkg/semver"
)

// VerifyResult contains the outcome of verifying an upgrade package.
type VerifyResult struct {
	Valid            bool
	ManifestValid    bool
	ChecksumsValid   bool
	SignaturePresent bool
	Manifest         *Manifest
	Warnings         []string
	Error            error
}

// CompatResult contains the outcome of a compatibility check.
type CompatResult struct {
	Compatible      bool
	CurrentVersion  string
	FromVersion     string
	ToVersion       string
	BreakingChanges []string
	Blockers        []string
}

// PackageVerifier verifies upgrade package signatures, checksums, and compatibility.
type PackageVerifier struct {
	trustedKeys [][]byte
}

// NewPackageVerifier creates a verifier with the given trusted public keys (PEM-encoded).
func NewPackageVerifier(trustedKeys [][]byte) *PackageVerifier {
	return &PackageVerifier{trustedKeys: trustedKeys}
}

// Verify checks the upgrade package manifest signature, file checksums, and manifest validity.
func (v *PackageVerifier) Verify(ctx context.Context, packageDir string) (*VerifyResult, error) {
	result := &VerifyResult{}

	// Read manifest
	manifestPath := filepath.Join(packageDir, "manifest.json")
	manifest, err := ReadManifest(manifestPath)
	if err != nil {
		result.Error = fmt.Errorf("reading manifest: %w", err)
		return result, nil
	}
	result.Manifest = manifest

	// Validate manifest
	if err := manifest.Validate(); err != nil {
		result.Error = fmt.Errorf("invalid manifest: %w", err)
		return result, nil
	}
	result.ManifestValid = true

	// Check signature if present
	sigPath := filepath.Join(packageDir, "signatures", "manifest.json.sig")
	if _, err := os.Stat(sigPath); err == nil {
		result.SignaturePresent = true
		if err := v.verifySignature(ctx, packageDir); err != nil {
			result.Error = fmt.Errorf("signature verification failed: %w", err)
			return result, nil
		}
	} else if manifest.RequiresVerification {
		result.Error = fmt.Errorf("package requires verification but no signature found")
		return result, nil
	} else {
		result.Warnings = append(result.Warnings, "package is unsigned")
	}

	// Verify checksums
	if err := v.verifyChecksums(packageDir, manifest); err != nil {
		result.Error = fmt.Errorf("checksum verification failed: %w", err)
		return result, nil
	}
	result.ChecksumsValid = true

	result.Valid = true
	return result, nil
}

// CheckCompatibility checks whether the upgrade package is compatible with the current version.
func (v *PackageVerifier) CheckCompatibility(manifest *Manifest, currentVersion string) (*CompatResult, error) {
	result := &CompatResult{
		CurrentVersion:  currentVersion,
		FromVersion:     manifest.FromVersion,
		ToVersion:       manifest.ToVersion,
		BreakingChanges: manifest.BreakingChanges,
	}

	current, err := semver.Parse(currentVersion)
	if err != nil {
		result.Blockers = append(result.Blockers, fmt.Sprintf("invalid current version %q: %v", currentVersion, err))
		return result, nil
	}

	from, err := semver.Parse(manifest.FromVersion)
	if err != nil {
		result.Blockers = append(result.Blockers, fmt.Sprintf("invalid from_version %q: %v", manifest.FromVersion, err))
		return result, nil
	}

	to, err := semver.Parse(manifest.ToVersion)
	if err != nil {
		result.Blockers = append(result.Blockers, fmt.Sprintf("invalid to_version %q: %v", manifest.ToVersion, err))
		return result, nil
	}

	// Current version must be >= from_version
	if current.LessThan(from) {
		result.Blockers = append(result.Blockers,
			fmt.Sprintf("current version %s is older than minimum required %s", currentVersion, manifest.FromVersion))
	}

	// Current version must be < to_version (no downgrade)
	if !current.LessThan(to) {
		result.Blockers = append(result.Blockers,
			fmt.Sprintf("current version %s is already at or newer than target %s", currentVersion, manifest.ToVersion))
	}

	result.Compatible = len(result.Blockers) == 0
	return result, nil
}

func (v *PackageVerifier) verifySignature(ctx context.Context, packageDir string) error {
	manifestPath := filepath.Join(packageDir, "manifest.json")
	sigPath := filepath.Join(packageDir, "signatures", "manifest.json.sig")

	for _, pubKeyPEM := range v.trustedKeys {
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

func (v *PackageVerifier) verifyChecksums(packageDir string, manifest *Manifest) error {
	for _, c := range manifest.Components {
		if err := verifyEntryChecksum(packageDir, c.Path, c.SHA256); err != nil {
			return fmt.Errorf("component %q: %w", c.Name, err)
		}
	}

	for _, m := range manifest.Modules {
		if err := verifyEntryChecksum(packageDir, m.Path, m.SHA256); err != nil {
			return fmt.Errorf("module %q: %w", m.Name, err)
		}
	}

	if manifest.Checksum != "" {
		actual, err := bootstrap.CalculateChecksum(packageDir)
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
	actual, err := bootstrap.HashFile(fullPath)
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
