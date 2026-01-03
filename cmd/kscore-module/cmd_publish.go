package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/registry"
	"github.com/shawnbutts/keystone-core/pkg/module/verify"
)

var (
	publishRegistry     string
	publishToken        string
	publishUsername     string
	publishPassword     string
	publishSignature    string
	publishForce        bool
	publishReleaseNotes string
	publishTags         []string
	publishDryRun       bool
)

var publishCmd = &cobra.Command{
	Use:   "publish <module-path>",
	Short: "Publish a module to a registry",
	Long: `Publish a module to a registry.

The module must be a built ZIP file with a valid manifest.

Examples:
  # Publish to the default registry
  kscore-module publish my-module-1.0.0.zip

  # Publish with a specific registry URL
  kscore-module publish my-module-1.0.0.zip --registry https://registry.example.com

  # Publish with authentication
  kscore-module publish my-module-1.0.0.zip --token $REGISTRY_TOKEN

  # Publish with signature
  kscore-module publish my-module-1.0.0.zip --signature my-module-1.0.0.zip.sig

  # Force overwrite existing version
  kscore-module publish my-module-1.0.0.zip --force

  # Dry run (validate without publishing)
  kscore-module publish my-module-1.0.0.zip --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: publishExecute,
}

func init() {
	publishCmd.Flags().StringVar(&publishRegistry, "registry", "", "Registry URL (defaults to KSCORE_REGISTRY or https://registry.keystonecore.io)")
	publishCmd.Flags().StringVar(&publishToken, "token", "", "Authentication token (can also use KSCORE_REGISTRY_TOKEN)")
	publishCmd.Flags().StringVar(&publishUsername, "username", "", "Username for basic auth (can also use KSCORE_REGISTRY_USERNAME)")
	publishCmd.Flags().StringVar(&publishPassword, "password", "", "Password for basic auth (can also use KSCORE_REGISTRY_PASSWORD)")
	publishCmd.Flags().StringVar(&publishSignature, "signature", "", "Path to detached signature file")
	publishCmd.Flags().BoolVar(&publishForce, "force", false, "Overwrite existing version")
	publishCmd.Flags().StringVar(&publishReleaseNotes, "release-notes", "", "Release notes for this version")
	publishCmd.Flags().StringSliceVar(&publishTags, "tag", nil, "Tags for this version (can be specified multiple times)")
	publishCmd.Flags().BoolVar(&publishDryRun, "dry-run", false, "Validate without publishing")
}

func publishExecute(cmd *cobra.Command, args []string) error {
	modulePath := args[0]

	// Check if module file exists
	info, err := os.Stat(modulePath)
	if err != nil {
		return fmt.Errorf("module not found: %s", modulePath)
	}
	if info.IsDir() {
		return fmt.Errorf("module path must be a file, not a directory: %s", modulePath)
	}

	fmt.Printf("Publishing: %s\n", modulePath)
	fmt.Printf("Size: %s\n\n", formatSize(info.Size()))

	// Extract and parse manifest from the module ZIP
	m, err := extractManifestFromZip(modulePath)
	if err != nil {
		return fmt.Errorf("failed to read manifest from module: %w", err)
	}

	fmt.Printf("Module: %s\n", m.Name)
	fmt.Printf("Version: %s\n", m.Version)
	fmt.Printf("Type: %s\n\n", m.Type)

	// Compute hash
	hasher := verify.NewDefaultHashVerifier()
	hash, err := hasher.ComputeHash(modulePath)
	if err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}
	fmt.Printf("SHA256: %s\n\n", hash)

	// Check for signature file
	sigPath := publishSignature
	if sigPath == "" {
		// Try default signature path
		defaultSigPath := modulePath + ".sig"
		if _, err := os.Stat(defaultSigPath); err == nil {
			sigPath = defaultSigPath
			fmt.Printf("Found signature: %s\n\n", sigPath)
		}
	} else {
		if _, err := os.Stat(sigPath); err != nil {
			return fmt.Errorf("signature file not found: %s", sigPath)
		}
		fmt.Printf("Signature: %s\n\n", sigPath)
	}

	// Get registry URL
	registryURL := publishRegistry
	if registryURL == "" {
		registryURL = os.Getenv("KSCORE_REGISTRY")
	}
	if registryURL == "" {
		registryURL = "https://registry.keystonecore.io"
	}
	registryURL = strings.TrimSuffix(registryURL, "/")

	fmt.Printf("Registry: %s\n\n", registryURL)

	// Dry run - just validate
	if publishDryRun {
		fmt.Println("=== Dry Run ===")
		fmt.Println("✓ Module file exists")
		fmt.Println("✓ Manifest is valid")
		fmt.Printf("✓ Hash computed: %s\n", hash[:16]+"...")
		if sigPath != "" {
			fmt.Println("✓ Signature file found")
		} else {
			fmt.Println("⚠ No signature file (module will be unsigned)")
		}
		fmt.Println("\nDry run complete. Use --dry-run=false to publish.")
		return nil
	}

	// Build authentication config
	auth := buildAuthConfig()
	if auth == nil {
		fmt.Println("⚠ No authentication configured. Publishing may fail if registry requires authentication.")
	}

	// Create registry client
	config := registry.DefaultRegistryConfig(registryURL)
	config.Auth = auth
	client := registry.NewHTTPClient(config)

	// Build publish request
	req := &registry.PublishRequest{
		ModulePath:    modulePath,
		Manifest:      m,
		SignaturePath: sigPath,
		Hash:          hash,
		Force:         publishForce,
		Tags:          publishTags,
		ReleaseNotes:  publishReleaseNotes,
	}

	// Publish
	fmt.Print("Publishing module... ")
	result, err := client.PublishModule(req)
	if err != nil {
		fmt.Println("FAILED")
		if registry.IsVersionExistsError(err) {
			return fmt.Errorf("version %s already exists (use --force to overwrite)", m.Version)
		}
		if registry.IsAuthError(err) {
			return fmt.Errorf("authentication failed: %v", err)
		}
		return fmt.Errorf("publish failed: %w", err)
	}
	fmt.Println("done")

	// Print result
	fmt.Println("\n=== Published ===")
	fmt.Printf("Module: %s@%s\n", result.ModuleName, result.Version)
	fmt.Printf("Hash: %s\n", result.Hash)
	fmt.Printf("Size: %s\n", formatSize(result.Size))
	fmt.Printf("URL: %s\n", result.URL)
	if result.SignatureVerified {
		fmt.Println("Signature: ✓ verified")
	}
	if result.SumDBRecorded {
		fmt.Println("SumDB: ✓ recorded")
	}
	fmt.Printf("Published: %s\n", result.PublishedAt.Format("2006-01-02 15:04:05"))

	fmt.Println("\n✓ Module published successfully!")
	return nil
}

// extractManifestFromZip extracts and parses the manifest from a module ZIP
func extractManifestFromZip(zipPath string) (*manifest.Manifest, error) {
	// Open ZIP file
	f, err := os.Open(zipPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// Use archive/zip to read
	zipReader, err := newZipReader(f, info.Size())
	if err != nil {
		return nil, err
	}

	// Look for module.yaml
	for _, file := range zipReader.File {
		if filepath.Base(file.Name) == "module.yaml" {
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open module.yaml: %w", err)
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("failed to read module.yaml: %w", err)
			}

			return manifest.Parse(data)
		}
	}

	return nil, fmt.Errorf("module.yaml not found in ZIP")
}

// buildAuthConfig builds authentication configuration from flags and environment
func buildAuthConfig() *registry.AuthConfig {
	// Token auth (bearer)
	token := publishToken
	if token == "" {
		token = os.Getenv("KSCORE_REGISTRY_TOKEN")
	}
	if token != "" {
		return &registry.AuthConfig{
			Type:  registry.AuthTypeBearer,
			Token: token,
		}
	}

	// Basic auth
	username := publishUsername
	if username == "" {
		username = os.Getenv("KSCORE_REGISTRY_USERNAME")
	}
	password := publishPassword
	if password == "" {
		password = os.Getenv("KSCORE_REGISTRY_PASSWORD")
	}
	if username != "" && password != "" {
		return &registry.AuthConfig{
			Type:     registry.AuthTypeBasic,
			Username: username,
			Password: password,
		}
	}

	return nil
}

// newZipReader creates a new zip.Reader from a file
func newZipReader(f *os.File, size int64) (*zip.Reader, error) {
	return zip.NewReader(f, size)
}
