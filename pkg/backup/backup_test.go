package backup

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupConfig(t *testing.T) {
	config := DefaultBackupConfig()

	if config.Type != BackupTypeFull {
		t.Errorf("expected default type %s, got %s", BackupTypeFull, config.Type)
	}
	if config.Compression != CompressionTypeGzip {
		t.Errorf("expected default compression %s, got %s", CompressionTypeGzip, config.Compression)
	}
	if len(config.Components) == 0 {
		t.Error("expected default components")
	}
}

func TestBackupManager_Creation(t *testing.T) {
	bm, err := NewBackupManager(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bm == nil {
		t.Fatal("expected non-nil backup manager")
	}
	if bm.config == nil {
		t.Error("expected default config")
	}
}

func TestBackupManager_RegisterExporter(t *testing.T) {
	bm, _ := NewBackupManager(nil, nil)

	exporter := &testExporter{component: ComponentTypeDatabase}
	bm.RegisterExporter(exporter)

	if len(bm.exporters) != 1 {
		t.Errorf("expected 1 exporter, got %d", len(bm.exporters))
	}
}

func TestBackupManager_SetDestination(t *testing.T) {
	bm, _ := NewBackupManager(nil, nil)
	dest := NewLocalDestination(t.TempDir(), nil)

	bm.SetDestination(dest)

	if bm.destination == nil {
		t.Error("expected destination to be set")
	}
}

func TestBackupManager_SetEncryptor(t *testing.T) {
	bm, _ := NewBackupManager(nil, nil)
	enc := NewNoopEncryptor()

	bm.SetEncryptor(enc)

	if bm.encryptor == nil {
		t.Error("expected encryptor to be set")
	}
}

func TestArtifactBuilder_Build(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create test files
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Build artifact
	builder, err := NewArtifactBuilder(tmpDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(t.TempDir(), "backup.tar.gz")
	manifest := &BackupManifest{
		ManifestVersion: ManifestVersion,
		Backup: BackupInfo{
			ID:   "test-backup",
			Name: "test",
		},
		CreatedAt: time.Now(),
	}

	err = builder.Build(ctx, outputPath, manifest, CompressionTypeGzip)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// Verify output exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("output file not created")
	}

	// Verify files are in manifest
	if len(manifest.Files) != 1 {
		t.Errorf("expected 1 file in manifest, got %d", len(manifest.Files))
	}
}

func TestArtifactReader_ReadManifest(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Build artifact
	builder, _ := NewArtifactBuilder(tmpDir, nil)
	outputPath := filepath.Join(t.TempDir(), "backup.tar.gz")
	manifest := &BackupManifest{
		ManifestVersion: ManifestVersion,
		Backup: BackupInfo{
			ID:   "test-backup",
			Name: "test",
		},
		CreatedAt: time.Now(),
	}
	builder.Build(ctx, outputPath, manifest, CompressionTypeGzip)

	// Read manifest
	reader := NewArtifactReader(outputPath, nil)
	readManifest, err := reader.ReadManifest(ctx)
	if err != nil {
		t.Fatalf("read manifest failed: %v", err)
	}

	if readManifest.Backup.ID != "test-backup" {
		t.Errorf("expected backup ID %s, got %s", "test-backup", readManifest.Backup.ID)
	}
}

func TestArtifactReader_Extract(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create test file
	testContent := "test content for extraction"
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Build artifact
	builder, _ := NewArtifactBuilder(tmpDir, nil)
	outputPath := filepath.Join(t.TempDir(), "backup.tar.gz")
	manifest := &BackupManifest{
		ManifestVersion: ManifestVersion,
		Backup:          BackupInfo{ID: "test"},
		CreatedAt:       time.Now(),
	}
	builder.Build(ctx, outputPath, manifest, CompressionTypeGzip)

	// Extract
	reader := NewArtifactReader(outputPath, nil)
	extractDir := t.TempDir()
	err := reader.Extract(ctx, extractDir)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	// Verify extracted file
	extractedFile := filepath.Join(extractDir, "test.txt")
	content, err := os.ReadFile(extractedFile)
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("expected content %q, got %q", testContent, string(content))
	}
}

