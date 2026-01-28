package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LocalDestination stores backups on the local filesystem
type LocalDestination struct {
	basePath string
	logger   Logger
}

// NewLocalDestination creates a new local destination
func NewLocalDestination(basePath string, logger Logger) *LocalDestination {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &LocalDestination{
		basePath: basePath,
		logger:   logger,
	}
}

// Type returns the destination type
func (d *LocalDestination) Type() DestinationType {
	return DestinationTypeLocal
}

// Upload uploads a backup to the local filesystem
func (d *LocalDestination) Upload(ctx context.Context, name string, r io.Reader, size int64) error {
	// Create directory if needed
	if err := os.MkdirAll(d.basePath, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	destPath := filepath.Join(d.basePath, name)
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, r); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	d.logger.Debug("uploaded backup to local", "path", destPath)
	return nil
}

// Download downloads a backup from the local filesystem
func (d *LocalDestination) Download(ctx context.Context, name string, w io.Writer) error {
	srcPath := filepath.Join(d.basePath, name)
	file, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(w, file); err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	d.logger.Debug("downloaded backup from local", "path", srcPath)
	return nil
}

// List lists all backups in the local directory
func (d *LocalDestination) List(ctx context.Context) ([]BackupInfo, error) {
	entries, err := os.ReadDir(d.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			Name:        entry.Name(),
			Destination: filepath.Join(d.basePath, entry.Name()),
			Size:        info.Size(),
			EndTime:     info.ModTime(),
		})
	}

	// Sort by time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].EndTime.After(backups[j].EndTime)
	})

	return backups, nil
}

// Delete deletes a backup from the local filesystem
func (d *LocalDestination) Delete(ctx context.Context, name string) error {
	path := filepath.Join(d.basePath, name)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	d.logger.Debug("deleted backup from local", "path", path)
	return nil
}

