package backup

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// newHash returns a new SHA-256 hash
func newHash() hash.Hash {
	return sha256.New()
}

// SQLiteExporter exports SQLite database
type SQLiteExporter struct {
	dbPath string
	logger Logger
}

// NewSQLiteExporter creates a new SQLite exporter
func NewSQLiteExporter(dbPath string, logger Logger) *SQLiteExporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &SQLiteExporter{
		dbPath: dbPath,
		logger: logger,
	}
}

// Name returns the exporter name
func (e *SQLiteExporter) Name() string {
	return "sqlite"
}

// Component returns the component type
func (e *SQLiteExporter) Component() ComponentType {
	return ComponentTypeDatabase
}

// Export exports the SQLite database using the backup API
func (e *SQLiteExporter) Export(ctx context.Context, w io.Writer) error {
	// Check if database exists
	if _, err := os.Stat(e.dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database file not found: %s", e.dbPath)
	}

	// Open database
	db, err := sql.Open("sqlite", e.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Use SQLite's backup by dumping the database
	// We'll do this by copying the database file with proper locking

	// First, checkpoint the WAL if it exists
	_, err = db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		e.logger.Warn("failed to checkpoint WAL", "error", err)
	}

	// Read the database file
	file, err := os.Open(e.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database file: %w", err)
	}
	defer file.Close()

	// Copy to writer
	_, err = io.Copy(w, file)
	if err != nil {
		return fmt.Errorf("failed to copy database: %w", err)
	}

	e.logger.Debug("exported SQLite database", "path", e.dbPath)
	return nil
}

