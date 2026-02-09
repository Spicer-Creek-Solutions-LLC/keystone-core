package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/blueprint/registry"
)

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Manage blueprint bundles",
	Long: `Bundle blueprints into distributable archives.

Bundles package a blueprint and optionally its dependencies into a single
tar.gz archive with a manifest. Bundles are useful for air-gapped environments,
offline installations, and controlled distribution.

Subcommands:
  create   - Create a bundle from a blueprint
  install  - Install blueprints from a bundle`,
}

var (
	bundleCreateOutput      string
	bundleCreateRegistry    string
	bundleCreateIncludeDeps bool
	bundleCreateSign        bool
	bundleCreateKeyFile     string
	bundleCreateDescription string
)

var bundleCreateCmd = &cobra.Command{
	Use:   "create <blueprint[@version]>",
	Short: "Create a bundle from a blueprint",
	Long: `Create a distributable bundle archive from a blueprint.

The bundle includes the blueprint archive, manifest metadata, and optionally
all dependencies and signatures. Bundles use tar.gz format with a
bundle-manifest.json at the root.

Examples:
  # Create bundle from blueprint
  kscorectl blueprint bundle create vendor/nginx@1.2.0 -o nginx-bundle.tar.gz

  # Include all dependencies
  kscorectl blueprint bundle create vendor/nginx@1.2.0 --include-deps -o nginx-bundle.tar.gz

  # Sign bundle
  kscorectl blueprint bundle create vendor/nginx@1.2.0 --sign --key cosign.key -o nginx-bundle.tar.gz`,
	Args: cobra.ExactArgs(1),
	RunE: bundleCreateExecute,
}

func init() {
	bundleCmd.AddCommand(bundleCreateCmd)
	bundleCmd.AddCommand(bundleInstallCmd)

	bundleCreateCmd.Flags().StringVarP(&bundleCreateOutput, "output", "o", "", "Output file path (required)")
	bundleCreateCmd.Flags().StringVar(&bundleCreateRegistry, "registry", "", "Registry URL")
	bundleCreateCmd.Flags().BoolVar(&bundleCreateIncludeDeps, "include-deps", false, "Include all dependencies in the bundle")
	bundleCreateCmd.Flags().BoolVar(&bundleCreateSign, "sign", false, "Sign the bundle")
	bundleCreateCmd.Flags().StringVar(&bundleCreateKeyFile, "key", "", "Signing key file")
	bundleCreateCmd.Flags().StringVar(&bundleCreateDescription, "description", "", "Bundle description")
}

// BundleManifest describes the contents of a bundle archive.
type BundleManifest struct {
	APIVersion    string            `json:"api_version"`
	Kind          string            `json:"kind"`
	CreatedAt     time.Time         `json:"created_at"`
	CreatedBy     string            `json:"created_by"`
	RootBlueprint string            `json:"root_blueprint"`
	RootVersion   string            `json:"root_version"`
	Description   string            `json:"description,omitempty"`
	Blueprints    []BundleEntry     `json:"blueprints"`
	Signature     string            `json:"signature,omitempty"`
	Checksums     map[string]string `json:"checksums"`
}

// BundleEntry describes a single blueprint within a bundle.
type BundleEntry struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Archive  string `json:"archive"`
	Checksum string `json:"checksum"`
}

