package aws

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()

	if cfg.Name != "aws" {
		t.Errorf("expected name 'aws', got %s", cfg.Name)
	}
	if cfg.AuthMethod != AuthMethodDefault {
		t.Errorf("expected auth method 'default', got %s", cfg.AuthMethod)
	}
	if cfg.DefaultCacheTTL != 5*time.Minute {
		t.Errorf("expected default cache TTL 5m, got %v", cfg.DefaultCacheTTL)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", cfg.Timeout)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected max retries 3, got %d", cfg.MaxRetries)
	}
}

func TestDefaultBackendConfig(t *testing.T) {
	cfg := DefaultBackendConfig()

	if cfg.Name != "aws" {
		t.Errorf("expected name 'aws', got %s", cfg.Name)
	}
	if cfg.PathPrefix != "aws/" {
		t.Errorf("expected path prefix 'aws/', got %s", cfg.PathPrefix)
	}
	if cfg.DefaultCacheTTL != 5*time.Minute {
		t.Errorf("expected default cache TTL 5m, got %v", cfg.DefaultCacheTTL)
	}
	if !cfg.JSONKeys {
		t.Error("expected JSON keys enabled")
	}
}

func TestAuthMethodValidation(t *testing.T) {
	tests := []struct {
		name       string
		authMethod AuthMethod
		config     *ClientConfig
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "static auth missing access key",
			authMethod: AuthMethodStatic,
			config: &ClientConfig{
				AuthMethod:      AuthMethodStatic,
				SecretAccessKey: "secret",
			},
			wantErr: true,
			errMsg:  "access_key_id",
		},
		{
			name:       "static auth missing secret key",
			authMethod: AuthMethodStatic,
			config: &ClientConfig{
				AuthMethod:  AuthMethodStatic,
				AccessKeyID: "access",
			},
			wantErr: true,
			errMsg:  "secret_access_key",
		},
		{
			name:       "assume role missing ARN",
			authMethod: AuthMethodAssumeRole,
			config: &ClientConfig{
				AuthMethod: AuthMethodAssumeRole,
			},
			wantErr: true,
			errMsg:  "assume_role_arn",
		},
		{
			name:       "web identity missing ARN",
			authMethod: AuthMethodWebIdentity,
			config: &ClientConfig{
				AuthMethod: AuthMethodWebIdentity,
			},
			wantErr: true,
			errMsg:  "assume_role_arn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := buildAWSConfig(ctx, tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSecretValueMethods(t *testing.T) {
	t.Run("GetString", func(t *testing.T) {
		sv := &SecretValue{SecretString: "test-secret"}
		if sv.GetString() != "test-secret" {
			t.Errorf("expected 'test-secret', got %s", sv.GetString())
		}
	})

	t.Run("GetBinary from binary", func(t *testing.T) {
		sv := &SecretValue{SecretBinary: []byte("binary-data")}
		if string(sv.GetBinary()) != "binary-data" {
			t.Errorf("expected 'binary-data', got %s", string(sv.GetBinary()))
		}
	})

	t.Run("GetBinary from string", func(t *testing.T) {
		sv := &SecretValue{SecretString: "string-data"}
		if string(sv.GetBinary()) != "string-data" {
			t.Errorf("expected 'string-data', got %s", string(sv.GetBinary()))
		}
	})

	t.Run("GetJSON success", func(t *testing.T) {
		sv := &SecretValue{SecretString: `{"key":"value"}`}
		var m map[string]string
		if err := sv.GetJSON(&m); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m["key"] != "value" {
			t.Errorf("expected key='value', got %s", m["key"])
		}
	})

	t.Run("GetJSON empty string", func(t *testing.T) {
		sv := &SecretValue{SecretString: ""}
		var m map[string]string
		if err := sv.GetJSON(&m); err == nil {
			t.Error("expected error for empty string")
		}
	})

	t.Run("GetMap success", func(t *testing.T) {
		sv := &SecretValue{SecretString: `{"key":"value","num":123}`}
		m, err := sv.GetMap()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m["key"] != "value" {
			t.Errorf("expected key='value', got %v", m["key"])
		}
	})

	t.Run("GetMap invalid JSON", func(t *testing.T) {
		sv := &SecretValue{SecretString: "not json"}
		_, err := sv.GetMap()
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestSecretValueVersionStages(t *testing.T) {
	tests := []struct {
		name         string
		stages       []string
		wantCurrent  bool
		wantPrevious bool
		wantPending  bool
	}{
		{
			name:        "AWSCURRENT",
			stages:      []string{"AWSCURRENT"},
			wantCurrent: true,
		},
		{
			name:         "AWSPREVIOUS",
			stages:       []string{"AWSPREVIOUS"},
			wantPrevious: true,
		},
		{
			name:        "AWSPENDING",
			stages:      []string{"AWSPENDING"},
			wantPending: true,
		},
		{
			name:         "multiple stages",
			stages:       []string{"AWSCURRENT", "custom-label"},
			wantCurrent:  true,
			wantPrevious: false,
		},
		{
			name:   "no stages",
			stages: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv := &SecretValue{VersionStages: tt.stages}
			if sv.IsCurrentVersion() != tt.wantCurrent {
				t.Errorf("IsCurrentVersion() = %v, want %v", sv.IsCurrentVersion(), tt.wantCurrent)
			}
			if sv.IsPreviousVersion() != tt.wantPrevious {
				t.Errorf("IsPreviousVersion() = %v, want %v", sv.IsPreviousVersion(), tt.wantPrevious)
			}
			if sv.IsPendingVersion() != tt.wantPending {
				t.Errorf("IsPendingVersion() = %v, want %v", sv.IsPendingVersion(), tt.wantPending)
			}
		})
	}
}

func TestSecretVersionInfoStages(t *testing.T) {
	t.Run("current version", func(t *testing.T) {
		v := &SecretVersionInfo{VersionStages: []string{"AWSCURRENT"}}
		if !v.IsCurrentVersion() {
			t.Error("expected IsCurrentVersion() to be true")
		}
	})

	t.Run("previous version", func(t *testing.T) {
		v := &SecretVersionInfo{VersionStages: []string{"AWSPREVIOUS"}}
		if !v.IsPreviousVersion() {
			t.Error("expected IsPreviousVersion() to be true")
		}
	})
}

func TestTranslateError(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		wantErr error
	}{
		{
			name:    "ResourceNotFoundException",
			errMsg:  "ResourceNotFoundException: Secret not found",
			wantErr: secrets.ErrSecretNotFound,
		},
		{
			name:    "AccessDeniedException",
			errMsg:  "AccessDeniedException: Access denied",
			wantErr: secrets.ErrAccessDenied,
		},
		{
			name:    "InvalidParameterException",
			errMsg:  "InvalidParameterException: Invalid parameter",
			wantErr: secrets.ErrInvalidPath,
		},
		{
			name:    "InvalidRequestException",
			errMsg:  "InvalidRequestException: Invalid request",
			wantErr: secrets.ErrInvalidPath,
		},
		{
			name:    "DecryptionFailure",
			errMsg:  "DecryptionFailure: Failed to decrypt",
			wantErr: secrets.ErrCacheDecryptionFailed,
		},
		{
			name:    "InternalServiceError",
			errMsg:  "InternalServiceError: Service unavailable",
			wantErr: secrets.ErrBackendUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := translateError(customError{msg: tt.errMsg})
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("translateError() = %v, want %v", err, tt.wantErr)
			}
		})
	}

	t.Run("nil error", func(t *testing.T) {
		if err := translateError(nil); err != nil {
			t.Errorf("translateError(nil) = %v, want nil", err)
		}
	})

	t.Run("unknown error", func(t *testing.T) {
		originalErr := customError{msg: "unknown error"}
		if err := translateError(originalErr); err.Error() != "unknown error" {
			t.Errorf("expected original error to be returned")
		}
	})
}