func TestArtifactReader_VerifyIntegrity(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Build artifact
	builder, _ := NewArtifactBuilder(tmpDir, nil)
	outputPath := filepath.Join(t.TempDir(), "backup.tar.gz")
	manifest := &BackupManifest{
		ManifestVersion: ManifestVersion,
		Backup:          BackupInfo{ID: "test"},
		CreatedAt:       time.Now(),
	}
	builder.Build(ctx, outputPath, manifest, CompressionTypeGzip)

	// Verify integrity
	reader := NewArtifactReader(outputPath, nil)
	result, err := reader.VerifyIntegrity(ctx)
	if err != nil {
		t.Fatalf("verify integrity failed: %v", err)
	}

	if !result.Valid {
		t.Error("expected valid integrity")
	}
	if result.TotalFiles != 1 {
		t.Errorf("expected 1 total file, got %d", result.TotalFiles)
	}
}

func TestLocalDestination(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	dest := NewLocalDestination(tmpDir, nil)

	// Test upload
	testData := []byte("test backup data")
	err := dest.Upload(ctx, "test-backup.tar.gz", bytes.NewReader(testData), int64(len(testData)))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	// Test list
	backups, err := dest.List(ctx)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
	}

	// Test download
	var buf bytes.Buffer
	err = dest.Download(ctx, "test-backup.tar.gz", &buf)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), testData) {
		t.Error("downloaded data does not match")
	}

	// Test delete
	err = dest.Delete(ctx, "test-backup.tar.gz")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	backups, _ = dest.List(ctx)
	if len(backups) != 0 {
		t.Errorf("expected 0 backups after delete, got %d", len(backups))
	}
}

func TestNoopEncryptor(t *testing.T) {
	ctx := context.Background()
	enc := NewNoopEncryptor()

	if enc.Type() != EncryptionTypeNone {
		t.Errorf("expected type %s, got %s", EncryptionTypeNone, enc.Type())
	}

	// Test encrypt/decrypt
	testData := []byte("test data")
	var encrypted bytes.Buffer
	err := enc.Encrypt(ctx, bytes.NewReader(testData), &encrypted)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	var decrypted bytes.Buffer
	err = enc.Decrypt(ctx, bytes.NewReader(encrypted.Bytes()), &decrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted.Bytes(), testData) {
		t.Error("decrypted data does not match original")
	}
}

