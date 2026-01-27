package handlers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestNewPluginHandler(t *testing.T) {
	h := NewPluginHandler("test-plugin", "/path/to/plugin", runbook.StepType("custom"))

	if h.name != "test-plugin" {
		t.Errorf("Expected name 'test-plugin', got %s", h.name)
	}

	if h.path != "/path/to/plugin" {
		t.Errorf("Expected path '/path/to/plugin', got %s", h.path)
	}

	if h.Type() != runbook.StepType("custom") {
		t.Errorf("Expected step type 'custom', got %s", h.Type())
	}
}

func TestPluginHandler_Validate(t *testing.T) {
	h := NewPluginHandler("test", "/path/to/plugin", runbook.StepType("custom"))

	t.Run("no config spec - always valid", func(t *testing.T) {
		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepType("custom"),
			Config: map[string]interface{}{},
		}

		if err := h.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("with config spec - required field missing", func(t *testing.T) {
		h.SetConfigSpec(map[string]PluginConfigSpec{
			"required_field": {
				Name:     "required_field",
				Type:     "string",
				Required: true,
			},
		})

		step := &runbook.Step{
			Name:   "test",
			Type:   runbook.StepType("custom"),
			Config: map[string]interface{}{},
		}

		if err := h.Validate(step); err == nil {
			t.Error("Expected error for missing required field")
		}
	})

	t.Run("with config spec - required field present", func(t *testing.T) {
		h.SetConfigSpec(map[string]PluginConfigSpec{
			"required_field": {
				Name:     "required_field",
				Type:     "string",
				Required: true,
			},
		})

		step := &runbook.Step{
			Name: "test",
			Type: runbook.StepType("custom"),
			Config: map[string]interface{}{
				"required_field": "value",
			},
		}

		if err := h.Validate(step); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})
}

func TestNewPluginRegistry(t *testing.T) {
	registry := NewRegistry()
	pluginRegistry := NewPluginRegistry(registry)

	if pluginRegistry == nil {
		t.Fatal("NewPluginRegistry() returned nil")
	}

	if pluginRegistry.registry != registry {
		t.Error("Plugin registry not linked to main registry")
	}

	if len(pluginRegistry.paths) == 0 {
		t.Error("Expected default plugin paths")
	}
}

func TestPluginRegistry_AddPluginPath(t *testing.T) {
	pluginRegistry := NewPluginRegistry(nil)
	initialCount := len(pluginRegistry.paths)

	pluginRegistry.AddPluginPath("/custom/path")

	if len(pluginRegistry.paths) != initialCount+1 {
		t.Errorf("Expected %d paths, got %d", initialCount+1, len(pluginRegistry.paths))
	}

	found := false
	for _, p := range pluginRegistry.paths {
		if p == "/custom/path" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Custom path not added")
	}
}

func TestPluginRegistry_RegisterPlugin(t *testing.T) {
	pluginRegistry := NewPluginRegistry(nil)

	t.Run("register new plugin", func(t *testing.T) {
		plugin := NewPluginHandler("test", "/path/to/plugin", runbook.StepType("custom"))

		if err := pluginRegistry.RegisterPlugin(plugin); err != nil {
			t.Errorf("RegisterPlugin() error = %v", err)
		}

		registered, ok := pluginRegistry.GetPlugin(runbook.StepType("custom"))
		if !ok {
			t.Error("Plugin not found after registration")
		}

		if registered.name != "test" {
			t.Errorf("Expected plugin name 'test', got %s", registered.name)
		}
	})

	t.Run("duplicate registration", func(t *testing.T) {
		plugin := NewPluginHandler("test2", "/path/to/plugin2", runbook.StepType("custom"))

		if err := pluginRegistry.RegisterPlugin(plugin); err == nil {
			t.Error("Expected error for duplicate registration")
		}
	})
}

func TestPluginRegistry_ListPlugins(t *testing.T) {
	pluginRegistry := NewPluginRegistry(nil)

	plugin1 := NewPluginHandler("plugin1", "/path/to/plugin1", runbook.StepType("type1"))
	plugin2 := NewPluginHandler("plugin2", "/path/to/plugin2", runbook.StepType("type2"))

	pluginRegistry.RegisterPlugin(plugin1)
	pluginRegistry.RegisterPlugin(plugin2)

	plugins := pluginRegistry.ListPlugins()

	if len(plugins) != 2 {
		t.Errorf("Expected 2 plugins, got %d", len(plugins))
	}
}

func TestPluginRegistry_DiscoverPlugins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix-specific test on Windows")
	}

	// Create a temp directory with a mock plugin
	tempDir := t.TempDir()
	pluginPath := filepath.Join(tempDir, "kscore-runbook-testplugin")

	// Create a mock plugin script
	f, err := os.Create(pluginPath)
	if err != nil {
		t.Fatalf("Failed to create mock plugin: %v", err)
	}
	f.WriteString("#!/bin/sh\necho 'mock plugin'")
	f.Close()
	os.Chmod(pluginPath, 0755)

	// Test discovery
	pluginRegistry := NewPluginRegistry(nil)
	pluginRegistry.AddPluginPath(tempDir)

	if err := pluginRegistry.DiscoverPlugins(); err != nil {
		t.Errorf("DiscoverPlugins() error = %v", err)
	}

	plugin, ok := pluginRegistry.GetPlugin(runbook.StepType("testplugin"))
	if !ok {
		t.Error("Plugin 'testplugin' not discovered")
	} else {
		if plugin.path != pluginPath {
			t.Errorf("Expected path %s, got %s", pluginPath, plugin.path)
		}
	}
}

