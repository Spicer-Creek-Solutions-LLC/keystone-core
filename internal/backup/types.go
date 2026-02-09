// Package backup provides backup and restore capabilities for Keystone Core clusters
package backup

import (
	"context"
	"io"
	"os"
	"time"
)

// Type identifies what kind of data is being backed up
type Type string

// Type constants define the kinds of data that can be backed up.
const (
	TypeFull          Type = "full"          // Complete cluster backup
	TypeIncremental   Type = "incremental"   // Only changed data since last backup
	TypeDatabase      Type = "database"      // Database only (SQLite or PostgreSQL)
	TypeConfiguration Type = "configuration" // Configuration files only
	TypeJetStream     Type = "jetstream"     // NATS JetStream data only
	TypeEtcd          Type = "etcd"          // etcd data only
	TypeSecrets       Type = "secrets"       // Secrets only
)

// Status represents the current state of a backup operation
type Status string

// Status constants define the states of a backup operation.
const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusUploading Status = "uploading"
	StatusVerifying Status = "verifying"
)

// EncryptionType identifies the encryption method for backup data
type EncryptionType string

// EncryptionType constants define the encryption methods for backup data.
const (
	EncryptionTypeNone          EncryptionType = "none"
	EncryptionTypeAge           EncryptionType = "age"            // age encryption (default)
	EncryptionTypeAWSKMS        EncryptionType = "aws-kms"        // AWS Key Management Service
	EncryptionTypeGCPKMS        EncryptionType = "gcp-kms"        // Google Cloud KMS
	EncryptionTypeAzureKeyVault EncryptionType = "azure-keyvault" // Azure Key Vault
	EncryptionTypeVaultTransit  EncryptionType = "vault-transit"  // HashiCorp Vault Transit
)

// DestinationType identifies where backups are stored
type DestinationType string

// DestinationType constants define where backups are stored.
const (
	DestinationTypeLocal     DestinationType = "local"      // Local filesystem
	DestinationTypeS3        DestinationType = "s3"         // AWS S3 or compatible
	DestinationTypeGCS       DestinationType = "gcs"        // Google Cloud Storage
	DestinationTypeAzureBlob DestinationType = "azure-blob" // Azure Blob Storage
	DestinationTypeSFTP      DestinationType = "sftp"       // SFTP server
	DestinationTypeRclone    DestinationType = "rclone"     // Rclone remote (50+ backends)
)

// CompressionType identifies the compression method for backup artifacts
type CompressionType string

// CompressionType constants define the compression methods for backup artifacts.
const (
	CompressionTypeNone  CompressionType = "none"
	CompressionTypeGzip  CompressionType = "gzip"  // gzip compression (default)
	CompressionTypeBzip2 CompressionType = "bzip2" // bzip2 compression (higher ratio, slower)
	CompressionTypeXz    CompressionType = "xz"    // xz/LZMA compression (highest ratio, slowest)
	CompressionTypeZstd  CompressionType = "zstd"  // Zstandard compression (fast, good ratio)
	CompressionTypeLz4   CompressionType = "lz4"   // LZ4 compression (fastest, lower ratio)
)

// ComponentType identifies cluster components that can be backed up
type ComponentType string

// ComponentType constants define cluster components that can be backed up.
const (
	ComponentTypeServer   ComponentType = "server"
	ComponentTypeAgent    ComponentType = "agent"
	ComponentTypeNATS     ComponentType = "nats"
	ComponentTypeDatabase ComponentType = "database"
	ComponentTypeEtcd     ComponentType = "etcd"
	ComponentTypeCerts    ComponentType = "certs"
	ComponentTypeConfig   ComponentType = "config"
)