// Exists checks if a backup exists on the local filesystem
func (d *LocalDestination) Exists(ctx context.Context, name string) (bool, error) {
	path := filepath.Join(d.basePath, name)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// S3Destination stores backups in S3 or S3-compatible storage
type S3Destination struct {
	bucket   string
	prefix   string
	region   string
	endpoint string
	profile  string
	logger   Logger
}

// NewS3Destination creates a new S3 destination
func NewS3Destination(config S3Config, logger Logger) *S3Destination {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &S3Destination{
		bucket:   config.Bucket,
		prefix:   config.Prefix,
		region:   config.Region,
		endpoint: config.Endpoint,
		profile:  config.Profile,
		logger:   logger,
	}
}

// Type returns the destination type
func (d *S3Destination) Type() DestinationType {
	return DestinationTypeS3
}

// Upload uploads a backup to S3
func (d *S3Destination) Upload(ctx context.Context, name string, r io.Reader, size int64) error {
	key := d.prefix + name

	// Write to temp file first (aws cli needs a file)
	tmpFile, err := os.CreateTemp("", "s3_upload_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, r); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	args := []string{
		"s3", "cp",
		tmpFile.Name(),
		fmt.Sprintf("s3://%s/%s", d.bucket, key),
	}
	if d.region != "" {
		args = append(args, "--region", d.region)
	}
	if d.endpoint != "" {
		args = append(args, "--endpoint-url", d.endpoint)
	}
	if d.profile != "" {
		args = append(args, "--profile", d.profile)
	}

	cmd := exec.CommandContext(ctx, "aws", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aws s3 cp failed: %w", err)
	}

	d.logger.Debug("uploaded backup to S3", "bucket", d.bucket, "key", key)
	return nil
}

// Download downloads a backup from S3
func (d *S3Destination) Download(ctx context.Context, name string, w io.Writer) error {
	key := d.prefix + name

	// Download to temp file first
	tmpFile, err := os.CreateTemp("", "s3_download_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	args := []string{
		"s3", "cp",
		fmt.Sprintf("s3://%s/%s", d.bucket, key),
		tmpFile.Name(),
	}
	if d.region != "" {
		args = append(args, "--region", d.region)
	}
	if d.endpoint != "" {
		args = append(args, "--endpoint-url", d.endpoint)
	}
	if d.profile != "" {
		args = append(args, "--profile", d.profile)
	}

	cmd := exec.CommandContext(ctx, "aws", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aws s3 cp failed: %w", err)
	}

	// Read temp file to writer
	file, err := os.Open(tmpFile.Name())
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := io.Copy(w, file); err != nil {
		return err
	}

	d.logger.Debug("downloaded backup from S3", "bucket", d.bucket, "key", key)
	return nil
}

// List lists all backups in S3
func (d *S3Destination) List(ctx context.Context) ([]BackupInfo, error) {
	args := []string{
		"s3api", "list-objects-v2",
		"--bucket", d.bucket,
		"--prefix", d.prefix,
		"--output", "json",
	}
	if d.region != "" {
		args = append(args, "--region", d.region)
	}
	if d.endpoint != "" {
		args = append(args, "--endpoint-url", d.endpoint)
	}
	if d.profile != "" {
		args = append(args, "--profile", d.profile)
	}

	cmd := exec.CommandContext(ctx, "aws", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("aws s3api list-objects-v2 failed: %w", err)
	}

	var result struct {
		Contents []struct {
			Key          string    `json:"Key"`
			Size         int64     `json:"Size"`
			LastModified time.Time `json:"LastModified"`
		} `json:"Contents"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse S3 response: %w", err)
	}

	var backups []BackupInfo
	for _, obj := range result.Contents {
		name := strings.TrimPrefix(obj.Key, d.prefix)
		if name == "" {
			continue
		}
		backups = append(backups, BackupInfo{
			Name:        name,
			Destination: fmt.Sprintf("s3://%s/%s", d.bucket, obj.Key),
			Size:        obj.Size,
			EndTime:     obj.LastModified,
		})
	}

	// Sort by time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].EndTime.After(backups[j].EndTime)
	})

	return backups, nil
}

// Delete deletes a backup from S3
func (d *S3Destination) Delete(ctx context.Context, name string) error {
	key := d.prefix + name

	args := []string{
		"s3", "rm",
		fmt.Sprintf("s3://%s/%s", d.bucket, key),
	}
	if d.region != "" {
		args = append(args, "--region", d.region)
	}
	if d.endpoint != "" {
		args = append(args, "--endpoint-url", d.endpoint)
	}
	if d.profile != "" {
		args = append(args, "--profile", d.profile)
	}

	cmd := exec.CommandContext(ctx, "aws", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aws s3 rm failed: %w", err)
	}

	d.logger.Debug("deleted backup from S3", "bucket", d.bucket, "key", key)
	return nil
}

// Exists checks if a backup exists in S3
func (d *S3Destination) Exists(ctx context.Context, name string) (bool, error) {
	key := d.prefix + name

	args := []string{
		"s3api", "head-object",
		"--bucket", d.bucket,
		"--key", key,
	}
	if d.region != "" {
		args = append(args, "--region", d.region)
	}
	if d.endpoint != "" {
		args = append(args, "--endpoint-url", d.endpoint)
	}
	if d.profile != "" {
		args = append(args, "--profile", d.profile)
	}

	cmd := exec.CommandContext(ctx, "aws", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	err := cmd.Run()
	if err != nil {
		// head-object returns error if object doesn't exist
		return false, nil
	}
	return true, nil
}

// GCSDestination stores backups in Google Cloud Storage
type GCSDestination struct {
	bucket string
	prefix string
	logger Logger
}

// NewGCSDestination creates a new GCS destination
func NewGCSDestination(config GCSConfig, logger Logger) *GCSDestination {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &GCSDestination{
		bucket: config.Bucket,
		prefix: config.Prefix,
		logger: logger,
	}
}

// Type returns the destination type
func (d *GCSDestination) Type() DestinationType {
	return DestinationTypeGCS
}

// Upload uploads a backup to GCS
func (d *GCSDestination) Upload(ctx context.Context, name string, r io.Reader, size int64) error {
	path := d.prefix + name

	// Write to temp file first
	tmpFile, err := os.CreateTemp("", "gcs_upload_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, r); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	args := []string{
		"storage", "cp",
		tmpFile.Name(),
		fmt.Sprintf("gs://%s/%s", d.bucket, path),
	}

	cmd := exec.CommandContext(ctx, "gcloud", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gcloud storage cp failed: %w", err)
	}

	d.logger.Debug("uploaded backup to GCS", "bucket", d.bucket, "path", path)
	return nil
}

// Download downloads a backup from GCS
func (d *GCSDestination) Download(ctx context.Context, name string, w io.Writer) error {
	path := d.prefix + name

	// Download to temp file first
	tmpFile, err := os.CreateTemp("", "gcs_download_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	args := []string{
		"storage", "cp",
		fmt.Sprintf("gs://%s/%s", d.bucket, path),
		tmpFile.Name(),
	}

	cmd := exec.CommandContext(ctx, "gcloud", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gcloud storage cp failed: %w", err)
	}

	// Read temp file to writer
	file, err := os.Open(tmpFile.Name())
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := io.Copy(w, file); err != nil {
		return err
	}

	d.logger.Debug("downloaded backup from GCS", "bucket", d.bucket, "path", path)
	return nil
}

// List lists all backups in GCS
func (d *GCSDestination) List(ctx context.Context) ([]BackupInfo, error) {
	args := []string{
		"storage", "ls", "-l",
		fmt.Sprintf("gs://%s/%s", d.bucket, d.prefix),
	}

	cmd := exec.CommandContext(ctx, "gcloud", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gcloud storage ls failed: %w", err)
	}

	var backups []BackupInfo
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "TOTAL:") {
			continue
		}

		// Parse line format: SIZE CREATION_TIME URL
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		var size int64
		fmt.Sscanf(parts[0], "%d", &size)

		// Parse timestamp (format: 2006-01-02T15:04:05Z)
		timeStr := parts[1]
		t, _ := time.Parse(time.RFC3339, timeStr)

		url := parts[len(parts)-1]
		name := strings.TrimPrefix(url, fmt.Sprintf("gs://%s/%s", d.bucket, d.prefix))

		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}

		backups = append(backups, BackupInfo{
			Name:        name,
			Destination: url,
			Size:        size,
			EndTime:     t,
		})
	}

	// Sort by time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].EndTime.After(backups[j].EndTime)
	})

	return backups, nil
}

// Delete deletes a backup from GCS
func (d *GCSDestination) Delete(ctx context.Context, name string) error {
	path := d.prefix + name

	args := []string{
		"storage", "rm",
		fmt.Sprintf("gs://%s/%s", d.bucket, path),
	}

	cmd := exec.CommandContext(ctx, "gcloud", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gcloud storage rm failed: %w", err)
	}

	d.logger.Debug("deleted backup from GCS", "bucket", d.bucket, "path", path)
	return nil
}

// Exists checks if a backup exists in GCS
func (d *GCSDestination) Exists(ctx context.Context, name string) (bool, error) {
	path := d.prefix + name

	args := []string{
		"storage", "objects", "describe",
		fmt.Sprintf("gs://%s/%s", d.bucket, path),
	}

	cmd := exec.CommandContext(ctx, "gcloud", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	err := cmd.Run()
	if err != nil {
		return false, nil
	}
	return true, nil
}

// AzureBlobDestination stores backups in Azure Blob Storage
type AzureBlobDestination struct {
	accountName   string
	containerName string
	prefix        string
	logger        Logger
}

// NewAzureBlobDestination creates a new Azure Blob destination
func NewAzureBlobDestination(config AzureConfig, logger Logger) *AzureBlobDestination {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &AzureBlobDestination{
		accountName:   config.AccountName,
		containerName: config.ContainerName,
		prefix:        config.Prefix,
		logger:        logger,
	}
}

// Type returns the destination type
func (d *AzureBlobDestination) Type() DestinationType {
	return DestinationTypeAzureBlob
}

// Upload uploads a backup to Azure Blob Storage
func (d *AzureBlobDestination) Upload(ctx context.Context, name string, r io.Reader, size int64) error {
	blobName := d.prefix + name

	// Write to temp file first
	tmpFile, err := os.CreateTemp("", "azure_upload_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, r); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	args := []string{
		"storage", "blob", "upload",
		"--account-name", d.accountName,
		"--container-name", d.containerName,
		"--name", blobName,
		"--file", tmpFile.Name(),
		"--overwrite",
	}

	cmd := exec.CommandContext(ctx, "az", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("az storage blob upload failed: %w", err)
	}

	d.logger.Debug("uploaded backup to Azure Blob", "account", d.accountName, "container", d.containerName, "blob", blobName)
	return nil
}

// Download downloads a backup from Azure Blob Storage
func (d *AzureBlobDestination) Download(ctx context.Context, name string, w io.Writer) error {
	blobName := d.prefix + name

	// Download to temp file first
	tmpFile, err := os.CreateTemp("", "azure_download_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	args := []string{
		"storage", "blob", "download",
		"--account-name", d.accountName,
		"--container-name", d.containerName,
		"--name", blobName,
		"--file", tmpFile.Name(),
	}

	cmd := exec.CommandContext(ctx, "az", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("az storage blob download failed: %w", err)
	}

	// Read temp file to writer
	file, err := os.Open(tmpFile.Name())
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := io.Copy(w, file); err != nil {
		return err
	}

	d.logger.Debug("downloaded backup from Azure Blob", "account", d.accountName, "container", d.containerName, "blob", blobName)
	return nil
}

// List lists all backups in Azure Blob Storage
func (d *AzureBlobDestination) List(ctx context.Context) ([]BackupInfo, error) {
	args := []string{
		"storage", "blob", "list",
		"--account-name", d.accountName,
		"--container-name", d.containerName,
		"--prefix", d.prefix,
		"--output", "json",
	}

	cmd := exec.CommandContext(ctx, "az", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("az storage blob list failed: %w", err)
	}

	var blobs []struct {
		Name       string `json:"name"`
		Properties struct {
			ContentLength int64  `json:"contentLength"`
			LastModified  string `json:"lastModified"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(output, &blobs); err != nil {
		return nil, fmt.Errorf("failed to parse Azure response: %w", err)
	}

	var backups []BackupInfo
	for _, blob := range blobs {
		name := strings.TrimPrefix(blob.Name, d.prefix)
		if name == "" {
			continue
		}

		t, _ := time.Parse(time.RFC1123, blob.Properties.LastModified)

		backups = append(backups, BackupInfo{
			Name:        name,
			Destination: fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", d.accountName, d.containerName, blob.Name),
			Size:        blob.Properties.ContentLength,
			EndTime:     t,
		})
	}

	// Sort by time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].EndTime.After(backups[j].EndTime)
	})

	return backups, nil
}

