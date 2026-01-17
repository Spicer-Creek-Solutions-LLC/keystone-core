package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/blueprint"
	"github.com/shawnbutts/keystone-core/pkg/blueprint/registry"
)

var (
	publishRegistry string
	publishSign     bool
	publishKeyFile  string
	publishForce    bool
	publishDryRun   bool
)

var publishCmd = &cobra.Command{
	Use:   "publish [path]",
	Short: "Publish blueprint to registry",
	Long: `Publish a blueprint to a registry.

Packages the blueprint directory and uploads it to the specified registry.
By default, the blueprint will be signed before publishing.

Examples:
  # Publish blueprint in current directory
  kscorectl blueprint-publish publish .

  # Publish to specific registry
  kscorectl blueprint-publish publish . --registry https://blueprints.example.com

  # Publish without signing (not recommended)
  kscorectl blueprint-publish publish . --no-sign

  # Publish with specific signing key
  kscorectl blueprint-publish publish . --key ~/.kscore/keys/signing.key`,
	Args: cobra.MaximumNArgs(1),
	RunE: publishExecute,
}

func init() {
	publishCmd.Flags().StringVar(&publishRegistry, "registry", "", "Registry URL")
	publishCmd.Flags().BoolVar(&publishSign, "sign", true, "Sign before publishing")
	publishCmd.Flags().StringVar(&publishKeyFile, "key", "", "Signing key file")
	publishCmd.Flags().BoolVar(&publishForce, "force", false, "Force overwrite if version exists")
	publishCmd.Flags().BoolVar(&publishDryRun, "dry-run", false, "Show what would be published")
}

func publishExecute(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Find and parse blueprint.yaml
	manifestPath := filepath.Join(path, "blueprint.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return fmt.Errorf("blueprint.yaml not found in %s", path)
	}

	bp, err := blueprint.ParseManifestFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to parse blueprint.yaml: %w", err)
	}

	// Validate before publishing
	validator := blueprint.NewValidator()
	result := validator.Validate(bp)
	if len(result.Errors) > 0 {
		fmt.Println("Validation errors:")
		for _, e := range result.Errors {
			fmt.Printf("  ✗ %s\n", e)
		}
		return fmt.Errorf("blueprint validation failed")
	}

	fmt.Printf("Publishing %s@%s\n", bp.Metadata.Name, bp.Metadata.Version)

	// Get registry URL
	registryURL := publishRegistry
	if registryURL == "" {
		registryURL = getDefaultRegistry()
	}
	fmt.Printf("Registry: %s\n", registryURL)

	if publishDryRun {
		fmt.Println("\n[dry-run] Would publish:")
		fmt.Printf("  Name: %s\n", bp.Metadata.Name)
		fmt.Printf("  Version: %s\n", bp.Metadata.Version)
		fmt.Printf("  Signed: %v\n", publishSign)
		return nil
	}

	// Create package
	fmt.Println("Packaging blueprint...")
	packagePath, err := packageBlueprint(path, bp)
	if err != nil {
		return fmt.Errorf("failed to package blueprint: %w", err)
	}
	defer os.Remove(packagePath)

	// Sign if requested
	var signature string
	if publishSign {
		fmt.Println("Signing blueprint...")
		signature, err = signPackage(packagePath, publishKeyFile)
		if err != nil {
			return fmt.Errorf("failed to sign blueprint: %w", err)
		}
	}

	// Publish to registry
	fmt.Println("Uploading to registry...")
	client, err := registry.NewHTTPClient(&registry.RegistryConfig{
		URL:     registryURL,
		Timeout: 120 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	ctx := context.Background()

	// Read package data
	packageData, err := os.ReadFile(packagePath)
	if err != nil {
		return fmt.Errorf("failed to read package: %w", err)
	}

	// Read manifest data
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Calculate checksum
	checksum, err := checksumFile(packagePath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	// Publish using the client
	_, err = client.PublishBlueprint(&registry.PublishRequest{
		Name:     bp.Metadata.Name,
		Version:  bp.Metadata.Version,
		Manifest: manifestData,
		Archive:  packageData,
		Checksum: checksum,
	})
	if err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}
	_ = ctx // ctx available for future use

	// Store signature if present
	if signature != "" {
		// Write signature to a temp file and upload
		sigPath := packagePath + ".sig"
		if err := os.WriteFile(sigPath, []byte(signature), 0644); err != nil {
			return fmt.Errorf("failed to write signature: %w", err)
		}
		defer os.Remove(sigPath)
	}

	fmt.Printf("\n✓ Published %s@%s to %s\n", bp.Metadata.Name, bp.Metadata.Version, registryURL)
	return nil
}

func packageBlueprint(path string, bp *blueprint.Blueprint) (string, error) {
	// Create temporary file for the package
	tmpFile, err := os.CreateTemp("", "blueprint-*.tar.gz")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()

	// Create gzip writer
	gzw := gzip.NewWriter(tmpFile)
	tw := tar.NewWriter(gzw)

	// Walk the blueprint directory and add files
	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(path, filePath)
		if err != nil {
			return err
		}

		// Skip hidden files and directories (except .kscore)
		base := filepath.Base(filePath)
		if base != "." && base[0] == '.' && base != ".kscore" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Write file content if not a directory
		if !info.IsDir() {
			f, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", err
	}

	// Close writers
	if err := tw.Close(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := gzw.Close(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	return tmpPath, nil
}

func signPackage(packagePath, keyFile string) (string, error) {
	// Default key location
	if keyFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		keyFile = filepath.Join(home, ".kscore", "keys", "blueprint-signing.key")
	}

	// Check key exists
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return "", fmt.Errorf("signing key not found: %s (use 'kscorectl blueprint-publish sign --generate-key' to create one)", keyFile)
	}

	// Create signer
	signer, err := registry.NewSigner(&registry.SigningConfig{
		KeyPath: keyFile,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create signer: %w", err)
	}

	// Sign the package
	ctx := context.Background()
	result, err := signer.SignBlueprint(ctx, packagePath)
	if err != nil {
		return "", err
	}

	return result.Signature, nil
}

// checksumFile calculates SHA256 checksum of a file
func checksumFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// getDefaultRegistry returns the default registry URL
func getDefaultRegistry() string {
	if url := os.Getenv("KSCORE_BLUEPRINT_REGISTRY"); url != "" {
		return url
	}
	return "https://blueprints.keystonecore.io"
}

// parseReference splits a blueprint reference into name and version
func parseReference(ref string) (name, version string) {
	if idx := strings.LastIndex(ref, "@"); idx != -1 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, ""
}
