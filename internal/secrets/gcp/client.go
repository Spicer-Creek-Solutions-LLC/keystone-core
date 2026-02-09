// Package gcp provides a GCP Secret Manager backend for the secrets broker.
package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// AuthMethod represents the authentication method for GCP.
type AuthMethod string

const (
	// AuthMethodDefault uses application default credentials.
	AuthMethodDefault AuthMethod = "default"

	// AuthMethodServiceAccount uses a service account key file.
	AuthMethodServiceAccount AuthMethod = "service_account"

	// AuthMethodWorkloadIdentity uses workload identity federation.
	AuthMethodWorkloadIdentity AuthMethod = "workload_identity"

	// AuthMethodImpersonation uses service account impersonation.
	AuthMethodImpersonation AuthMethod = "impersonation"
)

// ClientConfig configures the GCP Secret Manager client.
type ClientConfig struct {
	// ProjectID is the GCP project ID.
	ProjectID string `json:"project_id"`

	// AuthMethod specifies the authentication method to use.
	AuthMethod AuthMethod `json:"auth_method,omitempty"`

	// ServiceAccountKeyFile is the path to the service account key file.
	ServiceAccountKeyFile string `json:"service_account_key_file,omitempty"`

	// ServiceAccountKeyJSON is the raw service account key JSON.
	ServiceAccountKeyJSON []byte `json:"-"`

	// ImpersonateServiceAccount is the service account to impersonate.
	ImpersonateServiceAccount string `json:"impersonate_service_account,omitempty"`

	// ImpersonateDelegates are intermediate service accounts for impersonation chain.
	ImpersonateDelegates []string `json:"impersonate_delegates,omitempty"`

	// Endpoint overrides the default API endpoint.
	Endpoint string `json:"endpoint,omitempty"`

	// Timeout is the timeout for API requests.
	Timeout time.Duration `json:"timeout,omitempty"`

	// UserAgent is a custom user agent string.
	UserAgent string `json:"user_agent,omitempty"`

	// Scopes are the OAuth scopes to request.
	Scopes []string `json:"scopes,omitempty"`

	// QuotaProject is the project used for quota and billing.
	QuotaProject string `json:"quota_project,omitempty"`
}

// DefaultClientConfig returns a client configuration with sensible defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		AuthMethod: AuthMethodDefault,
		Timeout:    30 * time.Second,
		Scopes:     []string{"https://www.googleapis.com/auth/cloud-platform"},
	}
}

// Client is a GCP Secret Manager client.
type Client struct {
	config    *ClientConfig
	client    *secretmanager.Client
	projectID string
}

// NewClient creates a new GCP Secret Manager client.
func NewClient(ctx context.Context, cfg *ClientConfig) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	if cfg.ProjectID == "" {
		return nil, errors.New("project_id is required")
	}

	// Build client options
	opts, err := buildClientOptions(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build client options: %w", err)
	}

	// Create the Secret Manager client
	client, err := secretmanager.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Secret Manager client: %w", err)
	}

	return &Client{
		config:    cfg,
		client:    client,
		projectID: cfg.ProjectID,
	}, nil
}