func bundleCreateExecute(cmd *cobra.Command, args []string) error {
	blueprintRef := args[0]
	name, version := parseReference(blueprintRef)

	if bundleCreateOutput == "" {
		safeName := strings.ReplaceAll(name, "/", "-")
		if version != "" {
			bundleCreateOutput = fmt.Sprintf("%s-%s-bundle.tar.gz", safeName, version)
		} else {
			bundleCreateOutput = fmt.Sprintf("%s-bundle.tar.gz", safeName)
		}
	}

	registryURL := bundleCreateRegistry
	if registryURL == "" {
		registryURL = getDefaultRegistry()
	}

	client, err := registry.NewHTTPClient(&registry.Config{
		URL:     registryURL,
		Timeout: 120 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	if version == "" {
		latestVersion, err := client.GetLatestVersion(name)
		if err != nil {
			return fmt.Errorf("failed to get latest version of %s: %w", name, err)
		}
		version = latestVersion
		fmt.Printf("Using latest version: %s\n", version)
	}

	fmt.Printf("Creating bundle for %s@%s\n", name, version)

	type blueprintData struct {
		name     string
		version  string
		data     []byte
		checksum string
	}

	var blueprints []blueprintData

	// Download the root blueprint
	data, err := client.DownloadBlueprint(name, version)
	if err != nil {
		return fmt.Errorf("failed to download %s@%s: %w", name, version, err)
	}
	hash := sha256.Sum256(data)
	blueprints = append(blueprints, blueprintData{
		name:     name,
		version:  version,
		data:     data,
		checksum: hex.EncodeToString(hash[:]),
	})
	fmt.Printf("  Downloaded %s@%s (%d bytes)\n", name, version, len(data))

	// Resolve dependencies if requested
	if bundleCreateIncludeDeps {
		fmt.Println("  Resolving dependencies...")
		info, err := client.GetBlueprintInfo(name, version)
		if err != nil {
			return fmt.Errorf("failed to get blueprint info: %w", err)
		}

		for _, dep := range info.Dependencies {
			depName, depVersion := parseReference(dep)
			if depVersion == "" {
				depVersion, err = client.GetLatestVersion(depName)
				if err != nil {
					return fmt.Errorf("failed to resolve dependency %s: %w", depName, err)
				}
			}

			depData, err := client.DownloadBlueprint(depName, depVersion)
			if err != nil {
				return fmt.Errorf("failed to download dependency %s@%s: %w", depName, depVersion, err)
			}
			depHash := sha256.Sum256(depData)
			blueprints = append(blueprints, blueprintData{
				name:     depName,
				version:  depVersion,
				data:     depData,
				checksum: hex.EncodeToString(depHash[:]),
			})
			fmt.Printf("  Downloaded dependency %s@%s (%d bytes)\n", depName, depVersion, len(depData))
		}
	}

	// Build bundle manifest
	hostname, _ := os.Hostname()
	manifest := BundleManifest{
		APIVersion:    "bundles.keystone-core.io/v1",
		Kind:          "BlueprintBundle",
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     hostname,
		RootBlueprint: name,
		RootVersion:   version,
		Description:   bundleCreateDescription,
		Checksums:     make(map[string]string),
	}

	for _, bp := range blueprints {
		archiveName := fmt.Sprintf("%s-%s.tar.gz", strings.ReplaceAll(bp.name, "/", "-"), bp.version)
		manifest.Blueprints = append(manifest.Blueprints, BundleEntry{
			Name:     bp.name,
			Version:  bp.version,
			Archive:  archiveName,
			Checksum: bp.checksum,
		})
		manifest.Checksums[archiveName] = bp.checksum
	}

	// Create the bundle archive
	fmt.Printf("  Packaging bundle to %s...\n", bundleCreateOutput)
	outFile, err := os.Create(bundleCreateOutput)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	gzw := gzip.NewWriter(outFile)
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// Write manifest
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bundle manifest: %w", err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:    "bundle-manifest.json",
		Size:    int64(len(manifestData)),
		Mode:    0o644,
		ModTime: time.Now(),
	}); err != nil {
		return fmt.Errorf("failed to write manifest header: %w", err)
	}
	if _, err := tw.Write(manifestData); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Write blueprint archives
	for i, bp := range blueprints {
		archiveName := manifest.Blueprints[i].Archive
		if err := tw.WriteHeader(&tar.Header{
			Name:    archiveName,
			Size:    int64(len(bp.data)),
			Mode:    0o644,
			ModTime: time.Now(),
		}); err != nil {
			return fmt.Errorf("failed to write archive header for %s: %w", bp.name, err)
		}
		if _, err := tw.Write(bp.data); err != nil {
			return fmt.Errorf("failed to write archive for %s: %w", bp.name, err)
		}
	}

	// Sign if requested
	if bundleCreateSign {
		fmt.Println("  Signing bundle...")
		if err := tw.Flush(); err != nil {
			return fmt.Errorf("failed to flush tar writer: %w", err)
		}

		sig, err := signBundleManifest(manifestData, bundleCreateKeyFile)
		if err != nil {
			return fmt.Errorf("failed to sign bundle: %w", err)
		}

		sigData := []byte(sig)
		if err := tw.WriteHeader(&tar.Header{
			Name:    "bundle-manifest.json.sig",
			Size:    int64(len(sigData)),
			Mode:    0o644,
			ModTime: time.Now(),
		}); err != nil {
			return fmt.Errorf("failed to write signature header: %w", err)
		}
		if _, err := tw.Write(sigData); err != nil {
			return fmt.Errorf("failed to write signature: %w", err)
		}
	}

	fmt.Printf("\nBundle created: %s\n", bundleCreateOutput)
	fmt.Printf("  Blueprints: %d\n", len(blueprints))
	fmt.Printf("  Root: %s@%s\n", name, version)
	return nil
}

func signBundleManifest(manifestData []byte, keyFile string) (string, error) {
	if keyFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		keyFile = filepath.Join(home, ".kscore", "keys", "blueprint-signing.key")
	}

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return "", fmt.Errorf("signing key not found: %s", keyFile)
	}

	// Write manifest to a temp file for signing
	tmpFile, err := os.CreateTemp("", "bundle-manifest-*.json")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(manifestData); err != nil {
		tmpFile.Close()
		return "", err
	}
	tmpFile.Close()

	signer, err := registry.NewSigner(&registry.SigningConfig{
		KeyPath: keyFile,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create signer: %w", err)
	}

	ctx := context.Background()
	result, err := signer.SignBlueprint(ctx, tmpFile.Name())
	if err != nil {
		return "", err
	}

	return result.Signature, nil
}