func TestNewEncryptor(t *testing.T) {
	tests := []struct {
		name      string
		config    *EncryptionConfig
		expectErr bool
	}{
		{
			name:   "nil config returns noop",
			config: nil,
		},
		{
			name:   "none type returns noop",
			config: &EncryptionConfig{Type: EncryptionTypeNone},
		},
		{
			name: "age without config returns error",
			config: &EncryptionConfig{
				Type: EncryptionTypeAge,
			},
			expectErr: true,
		},
		{
			name: "aws kms without key returns error",
			config: &EncryptionConfig{
				Type: EncryptionTypeAWSKMS,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := NewEncryptor(tt.config, nil)
			if tt.expectErr {
				if err == nil {
					t.Error("expected error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if enc == nil {
					t.Error("expected encryptor")
				}
			}
		})
	}
}

func TestRetentionManager_Preview(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	dest := NewLocalDestination(tmpDir, nil)

	// Create test backups
	for i := 0; i < 15; i++ {
		name := "backup-" + time.Now().Add(time.Duration(-i)*24*time.Hour).Format("2006-01-02") + ".tar.gz"
		dest.Upload(ctx, name, strings.NewReader("test"), 4)
	}

	config := &RetentionConfig{
		MaxBackups: 5,
	}
	rm := NewRetentionManager(dest, config, nil)

	preview, err := rm.Preview(ctx)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if preview.TotalBackups != 15 {
		t.Errorf("expected 15 total backups, got %d", preview.TotalBackups)
	}
	if preview.KeepCount != 5 {
		t.Errorf("expected 5 to keep, got %d", preview.KeepCount)
	}
	if preview.DeleteCount != 10 {
		t.Errorf("expected 10 to delete, got %d", preview.DeleteCount)
	}
}

func TestRetentionManager_Apply(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	dest := NewLocalDestination(tmpDir, nil)

	// Create test backups
	for i := 0; i < 10; i++ {
		name := "backup-" + time.Now().Add(time.Duration(-i)*24*time.Hour).Format("2006-01-02") + ".tar.gz"
		dest.Upload(ctx, name, strings.NewReader("test"), 4)
	}

	config := &RetentionConfig{
		MaxBackups: 3,
	}
	rm := NewRetentionManager(dest, config, nil)

	deleted, err := rm.Apply(ctx)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if len(deleted) != 7 {
		t.Errorf("expected 7 deleted, got %d", len(deleted))
	}

	backups, _ := dest.List(ctx)
	if len(backups) != 3 {
		t.Errorf("expected 3 remaining backups, got %d", len(backups))
	}
}

func TestRestoreManager_Creation(t *testing.T) {
	rm := NewRestoreManager(nil, nil)
	if rm == nil {
		t.Fatal("expected non-nil restore manager")
	}
	if rm.config == nil {
		t.Error("expected default config")
	}
}

func TestRestoreManager_RegisterImporter(t *testing.T) {
	rm := NewRestoreManager(nil, nil)

	importer := &testImporter{component: ComponentTypeDatabase}
	rm.RegisterImporter(importer)

	if len(rm.importers) != 1 {
		t.Errorf("expected 1 importer, got %d", len(rm.importers))
	}
}

func TestSQLiteExporter_EstimateSize(t *testing.T) {
	ctx := context.Background()
	tmpFile := filepath.Join(t.TempDir(), "test.db")

	// Create test file
	if err := os.WriteFile(tmpFile, []byte("test database content"), 0644); err != nil {
		t.Fatal(err)
	}

	exporter := NewSQLiteExporter(tmpFile, nil)

	size, err := exporter.EstimateSize(ctx)
	if err != nil {
		t.Fatalf("estimate size failed: %v", err)
	}

	if size != 21 { // "test database content" = 21 bytes
		t.Errorf("expected size 21, got %d", size)
	}
}

func TestConfigExporter(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create test config
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("key: value"), 0644); err != nil {
		t.Fatal(err)
	}

	exporter := NewConfigExporter(tmpDir, nil)

	if exporter.Name() != "config" {
		t.Errorf("expected name 'config', got %s", exporter.Name())
	}
	if exporter.Component() != ComponentTypeConfig {
		t.Errorf("expected component %s, got %s", ComponentTypeConfig, exporter.Component())
	}

	size, err := exporter.EstimateSize(ctx)
	if err != nil {
		t.Fatalf("estimate size failed: %v", err)
	}
	if size != 10 { // "key: value" = 10 bytes
		t.Errorf("expected size 10, got %d", size)
	}
}

func TestNewDestination(t *testing.T) {
	tests := []struct {
		name      string
		config    *DestinationConfig
		expectErr bool
	}{
		{
			name:      "nil config returns error",
			config:    nil,
			expectErr: true,
		},
		{
			name: "local without path returns error",
			config: &DestinationConfig{
				Type: DestinationTypeLocal,
			},
			expectErr: true,
		},
		{
			name: "local with path succeeds",
			config: &DestinationConfig{
				Type: DestinationTypeLocal,
				Path: "/tmp/backups",
			},
		},
		{
			name: "s3 without bucket returns error",
			config: &DestinationConfig{
				Type: DestinationTypeS3,
			},
			expectErr: true,
		},
		{
			name: "s3 with config succeeds",
			config: &DestinationConfig{
				Type: DestinationTypeS3,
				S3:   &S3Config{Bucket: "test-bucket"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest, err := NewDestination(tt.config, nil)
			if tt.expectErr {
				if err == nil {
					t.Error("expected error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if dest == nil {
					t.Error("expected destination")
				}
			}
		})
	}
}

func TestMultiDestination(t *testing.T) {
	ctx := context.Background()
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	dest1 := NewLocalDestination(tmpDir1, nil)
	dest2 := NewLocalDestination(tmpDir2, nil)

	multi := NewMultiDestination([]Destination{dest1, dest2}, nil)

	// Upload
	testData := []byte("test data")
	err := multi.Upload(ctx, "test.tar.gz", bytes.NewReader(testData), int64(len(testData)))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	// Verify in both destinations
	backups1, _ := dest1.List(ctx)
	backups2, _ := dest2.List(ctx)

	if len(backups1) != 1 {
		t.Errorf("expected 1 backup in dest1, got %d", len(backups1))
	}
	if len(backups2) != 1 {
		t.Errorf("expected 1 backup in dest2, got %d", len(backups2))
	}

	// Download (should work from first available)
	var buf bytes.Buffer
	err = multi.Download(ctx, "test.tar.gz", &buf)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
}

func TestBackupTypes(t *testing.T) {
	tests := []struct {
		backupType BackupType
		expected   string
	}{
		{BackupTypeFull, "full"},
		{BackupTypeIncremental, "incremental"},
		{BackupTypeDatabase, "database"},
		{BackupTypeConfiguration, "configuration"},
	}

	for _, tt := range tests {
		if string(tt.backupType) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, string(tt.backupType))
		}
	}
}

