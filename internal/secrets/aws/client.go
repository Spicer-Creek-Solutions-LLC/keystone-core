// Package aws provides an AWS Secrets Manager backend for the secrets broker.
package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// AuthMethod represents the authentication method for AWS.
type AuthMethod string

const (
	// AuthMethodDefault uses the default AWS credential chain.
	AuthMethodDefault AuthMethod = "default"

	// AuthMethodStatic uses explicit access key and secret key.
	AuthMethodStatic AuthMethod = "static"

	// AuthMethodInstanceProfile uses EC2 instance profile credentials.
	AuthMethodInstanceProfile AuthMethod = "instance_profile"

	// AuthMethodAssumeRole uses STS AssumeRole.
	AuthMethodAssumeRole AuthMethod = "assume_role"

	// AuthMethodWebIdentity uses STS AssumeRoleWithWebIdentity (for EKS).
	AuthMethodWebIdentity AuthMethod = "web_identity"
)

// ClientConfig configures the AWS Secrets Manager client.
type ClientConfig struct {
	// Name is the backend instance name.
	Name string `json:"name,omitempty"`

	// Region is the AWS region.
	Region string `json:"region,omitempty"`

	// AuthMethod is the authentication method to use.
	AuthMethod AuthMethod `json:"auth_method,omitempty"`

	// AccessKeyID is the AWS access key ID (for static auth).
	AccessKeyID string `json:"access_key_id,omitempty"`

	// SecretAccessKey is the AWS secret access key (for static auth).
	SecretAccessKey string `json:"secret_access_key,omitempty"`

	// SessionToken is an optional session token (for temporary credentials).
	SessionToken string `json:"session_token,omitempty"`

	// AssumeRoleARN is the ARN of the role to assume (for assume_role auth).
	AssumeRoleARN string `json:"assume_role_arn,omitempty"`

	// ExternalID is the external ID for role assumption.
	ExternalID string `json:"external_id,omitempty"`

	// RoleSessionName is the session name for role assumption.
	RoleSessionName string `json:"role_session_name,omitempty"`

	// WebIdentityTokenFile is the path to the web identity token file (for EKS).
	WebIdentityTokenFile string `json:"web_identity_token_file,omitempty"`

	// Endpoint is a custom endpoint URL (for testing with LocalStack).
	Endpoint string `json:"endpoint,omitempty"`

	// DefaultCacheTTL is the default TTL for caching secrets.
	DefaultCacheTTL time.Duration `json:"default_cache_ttl,omitempty"`

	// Timeout is the request timeout.
	Timeout time.Duration `json:"timeout,omitempty"`

	// MaxRetries is the maximum number of retries for failed requests.
	MaxRetries int `json:"max_retries,omitempty"`
}

// DefaultClientConfig returns a client configuration with sensible defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Name:            "aws",
		AuthMethod:      AuthMethodDefault,
		DefaultCacheTTL: 5 * time.Minute,
		Timeout:         30 * time.Second,
		MaxRetries:      3,
	}
}

// Client provides methods for interacting with AWS Secrets Manager.
type Client struct {
	mu     sync.RWMutex
	config *ClientConfig

	client *secretsmanager.Client
	closed atomic.Bool
}

// NewClient creates a new AWS Secrets Manager client.
func NewClient(ctx context.Context, cfg *ClientConfig) (*Client, error) {
	if cfg == nil {
		cfg = DefaultClientConfig()
	}

	// Build AWS config
	awsCfg, err := buildAWSConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build AWS config: %w", err)
	}

	// Create Secrets Manager client
	smClient := secretsmanager.NewFromConfig(awsCfg, func(o *secretsmanager.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})

	return &Client{
		config: cfg,
		client: smClient,
	}, nil
}

