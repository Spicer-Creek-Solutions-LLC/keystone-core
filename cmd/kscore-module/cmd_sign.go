package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/verify"
)

var (
	signPrivateKey string
	signOutput     string
	signForce      bool
	signGenKey     string
	signKeyBits    int
)

var signCmd = &cobra.Command{
	Use:   "sign <module-path>",
	Short: "Sign a module with a private key",
	Long: `Sign a module archive with a private key.

Creates a detached signature file (.sig) that can be verified
using the corresponding public key.

Supported key types:
  - RSA (2048, 4096 bits)
  - ECDSA (P-256)
  - Ed25519

The signature is created using SHA-256 hash of the module contents,
then signed with the provided private key.

Examples:
  # Sign with a private key
  kscorectl module sign my-module.zip --key private.pem

  # Sign and output to specific file
  kscorectl module sign my-module.zip --key private.pem --output my-module.sig

  # Generate a new key pair and sign
  kscorectl module sign my-module.zip --generate-key ed25519

  # Generate RSA key with specific bit size
  kscorectl module sign my-module.zip --generate-key rsa --key-bits 4096

Key Generation:
  When using --generate-key, the command generates a new key pair:
  - Private key: <module-name>.key (keep secret!)
  - Public key: <module-name>.pub (share for verification)
  - Signature: <module-name>.sig

Verification:
  Use 'kscorectl module verify' with the public key to verify:
  $ kscorectl module verify my-module.zip --require-signature --public-key my-module.pub`,
	Args: cobra.ExactArgs(1),
	RunE: signExecute,
}

func init() {
	signCmd.Flags().StringVarP(&signPrivateKey, "key", "k", "", "Private key file (PEM format)")
	signCmd.Flags().StringVarP(&signOutput, "output", "o", "", "Output signature file (default: <module>.sig)")
	signCmd.Flags().BoolVarP(&signForce, "force", "f", false, "Overwrite existing signature file")
	signCmd.Flags().StringVar(&signGenKey, "generate-key", "", "Generate new key pair (rsa, ecdsa, ed25519)")
	signCmd.Flags().IntVar(&signKeyBits, "key-bits", 2048, "Key size in bits (for RSA)")
}

func signExecute(cmd *cobra.Command, args []string) error {
	modulePath := args[0]

	// Check if module file exists
	info, err := os.Stat(modulePath)
	if err != nil {
		return fmt.Errorf("module not found: %s", modulePath)
	}
	if info.IsDir() {
		return fmt.Errorf("expected a module archive file, got directory: %s", modulePath)
	}

	// Determine output path
	outputPath := signOutput
	if outputPath == "" {
		outputPath = modulePath + ".sig"
	}

	// Check if signature already exists
	if !signForce {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("signature file already exists: %s (use --force to overwrite)", outputPath)
		}
	}

	fmt.Printf("Signing: %s\n", modulePath)
	fmt.Printf("Size: %s\n", formatSize(info.Size()))

	// Get or generate private key
	var privateKeyPEM []byte
	var publicKeyPEM []byte

	if signGenKey != "" {
		// Generate new key pair
		fmt.Printf("Generating %s key pair...\n", strings.ToUpper(signGenKey))

		var err error
		privateKeyPEM, publicKeyPEM, err = verify.GenerateKeyPair(signGenKey, signKeyBits)
		if err != nil {
			return fmt.Errorf("failed to generate key pair: %w", err)
		}

		// Determine key file paths
		baseName := strings.TrimSuffix(modulePath, filepath.Ext(modulePath))
		privateKeyPath := baseName + ".key"
		publicKeyPath := baseName + ".pub"

		// Check if key files already exist
		if !signForce {
			if _, err := os.Stat(privateKeyPath); err == nil {
				return fmt.Errorf("private key file already exists: %s (use --force to overwrite)", privateKeyPath)
			}
			if _, err := os.Stat(publicKeyPath); err == nil {
				return fmt.Errorf("public key file already exists: %s (use --force to overwrite)", publicKeyPath)
			}
		}

		// Write private key (restrictive permissions)
		if err := os.WriteFile(privateKeyPath, privateKeyPEM, 0600); err != nil {
			return fmt.Errorf("failed to write private key: %w", err)
		}
		fmt.Printf("Private key: %s (keep secret!)\n", privateKeyPath)

		// Write public key
		if err := os.WriteFile(publicKeyPath, publicKeyPEM, 0644); err != nil {
			return fmt.Errorf("failed to write public key: %w", err)
		}
		fmt.Printf("Public key:  %s (share for verification)\n", publicKeyPath)
	} else if signPrivateKey != "" {
		// Read existing private key
		privateKeyPEM, err = os.ReadFile(signPrivateKey)
		if err != nil {
			return fmt.Errorf("failed to read private key: %w", err)
		}
	} else {
		return fmt.Errorf("private key required: use --key <file> or --generate-key <type>")
	}

	// Create signer and sign the module
	signer := verify.NewSigner()

	fmt.Print("Creating signature... ")
	if err := signer.SignToFile(modulePath, outputPath, privateKeyPEM); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("signing failed: %w", err)
	}
	fmt.Println("done")

	// Get signature file info
	sigInfo, err := os.Stat(outputPath)
	if err == nil {
		fmt.Printf("Signature:   %s (%s)\n", outputPath, formatSize(sigInfo.Size()))
	}

	// Compute and display module hash for reference
	hashVerifier := verify.NewDefaultHashVerifier()
	hash, err := hashVerifier.ComputeHash(modulePath)
	if err == nil {
		fmt.Printf("\nModule SHA256: %s\n", hash)
	}

	// Print verification command
	fmt.Println("\nTo verify this signature:")
	if publicKeyPEM != nil {
		baseName := strings.TrimSuffix(modulePath, filepath.Ext(modulePath))
		fmt.Printf("  kscorectl module verify %s --require-signature --public-key %s.pub\n", modulePath, baseName)
	} else {
		fmt.Printf("  kscorectl module verify %s --require-signature --public-key <public-key.pem>\n", modulePath)
	}

	fmt.Println("\n✓ Module signed successfully!")
	return nil
}