// EstimateSize estimates the export size
func (e *SQLiteExporter) EstimateSize(ctx context.Context) (int64, error) {
	info, err := os.Stat(e.dbPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// SQLiteImporter imports SQLite database
type SQLiteImporter struct {
	dbPath string
	logger Logger
}

// NewSQLiteImporter creates a new SQLite importer
func NewSQLiteImporter(dbPath string, logger Logger) *SQLiteImporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &SQLiteImporter{
		dbPath: dbPath,
		logger: logger,
	}
}

// Name returns the importer name
func (i *SQLiteImporter) Name() string {
	return "sqlite"
}

// Component returns the component type
func (i *SQLiteImporter) Component() ComponentType {
	return ComponentTypeDatabase
}

// Import imports the SQLite database
func (i *SQLiteImporter) Import(ctx context.Context, r io.Reader) error {
	// Create backup of existing database if it exists
	if _, err := os.Stat(i.dbPath); err == nil {
		backupPath := i.dbPath + ".backup"
		if err := copyFile(i.dbPath, backupPath); err != nil {
			i.logger.Warn("failed to backup existing database", "error", err)
		}
	}

	// Create directory if needed
	//nolint:gosec // G301: database directory needs to be accessible by service user
	if err := os.MkdirAll(filepath.Dir(i.dbPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write the database
	file, err := os.Create(i.dbPath)
	if err != nil {
		return fmt.Errorf("failed to create database file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, r)
	if err != nil {
		return fmt.Errorf("failed to write database: %w", err)
	}

	i.logger.Debug("imported SQLite database", "path", i.dbPath)
	return nil
}

// Verify verifies the imported database
func (i *SQLiteImporter) Verify(ctx context.Context) error {
	db, err := sql.Open("sqlite", i.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Run integrity check
	var result string
	err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result)
	if err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}

	if result != "ok" {
		return fmt.Errorf("database integrity check failed: %s", result)
	}

	return nil
}

// PostgreSQLExporter exports PostgreSQL database using pg_dump
type PostgreSQLExporter struct {
	host     string
	port     int
	database string
	user     string
	password string
	sslMode  string
	logger   Logger
}

// PostgreSQLConfig holds PostgreSQL connection configuration
type PostgreSQLConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string
}

// NewPostgreSQLExporter creates a new PostgreSQL exporter
func NewPostgreSQLExporter(config PostgreSQLConfig, logger Logger) *PostgreSQLExporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	if config.Port == 0 {
		config.Port = 5432
	}
	if config.SSLMode == "" {
		config.SSLMode = "prefer"
	}
	return &PostgreSQLExporter{
		host:     config.Host,
		port:     config.Port,
		database: config.Database,
		user:     config.User,
		password: config.Password,
		sslMode:  config.SSLMode,
		logger:   logger,
	}
}

// Name returns the exporter name
func (e *PostgreSQLExporter) Name() string {
	return "postgresql"
}

// Component returns the component type
func (e *PostgreSQLExporter) Component() ComponentType {
	return ComponentTypeDatabase
}

// Export exports the PostgreSQL database using pg_dump
func (e *PostgreSQLExporter) Export(ctx context.Context, w io.Writer) error {
	// Build pg_dump command
	args := []string{
		"-h", e.host,
		"-p", fmt.Sprintf("%d", e.port),
		"-U", e.user,
		"-d", e.database,
		"-F", "c", // Custom format for pg_restore compatibility
		"--no-password",
	}

	//nolint:gosec // G204: pg_dump execution is intentional for database backup
	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	// Set password via environment
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", e.password))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}

	e.logger.Debug("exported PostgreSQL database", "database", e.database)
	return nil
}

// EstimateSize estimates the export size
func (e *PostgreSQLExporter) EstimateSize(ctx context.Context) (int64, error) {
	// Connect to get database size
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		e.host, e.port, e.user, e.password, e.database, e.sslMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var size int64
	err = db.QueryRowContext(ctx, "SELECT pg_database_size($1)", e.database).Scan(&size)
	if err != nil {
		return 0, err
	}

	return size, nil
}

// PostgreSQLImporter imports PostgreSQL database using pg_restore
type PostgreSQLImporter struct {
	host     string
	port     int
	database string
	user     string
	password string
	sslMode  string
	logger   Logger
}

// NewPostgreSQLImporter creates a new PostgreSQL importer
func NewPostgreSQLImporter(config PostgreSQLConfig, logger Logger) *PostgreSQLImporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	if config.Port == 0 {
		config.Port = 5432
	}
	if config.SSLMode == "" {
		config.SSLMode = "prefer"
	}
	return &PostgreSQLImporter{
		host:     config.Host,
		port:     config.Port,
		database: config.Database,
		user:     config.User,
		password: config.Password,
		sslMode:  config.SSLMode,
		logger:   logger,
	}
}

// Name returns the importer name
func (i *PostgreSQLImporter) Name() string {
	return "postgresql"
}

// Component returns the component type
func (i *PostgreSQLImporter) Component() ComponentType {
	return ComponentTypeDatabase
}

// Import imports the PostgreSQL database using pg_restore
func (i *PostgreSQLImporter) Import(ctx context.Context, r io.Reader) error {
	// Write to temp file first (pg_restore needs a file)
	tmpFile, err := os.CreateTemp("", "pg_restore_*.dump")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, r); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Build pg_restore command
	args := []string{
		"-h", i.host,
		"-p", fmt.Sprintf("%d", i.port),
		"-U", i.user,
		"-d", i.database,
		"--clean",     // Drop objects before recreating
		"--if-exists", // Don't error if objects don't exist
		"--no-owner",  // Don't set ownership
		"--no-password",
		tmpFile.Name(),
	}

	//nolint:gosec // G204: pg_restore execution is intentional for database restore
	cmd := exec.CommandContext(ctx, "pg_restore", args...)
	cmd.Stderr = os.Stderr

	// Set password via environment
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", i.password))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_restore failed: %w", err)
	}

	i.logger.Debug("imported PostgreSQL database", "database", i.database)
	return nil
}

// Verify verifies the imported database
func (i *PostgreSQLImporter) Verify(ctx context.Context) error {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		i.host, i.port, i.user, i.password, i.database, i.sslMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer db.Close()

	// Simple connectivity check
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// EtcdExporter exports etcd data
type EtcdExporter struct {
	endpoints []string
	certFile  string
	keyFile   string
	caFile    string
	logger    Logger
}

// EtcdConfig holds etcd connection configuration
type EtcdConfig struct {
	Endpoints []string
	CertFile  string
	KeyFile   string
	CAFile    string
}

// NewEtcdExporter creates a new etcd exporter
func NewEtcdExporter(config EtcdConfig, logger Logger) *EtcdExporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &EtcdExporter{
		endpoints: config.Endpoints,
		certFile:  config.CertFile,
		keyFile:   config.KeyFile,
		caFile:    config.CAFile,
		logger:    logger,
	}
}

