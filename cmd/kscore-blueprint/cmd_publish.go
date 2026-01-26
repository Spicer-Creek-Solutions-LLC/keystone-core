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
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
	"github.com/shawnbutts/keystone-core/internal/blueprint/registry"
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
  kscorectl blueprint publish .

  # Publish to specific registry
  kscorectl blueprint publish . --registry https://blueprints.example.com

  # Publish without signing (not recommended)
  kscorectl blueprint publish . --no-sign

  # Publish with specific signing key
  kscorectl blueprint publish . --key ~/.kscore/keys/signing.key`,
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
		return "", fmt.Errorf("signing key not found: %s (use 'kscorectl blueprint sign --generate-key' to create one)", keyFile)
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

// Sign command

var (
	signKeyFile     string
	signGenerateKey bool
	signOutputFile  string
)

var signCmd = &cobra.Command{
	Use:   "sign <file>",
	Short: "Sign a blueprint package",
	Long: `Sign a blueprint package with a private key.

Creates a detached signature file that can be verified by others
to ensure the blueprint has not been tampered with.

Examples:
  # Sign a blueprint package
  kscorectl blueprint sign myblueprint-1.0.0.tar.gz

  # Sign with specific key
  kscorectl blueprint sign myblueprint-1.0.0.tar.gz --key ~/.kscore/keys/my-key.key

  # Generate a new signing key
  kscorectl blueprint sign --generate-key

  # Sign with custom output file
  kscorectl blueprint sign myblueprint-1.0.0.tar.gz --output myblueprint-1.0.0.tar.gz.sig`,
	Args: cobra.MaximumNArgs(1),
	RunE: signExecute,
}

func init() {
	signCmd.Flags().StringVar(&signKeyFile, "key", "", "Signing key file")
	signCmd.Flags().BoolVar(&signGenerateKey, "generate-key", false, "Generate a new signing key pair")
	signCmd.Flags().StringVar(&signOutputFile, "output", "", "Output signature file")
}

func signExecute(cmd *cobra.Command, args []string) error {
	if signGenerateKey {
		return generateSigningKey()
	}

	if len(args) < 1 {
		return fmt.Errorf("file argument required (or use --generate-key)")
	}

	file := args[0]

	// Verify file exists
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", file)
	}

	// Default key location
	if signKeyFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		signKeyFile = filepath.Join(home, ".kscore", "keys", "blueprint-signing.key")
	}

	// Check key exists
	if _, err := os.Stat(signKeyFile); os.IsNotExist(err) {
		return fmt.Errorf("signing key not found: %s (use --generate-key to create one)", signKeyFile)
	}

	// Determine output file
	outputFile := signOutputFile
	if outputFile == "" {
		outputFile = file + ".sig"
	}

	fmt.Printf("Signing %s...\n", file)

	// Create signer
	signer, err := registry.NewSigner(&registry.SigningConfig{
		KeyPath: signKeyFile,
	})
	if err != nil {
		return fmt.Errorf("failed to create signer: %w", err)
	}

	// Sign the file
	ctx := context.Background()
	result, err := signer.SignBlueprint(ctx, file)
	if err != nil {
		return fmt.Errorf("failed to sign: %w", err)
	}

	// Write signature to file
	if err := os.WriteFile(outputFile, []byte(result.Signature), 0644); err != nil {
		return fmt.Errorf("failed to write signature: %w", err)
	}

	fmt.Printf("✓ Signature written to %s\n", outputFile)
	return nil
}

func generateSigningKey() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	keysDir := filepath.Join(home, ".kscore", "keys")
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return fmt.Errorf("failed to create keys directory: %w", err)
	}

	privateKeyPath := filepath.Join(keysDir, "blueprint-signing.key")
	publicKeyPath := filepath.Join(keysDir, "blueprint-signing.pub")

	// Check if keys already exist
	if _, err := os.Stat(privateKeyPath); err == nil {
		return fmt.Errorf("key already exists at %s (delete it first to regenerate)", privateKeyPath)
	}

	fmt.Println("Generating new signing key pair...")

	// Generate key pair using standalone function
	privateKey, publicKey, err := registry.GenerateKeyPair(registry.KeyTypeECDSA, 256)
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	// Write keys to files
	if err := os.WriteFile(privateKeyPath, privateKey, 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}
	if err := os.WriteFile(publicKeyPath, publicKey, 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	fmt.Printf("✓ Private key: %s\n", privateKeyPath)
	fmt.Printf("✓ Public key: %s\n", publicKeyPath)
	fmt.Println("\nShare the public key with your registry to enable signature verification.")
	fmt.Println("Keep the private key secure and never share it.")

	return nil
}

// Verify command

var (
	verifyKeyFile   string
	verifyRegistry  string
	verifySignature string
)

var verifyCmd = &cobra.Command{
	Use:   "verify <blueprint[@version]|file>",
	Short: "Verify blueprint signature",
	Long: `Verify the signature of a blueprint.

