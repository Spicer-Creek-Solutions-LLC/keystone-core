package blueprint

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBundleManifest_Structure(t *testing.T) {
	manifest := BundleManifest{
		Format:        BundleFormat,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "test-user",
		RootBlueprint: "myorg/web-stack",
		RootVersion:   "1.0.0",
		Description:   "Test bundle",
		Blueprints: []BundledBlueprint{
			{
				Name:         "myorg/web-stack",
				Version:      "1.0.0",
				Path:         "blueprints/myorg/web-stack/1.0.0",
				Checksum:     "abc123",
				Dependencies: []string{"myorg/base@1.0.0"},
				Size:         1024,
			},
		},
		Modules: []BundledModule{
			{
				Name:     "myorg/custom-module",
				Version:  "1.0.0",
				Path:     "modules/myorg/custom-module/1.0.0",
				Checksum: "def456",
				Size:     512,
			},
		},
		Signatures: []BundleSignature{
			{
				KeyID:     "key-1",
				Algorithm: "ed25519",
				Signature: "sig-base64",
				SignedAt:  time.Now().UTC(),
				SignedBy:  "signer@example.com",
			},
		},
		Checksum: "overall-checksum",
		LockFile: map[string]string{
			"myorg/web-stack": "1.0.0",
			"myorg/base":      "1.0.0",
		},
	}

	if manifest.Format != "1.0" {
		t.Errorf("Format = %q, want %q", manifest.Format, "1.0")
	}
	if manifest.RootBlueprint != "myorg/web-stack" {
		t.Errorf("RootBlueprint = %q, want %q", manifest.RootBlueprint, "myorg/web-stack")
	}
	if len(manifest.Blueprints) != 1 {
		t.Errorf("len(Blueprints) = %d, want 1", len(manifest.Blueprints))
	}
	if len(manifest.Modules) != 1 {
		t.Errorf("len(Modules) = %d, want 1", len(manifest.Modules))
	}
	if len(manifest.Signatures) != 1 {
		t.Errorf("len(Signatures) = %d, want 1", len(manifest.Signatures))
	}
}

func TestBundleManifest_JSON(t *testing.T) {
	manifest := BundleManifest{
		Format:        BundleFormat,
		CreatedAt:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		RootBlueprint: "test/blueprint",
		RootVersion:   "1.0.0",
		Blueprints: []BundledBlueprint{
			{
				Name:     "test/blueprint",
				Version:  "1.0.0",
				Path:     "blueprints/test/blueprint/1.0.0",
				Checksum: "abc123",
				Size:     100,
			},
		},
		Checksum: "overall",
	}

	// Marshal to JSON
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}

	// Verify JSON contains expected fields
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"format":"1.0"`) {
		t.Error("JSON missing format field")
	}
	if !strings.Contains(jsonStr, `"root_blueprint":"test/blueprint"`) {
		t.Error("JSON missing root_blueprint field")
	}

	// Unmarshal back
	var decoded BundleManifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal manifest: %v", err)
	}

	if decoded.Format != manifest.Format {
		t.Errorf("Decoded Format = %q, want %q", decoded.Format, manifest.Format)
	}
	if decoded.RootBlueprint != manifest.RootBlueprint {
		t.Errorf("Decoded RootBlueprint = %q, want %q", decoded.RootBlueprint, manifest.RootBlueprint)
	}
}

func TestBundleConfig_Defaults(t *testing.T) {
	config := &BundleConfig{}

	if config.Compress {
		t.Error("Default Compress should be false")
	}
	if config.IncludeModules {
		t.Error("Default IncludeModules should be false")
	}
	if config.OutputPath != "" {
		t.Error("Default OutputPath should be empty")
	}
}

func TestNewBundler(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir(), false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	loader := NewLoader(storage)

	bundler := NewBundler(loader, storage)

	if bundler == nil {
		t.Fatal("NewBundler returned nil")
	}
	if bundler.loader != loader {
		t.Error("Bundler loader not set correctly")
	}
	if bundler.storage != storage {
		t.Error("Bundler storage not set correctly")
	}
	if bundler.resolver == nil {
		t.Error("Bundler resolver is nil")
	}
	if bundler.validator == nil {
		t.Error("Bundler validator is nil")
	}
}

func TestNewBundleInstaller(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir(), false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	blueprintDir := "/path/to/blueprints"
	moduleDir := "/path/to/modules"

	installer := NewBundleInstaller(storage, blueprintDir, moduleDir)

	if installer == nil {
		t.Fatal("NewBundleInstaller returned nil")
	}
	if installer.storage != storage {
		t.Error("BundleInstaller storage not set correctly")
	}
	if installer.blueprintDir != blueprintDir {
		t.Errorf("blueprintDir = %q, want %q", installer.blueprintDir, blueprintDir)
	}
	if installer.moduleDir != moduleDir {
		t.Errorf("moduleDir = %q, want %q", installer.moduleDir, moduleDir)
	}
}

func TestBundler_CreateBundle(t *testing.T) {
	// Create temp directory with a blueprint
	tempDir := t.TempDir()
	blueprintDir := filepath.Join(tempDir, "test-bp")
	if err := os.MkdirAll(blueprintDir, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}

	// Create blueprint.yaml
	blueprintYAML := `apiVersion: blueprints.keystone-core.io/v1