// Config holds configuration for the backup system
type Config struct {
	// Schedule in cron format (e.g., "0 2 * * *" for 2 AM daily)
	Schedule string `yaml:"schedule" json:"schedule"`

	// Type of backup to perform
	Type Type `yaml:"type" json:"type"`

	// Encryption settings
	Encryption EncryptionConfig `yaml:"encryption" json:"encryption"`

	// Destination settings
	Destination DestinationConfig `yaml:"destination" json:"destination"`

	// Retention settings
	Retention RetentionConfig `yaml:"retention" json:"retention"`

	// Components to include in backup
	Components []ComponentType `yaml:"components" json:"components"`

	// Compression specifies the compression type
	Compression CompressionType `yaml:"compression" json:"compression"`

	// Timeout for backup operation
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// MaxConcurrentExports limits parallel export operations
	MaxConcurrentExports int `yaml:"max_concurrent_exports" json:"max_concurrent_exports"`
}

// EncryptionConfig holds encryption settings
type EncryptionConfig struct {
	// Type of encryption to use
	Type EncryptionType `yaml:"type" json:"type"`

	// Age encryption settings
	// AgeRecipient is the age public key for encryption
	AgeRecipient string `yaml:"age_recipient" json:"age_recipient"`
	// AgeIdentityFile is the path to the age identity file for decryption
	AgeIdentityFile string `yaml:"age_identity_file" json:"age_identity_file"`
	// AgeIdentityPath is an alias for AgeIdentityFile
	AgeIdentityPath string `yaml:"age_identity_path" json:"age_identity_path"`

	// AWS KMS settings
	// AWSKMSKeyID is the AWS KMS key ARN or alias
	AWSKMSKeyID string `yaml:"aws_kms_key_id" json:"aws_kms_key_id"`
	// AWSKeyID is an alias for AWSKMSKeyID
	AWSKeyID string `yaml:"aws_key_id" json:"aws_key_id"`
	// AWSRegion is the AWS region for KMS
	AWSRegion string `yaml:"aws_region" json:"aws_region"`
	// AWSProfile is the AWS profile name
	AWSProfile string `yaml:"aws_profile" json:"aws_profile"`

	// GCP KMS settings
	// GCPKMSKeyName is the GCP KMS key resource name
	GCPKMSKeyName string `yaml:"gcp_kms_key_name" json:"gcp_kms_key_name"`
	// GCPProject is the GCP project ID
	GCPProject string `yaml:"gcp_project" json:"gcp_project"`
	// GCPLocation is the GCP location (region)
	GCPLocation string `yaml:"gcp_location" json:"gcp_location"`
	// GCPKeyRing is the GCP KMS key ring name
	GCPKeyRing string `yaml:"gcp_key_ring" json:"gcp_key_ring"`
	// GCPKey is the GCP KMS key name
	GCPKey string `yaml:"gcp_key" json:"gcp_key"`

	// Azure Key Vault settings
	// AzureKeyVaultURL is the Azure Key Vault URL
	AzureKeyVaultURL string `yaml:"azure_key_vault_url" json:"azure_key_vault_url"`
	// AzureVaultURL is an alias for AzureKeyVaultURL
	AzureVaultURL string `yaml:"azure_vault_url" json:"azure_vault_url"`
	// AzureKeyName is the Azure Key Vault key name
	AzureKeyName string `yaml:"azure_key_name" json:"azure_key_name"`

	// HashiCorp Vault Transit settings
	// VaultAddress is the HashiCorp Vault address
	VaultAddress string `yaml:"vault_address" json:"vault_address"`
	// VaultToken is the Vault authentication token
	VaultToken string `yaml:"vault_token" json:"vault_token"`
	// VaultTransitPath is the Vault transit engine path
	VaultTransitPath string `yaml:"vault_transit_path" json:"vault_transit_path"`
	// VaultMountPath is the Vault transit mount path
	VaultMountPath string `yaml:"vault_mount_path" json:"vault_mount_path"`
	// VaultKeyName is the Vault transit key name
	VaultKeyName string `yaml:"vault_key_name" json:"vault_key_name"`
	// VaultNamespace is the Vault namespace
	VaultNamespace string `yaml:"vault_namespace" json:"vault_namespace"`
}

