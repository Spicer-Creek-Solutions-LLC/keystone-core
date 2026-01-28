package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
)

// Publisher handles building and publishing blueprints to a registry.
type Publisher struct {
	client PublishableRegistry
	config *PublisherConfig
}

// PublisherConfig holds configuration for the publisher.
type PublisherConfig struct {
	// SkipValidation skips blueprint validation before publishing.
	SkipValidation bool

	// SigningKey is the path to the cosign private key for signing.
	SigningKey string

	// SigningKeyPassword is the password for the signing key.
	SigningKeyPassword string

	// SigningCert is the path to the signing certificate (optional).
	SigningCert string

	// KeylessSign enables keyless signing using OIDC (Fulcio).
	KeylessSign bool

	// OIDCIssuer is the OIDC issuer URL for keyless signing.
	OIDCIssuer string

	// OIDCClientID is the OIDC client ID for keyless signing.
	OIDCClientID string

	// IncludePatterns specifies which files to include (glob patterns).
	// Default: all files
	IncludePatterns []string

	// ExcludePatterns specifies which files to exclude (glob patterns).
	// Default: .git, .gitignore, *.swp, etc.
	ExcludePatterns []string

	// StripPrefix removes this prefix from file paths in the archive.
	StripPrefix string

	// Compression sets the gzip compression level (1-9, default 6).
	Compression int
}

// DefaultPublisherConfig returns a PublisherConfig with default values.
func DefaultPublisherConfig() *PublisherConfig {
	return &PublisherConfig{
		ExcludePatterns: []string{
			".git",
			".git/**",
			".gitignore",
			".gitattributes",
			"*.swp",
			"*.swo",
			"*~",
			".DS_Store",
			"Thumbs.db",
			"__pycache__",
			"*.pyc",
			".idea",
			".vscode",
			"node_modules",
		},
		Compression: 6,
	}
}

// NewPublisher creates a new Publisher.
func NewPublisher(client PublishableRegistry, config *PublisherConfig) *Publisher {
	if config == nil {
		config = DefaultPublisherConfig()
	}
	return &Publisher{
		client: client,
		config: config,
	}
}

// BuildResult contains the result of building a blueprint archive.
type BuildResult struct {
	// Archive is the built archive data.
	Archive []byte

	// Manifest is the manifest data.
	Manifest []byte

	// Blueprint is the parsed blueprint manifest.
	Blueprint *blueprint.Blueprint

	// Checksum is the SHA-256 checksum of the archive.
	Checksum string

	// Size is the archive size in bytes.
	Size int64

	// Files is the list of files included in the archive.
	Files []string
}

// Build builds a blueprint archive from a directory.
func (p *Publisher) Build(dir string) (*BuildResult, error) {
	// Read and validate manifest
	manifestPath := filepath.Join(dir, "blueprint.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		manifestPath = filepath.Join(dir, "blueprint.yml")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("blueprint.yaml not found in %s", dir)
		}
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	bp, err := blueprint.ParseManifest(manifestData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate blueprint
	if !p.config.SkipValidation {
		if err := p.validateBlueprint(bp, dir); err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}
	}

	// Build archive
	archive, files, err := p.buildArchive(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to build archive: %w", err)
	}

	// Calculate checksum
	hash := sha256.Sum256(archive)
	checksum := "sha256:" + hex.EncodeToString(hash[:])

	return &BuildResult{
		Archive:   archive,
		Manifest:  manifestData,
		Blueprint: bp,
		Checksum:  checksum,
		Size:      int64(len(archive)),
		Files:     files,
	}, nil
}

// Publish publishes a blueprint to the registry.
func (p *Publisher) Publish(dir string) (*PublishResult, error) {
	// Build the blueprint
	build, err := p.Build(dir)
	if err != nil {
		return nil, err
	}

	return p.PublishBuild(build)
}

