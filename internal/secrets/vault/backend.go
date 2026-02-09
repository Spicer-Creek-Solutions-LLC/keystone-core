// Package vault provides a HashiCorp Vault backend for the secrets broker.
package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// Backend implements the secrets.SecretBackend interface for HashiCorp Vault.
type Backend struct {
	client *Client
	name   string

	// Engine configurations by mount path
	engines map[string]*EngineConfig
}

// EngineConfig configures a secret engine mount.
type EngineConfig struct {
	// MountPath is the mount path of the engine.
	MountPath string `json:"mount_path"`

	// Type is the engine type (kv, kv-v2, database, aws, pki, transit, etc.).
	Type string `json:"type"`

	// Version is the KV engine version (1 or 2). Only applies to kv type.
	Version int `json:"version,omitempty"`

	// DefaultTTL is the default lease TTL for dynamic secrets.
	DefaultTTL time.Duration `json:"default_ttl,omitempty"`

	// MaxTTL is the maximum lease TTL for dynamic secrets.
	MaxTTL time.Duration `json:"max_ttl,omitempty"`
}

// BackendConfig configures the Vault backend.
type BackendConfig struct {
	// Name is the unique name for this backend instance.
	Name string `json:"name"`

	// Client is the Vault client configuration.
	Client *ClientConfig `json:"client"`

	// Engines configures the secret engines to use.
	Engines []*EngineConfig `json:"engines,omitempty"`

	// DefaultKVVersion is the default KV version (1 or 2).
	DefaultKVVersion int `json:"default_kv_version,omitempty"`
}

