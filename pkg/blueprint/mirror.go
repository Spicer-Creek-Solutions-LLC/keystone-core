package blueprint

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MirrorConfig configures a blueprint mirror server
type MirrorConfig struct {
	// StorageDir is the directory for storing blueprints
	StorageDir string `json:"storage_dir" yaml:"storage_dir"`

	// ListenAddr is the address to listen on (e.g., ":8080")
	ListenAddr string `json:"listen_addr" yaml:"listen_addr"`

	// UpstreamURL is the upstream registry to sync from (optional)
	UpstreamURL string `json:"upstream_url,omitempty" yaml:"upstream_url,omitempty"`

	// SyncInterval is how often to sync from upstream
	SyncInterval time.Duration `json:"sync_interval,omitempty" yaml:"sync_interval,omitempty"`

	// AllowPush allows pushing blueprints to the mirror
	AllowPush bool `json:"allow_push" yaml:"allow_push"`

	// TrustedKeys for signature verification
	TrustedKeys []string `json:"trusted_keys,omitempty" yaml:"trusted_keys,omitempty"`

	// RequireSignatures requires all blueprints to be signed
	RequireSignatures bool `json:"require_signatures" yaml:"require_signatures"`
}

// MirrorServer serves blueprints from a local mirror
type MirrorServer struct {
	config  *MirrorConfig
	storage Storage
	loader  *Loader
	mu      sync.RWMutex

	// Index of available blueprints
	index map[string][]string // name -> versions
}