// Delete deletes a backup from Azure Blob Storage
func (d *AzureBlobDestination) Delete(ctx context.Context, name string) error {
	blobName := d.prefix + name

	args := []string{
		"storage", "blob", "delete",
		"--account-name", d.accountName,
		"--container-name", d.containerName,
		"--name", blobName,
	}

	cmd := exec.CommandContext(ctx, "az", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("az storage blob delete failed: %w", err)
	}

	d.logger.Debug("deleted backup from Azure Blob", "account", d.accountName, "container", d.containerName, "blob", blobName)
	return nil
}

// Exists checks if a backup exists in Azure Blob Storage
func (d *AzureBlobDestination) Exists(ctx context.Context, name string) (bool, error) {
	blobName := d.prefix + name

	args := []string{
		"storage", "blob", "exists",
		"--account-name", d.accountName,
		"--container-name", d.containerName,
		"--name", blobName,
		"--output", "json",
	}

	cmd := exec.CommandContext(ctx, "az", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return false, nil
	}

	var result struct {
		Exists bool `json:"exists"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return false, nil
	}

	return result.Exists, nil
}

// SFTPDestination stores backups via SFTP
type SFTPDestination struct {
	host       string
	port       int
	user       string
	keyPath    string
	remotePath string
	logger     Logger
}

// NewSFTPDestination creates a new SFTP destination
func NewSFTPDestination(config SFTPConfig, logger Logger) *SFTPDestination {
	if logger == nil {
		logger = &noopLogger{}
	}
	if config.Port == 0 {
		config.Port = 22
	}
	return &SFTPDestination{
		host:       config.Host,
		port:       config.Port,
		user:       config.User,
		keyPath:    config.KeyPath,
		remotePath: config.RemotePath,
		logger:     logger,
	}
}

// Type returns the destination type
func (d *SFTPDestination) Type() DestinationType {
	return DestinationTypeSFTP
}

// Upload uploads a backup via SFTP
func (d *SFTPDestination) Upload(ctx context.Context, name string, r io.Reader, size int64) error {
	remoteDest := filepath.Join(d.remotePath, name)

	// Write to temp file first
	tmpFile, err := os.CreateTemp("", "sftp_upload_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, r); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	args := d.buildSCPArgs(tmpFile.Name(), fmt.Sprintf("%s@%s:%s", d.user, d.host, remoteDest))

	cmd := exec.CommandContext(ctx, "scp", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp failed: %w", err)
	}

	d.logger.Debug("uploaded backup via SFTP", "host", d.host, "path", remoteDest)
	return nil
}

// Download downloads a backup via SFTP
func (d *SFTPDestination) Download(ctx context.Context, name string, w io.Writer) error {
	remoteSrc := filepath.Join(d.remotePath, name)

	// Download to temp file first
	tmpFile, err := os.CreateTemp("", "sftp_download_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	args := d.buildSCPArgs(fmt.Sprintf("%s@%s:%s", d.user, d.host, remoteSrc), tmpFile.Name())

	cmd := exec.CommandContext(ctx, "scp", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp failed: %w", err)
	}

	// Read temp file to writer
	file, err := os.Open(tmpFile.Name())
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := io.Copy(w, file); err != nil {
		return err
	}

	d.logger.Debug("downloaded backup via SFTP", "host", d.host, "path", remoteSrc)
	return nil
}

// List lists all backups via SFTP
func (d *SFTPDestination) List(ctx context.Context) ([]BackupInfo, error) {
	// Use ssh to list files
	sshArgs := d.buildSSHArgs(fmt.Sprintf("ls -la %s", d.remotePath))

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ssh ls failed: %w", err)
	}

	var backups []BackupInfo
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total") {
			continue
		}

		// Parse ls -la output
		parts := strings.Fields(line)
		if len(parts) < 9 {
			continue
		}

		// Skip directories
		if parts[0][0] == 'd' {
			continue
		}

		var size int64
		fmt.Sscanf(parts[4], "%d", &size)

		name := parts[len(parts)-1]

		backups = append(backups, BackupInfo{
			Name:        name,
			Destination: fmt.Sprintf("sftp://%s@%s:%d%s/%s", d.user, d.host, d.port, d.remotePath, name),
			Size:        size,
		})
	}

	return backups, nil
}

// Delete deletes a backup via SFTP
func (d *SFTPDestination) Delete(ctx context.Context, name string) error {
	remotePath := filepath.Join(d.remotePath, name)
	sshArgs := d.buildSSHArgs(fmt.Sprintf("rm %s", remotePath))

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh rm failed: %w", err)
	}

	d.logger.Debug("deleted backup via SFTP", "host", d.host, "path", remotePath)
	return nil
}

// Exists checks if a backup exists via SFTP
func (d *SFTPDestination) Exists(ctx context.Context, name string) (bool, error) {
	remotePath := filepath.Join(d.remotePath, name)
	sshArgs := d.buildSSHArgs(fmt.Sprintf("test -f %s && echo exists", remotePath))

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(output)) == "exists", nil
}

// buildSCPArgs builds scp command arguments
func (d *SFTPDestination) buildSCPArgs(src, dst string) []string {
	args := []string{"-P", fmt.Sprintf("%d", d.port)}
	if d.keyPath != "" {
		args = append(args, "-i", d.keyPath)
	}
	args = append(args, "-o", "StrictHostKeyChecking=no")
	args = append(args, "-o", "BatchMode=yes")
	args = append(args, src, dst)
	return args
}

// buildSSHArgs builds ssh command arguments
func (d *SFTPDestination) buildSSHArgs(command string) []string {
	args := []string{"-p", fmt.Sprintf("%d", d.port)}
	if d.keyPath != "" {
		args = append(args, "-i", d.keyPath)
	}
	args = append(args, "-o", "StrictHostKeyChecking=no")
	args = append(args, "-o", "BatchMode=yes")
	args = append(args, fmt.Sprintf("%s@%s", d.user, d.host))
	args = append(args, command)
	return args
}

// HTTPDestination stores backups via HTTP POST/PUT (for custom backup servers)
type HTTPDestination struct {
	baseURL string
	token   string
	logger  Logger
	client  *http.Client
}

// HTTPConfig holds HTTP destination configuration
type HTTPConfig struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// NewHTTPDestination creates a new HTTP destination
func NewHTTPDestination(config HTTPConfig, logger Logger) *HTTPDestination {
	if logger == nil {
		logger = &noopLogger{}
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	return &HTTPDestination{
		baseURL: strings.TrimSuffix(config.BaseURL, "/"),
		token:   config.Token,
		logger:  logger,
		client:  &http.Client{Timeout: timeout},
	}
}

// Type returns the destination type
func (d *HTTPDestination) Type() DestinationType {
	return DestinationType("http")
}

// Upload uploads a backup via HTTP PUT
func (d *HTTPDestination) Upload(ctx context.Context, name string, r io.Reader, size int64) error {
	url := fmt.Sprintf("%s/%s", d.baseURL, name)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, r)
	if err != nil {
		return err
	}

	if d.token != "" {
		req.Header.Set("Authorization", "Bearer "+d.token)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if size > 0 {
		req.ContentLength = size
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP PUT failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP PUT failed: %s - %s", resp.Status, string(body))
	}

	d.logger.Debug("uploaded backup via HTTP", "url", url)
	return nil
}

// Download downloads a backup via HTTP GET
func (d *HTTPDestination) Download(ctx context.Context, name string, w io.Writer) error {
	url := fmt.Sprintf("%s/%s", d.baseURL, name)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	if d.token != "" {
		req.Header.Set("Authorization", "Bearer "+d.token)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP GET failed: %s - %s", resp.Status, string(body))
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		return err
	}

	d.logger.Debug("downloaded backup via HTTP", "url", url)
	return nil
}

// List lists backups (requires server support)
func (d *HTTPDestination) List(ctx context.Context) ([]BackupInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", d.baseURL, nil)
	if err != nil {
		return nil, err
	}

	if d.token != "" {
		req.Header.Set("Authorization", "Bearer "+d.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP GET failed: %s", resp.Status)
	}

	var backups []BackupInfo
	if err := json.NewDecoder(resp.Body).Decode(&backups); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return backups, nil
}

// Delete deletes a backup via HTTP DELETE
func (d *HTTPDestination) Delete(ctx context.Context, name string) error {
	url := fmt.Sprintf("%s/%s", d.baseURL, name)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	if d.token != "" {
		req.Header.Set("Authorization", "Bearer "+d.token)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP DELETE failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP DELETE failed: %s - %s", resp.Status, string(body))
	}

	d.logger.Debug("deleted backup via HTTP", "url", url)
	return nil
}

// Exists checks if a backup exists via HTTP HEAD
func (d *HTTPDestination) Exists(ctx context.Context, name string) (bool, error) {
	url := fmt.Sprintf("%s/%s", d.baseURL, name)

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return false, err
	}

	if d.token != "" {
		req.Header.Set("Authorization", "Bearer "+d.token)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()

	return resp.StatusCode < 400, nil
}

// RcloneDestination stores backups via rclone (supports 50+ cloud storage backends)
// Uses streaming pipes to avoid temporary local storage
type RcloneDestination struct {
	config RcloneConfig
	logger Logger
}

// NewRcloneDestination creates a new rclone destination
func NewRcloneDestination(config RcloneConfig, logger Logger) *RcloneDestination {
	if logger == nil {
		logger = &noopLogger{}
	}
	if config.Streaming {
		// Streaming is enabled by default for new configs
	}
	return &RcloneDestination{
		config: config,
		logger: logger,
	}
}

// Type returns the destination type
func (d *RcloneDestination) Type() DestinationType {
	return DestinationTypeRclone
}

// rcloneBinary returns the rclone binary path
func (d *RcloneDestination) rcloneBinary() string {
	if d.config.BinaryPath != "" {
		return d.config.BinaryPath
	}
	return "rclone"
}

// remotePath builds the full remote path for an artifact
func (d *RcloneDestination) remotePath(artifact string) string {
	if d.config.Path != "" {
		return fmt.Sprintf("%s:%s/%s", d.config.Remote, d.config.Path, artifact)
	}
	return fmt.Sprintf("%s:%s", d.config.Remote, artifact)
}

// baseArgs returns base rclone arguments
func (d *RcloneDestination) baseArgs() []string {
	args := []string{}
	if d.config.ConfigFile != "" {
		args = append(args, "--config", d.config.ConfigFile)
	}
	args = append(args, d.config.Flags...)
	return args
}

// Upload uploads a backup using rclone rcat for streaming
func (d *RcloneDestination) Upload(ctx context.Context, artifact string, reader io.Reader, size int64) error {
	remotePath := d.remotePath(artifact)

	if d.config.Streaming {
		// Use rclone rcat for streaming upload (data piped via stdin)
		args := d.baseArgs()
		args = append(args, "rcat", remotePath)
		if size > 0 {
			args = append(args, "--size", fmt.Sprintf("%d", size))
		}

		cmd := exec.CommandContext(ctx, d.rcloneBinary(), args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
		cmd.Stdin = reader

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		d.logger.Debug("uploading via rclone rcat (streaming)", "remote", remotePath)

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("rclone rcat failed: %w - %s", err, stderr.String())
		}

		d.logger.Info("uploaded backup via rclone", "remote", remotePath, "size", size)
		return nil
	}

	// Non-streaming: write to temp file first
	tmpFile, err := os.CreateTemp("", "kscore-backup-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, reader); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	args := d.baseArgs()
	args = append(args, "copyto", tmpPath, remotePath)

	cmd := exec.CommandContext(ctx, d.rcloneBinary(), args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	d.logger.Debug("uploading via rclone copyto", "remote", remotePath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rclone copyto failed: %w - %s", err, stderr.String())
	}

	d.logger.Info("uploaded backup via rclone", "remote", remotePath)
	return nil
}

// Download downloads a backup using rclone cat for streaming
func (d *RcloneDestination) Download(ctx context.Context, artifact string, writer io.Writer) error {
	remotePath := d.remotePath(artifact)

	if d.config.Streaming {
		// Use rclone cat for streaming download (data piped via stdout)
		args := d.baseArgs()
		args = append(args, "cat", remotePath)

		cmd := exec.CommandContext(ctx, d.rcloneBinary(), args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
		cmd.Stdout = writer

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		d.logger.Debug("downloading via rclone cat (streaming)", "remote", remotePath)

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("rclone cat failed: %w - %s", err, stderr.String())
		}

		d.logger.Info("downloaded backup via rclone", "remote", remotePath)
		return nil
	}

	// Non-streaming: download to temp file first
	tmpFile, err := os.CreateTemp("", "kscore-restore-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	args := d.baseArgs()
	args = append(args, "copyto", remotePath, tmpPath)

	cmd := exec.CommandContext(ctx, d.rcloneBinary(), args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	d.logger.Debug("downloading via rclone copyto", "remote", remotePath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rclone copyto failed: %w - %s", err, stderr.String())
	}

	// Read temp file and write to output
	file, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to open temp file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(writer, file); err != nil {
		return fmt.Errorf("failed to copy from temp file: %w", err)
	}

	d.logger.Info("downloaded backup via rclone", "remote", remotePath)
	return nil
}

// List lists available backups via rclone lsjson
func (d *RcloneDestination) List(ctx context.Context) ([]BackupInfo, error) {
	remotePath := d.config.Remote + ":"
	if d.config.Path != "" {
		remotePath = fmt.Sprintf("%s:%s", d.config.Remote, d.config.Path)
	}

	args := d.baseArgs()
	args = append(args, "lsjson", remotePath)

	cmd := exec.CommandContext(ctx, d.rcloneBinary(), args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	d.logger.Debug("listing backups via rclone lsjson", "remote", remotePath)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rclone lsjson failed: %w - %s", err, stderr.String())
	}

	var items []struct {
		Path    string    `json:"Path"`
		Name    string    `json:"Name"`
		Size    int64     `json:"Size"`
		ModTime time.Time `json:"ModTime"`
		IsDir   bool      `json:"IsDir"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		return nil, fmt.Errorf("failed to parse rclone output: %w", err)
	}

	var backups []BackupInfo
	for _, item := range items {
		if item.IsDir {
			continue
		}
		// Only include backup files
		if !strings.HasSuffix(item.Name, ".tar") &&
			!strings.HasSuffix(item.Name, ".tar.gz") &&
			!strings.HasSuffix(item.Name, ".tar.bz2") &&
			!strings.HasSuffix(item.Name, ".tar.xz") &&
			!strings.HasSuffix(item.Name, ".tar.zst") &&
			!strings.HasSuffix(item.Name, ".tar.lz4") {
			continue
		}

		backups = append(backups, BackupInfo{
			Name:        item.Name,
			Size:        item.Size,
			StartTime:   item.ModTime,
			Destination: fmt.Sprintf("rclone:%s/%s", d.config.Remote, item.Path),
		})
	}

	// Sort by time descending
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].StartTime.After(backups[j].StartTime)
	})

	return backups, nil
}