type customError struct {
	msg string
}

func (e customError) Error() string {
	return e.msg
}

func TestBackendType(t *testing.T) {
	cfg := &BackendConfig{
		ClientConfig: &ClientConfig{
			Region:     "us-east-1",
			AuthMethod: AuthMethodDefault,
		},
	}

	// We can't create a real backend without AWS credentials,
	// but we can test the type methods
	t.Run("backend type is AWS", func(t *testing.T) {
		// Create a minimal backend for type testing
		b := &Backend{
			name:   "test-aws",
			config: cfg,
		}

		if b.Type() != secrets.BackendTypeAWS {
			t.Errorf("expected backend type AWS, got %s", b.Type())
		}

		if b.Name() != "test-aws" {
			t.Errorf("expected name 'test-aws', got %s", b.Name())
		}
	})
}

func TestResolveSecretName(t *testing.T) {
	tests := []struct {
		name       string
		pathPrefix string
		path       string
		want       string
	}{
		{
			name:       "with prefix",
			pathPrefix: "aws/",
			path:       "aws/myapp/secret",
			want:       "myapp/secret",
		},
		{
			name:       "without prefix",
			pathPrefix: "aws/",
			path:       "myapp/secret",
			want:       "myapp/secret",
		},
		{
			name:       "empty prefix",
			pathPrefix: "",
			path:       "myapp/secret",
			want:       "myapp/secret",
		},
		{
			name:       "leading and trailing slashes",
			pathPrefix: "aws/",
			path:       "/myapp/secret/",
			want:       "myapp/secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backend{
				config: &BackendConfig{
					PathPrefix: tt.pathPrefix,
				},
			}
			got := b.resolveSecretName(tt.path)
			if got != tt.want {
				t.Errorf("resolveSecretName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestValueToSecret(t *testing.T) {
	t.Run("JSON secret", func(t *testing.T) {
		b := &Backend{
			config: &BackendConfig{
				JSONKeys: true,
			},
		}

		value := &SecretValue{
			ARN:           "arn:aws:secretsmanager:us-east-1:123456789:secret:test",
			Name:          "test",
			VersionID:     "v1",
			VersionStages: []string{"AWSCURRENT"},
			SecretString:  `{"username":"admin","password":"secret123"}`,
			CreatedDate:   time.Now(),
		}

		secret := b.valueToSecret("aws/test", value)

		if secret.Path != "aws/test" {
			t.Errorf("expected path 'aws/test', got %s", secret.Path)
		}
		if secret.Backend != secrets.BackendTypeAWS {
			t.Errorf("expected backend AWS, got %s", secret.Backend)
		}
		if secret.Type != secrets.SecretTypeStatic {
			t.Errorf("expected type static, got %s", secret.Type)
		}
		if secret.Data["username"] != "admin" {
			t.Errorf("expected username 'admin', got %v", secret.Data["username"])
		}
		if secret.Data["password"] != "secret123" {
			t.Errorf("expected password 'secret123', got %v", secret.Data["password"])
		}
		if secret.Metadata["version_stage"] != "AWSCURRENT" {
			t.Errorf("expected version_stage 'AWSCURRENT', got %s", secret.Metadata["version_stage"])
		}
	})

	t.Run("non-JSON secret", func(t *testing.T) {
		b := &Backend{
			config: &BackendConfig{
				JSONKeys: true,
			},
		}

		value := &SecretValue{
			ARN:          "arn:aws:secretsmanager:us-east-1:123456789:secret:test",
			Name:         "test",
			SecretString: "plain-text-secret",
		}

		secret := b.valueToSecret("aws/test", value)

		if secret.Data["value"] != "plain-text-secret" {
			t.Errorf("expected value 'plain-text-secret', got %v", secret.Data["value"])
		}
	})

	t.Run("binary secret", func(t *testing.T) {
		b := &Backend{
			config: &BackendConfig{
				JSONKeys: false,
			},
		}

		value := &SecretValue{
			ARN:          "arn:aws:secretsmanager:us-east-1:123456789:secret:test",
			Name:         "test",
			SecretBinary: []byte{0x01, 0x02, 0x03},
		}

		secret := b.valueToSecret("aws/test", value)

		if string(secret.Data["value"].([]byte)) != "\x01\x02\x03" {
			t.Error("expected binary data to be preserved")
		}
	})

	t.Run("previous version stage", func(t *testing.T) {
		b := &Backend{
			config: &BackendConfig{
				JSONKeys: true,
			},
		}

		value := &SecretValue{
			ARN:           "arn:aws:secretsmanager:us-east-1:123456789:secret:test",
			VersionStages: []string{"AWSPREVIOUS"},
			SecretString:  `{"key":"value"}`,
		}

		secret := b.valueToSecret("aws/test", value)

		if secret.Metadata["version_stage"] != "AWSPREVIOUS" {
			t.Errorf("expected version_stage 'AWSPREVIOUS', got %s", secret.Metadata["version_stage"])
		}
	})

	t.Run("pending version stage", func(t *testing.T) {
		b := &Backend{
			config: &BackendConfig{
				JSONKeys: true,
			},
		}

		value := &SecretValue{
			ARN:           "arn:aws:secretsmanager:us-east-1:123456789:secret:test",
			VersionStages: []string{"AWSPENDING"},
			SecretString:  `{"key":"value"}`,
		}

		secret := b.valueToSecret("aws/test", value)

		if secret.Metadata["version_stage"] != "AWSPENDING" {
			t.Errorf("expected version_stage 'AWSPENDING', got %s", secret.Metadata["version_stage"])
		}
	})
}

func TestRotationStatus(t *testing.T) {
	status := &RotationStatus{
		Enabled:        true,
		LambdaARN:      "arn:aws:lambda:us-east-1:123456789:function:rotator",
		LastRotated:    time.Now().Add(-24 * time.Hour),
		NextRotation:   time.Now().Add(6 * 24 * time.Hour),
		AutoRotateDays: 7,
	}

	if !status.Enabled {
		t.Error("expected rotation enabled")
	}
	if status.AutoRotateDays != 7 {
		t.Errorf("expected auto rotate days 7, got %d", status.AutoRotateDays)
	}
}

func TestCrossAccountConfigValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("nil config", func(t *testing.T) {
		_, err := NewCrossAccountBackend(ctx, nil, nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("missing role ARN", func(t *testing.T) {
		_, err := NewCrossAccountBackend(ctx, &CrossAccountConfig{
			AccountID: "123456789",
		}, nil)
		if err == nil {
			t.Error("expected error for missing role ARN")
		}
	})
}

func TestGetSecretOptions(t *testing.T) {
	t.Run("WithVersionID", func(t *testing.T) {
		opts := &getSecretOptions{}
		WithVersionID("version-123")(opts)
		if opts.versionID != "version-123" {
			t.Errorf("expected version ID 'version-123', got %s", opts.versionID)
		}
	})

	t.Run("WithVersionStage", func(t *testing.T) {
		opts := &getSecretOptions{}
		WithVersionStage("AWSCURRENT")(opts)
		if opts.versionStage != "AWSCURRENT" {
			t.Errorf("expected version stage 'AWSCURRENT', got %s", opts.versionStage)
		}
	})
}

func TestListSecretsOptions(t *testing.T) {
	t.Run("WithNamePrefix", func(t *testing.T) {
		opts := &listSecretsOptions{}
		WithNamePrefix("myapp/")(opts)
		if len(opts.filters) != 1 {
			t.Errorf("expected 1 filter, got %d", len(opts.filters))
		}
	})

	t.Run("WithTagKey", func(t *testing.T) {
		opts := &listSecretsOptions{}
		WithTagKey("environment")(opts)
		if len(opts.filters) != 1 {
			t.Errorf("expected 1 filter, got %d", len(opts.filters))
		}
	})

	t.Run("WithTagValue", func(t *testing.T) {
		opts := &listSecretsOptions{}
		WithTagValue("environment", "production")(opts)
		if len(opts.filters) != 1 {
			t.Errorf("expected 1 filter, got %d", len(opts.filters))
		}
	})

	t.Run("WithDescription", func(t *testing.T) {
		opts := &listSecretsOptions{}
		WithDescription("database")(opts)
		if len(opts.filters) != 1 {
			t.Errorf("expected 1 filter, got %d", len(opts.filters))
		}
	})

	t.Run("WithMaxResults", func(t *testing.T) {
		opts := &listSecretsOptions{}
		WithMaxResults(100)(opts)
		if opts.maxResults != 100 {
			t.Errorf("expected max results 100, got %d", opts.maxResults)
		}
	})

	t.Run("multiple options", func(t *testing.T) {
		opts := &listSecretsOptions{}
		WithNamePrefix("myapp/")(opts)
		WithTagKey("environment")(opts)
		WithMaxResults(50)(opts)
		if len(opts.filters) != 2 {
			t.Errorf("expected 2 filters, got %d", len(opts.filters))
		}
		if opts.maxResults != 50 {
			t.Errorf("expected max results 50, got %d", opts.maxResults)
		}
	})
}

func TestRotateSecretOptions(t *testing.T) {
	t.Run("WithRotationLambda", func(t *testing.T) {
		opts := &rotateSecretOptions{}
		WithRotationLambda("arn:aws:lambda:us-east-1:123:function:rotate")(opts)
		if opts.rotationLambdaARN != "arn:aws:lambda:us-east-1:123:function:rotate" {
			t.Errorf("unexpected rotation lambda ARN: %s", opts.rotationLambdaARN)
		}
	})

	t.Run("WithRotationDays", func(t *testing.T) {
		opts := &rotateSecretOptions{}
		WithRotationDays(30)(opts)
		if opts.rotationRules == nil {
			t.Error("expected rotation rules to be set")
		}
	})

	t.Run("WithRotateImmediately", func(t *testing.T) {
		opts := &rotateSecretOptions{}
		WithRotateImmediately()(opts)
		if !opts.rotateImmediately {
			t.Error("expected rotate immediately to be true")
		}
	})
}

func TestBackendRenewRevokeLeaseNotSupported(t *testing.T) {
	b := &Backend{
		name:   "test",
		config: DefaultBackendConfig(),
	}

	ctx := context.Background()

	t.Run("RenewLease returns error", func(t *testing.T) {
		_, err := b.RenewLease(ctx, "lease-id", time.Hour)
		if !errors.Is(err, secrets.ErrLeaseNotFound) {
			t.Errorf("expected ErrLeaseNotFound, got %v", err)
		}
	})

	t.Run("RevokeLease returns error", func(t *testing.T) {
		err := b.RevokeLease(ctx, "lease-id")
		if !errors.Is(err, secrets.ErrLeaseNotFound) {
			t.Errorf("expected ErrLeaseNotFound, got %v", err)
		}
	})
}

func TestBackendClose(t *testing.T) {
	t.Run("close nil client", func(t *testing.T) {
		b := &Backend{
			name:   "test",
			config: DefaultBackendConfig(),
		}
		if err := b.Close(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestSecretMetadataJSON(t *testing.T) {
	metadata := &SecretMetadata{
		ARN:             "arn:aws:secretsmanager:us-east-1:123456789:secret:test",
		Name:            "test",
		Description:     "Test secret",
		RotationEnabled: true,
		Tags:            map[string]string{"env": "prod"},
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded SecretMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ARN != metadata.ARN {
		t.Errorf("expected ARN %s, got %s", metadata.ARN, decoded.ARN)
	}
	if decoded.RotationEnabled != metadata.RotationEnabled {
		t.Error("expected rotation enabled")
	}
}

func TestSecretListEntryFields(t *testing.T) {
	entry := &SecretListEntry{
		ARN:             "arn:aws:secretsmanager:us-east-1:123456789:secret:test",
		Name:            "test-secret",
		Description:     "A test secret",
		RotationEnabled: true,
		CreatedDate:     time.Now(),
		Tags:            map[string]string{"app": "myapp"},
	}

	if entry.Name != "test-secret" {
		t.Errorf("expected name 'test-secret', got %s", entry.Name)
	}
	if !entry.RotationEnabled {
		t.Error("expected rotation enabled")
	}
	if entry.Tags["app"] != "myapp" {
		t.Errorf("expected tag app=myapp, got %s", entry.Tags["app"])
	}
}

func TestRotationRulesFields(t *testing.T) {
	rules := &RotationRules{
		AutomaticallyAfterDays: 30,
		Duration:               "2h",
		ScheduleExpression:     "cron(0 12 * * ? *)",
	}

	if rules.AutomaticallyAfterDays != 30 {
		t.Errorf("expected 30 days, got %d", rules.AutomaticallyAfterDays)
	}
	if rules.ScheduleExpression != "cron(0 12 * * ? *)" {
		t.Errorf("unexpected schedule expression: %s", rules.ScheduleExpression)
	}
}

func TestReplicationStatusFields(t *testing.T) {
	status := ReplicationStatus{
		Region:        "eu-west-1",
		Status:        "InSync",
		StatusMessage: "Replication is in sync",
		KmsKeyID:      "arn:aws:kms:eu-west-1:123:key/abc",
	}

	if status.Region != "eu-west-1" {
		t.Errorf("expected region 'eu-west-1', got %s", status.Region)
	}
	if status.Status != "InSync" {
		t.Errorf("expected status 'InSync', got %s", status.Status)
	}
}
