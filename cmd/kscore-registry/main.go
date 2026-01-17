// Package main provides the kscore-registry server for module distribution
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/registry"
	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
	"github.com/shawnbutts/keystone-core/pkg/module/verify"
)

var version = "0.1.0"

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
}

// Server is the module registry HTTP server
type Server struct {
	config Config
	mux    *http.ServeMux
	mu     sync.RWMutex
	hasher verify.HashVerifier
}

// StoredModule represents module metadata stored on disk
type StoredModule struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Hash        string            `json:"hash"`
	PublishedAt time.Time         `json:"published_at"`
	Size        int64             `json:"size"`
	Description string            `json:"description,omitempty"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
	Signature   string            `json:"signature,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	ReleaseNotes string           `json:"release_notes,omitempty"`
}

// NewServer creates a new registry server
func NewServer(config Config) *Server {
	s := &Server{
		config: config,
		mux:    http.NewServeMux(),
		hasher: verify.NewDefaultHashVerifier(),
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	moduleDir := filepath.Join(s.config.DataDir, moduleName)
	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeError(w, http.StatusNotFound, registry.ErrCodeModuleNotFound,
				fmt.Sprintf("Module not found: %s", moduleName))
			return
		}
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to list versions: %v", err))
		return
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Verify it has module.zip
			zipPath := filepath.Join(moduleDir, entry.Name(), "module.zip")
			if _, err := os.Stat(zipPath); err == nil {
				versions = append(versions, entry.Name())
			}
		}
	}

	// Sort versions in descending order (newest first)
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

	w.Header().Set("Content-Type", "text/plain")
	for _, v := range versions {
		fmt.Fprintln(w, v)
	}
}

// handleGetInfo returns module metadata
func (s *Server) handleGetInfo(w http.ResponseWriter, r *http.Request, moduleName, version string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed")
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	stored, err := s.loadStoredModule(moduleName, version)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeError(w, http.StatusNotFound, registry.ErrCodeVersionNotFound,
				fmt.Sprintf("Version not found: %s@%s", moduleName, version))
			return
		}
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to load module info: %v", err))
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	manifestPath := filepath.Join(s.config.DataDir, moduleName, version, "module.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeError(w, http.StatusNotFound, registry.ErrCodeVersionNotFound,
				fmt.Sprintf("Version not found: %s@%s", moduleName, version))
			return
		}
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to read manifest: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.Write(data)
}

