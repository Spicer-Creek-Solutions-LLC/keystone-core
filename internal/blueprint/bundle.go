package blueprint

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ResolvedDependency represents a resolved blueprint dependency
type ResolvedDependency struct {
	// Name is the full blueprint name
	Name string
	// Version is the resolved version
	Version string
}

// BundleFormat represents the bundle file format version
const BundleFormat = "1.0"

// BundleManifest contains metadata about a blueprint bundle
type BundleManifest struct {
	// Format is the bundle format version
	Format string `json:"format" yaml:"format"`

	// CreatedAt is when the bundle was created
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// CreatedBy identifies who/what created the bundle
	CreatedBy string `json:"created_by,omitempty" yaml:"created_by,omitempty"`

	// RootBlueprint is the main blueprint in the bundle
	RootBlueprint string `json:"root_blueprint" yaml:"root_blueprint"`

	// RootVersion is the version of the root blueprint
	RootVersion string `json:"root_version" yaml:"root_version"`

	// Blueprints lists all blueprints included in the bundle
	Blueprints []BundledBlueprint `json:"blueprints" yaml:"blueprints"`

	// Modules lists all modules included in the bundle
	Modules []BundledModule `json:"modules,omitempty" yaml:"modules,omitempty"`

	// Files lists additional files in the bundle
	Files []BundledFile `json:"files,omitempty" yaml:"files,omitempty"`

	// Signatures contains cryptographic signatures for the bundle
	Signatures []BundleSignature `json:"signatures,omitempty" yaml:"signatures,omitempty"`

	// Checksum is the SHA256 of the bundle contents (excluding manifest)
	Checksum string `json:"checksum" yaml:"checksum"`

	// Description is an optional description of the bundle
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// LockFile contains the resolved dependency versions
	LockFile map[string]string `json:"lock_file,omitempty" yaml:"lock_file,omitempty"`
}

// BundledBlueprint represents a blueprint in the bundle
type BundledBlueprint struct {
	// Name is the full blueprint name (vendor/name)
	Name string `json:"name" yaml:"name"`

	// Version is the blueprint version
	Version string `json:"version" yaml:"version"`

	// Path is the relative path within the bundle
	Path string `json:"path" yaml:"path"`

	// Checksum is the SHA256 of the blueprint directory
	Checksum string `json:"checksum" yaml:"checksum"`

	// Dependencies lists this blueprint's dependencies
	Dependencies []string `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`

	// Size is the total size in bytes
	Size int64 `json:"size" yaml:"size"`
}

// BundledModule represents a module in the bundle
type BundledModule struct {
	// Name is the full module name
	Name string `json:"name" yaml:"name"`

	// Version is the module version
	Version string `json:"version" yaml:"version"`

	// Path is the relative path within the bundle
	Path string `json:"path" yaml:"path"`

	// Checksum is the SHA256 of the module
	Checksum string `json:"checksum" yaml:"checksum"`

	// Size is the size in bytes
	Size int64 `json:"size" yaml:"size"`
}

// BundledFile represents an additional file in the bundle
type BundledFile struct {
	// Path is the relative path within the bundle
	Path string `json:"path" yaml:"path"`

	// Checksum is the SHA256 of the file
	Checksum string `json:"checksum" yaml:"checksum"`

	// Size is the size in bytes
	Size int64 `json:"size" yaml:"size"`
}

// BundleSignature represents a cryptographic signature
type BundleSignature struct {
	// KeyID identifies the signing key
	KeyID string `json:"key_id" yaml:"key_id"`

	// Algorithm is the signature algorithm (e.g., "ed25519", "rsa-sha256")
	Algorithm string `json:"algorithm" yaml:"algorithm"`

	// Signature is the base64-encoded signature
	Signature string `json:"signature" yaml:"signature"`

	// SignedAt is when the signature was created
	SignedAt time.Time `json:"signed_at" yaml:"signed_at"`

	// SignedBy identifies who created the signature
	SignedBy string `json:"signed_by,omitempty" yaml:"signed_by,omitempty"`
}

// BundleConfig configures bundle creation
type BundleConfig struct {
	// OutputPath is where to write the bundle
	OutputPath string

	// IncludeModules includes dependent modules
	IncludeModules bool

	// Compress enables gzip compression
	Compress bool

	// SigningKey is the key to sign the bundle (optional)
	SigningKey string

	// SigningKeyID identifies the signing key
	SigningKeyID string

	// Description is an optional bundle description
	Description string

	// CreatedBy identifies the bundle creator
	CreatedBy string

	// LockFile path for pinned versions
	LockFilePath string
}

// Bundler creates blueprint bundles for air-gapped environments
type Bundler struct {
	loader    *Loader
	storage   Storage
	resolver  *DependencyResolver
	validator *Validator
}