// PublishBuild publishes a pre-built blueprint to the registry.
func (p *Publisher) PublishBuild(build *BuildResult) (*PublishResult, error) {
	// Create publish request
	req := &PublishRequest{
		Name:     build.Blueprint.FullName(),
		Version:  build.Blueprint.Metadata.Version,
		Archive:  build.Archive,
		Manifest: build.Manifest,
		Checksum: build.Checksum,
	}

	// Sign if key is provided
	if p.config.SigningKey != "" {
		sig, cert, err := p.signArchive(build.Archive)
		if err != nil {
			return nil, fmt.Errorf("failed to sign archive: %w", err)
		}
		req.Signature = sig
		req.Certificate = cert
	}

	// Publish to registry
	return p.client.PublishBlueprint(req)
}

// validateBlueprint validates a blueprint before publishing.
func (p *Publisher) validateBlueprint(bp *blueprint.Blueprint, dir string) error {
	// Validate required fields
	if bp.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if bp.Metadata.Version == "" {
		return fmt.Errorf("metadata.version is required")
	}

	// Validate version format
	if !isValidVersion(bp.Metadata.Version) {
		return fmt.Errorf("invalid version format: %s (expected semver)", bp.Metadata.Version)
	}

	// Validate states directory exists
	statesDir := filepath.Join(dir, "states")
	if info, err := os.Stat(statesDir); err != nil || !info.IsDir() {
		return fmt.Errorf("states/ directory is required")
	}

	// Validate that at least one state file exists
	stateFiles, err := filepath.Glob(filepath.Join(statesDir, "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to list state files: %w", err)
	}
	ymlFiles, _ := filepath.Glob(filepath.Join(statesDir, "*.yml"))
	stateFiles = append(stateFiles, ymlFiles...)
	if len(stateFiles) == 0 {
		return fmt.Errorf("no state files found in states/")
	}

	// Validate parameter schemas
	for name, param := range bp.Parameters {
		if param.Type == "" {
			return fmt.Errorf("parameter %q missing type", name)
		}
		if !isValidParameterType(param.Type) {
			return fmt.Errorf("parameter %q has invalid type: %s", name, param.Type)
		}
	}

	return nil
}

// buildArchive creates a tar.gz archive of the blueprint directory.
func (p *Publisher) buildArchive(dir string) ([]byte, []string, error) {
	var buf bytes.Buffer
	var files []string

	// Create gzip writer
	gw, err := gzip.NewWriterLevel(&buf, p.config.Compression)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer gw.Close()

	// Create tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Walk directory
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Skip root
		if relPath == "." {
			return nil
		}

		// Apply strip prefix
		if p.config.StripPrefix != "" {
			relPath = strings.TrimPrefix(relPath, p.config.StripPrefix)
			relPath = strings.TrimPrefix(relPath, "/")
		}

		// Check exclude patterns
		if p.shouldExclude(relPath, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check include patterns
		if !p.shouldInclude(relPath) {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}

		// Use relative path in archive
		header.Name = relPath

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("failed to read symlink: %w", err)
			}
			header.Linkname = link
		}

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		// Write file content for regular files
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			defer file.Close()

			if _, err := io.Copy(tw, file); err != nil {
				return fmt.Errorf("failed to write file content: %w", err)
			}

			files = append(files, relPath)
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	// Close writers to flush
	if err := tw.Close(); err != nil {
		return nil, nil, fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), files, nil
}

// shouldExclude checks if a path should be excluded.
func (p *Publisher) shouldExclude(path string, isDir bool) bool {
	for _, pattern := range p.config.ExcludePatterns {
		// Check exact match
		if pattern == path {
			return true
		}

		// Check glob pattern
		matched, _ := filepath.Match(pattern, path)
		if matched {
			return true
		}

		// Check base name
		base := filepath.Base(path)
		matched, _ = filepath.Match(pattern, base)
		if matched {
			return true
		}

		// Check directory prefix patterns (e.g., ".git/**")
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "/**")
			if strings.HasPrefix(path, prefix+"/") || path == prefix {
				return true
			}
		}
	}
	return false
}