// DestinationConfig holds backup destination settings
type DestinationConfig struct {
	// Type of destination
	Type DestinationType `yaml:"type" json:"type"`

	// Path is the local filesystem path or bucket/container prefix
	Path string `yaml:"path" json:"path"`

	// S3 configuration
	S3 *S3Config `yaml:"s3,omitempty" json:"s3,omitempty"`

	// GCS configuration
	GCS *GCSConfig `yaml:"gcs,omitempty" json:"gcs,omitempty"`

	// Azure configuration
	Azure *AzureConfig `yaml:"azure,omitempty" json:"azure,omitempty"`

	// SFTP configuration
	SFTP *SFTPConfig `yaml:"sftp,omitempty" json:"sftp,omitempty"`

	// Rclone configuration (supports 50+ cloud storage backends)
	Rclone *RcloneConfig `yaml:"rclone,omitempty" json:"rclone,omitempty"`
}

// S3Config holds S3-specific configuration
type S3Config struct {
	Bucket          string `yaml:"bucket" json:"bucket"`
	Region          string `yaml:"region" json:"region"`
	Endpoint        string `yaml:"endpoint" json:"endpoint"` // For S3-compatible storage
	AccessKeyID     string `yaml:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key" json:"secret_access_key"`
	UsePathStyle    bool   `yaml:"use_path_style" json:"use_path_style"` // For MinIO etc.
	StorageClass    string `yaml:"storage_class" json:"storage_class"`
	Prefix          string `yaml:"prefix" json:"prefix"`   // Object key prefix
	Profile         string `yaml:"profile" json:"profile"` // AWS profile name
}

// GCSConfig holds GCS-specific configuration
type GCSConfig struct {
	Bucket          string `yaml:"bucket" json:"bucket"`
	ProjectID       string `yaml:"project_id" json:"project_id"`
	CredentialsFile string `yaml:"credentials_file" json:"credentials_file"`
	StorageClass    string `yaml:"storage_class" json:"storage_class"`
	Prefix          string `yaml:"prefix" json:"prefix"` // Object key prefix
}

// AzureConfig holds Azure Blob-specific configuration
type AzureConfig struct {
	Container        string `yaml:"container" json:"container"`
	ContainerName    string `yaml:"container_name" json:"container_name"` // Alias for Container
	AccountName      string `yaml:"account_name" json:"account_name"`
	AccountKey       string `yaml:"account_key" json:"account_key"`
	ConnectionString string `yaml:"connection_string" json:"connection_string"`
	AccessTier       string `yaml:"access_tier" json:"access_tier"`
	Prefix           string `yaml:"prefix" json:"prefix"` // Blob name prefix
}

// SFTPConfig holds SFTP-specific configuration
type SFTPConfig struct {
	Host           string `yaml:"host" json:"host"`
	Port           int    `yaml:"port" json:"port"`
	User           string `yaml:"user" json:"user"`
	Password       string `yaml:"password" json:"password"`
	PrivateKeyFile string `yaml:"private_key_file" json:"private_key_file"`
	KeyPath        string `yaml:"key_path" json:"key_path"` // Alias for PrivateKeyFile
	HostKeyFile    string `yaml:"host_key_file" json:"host_key_file"`
	RemotePath     string `yaml:"remote_path" json:"remote_path"` // Remote directory path
}

// RcloneConfig holds rclone-specific configuration for streaming to 50+ cloud providers
type RcloneConfig struct {
	// Remote is the rclone remote name (configured via rclone config)
	Remote string `yaml:"remote" json:"remote"`
	// Path is the path within the remote (e.g., "bucket/backups" or "folder/subdir")
	Path string `yaml:"path" json:"path"`
	// ConfigFile is an optional path to the rclone config file (default: ~/.config/rclone/rclone.conf)
	ConfigFile string `yaml:"config_file" json:"config_file"`
	// BinaryPath is an optional path to the rclone binary (default: searches PATH)
	BinaryPath string `yaml:"binary_path" json:"binary_path"`
	// Flags are additional rclone flags to pass (e.g., ["--fast-list", "--transfers=4"])
	Flags []string `yaml:"flags" json:"flags"`
	// Streaming enables streaming mode (pipes data directly without temp files)
	Streaming bool `yaml:"streaming" json:"streaming"`
}