// handleDownload returns the module ZIP file
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request, moduleName, version string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed")
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	zipPath := filepath.Join(s.config.DataDir, moduleName, version, "module.zip")
	info, err := os.Stat(zipPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeError(w, http.StatusNotFound, registry.ErrCodeVersionNotFound,
				fmt.Sprintf("Version not found: %s@%s", moduleName, version))
			return
		}
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to stat module: %v", err))
		return
	}

	f, err := os.Open(zipPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to open module: %v", err))
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s-%s.zip",
		strings.ReplaceAll(moduleName, "/", "-"), version))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	io.Copy(w, f)
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
			apiKey = r.Header.Get("Authorization")
			if strings.HasPrefix(apiKey, "Bearer ") {
				apiKey = strings.TrimPrefix(apiKey, "Bearer ")
			}
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

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if version already exists
	versionDir := filepath.Join(s.config.DataDir, moduleName, version)
	if _, err := os.Stat(versionDir); err == nil && !force {
		s.writeError(w, http.StatusConflict, registry.ErrCodeVersionExists,
			fmt.Sprintf("Version already exists: %s@%s", moduleName, version))
		return
	}

	// Create directory
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to create directory: %v", err))
		return
	}

	// Write module file
	zipPath := filepath.Join(versionDir, "module.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to create file: %v", err))
		return
	}

	size, err := io.Copy(zipFile, moduleFile)
	zipFile.Close()
	if err != nil {
		os.Remove(zipPath)
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to write file: %v", err))
		return
	}

	// Compute hash if not provided
	if hash == "" {
		hash, err = s.hasher.ComputeHash(zipPath)
		if err != nil {
			os.RemoveAll(versionDir)
			s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
				fmt.Sprintf("Failed to compute hash: %v", err))
			return
		}
	} else {
		// Verify provided hash
		computedHash, err := s.hasher.ComputeHash(zipPath)
		if err != nil {
			os.RemoveAll(versionDir)
			s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
				fmt.Sprintf("Failed to verify hash: %v", err))
			return
		}
		if computedHash != hash {
			os.RemoveAll(versionDir)
			s.writeError(w, http.StatusBadRequest, registry.ErrCodeInvalidModule,
				fmt.Sprintf("Hash mismatch: expected %s, got %s", hash, computedHash))
			return
		}
	}

	// Write manifest
	manifestPath := filepath.Join(versionDir, "module.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0644); err != nil {
		os.RemoveAll(versionDir)
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to write manifest: %v", err))
		return
	}

	// Write signature if provided
	if signature != "" {
		sigPath := filepath.Join(versionDir, "module.sig")
		if err := os.WriteFile(sigPath, []byte(signature), 0644); err != nil {
			os.RemoveAll(versionDir)
			s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
				fmt.Sprintf("Failed to write signature: %v", err))
			return
		}
	}

	// Write module info
	stored := StoredModule{
		Name:         moduleName,
		Version:      version,
		Hash:         hash,
		PublishedAt:  time.Now(),
		Size:         size,
		Description:  m.Description,
		Dependencies: m.Dependencies,
		Signature:    signature,
		Tags:         tags,
		ReleaseNotes: releaseNotes,
	}

	infoPath := filepath.Join(versionDir, "module.info")
	infoData, _ := json.MarshalIndent(stored, "", "  ")
	if err := os.WriteFile(infoPath, infoData, 0644); err != nil {
		os.RemoveAll(versionDir)
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to write info: %v", err))
		return
	}

	// Return result
	result := registry.PublishResult{
		ModuleName:  moduleName,
		Version:     version,
		Hash:        hash,
		URL:         fmt.Sprintf("/%s/@v/%s.zip", moduleName, version),
		PublishedAt: stored.PublishedAt,
		Size:        size,
	}

	// Log upload
	log.Printf("Published %s@%s (%d bytes, hash: %s) by %s",
		moduleName, version, size, hash[:16]+"...", r.RemoteAddr)

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
			apiKey = r.Header.Get("Authorization")
			if strings.HasPrefix(apiKey, "Bearer ") {
				apiKey = strings.TrimPrefix(apiKey, "Bearer ")
			}
		}
		if apiKey != s.config.APIKey {
			s.writeError(w, http.StatusUnauthorized, registry.ErrCodeUnauthorized, "Invalid API key")
			return
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	versionDir := filepath.Join(s.config.DataDir, moduleName, version)
	if _, err := os.Stat(versionDir); os.IsNotExist(err) {
		s.writeError(w, http.StatusNotFound, registry.ErrCodeVersionNotFound,
			fmt.Sprintf("Version not found: %s@%s", moduleName, version))
		return
	}

	if err := os.RemoveAll(versionDir); err != nil {
		s.writeError(w, http.StatusInternalServerError, registry.ErrCodeServerError,
			fmt.Sprintf("Failed to delete: %v", err))
		return
	}

	// Clean up empty module directory
	moduleDir := filepath.Join(s.config.DataDir, moduleName)
	entries, _ := os.ReadDir(moduleDir)
	if len(entries) == 0 {
		os.RemoveAll(moduleDir)
	}

	log.Printf("Deleted %s@%s by %s", moduleName, version, r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

// loadStoredModule loads module metadata from disk
func (s *Server) loadStoredModule(moduleName, version string) (*StoredModule, error) {
	infoPath := filepath.Join(s.config.DataDir, moduleName, version, "module.info")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, err
	}

	var stored StoredModule
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}

	return &stored, nil
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

	rootCmd.Flags().StringVar(&config.DataDir, "data", "./data", "Data directory for storing modules")
	rootCmd.Flags().StringVar(&config.ListenAddr, "listen", ":8090", "Address to listen on")
	rootCmd.Flags().StringVar(&config.APIKey, "api-key", "", "API key for write operations (or KSCORE_REGISTRY_API_KEY env)")
	rootCmd.Flags().BoolVar(&config.ReadOnly, "readonly", false, "Disable write operations")
	rootCmd.Flags().Int64Var(&config.MaxUploadSize, "max-upload-size", 100<<20, "Maximum upload size in bytes (default 100MB)")
	rootCmd.Flags().BoolVar(&config.EnableCORS, "cors", false, "Enable CORS headers (requires --cors-origins)")
	rootCmd.Flags().StringVar(&config.CORSOrigins, "cors-origins", "", "Comma-separated list of allowed CORS origins (e.g., 'https://example.com,https://app.example.com')")

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

	// Create data directory
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	handler := NewServer(config)

	log.Printf("Starting kscore-registry on %s", config.ListenAddr)
	log.Printf("  Data directory: %s", config.DataDir)
	if config.ReadOnly {
		log.Printf("  Mode: read-only")
	} else if config.APIKey != "" {
		log.Printf("  Mode: authenticated write (API key required)")
	} else {
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