// buildAWSConfig builds the AWS SDK config based on the client configuration.
func buildAWSConfig(ctx context.Context, cfg *ClientConfig) (aws.Config, error) {
	var opts []func(*config.LoadOptions) error

	// Set region
	if cfg.Region != "" {
		opts = append(opts, config.WithRegion(cfg.Region))
	}

	// Set retry configuration
	if cfg.MaxRetries > 0 {
		opts = append(opts, config.WithRetryMaxAttempts(cfg.MaxRetries))
	}

	// Handle authentication method
	switch cfg.AuthMethod {
	case AuthMethodStatic:
		if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
			return aws.Config{}, fmt.Errorf("access_key_id and secret_access_key required for static auth")
		}
		creds := credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			cfg.SessionToken,
		)
		opts = append(opts, config.WithCredentialsProvider(creds))

	case AuthMethodAssumeRole:
		if cfg.AssumeRoleARN == "" {
			return aws.Config{}, fmt.Errorf("assume_role_arn required for assume_role auth")
		}
		// First load base config
		baseCfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return aws.Config{}, fmt.Errorf("failed to load base config: %w", err)
		}

		// Create STS client and assume role
		stsClient := sts.NewFromConfig(baseCfg)
		assumeRoleOpts := []func(*stscreds.AssumeRoleOptions){}
		if cfg.ExternalID != "" {
			assumeRoleOpts = append(assumeRoleOpts, func(o *stscreds.AssumeRoleOptions) {
				o.ExternalID = aws.String(cfg.ExternalID)
			})
		}
		if cfg.RoleSessionName != "" {
			assumeRoleOpts = append(assumeRoleOpts, func(o *stscreds.AssumeRoleOptions) {
				o.RoleSessionName = cfg.RoleSessionName
			})
		}

		creds := stscreds.NewAssumeRoleProvider(stsClient, cfg.AssumeRoleARN, assumeRoleOpts...)
		opts = append(opts, config.WithCredentialsProvider(creds))

	case AuthMethodWebIdentity:
		if cfg.AssumeRoleARN == "" {
			return aws.Config{}, fmt.Errorf("assume_role_arn required for web_identity auth")
		}
		// Web identity is handled automatically by the SDK when
		// AWS_WEB_IDENTITY_TOKEN_FILE and AWS_ROLE_ARN are set
		// or we can configure it explicitly
		if cfg.WebIdentityTokenFile != "" {
			baseCfg, err := config.LoadDefaultConfig(ctx, opts...)
			if err != nil {
				return aws.Config{}, fmt.Errorf("failed to load base config: %w", err)
			}
			stsClient := sts.NewFromConfig(baseCfg)
			creds := stscreds.NewWebIdentityRoleProvider(
				stsClient,
				cfg.AssumeRoleARN,
				stscreds.IdentityTokenFile(cfg.WebIdentityTokenFile),
				func(o *stscreds.WebIdentityRoleOptions) {
					if cfg.RoleSessionName != "" {
						o.RoleSessionName = cfg.RoleSessionName
					}
				},
			)
			opts = append(opts, config.WithCredentialsProvider(creds))
		}

	case AuthMethodInstanceProfile, AuthMethodDefault:
		// Use default credential chain (includes instance profile)
		// No additional configuration needed
	}

	return config.LoadDefaultConfig(ctx, opts...)
}

// Close closes the client.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	return nil
}

// GetSecret retrieves a secret by name or ARN.
func (c *Client) GetSecret(ctx context.Context, secretID string, opts ...GetSecretOption) (*SecretValue, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("client is closed")
	}

	options := &getSecretOptions{}
	for _, opt := range opts {
		opt(options)
	}

	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	}

	if options.versionID != "" {
		input.VersionId = aws.String(options.versionID)
	}
	if options.versionStage != "" {
		input.VersionStage = aws.String(options.versionStage)
	}

	result, err := c.client.GetSecretValue(ctx, input)
	if err != nil {
		return nil, translateError(err)
	}

	return &SecretValue{
		ARN:           aws.ToString(result.ARN),
		Name:          aws.ToString(result.Name),
		VersionID:     aws.ToString(result.VersionId),
		VersionStages: result.VersionStages,
		SecretString:  aws.ToString(result.SecretString),
		SecretBinary:  result.SecretBinary,
		CreatedDate:   aws.ToTime(result.CreatedDate),
	}, nil
}

// getSecretOptions holds options for GetSecret.
type getSecretOptions struct {
	versionID    string
	versionStage string
}

// GetSecretOption is a function that configures secret retrieval.
type GetSecretOption func(*getSecretOptions)

// WithVersionID retrieves a specific version by ID.
func WithVersionID(versionID string) GetSecretOption {
	return func(o *getSecretOptions) {
		o.versionID = versionID
	}
}

// WithVersionStage retrieves a specific version by stage label.
func WithVersionStage(stage string) GetSecretOption {
	return func(o *getSecretOptions) {
		o.versionStage = stage
	}
}

