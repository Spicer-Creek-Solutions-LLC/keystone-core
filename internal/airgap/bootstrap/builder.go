package bootstrap

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BuilderConfig configures the package builder.
type BuilderConfig struct {
	Version   string
	Platform  Platform
	BuildDir  string
	OutputPath string
	CreatedBy  string

	SigningKey []byte // PEM private key; nil = unsigned

	IncludeModules    bool
	IncludeBlueprints bool
	IncludeDocs       bool

	ModulesDir    string
	BlueprintsDir string
	DocsDir       string
	PoliciesDir   string
}

// Builder creates self-contained bootstrap packages.
type Builder struct {
	config BuilderConfig
}

// NewBuilder creates a new package builder.
func NewBuilder(config BuilderConfig) (*Builder, error) {
	if config.Version == "" {
		return nil, fmt.Errorf("version is required")
	}
	if err := config.Platform.Validate(); err != nil {
		return nil, fmt.Errorf("platform: %w", err)
	}
	if config.BuildDir == "" {
		return nil, fmt.Errorf("build directory is required")
	}
	return &Builder{config: config}, nil
}

// Build creates a bootstrap package and returns the manifest.
func (b *Builder) Build(ctx context.Context) (*Manifest, error) {
	staging, err := os.MkdirTemp("", "kscore-bootstrap-*")
	if err != nil {
		return nil, fmt.Errorf("creating staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	// Collect binaries
	cr, err := CollectBinaries(b.config.BuildDir, b.config.Platform, b.config.Version, staging)
	if err != nil {
		return nil, fmt.Errorf("collecting binaries: %w", err)
	}
	for _, w := range cr.Warnings {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", w)
	}

	manifest := &Manifest{
		SchemaVersion:        SchemaVersion,
		Version:              b.config.Version,
		Platform:             b.config.Platform,
		Created:              time.Now().UTC(),
		CreatedBy:            b.config.CreatedBy,
		Components:           cr.Entries,
		RequiresVerification: b.config.SigningKey != nil,
	}

	// Bundle optional content
	if b.config.IncludeModules && b.config.ModulesDir != "" {
		modules, err := BundleModules(b.config.ModulesDir, staging)
		if err != nil {
			return nil, fmt.Errorf("bundling modules: %w", err)
		}
		manifest.Modules = modules
	}

	if b.config.IncludeBlueprints && b.config.BlueprintsDir != "" {
		blueprints, err := BundleBlueprints(b.config.BlueprintsDir, staging)
		if err != nil {
			return nil, fmt.Errorf("bundling blueprints: %w", err)
		}
		manifest.Blueprints = blueprints
	}

	if b.config.PoliciesDir != "" {
		policies, err := BundlePolicies(b.config.PoliciesDir, staging)
		if err != nil {
			return nil, fmt.Errorf("bundling policies: %w", err)
		}
		manifest.Policies = policies
	}

	if b.config.IncludeDocs && b.config.DocsDir != "" {
		docs, err := BundleDocs(b.config.DocsDir, staging)
		if err != nil {
			return nil, fmt.Errorf("bundling docs: %w", err)
		}
		manifest.Docs = docs
	}

	// Bundle config templates and install script
	params := DefaultConfigParams(b.config.Version)
	configEntries, err := BundleConfigTemplates(params, b.config.Platform, staging)
	if err != nil {
		return nil, fmt.Errorf("bundling config templates: %w", err)
	}
	// Separate install script from config templates
	for _, entry := range configEntries {
		if entry.Name == "install.sh" {
			continue
		}
		manifest.ConfigTemplates = append(manifest.ConfigTemplates, entry)
	}

	// Calculate checksum over all staged content
	checksum, err := CalculateChecksum(staging)
	if err != nil {
		return nil, fmt.Errorf("calculating checksum: %w", err)
	}
	manifest.Checksum = checksum

	// Write manifest
	manifestPath := filepath.Join(staging, "manifest.json")
	if err := WriteManifest(manifest, manifestPath); err != nil {
		return nil, fmt.Errorf("writing manifest: %w", err)
	}

	// Sign if key provided
	if b.config.SigningKey != nil {
		signer, err := NewPackageSigner(b.config.SigningKey)
		if err != nil {
			return nil, fmt.Errorf("creating signer: %w", err)
		}
		if err := signer.SignManifest(ctx, staging); err != nil {
			return nil, fmt.Errorf("signing manifest: %w", err)
		}
		if err := signer.WritePublicKey(staging); err != nil {
			return nil, fmt.Errorf("writing public key: %w", err)
		}
	}

	// Create archive
	outputPath := b.config.OutputPath
	if outputPath == "" {
		outputPath = fmt.Sprintf("keystone-bootstrap-%s-%s-%s.tar.gz",
			b.config.Version, b.config.Platform.OS, b.config.Platform.Arch)
	}

	if err := createArchive(staging, outputPath); err != nil {
		return nil, fmt.Errorf("creating archive: %w", err)
	}

	return manifest, nil
}

func createArchive(sourceDir, destPath string) error {
	if dir := filepath.Dir(destPath); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}

	outFile, err := os.Create(destPath) //nolint:gosec // G304: output path from config
	if err != nil {
		return err
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			data, err := os.ReadFile(path) //nolint:gosec // G304: path from filepath.Walk within sourceDir
			if err != nil {
				return err
			}
			if _, err := tw.Write(data); err != nil {
				return err
			}
		}

		return nil
	})
}
