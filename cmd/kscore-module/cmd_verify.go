package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/verify"
)

var (
	verifyRequireSig    bool
	verifyRequireSumDB  bool
	verifyExpectedHash  string
	verifyPublicKey     string
	verifySumDBURL      string
	verifyAllowInsecure bool
)

var verifyCmd = &cobra.Command{
	Use:   "verify <path>",
	Short: "Verify module integrity",
	Long: `Verify a module's cryptographic integrity.

Verification checks:
  - SHA256 hash integrity
  - Digital signature (RSA, ECDSA, Ed25519)
  - Transparency log (SumDB) verification

Examples:
  # Verify hash only
  kscorectl module verify my-module-1.0.0.zip

  # Verify with expected hash
  kscorectl module verify my-module.zip --hash sha256:abc123...

  # Require signature verification
  kscorectl module verify my-module.zip --require-signature --public-key trusted.pem

  # Full verification including SumDB
  kscorectl module verify my-module.zip --require-signature --require-sumdb`,
	Args: cobra.ExactArgs(1),
	RunE: verifyExecute,
}

func init() {
	verifyCmd.Flags().BoolVar(&verifyRequireSig, "require-signature", false, "Require valid signature")
	verifyCmd.Flags().BoolVar(&verifyRequireSumDB, "require-sumdb", false, "Require SumDB verification")
	verifyCmd.Flags().StringVar(&verifyExpectedHash, "hash", "", "Expected hash (format: sha256:hex)")
	verifyCmd.Flags().StringVar(&verifyPublicKey, "public-key", "", "Public key file for signature verification")
	verifyCmd.Flags().StringVar(&verifySumDBURL, "sumdb-url", "", "SumDB URL for transparency verification")
	verifyCmd.Flags().BoolVar(&verifyAllowInsecure, "allow-insecure", false, "Allow verification to proceed even if some checks fail")
}

func verifyExecute(cmd *cobra.Command, args []string) error {
	modulePath := args[0]

	// Check if file exists
	info, err := os.Stat(modulePath)
	if err != nil {
		return fmt.Errorf("file not found: %s", modulePath)
	}

	fmt.Printf("Verifying: %s\n", modulePath)
	fmt.Printf("Size: %s\n\n", formatSize(info.Size()))

	// Create verifier components
	hashVerifier := verify.NewDefaultHashVerifier()
	sigVerifier := verify.NewSignatureVerifier(verify.SignatureFormatPKCS1)

	var results []verifyResult

	// 1. Hash verification
	fmt.Print("Computing SHA256 hash... ")
	hash, err := hashVerifier.ComputeHash(modulePath)
	if err != nil {
		fmt.Println("FAILED")
		results = append(results, verifyResult{"Hash computation", false, err.Error()})
	} else {
		fmt.Println("done")
		results = append(results, verifyResult{"Hash computation", true, hash})

		// Check against expected hash if provided
		if verifyExpectedHash != "" {
			expectedHash := normalizeHash(verifyExpectedHash)
			if hash == expectedHash {
				results = append(results, verifyResult{"Hash match", true, "matches expected"})
			} else {
				results = append(results, verifyResult{"Hash match", false, "hash mismatch"})
			}
		}
	}

	// 2. Signature verification
	sigFile := modulePath + ".sig"
	if _, err := os.Stat(sigFile); err == nil {
		fmt.Print("Verifying signature... ")

		if verifyPublicKey == "" {
			fmt.Println("SKIPPED (no public key)")
			results = append(results, verifyResult{"Signature", false, "no public key provided"})
		} else {
			// Read public key
			keyData, err := os.ReadFile(verifyPublicKey)
			if err != nil {
				fmt.Println("FAILED")
				results = append(results, verifyResult{"Signature", false, fmt.Sprintf("failed to read key: %v", err)})
			} else {
				valid, err := sigVerifier.VerifySignature(modulePath, sigFile, keyData)
				if err != nil {
					fmt.Println("FAILED")
					results = append(results, verifyResult{"Signature", false, err.Error()})
				} else if !valid {
					fmt.Println("INVALID")
					results = append(results, verifyResult{"Signature", false, "signature verification failed"})
				} else {
					fmt.Println("VALID")
					results = append(results, verifyResult{"Signature", true, "valid"})
				}
			}
		}
	} else if verifyRequireSig {
		fmt.Println("Signature file not found")
		results = append(results, verifyResult{"Signature", false, "signature file not found: " + sigFile})
	} else {
		results = append(results, verifyResult{"Signature", true, "not present (not required)"})
	}

	// 3. SumDB verification
	if verifySumDBURL != "" || verifyRequireSumDB {
		fmt.Print("Checking transparency log... ")

		if verifySumDBURL == "" {
			fmt.Println("SKIPPED (no SumDB URL)")
			if verifyRequireSumDB {
				results = append(results, verifyResult{"SumDB", false, "SumDB URL required but not provided"})
			}
		} else {
			// In a real implementation, we would query the SumDB
			fmt.Println("SKIPPED (not yet implemented)")
			results = append(results, verifyResult{"SumDB", false, "SumDB verification not yet implemented"})
		}
	}

	// Print results
	fmt.Println("\n=== Verification Results ===")
	allPassed := true
	for _, r := range results {
		status := "✓"
		if !r.passed {
			status = "✗"
			allPassed = false
		}
		fmt.Printf("%s %-20s %s\n", status, r.check+":", r.detail)
	}

	// Print hash for reference
	if hash != "" {
		fmt.Printf("\nSHA256: %s\n", hash)
	}

	// Final verdict
	fmt.Println()
	if allPassed {
		fmt.Println("✓ Verification passed!")
		return nil
	}

	if verifyAllowInsecure {
		fmt.Println("⚠ Verification completed with failures (--allow-insecure)")
		return nil
	}

	fmt.Println("✗ Verification failed!")
	return fmt.Errorf("verification failed")
}

type verifyResult struct {
	check  string
	passed bool
	detail string
}

func normalizeHash(hash string) string {
	// Remove algorithm prefix if present
	if strings.HasPrefix(hash, "sha256:") {
		return strings.TrimPrefix(hash, "sha256:")
	}
	if strings.HasPrefix(hash, "sha512:") {
		return strings.TrimPrefix(hash, "sha512:")
	}
	return hash
}