// SecretValue represents a secret value from AWS Secrets Manager.
type SecretValue struct {
	// ARN is the secret's Amazon Resource Name.
	ARN string `json:"arn"`

	// Name is the secret's friendly name.
	Name string `json:"name"`

	// VersionID is the unique identifier for this version.
	VersionID string `json:"version_id"`

	// VersionStages is the list of staging labels for this version.
	VersionStages []string `json:"version_stages"`

	// SecretString contains the secret if stored as a string.
	SecretString string `json:"secret_string,omitempty"`

	// SecretBinary contains the secret if stored as binary.
	SecretBinary []byte `json:"secret_binary,omitempty"`

	// CreatedDate is when this version was created.
	CreatedDate time.Time `json:"created_date"`
}

// GetString returns the secret as a string.
func (s *SecretValue) GetString() string {
	return s.SecretString
}

// GetBinary returns the secret as binary data.
func (s *SecretValue) GetBinary() []byte {
	if s.SecretBinary != nil {
		return s.SecretBinary
	}
	return []byte(s.SecretString)
}

// GetJSON unmarshals the secret string as JSON into the provided value.
func (s *SecretValue) GetJSON(v interface{}) error {
	if s.SecretString == "" {
		return fmt.Errorf("secret has no string value")
	}
	return json.Unmarshal([]byte(s.SecretString), v)
}

// GetMap returns the secret as a map if it's JSON.
func (s *SecretValue) GetMap() (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := s.GetJSON(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// IsCurrentVersion returns true if this is the current version.
func (s *SecretValue) IsCurrentVersion() bool {
	for _, stage := range s.VersionStages {
		if stage == "AWSCURRENT" {
			return true
		}
	}
	return false
}

// IsPreviousVersion returns true if this is the previous version.
func (s *SecretValue) IsPreviousVersion() bool {
	for _, stage := range s.VersionStages {
		if stage == "AWSPREVIOUS" {
			return true
		}
	}
	return false
}

// IsPendingVersion returns true if this is a pending version (during rotation).
func (s *SecretValue) IsPendingVersion() bool {
	for _, stage := range s.VersionStages {
		if stage == "AWSPENDING" {
			return true
		}
	}
	return false
}

// DescribeSecret retrieves metadata about a secret.
func (c *Client) DescribeSecret(ctx context.Context, secretID string) (*SecretMetadata, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("client is closed")
	}

	input := &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretID),
	}

	result, err := c.client.DescribeSecret(ctx, input)
	if err != nil {
		return nil, translateError(err)
	}

	metadata := &SecretMetadata{
		ARN:                    aws.ToString(result.ARN),
		Name:                   aws.ToString(result.Name),
		Description:            aws.ToString(result.Description),
		KmsKeyId:               aws.ToString(result.KmsKeyId),
		RotationEnabled:        aws.ToBool(result.RotationEnabled),
		RotationLambdaARN:      aws.ToString(result.RotationLambdaARN),
		LastRotatedDate:        aws.ToTime(result.LastRotatedDate),
		LastChangedDate:        aws.ToTime(result.LastChangedDate),
		LastAccessedDate:       aws.ToTime(result.LastAccessedDate),
		DeletedDate:            aws.ToTime(result.DeletedDate),
		NextRotationDate:       aws.ToTime(result.NextRotationDate),
		Tags:                   make(map[string]string),
		VersionIdsToStages:     make(map[string][]string),
		OwningService:          aws.ToString(result.OwningService),
		PrimaryRegion:          aws.ToString(result.PrimaryRegion),
		CreatedDate:            aws.ToTime(result.CreatedDate),
	}

	for _, tag := range result.Tags {
		metadata.Tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	for versionID, stages := range result.VersionIdsToStages {
		metadata.VersionIdsToStages[versionID] = stages
	}

	if result.RotationRules != nil {
		metadata.RotationRules = &RotationRules{
			AutomaticallyAfterDays: aws.ToInt64(result.RotationRules.AutomaticallyAfterDays),
			Duration:               aws.ToString(result.RotationRules.Duration),
			ScheduleExpression:     aws.ToString(result.RotationRules.ScheduleExpression),
		}
	}

	if result.ReplicationStatus != nil {
		for _, rs := range result.ReplicationStatus {
			metadata.ReplicationStatus = append(metadata.ReplicationStatus, ReplicationStatus{
				Region:           aws.ToString(rs.Region),
				Status:           string(rs.Status),
				StatusMessage:    aws.ToString(rs.StatusMessage),
				KmsKeyId:         aws.ToString(rs.KmsKeyId),
				LastAccessedDate: aws.ToTime(rs.LastAccessedDate),
			})
		}
	}

	return metadata, nil
}