// NewBackend creates a new Vault backend.
func NewBackend(config *BackendConfig) (*Backend, error) {
	if config == nil {
		return nil, fmt.Errorf("backend config is required")
	}

	if config.Name == "" {
		config.Name = "vault"
	}

	client, err := NewClient(config.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Configure authenticator if auth config is provided
	if config.Client != nil && config.Client.Auth != nil {
		auth, err := NewAuthenticator(config.Client.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to create authenticator: %w", err)
		}
		client.SetAuthenticator(auth)
	}

	engines := make(map[string]*EngineConfig)
	for _, eng := range config.Engines {
		engines[eng.MountPath] = eng
	}

	// Set default KV version
	defaultVersion := config.DefaultKVVersion
	if defaultVersion == 0 {
		defaultVersion = 2 // Default to KV v2
	}

	// Add default secret/ engine if no engines configured
	if len(engines) == 0 {
		engines["secret"] = &EngineConfig{
			MountPath: "secret",
			Type:      "kv",
			Version:   defaultVersion,
		}
	}

	return &Backend{
		client:  client,
		name:    config.Name,
		engines: engines,
	}, nil
}

// Type returns the backend type.
func (b *Backend) Type() secrets.BackendType {
	return secrets.BackendTypeVault
}

// Name returns the backend instance name.
func (b *Backend) Name() string {
	return b.name
}

// Healthy returns true if the backend is healthy.
func (b *Backend) Healthy(ctx context.Context) bool {
	return b.client.Healthy(ctx)
}

// Client returns the underlying Vault client.
func (b *Backend) Client() *Client {
	return b.client
}

// Authenticate authenticates the backend.
func (b *Backend) Authenticate(ctx context.Context) error {
	return b.client.Authenticate(ctx)
}

// StartTokenRenewal starts automatic token renewal.
func (b *Backend) StartTokenRenewal(ctx context.Context) {
	b.client.StartTokenRenewal(ctx)
}

// StopTokenRenewal stops automatic token renewal.
func (b *Backend) StopTokenRenewal() {
	b.client.StopTokenRenewal()
}

// Read reads a secret from Vault.
func (b *Backend) Read(ctx context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	path := req.Path
	mount, subPath := b.splitPath(path)
	engine := b.getEngine(mount)

	// Determine engine type and version
	engineType := "kv"
	kvVersion := 2
	if engine != nil {
		engineType = engine.Type
		if engine.Version > 0 {
			kvVersion = engine.Version
		}
	}

	var resp map[string]interface{}
	var err error

	switch engineType {
	case "kv", "kv-v2":
		if kvVersion == 2 {
			resp, err = b.readKVv2(ctx, mount, subPath, req.Version)
		} else {
			resp, err = b.readKVv1(ctx, mount, subPath)
		}
	default:
		// Generic read for other engine types
		resp, err = b.client.Read(ctx, path)
	}

	if err != nil {
		return nil, err
	}

	return b.parseSecret(path, resp, engine)
}

// readKVv1 reads a secret from a KV v1 engine.
func (b *Backend) readKVv1(ctx context.Context, mount, path string) (map[string]interface{}, error) {
	fullPath := mount + "/" + path
	return b.client.Read(ctx, fullPath)
}

// readKVv2 reads a secret from a KV v2 engine.
func (b *Backend) readKVv2(ctx context.Context, mount, path string, version int) (map[string]interface{}, error) {
	// KV v2 uses /data/ prefix for reading secrets
	fullPath := mount + "/data/" + path

	if version > 0 {
		fullPath = fmt.Sprintf("%s?version=%d", fullPath, version)
	}

	return b.client.Read(ctx, fullPath)
}

// ReadDynamic reads or generates a dynamic secret.
func (b *Backend) ReadDynamic(ctx context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	path := req.Path
	mount, subPath := b.splitPath(path)
	engine := b.getEngine(mount)

	// Build request data for dynamic secrets
	data := make(map[string]interface{})
	if req.TTL > 0 {
		data["ttl"] = fmt.Sprintf("%ds", int(req.TTL.Seconds()))
	}

	var resp map[string]interface{}
	var err error

	// Determine the API call based on engine type
	engineType := ""
	if engine != nil {
		engineType = engine.Type
	}

	switch engineType {
	case "database":
		// Database credentials use /creds/ prefix
		fullPath := mount + "/creds/" + subPath
		resp, err = b.client.Read(ctx, fullPath)
	case "aws":
		// AWS credentials use /creds/ or /sts/ prefix
		fullPath := mount + "/creds/" + subPath
		resp, err = b.client.Read(ctx, fullPath)
	case "pki":
		// PKI certificates use /issue/ prefix
		fullPath := mount + "/issue/" + subPath
		resp, err = b.client.Write(ctx, fullPath, data)
	case "ssh":
		// SSH certificates use /sign/ prefix
		fullPath := mount + "/sign/" + subPath
		resp, err = b.client.Write(ctx, fullPath, data)
	default:
		// Generic read for unknown dynamic engines
		resp, err = b.client.Read(ctx, path)
	}

	if err != nil {
		return nil, err
	}

	secret, err := b.parseSecret(path, resp, engine)
	if err != nil {
		return nil, err
	}

	// Mark as dynamic secret
	secret.Type = secrets.SecretTypeDynamic

	return secret, nil
}

// List lists secrets under a path prefix.
func (b *Backend) List(ctx context.Context, prefix string) ([]string, error) {
	mount, subPath := b.splitPath(prefix)
	engine := b.getEngine(mount)

	// Determine KV version
	kvVersion := 2
	if engine != nil && engine.Version > 0 {
		kvVersion = engine.Version
	}

	var fullPath string
	if kvVersion == 2 && (engine == nil || engine.Type == "kv" || engine.Type == "kv-v2") {
		// KV v2 uses /metadata/ prefix for listing
		fullPath = mount + "/metadata/" + subPath
	} else {
		fullPath = mount + "/" + subPath
	}

	return b.client.List(ctx, fullPath)
}

// RenewLease renews a lease.
func (b *Backend) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*secrets.Lease, error) {
	data := map[string]interface{}{
		"lease_id":  leaseID,
		"increment": int(increment.Seconds()),
	}

	resp, err := b.client.Write(ctx, "sys/leases/renew", data)
	if err != nil {
		return nil, fmt.Errorf("failed to renew lease: %w", err)
	}

	return b.parseLease(leaseID, resp)
}

// RevokeLease revokes a lease.
func (b *Backend) RevokeLease(ctx context.Context, leaseID string) error {
	data := map[string]interface{}{
		"lease_id": leaseID,
	}

	_, err := b.client.Write(ctx, "sys/leases/revoke", data)
	if err != nil {
		return fmt.Errorf("failed to revoke lease: %w", err)
	}

	return nil
}