// Name returns the exporter name
func (e *EtcdExporter) Name() string {
	return "etcd"
}

// Component returns the component type
func (e *EtcdExporter) Component() ComponentType {
	return ComponentTypeEtcd
}

// Export exports etcd data using etcdctl snapshot
func (e *EtcdExporter) Export(ctx context.Context, w io.Writer) error {
	// Create temp file for snapshot
	tmpFile, err := os.CreateTemp("", "etcd_snapshot_*.db")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Build etcdctl command
	args := []string{
		"snapshot", "save", tmpFile.Name(),
		"--endpoints=" + strings.Join(e.endpoints, ","),
	}

	if e.certFile != "" {
		args = append(args, "--cert="+e.certFile)
	}
	if e.keyFile != "" {
		args = append(args, "--key="+e.keyFile)
	}
	if e.caFile != "" {
		args = append(args, "--cacert="+e.caFile)
	}

	//nolint:gosec // G204: etcdctl execution is intentional for etcd backup
	cmd := exec.CommandContext(ctx, "etcdctl", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("etcdctl snapshot failed: %w", err)
	}

	// Copy snapshot to writer
	file, err := os.Open(tmpFile.Name())
	if err != nil {
		return fmt.Errorf("failed to open snapshot: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(w, file); err != nil {
		return fmt.Errorf("failed to copy snapshot: %w", err)
	}

	e.logger.Debug("exported etcd snapshot")
	return nil
}

// EstimateSize estimates the export size
func (e *EtcdExporter) EstimateSize(ctx context.Context) (int64, error) {
	// etcd snapshot size is hard to estimate, return 0
	return 0, nil
}

// EtcdImporter imports etcd data
type EtcdImporter struct {
	dataDir   string
	endpoints []string
	certFile  string
	keyFile   string
	caFile    string
	logger    Logger
}

// NewEtcdImporter creates a new etcd importer
func NewEtcdImporter(dataDir string, config EtcdConfig, logger Logger) *EtcdImporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &EtcdImporter{
		dataDir:   dataDir,
		endpoints: config.Endpoints,
		certFile:  config.CertFile,
		keyFile:   config.KeyFile,
		caFile:    config.CAFile,
		logger:    logger,
	}
}

// Name returns the importer name
func (i *EtcdImporter) Name() string {
	return "etcd"
}

// Component returns the component type
func (i *EtcdImporter) Component() ComponentType {
	return ComponentTypeEtcd
}

// Import imports etcd data using etcdctl snapshot restore
func (i *EtcdImporter) Import(ctx context.Context, r io.Reader) error {
	// Create temp file for snapshot
	tmpFile, err := os.CreateTemp("", "etcd_restore_*.db")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, r); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}
	tmpFile.Close()

	// Build etcdctl command
	args := []string{
		"snapshot", "restore", tmpFile.Name(),
		"--data-dir=" + i.dataDir,
	}

	//nolint:gosec // G204: etcdctl execution is intentional for etcd restore
	cmd := exec.CommandContext(ctx, "etcdctl", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("etcdctl restore failed: %w", err)
	}

	i.logger.Debug("restored etcd snapshot", "data_dir", i.dataDir)
	return nil
}

// Verify verifies the restored etcd
func (i *EtcdImporter) Verify(ctx context.Context) error {
	// Check data directory exists
	if _, err := os.Stat(i.dataDir); err != nil {
		return fmt.Errorf("etcd data directory not found: %w", err)
	}
	return nil
}

// JetStreamExporter exports NATS JetStream data
type JetStreamExporter struct {
	natsURL string
	dataDir string
	logger  Logger
}

// NewJetStreamExporter creates a new JetStream exporter
func NewJetStreamExporter(natsURL, dataDir string, logger Logger) *JetStreamExporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &JetStreamExporter{
		natsURL: natsURL,
		dataDir: dataDir,
		logger:  logger,
	}
}

// Name returns the exporter name
func (e *JetStreamExporter) Name() string {
	return "jetstream"
}

// Component returns the component type
func (e *JetStreamExporter) Component() ComponentType {
	return ComponentTypeNATS
}

