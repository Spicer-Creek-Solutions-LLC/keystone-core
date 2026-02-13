package storage

import (
	"fmt"

	"github.com/shawnbutts/keystone-core/internal/files/backend"
)

// newS3Backend creates an S3-backed registry storage via the BackendAdapter.
func newS3Backend(cfg *BackendConfig) (Backend, error) {
	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("s3 storage: --s3-bucket is required")
	}
	if cfg.S3Region == "" && cfg.S3Endpoint == "" {
		return nil, fmt.Errorf("s3 storage: --s3-region or --s3-endpoint is required")
	}

	s3Config := &backend.S3Config{
		Config: backend.Config{
			Name: "registry-s3",
			Type: backend.TypeS3,
		},
		Bucket:       cfg.S3Bucket,
		Region:       cfg.S3Region,
		Endpoint:     cfg.S3Endpoint,
		Prefix:       cfg.S3Prefix,
		UsePathStyle: cfg.S3PathStyle,
	}

	s3Backend, err := backend.NewS3Backend(s3Config)
	if err != nil {
		return nil, fmt.Errorf("create s3 backend: %w", err)
	}

	// NOTE: The S3 client must be wired via s3Backend.SetClient() before use.
	// Production deployments should create an AWS SDK client and call SetClient.
	return NewBackendAdapter(s3Backend), nil
}

// newGCSBackend creates a GCS-backed registry storage via the BackendAdapter.
func newGCSBackend(cfg *BackendConfig) (Backend, error) {
	if cfg.GCSBucket == "" {
		return nil, fmt.Errorf("gcs storage: --gcs-bucket is required")
	}

	gcsConfig := &backend.GCSConfig{
		Config: backend.Config{
			Name: "registry-gcs",
			Type: backend.TypeGCS,
		},
		Bucket:          cfg.GCSBucket,
		Project:         cfg.GCSProject,
		CredentialsFile: cfg.GCSCredentialsFile,
		Prefix:          cfg.GCSPrefix,
	}

	gcsBackend, err := backend.NewGCSBackend(gcsConfig)
	if err != nil {
		return nil, fmt.Errorf("create gcs backend: %w", err)
	}

	return NewBackendAdapter(gcsBackend), nil
}

// newAzureBackend creates an Azure-backed registry storage via the BackendAdapter.
func newAzureBackend(cfg *BackendConfig) (Backend, error) {
	if cfg.AzureContainer == "" {
		return nil, fmt.Errorf("azure storage: --azure-container is required")
	}
	if cfg.AzureAccountName == "" && cfg.AzureConnectionString == "" {
		return nil, fmt.Errorf("azure storage: --azure-account-name or --azure-connection-string is required")
	}

	azureConfig := &backend.AzureConfig{
		Config: backend.Config{
			Name: "registry-azure",
			Type: backend.TypeAzure,
		},
		Container:        cfg.AzureContainer,
		AccountName:      cfg.AzureAccountName,
		ConnectionString: cfg.AzureConnectionString,
		Prefix:           cfg.AzurePrefix,
	}

	azureBackend, err := backend.NewAzureBackend(azureConfig)
	if err != nil {
		return nil, fmt.Errorf("create azure backend: %w", err)
	}

	return NewBackendAdapter(azureBackend), nil
}

// newNATSBackend creates a NATS-backed registry storage via the BackendAdapter.
func newNATSBackend(cfg *BackendConfig) (Backend, error) {
	if cfg.NATSBucket == "" {
		return nil, fmt.Errorf("nats storage: --nats-bucket is required")
	}
	if cfg.NATSUrl == "" {
		return nil, fmt.Errorf("nats storage: --nats-url is required")
	}

	natsConfig := &backend.NATSConfig{
		Config: backend.Config{
			Name: "registry-nats",
			Type: backend.TypeNATSObject,
		},
		BucketName:  cfg.NATSBucket,
		URL:         cfg.NATSUrl,
		Credentials: cfg.NATSCredentials,
		Prefix:      cfg.NATSPrefix,
	}

	natsBackend, err := backend.NewNATSBackend(natsConfig)
	if err != nil {
		return nil, fmt.Errorf("create nats backend: %w", err)
	}

	return NewBackendAdapter(natsBackend), nil
}