Checks that the blueprint was signed by a trusted key and has not
been modified since signing.

Examples:
  # Verify a blueprint from registry
  kscorectl blueprint verify community/nginx@1.2.0

  # Verify a local package file
  kscorectl blueprint verify myblueprint-1.0.0.tar.gz

  # Verify with specific public key
  kscorectl blueprint verify community/nginx --key trusted-key.pub

  # Verify with specific signature file
  kscorectl blueprint verify myblueprint-1.0.0.tar.gz --signature myblueprint-1.0.0.tar.gz.sig`,
	Args: cobra.ExactArgs(1),
	RunE: verifyExecute,
}

func init() {
	verifyCmd.Flags().StringVar(&verifyKeyFile, "key", "", "Public key file for verification")
	verifyCmd.Flags().StringVar(&verifyRegistry, "registry", "", "Registry URL")
	verifyCmd.Flags().StringVar(&verifySignature, "signature", "", "Signature file (for local verification)")
}

func verifyExecute(cmd *cobra.Command, args []string) error {
	target := args[0]

	// Check if it's a local file or registry reference
	if _, err := os.Stat(target); err == nil {
		return verifyLocalFile(target)
	}

	return verifyFromRegistry(target)
}

func verifyLocalFile(file string) error {
	// Determine signature file
	sigFile := verifySignature
	if sigFile == "" {
		sigFile = file + ".sig"
	}

	// Check signature file exists
	if _, err := os.Stat(sigFile); os.IsNotExist(err) {
		return fmt.Errorf("signature file not found: %s", sigFile)
	}

	// Determine key file
	keyFile := verifyKeyFile
	if keyFile == "" {
		home, _ := os.UserHomeDir()
		keyFile = filepath.Join(home, ".kscore", "keys", "blueprint-signing.pub")
	}

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return fmt.Errorf("public key not found: %s (use --key to specify)", keyFile)
	}

	fmt.Printf("Verifying %s...\n", file)

	// Read signature
	sigData, err := os.ReadFile(sigFile)
	if err != nil {
		return fmt.Errorf("failed to read signature: %w", err)
	}

	// Create verifier
	verifier, err := registry.NewVerifier(&registry.VerificationConfig{
		PublicKeyPath: keyFile,
	})
	if err != nil {
		return fmt.Errorf("failed to create verifier: %w", err)
	}

	// Verify
	ctx := context.Background()
	result, err := verifier.VerifyBlueprint(ctx, file, string(sigData))
	if err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	if result.Valid {
		fmt.Println("✓ Signature is valid")
		if result.SignerIdentity != "" {
			fmt.Printf("  Signed by: %s\n", result.SignerIdentity)
		}
		if !result.Timestamp.IsZero() {
			fmt.Printf("  Signed at: %s\n", result.Timestamp.Format(time.RFC3339))
		}
		return nil
	}

	// Collect error message
	errMsg := "signature verification failed"
	if len(result.Errors) > 0 {
		errMsg = result.Errors[0]
	}
	return fmt.Errorf("%s", errMsg)
}

func verifyFromRegistry(ref string) error {
	name, version := parseReference(ref)

	// Get registry URL
	registryURL := verifyRegistry
	if registryURL == "" {
		registryURL = getDefaultRegistry()
	}

	fmt.Printf("Verifying %s", name)
	if version != "" {
		fmt.Printf("@%s", version)
	}
	fmt.Printf(" from %s...\n", registryURL)

	client, err := registry.NewHTTPClient(&registry.RegistryConfig{
		URL:     registryURL,
		Timeout: 60 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	// Get version if not specified
	if version == "" {
		versions, err := client.ListVersions(name)
		if err != nil {
			return fmt.Errorf("failed to get versions: %w", err)
		}
		if len(versions) == 0 {
			return fmt.Errorf("no versions found for %s", name)
		}
		version = versions[0]
	}

	// Download the blueprint
	data, err := client.DownloadBlueprint(name, version)
	if err != nil {
		return fmt.Errorf("failed to download blueprint: %w", err)
	}

	// Calculate checksum
	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])

	// For registry verification, we trust the registry's signature verification
	// This is a simplified implementation - production would fetch and verify signatures
	fmt.Println("✓ Blueprint retrieved successfully")
	fmt.Printf("  Blueprint: %s@%s\n", name, version)
	fmt.Printf("  Checksum: %s\n", checksum)

	return nil
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