// NewMirrorServer creates a new mirror server
func NewMirrorServer(config *MirrorConfig) (*MirrorServer, error) {
	if config.StorageDir == "" {
		return nil, fmt.Errorf("storage_dir is required")
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(config.StorageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	storage, err := NewLocalStorage(config.StorageDir, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}
	loader := NewLoader(storage)

	server := &MirrorServer{
		config:  config,
		storage: storage,
		loader:  loader,
		index:   make(map[string][]string),
	}

	// Build initial index
	if err := server.rebuildIndex(); err != nil {
		return nil, fmt.Errorf("failed to build index: %w", err)
	}

	return server, nil
}

// rebuildIndex scans the storage directory and builds the index
func (m *MirrorServer) rebuildIndex() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.index = make(map[string][]string)

	// Scan storage directory
	entries, err := os.ReadDir(m.config.StorageDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Vendor directory
		vendor := entry.Name()
		vendorPath := filepath.Join(m.config.StorageDir, vendor)

		blueprints, err := os.ReadDir(vendorPath)
		if err != nil {
			continue
		}

		for _, bp := range blueprints {
			if !bp.IsDir() {
				continue
			}

			name := vendor + "/" + bp.Name()
			bpPath := filepath.Join(vendorPath, bp.Name())

			// Get versions
			versions, err := os.ReadDir(bpPath)
			if err != nil {
				continue
			}

			for _, ver := range versions {
				if !ver.IsDir() {
					continue
				}
				m.index[name] = append(m.index[name], ver.Name())
			}

			// Sort versions
			sort.Strings(m.index[name])
		}
	}

	return nil
}

// Handler returns an HTTP handler for the mirror server
func (m *MirrorServer) Handler() http.Handler {
	mux := http.NewServeMux()

	// List all blueprints
	mux.HandleFunc("/v1/blueprints", m.handleListBlueprints)

	// Get blueprint info
	mux.HandleFunc("/v1/blueprints/", m.handleBlueprintRequest)

	// Get/Put bundle
	mux.HandleFunc("/v1/bundles/", m.handleBundleRequest)

	// Health check
	mux.HandleFunc("/health", m.handleHealth)

	// Index info
	mux.HandleFunc("/v1/index", m.handleIndex)

	return mux
}

// handleListBlueprints lists all available blueprints
func (m *MirrorServer) handleListBlueprints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	type blueprintInfo struct {
		Name     string   `json:"name"`
		Versions []string `json:"versions"`
	}

	var blueprints []blueprintInfo
	for name, versions := range m.index {
		blueprints = append(blueprints, blueprintInfo{
			Name:     name,
			Versions: versions,
		})
	}

	// Sort by name
	sort.Slice(blueprints, func(i, j int) bool {
		return blueprints[i].Name < blueprints[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(blueprints)
}

// handleBlueprintRequest handles requests for specific blueprints
func (m *MirrorServer) handleBlueprintRequest(w http.ResponseWriter, r *http.Request) {
	// Parse path: /v1/blueprints/{vendor}/{name}[@{version}]
	path := strings.TrimPrefix(r.URL.Path, "/v1/blueprints/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		http.Error(w, "Invalid blueprint path", http.StatusBadRequest)
		return
	}

	vendor := parts[0]
	nameAndVersion := parts[1]

	// Parse name and version
	name := vendor + "/" + nameAndVersion
	version := ""

	if len(parts) >= 3 {
		version = parts[2]
	} else if strings.Contains(nameAndVersion, "@") {
		parts := strings.SplitN(nameAndVersion, "@", 2)
		name = vendor + "/" + parts[0]
		version = parts[1]
	}

	switch r.Method {
	case http.MethodGet:
		m.handleGetBlueprint(w, r, name, version)
	case http.MethodPut:
		if !m.config.AllowPush {
			http.Error(w, "Push not allowed", http.StatusForbidden)
			return
		}
		m.handlePutBlueprint(w, r, name, version)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetBlueprint returns blueprint info or content
func (m *MirrorServer) handleGetBlueprint(w http.ResponseWriter, r *http.Request, name, version string) {
	m.mu.RLock()
	versions, exists := m.index[name]
	m.mu.RUnlock()

	if !exists {
		http.Error(w, "Blueprint not found", http.StatusNotFound)
		return
	}

	// If no version specified, return version list
	if version == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":     name,
			"versions": versions,
		})
		return
	}

	// Check version exists
	found := false
	for _, v := range versions {
		if v == version {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "Version not found", http.StatusNotFound)
		return
	}

	// Return blueprint content
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid blueprint name", http.StatusBadRequest)
		return
	}

	bpPath := filepath.Join(m.config.StorageDir, parts[0], parts[1], version, "blueprint.yaml")

	data, err := os.ReadFile(bpPath)
	if err != nil {
		http.Error(w, "Failed to read blueprint", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.Write(data)
}

// handlePutBlueprint stores a blueprint
func (m *MirrorServer) handlePutBlueprint(w http.ResponseWriter, r *http.Request, name, version string) {
	if version == "" {
		http.Error(w, "Version required", http.StatusBadRequest)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Create blueprint directory
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid blueprint name", http.StatusBadRequest)
		return
	}

	bpDir := filepath.Join(m.config.StorageDir, parts[0], parts[1], version)
	if err := os.MkdirAll(bpDir, 0755); err != nil {
		http.Error(w, "Failed to create directory", http.StatusInternalServerError)
		return
	}

	// Write blueprint
	bpPath := filepath.Join(bpDir, "blueprint.yaml")
	if err := os.WriteFile(bpPath, body, 0644); err != nil {
		http.Error(w, "Failed to write blueprint", http.StatusInternalServerError)
		return
	}

	// Rebuild index
	m.rebuildIndex()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "created",
		"name":    name,
		"version": version,
	})
}

// handleBundleRequest handles bundle upload/download
func (m *MirrorServer) handleBundleRequest(w http.ResponseWriter, r *http.Request) {
	// Parse path: /v1/bundles/{vendor}/{name}/{version}
	path := strings.TrimPrefix(r.URL.Path, "/v1/bundles/")
	parts := strings.Split(path, "/")

	if len(parts) < 3 {
		http.Error(w, "Invalid bundle path", http.StatusBadRequest)
		return
	}

	vendor := parts[0]
	name := parts[1]
	version := parts[2]
	fullName := vendor + "/" + name

	switch r.Method {
	case http.MethodGet:
		m.handleGetBundle(w, r, fullName, version)
	case http.MethodPut:
		if !m.config.AllowPush {
			http.Error(w, "Push not allowed", http.StatusForbidden)
			return
		}
		m.handlePutBundle(w, r, fullName, version)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetBundle returns a bundle file
func (m *MirrorServer) handleGetBundle(w http.ResponseWriter, r *http.Request, name, version string) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid blueprint name", http.StatusBadRequest)
		return
	}

	bundlePath := filepath.Join(m.config.StorageDir, "bundles", parts[0], parts[1], version+".tar.gz")

	file, err := os.Open(bundlePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Bundle not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to read bundle", http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.Error(w, "Failed to get bundle info", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s-%s-bundle.tar.gz", parts[1], version))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	io.Copy(w, file)
}