func TestEncryptionTypes(t *testing.T) {
	tests := []struct {
		encType  EncryptionType
		expected string
	}{
		{EncryptionTypeNone, "none"},
		{EncryptionTypeAge, "age"},
		{EncryptionTypeAWSKMS, "aws-kms"},
		{EncryptionTypeGCPKMS, "gcp-kms"},
		{EncryptionTypeAzureKeyVault, "azure-keyvault"},
		{EncryptionTypeVaultTransit, "vault-transit"},
	}

	for _, tt := range tests {
		if string(tt.encType) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, string(tt.encType))
		}
	}
}

func TestDestinationTypes(t *testing.T) {
	tests := []struct {
		destType DestinationType
		expected string
	}{
		{DestinationTypeLocal, "local"},
		{DestinationTypeS3, "s3"},
		{DestinationTypeGCS, "gcs"},
		{DestinationTypeAzureBlob, "azure-blob"},
		{DestinationTypeSFTP, "sftp"},
	}

	for _, tt := range tests {
		if string(tt.destType) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, string(tt.destType))
		}
	}
}

func TestComponentTypes(t *testing.T) {
	tests := []struct {
		compType ComponentType
		expected string
	}{
		{ComponentTypeServer, "server"},
		{ComponentTypeAgent, "agent"},
		{ComponentTypeNATS, "nats"},
		{ComponentTypeDatabase, "database"},
		{ComponentTypeEtcd, "etcd"},
		{ComponentTypeCerts, "certs"},
		{ComponentTypeConfig, "config"},
	}

	for _, tt := range tests {
		if string(tt.compType) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, string(tt.compType))
		}
	}
}

// Test helpers

type testExporter struct {
	component ComponentType
}

func (e *testExporter) Name() string              { return "test" }
func (e *testExporter) Component() ComponentType  { return e.component }
func (e *testExporter) Export(ctx context.Context, w io.Writer) error {
	_, err := w.Write([]byte("test data"))
	return err
}
func (e *testExporter) EstimateSize(ctx context.Context) (int64, error) { return 9, nil }

type testImporter struct {
	component ComponentType
}

func (i *testImporter) Name() string             { return "test" }
func (i *testImporter) Component() ComponentType { return i.component }
func (i *testImporter) Import(ctx context.Context, r io.Reader) error {
	_, err := io.ReadAll(r)
	return err
}
func (i *testImporter) Verify(ctx context.Context) error { return nil }

// EtcdExporter tests

func TestEtcdExporter_Creation(t *testing.T) {
	config := EtcdConfig{
		Endpoints: []string{"localhost:2379"},
		CertFile:  "/path/to/cert",
		KeyFile:   "/path/to/key",
		CAFile:    "/path/to/ca",
	}

	exporter := NewEtcdExporter(config, nil)

	if exporter == nil {
		t.Fatal("expected non-nil exporter")
	}
	if len(exporter.endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(exporter.endpoints))
	}
	if exporter.endpoints[0] != "localhost:2379" {
		t.Errorf("expected endpoint localhost:2379, got %s", exporter.endpoints[0])
	}
	if exporter.certFile != "/path/to/cert" {
		t.Errorf("expected certFile /path/to/cert, got %s", exporter.certFile)
	}
	if exporter.keyFile != "/path/to/key" {
		t.Errorf("expected keyFile /path/to/key, got %s", exporter.keyFile)
	}
	if exporter.caFile != "/path/to/ca" {
		t.Errorf("expected caFile /path/to/ca, got %s", exporter.caFile)
	}
}

func TestEtcdExporter_Name(t *testing.T) {
	exporter := NewEtcdExporter(EtcdConfig{}, nil)

	if exporter.Name() != "etcd" {
		t.Errorf("expected name 'etcd', got %s", exporter.Name())
	}
}

func TestEtcdExporter_Component(t *testing.T) {
	exporter := NewEtcdExporter(EtcdConfig{}, nil)

	if exporter.Component() != ComponentTypeEtcd {
		t.Errorf("expected component %s, got %s", ComponentTypeEtcd, exporter.Component())
	}
}