// buildClientOptions builds the client options based on configuration.
func buildClientOptions(cfg *ClientConfig) ([]option.ClientOption, error) {
	var opts []option.ClientOption

	// Set endpoint if specified
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithEndpoint(cfg.Endpoint))
	}

	// Set user agent if specified
	if cfg.UserAgent != "" {
		opts = append(opts, option.WithUserAgent(cfg.UserAgent))
	}

	// Set quota project if specified
	if cfg.QuotaProject != "" {
		opts = append(opts, option.WithQuotaProject(cfg.QuotaProject))
	}

	// Set scopes if specified
	if len(cfg.Scopes) > 0 {
		opts = append(opts, option.WithScopes(cfg.Scopes...))
	}

	// Configure authentication based on method
	switch cfg.AuthMethod {
	case AuthMethodDefault, "":
		// Use application default credentials (no additional options needed)

	case AuthMethodServiceAccount:
		switch {
		case len(cfg.ServiceAccountKeyJSON) > 0:
			opts = append(opts, option.WithCredentialsJSON(cfg.ServiceAccountKeyJSON)) //nolint:staticcheck // SA1019: option.WithCredentialsJSON is deprecated but requires using impersonate package for migration
		case cfg.ServiceAccountKeyFile != "":
			opts = append(opts, option.WithCredentialsFile(cfg.ServiceAccountKeyFile)) //nolint:staticcheck // SA1019: option.WithCredentialsFile is deprecated but requires using impersonate package for migration
		default:
			return nil, errors.New("service_account_key_file or service_account_key_json is required for service account authentication")
		}

	case AuthMethodWorkloadIdentity:
		// Workload identity uses default credentials when running on GKE/Cloud Run
		// No additional configuration needed

	case AuthMethodImpersonation:
		if cfg.ImpersonateServiceAccount == "" {
			return nil, errors.New("impersonate_service_account is required for impersonation authentication")
		}
		opts = append(opts, option.ImpersonateCredentials(cfg.ImpersonateServiceAccount, cfg.ImpersonateDelegates...)) //nolint:staticcheck // SA1019: option.ImpersonateCredentials is deprecated but requires using impersonate package for migration

	default:
		return nil, fmt.Errorf("unsupported auth method: %s", cfg.AuthMethod)
	}

	return opts, nil
}

// SecretValue represents a secret version value from GCP Secret Manager.
type SecretValue struct {
	// Name is the full resource name of the secret version.
	Name string `json:"name"`

	// SecretName is the secret name (without version).
	SecretName string `json:"secret_name"`

	// Version is the version number or alias.
	Version string `json:"version"`

	// Data is the secret payload.
	Data []byte `json:"data"`

	// DataString is the secret payload as a string.
	DataString string `json:"data_string"`

	// State is the version state.
	State SecretVersionState `json:"state"`

	// CreateTime is when the version was created.
	CreateTime time.Time `json:"create_time,omitempty"`

	// DestroyTime is when the version was destroyed (if applicable).
	DestroyTime time.Time `json:"destroy_time,omitempty"`

	// ReplicationStatus contains replication status information.
	ReplicationStatus map[string]string `json:"replication_status,omitempty"`

	// Etag is the version etag.
	Etag string `json:"etag,omitempty"`

	// ClientSpecifiedPayloadChecksum indicates if the payload has a client checksum.
	ClientSpecifiedPayloadChecksum bool `json:"client_specified_payload_checksum,omitempty"`
}

// SecretVersionState represents the state of a secret version.
type SecretVersionState string

const (
	// SecretVersionStateEnabled indicates the version is enabled.
	SecretVersionStateEnabled SecretVersionState = "ENABLED"

	// SecretVersionStateDisabled indicates the version is disabled.
	SecretVersionStateDisabled SecretVersionState = "DISABLED"

	// SecretVersionStateDestroyed indicates the version is destroyed.
	SecretVersionStateDestroyed SecretVersionState = "DESTROYED"
)

// IsEnabled returns true if the version is enabled.
func (s *SecretValue) IsEnabled() bool {
	return s.State == SecretVersionStateEnabled
}

// IsDisabled returns true if the version is disabled.
func (s *SecretValue) IsDisabled() bool {
	return s.State == SecretVersionStateDisabled
}

// IsDestroyed returns true if the version is destroyed.
func (s *SecretValue) IsDestroyed() bool {
	return s.State == SecretVersionStateDestroyed
}

// GetString returns the secret data as a string.
func (s *SecretValue) GetString() string {
	if s.DataString != "" {
		return s.DataString
	}
	return string(s.Data)
}

// GetJSON parses the secret data as JSON into the provided interface.
func (s *SecretValue) GetJSON(v interface{}) error {
	return json.Unmarshal(s.Data, v)
}

