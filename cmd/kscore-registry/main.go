// Package main provides the kscore-registry server for module distribution
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/registry/storage"
	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/registry"
	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
	"github.com/shawnbutts/keystone-core/pkg/module/verify"
)

var version = "0.1.0"

func renderVersionList(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	return strings.Join(versions, "\n") + "\n"
}

// Config holds server configuration
type Config struct {
	// DataDir is the directory for storing modules
	DataDir string

	// ListenAddr is the address to listen on
	ListenAddr string

	// APIKey is the optional API key for authentication (write operations)
	APIKey string

	// ReadOnly disables write operations
	ReadOnly bool

	// MaxUploadSize is the maximum upload size in bytes
	MaxUploadSize int64

	// EnableCORS enables CORS headers
	EnableCORS bool

	// CORSOrigins is a comma-separated list of allowed origins (default: none, requires explicit config)
	CORSOrigins string

	// StorageBackend selects the storage backend type (filesystem, s3, gcs, azure, nats)
	StorageBackend string

	// StorageConfig holds backend-specific configuration
	StorageConfig storage.BackendConfig
}

// Server is the module registry HTTP server
type Server struct {
	config  Config
	mux     *http.ServeMux
	storage storage.Backend
	hasher  verify.HashVerifier
}

// NewServer creates a new registry server
func NewServer(config Config, store storage.Backend) *Server {
	s := &Server{
		config:  config,
		mux:     http.NewServeMux(),
		storage: store,
		hasher:  verify.NewDefaultHashVerifier(),
	}
	s.setupRoutes()
	return s
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() {
	// Health check
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/", s.handleRegistry)
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	}

	if err := s.storage.Health(r.Context()); err != nil {
		resp["status"] = "degraded"
		resp["storage_error"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleRegistry routes requests to the appropriate handler
func (s *Server) handleRegistry(w http.ResponseWriter, r *http.Request) {
	if s.config.EnableCORS {
		// Use configured origins, or reject if none configured
		origin := r.Header.Get("Origin")
		if origin != "" && s.isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		s.handleIndex(w, r)
		return
	}

	// Parse path: <module>/@v/list, <module>/@v/<version>.info, etc.
	parts := strings.Split(path, "/@v/")
	if len(parts) != 2 {
		s.writeError(w, http.StatusNotFound, registry.ErrCodeModuleNotFound, "Invalid path format")
		return
	}

	moduleName := parts[0]
	versionPart := parts[1]

	switch {
	case versionPart == "list":
		s.handleListVersions(w, r, moduleName)
	case strings.HasSuffix(versionPart, ".info"):
		version := strings.TrimSuffix(versionPart, ".info")
		s.handleGetInfo(w, r, moduleName, version)
	case strings.HasSuffix(versionPart, ".mod"):
		version := strings.TrimSuffix(versionPart, ".mod")
		s.handleGetManifest(w, r, moduleName, version)
	case strings.HasSuffix(versionPart, ".zip"):
		version := strings.TrimSuffix(versionPart, ".zip")
		s.handleDownload(w, r, moduleName, version)
	default:
		// POST/DELETE for <module>/@v/<version>
		switch r.Method {
		case http.MethodPost:
			s.handlePublish(w, r, moduleName, versionPart)
		case http.MethodDelete:
			s.handleDelete(w, r, moduleName, versionPart)
		default:
			s.writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed")
		}
	}
}

// handleIndex returns server info
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":     "kscore-registry",
		"version":  version,
		"readonly": s.config.ReadOnly,
	})
}

// handleListVersions returns all versions for a module
func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request, moduleName string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed")
		return
	}

	versions, err := s.storage.ListVersions(r.Context(), moduleName)
	if err != nil {
		status, code := mapStorageError(err)
		s.writeError(w, status, code, fmt.Sprintf("Module not found: %s", moduleName))
		return
	}

	// Sort versions in descending order (newest first)
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

	output := renderVersionList(versions)
	w.Header().Set("Content-Type", "text/plain")
	if _, err := io.WriteString(w, output); err != nil { // nosemgrep: go.lang.security.audit.xss.no-io-writestring-to-responsewriter.no-io-writestring-to-responsewriter -- plain text version list response, no HTML content
		log.Printf("Failed to write version list: %v", err)
	}
}