// SecretMetadata contains metadata about a secret.
type SecretMetadata struct {
	// ARN is the secret's Amazon Resource Name.
	ARN string `json:"arn"`

	// Name is the secret's friendly name.
	Name string `json:"name"`

	// Description is the secret's description.
	Description string `json:"description,omitempty"`

	// KmsKeyId is the KMS key used to encrypt the secret.
	KmsKeyId string `json:"kms_key_id,omitempty"`

	// RotationEnabled indicates if automatic rotation is enabled.
	RotationEnabled bool `json:"rotation_enabled"`

	// RotationLambdaARN is the ARN of the Lambda function for rotation.
	RotationLambdaARN string `json:"rotation_lambda_arn,omitempty"`

	// RotationRules contains rotation schedule rules.
	RotationRules *RotationRules `json:"rotation_rules,omitempty"`

	// LastRotatedDate is when the secret was last rotated.
	LastRotatedDate time.Time `json:"last_rotated_date,omitempty"`

	// LastChangedDate is when the secret was last changed.
	LastChangedDate time.Time `json:"last_changed_date,omitempty"`

	// LastAccessedDate is when the secret was last accessed.
	LastAccessedDate time.Time `json:"last_accessed_date,omitempty"`

	// DeletedDate is when the secret was deleted (if pending deletion).
	DeletedDate time.Time `json:"deleted_date,omitempty"`

	// NextRotationDate is when the next rotation is scheduled.
	NextRotationDate time.Time `json:"next_rotation_date,omitempty"`

	// Tags are the secret's tags.
	Tags map[string]string `json:"tags,omitempty"`

	// VersionIdsToStages maps version IDs to their staging labels.
	VersionIdsToStages map[string][]string `json:"version_ids_to_stages,omitempty"`

	// OwningService is the service that created the secret.
	OwningService string `json:"owning_service,omitempty"`

	// PrimaryRegion is the primary region for replicated secrets.
	PrimaryRegion string `json:"primary_region,omitempty"`

	// ReplicationStatus contains replication status for replicated secrets.
	ReplicationStatus []ReplicationStatus `json:"replication_status,omitempty"`

	// CreatedDate is when the secret was created.
	CreatedDate time.Time `json:"created_date,omitempty"`
}

// RotationRules contains rotation schedule configuration.
type RotationRules struct {
	// AutomaticallyAfterDays is the number of days between automatic rotations.
	AutomaticallyAfterDays int64 `json:"automatically_after_days,omitempty"`

	// Duration is the rotation window duration.
	Duration string `json:"duration,omitempty"`

	// ScheduleExpression is the cron expression for rotation.
	ScheduleExpression string `json:"schedule_expression,omitempty"`
}

// ReplicationStatus contains replication status for a region.
type ReplicationStatus struct {
	// Region is the replication region.
	Region string `json:"region"`

	// Status is the replication status.
	Status string `json:"status"`

	// StatusMessage contains additional status information.
	StatusMessage string `json:"status_message,omitempty"`

	// KmsKeyId is the KMS key used in the replica region.
	KmsKeyId string `json:"kms_key_id,omitempty"`

	// LastAccessedDate is when the replica was last accessed.
	LastAccessedDate time.Time `json:"last_accessed_date,omitempty"`
}

