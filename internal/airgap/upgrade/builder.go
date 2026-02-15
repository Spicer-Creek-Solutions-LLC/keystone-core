package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
)

// BuilderConfig configures the upgrade package builder.
type BuilderConfig struct {
	FromVersion     string
	ToVersion       string
	Platform        bootstrap.Platform
	BuildDir        string // Directory containing new version binaries ({os}/{arch}/)
	OutputPath      string
	CreatedBy       string
	SigningKey      []byte // PEM private key; nil = unsigned
	ModulesDir      string // Updated modules to include
	MigrationsDir   string // SQL/script migrations to include
	PreScriptsDir   string // Pre-upgrade scripts
	PostScriptsDir  string // Post-upgrade scripts
	ConfigChanges   []ConfigChange
	BreakingChanges []string
}

// Builder creates upgrade packages for air-gapped deployments.
type Builder struct {
	config BuilderConfig
}

// NewBuilder creates a new upgrade package builder.
func NewBuilder(config BuilderConfig) (*Builder, error) {
	if config.FromVersion == "" {
		return nil, fmt.Errorf("from_version is required")
	}
	if config.ToVersion == "" {
		return nil, fmt.Errorf("to_version is required")
	}
	if err := config.Platform.Validate(); err != nil {
		return nil, fmt.Errorf("platform: %w", err)
	}
	if config.BuildDir == "" {
		return nil, fmt.Errorf("build directory is required")
	}
	return &Builder{config: config}, nil
}

// Build creates an upgrade package and returns the manifest.
func (b *Builder) Build(ctx context.Context) (*Manifest, error) {
	staging, err := os.MkdirTemp("", "kscore-upgrade-*")
	if err != nil {
		return nil, fmt.Errorf("creating staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	// Collect binaries from build directory
	cr, err := bootstrap.CollectBinaries(b.config.BuildDir, b.config.Platform, b.config.ToVersion, staging)
	if err != nil {
		return nil, fmt.Errorf("collecting binaries: %w", err)
	}
	for _, w := range cr.Warnings {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", w)
	}

	manifest := &Manifest{
		SchemaVersion:        SchemaVersion,
		FromVersion:          b.config.FromVersion,
		ToVersion:            b.config.ToVersion,
		Platform:             b.config.Platform,
		Created:              time.Now().UTC(),
		CreatedBy:            b.config.CreatedBy,
		BreakingChanges:      b.config.BreakingChanges,
		Components:           cr.Entries,
		ConfigChanges:        b.config.ConfigChanges,
		RequiresVerification: b.config.SigningKey != nil,
	}

	// Bundle modules if provided
	if b.config.ModulesDir != "" {
		modules, err := bootstrap.BundleModules(b.config.ModulesDir, staging)
		if err != nil {
			return nil, fmt.Errorf("bundling modules: %w", err)
		}
		manifest.Modules = modules
	}

	// Copy migrations if provided
	if b.config.MigrationsDir != "" {
		migrations, err := bundleMigrations(b.config.MigrationsDir, staging)
		if err != nil {
			return nil, fmt.Errorf("bundling migrations: %w", err)
		}
		manifest.Migrations = migrations
	}

	// Copy pre-upgrade scripts if provided
	if b.config.PreScriptsDir != "" {
		scripts, err := bundleScripts(b.config.PreScriptsDir, staging, "pre-upgrade")
		if err != nil {
			return nil, fmt.Errorf("bundling pre-upgrade scripts: %w", err)
		}
		manifest.PreScripts = scripts
	}

	// Copy post-upgrade scripts if provided
	if b.config.PostScriptsDir != "" {
		scripts, err := bundleScripts(b.config.PostScriptsDir, staging, "post-upgrade")
		if err != nil {
			return nil, fmt.Errorf("bundling post-upgrade scripts: %w", err)
		}
		manifest.PostScripts = scripts
	}

	// Calculate checksum
	checksum, err := bootstrap.CalculateChecksum(staging)
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
		signer, err := bootstrap.NewPackageSigner(b.config.SigningKey)
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
		outputPath = fmt.Sprintf("keystone-upgrade-%s-to-%s-%s-%s.tar.gz",
			b.config.FromVersion, b.config.ToVersion,
			b.config.Platform.OS, b.config.Platform.Arch)
	}

	if err := createArchive(staging, outputPath); err != nil {
		return nil, fmt.Errorf("creating archive: %w", err)
	}

	return manifest, nil
}

func bundleMigrations(srcDir, stagingDir string) ([]MigrationEntry, error) {
	dstDir := filepath.Join(stagingDir, "migrations")
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating migrations dir: %w", err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("reading migrations dir: %w", err)
	}

	// Sort entries by name for deterministic ordering
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var migrations []MigrationEntry
	order := 1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		srcPath := filepath.Join(srcDir, name)
		dstPath := filepath.Join(dstDir, name)

		data, err := os.ReadFile(srcPath) //nolint:gosec // G304: path from controlled source dir
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		//nolint:gosec // G306: migration files need to be readable
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", name, err)
		}

		migrations = append(migrations, MigrationEntry{
			Name:  name,
			Path:  "migrations/" + name,
			Order: order,
		})
		order++
	}

	return migrations, nil
}

func bundleScripts(srcDir, stagingDir, subdir string) ([]ScriptEntry, error) {
	dstDir := filepath.Join(stagingDir, "scripts", subdir)
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating scripts dir: %w", err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("reading scripts dir: %w", err)
	}

	var scripts []ScriptEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		srcPath := filepath.Join(srcDir, name)
		dstPath := filepath.Join(dstDir, name)

		data, err := os.ReadFile(srcPath) //nolint:gosec // G304: path from controlled source dir
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		//nolint:gosec // G306: scripts need to be executable
		if err := os.WriteFile(dstPath, data, 0o755); err != nil {
			return nil, fmt.Errorf("writing %s: %w", name, err)
		}

		scripts = append(scripts, ScriptEntry{
			Name:     name,
			Path:     filepath.Join("scripts", subdir, name),
			Required: true,
		})
	}

	return scripts, nil
}