// Export exports JetStream data by copying the data directory
func (e *JetStreamExporter) Export(ctx context.Context, w io.Writer) error {
	// For embedded NATS with JetStream, the data is in the data directory
	// We'll create a tar archive of the JetStream directory

	if e.dataDir == "" {
		e.logger.Warn("no JetStream data directory configured")
		return nil
	}

	jetStreamDir := filepath.Join(e.dataDir, "jetstream")
	if _, err := os.Stat(jetStreamDir); os.IsNotExist(err) {
		e.logger.Warn("JetStream data directory not found", "path", jetStreamDir)
		return nil
	}

	// Use tar to create archive
	//nolint:gosec // G204: tar execution is intentional for JetStream backup
	cmd := exec.CommandContext(ctx, "tar", "-cf", "-", "-C", e.dataDir, "jetstream")
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to archive JetStream data: %w", err)
	}

	e.logger.Debug("exported JetStream data", "data_dir", jetStreamDir)
	return nil
}

// EstimateSize estimates the export size
func (e *JetStreamExporter) EstimateSize(ctx context.Context) (int64, error) {
	jetStreamDir := filepath.Join(e.dataDir, "jetstream")
	return getDirSize(jetStreamDir)
}

// JetStreamImporter imports NATS JetStream data
type JetStreamImporter struct {
	dataDir string
	logger  Logger
}

// NewJetStreamImporter creates a new JetStream importer
func NewJetStreamImporter(dataDir string, logger Logger) *JetStreamImporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &JetStreamImporter{
		dataDir: dataDir,
		logger:  logger,
	}
}

// Name returns the importer name
func (i *JetStreamImporter) Name() string {
	return "jetstream"
}

// Component returns the component type
func (i *JetStreamImporter) Component() ComponentType {
	return ComponentTypeNATS
}

// Import imports JetStream data
func (i *JetStreamImporter) Import(ctx context.Context, r io.Reader) error {
	// Extract tar archive to data directory
	//nolint:gosec // G204: tar execution is intentional for JetStream restore
	cmd := exec.CommandContext(ctx, "tar", "-xf", "-", "-C", i.dataDir)
	cmd.Stdin = r
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract JetStream data: %w", err)
	}

	i.logger.Debug("imported JetStream data", "data_dir", i.dataDir)
	return nil
}

// Verify verifies the imported JetStream data
func (i *JetStreamImporter) Verify(ctx context.Context) error {
	jetStreamDir := filepath.Join(i.dataDir, "jetstream")
	if _, err := os.Stat(jetStreamDir); err != nil {
		return fmt.Errorf("JetStream data directory not found: %w", err)
	}
	return nil
}

// ConfigExporter exports configuration files
type ConfigExporter struct {
	configDir string
	logger    Logger
}

// NewConfigExporter creates a new config exporter
func NewConfigExporter(configDir string, logger Logger) *ConfigExporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &ConfigExporter{
		configDir: configDir,
		logger:    logger,
	}
}

// Name returns the exporter name
func (e *ConfigExporter) Name() string {
	return "config"
}

// Component returns the component type
func (e *ConfigExporter) Component() ComponentType {
	return ComponentTypeConfig
}

// Export exports configuration files
func (e *ConfigExporter) Export(ctx context.Context, w io.Writer) error {
	if _, err := os.Stat(e.configDir); os.IsNotExist(err) {
		e.logger.Warn("config directory not found", "path", e.configDir)
		return nil
	}

	// Use tar to create archive
	//nolint:gosec // G204: tar execution is intentional for config backup operations
	cmd := exec.CommandContext(ctx, "tar", "-cf", "-", "-C", filepath.Dir(e.configDir), filepath.Base(e.configDir))
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to archive config: %w", err)
	}

	e.logger.Debug("exported config", "config_dir", e.configDir)
	return nil
}

// EstimateSize estimates the export size
func (e *ConfigExporter) EstimateSize(ctx context.Context) (int64, error) {
	return getDirSize(e.configDir)
}

// ConfigImporter imports configuration files
type ConfigImporter struct {
	configDir string
	logger    Logger
}

// NewConfigImporter creates a new config importer
func NewConfigImporter(configDir string, logger Logger) *ConfigImporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &ConfigImporter{
		configDir: configDir,
		logger:    logger,
	}
}

// Name returns the importer name
func (i *ConfigImporter) Name() string {
	return "config"
}