// ListSecrets lists secrets matching optional filters.
func (c *Client) ListSecrets(ctx context.Context, opts ...ListSecretsOption) ([]*SecretListEntry, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("client is closed")
	}

	options := &listSecretsOptions{}
	for _, opt := range opts {
		opt(options)
	}

	input := &secretsmanager.ListSecretsInput{}

	if len(options.filters) > 0 {
		input.Filters = options.filters
	}
	if options.maxResults > 0 {
		input.MaxResults = aws.Int32(int32(options.maxResults))
	}

	var entries []*SecretListEntry
	paginator := secretsmanager.NewListSecretsPaginator(c.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, translateError(err)
		}

		for _, secret := range page.SecretList {
			entry := &SecretListEntry{
				ARN:               aws.ToString(secret.ARN),
				Name:              aws.ToString(secret.Name),
				Description:       aws.ToString(secret.Description),
				KmsKeyId:          aws.ToString(secret.KmsKeyId),
				RotationEnabled:   aws.ToBool(secret.RotationEnabled),
				RotationLambdaARN: aws.ToString(secret.RotationLambdaARN),
				LastRotatedDate:   aws.ToTime(secret.LastRotatedDate),
				LastChangedDate:   aws.ToTime(secret.LastChangedDate),
				LastAccessedDate:  aws.ToTime(secret.LastAccessedDate),
				DeletedDate:       aws.ToTime(secret.DeletedDate),
				CreatedDate:       aws.ToTime(secret.CreatedDate),
				Tags:              make(map[string]string),
			}

			for _, tag := range secret.Tags {
				entry.Tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
			}

			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// listSecretsOptions holds options for ListSecrets.
type listSecretsOptions struct {
	filters    []types.Filter
	maxResults int
}

// ListSecretsOption is a function that configures secret listing.
type ListSecretsOption func(*listSecretsOptions)

// WithNamePrefix filters secrets by name prefix.
func WithNamePrefix(prefix string) ListSecretsOption {
	return func(o *listSecretsOptions) {
		o.filters = append(o.filters, types.Filter{
			Key:    types.FilterNameStringTypeName,
			Values: []string{prefix},
		})
	}
}

// WithTagKey filters secrets by tag key.
func WithTagKey(key string) ListSecretsOption {
	return func(o *listSecretsOptions) {
		o.filters = append(o.filters, types.Filter{
			Key:    types.FilterNameStringTypeTagKey,
			Values: []string{key},
		})
	}
}

// WithTagValue filters secrets by tag value.
func WithTagValue(key, value string) ListSecretsOption {
	return func(o *listSecretsOptions) {
		o.filters = append(o.filters, types.Filter{
			Key:    types.FilterNameStringTypeTagValue,
			Values: []string{value},
		})
	}
}

// WithDescription filters secrets by description.
func WithDescription(desc string) ListSecretsOption {
	return func(o *listSecretsOptions) {
		o.filters = append(o.filters, types.Filter{
			Key:    types.FilterNameStringTypeDescription,
			Values: []string{desc},
		})
	}
}

// WithMaxResults limits the number of results.
func WithMaxResults(max int) ListSecretsOption {
	return func(o *listSecretsOptions) {
		o.maxResults = max
	}
}

// SecretListEntry represents a secret in a list response.
type SecretListEntry struct {
	// ARN is the secret's Amazon Resource Name.
	ARN string `json:"arn"`

	// Name is the secret's friendly name.
	Name string `json:"name"`

	// Description is the secret's description.
	Description string `json:"description,omitempty"`

	// KmsKeyId is the KMS key used to encrypt the secret.
	KmsKeyId string `json:"kms_key_id,omitempty"`

	// RotationEnabled indicates if automatic rotation is enabled.
	RotationEnabled bool `json:"rotation_enabled"`

	// RotationLambdaARN is the ARN of the Lambda function for rotation.
	RotationLambdaARN string `json:"rotation_lambda_arn,omitempty"`

	// LastRotatedDate is when the secret was last rotated.
	LastRotatedDate time.Time `json:"last_rotated_date,omitempty"`

	// LastChangedDate is when the secret was last changed.
	LastChangedDate time.Time `json:"last_changed_date,omitempty"`

	// LastAccessedDate is when the secret was last accessed.
	LastAccessedDate time.Time `json:"last_accessed_date,omitempty"`

	// DeletedDate is when the secret was deleted (if pending deletion).
	DeletedDate time.Time `json:"deleted_date,omitempty"`

	// CreatedDate is when the secret was created.
	CreatedDate time.Time `json:"created_date,omitempty"`

	// Tags are the secret's tags.
	Tags map[string]string `json:"tags,omitempty"`
}

// ListSecretVersions lists all versions of a secret.
func (c *Client) ListSecretVersions(ctx context.Context, secretID string) ([]*SecretVersionInfo, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("client is closed")
	}

	input := &secretsmanager.ListSecretVersionIdsInput{
		SecretId:         aws.String(secretID),
		IncludeDeprecated: aws.Bool(true),
	}

	var versions []*SecretVersionInfo
	paginator := secretsmanager.NewListSecretVersionIdsPaginator(c.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, translateError(err)
		}

		for _, v := range page.Versions {
			versions = append(versions, &SecretVersionInfo{
				VersionID:      aws.ToString(v.VersionId),
				VersionStages:  v.VersionStages,
				CreatedDate:    aws.ToTime(v.CreatedDate),
				LastAccessedDate: aws.ToTime(v.LastAccessedDate),
			})
		}
	}

	return versions, nil
}