// handleGetInfo returns module metadata
func (s *Server) handleGetInfo(w http.ResponseWriter, r *http.Request, moduleName, version string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed")
		return
	}

	stored, err := s.storage.GetInfo(r.Context(), moduleName, version)
	if err != nil {
		status, code := mapStorageError(err)
		if errors.Is(err, storage.ErrVersionNotFound) {
			s.writeError(w, status, code, fmt.Sprintf("Version not found: %s@%s", moduleName, version))
		} else {
			s.writeError(w, status, code, fmt.Sprintf("Failed to load module info: %v", err))
		}
		return
	}

	// Convert to ModuleInfo
	info := resolver.ModuleInfo{
		Name:         stored.Name,
		Version:      stored.Version,
		Hash:         stored.Hash,
		PublishedAt:  stored.PublishedAt,
		Description:  stored.Description,
		Dependencies: stored.Dependencies,
		Size:         stored.Size,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleGetManifest returns the module manifest
func (s *Server) handleGetManifest(w http.ResponseWriter, r *http.Request, moduleName, version string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed")
		return
	}

	rc, _, err := s.storage.GetManifest(r.Context(), moduleName, version)
	if err != nil {
		status, code := mapStorageError(err)
		if errors.Is(err, storage.ErrVersionNotFound) {
			s.writeError(w, status, code, fmt.Sprintf("Version not found: %s@%s", moduleName, version))
		} else {
			s.writeError(w, status, code, fmt.Sprintf("Failed to open manifest: %v", err))
		}
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/x-yaml")
	io.Copy(w, rc)
}

// handleDownload returns the module ZIP file
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request, moduleName, version string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed")
		return
	}

	rc, size, err := s.storage.GetZip(r.Context(), moduleName, version)
	if err != nil {
		status, code := mapStorageError(err)
		if errors.Is(err, storage.ErrVersionNotFound) {
			s.writeError(w, status, code, fmt.Sprintf("Version not found: %s@%s", moduleName, version))
		} else {
			s.writeError(w, status, code, fmt.Sprintf("Failed to open module: %v", err))
		}
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s-%s.zip",
		strings.ReplaceAll(moduleName, "/", "-"), version))
	if size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	}
	io.Copy(w, rc)
}