// CompressionConfig holds compression settings with more control
type CompressionConfig struct {
	// Type is the compression algorithm to use
	Type CompressionType `yaml:"type" json:"type"`
	// Level is the compression level (algorithm-specific, 0=default)
	Level int `yaml:"level" json:"level"`
	// Threads is the number of threads for parallel compression (0=auto, zstd/xz only)
	Threads int `yaml:"threads" json:"threads"`
}

// RetentionConfig holds backup retention settings
type RetentionConfig struct {
	// MaxBackups is the maximum number of backups to keep (0 = unlimited)
	MaxBackups int `yaml:"max_backups" json:"max_backups"`

	// MaxAge is the maximum age of backups to keep
	MaxAge time.Duration `yaml:"max_age" json:"max_age"`

	// KeepDaily is the number of daily backups to keep
	KeepDaily int `yaml:"keep_daily" json:"keep_daily"`

	// KeepWeekly is the number of weekly backups to keep
	KeepWeekly int `yaml:"keep_weekly" json:"keep_weekly"`

	// KeepMonthly is the number of monthly backups to keep
	KeepMonthly int `yaml:"keep_monthly" json:"keep_monthly"`

	// KeepYearly is the number of yearly backups to keep
	KeepYearly int `yaml:"keep_yearly" json:"keep_yearly"`
}

