package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

// VerifyResult describes the verification outcome for a single module version.
type VerifyResult struct {
	Module   string
	Version  string
	Valid    bool
	SignedBy string // name of the trust root that matched
	Error    error
}

// Verifier checks module signatures against trusted keys.
type Verifier struct {
	trust *TrustStore
}

// NewVerifier creates a verifier backed by a trust store.
func NewVerifier(trust *TrustStore) *Verifier {
	return &Verifier{trust: trust}
}

// VerifyModule checks the signature of a single module version.
// It reads module.sig from the version directory and tries each active
// trust root's public key.
func (v *Verifier) VerifyModule(modulesDir, moduleName, version string) (*VerifyResult, error) {
	result := &VerifyResult{
		Module:  moduleName,
		Version: version,
	}

	versionDir := filepath.Join(modulesDir, moduleName, version)
	sigPath := filepath.Join(versionDir, "module.sig")
	zipPath := filepath.Join(versionDir, "module.zip")

	// Check if the zip exists
	if _, err := os.Stat(zipPath); err != nil {
		result.Error = fmt.Errorf("module zip not found: %w", err)
		return result, nil
	}

	// Check if signature exists
	if _, err := os.Stat(sigPath); os.IsNotExist(err) {
		if v.trust.RequireSignatures() {
			result.Error = fmt.Errorf("signature required but not found")
			return result, nil
		}
		// No signature and not required — skip
		result.Valid = true
		result.SignedBy = "(unsigned)"
		return result, nil
	}

	// Try each active trust root
	ctx := context.Background()
	for _, root := range v.trust.ActiveRoots() {
		verifier, err := signing.NewKeyVerifier(&signing.KeyVerifierConfig{
			PublicKeyPEM:  root.PublicKey,
			HashAlgorithm: signing.HashSHA256,
		})
		if err != nil {
			continue
		}

		valid, err := verifier.VerifyFile(ctx, zipPath, sigPath)
		if err != nil {
			continue
		}
		if valid {
			result.Valid = true
			result.SignedBy = root.Name
			return result, nil
		}
	}

	result.Error = fmt.Errorf("signature could not be verified against any trusted key")
	return result, nil
}

// VerifyAll checks signatures of all modules in the registry.
func (v *Verifier) VerifyAll(modulesDir string) ([]VerifyResult, error) {
	modules, err := discoverModulesInDir(modulesDir, modulesDir, "")
	if err != nil {
		return nil, fmt.Errorf("discover modules: %w", err)
	}

	var results []VerifyResult

	for _, moduleName := range modules {
		moduleDir := filepath.Join(modulesDir, moduleName)
		entries, err := os.ReadDir(moduleDir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			version := e.Name()
			zipPath := filepath.Join(moduleDir, version, "module.zip")
			if _, err := os.Stat(zipPath); err != nil {
				continue
			}

			result, err := v.VerifyModule(modulesDir, moduleName, version)
			if err != nil {
				results = append(results, VerifyResult{
					Module:  moduleName,
					Version: version,
					Error:   err,
				})
				continue
			}
			results = append(results, *result)
		}
	}

	return results, nil
}