// GetMap parses the secret data as a JSON object and returns it as a map.
func (s *SecretValue) GetMap() (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(s.Data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SecretMetadata represents metadata about a secret.
type SecretMetadata struct {
	// Name is the full resource name of the secret.
	Name string `json:"name"`

	// CreateTime is when the secret was created.
	CreateTime time.Time `json:"create_time,omitempty"`

	// Labels are user-defined labels.
	Labels map[string]string `json:"labels,omitempty"`

	// Replication contains replication configuration.
	Replication *ReplicationConfig `json:"replication,omitempty"`

	// Rotation contains rotation configuration.
	Rotation *RotationConfig `json:"rotation,omitempty"`

	// Expiration contains expiration configuration.
	Expiration *ExpirationConfig `json:"expiration,omitempty"`

	// Topics are Pub/Sub topics for notifications.
	Topics []string `json:"topics,omitempty"`

	// Etag is the secret etag.
	Etag string `json:"etag,omitempty"`

	// VersionAliases maps alias names to version numbers.
	VersionAliases map[string]int64 `json:"version_aliases,omitempty"`

	// Annotations are user-defined annotations.
	Annotations map[string]string `json:"annotations,omitempty"`

	// CustomerManagedEncryption contains CMEK configuration.
	CustomerManagedEncryption *CMEKConfig `json:"customer_managed_encryption,omitempty"`
}

// ReplicationConfig contains replication configuration.
type ReplicationConfig struct {
	// Automatic indicates automatic replication is enabled.
	Automatic bool `json:"automatic,omitempty"`

	// UserManaged contains user-managed replication config.
	UserManaged *UserManagedReplication `json:"user_managed,omitempty"`
}

// UserManagedReplication contains user-managed replication configuration.
type UserManagedReplication struct {
	// Replicas are the replica configurations.
	Replicas []ReplicaConfig `json:"replicas,omitempty"`
}

// ReplicaConfig contains configuration for a single replica.
type ReplicaConfig struct {
	// Location is the replica location.
	Location string `json:"location"`

	// CustomerManagedEncryption is the CMEK config for this replica.
	CustomerManagedEncryption *CMEKConfig `json:"customer_managed_encryption,omitempty"`
}

// RotationConfig contains rotation configuration.
type RotationConfig struct {
	// NextRotationTime is when the next rotation should occur.
	NextRotationTime time.Time `json:"next_rotation_time,omitempty"`

	// RotationPeriod is the rotation period.
	RotationPeriod time.Duration `json:"rotation_period,omitempty"`
}

// ExpirationConfig contains expiration configuration.
type ExpirationConfig struct {
	// ExpireTime is the absolute expiration time.
	ExpireTime time.Time `json:"expire_time,omitempty"`

	// TTL is the time-to-live from creation.
	TTL time.Duration `json:"ttl,omitempty"`
}

// CMEKConfig contains customer-managed encryption key configuration.
type CMEKConfig struct {
	// KMSKeyName is the Cloud KMS key name.
	KMSKeyName string `json:"kms_key_name"`
}

// SecretVersionInfo represents information about a secret version.
type SecretVersionInfo struct {
	// Name is the full resource name of the secret version.
	Name string `json:"name"`

	// Version is the version number.
	Version string `json:"version"`

	// State is the version state.
	State SecretVersionState `json:"state"`

	// CreateTime is when the version was created.
	CreateTime time.Time `json:"create_time,omitempty"`

	// DestroyTime is when the version was destroyed (if applicable).
	DestroyTime time.Time `json:"destroy_time,omitempty"`

	// Etag is the version etag.
	Etag string `json:"etag,omitempty"`

	// ClientSpecifiedPayloadChecksum indicates if the payload has a client checksum.
	ClientSpecifiedPayloadChecksum bool `json:"client_specified_payload_checksum,omitempty"`

	// ReplicationStatus contains replication status information.
	ReplicationStatus map[string]string `json:"replication_status,omitempty"`
}

// SecretListEntry represents a secret in the list.
type SecretListEntry struct {
	// Name is the full resource name of the secret.
	Name string `json:"name"`

	// ShortName is the secret name without the project prefix.
	ShortName string `json:"short_name"`

	// CreateTime is when the secret was created.
	CreateTime time.Time `json:"create_time,omitempty"`

	// Labels are user-defined labels.
	Labels map[string]string `json:"labels,omitempty"`

	// Etag is the secret etag.
	Etag string `json:"etag,omitempty"`
}

// GetSecret retrieves a secret version from GCP Secret Manager.
func (c *Client) GetSecret(ctx context.Context, name string, opts ...GetSecretOption) (*SecretValue, error) {
	options := &getSecretOptions{
		version: "latest",
	}
	for _, opt := range opts {
		opt(options)
	}

	// Build the version name
	versionName := c.buildVersionName(name, options.version)

	// Access the secret version
	result, err := c.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: versionName,
	})
	if err != nil {
		return nil, translateError(err)
	}

	// Parse the version name to extract version number
	version := extractVersionFromName(result.Name)

	return &SecretValue{
		Name:       result.Name,
		SecretName: name,
		Version:    version,
		Data:       result.Payload.Data,
		DataString: string(result.Payload.Data),
	}, nil
}