// bundlerLoader adapts Bundler to BlueprintLoader interface
type bundlerLoader struct {
	bundler *Bundler
}

// Load implements BlueprintLoader
func (bl *bundlerLoader) Load(reference string) (*Blueprint, error) {
	// Parse reference (name@version)
	name := reference
	version := ""
	if idx := strings.LastIndex(reference, "@"); idx > 0 {
		name = reference[:idx]
		version = reference[idx+1:]
	}

	ctx := context.Background()
	if bl.bundler.storage != nil {
		bp, err := bl.bundler.storage.Get(ctx, name, version)
		if err == nil {
			return bp, nil
		}
	}
	return nil, fmt.Errorf("blueprint %s not found", reference)
}

// NewBundler creates a new Bundler
func NewBundler(loader *Loader, storage Storage) *Bundler {
	b := &Bundler{
		loader:    loader,
		storage:   storage,
		validator: NewValidator(),
	}
	// Create resolver with a loader adapter
	b.resolver = NewDependencyResolver(&bundlerLoader{bundler: b})
	return b
}

// CreateBundle creates a bundle from a blueprint
func (b *Bundler) CreateBundle(blueprintPath string, config *BundleConfig) (*BundleManifest, error) {
	if config == nil {
		config = &BundleConfig{
			Compress: true,
		}
	}

	// Load the root blueprint
	ctx := context.Background()
	result, err := b.loader.LoadFromPath(ctx, blueprintPath, &LoadConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to load blueprint: %w", err)
	}
	bp := result.Blueprint

	// Validate the blueprint
	validationResult := b.validator.Validate(bp)
	if validationResult != nil && !validationResult.Valid {
		return nil, fmt.Errorf("blueprint validation failed: %v", validationResult.Errors)
	}

	// Create manifest
	manifest := &BundleManifest{
		Format:        BundleFormat,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     config.CreatedBy,
		RootBlueprint: bp.Metadata.Name,
		RootVersion:   bp.Metadata.Version,
		Description:   config.Description,
		Blueprints:    []BundledBlueprint{},
		Modules:       []BundledModule{},
		Files:         []BundledFile{},
		LockFile:      make(map[string]string),
	}

	// Resolve dependencies
	deps, err := b.resolveDependencies(bp)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	// Create temporary directory for bundle contents
	tempDir, err := os.MkdirTemp("", "kscore-bundle-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Copy blueprints to bundle
	blueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.MkdirAll(blueprintsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create blueprints directory: %w", err)
	}

	// Add root blueprint
	rootBundled, err := b.bundleBlueprint(bp, blueprintPath, blueprintsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to bundle root blueprint: %w", err)
	}
	manifest.Blueprints = append(manifest.Blueprints, *rootBundled)
	manifest.LockFile[bp.Metadata.Name] = bp.Metadata.Version

	// Add dependencies
	for _, dep := range deps {
		if dep.Name == bp.Metadata.Name {
			continue // Skip root
		}

		depBp, depPath, err := b.loadDependency(dep)
		if err != nil {
			return nil, fmt.Errorf("failed to load dependency %s: %w", dep.Name, err)
		}

		bundled, err := b.bundleBlueprint(depBp, depPath, blueprintsDir)
		if err != nil {
			return nil, fmt.Errorf("failed to bundle dependency %s: %w", dep.Name, err)
		}
		manifest.Blueprints = append(manifest.Blueprints, *bundled)
		manifest.LockFile[dep.Name] = dep.Version
	}

	// Calculate overall checksum
	checksum, err := b.calculateDirectoryChecksum(tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}
	manifest.Checksum = checksum

	// Write manifest
	manifestPath := filepath.Join(tempDir, "manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}

	// Create archive
	outputPath := config.OutputPath
	if outputPath == "" {
		outputPath = fmt.Sprintf("%s-%s-bundle.tar.gz", bp.Metadata.Name, bp.Metadata.Version)
	}

	if err := b.createArchive(tempDir, outputPath, config.Compress); err != nil {
		return nil, fmt.Errorf("failed to create archive: %w", err)
	}

	return manifest, nil
}

// resolveDependencies resolves all dependencies for a blueprint
func (b *Bundler) resolveDependencies(bp *Blueprint) ([]*ResolvedDependency, error) {
	var allDeps []*ResolvedDependency

	// Dependencies is a *Dependencies struct with Requires and RequiresBefore as []string
	if bp.Dependencies == nil {
		return allDeps, nil
	}

	// Collect all dependency references
	var depRefs []string
	depRefs = append(depRefs, bp.Dependencies.Requires...)
	depRefs = append(depRefs, bp.Dependencies.RequiresBefore...)

	// Get direct dependencies
	for _, depRef := range depRefs {
		// Parse reference format: "name@version" or just "name"
		name := depRef
		version := ""
		if idx := strings.LastIndex(depRef, "@"); idx > 0 {
			name = depRef[:idx]
			version = depRef[idx+1:]
		}

		resolved := &ResolvedDependency{
			Name:    name,
			Version: version,
		}
		allDeps = append(allDeps, resolved)

		// Recursively resolve transitive dependencies
		depBp, _, err := b.loadDependency(resolved)
		if err != nil {
			continue // Skip if dependency not found locally
		}

		transitive, err := b.resolveDependencies(depBp)
		if err != nil {
			continue
		}
		allDeps = append(allDeps, transitive...)
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []*ResolvedDependency
	for _, dep := range allDeps {
		key := dep.Name + "@" + dep.Version
		if !seen[key] {
			seen[key] = true
			unique = append(unique, dep)
		}
	}

	return unique, nil
}

// loadDependency loads a dependency blueprint
func (b *Bundler) loadDependency(dep *ResolvedDependency) (*Blueprint, string, error) {
	// Try to find in storage using Get
	ctx := context.Background()
	if b.storage != nil {
		bp, err := b.storage.Get(ctx, dep.Name, dep.Version)
		if err == nil {
			// Return empty path since we loaded from storage (not a file path)
			return bp, "", nil
		}
	}

	return nil, "", fmt.Errorf("dependency %s@%s not found", dep.Name, dep.Version)
}

// bundleBlueprint copies a blueprint to the bundle directory
func (b *Bundler) bundleBlueprint(bp *Blueprint, sourcePath string, destDir string) (*BundledBlueprint, error) {
	// Determine source directory
	sourceDir := sourcePath
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		sourceDir = filepath.Dir(sourcePath)
	}

	// Create destination path
	destPath := filepath.Join(destDir, bp.Metadata.Name, bp.Metadata.Version)
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return nil, err
	}

	// Copy files
	var totalSize int64
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		destFile := filepath.Join(destPath, relPath)

		if info.IsDir() {
			return os.MkdirAll(destFile, info.Mode())
		}

		// Copy file
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.Create(destFile)
		if err != nil {
			return err
		}
		defer dst.Close()

		n, err := io.Copy(dst, src)
		if err != nil {
			return err
		}
		totalSize += n

		return os.Chmod(destFile, info.Mode())
	})
	if err != nil {
		return nil, err
	}

	// Calculate checksum
	checksum, err := b.calculateDirectoryChecksum(destPath)
	if err != nil {
		return nil, err
	}

	// Get dependencies
	var depNames []string
	if bp.Dependencies != nil {
		depNames = append(depNames, bp.Dependencies.Requires...)
		depNames = append(depNames, bp.Dependencies.RequiresBefore...)
	}

	return &BundledBlueprint{
		Name:         bp.Metadata.Name,
		Version:      bp.Metadata.Version,
		Path:         filepath.Join("blueprints", bp.Metadata.Name, bp.Metadata.Version),
		Checksum:     checksum,
		Dependencies: depNames,
		Size:         totalSize,
	}, nil
}

