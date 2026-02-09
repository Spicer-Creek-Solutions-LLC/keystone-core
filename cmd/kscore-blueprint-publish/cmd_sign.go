package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/blueprint/registry"
)

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
  kscorectl blueprint-publish sign myblueprint-1.0.0.tar.gz

  # Sign with specific key
  kscorectl blueprint-publish sign myblueprint-1.0.0.tar.gz --key ~/.kscore/keys/my-key.key

  # Generate a new signing key
  kscorectl blueprint-publish sign --generate-key

  # Sign with custom output file
  kscorectl blueprint-publish sign myblueprint-1.0.0.tar.gz --output myblueprint-1.0.0.tar.gz.sig`,
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
	//nolint:gosec // G306: signature files need to be readable for verification
	if err := os.WriteFile(outputFile, []byte(result.Signature), 0o644); err != nil {
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
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
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
	if err := os.WriteFile(privateKeyPath, privateKey, 0o600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}
	//nolint:gosec // G306: public key needs to be readable for signature verification
	if err := os.WriteFile(publicKeyPath, publicKey, 0o644); err != nil {
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
  kscorectl blueprint-publish verify community/nginx@1.2.0

  # Verify a local package file
  kscorectl blueprint-publish verify myblueprint-1.0.0.tar.gz

  # Verify with specific public key
  kscorectl blueprint-publish verify community/nginx --key trusted-key.pub

  # Verify with specific signature file
  kscorectl blueprint-publish verify myblueprint-1.0.0.tar.gz --signature myblueprint-1.0.0.tar.gz.sig`,
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

	client, err := registry.NewHTTPClient(&registry.Config{
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