// GetSecretOption configures GetSecret behavior.
type GetSecretOption func(*getSecretOptions)

type getSecretOptions struct {
	version string
}

// WithVersion retrieves a specific version of the secret.
func WithVersion(version string) GetSecretOption {
	return func(o *getSecretOptions) {
		o.version = version
	}
}

// WithLatestVersion retrieves the latest version.
func WithLatestVersion() GetSecretOption {
	return func(o *getSecretOptions) {
		o.version = "latest"
	}
}

// GetSecretMetadata retrieves metadata about a secret.
func (c *Client) GetSecretMetadata(ctx context.Context, name string) (*SecretMetadata, error) {
	secretName := c.buildSecretName(name)

	result, err := c.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{
		Name: secretName,
	})
	if err != nil {
		return nil, translateError(err)
	}

	return secretMetadataFromProto(result), nil
}

// GetSecretVersionMetadata retrieves metadata about a specific secret version.
func (c *Client) GetSecretVersionMetadata(ctx context.Context, name, version string) (*SecretVersionInfo, error) {
	versionName := c.buildVersionName(name, version)

	result, err := c.client.GetSecretVersion(ctx, &secretmanagerpb.GetSecretVersionRequest{
		Name: versionName,
	})
	if err != nil {
		return nil, translateError(err)
	}

	return secretVersionInfoFromProto(result), nil
}

// ListSecrets lists secrets in the project.
func (c *Client) ListSecrets(ctx context.Context, opts ...ListSecretsOption) ([]*SecretListEntry, error) {
	options := &listSecretsOptions{}
	for _, opt := range opts {
		opt(options)
	}

	req := &secretmanagerpb.ListSecretsRequest{
		Parent:   c.buildParent(),
		PageSize: int32(options.pageSize), //nolint:gosec // G115: pagination size
		Filter:   options.filter,
	}

	var entries []*SecretListEntry
	it := c.client.ListSecrets(ctx, req)
	for {
		secret, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, translateError(err)
		}

		entries = append(entries, &SecretListEntry{
			Name:       secret.Name,
			ShortName:  extractSecretShortName(secret.Name),
			CreateTime: secret.CreateTime.AsTime(),
			Labels:     secret.Labels,
			Etag:       secret.Etag,
		})
	}

	return entries, nil
}

// ListSecretsOption configures ListSecrets behavior.
type ListSecretsOption func(*listSecretsOptions)

type listSecretsOptions struct {
	pageSize int
	filter   string
}

// WithPageSize sets the page size for listing.
func WithPageSize(size int) ListSecretsOption {
	return func(o *listSecretsOptions) {
		o.pageSize = size
	}
}

// WithFilter sets a filter for listing.
func WithFilter(filter string) ListSecretsOption {
	return func(o *listSecretsOptions) {
		o.filter = filter
	}
}