// handlePutBundle stores a bundle file
func (m *MirrorServer) handlePutBundle(w http.ResponseWriter, r *http.Request, name, version string) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid blueprint name", http.StatusBadRequest)
		return
	}

	// Verify signature if required
	if m.config.RequireSignatures {
		// Create temp file for verification
		tempFile, err := os.CreateTemp("", "bundle-verify-*")
		if err != nil {
			http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
			return
		}
		tempPath := tempFile.Name()
		defer os.Remove(tempPath)

		// Copy body to temp file
		if _, err := io.Copy(tempFile, r.Body); err != nil {
			tempFile.Close()
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		tempFile.Close()

		// Verify signature
		if _, err := VerifyBundleSignature(tempPath, m.config.TrustedKeys); err != nil {
			http.Error(w, fmt.Sprintf("Signature verification failed: %v", err), http.StatusBadRequest)
			return
		}

		// Read verified file for storage
		r.Body, _ = os.Open(tempPath)
	}

	// Create bundle directory
	bundleDir := filepath.Join(m.config.StorageDir, "bundles", parts[0], parts[1])
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		http.Error(w, "Failed to create directory", http.StatusInternalServerError)
		return
	}

	// Write bundle
	bundlePath := filepath.Join(bundleDir, version+".tar.gz")
	file, err := os.Create(bundlePath)
	if err != nil {
		http.Error(w, "Failed to create bundle file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	if _, err := io.Copy(file, r.Body); err != nil {
		http.Error(w, "Failed to write bundle", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "created",
		"name":    name,
		"version": version,
	})
}

// handleHealth returns server health status
func (m *MirrorServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().UTC(),
	})
}

// handleIndex returns the full index
func (m *MirrorServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m.index)
}

// Start starts the mirror server
func (m *MirrorServer) Start() error {
	if m.config.ListenAddr == "" {
		return fmt.Errorf("listen_addr is required")
	}

	return http.ListenAndServe(m.config.ListenAddr, m.Handler())
}