// Close closes the backend connection.
func (b *Backend) Close() error {
	return b.client.Close()
}

// Write writes a secret to Vault.
func (b *Backend) Write(ctx context.Context, path string, data map[string]interface{}) error {
	mount, subPath := b.splitPath(path)
	engine := b.getEngine(mount)

	// Determine KV version
	kvVersion := 2
	if engine != nil && engine.Version > 0 {
		kvVersion = engine.Version
	}

	if kvVersion == 2 && (engine == nil || engine.Type == "kv" || engine.Type == "kv-v2") {
		return b.writeKVv2(ctx, mount, subPath, data)
	}

	return b.writeKVv1(ctx, mount, subPath, data)
}

// writeKVv1 writes a secret to a KV v1 engine.
func (b *Backend) writeKVv1(ctx context.Context, mount, path string, data map[string]interface{}) error {
	fullPath := mount + "/" + path
	_, err := b.client.Write(ctx, fullPath, data)
	return err
}

// writeKVv2 writes a secret to a KV v2 engine.
func (b *Backend) writeKVv2(ctx context.Context, mount, path string, data map[string]interface{}) error {
	fullPath := mount + "/data/" + path

	// KV v2 wraps data in a "data" field
	wrapped := map[string]interface{}{
		"data": data,
	}

	_, err := b.client.Write(ctx, fullPath, wrapped)
	return err
}

// Delete deletes a secret from Vault.
func (b *Backend) Delete(ctx context.Context, path string) error {
	mount, subPath := b.splitPath(path)
	engine := b.getEngine(mount)

	// Determine KV version
	kvVersion := 2
	if engine != nil && engine.Version > 0 {
		kvVersion = engine.Version
	}

	var fullPath string
	if kvVersion == 2 && (engine == nil || engine.Type == "kv" || engine.Type == "kv-v2") {
		// KV v2 uses /data/ prefix for deletion (soft delete)
		fullPath = mount + "/data/" + subPath
	} else {
		fullPath = mount + "/" + subPath
	}

	_, err := b.client.Delete(ctx, fullPath)
	return err
}

// DestroyVersions permanently destroys specific versions of a KV v2 secret.
func (b *Backend) DestroyVersions(ctx context.Context, path string, versions []int) error {
	mount, subPath := b.splitPath(path)

	fullPath := mount + "/destroy/" + subPath
	data := map[string]interface{}{
		"versions": versions,
	}

	_, err := b.client.Write(ctx, fullPath, data)
	return err
}

// UndeleteVersions undeletes specific versions of a KV v2 secret.
func (b *Backend) UndeleteVersions(ctx context.Context, path string, versions []int) error {
	mount, subPath := b.splitPath(path)

	fullPath := mount + "/undelete/" + subPath
	data := map[string]interface{}{
		"versions": versions,
	}

	_, err := b.client.Write(ctx, fullPath, data)
	return err
}

// GetMetadata retrieves metadata for a KV v2 secret.
func (b *Backend) GetMetadata(ctx context.Context, path string) (*SecretMetadata, error) {
	mount, subPath := b.splitPath(path)

	fullPath := mount + "/metadata/" + subPath
	resp, err := b.client.Read(ctx, fullPath)
	if err != nil {
		return nil, err
	}

	return b.parseMetadata(resp)
}

// SecretMetadata contains metadata for a KV v2 secret.
type SecretMetadata struct {
	// CreatedTime is when the secret was created.
	CreatedTime time.Time `json:"created_time"`

	// CurrentVersion is the current version number.
	CurrentVersion int `json:"current_version"`

	// MaxVersions is the maximum number of versions to keep.
	MaxVersions int `json:"max_versions"`

	// OldestVersion is the oldest version number.
	OldestVersion int `json:"oldest_version"`

	// UpdatedTime is when the secret was last updated.
	UpdatedTime time.Time `json:"updated_time"`

	// Versions contains version-specific metadata.
	Versions map[int]*VersionMetadata `json:"versions"`

	// CustomMetadata contains user-defined metadata.
	CustomMetadata map[string]string `json:"custom_metadata"`

	// CASRequired indicates if check-and-set is required.
	CASRequired bool `json:"cas_required"`

	// DeleteVersionAfter is the duration after which versions are deleted.
	DeleteVersionAfter time.Duration `json:"delete_version_after"`
}

