package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPSumDBClient implements SumDBClient using HTTP
type HTTPSumDBClient struct {
	baseURL    string
	httpClient *http.Client
	cache      map[string]sumDBEntry
	cacheMu    sync.RWMutex
}

// sumDBEntry represents a cached entry from the SumDB
type sumDBEntry struct {
	Hash      string
	Timestamp time.Time
}

// NewHTTPSumDBClient creates a new HTTP-based SumDB client
func NewHTTPSumDBClient(baseURL string) *HTTPSumDBClient {
	return &HTTPSumDBClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: make(map[string]sumDBEntry),
	}
}

// Lookup retrieves the hash for a module from the SumDB
func (c *HTTPSumDBClient) Lookup(moduleName, version string) (string, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s@%s", moduleName, version)
	c.cacheMu.RLock()
	if entry, exists := c.cache[cacheKey]; exists {
		c.cacheMu.RUnlock()
		return entry.Hash, nil
	}
	c.cacheMu.RUnlock()

	// Build lookup URL
	// Format: /lookup/{module}@{version}
	lookupPath := fmt.Sprintf("/lookup/%s@%s", url.PathEscape(moduleName), url.PathEscape(version))
	lookupURL := c.baseURL + lookupPath

	// Make HTTP request
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, lookupURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSumDBUnavailable, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSumDBUnavailable, err)
	}
	defer resp.Body.Close()

	// Handle response
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("%w: module not found in SumDB", ErrSumDBVerificationFailed)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: HTTP %d", ErrSumDBUnavailable, resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response (simplified - assumes JSON response)
	var response struct {
		Module  string `json:"module"`
		Version string `json:"version"`
		Hash    string `json:"hash"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		// Try plain text format (hash on single line)
		hash := strings.TrimSpace(string(body))
		if hash != "" {
			// Cache the result
			c.cacheMu.Lock()
			c.cache[cacheKey] = sumDBEntry{
				Hash:      hash,
				Timestamp: time.Now(),
			}
			c.cacheMu.Unlock()

			return hash, nil
		}
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Validate response
	if response.Module != moduleName || response.Version != version {
		return "", fmt.Errorf("%w: response mismatch", ErrSumDBVerificationFailed)
	}

	// Cache the result
	c.cacheMu.Lock()
	c.cache[cacheKey] = sumDBEntry{
		Hash:      response.Hash,
		Timestamp: time.Now(),
	}
	c.cacheMu.Unlock()

	return response.Hash, nil
}

// Verify verifies a module against the SumDB
func (c *HTTPSumDBClient) Verify(moduleName, version, hash string) (bool, error) {
	// Lookup the expected hash from SumDB
	expectedHash, err := c.Lookup(moduleName, version)
	if err != nil {
		return false, err
	}

	// Normalize hashes for comparison
	expectedHash = normalizeHash(expectedHash)
	actualHash := normalizeHash(hash)

	// Compare hashes
	if expectedHash != actualHash {
		return false, nil
	}

	return true, nil
}

// Submit submits a new module hash to the SumDB
func (c *HTTPSumDBClient) Submit(moduleName, version, hash string) error {
	// Build submit URL
	submitPath := "/submit"
	submitURL := c.baseURL + submitPath

	// Prepare submission data
	data := map[string]string{
		"module":  moduleName,
		"version": version,
		"hash":    hash,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Make HTTP POST request
	req, err := http.NewRequestWithContext(context.Background(), "POST", submitURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSumDBUnavailable, err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: HTTP %d: %s", ErrSumDBUnavailable, resp.StatusCode, string(body))
	}

	return nil
}

// ClearCache clears the lookup cache
func (c *HTTPSumDBClient) ClearCache() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.cache = make(map[string]sumDBEntry)
}

// InMemorySumDB is an in-memory implementation of SumDB for testing
type InMemorySumDB struct {
	entries map[string]string
	mu      sync.RWMutex
}

// NewInMemorySumDB creates a new in-memory SumDB
func NewInMemorySumDB() *InMemorySumDB {
	return &InMemorySumDB{
		entries: make(map[string]string),
	}
}

// Lookup retrieves the hash for a module
func (db *InMemorySumDB) Lookup(moduleName, version string) (string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	key := fmt.Sprintf("%s@%s", moduleName, version)
	hash, exists := db.entries[key]
	if !exists {
		return "", fmt.Errorf("%w: module not found", ErrSumDBVerificationFailed)
	}

	return hash, nil
}

// Verify verifies a module hash
func (db *InMemorySumDB) Verify(moduleName, version, hash string) (bool, error) {
	expectedHash, err := db.Lookup(moduleName, version)
	if err != nil {
		return false, err
	}

	// Normalize hashes
	expectedHash = normalizeHash(expectedHash)
	actualHash := normalizeHash(hash)

	return expectedHash == actualHash, nil
}

// Submit adds a new module hash
func (db *InMemorySumDB) Submit(moduleName, version, hash string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	key := fmt.Sprintf("%s@%s", moduleName, version)

	// Check if already exists with different hash
	if existingHash, exists := db.entries[key]; exists {
		if normalizeHash(existingHash) != normalizeHash(hash) {
			return fmt.Errorf("%w: hash mismatch for existing entry", ErrSumDBVerificationFailed)
		}
		// Same hash, no-op
		return nil
	}

	db.entries[key] = hash
	return nil
}

// Record records a module hash (alias for Submit to satisfy SumDBClient interface)
func (db *InMemorySumDB) Record(moduleName, version, hash string) error {
	return db.Submit(moduleName, version, hash)
}

// Clear removes all entries
func (db *InMemorySumDB) Clear() {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.entries = make(map[string]string)
}

// Count returns the number of entries
func (db *InMemorySumDB) Count() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.entries)
}
