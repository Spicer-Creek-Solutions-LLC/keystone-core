package transfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

// ImportConfig configures an import operation.
type ImportConfig struct {
	PackagePath string
	PreviewOnly bool
	TrustedKeys [][]byte // PEM-encoded public keys
	AgeIdentity string   // age identity file path for decryption
	DataSets    []string // filter to specific datasets; nil = all
	OutputDir   string
}

// ImportResult holds the outcome of an import operation.
type ImportResult struct {
	Manifest       *ExportManifest
	Verified       bool
	SignatureValid bool
	ChecksumsValid bool
	DataSets       []ImportedDataSet
	Warnings       []string
}

// ImportedDataSet describes an extracted data set.
type ImportedDataSet struct {
	Name  string
	Count int64
	Size  int64
	Path  string
}

// Importer verifies and extracts export packages.
type Importer struct {
	config ImportConfig
}

// NewImporter creates a new importer.
func NewImporter(config ImportConfig) (*Importer, error) {
	if config.PackagePath == "" {
		return nil, fmt.Errorf("package_path is required")
	}
	if config.OutputDir == "" && !config.PreviewOnly {
		return nil, fmt.Errorf("output_dir is required when not in preview mode")
	}
	return &Importer{config: config}, nil
}

// Import decrypts (if needed), verifies, and extracts the package.
func (imp *Importer) Import(ctx context.Context) (*ImportResult, error) {
	result := &ImportResult{}

	archivePath := imp.config.PackagePath
	var decryptedPath string

	if strings.HasSuffix(archivePath, ".age") {
		if imp.config.AgeIdentity == "" {
			return nil, fmt.Errorf("package is encrypted but no age identity provided")
		}
		tmp, err := os.MkdirTemp("", "kscore-transfer-decrypt-*")
		if err != nil {
			return nil, fmt.Errorf("creating temp dir for decryption: %w", err)
		}
		defer os.RemoveAll(tmp)

		decryptedPath = filepath.Join(tmp, "package.tar.gz")
		if err := decryptFile(ctx, imp.config.AgeIdentity, archivePath, decryptedPath); err != nil {
			return nil, fmt.Errorf("decrypting package: %w", err)
		}
		archivePath = decryptedPath
	}

	extractDir := imp.config.OutputDir
	if imp.config.PreviewOnly {
		tmp, err := os.MkdirTemp("", "kscore-transfer-preview-*")
		if err != nil {
			return nil, fmt.Errorf("creating temp dir for preview: %w", err)
		}
		defer os.RemoveAll(tmp)
		extractDir = tmp
	}

	if err := extractArchive(archivePath, extractDir); err != nil {
		return nil, fmt.Errorf("extracting archive: %w", err)
	}

	manifest, err := ReadManifest(filepath.Join(extractDir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	result.Manifest = manifest

	if manifest.RequiresVerification || len(imp.config.TrustedKeys) > 0 {
		sigErr := verifyManifestSignature(ctx, extractDir, imp.config.TrustedKeys)
		result.SignatureValid = sigErr == nil
		if sigErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("signature verification failed: %v", sigErr))
		}
	} else {
		result.SignatureValid = true
	}

	checksumErr := verifyDataChecksums(extractDir, manifest)
	result.ChecksumsValid = checksumErr == nil
	if checksumErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("checksum verification failed: %v", checksumErr))
	}

	result.Verified = result.SignatureValid && result.ChecksumsValid

	wantDS := make(map[string]bool)
	if len(imp.config.DataSets) > 0 {
		for _, name := range imp.config.DataSets {
			wantDS[name] = true
		}
	}

	for _, ds := range manifest.DataSets {
		if len(wantDS) > 0 && !wantDS[ds.Name] {
			continue
		}
		imported := ImportedDataSet{
			Name:  ds.Name,
			Count: ds.Count,
			Size:  ds.Size,
		}
		if !imp.config.PreviewOnly {
			imported.Path = filepath.Join(extractDir, ds.Path)
		}
		result.DataSets = append(result.DataSets, imported)
	}

	return result, nil
}

func verifyManifestSignature(ctx context.Context, packageDir string, trustedKeys [][]byte) error {
	sigPath := filepath.Join(packageDir, "signatures", "manifest.json.sig")
	if _, err := os.Stat(sigPath); os.IsNotExist(err) {
		return fmt.Errorf("signature file not found")
	}

	keys := trustedKeys
	if len(keys) == 0 {
		bundledKey := filepath.Join(packageDir, "signatures", "cosign.pub")
		if data, err := os.ReadFile(bundledKey); err == nil { //nolint:gosec // G304: path within extracted package
			keys = append(keys, data)
		}
	}
	if len(keys) == 0 {
		return fmt.Errorf("no trusted keys available for verification")
	}

	manifestPath := filepath.Join(packageDir, "manifest.json")
	for _, pubKeyPEM := range keys {
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

func verifyDataChecksums(packageDir string, manifest *ExportManifest) error {
	for _, ds := range manifest.DataSets {
		if ds.SHA256 == "" {
			continue
		}
		fullPath := filepath.Join(packageDir, ds.Path)
		actual, err := hashFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("dataset %q: file not found: %s", ds.Name, ds.Path)
			}
			return fmt.Errorf("dataset %q: hashing %s: %w", ds.Name, ds.Path, err)
		}
		if actual != ds.SHA256 {
			return fmt.Errorf("dataset %q: checksum mismatch: expected %s, got %s", ds.Name, ds.SHA256, actual)
		}
	}

	if manifest.Checksum != "" {
		actual, err := calculateChecksum(packageDir)
		if err != nil {
			return fmt.Errorf("calculating package checksum: %w", err)
		}
		if actual != manifest.Checksum {
			return fmt.Errorf("package checksum mismatch: expected %s, got %s", manifest.Checksum, actual)
		}
	}

	return nil
}