// VersionMetadata contains metadata for a specific version.
type VersionMetadata struct {
	// CreatedTime is when this version was created.
	CreatedTime time.Time `json:"created_time"`

	// DeletionTime is when this version was deleted (if soft-deleted).
	DeletionTime time.Time `json:"deletion_time,omitempty"`

	// Destroyed indicates if this version was permanently destroyed.
	Destroyed bool `json:"destroyed"`

	// Version is the version number.
	Version int `json:"version"`
}

// parseMetadata parses Vault metadata response.
func (b *Backend) parseMetadata(resp map[string]interface{}) (*SecretMetadata, error) {
	if resp == nil {
		return nil, secrets.ErrSecretNotFound
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid metadata response")
	}

	meta := &SecretMetadata{
		Versions:       make(map[int]*VersionMetadata),
		CustomMetadata: make(map[string]string),
	}

	if currentVersion, ok := data["current_version"].(float64); ok {
		meta.CurrentVersion = int(currentVersion)
	}

	if maxVersions, ok := data["max_versions"].(float64); ok {
		meta.MaxVersions = int(maxVersions)
	}

	if oldestVersion, ok := data["oldest_version"].(float64); ok {
		meta.OldestVersion = int(oldestVersion)
	}

	if casRequired, ok := data["cas_required"].(bool); ok {
		meta.CASRequired = casRequired
	}

	if createdTime, ok := data["created_time"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, createdTime); err == nil {
			meta.CreatedTime = t
		}
	}

	if updatedTime, ok := data["updated_time"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, updatedTime); err == nil {
			meta.UpdatedTime = t
		}
	}

	if deleteAfter, ok := data["delete_version_after"].(string); ok {
		if d, err := time.ParseDuration(deleteAfter); err == nil {
			meta.DeleteVersionAfter = d
		}
	}

	if customMeta, ok := data["custom_metadata"].(map[string]interface{}); ok {
		for k, v := range customMeta {
			if s, ok := v.(string); ok {
				meta.CustomMetadata[k] = s
			}
		}
	}

	if versions, ok := data["versions"].(map[string]interface{}); ok {
		for vStr, vData := range versions {
			vNum, err := strconv.Atoi(vStr)
			if err != nil {
				continue
			}

			vMeta := &VersionMetadata{Version: vNum}
			if vm, ok := vData.(map[string]interface{}); ok {
				if destroyed, ok := vm["destroyed"].(bool); ok {
					vMeta.Destroyed = destroyed
				}
				if createdTime, ok := vm["created_time"].(string); ok {
					if t, err := time.Parse(time.RFC3339Nano, createdTime); err == nil {
						vMeta.CreatedTime = t
					}
				}
				if deletionTime, ok := vm["deletion_time"].(string); ok {
					if t, err := time.Parse(time.RFC3339Nano, deletionTime); err == nil && !t.IsZero() {
						vMeta.DeletionTime = t
					}
				}
			}
			meta.Versions[vNum] = vMeta
		}
	}

	return meta, nil
}

// splitPath splits a path into mount and sub-path.
func (b *Backend) splitPath(path string) (mount, subPath string) {
	// Try to find longest matching mount
	longestMatch := ""
	exactMatch := false
	for m := range b.engines {
		if strings.HasPrefix(path, m+"/") && len(m) > len(longestMatch) {
			longestMatch = m
		}
		if path == m {
			longestMatch = m
			exactMatch = true
		}
	}

	if longestMatch != "" {
		mount = longestMatch
		if exactMatch {
			subPath = ""
		} else {
			subPath = strings.TrimPrefix(path, mount+"/")
		}
	} else {
		// Default: split on first /
		parts := strings.SplitN(path, "/", 2)
		mount = parts[0]
		if len(parts) > 1 {
			subPath = parts[1]
		}
	}

	return mount, subPath
}