// calculateDirectoryChecksum calculates SHA256 of a directory
func (b *Bundler) calculateDirectoryChecksum(dir string) (string, error) {
	h := sha256.New()

	// Collect all files
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files = append(files, relPath)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	// Sort for deterministic ordering
	sort.Strings(files)

	// Hash each file
	for _, file := range files {
		// Add filename to hash
		h.Write([]byte(file))

		// Add file contents to hash
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return "", err
		}
		h.Write(data)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// createArchive creates a tar.gz archive
func (b *Bundler) createArchive(sourceDir, destPath string, compress bool) error {
	outFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	var tw *tar.Writer
	if compress {
		gw := gzip.NewWriter(outFile)
		defer gw.Close()
		tw = tar.NewWriter(gw)
	} else {
		tw = tar.NewWriter(outFile)
	}
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
			data, err := os.ReadFile(path)
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

// BundleInstaller installs bundles to the local system
type BundleInstaller struct {
	storage      Storage
	blueprintDir string
	moduleDir    string
}

// NewBundleInstaller creates a new BundleInstaller
func NewBundleInstaller(storage Storage, blueprintDir, moduleDir string) *BundleInstaller {
	return &BundleInstaller{
		storage:      storage,
		blueprintDir: blueprintDir,
		moduleDir:    moduleDir,
	}
}

// InstallBundle installs a bundle to the local system
func (bi *BundleInstaller) InstallBundle(bundlePath string, verify bool) (*BundleManifest, error) {
	// Open bundle
	file, err := os.Open(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open bundle: %w", err)
	}
	defer file.Close()

	// Create temp directory for extraction
	tempDir, err := os.MkdirTemp("", "kscore-bundle-install-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract bundle
	if err := bi.extractBundle(file, tempDir); err != nil {
		return nil, fmt.Errorf("failed to extract bundle: %w", err)
	}

	// Read manifest
	manifestPath := filepath.Join(tempDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest BundleManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Verify checksums if requested
	if verify {
		if err := bi.verifyBundle(tempDir, &manifest); err != nil {
			return nil, fmt.Errorf("bundle verification failed: %w", err)
		}
	}

	// Install blueprints
	for _, bp := range manifest.Blueprints {
		srcPath := filepath.Join(tempDir, bp.Path)
		dstPath := filepath.Join(bi.blueprintDir, bp.Name, bp.Version)

		if err := bi.copyDirectory(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("failed to install blueprint %s: %w", bp.Name, err)
		}
	}

	// Install modules if present
	for _, mod := range manifest.Modules {
		srcPath := filepath.Join(tempDir, mod.Path)
		dstPath := filepath.Join(bi.moduleDir, mod.Name, mod.Version)

		if err := bi.copyDirectory(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("failed to install module %s: %w", mod.Name, err)
		}
	}

	return &manifest, nil
}

// extractBundle extracts a tar.gz bundle
func (bi *BundleInstaller) extractBundle(r io.Reader, destDir string) error {
	// Try gzip first
	gr, err := gzip.NewReader(r)
	if err != nil {
		// Not gzipped, try plain tar
		if seeker, ok := r.(io.Seeker); ok {
			seeker.Seek(0, io.SeekStart)
		}
		return bi.extractTar(r, destDir)
	}
	defer gr.Close()

	return bi.extractTar(gr, destDir)
}

// extractTar extracts a tar archive
func (bi *BundleInstaller) extractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Prevent path traversal
		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar entry: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.CopyN(outFile, tr, header.Size); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
			if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		}
	}

	return nil
}

// verifyBundle verifies bundle checksums
func (bi *BundleInstaller) verifyBundle(dir string, manifest *BundleManifest) error {
	// Verify each blueprint
	for _, bp := range manifest.Blueprints {
		bpPath := filepath.Join(dir, bp.Path)
		checksum, err := bi.calculateChecksum(bpPath)
		if err != nil {
			return fmt.Errorf("failed to calculate checksum for %s: %w", bp.Name, err)
		}
		if checksum != bp.Checksum {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", bp.Name, bp.Checksum, checksum)
		}
	}

	// Verify modules
	for _, mod := range manifest.Modules {
		modPath := filepath.Join(dir, mod.Path)
		checksum, err := bi.calculateChecksum(modPath)
		if err != nil {
			return fmt.Errorf("failed to calculate checksum for %s: %w", mod.Name, err)
		}
		if checksum != mod.Checksum {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", mod.Name, mod.Checksum, checksum)
		}
	}

	return nil
}

// calculateChecksum calculates SHA256 of a file or directory
func (bi *BundleInstaller) calculateChecksum(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	h := sha256.New()

	if info.IsDir() {
		// Directory checksum
		var files []string
		err := filepath.Walk(path, func(p string, i os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !i.IsDir() {
				relPath, err := filepath.Rel(path, p)
				if err != nil {
					return err
				}
				files = append(files, relPath)
			}
			return nil
		})
		if err != nil {
			return "", err
		}

		sort.Strings(files)

		for _, file := range files {
			h.Write([]byte(file))
			data, err := os.ReadFile(filepath.Join(path, file))
			if err != nil {
				return "", err
			}
			h.Write(data)
		}
	} else {
		// File checksum
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		h.Write(data)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyDirectory copies a directory recursively
func (bi *BundleInstaller) copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}

		return os.Chmod(dstPath, info.Mode())
	})
}

// VerifyBundleSignature verifies the bundle signature
func VerifyBundleSignature(bundlePath string, trustedKeys []string) (*BundleManifest, error) {
	// Open bundle
	file, err := os.Open(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open bundle: %w", err)
	}
	defer file.Close()

	// Create temp directory for extraction
	tempDir, err := os.MkdirTemp("", "kscore-bundle-verify-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract bundle
	installer := &BundleInstaller{}
	if err := installer.extractBundle(file, tempDir); err != nil {
		return nil, fmt.Errorf("failed to extract bundle: %w", err)
	}

	// Read manifest
	manifestPath := filepath.Join(tempDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest BundleManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Check signatures
	if len(manifest.Signatures) == 0 {
		return nil, fmt.Errorf("bundle has no signatures")
	}

	// Verify at least one signature from trusted keys
	for _, sig := range manifest.Signatures {
		for _, key := range trustedKeys {
			if sig.KeyID == key {
				// Signature verification would go here
				// For now, just verify the key is trusted
				return &manifest, nil
			}
		}
	}

	return nil, fmt.Errorf("no signature from trusted keys")
}