// Info represents metadata about a backup
type Info struct {
	// ID is the unique identifier for this backup
	ID string `json:"id"`

	// Name is a human-readable name for the backup
	Name string `json:"name"`

	// Type of backup
	Type Type `json:"type"`

	// Status of the backup
	Status Status `json:"status"`

	// StartTime when the backup started
	StartTime time.Time `json:"start_time"`

	// EndTime when the backup completed (if completed)
	EndTime time.Time `json:"end_time,omitempty"`

	// Duration of the backup operation
	Duration time.Duration `json:"duration,omitempty"`

	// Size in bytes
	Size int64 `json:"size"`

	// Checksum of the backup artifact (SHA-256)
	Checksum string `json:"checksum"`

	// Encrypted indicates if the backup is encrypted
	Encrypted bool `json:"encrypted"`

	// EncryptionType used for this backup
	EncryptionType EncryptionType `json:"encryption_type,omitempty"`

	// Components included in the backup
	Components []ComponentBackupInfo `json:"components"`

	// ClusterID of the source cluster
	ClusterID string `json:"cluster_id"`

	// ClusterName of the source cluster
	ClusterName string `json:"cluster_name"`

	// Version of Keystone Core that created the backup
	Version string `json:"version"`

	// Destination where the backup is stored
	Destination string `json:"destination"`

	// Error message if the backup failed
	Error string `json:"error,omitempty"`

	// Metadata contains additional backup metadata
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ComponentBackupInfo contains backup information for a specific component
type ComponentBackupInfo struct {
	// Type of component
	Type ComponentType `json:"type"`

	// Status of this component's backup
	Status Status `json:"status"`

	// Size in bytes
	Size int64 `json:"size"`

	// Checksum of the component data
	Checksum string `json:"checksum"`

	// Duration to backup this component
	Duration time.Duration `json:"duration"`

	// Error message if this component failed
	Error string `json:"error,omitempty"`

	// ItemCount is the number of items backed up (rows, files, etc.)
	ItemCount int64 `json:"item_count,omitempty"`
}

// Manifest is the manifest file included in each backup artifact
type Manifest struct {
	// Version of the manifest format
	ManifestVersion string `json:"manifest_version"`

	// Backup information
	Backup Info `json:"backup"`

	// Files lists all files in the backup with their checksums
	Files []ManifestFile `json:"files"`

	// Schema versions for each component
	SchemaVersions map[ComponentType]string `json:"schema_versions"`

	// CreatedAt when this manifest was created
	CreatedAt time.Time `json:"created_at"`
}

// ManifestFile represents a file in the backup manifest
type ManifestFile struct {
	// Path relative to the backup root
	Path string `json:"path"`

	// Size in bytes
	Size int64 `json:"size"`

	// Mode is the file mode
	Mode os.FileMode `json:"mode"`

	// ModTime is the file modification time
	ModTime time.Time `json:"mod_time"`

	// Checksum (SHA-256)
	Checksum string `json:"checksum"`

	// Component this file belongs to
	Component ComponentType `json:"component,omitempty"`

	// Encrypted indicates if this file is encrypted
	Encrypted bool `json:"encrypted,omitempty"`
}

// Progress tracks the progress of a backup operation
type Progress struct {
	// Phase of the backup
	Phase string `json:"phase"`

	// Component currently being backed up
	CurrentComponent ComponentType `json:"current_component,omitempty"`

	// TotalComponents in the backup
	TotalComponents int `json:"total_components"`

	// CompletedComponents so far
	CompletedComponents int `json:"completed_components"`

	// BytesProcessed so far
	BytesProcessed int64 `json:"bytes_processed"`

	// TotalBytes to process (if known)
	TotalBytes int64 `json:"total_bytes,omitempty"`

	// PercentComplete (0-100)
	PercentComplete int `json:"percent_complete"`

	// Message describes current activity
	Message string `json:"message"`

	// StartTime of the backup
	StartTime time.Time `json:"start_time"`

	// EstimatedCompletion time
	EstimatedCompletion time.Time `json:"estimated_completion,omitempty"`
}

// RestoreConfig holds configuration for restore operations
type RestoreConfig struct {
	// Source is the backup artifact to restore from
	Source string `yaml:"source" json:"source"`

	// Decryption settings (if backup is encrypted)
	Decryption EncryptionConfig `yaml:"decryption" json:"decryption"`

	// Components to restore (empty = all)
	Components []ComponentType `yaml:"components" json:"components"`

	// SkipVerification skips backup verification before restore
	SkipVerification bool `yaml:"skip_verification" json:"skip_verification"`

	// VerifyIntegrity verifies checksums during restore
	VerifyIntegrity bool `yaml:"verify_integrity" json:"verify_integrity"`

	// StopServices stops services before restore
	StopServices bool `yaml:"stop_services" json:"stop_services"`

	// RestartServices restarts services after restore
	RestartServices bool `yaml:"restart_services" json:"restart_services"`

	// Force allows restore even if cluster is running
	Force bool `yaml:"force" json:"force"`

	// DryRun validates without actually restoring
	DryRun bool `yaml:"dry_run" json:"dry_run"`

	// Timeout for restore operation
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// RestoreStatus represents the status of a restore operation
type RestoreStatus string

// RestoreStatus constants define the states of a restore operation.
const (
	RestoreStatusPending   RestoreStatus = "pending"
	RestoreStatusRunning   RestoreStatus = "running"
	RestoreStatusVerifying RestoreStatus = "verifying"
	RestoreStatusCompleted RestoreStatus = "completed"
	RestoreStatusFailed    RestoreStatus = "failed"
	RestoreStatusCancelled RestoreStatus = "cancelled"
)

// RestoreInfo represents metadata about a restore operation
type RestoreInfo struct {
	// ID of the restore operation
	ID string `json:"id"`

	// BackupID being restored
	BackupID string `json:"backup_id"`

	// BackupName is the name of the backup being restored
	BackupName string `json:"backup_name"`

	// Status of the restore
	Status RestoreStatus `json:"status"`

	// StartTime of the restore
	StartTime time.Time `json:"start_time"`

	// EndTime of the restore
	EndTime time.Time `json:"end_time,omitempty"`

	// Duration of the restore
	Duration time.Duration `json:"duration,omitempty"`

	// Components restored
	Components []ComponentRestoreInfo `json:"components"`

	// Error message if restore failed
	Error string `json:"error,omitempty"`
}

// ComponentRestoreInfo contains restore information for a specific component
type ComponentRestoreInfo struct {
	// Type of component
	Type ComponentType `json:"type"`

	// Status of this component's restore
	Status RestoreStatus `json:"status"`

	// Size of data restored
	Size int64 `json:"size"`

	// Duration to restore this component
	Duration time.Duration `json:"duration"`

	// ItemCount is the number of items restored
	ItemCount int64 `json:"item_count,omitempty"`

	// Error message if this component failed
	Error string `json:"error,omitempty"`
}

// RestoreProgressCallback is called during restore progress
type RestoreProgressCallback func(progress *RestoreProgress)

// RestoreProgress holds progress information for a restore operation
type RestoreProgress struct {
	Phase               string        `json:"phase"`
	CurrentComponent    ComponentType `json:"current_component,omitempty"`
	TotalComponents     int           `json:"total_components"`
	CompletedComponents int           `json:"completed_components"`
	BytesRestored       int64         `json:"bytes_restored"`
	PercentComplete     int           `json:"percent_complete"`
	Message             string        `json:"message"`
}

// ProgressCallback is called during backup to report progress
type ProgressCallback func(*Progress)

// Exporter exports data from a specific component
type Exporter interface {
	// Name returns the exporter name
	Name() string

	// Component returns the component type this exporter handles
	Component() ComponentType

	// Export exports data to the given writer
	Export(ctx context.Context, w io.Writer) error

	// EstimateSize estimates the export size in bytes
	EstimateSize(ctx context.Context) (int64, error)
}

// Importer imports data into a specific component
type Importer interface {
	// Name returns the importer name
	Name() string

	// Component returns the component type this importer handles
	Component() ComponentType

	// Import imports data from the given reader
	Import(ctx context.Context, r io.Reader) error

	// Verify verifies the imported data
	Verify(ctx context.Context) error
}

// Encryptor handles encryption of backup data
type Encryptor interface {
	// Type returns the encryption type
	Type() EncryptionType

	// Encrypt encrypts data from src to dst
	Encrypt(ctx context.Context, src io.Reader, dst io.Writer) error

	// Decrypt decrypts data from src to dst
	Decrypt(ctx context.Context, src io.Reader, dst io.Writer) error

	// EncryptFile encrypts a file
	EncryptFile(ctx context.Context, srcPath, dstPath string) error

	// DecryptFile decrypts a file
	DecryptFile(ctx context.Context, srcPath, dstPath string) error
}

// Destination handles uploading and downloading backup artifacts
type Destination interface {
	// Type returns the destination type
	Type() DestinationType

	// Upload uploads a backup artifact
	Upload(ctx context.Context, artifact string, reader io.Reader, size int64) error

	// Download downloads a backup artifact
	Download(ctx context.Context, artifact string, writer io.Writer) error

	// List lists available backups
	List(ctx context.Context) ([]Info, error)

	// Delete deletes a backup artifact
	Delete(ctx context.Context, artifact string) error

	// Exists checks if a backup artifact exists
	Exists(ctx context.Context, artifact string) (bool, error)
}

// Logger interface for backup operations
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// DefaultBackupConfig returns a sensible default configuration
func DefaultBackupConfig() *Config {
	return &Config{
		Type:                 TypeFull,
		Compression:          CompressionTypeGzip,
		Timeout:              30 * time.Minute,
		MaxConcurrentExports: 4,
		Encryption: EncryptionConfig{
			Type: EncryptionTypeNone,
		},
		Destination: DestinationConfig{
			Type: DestinationTypeLocal,
			Path: "/var/lib/keystone-core/backups",
		},
		Retention: RetentionConfig{
			MaxBackups:  10,
			MaxAge:      30 * 24 * time.Hour, // 30 days
			KeepDaily:   7,
			KeepWeekly:  4,
			KeepMonthly: 3,
		},
		Components: []ComponentType{
			ComponentTypeDatabase,
			ComponentTypeConfig,
			ComponentTypeCerts,
			ComponentTypeEtcd,
		},
	}
}

// ManifestVersion is the current version of the backup manifest format
const ManifestVersion = "1.0"