kind: Blueprint
metadata:
  name: test-bp
  version: 1.0.0
  description: Test blueprint

parameters:
  port:
    type: integer
    default: 8080
`
	if err := os.WriteFile(filepath.Join(blueprintDir, "blueprint.yaml"), []byte(blueprintYAML), 0644); err != nil {
		t.Fatalf("Failed to create blueprint.yaml: %v", err)
	}

	// Create bundler
	storage, err := NewLocalStorage(tempDir, false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	loader := NewLoader(storage)
	bundler := NewBundler(loader, storage)

	// Create bundle config
	outputPath := filepath.Join(tempDir, "test-bundle.tar.gz")
	config := &BundleConfig{
		OutputPath: outputPath,
		Compress:   true,
		CreatedBy:  "test",
	}

	// Create bundle
	manifest, err := bundler.CreateBundle(blueprintDir, config)
	if err != nil {
		t.Fatalf("CreateBundle failed: %v", err)
	}

	// Verify manifest
	if manifest.RootBlueprint != "test-bp" {
		t.Errorf("RootBlueprint = %q, want %q", manifest.RootBlueprint, "test-bp")
	}
	if manifest.RootVersion != "1.0.0" {
		t.Errorf("RootVersion = %q, want %q", manifest.RootVersion, "1.0.0")
	}
	if len(manifest.Blueprints) != 1 {
		t.Errorf("len(Blueprints) = %d, want 1", len(manifest.Blueprints))
	}

	// Verify bundle file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Bundle file was not created")
	}

	// Verify bundle is valid gzip
	file, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("Failed to open bundle: %v", err)
	}
	defer file.Close()

	gr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("Bundle is not valid gzip: %v", err)
	}
	defer gr.Close()

	// Verify it's a valid tar
	tr := tar.NewReader(gr)
	foundManifest := false
	for {
		header, err := tr.Next()
		if err != nil {
			break
		}
		if header.Name == "manifest.json" {
			foundManifest = true
		}
	}
	if !foundManifest {
		t.Error("Bundle does not contain manifest.json")
	}
}

func TestBundler_CreateBundle_InvalidBlueprint(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir(), false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	loader := NewLoader(storage)
	bundler := NewBundler(loader, storage)

	_, err = bundler.CreateBundle("/nonexistent/path", nil)
	if err == nil {
		t.Error("Expected error for nonexistent blueprint")
	}
}

func TestBundler_CalculateDirectoryChecksum(t *testing.T) {
	tempDir := t.TempDir()

	// Create some files
	if err := os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}

	bundler := &Bundler{}

	// Calculate checksum
	checksum1, err := bundler.calculateDirectoryChecksum(tempDir)
	if err != nil {
		t.Fatalf("calculateDirectoryChecksum failed: %v", err)
	}

	if checksum1 == "" {
		t.Error("Checksum should not be empty")
	}
	if len(checksum1) != 64 { // SHA256 hex
		t.Errorf("Checksum length = %d, want 64", len(checksum1))
	}

	// Calculate again - should be same
	checksum2, err := bundler.calculateDirectoryChecksum(tempDir)
	if err != nil {
		t.Fatalf("calculateDirectoryChecksum failed: %v", err)
	}

	if checksum1 != checksum2 {
		t.Errorf("Checksums differ: %s != %s", checksum1, checksum2)
	}

	// Modify a file - checksum should change
	if err := os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("modified"), 0644); err != nil {
		t.Fatalf("Failed to modify file1: %v", err)
	}

	checksum3, err := bundler.calculateDirectoryChecksum(tempDir)
	if err != nil {
		t.Fatalf("calculateDirectoryChecksum failed: %v", err)
	}

	if checksum1 == checksum3 {
		t.Error("Checksum should change when file is modified")
	}
}

func TestBundleInstaller_ExtractBundle(t *testing.T) {
	// Create a test bundle
	tempDir := t.TempDir()
	bundleDir := filepath.Join(tempDir, "bundle-contents")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("Failed to create bundle dir: %v", err)
	}

	// Create manifest
	manifest := BundleManifest{
		Format:        BundleFormat,
		RootBlueprint: "test/bp",
		RootVersion:   "1.0.0",
		Blueprints: []BundledBlueprint{
			{Name: "test/bp", Version: "1.0.0", Path: "blueprints/test/bp/1.0.0"},
		},
	}
	manifestData, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create blueprint dir
	bpDir := filepath.Join(bundleDir, "blueprints", "test", "bp", "1.0.0")
	if err := os.MkdirAll(bpDir, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write blueprint: %v", err)
	}

	// Create tar.gz
	bundlePath := filepath.Join(tempDir, "test.tar.gz")
	bundler := &Bundler{}
	if err := bundler.createArchive(bundleDir, bundlePath, true); err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	// Extract
	extractDir := filepath.Join(tempDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		t.Fatalf("Failed to create extract dir: %v", err)
	}

	file, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("Failed to open bundle: %v", err)
	}
	defer file.Close()

	installer := &BundleInstaller{}
	if err := installer.extractBundle(file, extractDir); err != nil {
		t.Fatalf("extractBundle failed: %v", err)
	}

	// Verify extraction
	if _, err := os.Stat(filepath.Join(extractDir, "manifest.json")); os.IsNotExist(err) {
		t.Error("manifest.json not extracted")
	}
	if _, err := os.Stat(filepath.Join(extractDir, "blueprints", "test", "bp", "1.0.0", "blueprint.yaml")); os.IsNotExist(err) {
		t.Error("blueprint.yaml not extracted")
	}
}

func TestBundleInstaller_InstallBundle(t *testing.T) {
	// Create a test bundle
	tempDir := t.TempDir()
	bundleDir := filepath.Join(tempDir, "bundle-contents")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("Failed to create bundle dir: %v", err)
	}

	// Create blueprint dir with content
	bpDir := filepath.Join(bundleDir, "blueprints", "test", "bp", "1.0.0")
	if err := os.MkdirAll(bpDir, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}
	bpContent := []byte("metadata:\n  name: test/bp\n  version: 1.0.0\n")
	if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), bpContent, 0644); err != nil {
		t.Fatalf("Failed to write blueprint: %v", err)
	}

	// Calculate checksum for the blueprint
	installer := &BundleInstaller{}
	checksum, err := installer.calculateChecksum(bpDir)
	if err != nil {
		t.Fatalf("Failed to calculate checksum: %v", err)
	}

	// Create manifest with correct checksum
	manifest := BundleManifest{
		Format:        BundleFormat,
		RootBlueprint: "test/bp",
		RootVersion:   "1.0.0",
		Blueprints: []BundledBlueprint{
			{
				Name:     "test/bp",
				Version:  "1.0.0",
				Path:     "blueprints/test/bp/1.0.0",
				Checksum: checksum,
			},
		},
		Checksum: "overall",
	}
	manifestData, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create tar.gz
	bundlePath := filepath.Join(tempDir, "test.tar.gz")
	bundler := &Bundler{}
	if err := bundler.createArchive(bundleDir, bundlePath, true); err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	// Install
	blueprintInstallDir := filepath.Join(tempDir, "installed-blueprints")
	moduleInstallDir := filepath.Join(tempDir, "installed-modules")

	storage, err := NewLocalStorage(tempDir, false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	fullInstaller := NewBundleInstaller(storage, blueprintInstallDir, moduleInstallDir)

	installedManifest, installErr := fullInstaller.InstallBundle(bundlePath, true)
	if installErr != nil {
		t.Fatalf("InstallBundle failed: %v", installErr)
	}

	if installedManifest.RootBlueprint != "test/bp" {
		t.Errorf("RootBlueprint = %q, want %q", installedManifest.RootBlueprint, "test/bp")
	}

	// Verify blueprint was installed
	installedPath := filepath.Join(blueprintInstallDir, "test/bp", "1.0.0", "blueprint.yaml")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		t.Error("Blueprint was not installed")
	}
}

func TestBundleInstaller_VerifyBundle_ChecksumMismatch(t *testing.T) {
	tempDir := t.TempDir()

	// Create blueprint dir
	bpDir := filepath.Join(tempDir, "blueprints", "test", "bp", "1.0.0")
	if err := os.MkdirAll(bpDir, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write blueprint: %v", err)
	}

	manifest := &BundleManifest{
		Blueprints: []BundledBlueprint{
			{
				Name:     "test/bp",
				Path:     "blueprints/test/bp/1.0.0",
				Checksum: "wrongchecksum",
			},
		},
	}

	installer := &BundleInstaller{}
	err := installer.verifyBundle(tempDir, manifest)
	if err == nil {
		t.Error("Expected error for checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("Error should mention checksum mismatch: %v", err)
	}
}

func TestBundleInstaller_CalculateChecksum_File(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	content := []byte("hello world")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	installer := &BundleInstaller{}
	checksum, err := installer.calculateChecksum(testFile)
	if err != nil {
		t.Fatalf("calculateChecksum failed: %v", err)
	}

	if checksum == "" {
		t.Error("Checksum should not be empty")
	}
	if len(checksum) != 64 {
		t.Errorf("Checksum length = %d, want 64", len(checksum))
	}

	// Same content should give same checksum
	testFile2 := filepath.Join(tempDir, "test2.txt")
	if err := os.WriteFile(testFile2, content, 0644); err != nil {
		t.Fatalf("Failed to create test file 2: %v", err)
	}

	checksum2, err := installer.calculateChecksum(testFile2)
	if err != nil {
		t.Fatalf("calculateChecksum failed: %v", err)
	}

	if checksum != checksum2 {
		t.Error("Same content should give same checksum")
	}
}

func TestBundleInstaller_CopyDirectory(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	dstDir := filepath.Join(tempDir, "dst")

	// Create source structure
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}

	installer := &BundleInstaller{}
	if err := installer.copyDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("copyDirectory failed: %v", err)
	}

	// Verify copy
	content1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	if err != nil {
		t.Fatalf("Failed to read copied file1: %v", err)
	}
	if string(content1) != "content1" {
		t.Errorf("file1 content = %q, want %q", string(content1), "content1")
	}

	content2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
	if err != nil {
		t.Fatalf("Failed to read copied file2: %v", err)
	}
	if string(content2) != "content2" {
		t.Errorf("file2 content = %q, want %q", string(content2), "content2")
	}
}

func TestVerifyBundleSignature_NoSignatures(t *testing.T) {
	// Create a bundle without signatures
	tempDir := t.TempDir()
	bundleDir := filepath.Join(tempDir, "bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("Failed to create bundle dir: %v", err)
	}

	manifest := BundleManifest{
		Format:        BundleFormat,
		RootBlueprint: "test/bp",
		RootVersion:   "1.0.0",
		Signatures:    []BundleSignature{}, // No signatures
	}
	manifestData, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create tar.gz
	bundlePath := filepath.Join(tempDir, "test.tar.gz")
	bundler := &Bundler{}
	if err := bundler.createArchive(bundleDir, bundlePath, true); err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	// Verify should fail
	_, err := VerifyBundleSignature(bundlePath, []string{"trusted-key"})
	if err == nil {
		t.Error("Expected error for bundle without signatures")
	}
	if !strings.Contains(err.Error(), "no signatures") {
		t.Errorf("Error should mention no signatures: %v", err)
	}
}

func TestVerifyBundleSignature_UntrustedKey(t *testing.T) {
	// Create a bundle with signature from untrusted key
	tempDir := t.TempDir()
	bundleDir := filepath.Join(tempDir, "bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("Failed to create bundle dir: %v", err)
	}

	manifest := BundleManifest{
		Format:        BundleFormat,
		RootBlueprint: "test/bp",
		RootVersion:   "1.0.0",
		Signatures: []BundleSignature{
			{
				KeyID:     "untrusted-key",
				Algorithm: "ed25519",
				Signature: "sig",
			},
		},
	}
	manifestData, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create tar.gz
	bundlePath := filepath.Join(tempDir, "test.tar.gz")
	bundler := &Bundler{}
	if err := bundler.createArchive(bundleDir, bundlePath, true); err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	// Verify should fail - untrusted key
	_, err := VerifyBundleSignature(bundlePath, []string{"trusted-key-1", "trusted-key-2"})
	if err == nil {
		t.Error("Expected error for untrusted key")
	}
	if !strings.Contains(err.Error(), "no signature from trusted keys") {
		t.Errorf("Error should mention untrusted key: %v", err)
	}
}

func TestVerifyBundleSignature_TrustedKey(t *testing.T) {
	// Create a bundle with signature from trusted key
	tempDir := t.TempDir()
	bundleDir := filepath.Join(tempDir, "bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("Failed to create bundle dir: %v", err)
	}

	manifest := BundleManifest{
		Format:        BundleFormat,
		RootBlueprint: "test/bp",
		RootVersion:   "1.0.0",
		Signatures: []BundleSignature{
			{
				KeyID:     "trusted-key",
				Algorithm: "ed25519",
				Signature: "sig",
			},
		},
	}
	manifestData, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create tar.gz
	bundlePath := filepath.Join(tempDir, "test.tar.gz")
	bundler := &Bundler{}
	if err := bundler.createArchive(bundleDir, bundlePath, true); err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	// Verify should succeed - trusted key
	result, err := VerifyBundleSignature(bundlePath, []string{"trusted-key"})
	if err != nil {
		t.Fatalf("VerifyBundleSignature failed: %v", err)
	}
	if result.RootBlueprint != "test/bp" {
		t.Errorf("RootBlueprint = %q, want %q", result.RootBlueprint, "test/bp")
	}
}

func TestBundledBlueprint_Structure(t *testing.T) {
	bp := BundledBlueprint{
		Name:         "myorg/test",
		Version:      "1.2.3",
		Path:         "blueprints/myorg/test/1.2.3",
		Checksum:     "abc123def456",
		Dependencies: []string{"dep1@1.0.0", "dep2@2.0.0"},
		Size:         1024,
	}

	if bp.Name != "myorg/test" {
		t.Errorf("Name = %q, want %q", bp.Name, "myorg/test")
	}
	if bp.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", bp.Version, "1.2.3")
	}
	if len(bp.Dependencies) != 2 {
		t.Errorf("len(Dependencies) = %d, want 2", len(bp.Dependencies))
	}
	if bp.Size != 1024 {
		t.Errorf("Size = %d, want 1024", bp.Size)
	}
}

func TestBundledModule_Structure(t *testing.T) {
	mod := BundledModule{
		Name:     "myorg/custom",
		Version:  "1.0.0",
		Path:     "modules/myorg/custom/1.0.0",
		Checksum: "checksum123",
		Size:     512,
	}

	if mod.Name != "myorg/custom" {
		t.Errorf("Name = %q, want %q", mod.Name, "myorg/custom")
	}
	if mod.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", mod.Version, "1.0.0")
	}
	if mod.Size != 512 {
		t.Errorf("Size = %d, want 512", mod.Size)
	}
}

func TestBundleSignature_Structure(t *testing.T) {
	sig := BundleSignature{
		KeyID:     "key-id-123",
		Algorithm: "ed25519",
		Signature: "base64-signature",
		SignedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		SignedBy:  "user@example.com",
	}

	if sig.KeyID != "key-id-123" {
		t.Errorf("KeyID = %q, want %q", sig.KeyID, "key-id-123")
	}
	if sig.Algorithm != "ed25519" {
		t.Errorf("Algorithm = %q, want %q", sig.Algorithm, "ed25519")
	}
	if sig.SignedBy != "user@example.com" {
		t.Errorf("SignedBy = %q, want %q", sig.SignedBy, "user@example.com")
	}
}

func TestBundleFormat_Constant(t *testing.T) {
	if BundleFormat != "1.0" {
		t.Errorf("BundleFormat = %q, want %q", BundleFormat, "1.0")
	}
}

func TestBundler_CreateArchive_Uncompressed(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	// Create test file
	if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create uncompressed archive
	archivePath := filepath.Join(tempDir, "test.tar")
	bundler := &Bundler{}
	if err := bundler.createArchive(srcDir, archivePath, false); err != nil {
		t.Fatalf("createArchive failed: %v", err)
	}

	// Verify it's a valid tar (not gzip)
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("Failed to open archive: %v", err)
	}
	defer file.Close()

	// Should not be valid gzip
	_, err = gzip.NewReader(file)
	if err == nil {
		t.Error("Uncompressed archive should not be valid gzip")
	}

	// Reset and verify it's valid tar
	file.Seek(0, 0)
	tr := tar.NewReader(file)
	header, err := tr.Next()
	if err != nil {
		t.Fatalf("Not a valid tar: %v", err)
	}
	if header.Name != "test.txt" {
		t.Errorf("First entry = %q, want %q", header.Name, "test.txt")
	}
}

func TestBundleInstaller_ExtractTar_PathTraversal(t *testing.T) {
	tempDir := t.TempDir()

	// Create a malicious tar with path traversal
	tarPath := filepath.Join(tempDir, "malicious.tar")
	tarFile, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("Failed to create tar file: %v", err)
	}

	tw := tar.NewWriter(tarFile)
	// Add a file with path traversal
	header := &tar.Header{
		Name: "../../../etc/passwd",
		Mode: 0644,
		Size: 5,
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}
	if _, err := tw.Write([]byte("test\n")); err != nil {
		t.Fatalf("Failed to write content: %v", err)
	}
	tw.Close()
	tarFile.Close()

	// Try to extract
	extractDir := filepath.Join(tempDir, "extract")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		t.Fatalf("Failed to create extract dir: %v", err)
	}

	tarFile, err = os.Open(tarPath)
	if err != nil {
		t.Fatalf("Failed to open tar: %v", err)
	}
	defer tarFile.Close()

	installer := &BundleInstaller{}
	err = installer.extractTar(tarFile, extractDir)
	if err == nil {
		t.Error("Expected error for path traversal attempt")
	}
	if !strings.Contains(err.Error(), "invalid tar entry") {
		t.Errorf("Error should mention invalid tar entry: %v", err)
	}
}