// shouldInclude checks if a path should be included.
func (p *Publisher) shouldInclude(path string) bool {
	// If no include patterns, include everything
	if len(p.config.IncludePatterns) == 0 {
		return true
	}

	for _, pattern := range p.config.IncludePatterns {
		matched, _ := filepath.Match(pattern, path)
		if matched {
			return true
		}

		// Check base name
		base := filepath.Base(path)
		matched, _ = filepath.Match(pattern, base)
		if matched {
			return true
		}
	}
	return false
}

// signArchive signs the archive using the configured signing key.
func (p *Publisher) signArchive(archive []byte) (signature, certificate []byte, err error) {
	// Create signer configuration
	signingConfig := &SigningConfig{
		KeyPath:     p.config.SigningKey,
		KeyPassword: p.config.SigningKeyPassword,
		Format:      SignatureFormatCosign,
	}

	// Add certificate path if configured
	if p.config.SigningCert != "" {
		signingConfig.CertPath = p.config.SigningCert
	}

	// Create signer
	signer, err := NewSigner(signingConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create signer: %w", err)
	}

	// Sign the archive
	ctx := context.Background()
	result, err := signer.Sign(ctx, archive)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign archive: %w", err)
	}

	// Decode signature from base64
	sigBytes, err := base64.StdEncoding.DecodeString(result.Signature)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode signature: %w", err)
	}

	// Decode certificate if present
	var certBytes []byte
	if result.Certificate != "" {
		certBytes, err = base64.StdEncoding.DecodeString(result.Certificate)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode certificate: %w", err)
		}
	}

	return sigBytes, certBytes, nil
}

// isValidVersion checks if a version string is valid semver.
func isValidVersion(version string) bool {
	// Simple semver validation
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}

	// Allow prerelease suffix
	if len(parts) == 3 {
		lastPart := parts[2]
		if idx := strings.IndexAny(lastPart, "-+"); idx >= 0 {
			parts[2] = lastPart[:idx]
		}
	}

	for _, part := range parts {
		if part == "" {
			return false
		}
		// Check that it's a number (simple check)
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}

	return true
}

// isValidParameterType checks if a parameter type is valid.
func isValidParameterType(t string) bool {
	validTypes := map[string]bool{
		"string":  true,
		"number":  true,
		"integer": true,
		"boolean": true,
		"array":   true,
		"object":  true,
	}
	return validTypes[t]
}

// ExtractBlueprint extracts a blueprint archive to a directory.
func ExtractBlueprint(archive []byte, destDir string) error {
	// Create gzip reader
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	// Create tar reader
	tr := tar.NewReader(gr)

	// Extract files
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Sanitize path to prevent path traversal
		targetPath := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid path in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

		case tar.TypeReg:
			// Create parent directory
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			// Create file
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.CopyN(file, tr, header.Size); err != nil {
				file.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			file.Close()

		case tar.TypeSymlink:
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return fmt.Errorf("failed to create symlink: %w", err)
			}
		}

		// Set modification time
		if err := os.Chtimes(targetPath, time.Now(), header.ModTime); err != nil {
			// Ignore chtimes errors for symlinks
			if header.Typeflag != tar.TypeSymlink {
				return fmt.Errorf("failed to set modification time: %w", err)
			}
		}
	}

	return nil
}

// VerifyChecksum verifies the checksum of archive data.
func VerifyChecksum(data []byte, expected string) error {
	hash := sha256.Sum256(data)
	computed := "sha256:" + hex.EncodeToString(hash[:])

	// Handle checksums without prefix
	if !strings.HasPrefix(expected, "sha256:") {
		expected = "sha256:" + expected
	}

	if !strings.EqualFold(computed, expected) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, computed)
	}
	return nil
}