// handlePublish handles module upload
func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request, moduleName, version string) {
	if s.config.ReadOnly {
		s.writeError(w, http.StatusForbidden, registry.ErrCodeForbidden, "Registry is read-only")
		return
	}

	// Check authentication
	if s.config.APIKey != "" {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if apiKey != s.config.APIKey {
			s.writeError(w, http.StatusUnauthorized, registry.ErrCodeUnauthorized, "Invalid API key")
			return
		}
	}

	// Limit upload size
	if s.config.MaxUploadSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxUploadSize)
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max memory
		s.writeError(w, http.StatusBadRequest, registry.ErrCodeInvalidModule,
			fmt.Sprintf("Failed to parse form: %v", err))
		return
	}

	// Get required fields
	moduleFile, fileHeader, err := r.FormFile("module")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, registry.ErrCodeInvalidModule, "Module file required")
		return
	}
	defer moduleFile.Close()

	manifestData := r.FormValue("manifest")
	if manifestData == "" {
		s.writeError(w, http.StatusBadRequest, registry.ErrCodeInvalidModule, "Manifest required")
		return
	}

	hash := r.FormValue("hash")
	signature := r.FormValue("signature")
	force := r.FormValue("force") == "true"
	releaseNotes := r.FormValue("release_notes")

	var tags []string
	if tagsJSON := r.FormValue("tags"); tagsJSON != "" {
		json.Unmarshal([]byte(tagsJSON), &tags)
	}

	// Parse manifest
	var m manifest.Manifest
	if err := yaml.Unmarshal([]byte(manifestData), &m); err != nil {
		s.writeError(w, http.StatusBadRequest, registry.ErrCodeInvalidModule,
			fmt.Sprintf("Invalid manifest: %v", err))
		return
	}

	// Validate version matches
	if m.Name != moduleName || m.Version != version {
		s.writeError(w, http.StatusBadRequest, registry.ErrCodeInvalidModule,
			fmt.Sprintf("Manifest mismatch: expected %s@%s, got %s@%s",
				moduleName, version, m.Name, m.Version))
		return
	}

	// Early conflict check: reject before expensive hash computation
	if !force {
		exists, err := s.storage.VersionExists(r.Context(), moduleName, version)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
				fmt.Sprintf("Failed to check version: %v", err))
			return
		}
		if exists {
			s.writeError(w, http.StatusConflict, registry.ErrCodeVersionExists,
				fmt.Sprintf("Version already exists: %s@%s", moduleName, version))
			return
		}
	}

	// Write upload to temp file for hash computation
	tmpFile, err := os.CreateTemp("", "kscore-registry-upload-*.zip")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to create temp file: %v", err))
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, moduleFile); err != nil {
		tmpFile.Close()
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to write temp file: %v", err))
		return
	}
	tmpFile.Close()

	// Compute or verify hash
	if hash == "" {
		hash, err = s.hasher.ComputeHash(tmpPath)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
				fmt.Sprintf("Failed to compute hash: %v", err))
			return
		}
	} else {
		computedHash, err := s.hasher.ComputeHash(tmpPath)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
				fmt.Sprintf("Failed to verify hash: %v", err))
			return
		}
		if computedHash != hash {
			s.writeError(w, http.StatusBadRequest, registry.ErrCodeInvalidModule,
				fmt.Sprintf("Hash mismatch: expected %s, got %s", hash, computedHash))
			return
		}
	}

	// Open temp file for storage backend
	zipReader, err := os.Open(tmpPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to open temp file: %v", err))
		return
	}
	defer zipReader.Close()

	req := &storage.PublishRequest{
		ModuleName:   moduleName,
		Version:      version,
		ZipData:      zipReader,
		Manifest:     []byte(manifestData),
		Signature:    signature,
		Hash:         hash,
		Force:        force,
		Description:  m.Description,
		Dependencies: m.Dependencies,
		Tags:         tags,
		ReleaseNotes: releaseNotes,
	}

	pubResult, err := s.storage.Publish(r.Context(), req)
	if err != nil {
		status, code := mapStorageError(err)
		if errors.Is(err, storage.ErrVersionExists) {
			s.writeError(w, status, code,
				fmt.Sprintf("Version already exists: %s@%s", moduleName, version))
		} else {
			s.writeError(w, status, code, fmt.Sprintf("Failed to publish: %v", err))
		}
		return
	}

	// Return result
	result := registry.PublishResult{
		ModuleName:  moduleName,
		Version:     version,
		Hash:        pubResult.Hash,
		URL:         fmt.Sprintf("/%s/@v/%s.zip", moduleName, version),
		PublishedAt: time.Now(),
		Size:        pubResult.Size,
	}

	// Log upload
	log.Printf("Published %s@%s (%d bytes, hash: %s) by %s",
		moduleName, version, pubResult.Size, hash[:16]+"...", r.RemoteAddr)

	// Also log original filename if available
	if fileHeader != nil && fileHeader.Filename != "" {
		log.Printf("  Original filename: %s", fileHeader.Filename)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// handleDelete removes a module version
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, moduleName, version string) {
	if s.config.ReadOnly {
		s.writeError(w, http.StatusForbidden, registry.ErrCodeForbidden, "Registry is read-only")
		return
	}

	// Check authentication
	if s.config.APIKey != "" {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if apiKey != s.config.APIKey {
			s.writeError(w, http.StatusUnauthorized, registry.ErrCodeUnauthorized, "Invalid API key")
			return
		}
	}

	if err := s.storage.Delete(r.Context(), moduleName, version); err != nil {
		status, code := mapStorageError(err)
		if errors.Is(err, storage.ErrVersionNotFound) {
			s.writeError(w, status, code,
				fmt.Sprintf("Version not found: %s@%s", moduleName, version))
		} else {
			s.writeError(w, status, code, fmt.Sprintf("Failed to delete: %v", err))
		}
		return
	}

	log.Printf("Deleted %s@%s by %s", moduleName, version, r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

// mapStorageError maps storage errors to HTTP status codes and error codes.
func mapStorageError(err error) (status int, code string) {
	switch {
	case errors.Is(err, storage.ErrModuleNotFound):
		return http.StatusNotFound, registry.ErrCodeModuleNotFound
	case errors.Is(err, storage.ErrVersionNotFound):
		return http.StatusNotFound, registry.ErrCodeVersionNotFound
	case errors.Is(err, storage.ErrVersionExists):
		return http.StatusConflict, registry.ErrCodeVersionExists
	case errors.Is(err, storage.ErrHashMismatch):
		return http.StatusBadRequest, registry.ErrCodeInvalidModule
	default:
		return http.StatusInternalServerError, registry.ErrCodeServerError
	}
}

// writeError writes a JSON error response
func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"code":    code,
		"message": message,
	})
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// isAllowedOrigin checks if the origin is in the allowed list
func (s *Server) isAllowedOrigin(origin string) bool {
	if s.config.CORSOrigins == "" {
		return false // No origins configured = no CORS allowed
	}
	allowedOrigins := strings.Split(s.config.CORSOrigins, ",")
	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" {
			// Explicitly configured wildcard (user accepts the risk)
			return true
		}
		if allowed == origin {
			return true
		}
	}
	return false
}