// getEngine returns the engine config for a mount path.
func (b *Backend) getEngine(mount string) *EngineConfig {
	return b.engines[mount]
}

// parseSecret parses a Vault response into a Secret.
func (b *Backend) parseSecret(path string, resp map[string]interface{}, engine *EngineConfig) (*secrets.Secret, error) {
	if resp == nil {
		return nil, secrets.ErrSecretNotFound
	}

	secret := &secrets.Secret{
		Path:     path,
		Backend:  secrets.BackendTypeVault,
		Type:     secrets.SecretTypeStatic,
		Data:     make(map[string]interface{}),
		Metadata: make(map[string]string),
	}

	// KV v2 wraps data in a "data" field
	kvVersion := 2
	if engine != nil && engine.Version > 0 {
		kvVersion = engine.Version
	}

	if kvVersion == 2 {
		// KV v2 response structure
		if data, ok := resp["data"].(map[string]interface{}); ok {
			if innerData, ok := data["data"].(map[string]interface{}); ok {
				secret.Data = innerData
			}

			// Parse metadata
			if meta, ok := data["metadata"].(map[string]interface{}); ok {
				if version, ok := meta["version"].(float64); ok {
					secret.Version = int(version)
				}
				if createdTime, ok := meta["created_time"].(string); ok {
					if t, err := time.Parse(time.RFC3339Nano, createdTime); err == nil {
						secret.CreatedAt = t
					}
				}
			}
		}
	} else {
		// KV v1 response structure - data is at top level
		if data, ok := resp["data"].(map[string]interface{}); ok {
			secret.Data = data
		}
	}

	// Parse lease information
	if leaseID, ok := resp["lease_id"].(string); ok && leaseID != "" {
		lease := &secrets.Lease{
			ID:         leaseID,
			SecretPath: path,
			Backend:    secrets.BackendTypeVault,
			State:      secrets.LeaseStateActive,
			IssuedAt:   time.Now(),
			Renewable:  true,
			Revocable:  true,
		}

		if leaseDuration, ok := resp["lease_duration"].(float64); ok {
			lease.TTL = time.Duration(leaseDuration) * time.Second
			lease.ExpiresAt = lease.IssuedAt.Add(lease.TTL)
		}

		if renewable, ok := resp["renewable"].(bool); ok {
			lease.Renewable = renewable
		}

		secret.Lease = lease
		secret.Renewable = lease.Renewable
		secret.Type = secrets.SecretTypeDynamic
	}

	// Handle base64-encoded values
	for k, v := range secret.Data {
		if s, ok := v.(string); ok && isBase64(s) {
			if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
				// Keep as string if it's valid UTF-8, otherwise keep as bytes
				secret.Data[k] = string(decoded)
			}
		}
	}

	return secret, nil
}

// parseLease parses a Vault lease response.
func (b *Backend) parseLease(leaseID string, resp map[string]interface{}) (*secrets.Lease, error) {
	lease := &secrets.Lease{
		ID:        leaseID,
		Backend:   secrets.BackendTypeVault,
		State:     secrets.LeaseStateActive,
		Renewable: true,
		Revocable: true,
	}

	if leaseDuration, ok := resp["lease_duration"].(float64); ok {
		lease.TTL = time.Duration(leaseDuration) * time.Second
		lease.ExpiresAt = time.Now().Add(lease.TTL)
	}

	if renewable, ok := resp["renewable"].(bool); ok {
		lease.Renewable = renewable
	}

	return lease, nil
}

// isBase64 checks if a string looks like base64 encoded data.
func isBase64(s string) bool {
	if len(s) < 4 || len(s)%4 != 0 {
		return false
	}
	// Only check strings that look like they might be binary data encoded as base64
	for _, r := range s {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '+' && r != '/' && r != '=' {
			return false
		}
	}
	return true
}

// RegisterEngine registers a new engine configuration.
func (b *Backend) RegisterEngine(config *EngineConfig) {
	if config != nil && config.MountPath != "" {
		b.engines[config.MountPath] = config
	}
}