// ListSecretVersions lists all versions of a secret.
func (c *Client) ListSecretVersions(ctx context.Context, name string, opts ...ListVersionsOption) ([]*SecretVersionInfo, error) {
	options := &listVersionsOptions{}
	for _, opt := range opts {
		opt(options)
	}

	secretName := c.buildSecretName(name)

	req := &secretmanagerpb.ListSecretVersionsRequest{
		Parent:   secretName,
		PageSize: int32(options.pageSize), //nolint:gosec // G115: pagination size
		Filter:   options.filter,
	}

	var versions []*SecretVersionInfo
	it := c.client.ListSecretVersions(ctx, req)
	for {
		version, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, translateError(err)
		}

		versions = append(versions, secretVersionInfoFromProto(version))
	}

	return versions, nil
}

// ListVersionsOption configures ListSecretVersions behavior.
type ListVersionsOption func(*listVersionsOptions)

type listVersionsOptions struct {
	pageSize int
	filter   string
}

// WithVersionsPageSize sets the page size for version listing.
func WithVersionsPageSize(size int) ListVersionsOption {
	return func(o *listVersionsOptions) {
		o.pageSize = size
	}
}

// WithVersionsFilter sets a filter for version listing.
func WithVersionsFilter(filter string) ListVersionsOption {
	return func(o *listVersionsOptions) {
		o.filter = filter
	}
}

// EnableSecretVersion enables a disabled secret version.
func (c *Client) EnableSecretVersion(ctx context.Context, name, version string) (*SecretVersionInfo, error) {
	versionName := c.buildVersionName(name, version)

	result, err := c.client.EnableSecretVersion(ctx, &secretmanagerpb.EnableSecretVersionRequest{
		Name: versionName,
	})
	if err != nil {
		return nil, translateError(err)
	}

	return secretVersionInfoFromProto(result), nil
}

// DisableSecretVersion disables an enabled secret version.
func (c *Client) DisableSecretVersion(ctx context.Context, name, version string) (*SecretVersionInfo, error) {
	versionName := c.buildVersionName(name, version)

	result, err := c.client.DisableSecretVersion(ctx, &secretmanagerpb.DisableSecretVersionRequest{
		Name: versionName,
	})
	if err != nil {
		return nil, translateError(err)
	}

	return secretVersionInfoFromProto(result), nil
}

// DestroySecretVersion destroys a secret version.
func (c *Client) DestroySecretVersion(ctx context.Context, name, version string) (*SecretVersionInfo, error) {
	versionName := c.buildVersionName(name, version)

	result, err := c.client.DestroySecretVersion(ctx, &secretmanagerpb.DestroySecretVersionRequest{
		Name: versionName,
	})
	if err != nil {
		return nil, translateError(err)
	}

	return secretVersionInfoFromProto(result), nil
}

// Close closes the client.
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Helper functions

func (c *Client) buildParent() string {
	return fmt.Sprintf("projects/%s", c.projectID)
}

func (c *Client) buildSecretName(name string) string {
	// If already a full resource name, return as-is
	if strings.HasPrefix(name, "projects/") {
		return name
	}
	return fmt.Sprintf("projects/%s/secrets/%s", c.projectID, name)
}

func (c *Client) buildVersionName(name, version string) string {
	secretName := c.buildSecretName(name)
	return fmt.Sprintf("%s/versions/%s", secretName, version)
}