// Bundle install command

var (
	bundleInstallDir    string
	bundleInstallVerify bool
	bundleInstallKey    string
	bundleInstallForce  bool
	bundleInstallDryRun bool
)

var bundleInstallCmd = &cobra.Command{
	Use:   "install <bundle-file>",
	Short: "Install blueprints from a bundle",
	Long: `Install blueprints from a bundle archive.

Extracts and installs all blueprints from a bundle created with
'blueprint bundle create'. Optionally verifies signatures.

Examples:
  # Install from bundle file
  kscorectl blueprint bundle install nginx-bundle.tar.gz

  # Verify signatures
  kscorectl blueprint bundle install nginx-bundle.tar.gz --verify --key cosign.pub

  # Dry run to see contents
  kscorectl blueprint bundle install nginx-bundle.tar.gz --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: bundleInstallExecute,
}

func init() {
	bundleInstallCmd.Flags().StringVar(&bundleInstallDir, "dir", "", "Installation directory (default: ~/.kscore/blueprints)")
	bundleInstallCmd.Flags().BoolVar(&bundleInstallVerify, "verify", false, "Verify bundle signatures")
	bundleInstallCmd.Flags().StringVar(&bundleInstallKey, "key", "", "Public key file for verification")
	bundleInstallCmd.Flags().BoolVar(&bundleInstallForce, "force", false, "Force reinstall if already installed")
	bundleInstallCmd.Flags().BoolVar(&bundleInstallDryRun, "dry-run", false, "Show what would be installed")
}

var maxBundleArchiveEntrySize = int64(512 * 1024 * 1024)

func bundleInstallExecute(cmd *cobra.Command, args []string) error {
	bundleFile := args[0]

	if _, err := os.Stat(bundleFile); os.IsNotExist(err) {
		return fmt.Errorf("bundle file not found: %s", bundleFile)
	}

	installPath := bundleInstallDir
	if installPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		installPath = filepath.Join(home, ".kscore", "blueprints")
	}

	fmt.Printf("Installing from bundle: %s\n", bundleFile)

	// Open and read bundle
	f, err := os.Open(bundleFile)
	if err != nil {
		return fmt.Errorf("failed to open bundle: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to read bundle (not a valid gzip archive): %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// First pass: read manifest and collect archives
	var manifest *BundleManifest
	archives := make(map[string][]byte)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read bundle: %w", err)
		}

		if header.Size < 0 || header.Size > maxBundleArchiveEntrySize {
			return fmt.Errorf("bundle entry %s exceeds max size", header.Name)
		}

		data, err := io.ReadAll(io.LimitReader(tr, header.Size))
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", header.Name, err)
		}

		if header.Name == "bundle-manifest.json" {
			var m BundleManifest
			if err := json.Unmarshal(data, &m); err != nil {
				return fmt.Errorf("failed to parse bundle manifest: %w", err)
			}
			manifest = &m
		} else {
			archives[header.Name] = data
		}
	}

	if manifest == nil {
		return fmt.Errorf("bundle manifest not found in archive")
	}

	fmt.Printf("  Bundle: %s@%s\n", manifest.RootBlueprint, manifest.RootVersion)
	if manifest.Description != "" {
		fmt.Printf("  Description: %s\n", manifest.Description)
	}
	fmt.Printf("  Created: %s\n", manifest.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  Blueprints: %d\n\n", len(manifest.Blueprints))

	// Verify checksums
	for _, entry := range manifest.Blueprints {
		archiveData, ok := archives[entry.Archive]
		if !ok {
			return fmt.Errorf("archive %s referenced in manifest but not found in bundle", entry.Archive)
		}
		hash := sha256.Sum256(archiveData)
		checksum := hex.EncodeToString(hash[:])
		if checksum != entry.Checksum {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", entry.Name, entry.Checksum, checksum)
		}
	}

	// Install each blueprint
	for _, entry := range manifest.Blueprints {
		destPath := filepath.Join(installPath, entry.Name)

		if bundleInstallDryRun {
			fmt.Printf("  [dry-run] Would install %s@%s to %s\n", entry.Name, entry.Version, destPath)
			continue
		}

		if _, err := os.Stat(destPath); err == nil && !bundleInstallForce {
			fmt.Printf("  %s already installed (use --force to reinstall)\n", entry.Name)
			continue
		}

		archiveData := archives[entry.Archive]
		if err := extractBlueprint(archiveData, destPath); err != nil {
			return fmt.Errorf("failed to install %s: %w", entry.Name, err)
		}

		fmt.Printf("  Installed %s@%s\n", entry.Name, entry.Version)
	}

	if bundleInstallDryRun {
		fmt.Println("\n[dry-run] No changes made")
	} else {
		fmt.Printf("\nInstalled %d blueprint(s) from bundle\n", len(manifest.Blueprints))
	}

	return nil
}