func TestEtcdExporter_EstimateSize(t *testing.T) {
	ctx := context.Background()
	exporter := NewEtcdExporter(EtcdConfig{}, nil)

	size, err := exporter.EstimateSize(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// etcd snapshot size is hard to estimate, returns 0
	if size != 0 {
		t.Errorf("expected size 0, got %d", size)
	}
}

func TestEtcdExporter_MultipleEndpoints(t *testing.T) {
	config := EtcdConfig{
		Endpoints: []string{"localhost:2379", "localhost:2380", "localhost:2381"},
	}

	exporter := NewEtcdExporter(config, nil)

	if len(exporter.endpoints) != 3 {
		t.Errorf("expected 3 endpoints, got %d", len(exporter.endpoints))
	}
}

// EtcdImporter tests

func TestEtcdImporter_Creation(t *testing.T) {
	config := EtcdConfig{
		Endpoints: []string{"localhost:2379"},
		CertFile:  "/path/to/cert",
		KeyFile:   "/path/to/key",
		CAFile:    "/path/to/ca",
	}

	importer := NewEtcdImporter("/var/lib/etcd", config, nil)

	if importer == nil {
		t.Fatal("expected non-nil importer")
	}
	if importer.dataDir != "/var/lib/etcd" {
		t.Errorf("expected dataDir /var/lib/etcd, got %s", importer.dataDir)
	}
	if len(importer.endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(importer.endpoints))
	}
	if importer.certFile != "/path/to/cert" {
		t.Errorf("expected certFile /path/to/cert, got %s", importer.certFile)
	}
	if importer.keyFile != "/path/to/key" {
		t.Errorf("expected keyFile /path/to/key, got %s", importer.keyFile)
	}
	if importer.caFile != "/path/to/ca" {
		t.Errorf("expected caFile /path/to/ca, got %s", importer.caFile)
	}
}

func TestEtcdImporter_Name(t *testing.T) {
	importer := NewEtcdImporter("/var/lib/etcd", EtcdConfig{}, nil)

	if importer.Name() != "etcd" {
		t.Errorf("expected name 'etcd', got %s", importer.Name())
	}
}

func TestEtcdImporter_Component(t *testing.T) {
	importer := NewEtcdImporter("/var/lib/etcd", EtcdConfig{}, nil)

	if importer.Component() != ComponentTypeEtcd {
		t.Errorf("expected component %s, got %s", ComponentTypeEtcd, importer.Component())
	}
}

func TestEtcdImporter_Verify_Success(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	importer := NewEtcdImporter(tmpDir, EtcdConfig{}, nil)

	err := importer.Verify(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEtcdImporter_Verify_Failure(t *testing.T) {
	ctx := context.Background()

	importer := NewEtcdImporter("/nonexistent/path/to/etcd", EtcdConfig{}, nil)

	err := importer.Verify(ctx)
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestEtcdExporter_ImplementsExporter(t *testing.T) {
	var _ Exporter = (*EtcdExporter)(nil)
}

func TestEtcdImporter_ImplementsImporter(t *testing.T) {
	var _ Importer = (*EtcdImporter)(nil)
}

func TestBackupManager_RegisterEtcdExporter(t *testing.T) {
	bm, _ := NewBackupManager(nil, nil)

	config := EtcdConfig{
		Endpoints: []string{"localhost:2379"},
	}
	exporter := NewEtcdExporter(config, nil)
	bm.RegisterExporter(exporter)

	if len(bm.exporters) != 1 {
		t.Errorf("expected 1 exporter, got %d", len(bm.exporters))
	}
	if _, exists := bm.exporters[ComponentTypeEtcd]; !exists {
		t.Error("expected etcd exporter to be registered")
	}
}

func TestRestoreManager_RegisterEtcdImporter(t *testing.T) {
	rm := NewRestoreManager(nil, nil)

	config := EtcdConfig{
		Endpoints: []string{"localhost:2379"},
	}
	importer := NewEtcdImporter("/var/lib/etcd", config, nil)
	rm.RegisterImporter(importer)

	if len(rm.importers) != 1 {
		t.Errorf("expected 1 importer, got %d", len(rm.importers))
	}
	if _, exists := rm.importers[ComponentTypeEtcd]; !exists {
		t.Error("expected etcd importer to be registered")
	}
}
