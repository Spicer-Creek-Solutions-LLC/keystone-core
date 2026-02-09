package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// PluginPrefix is the required prefix for all Keystone Core plugins
	PluginPrefix = "kscore-"
)

// Plugin represents a discovered Keystone Core plugin
type Plugin struct {
	Name string // Plugin name without prefix (e.g., "exec" for "kscore-exec")
	Path string // Full path to the plugin binary
}

// Discovery manages plugin discovery and caching
type Discovery struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin // Key is plugin name (without prefix)
	cached  bool
}

// NewDiscovery creates a new plugin discovery manager
func NewDiscovery() *Discovery {
	return &Discovery{
		plugins: make(map[string]*Plugin),
	}
}

// Discover scans the PATH for kscore-* binaries and caches them
func (d *Discovery) Discover() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Clear existing cache
	d.plugins = make(map[string]*Plugin)

	// Get PATH environment variable
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return fmt.Errorf("PATH environment variable not set")
	}

	// Split PATH into directories
	pathDirs := filepath.SplitList(pathEnv)

	// Scan each directory for kscore-* binaries
	for _, dir := range pathDirs {
		if dir == "" {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			// Skip directories we can't read
			continue
		}

		for _, entry := range entries {
			// Skip directories
			if entry.IsDir() {
				continue
			}

			name := entry.Name()

			// Check if it starts with our prefix
			if !strings.HasPrefix(name, PluginPrefix) {
				continue
			}

			// Get full path
			fullPath := filepath.Join(dir, name)

			// Validate executability
			if !isExecutable(fullPath) {
				continue
			}

			// Extract plugin name (remove prefix)
			pluginName := strings.TrimPrefix(name, PluginPrefix)

			// Only keep first occurrence (earlier in PATH wins)
			if _, exists := d.plugins[pluginName]; !exists {
				d.plugins[pluginName] = &Plugin{
					Name: pluginName,
					Path: fullPath,
				}
			}
		}
	}

	d.cached = true
	return nil
}

// Get retrieves a plugin by name (without prefix)
func (d *Discovery) Get(name string) (*Plugin, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.cached {
		return nil, fmt.Errorf("plugins not discovered yet, call Discover() first")
	}

	plugin, exists := d.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %q not found", name)
	}

	return plugin, nil
}

// List returns all discovered plugins
func (d *Discovery) List() []*Plugin {
	d.mu.RLock()
	defer d.mu.RUnlock()

	plugins := make([]*Plugin, 0, len(d.plugins))
	for _, plugin := range d.plugins {
		plugins = append(plugins, plugin)
	}

	return plugins
}

// Has checks if a plugin exists
func (d *Discovery) Has(name string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, exists := d.plugins[name]
	return exists
}

// isExecutable checks if a file is executable
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return false
	}

	// Check if it has execute permission (Unix-like systems)
	// On Windows, any file with .exe extension is considered executable
	if info.Mode().Perm()&0o111 != 0 {
		return true
	}

	// Additional check: try to resolve it as an executable
	_, err = exec.LookPath(path)
	return err == nil
}
