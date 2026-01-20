// Package registry provides container registry authentication support for Keystone.
package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// CredentialHelper is the interface for Docker credential helpers.
type CredentialHelper interface {
	// Get retrieves credentials for a server URL.
	Get(ctx context.Context, serverURL string) (*Credential, error)
	// Store stores credentials for a server URL.
	Store(ctx context.Context, cred *Credential) error
	// Erase removes credentials for a server URL.
	Erase(ctx context.Context, serverURL string) error
	// List lists all stored credentials (server URL -> username).
	List(ctx context.Context) (map[string]string, error)
}

// ExternalCredentialHelper wraps external docker-credential-* binaries.
type ExternalCredentialHelper struct {
	helperName string
	timeout    time.Duration
}

// credentialHelperResponse is the JSON response from credential helpers.
type credentialHelperResponse struct {
	ServerURL string `json:"ServerURL"`
	Username  string `json:"Username"`
	Secret    string `json:"Secret"`
}

// NewExternalCredentialHelper creates a new external credential helper wrapper.
func NewExternalCredentialHelper(helperName string) *ExternalCredentialHelper {
	return &ExternalCredentialHelper{
		helperName: helperName,
		timeout:    30 * time.Second,
	}
}

// NewExternalCredentialHelperWithTimeout creates a helper with custom timeout.
func NewExternalCredentialHelperWithTimeout(helperName string, timeout time.Duration) *ExternalCredentialHelper {
	return &ExternalCredentialHelper{
		helperName: helperName,
		timeout:    timeout,
	}
}

// binaryName returns the full binary name for the helper.
func (h *ExternalCredentialHelper) binaryName() string {
	return "docker-credential-" + h.helperName
}

// Get retrieves credentials from the helper.
func (h *ExternalCredentialHelper) Get(ctx context.Context, serverURL string) (*Credential, error) {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.binaryName(), "get")
	cmd.Stdin = strings.NewReader(serverURL)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Check if it's a "not found" error (exit code 1 with specific message)
		if strings.Contains(stderr.String(), "credentials not found") ||
			strings.Contains(stdout.String(), "credentials not found") {
			return nil, fmt.Errorf("credentials not found for %s", serverURL)
		}
		return nil, fmt.Errorf("credential helper %s failed: %v - %s", h.binaryName(), err, stderr.String())
	}

	var resp credentialHelperResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse credential helper response: %w", err)
	}

	return &Credential{
		Type:     DetectRegistryType(serverURL),
		Registry: serverURL,
		Username: resp.Username,
		Password: resp.Secret,
	}, nil
}

// Store stores credentials using the helper.
func (h *ExternalCredentialHelper) Store(ctx context.Context, cred *Credential) error {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	input := credentialHelperResponse{
		ServerURL: cred.Registry,
		Username:  cred.Username,
		Secret:    cred.Password,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, h.binaryName(), "store")
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("credential helper %s store failed: %v - %s", h.binaryName(), err, stderr.String())
	}

	return nil
}

// Erase removes credentials using the helper.
func (h *ExternalCredentialHelper) Erase(ctx context.Context, serverURL string) error {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.binaryName(), "erase")
	cmd.Stdin = strings.NewReader(serverURL)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("credential helper %s erase failed: %v - %s", h.binaryName(), err, stderr.String())
	}

	return nil
}

// List lists all stored credentials.
func (h *ExternalCredentialHelper) List(ctx context.Context) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.binaryName(), "list")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("credential helper %s list failed: %v - %s", h.binaryName(), err, stderr.String())
	}

	var result map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse credential helper list response: %w", err)
	}

	return result, nil
}

// IsAvailable checks if the credential helper binary exists and is executable.
func (h *ExternalCredentialHelper) IsAvailable() bool {
	_, err := exec.LookPath(h.binaryName())
	return err == nil
}

// CredentialHelperRegistry manages credential helpers for different registries.
type CredentialHelperRegistry struct {
	helpers      map[string]CredentialHelper // registry pattern -> helper
	defaultStore string                      // default credsStore
	mu           sync.RWMutex
}

// NewCredentialHelperRegistry creates a new credential helper registry.
func NewCredentialHelperRegistry() *CredentialHelperRegistry {
	return &CredentialHelperRegistry{
		helpers: make(map[string]CredentialHelper),
	}
}

// LoadFromDockerConfig loads credential helpers from a Docker config.
func (r *CredentialHelperRegistry) LoadFromDockerConfig(config *DockerConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Set default credential store
	if config.CredsStore != "" {
		r.defaultStore = config.CredsStore
	}

	// Register per-registry credential helpers
	for registry, helperName := range config.CredHelpers {
		r.helpers[registry] = NewExternalCredentialHelper(helperName)
	}
}

// RegisterHelper registers a credential helper for a registry.
func (r *CredentialHelperRegistry) RegisterHelper(registry string, helper CredentialHelper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.helpers[registry] = helper
}

// SetDefaultStore sets the default credential store.
func (r *CredentialHelperRegistry) SetDefaultStore(storeName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultStore = storeName
}

// GetHelper returns the credential helper for a registry.
func (r *CredentialHelperRegistry) GetHelper(registry string) CredentialHelper {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check for exact match first
	if helper, ok := r.helpers[registry]; ok {
		return helper
	}

	// Check for pattern matches (e.g., "*.gcr.io")
	for pattern, helper := range r.helpers {
		if matchRegistryPattern(pattern, registry) {
			return helper
		}
	}

	// Fall back to default store
	if r.defaultStore != "" {
		return NewExternalCredentialHelper(r.defaultStore)
	}

	return nil
}

// GetCredential retrieves credentials for a registry using the appropriate helper.
func (r *CredentialHelperRegistry) GetCredential(ctx context.Context, registry string) (*Credential, error) {
	helper := r.GetHelper(registry)
	if helper == nil {
		return nil, fmt.Errorf("no credential helper found for registry %s", registry)
	}
	return helper.Get(ctx, registry)
}

// HasHelper checks if a helper is configured for the registry.
func (r *CredentialHelperRegistry) HasHelper(registry string) bool {
	return r.GetHelper(registry) != nil
}

// matchRegistryPattern checks if a registry matches a pattern.
func matchRegistryPattern(pattern, registry string) bool {
	// Simple wildcard matching
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // Remove "*"
		return strings.HasSuffix(registry, suffix)
	}

	// Check if registry starts with the pattern
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(registry, prefix)
	}

	return pattern == registry
}

// CommonCredentialHelpers contains names of well-known credential helpers.
var CommonCredentialHelpers = map[string]string{
	"ecr":        "ecr-login",        // AWS ECR
	"gcr":        "gcr",              // Google Container Registry (older)
	"gcloud":     "gcloud",           // Google Cloud (newer, recommended)
	"acr":        "acr-env",          // Azure Container Registry
	"osxkeychain": "osxkeychain",     // macOS Keychain
	"wincred":    "wincred",          // Windows Credential Manager
	"secretservice": "secretservice", // Linux Secret Service (GNOME Keyring)
	"pass":       "pass",             // pass (Unix password manager)
}

// GetCommonHelper returns a common credential helper by type.
func GetCommonHelper(helperType string) CredentialHelper {
	if name, ok := CommonCredentialHelpers[helperType]; ok {
		return NewExternalCredentialHelper(name)
	}
	return nil
}
