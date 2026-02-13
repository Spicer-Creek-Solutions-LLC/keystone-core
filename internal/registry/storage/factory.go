package storage

import "fmt"

// BackendConfig holds configuration for creating storage backends.
type BackendConfig struct {
	// DataDir is the root directory for filesystem storage.
	DataDir string

	// S3 configuration
	S3Bucket    string
	S3Region    string
	S3Endpoint  string
	S3Prefix    string
	S3PathStyle bool

	// GCS configuration
	GCSBucket          string
	GCSProject         string
	GCSCredentialsFile string
	GCSPrefix          string

	// Azure configuration
	AzureContainer        string
	AzureAccountName      string
	AzureConnectionString string
	AzurePrefix           string

	// NATS configuration
	NATSUrl         string
	NATSBucket      string
	NATSCredentials string
	NATSPrefix      string
}

// NewBackend creates a storage backend from the given type and configuration.
func NewBackend(backendType string, cfg *BackendConfig) (Backend, error) {
	switch backendType {
	case "filesystem", "fs", "":
		return NewFilesystemBackend(cfg.DataDir)
	case "s3":
		return newS3Backend(cfg)
	case "gcs":
		return newGCSBackend(cfg)
	case "azure":
		return newAzureBackend(cfg)
	case "nats":
		return newNATSBackend(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage backend: %s", backendType)
	}
}