// Component returns the component type
func (i *ConfigImporter) Component() ComponentType {
	return ComponentTypeConfig
}

// Import imports configuration files
func (i *ConfigImporter) Import(ctx context.Context, r io.Reader) error {
	// Create config directory
	//nolint:gosec // G301: config directory needs to be accessible by service user
	if err := os.MkdirAll(filepath.Dir(i.configDir), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Extract tar archive
	//nolint:gosec // G204: tar execution is intentional for config restore operations
	cmd := exec.CommandContext(ctx, "tar", "-xf", "-", "-C", filepath.Dir(i.configDir))
	cmd.Stdin = r
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract config: %w", err)
	}

	i.logger.Debug("imported config", "config_dir", i.configDir)
	return nil
}

// Verify verifies the imported configuration
func (i *ConfigImporter) Verify(ctx context.Context) error {
	if _, err := os.Stat(i.configDir); err != nil {
		return fmt.Errorf("config directory not found: %w", err)
	}
	return nil
}

// CertsExporter exports certificate files
type CertsExporter struct {
	certsDir string
	logger   Logger
}

// NewCertsExporter creates a new certs exporter
func NewCertsExporter(certsDir string, logger Logger) *CertsExporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &CertsExporter{
		certsDir: certsDir,
		logger:   logger,
	}
}

// Name returns the exporter name
func (e *CertsExporter) Name() string {
	return "certs"
}

// Component returns the component type
func (e *CertsExporter) Component() ComponentType {
	return ComponentTypeCerts
}

// Export exports certificate files
func (e *CertsExporter) Export(ctx context.Context, w io.Writer) error {
	if _, err := os.Stat(e.certsDir); os.IsNotExist(err) {
		e.logger.Warn("certs directory not found", "path", e.certsDir)
		return nil
	}

	// Use tar to create archive
	//nolint:gosec // G204: tar execution is intentional for certificate backup operations
	cmd := exec.CommandContext(ctx, "tar", "-cf", "-", "-C", filepath.Dir(e.certsDir), filepath.Base(e.certsDir))
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to archive certs: %w", err)
	}

	e.logger.Debug("exported certs", "certs_dir", e.certsDir)
	return nil
}

// EstimateSize estimates the export size
func (e *CertsExporter) EstimateSize(ctx context.Context) (int64, error) {
	return getDirSize(e.certsDir)
}

// CertsImporter imports certificate files
type CertsImporter struct {
	certsDir string
	logger   Logger
}

// NewCertsImporter creates a new certs importer
func NewCertsImporter(certsDir string, logger Logger) *CertsImporter {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &CertsImporter{
		certsDir: certsDir,
		logger:   logger,
	}
}

// Name returns the importer name
func (i *CertsImporter) Name() string {
	return "certs"
}

// Component returns the component type
func (i *CertsImporter) Component() ComponentType {
	return ComponentTypeCerts
}

// Import imports certificate files
func (i *CertsImporter) Import(ctx context.Context, r io.Reader) error {
	// Create certs directory
	//nolint:gosec // G301: certs directory needs to be accessible by service user
	if err := os.MkdirAll(filepath.Dir(i.certsDir), 0o755); err != nil {
		return fmt.Errorf("failed to create certs directory: %w", err)
	}

	// Extract tar archive
	//nolint:gosec // G204: tar execution is intentional for certificate restore operations
	cmd := exec.CommandContext(ctx, "tar", "-xf", "-", "-C", filepath.Dir(i.certsDir))
	cmd.Stdin = r
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract certs: %w", err)
	}

	i.logger.Debug("imported certs", "certs_dir", i.certsDir)
	return nil
}

// Verify verifies the imported certificates
func (i *CertsImporter) Verify(ctx context.Context) error {
	if _, err := os.Stat(i.certsDir); err != nil {
		return fmt.Errorf("certs directory not found: %w", err)
	}
	return nil
}

// getDirSize calculates the total size of a directory
func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Copy file mode
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, sourceInfo.Mode())
}

// calculateFileChecksum calculates SHA-256 checksum of a file
func calculateFileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	return calculateChecksum(file)
}

// calculateChecksum calculates SHA-256 checksum of data
func calculateChecksum(r io.Reader) (string, error) {
	h := newHash()
	if _, err := io.Copy(h, bufio.NewReader(r)); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}