func main() {
	var config Config

	rootCmd := &cobra.Command{
		Use:   "kscore-registry",
		Short: "Keystone Core module registry server",
		Long: `kscore-registry is a lightweight HTTP server for hosting Keystone Core modules.

It provides a Go-mod style API for module discovery and distribution:
  GET  /<module>/@v/list           - List available versions
  GET  /<module>/@v/<version>.info - Get module metadata
  GET  /<module>/@v/<version>.mod  - Get module manifest
  GET  /<module>/@v/<version>.zip  - Download module
  POST /<module>/@v/<version>      - Publish module (requires auth)
  DELETE /<module>/@v/<version>    - Delete module (requires auth)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(config)
		},
	}

	// General flags
	rootCmd.Flags().StringVar(&config.DataDir, "data", "./data", "Data directory for filesystem storage")
	rootCmd.Flags().StringVar(&config.ListenAddr, "listen", ":8090", "Address to listen on")
	rootCmd.Flags().StringVar(&config.APIKey, "api-key", "", "API key for write operations (or KSCORE_REGISTRY_API_KEY env)")
	rootCmd.Flags().BoolVar(&config.ReadOnly, "readonly", false, "Disable write operations")
	rootCmd.Flags().Int64Var(&config.MaxUploadSize, "max-upload-size", 100<<20, "Maximum upload size in bytes (default 100MB)")
	rootCmd.Flags().BoolVar(&config.EnableCORS, "cors", false, "Enable CORS headers (requires --cors-origins)")
	rootCmd.Flags().StringVar(&config.CORSOrigins, "cors-origins", "", "Comma-separated list of allowed CORS origins (e.g., 'https://example.com,https://app.example.com')")

	// Storage backend flags
	rootCmd.Flags().StringVar(&config.StorageBackend, "storage-backend", "filesystem", "Storage backend type (filesystem, s3, gcs, azure, nats)")

	// S3 flags
	rootCmd.Flags().StringVar(&config.StorageConfig.S3Bucket, "s3-bucket", "", "S3 bucket name")
	rootCmd.Flags().StringVar(&config.StorageConfig.S3Region, "s3-region", "", "S3 region")
	rootCmd.Flags().StringVar(&config.StorageConfig.S3Endpoint, "s3-endpoint", "", "S3-compatible endpoint URL (for MinIO)")
	rootCmd.Flags().StringVar(&config.StorageConfig.S3Prefix, "s3-prefix", "", "S3 key prefix")
	rootCmd.Flags().BoolVar(&config.StorageConfig.S3PathStyle, "s3-path-style", false, "Use S3 path-style URLs")

	// GCS flags
	rootCmd.Flags().StringVar(&config.StorageConfig.GCSBucket, "gcs-bucket", "", "GCS bucket name")
	rootCmd.Flags().StringVar(&config.StorageConfig.GCSProject, "gcs-project", "", "GCP project ID")
	rootCmd.Flags().StringVar(&config.StorageConfig.GCSCredentialsFile, "gcs-credentials-file", "", "Path to GCS service account JSON")
	rootCmd.Flags().StringVar(&config.StorageConfig.GCSPrefix, "gcs-prefix", "", "GCS object prefix")

	// Azure flags
	rootCmd.Flags().StringVar(&config.StorageConfig.AzureContainer, "azure-container", "", "Azure Blob container name")
	rootCmd.Flags().StringVar(&config.StorageConfig.AzureAccountName, "azure-account-name", "", "Azure storage account name")
	rootCmd.Flags().StringVar(&config.StorageConfig.AzureConnectionString, "azure-connection-string", "", "Azure connection string")
	rootCmd.Flags().StringVar(&config.StorageConfig.AzurePrefix, "azure-prefix", "", "Azure blob prefix")

	// NATS flags
	rootCmd.Flags().StringVar(&config.StorageConfig.NATSUrl, "nats-url", "", "NATS server URL")
	rootCmd.Flags().StringVar(&config.StorageConfig.NATSBucket, "nats-bucket", "", "NATS Object Store bucket name")
	rootCmd.Flags().StringVar(&config.StorageConfig.NATSCredentials, "nats-credentials", "", "Path to NATS credentials file")
	rootCmd.Flags().StringVar(&config.StorageConfig.NATSPrefix, "nats-prefix", "", "NATS object prefix")

	// Migrate subcommand
	migrateCmd := &cobra.Command{
		Use:   "migrate-storage",
		Short: "Migrate modules between storage backends",
		Long:  "Migrate all modules from one storage backend to another. Supports dry-run mode.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrate(cmd, config)
		},
	}
	migrateCmd.Flags().String("dest-backend", "", "Destination storage backend type")
	migrateCmd.Flags().String("dest-data", "", "Destination data directory (for filesystem)")
	migrateCmd.Flags().String("dest-s3-bucket", "", "Destination S3 bucket")
	migrateCmd.Flags().String("dest-s3-region", "", "Destination S3 region")
	migrateCmd.Flags().String("dest-s3-endpoint", "", "Destination S3 endpoint")
	migrateCmd.Flags().String("dest-s3-prefix", "", "Destination S3 prefix")
	migrateCmd.Flags().Bool("dry-run", false, "Show what would be migrated without actually doing it")
	rootCmd.AddCommand(migrateCmd)

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kscore-registry version %s\n", version)
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServer(config Config) error {
	// Check for API key from environment
	if config.APIKey == "" {
		config.APIKey = os.Getenv("KSCORE_REGISTRY_API_KEY")
	}

	// Warn if CORS is enabled without configured origins
	if config.EnableCORS && config.CORSOrigins == "" {
		log.Printf("WARNING: CORS enabled but no origins configured. CORS requests will be rejected.")
		log.Printf("  Use --cors-origins to specify allowed origins.")
	}

	// Create storage backend
	config.StorageConfig.DataDir = config.DataDir
	store, err := storage.NewBackend(config.StorageBackend, &config.StorageConfig)
	if err != nil {
		return fmt.Errorf("failed to create storage backend: %w", err)
	}
	defer store.Close()

	handler := NewServer(config, store)

	log.Printf("Starting kscore-registry on %s", config.ListenAddr)
	log.Printf("  Storage backend: %s", config.StorageBackend)
	if config.StorageBackend == "" || config.StorageBackend == "filesystem" || config.StorageBackend == "fs" {
		log.Printf("  Data directory: %s", config.DataDir)
	}
	switch {
	case config.ReadOnly:
		log.Printf("  Mode: read-only")
	case config.APIKey != "":
		log.Printf("  Mode: authenticated write (API key required)")
	default:
		log.Printf("  Mode: open write (no authentication)")
	}

	// Configure server with timeouts to prevent Slowloris attacks
	server := &http.Server{
		Addr:         config.ListenAddr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Allow longer for module downloads
		IdleTimeout:  120 * time.Second,
	}

	return server.ListenAndServe()
}

func runMigrate(cmd *cobra.Command, srcConfig Config) error {
	destBackendType, _ := cmd.Flags().GetString("dest-backend")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if destBackendType == "" {
		return fmt.Errorf("--dest-backend is required")
	}

	// Create source backend
	srcConfig.StorageConfig.DataDir = srcConfig.DataDir
	source, err := storage.NewBackend(srcConfig.StorageBackend, &srcConfig.StorageConfig)
	if err != nil {
		return fmt.Errorf("failed to create source backend: %w", err)
	}
	defer source.Close()

	// Build destination config
	destCfg := &storage.BackendConfig{}
	destCfg.DataDir, _ = cmd.Flags().GetString("dest-data")
	destCfg.S3Bucket, _ = cmd.Flags().GetString("dest-s3-bucket")
	destCfg.S3Region, _ = cmd.Flags().GetString("dest-s3-region")
	destCfg.S3Endpoint, _ = cmd.Flags().GetString("dest-s3-endpoint")
	destCfg.S3Prefix, _ = cmd.Flags().GetString("dest-s3-prefix")

	dest, err := storage.NewBackend(destBackendType, destCfg)
	if err != nil {
		return fmt.Errorf("failed to create destination backend: %w", err)
	}
	defer dest.Close()

	return storage.Migrate(cmd.Context(), source, dest, &storage.MigrateOptions{DryRun: dryRun})
}