func TestPluginRegistry_DiscoverPlugins_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	pluginRegistry := NewPluginRegistry(nil)
	pluginRegistry.AddPluginPath(tempDir)

	// Should not error on empty directory
	if err := pluginRegistry.DiscoverPlugins(); err != nil {
		t.Errorf("DiscoverPlugins() error = %v", err)
	}

	plugins := pluginRegistry.ListPlugins()
	if len(plugins) != 0 {
		t.Errorf("Expected 0 plugins, got %d", len(plugins))
	}
}

func TestPluginRegistry_DiscoverPlugins_NonexistentPath(t *testing.T) {
	pluginRegistry := NewPluginRegistry(nil)
	pluginRegistry.AddPluginPath("/nonexistent/path")

	// Should not error on nonexistent path
	if err := pluginRegistry.DiscoverPlugins(); err != nil {
		t.Errorf("DiscoverPlugins() error = %v", err)
	}
}

func TestPluginRegistry_WithMainRegistry(t *testing.T) {
	mainRegistry := NewRegistry()
	pluginRegistry := NewPluginRegistry(mainRegistry)

	plugin := NewPluginHandler("test", "/path/to/plugin", runbook.StepType("custom"))
	if err := pluginRegistry.RegisterPlugin(plugin); err != nil {
		t.Errorf("RegisterPlugin() error = %v", err)
	}

	// Check that it's also in the main registry
	handler, ok := mainRegistry.Get(runbook.StepType("custom"))
	if !ok {
		t.Error("Plugin not registered in main registry")
	}

	if handler.Type() != runbook.StepType("custom") {
		t.Errorf("Expected step type 'custom', got %s", handler.Type())
	}
}

func TestGetDefaultPluginPaths(t *testing.T) {
	paths := getDefaultPluginPaths()

	if len(paths) == 0 {
		t.Error("Expected at least one default path")
	}

	// Check standard paths are present
	standardPaths := []string{
		"/usr/local/lib/keystone/runbook-plugins",
		"/usr/lib/keystone/runbook-plugins",
	}

	for _, expected := range standardPaths {
		found := false
		for _, p := range paths {
			if p == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected standard path %s not found", expected)
		}
	}
}

func TestGetDefaultPluginPaths_WithEnv(t *testing.T) {
	// Set custom path in environment
	oldPath := os.Getenv("KEYSTONE_RUNBOOK_PLUGIN_PATH")
	defer os.Setenv("KEYSTONE_RUNBOOK_PLUGIN_PATH", oldPath)

	os.Setenv("KEYSTONE_RUNBOOK_PLUGIN_PATH", "/custom/env/path")

	paths := getDefaultPluginPaths()

	found := false
	for _, p := range paths {
		if p == "/custom/env/path" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Custom env path not included")
	}
}