// MirrorClient connects to a mirror server
type MirrorClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewMirrorClient creates a new mirror client
func NewMirrorClient(baseURL string) *MirrorClient {
	return &MirrorClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListBlueprints returns all blueprints in the mirror
func (c *MirrorClient) ListBlueprints() ([]BlueprintInfo, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/v1/blueprints")
	if err != nil {
		return nil, fmt.Errorf("failed to list blueprints: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var blueprints []BlueprintInfo
	if err := json.NewDecoder(resp.Body).Decode(&blueprints); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return blueprints, nil
}

// GetVersions returns all versions of a blueprint
func (c *MirrorClient) GetVersions(name string) ([]string, error) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid blueprint name: %s", name)
	}

	resp, err := c.httpClient.Get(fmt.Sprintf("%s/v1/blueprints/%s/%s", c.baseURL, parts[0], parts[1]))
	if err != nil {
		return nil, fmt.Errorf("failed to get versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("blueprint not found: %s", name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var result struct {
		Versions []string `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Versions, nil
}

// GetBlueprint downloads a specific blueprint version
func (c *MirrorClient) GetBlueprint(name, version string) ([]byte, error) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid blueprint name: %s", name)
	}

	url := fmt.Sprintf("%s/v1/blueprints/%s/%s/%s", c.baseURL, parts[0], parts[1], version)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get blueprint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("blueprint not found: %s@%s", name, version)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// DownloadBundle downloads a bundle file
func (c *MirrorClient) DownloadBundle(name, version, destPath string) error {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid blueprint name: %s", name)
	}

	url := fmt.Sprintf("%s/v1/bundles/%s/%s/%s", c.baseURL, parts[0], parts[1], version)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download bundle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("bundle not found: %s@%s", name, version)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	// Create destination file
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// UploadBundle uploads a bundle to the mirror
func (c *MirrorClient) UploadBundle(bundlePath, name, version string) error {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid blueprint name: %s", name)
	}

	file, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to open bundle: %w", err)
	}
	defer file.Close()

	url := fmt.Sprintf("%s/v1/bundles/%s/%s/%s", c.baseURL, parts[0], parts[1], version)
	req, err := http.NewRequest(http.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload bundle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// UploadBlueprint uploads a blueprint to the mirror
func (c *MirrorClient) UploadBlueprint(content []byte, name, version string) error {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid blueprint name: %s", name)
	}

	url := fmt.Sprintf("%s/v1/blueprints/%s/%s/%s", c.baseURL, parts[0], parts[1], version)
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(string(content)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-yaml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload blueprint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("push not allowed on this mirror")
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// MirrorSyncer syncs blueprints between mirrors
type MirrorSyncer struct {
	source *MirrorClient
	dest   *MirrorClient
}

// NewMirrorSyncer creates a new mirror syncer
func NewMirrorSyncer(sourceURL, destURL string) *MirrorSyncer {
	return &MirrorSyncer{
		source: NewMirrorClient(sourceURL),
		dest:   NewMirrorClient(destURL),
	}
}

// SyncAll syncs all blueprints from source to dest
func (s *MirrorSyncer) SyncAll() (*SyncResult, error) {
	result := &SyncResult{
		StartedAt: time.Now(),
	}

	// Get source blueprints
	blueprints, err := s.source.ListBlueprints()
	if err != nil {
		return nil, fmt.Errorf("failed to list source blueprints: %w", err)
	}

	// Get destination blueprints
	destBlueprints, err := s.dest.ListBlueprints()
	if err != nil {
		// If dest is empty, that's ok
		destBlueprints = nil
	}

	// Build dest index
	destIndex := make(map[string]map[string]bool)
	for _, bp := range destBlueprints {
		destIndex[bp.Name] = make(map[string]bool)
		for _, v := range bp.AvailableVersions {
			destIndex[bp.Name][v] = true
		}
	}

	// Sync each blueprint
	for _, bp := range blueprints {
		for _, version := range bp.AvailableVersions {
			// Skip if already exists
			if destIndex[bp.Name] != nil && destIndex[bp.Name][version] {
				result.Skipped++
				continue
			}

			// Download from source
			content, err := s.source.GetBlueprint(bp.Name, version)
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s@%s: %v", bp.Name, version, err))
				continue
			}

			// Upload to dest
			if err := s.dest.UploadBlueprint(content, bp.Name, version); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s@%s: %v", bp.Name, version, err))
				continue
			}

			result.Synced++
		}
	}

	result.CompletedAt = time.Now()
	return result, nil
}

// SyncBlueprint syncs a specific blueprint
func (s *MirrorSyncer) SyncBlueprint(name string) (*SyncResult, error) {
	result := &SyncResult{
		StartedAt: time.Now(),
	}

	// Get versions from source
	versions, err := s.source.GetVersions(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get source versions: %w", err)
	}

	// Get versions from dest
	destVersions, _ := s.dest.GetVersions(name)
	destIndex := make(map[string]bool)
	for _, v := range destVersions {
		destIndex[v] = true
	}

	// Sync each version
	for _, version := range versions {
		if destIndex[version] {
			result.Skipped++
			continue
		}

		content, err := s.source.GetBlueprint(name, version)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", version, err))
			continue
		}

		if err := s.dest.UploadBlueprint(content, name, version); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", version, err))
			continue
		}

		result.Synced++
	}

	result.CompletedAt = time.Now()
	return result, nil
}

// SyncResult contains results from a sync operation
type SyncResult struct {
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Synced      int       `json:"synced"`
	Skipped     int       `json:"skipped"`
	Failed      int       `json:"failed"`
	Errors      []string  `json:"errors,omitempty"`
}

// ExportToDirectory exports all blueprints to a directory for offline use
func (c *MirrorClient) ExportToDirectory(destDir string) error {
	// Get all blueprints
	blueprints, err := c.ListBlueprints()
	if err != nil {
		return fmt.Errorf("failed to list blueprints: %w", err)
	}

	// Download each blueprint
	for _, bp := range blueprints {
		for _, version := range bp.AvailableVersions {
			content, err := c.GetBlueprint(bp.Name, version)
			if err != nil {
				return fmt.Errorf("failed to get %s@%s: %w", bp.Name, version, err)
			}

			// Create directory structure
			parts := strings.SplitN(bp.Name, "/", 2)
			if len(parts) != 2 {
				continue
			}

			bpDir := filepath.Join(destDir, parts[0], parts[1], version)
			if err := os.MkdirAll(bpDir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			// Write blueprint
			bpPath := filepath.Join(bpDir, "blueprint.yaml")
			if err := os.WriteFile(bpPath, content, 0644); err != nil {
				return fmt.Errorf("failed to write blueprint: %w", err)
			}
		}
	}

	return nil
}

// ImportFromDirectory imports blueprints from a directory
func (c *MirrorClient) ImportFromDirectory(srcDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Name() != "blueprint.yaml" {
			return nil
		}

		// Extract name and version from path
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		parts := strings.Split(filepath.Dir(rel), string(os.PathSeparator))
		if len(parts) < 3 {
			return nil
		}

		vendor := parts[0]
		name := parts[1]
		version := parts[2]
		fullName := vendor + "/" + name

		// Read and upload
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		if err := c.UploadBlueprint(content, fullName, version); err != nil {
			return fmt.Errorf("failed to upload %s@%s: %w", fullName, version, err)
		}

		return nil
	})
}
