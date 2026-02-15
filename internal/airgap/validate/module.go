package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ModuleChecker verifies that all required modules exist in the local registry.
type ModuleChecker struct {
	RegistryDir     string
	RequiredModules []string
}

// Name returns the checker name.
func (c *ModuleChecker) Name() string { return "module-availability" }

// Category returns the check category.
func (c *ModuleChecker) Category() CheckCategory { return CategoryModule }

// Check verifies modules are present in the local registry.
func (c *ModuleChecker) Check(ctx context.Context) ([]Finding, error) {
	if c.RegistryDir == "" {
		return []Finding{{
			Category: CategoryModule,
			Check:    "module-availability",
			Severity: SeverityWarn,
			Message:  "no registry directory configured",
		}}, nil
	}

	available, err := c.readRegistryModules()
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}

	if len(c.RequiredModules) == 0 {
		return []Finding{{
			Category: CategoryModule,
			Check:    "module-availability",
			Severity: SeverityPass,
			Message:  fmt.Sprintf("registry contains %d modules (no required list specified)", len(available)),
		}}, nil
	}

	var findings []Finding
	for _, mod := range c.RequiredModules {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if available[mod] {
			findings = append(findings, Finding{
				Category: CategoryModule,
				Check:    "module-availability",
				Severity: SeverityPass,
				Message:  fmt.Sprintf("module %q available in local registry", mod),
			})
		} else {
			findings = append(findings, Finding{
				Category:    CategoryModule,
				Check:       "module-availability",
				Severity:    SeverityFail,
				Message:     fmt.Sprintf("module %q not found in local registry", mod),
				Remediation: fmt.Sprintf("Import %q into the local registry before air-gapped deployment", mod),
			})
		}
	}
	return findings, nil
}

func (c *ModuleChecker) readRegistryModules() (map[string]bool, error) {
	modules := make(map[string]bool)

	// Try reading index.json first
	indexPath := filepath.Join(c.RegistryDir, "index.json")
	data, readErr := os.ReadFile(indexPath) //#nosec G304 -- path constructed from validated registry dir
	if readErr == nil {
		var idx struct {
			Modules []struct {
				Name string `json:"name"`
			} `json:"modules"`
		}
		if unmarshalErr := json.Unmarshal(data, &idx); unmarshalErr == nil {
			for _, m := range idx.Modules {
				modules[m.Name] = true
			}
			return modules, nil
		}
	}

	// Fall back to directory scanning
	modulesDir := filepath.Join(c.RegistryDir, "modules")
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return modules, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// vendor/package structure
			subEntries, err := os.ReadDir(filepath.Join(modulesDir, entry.Name()))
			if err != nil {
				continue
			}
			for _, sub := range subEntries {
				if sub.IsDir() {
					modules[entry.Name()+"/"+sub.Name()] = true
				}
			}
		}
	}
	return modules, nil
}