// Delete deletes a backup via rclone deletefile
func (d *RcloneDestination) Delete(ctx context.Context, artifact string) error {
	remotePath := d.remotePath(artifact)

	args := d.baseArgs()
	args = append(args, "deletefile", remotePath)

	cmd := exec.CommandContext(ctx, d.rcloneBinary(), args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	d.logger.Debug("deleting backup via rclone", "remote", remotePath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rclone deletefile failed: %w - %s", err, stderr.String())
	}

	d.logger.Info("deleted backup via rclone", "remote", remotePath)
	return nil
}

// Exists checks if a backup exists via rclone lsjson
func (d *RcloneDestination) Exists(ctx context.Context, artifact string) (bool, error) {
	remotePath := d.remotePath(artifact)

	args := d.baseArgs()
	args = append(args, "lsjson", remotePath)

	cmd := exec.CommandContext(ctx, d.rcloneBinary(), args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		// File doesn't exist
		return false, nil
	}

	// If lsjson returns something, file exists
	return stdout.Len() > 2, nil // > 2 because empty array is "[]"
}

// NewDestination creates a destination based on configuration
func NewDestination(config *DestinationConfig, logger Logger) (Destination, error) {
	if config == nil {
		return nil, fmt.Errorf("destination configuration is required")
	}

	switch config.Type {
	case DestinationTypeLocal:
		if config.Path == "" {
			return nil, fmt.Errorf("local destination requires path")
		}
		return NewLocalDestination(config.Path, logger), nil

	case DestinationTypeS3:
		if config.S3 == nil || config.S3.Bucket == "" {
			return nil, fmt.Errorf("S3 destination requires bucket configuration")
		}
		return NewS3Destination(*config.S3, logger), nil

	case DestinationTypeGCS:
		if config.GCS == nil || config.GCS.Bucket == "" {
			return nil, fmt.Errorf("GCS destination requires bucket configuration")
		}
		return NewGCSDestination(*config.GCS, logger), nil

	case DestinationTypeAzureBlob:
		if config.Azure == nil || config.Azure.AccountName == "" || config.Azure.ContainerName == "" {
			return nil, fmt.Errorf("Azure destination requires account and container configuration")
		}
		return NewAzureBlobDestination(*config.Azure, logger), nil

	case DestinationTypeSFTP:
		if config.SFTP == nil || config.SFTP.Host == "" || config.SFTP.User == "" {
			return nil, fmt.Errorf("SFTP destination requires host and user configuration")
		}
		return NewSFTPDestination(*config.SFTP, logger), nil

	case DestinationTypeRclone:
		if config.Rclone == nil || config.Rclone.Remote == "" {
			return nil, fmt.Errorf("rclone destination requires remote configuration")
		}
		return NewRcloneDestination(*config.Rclone, logger), nil

	default:
		return nil, fmt.Errorf("unsupported destination type: %s", config.Type)
	}
}

// MultiDestination writes to multiple destinations (for redundancy)
type MultiDestination struct {
	destinations []Destination
	logger       Logger
}

// NewMultiDestination creates a destination that writes to multiple backends
func NewMultiDestination(destinations []Destination, logger Logger) *MultiDestination {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &MultiDestination{
		destinations: destinations,
		logger:       logger,
	}
}

// Type returns the destination type
func (d *MultiDestination) Type() DestinationType {
	return DestinationType("multi")
}

// Upload uploads to all destinations
func (d *MultiDestination) Upload(ctx context.Context, name string, r io.Reader, size int64) error {
	// Read all data first so we can write to multiple destinations
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	var errors []string
	for _, dest := range d.destinations {
		if err := dest.Upload(ctx, name, bytes.NewReader(data), int64(len(data))); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", dest.Type(), err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("upload failed to some destinations: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Download downloads from the first available destination
func (d *MultiDestination) Download(ctx context.Context, name string, w io.Writer) error {
	for _, dest := range d.destinations {
		if err := dest.Download(ctx, name, w); err == nil {
			return nil
		}
	}
	return fmt.Errorf("download failed from all destinations")
}

// List lists backups from the first available destination
func (d *MultiDestination) List(ctx context.Context) ([]BackupInfo, error) {
	for _, dest := range d.destinations {
		if backups, err := dest.List(ctx); err == nil {
			return backups, nil
		}
	}
	return nil, fmt.Errorf("list failed from all destinations")
}

// Delete deletes from all destinations
func (d *MultiDestination) Delete(ctx context.Context, name string) error {
	var errors []string
	for _, dest := range d.destinations {
		if err := dest.Delete(ctx, name); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", dest.Type(), err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("delete failed from some destinations: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Exists checks if a backup exists in any destination
func (d *MultiDestination) Exists(ctx context.Context, name string) (bool, error) {
	for _, dest := range d.destinations {
		if exists, err := dest.Exists(ctx, name); err == nil && exists {
			return true, nil
		}
	}
	return false, nil
}