// SecretVersionInfo contains information about a secret version.
type SecretVersionInfo struct {
	// VersionID is the unique identifier for this version.
	VersionID string `json:"version_id"`

	// VersionStages is the list of staging labels for this version.
	VersionStages []string `json:"version_stages"`

	// CreatedDate is when this version was created.
	CreatedDate time.Time `json:"created_date"`

	// LastAccessedDate is when this version was last accessed.
	LastAccessedDate time.Time `json:"last_accessed_date,omitempty"`
}

// IsCurrentVersion returns true if this is the current version.
func (v *SecretVersionInfo) IsCurrentVersion() bool {
	for _, stage := range v.VersionStages {
		if stage == "AWSCURRENT" {
			return true
		}
	}
	return false
}

// IsPreviousVersion returns true if this is the previous version.
func (v *SecretVersionInfo) IsPreviousVersion() bool {
	for _, stage := range v.VersionStages {
		if stage == "AWSPREVIOUS" {
			return true
		}
	}
	return false
}

// RotateSecret initiates rotation of a secret.
func (c *Client) RotateSecret(ctx context.Context, secretID string, opts ...RotateSecretOption) (*RotateSecretResult, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("client is closed")
	}

	options := &rotateSecretOptions{}
	for _, opt := range opts {
		opt(options)
	}

	input := &secretsmanager.RotateSecretInput{
		SecretId: aws.String(secretID),
	}

	if options.clientRequestToken != "" {
		input.ClientRequestToken = aws.String(options.clientRequestToken)
	}
	if options.rotationLambdaARN != "" {
		input.RotationLambdaARN = aws.String(options.rotationLambdaARN)
	}
	if options.rotationRules != nil {
		input.RotationRules = options.rotationRules
	}
	if options.rotateImmediately {
		input.RotateImmediately = aws.Bool(true)
	}

	result, err := c.client.RotateSecret(ctx, input)
	if err != nil {
		return nil, translateError(err)
	}

	return &RotateSecretResult{
		ARN:       aws.ToString(result.ARN),
		Name:      aws.ToString(result.Name),
		VersionID: aws.ToString(result.VersionId),
	}, nil
}

// rotateSecretOptions holds options for RotateSecret.
type rotateSecretOptions struct {
	clientRequestToken string
	rotationLambdaARN  string
	rotationRules      *types.RotationRulesType
	rotateImmediately  bool
}

// RotateSecretOption is a function that configures secret rotation.
type RotateSecretOption func(*rotateSecretOptions)

// WithRotationLambda sets the Lambda function for rotation.
func WithRotationLambda(arn string) RotateSecretOption {
	return func(o *rotateSecretOptions) {
		o.rotationLambdaARN = arn
	}
}

// WithRotationDays sets the rotation interval in days.
func WithRotationDays(days int64) RotateSecretOption {
	return func(o *rotateSecretOptions) {
		if o.rotationRules == nil {
			o.rotationRules = &types.RotationRulesType{}
		}
		o.rotationRules.AutomaticallyAfterDays = aws.Int64(days)
	}
}

// WithRotateImmediately requests immediate rotation.
func WithRotateImmediately() RotateSecretOption {
	return func(o *rotateSecretOptions) {
		o.rotateImmediately = true
	}
}

// RotateSecretResult contains the result of a rotation request.
type RotateSecretResult struct {
	// ARN is the secret's Amazon Resource Name.
	ARN string `json:"arn"`

	// Name is the secret's friendly name.
	Name string `json:"name"`

	// VersionID is the ID of the new version created during rotation.
	VersionID string `json:"version_id"`
}

// translateError translates AWS errors to secrets package errors.
func translateError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	switch {
	case strings.Contains(errMsg, "ResourceNotFoundException"):
		return secrets.ErrSecretNotFound
	case strings.Contains(errMsg, "AccessDeniedException"):
		return secrets.ErrAccessDenied
	case strings.Contains(errMsg, "InvalidParameterException"):
		return secrets.ErrInvalidPath
	case strings.Contains(errMsg, "InvalidRequestException"):
		return secrets.ErrInvalidPath
	case strings.Contains(errMsg, "DecryptionFailure"):
		return secrets.ErrCacheDecryptionFailed
	case strings.Contains(errMsg, "InternalServiceError"):
		return secrets.ErrBackendUnavailable
	default:
		return err
	}
}