func extractVersionFromName(name string) string {
	parts := strings.Split(name, "/versions/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func extractSecretShortName(name string) string {
	// Format: projects/{project}/secrets/{secret}
	parts := strings.Split(name, "/secrets/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return name
}

func secretMetadataFromProto(s *secretmanagerpb.Secret) *SecretMetadata {
	meta := &SecretMetadata{
		Name:        s.Name,
		Labels:      s.Labels,
		Etag:        s.Etag,
		Annotations: s.Annotations,
	}

	if s.CreateTime != nil {
		meta.CreateTime = s.CreateTime.AsTime()
	}

	// Parse replication config
	if s.Replication != nil {
		meta.Replication = &ReplicationConfig{}
		switch r := s.Replication.Replication.(type) {
		case *secretmanagerpb.Replication_Automatic_:
			meta.Replication.Automatic = true
			if r.Automatic.CustomerManagedEncryption != nil {
				meta.CustomerManagedEncryption = &CMEKConfig{
					KMSKeyName: r.Automatic.CustomerManagedEncryption.KmsKeyName,
				}
			}
		case *secretmanagerpb.Replication_UserManaged_:
			meta.Replication.UserManaged = &UserManagedReplication{}
			for _, replica := range r.UserManaged.Replicas {
				rc := ReplicaConfig{Location: replica.Location}
				if replica.CustomerManagedEncryption != nil {
					rc.CustomerManagedEncryption = &CMEKConfig{
						KMSKeyName: replica.CustomerManagedEncryption.KmsKeyName,
					}
				}
				meta.Replication.UserManaged.Replicas = append(meta.Replication.UserManaged.Replicas, rc)
			}
		}
	}

	// Parse rotation config
	if s.Rotation != nil {
		meta.Rotation = &RotationConfig{}
		if s.Rotation.NextRotationTime != nil {
			meta.Rotation.NextRotationTime = s.Rotation.NextRotationTime.AsTime()
		}
		if s.Rotation.RotationPeriod != nil {
			meta.Rotation.RotationPeriod = s.Rotation.RotationPeriod.AsDuration()
		}
	}

	// Parse expiration config
	if s.Expiration != nil {
		meta.Expiration = &ExpirationConfig{}
		switch e := s.Expiration.(type) {
		case *secretmanagerpb.Secret_ExpireTime:
			meta.Expiration.ExpireTime = e.ExpireTime.AsTime()
		case *secretmanagerpb.Secret_Ttl:
			meta.Expiration.TTL = e.Ttl.AsDuration()
		}
	}

	// Parse topics
	for _, topic := range s.Topics {
		meta.Topics = append(meta.Topics, topic.Name)
	}

	// Parse version aliases
	if len(s.VersionAliases) > 0 {
		meta.VersionAliases = s.VersionAliases
	}

	return meta
}

func secretVersionInfoFromProto(v *secretmanagerpb.SecretVersion) *SecretVersionInfo {
	info := &SecretVersionInfo{
		Name:                           v.Name,
		Version:                        extractVersionFromName(v.Name),
		State:                          SecretVersionState(v.State.String()),
		Etag:                           v.Etag,
		ClientSpecifiedPayloadChecksum: v.ClientSpecifiedPayloadChecksum,
	}

	if v.CreateTime != nil {
		info.CreateTime = v.CreateTime.AsTime()
	}
	if v.DestroyTime != nil {
		info.DestroyTime = v.DestroyTime.AsTime()
	}

	// Parse replication status
	if v.ReplicationStatus != nil {
		info.ReplicationStatus = make(map[string]string)
		if auto := v.ReplicationStatus.GetAutomatic(); auto != nil {
			if auto.CustomerManagedEncryption != nil {
				info.ReplicationStatus["kms_key_version_name"] = auto.CustomerManagedEncryption.KmsKeyVersionName
			}
		}
		if userManaged := v.ReplicationStatus.GetUserManaged(); userManaged != nil {
			for _, replica := range userManaged.Replicas {
				if replica.CustomerManagedEncryption != nil {
					info.ReplicationStatus[replica.Location] = replica.CustomerManagedEncryption.KmsKeyVersionName
				}
			}
		}
	}

	return info
}

// translateError translates GCP errors to secrets package errors.
func translateError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return err
	}

	switch st.Code() {
	case codes.NotFound:
		return secrets.ErrSecretNotFound
	case codes.PermissionDenied, codes.Unauthenticated:
		return secrets.ErrAccessDenied
	case codes.Unavailable:
		return secrets.ErrBackendUnavailable
	case codes.InvalidArgument:
		return fmt.Errorf("%w: %s", secrets.ErrInvalidPath, st.Message())
	case codes.FailedPrecondition:
		return fmt.Errorf("operation failed: %s", st.Message())
	default:
	}

	return err
}

// VersionNumber converts a version string to an integer.
func VersionNumber(version string) (int64, error) {
	if version == "latest" {
		return -1, nil
	}
	return strconv.ParseInt(version, 10, 64)
}
