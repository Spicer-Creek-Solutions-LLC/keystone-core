package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// BlueprintFile represents a blueprint state file
type BlueprintFile struct {
	Path          string   `json:"path"`
	RelPath       string   `json:"rel_path"`
	Blueprint     string   `json:"blueprint"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Version       string   `json:"version"`
	StateCount    int      `json:"state_count"`
	HasIncludes   bool     `json:"has_includes"`
	Valid         bool     `json:"valid"`
	Errors        []string `json:"errors,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	UsesGoTemplate bool    `json:"uses_go_template"`
}

// BlueprintManifest represents a blueprint.yaml manifest
type BlueprintManifest struct {
	Path        string            `json:"path"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Parameters  int               `json:"parameter_count"`
	Features    int               `json:"feature_count"`
	Valid       bool              `json:"valid"`
	Errors      []string          `json:"errors,omitempty"`
}

// BlueprintInventory holds blueprint validation results
type BlueprintInventory struct {
	RootDir     string              `json:"root_dir"`
	Blueprints  []BlueprintManifest `json:"blueprints"`
	StateFiles  []BlueprintFile     `json:"state_files"`
	Summary     BlueprintSummary    `json:"summary"`
}

// BlueprintSummary provides summary statistics
type BlueprintSummary struct {
	TotalBlueprints  int `json:"total_blueprints"`
	TotalStateFiles  int `json:"total_state_files"`
	TotalStates      int `json:"total_states"`
	ValidFiles       int `json:"valid_files"`
	InvalidFiles     int `json:"invalid_files"`
	FilesWithWarnings int `json:"files_with_warnings"`
	GoTemplateUsage  int `json:"go_template_usage"`
}

// StateFileContent represents parsed state file content
type StateFileContent struct {
	Metadata struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Version     string `yaml:"version"`
	} `yaml:"metadata"`
	States    map[string]interface{} `yaml:"states"`
	Include   []string               `yaml:"include"`
	IncludeIf []struct {
		Path      string `yaml:"path"`
		Condition string `yaml:"condition"`
	} `yaml:"include_if"`
}

// BlueprintYAML represents a blueprint.yaml manifest
type BlueprintYAML struct {
	// Top-level fields
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`

	// Metadata section (where name, version, description live)
	Metadata struct {
		Name        string `yaml:"name"`
		Version     string `yaml:"version"`
		Description string `yaml:"description"`
	} `yaml:"metadata"`

	// Top-level parameters/features (lamp-stack style)
	Parameters map[string]interface{} `yaml:"parameters"`
	Features   map[string]interface{} `yaml:"features"`

	// Spec section (monitoring-stack style)
	Spec struct {
		Parameters map[string]interface{} `yaml:"parameters"`
		Features   map[string]interface{} `yaml:"features"`
	} `yaml:"spec"`
}

// BlueprintValidator validates blueprint files
type BlueprintValidator struct {
	rootDir   string
	inventory BlueprintInventory
}

// NewBlueprintValidator creates a new blueprint validator
func NewBlueprintValidator(rootDir string) *BlueprintValidator {
	return &BlueprintValidator{
		rootDir: rootDir,
		inventory: BlueprintInventory{
			RootDir: rootDir,
		},
	}
}

// ValidateBlueprints validates all blueprints in the examples directory
func (bv *BlueprintValidator) ValidateBlueprints(verbose bool) *BlueprintInventory {
	blueprintsDir := filepath.Join(bv.rootDir, "examples", "blueprints")

	if verbose {
		fmt.Printf("Scanning blueprints in %s\n", blueprintsDir)
	}

	// Find all blueprint directories
	entries, err := os.ReadDir(blueprintsDir)
	if err != nil {
		if verbose {
			fmt.Printf("  Warning: Could not read blueprints directory: %v\n", err)
		}
		return &bv.inventory
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		blueprintPath := filepath.Join(blueprintsDir, entry.Name())
		bv.validateBlueprint(blueprintPath, entry.Name(), verbose)
	}

	// Calculate summary
	bv.calculateSummary()

	return &bv.inventory
}

func (bv *BlueprintValidator) validateBlueprint(path, name string, verbose bool) {
	if verbose {
		fmt.Printf("  Validating blueprint: %s\n", name)
	}

	// Check for blueprint.yaml manifest
	manifestPath := filepath.Join(path, "blueprint.yaml")
	if _, err := os.Stat(manifestPath); err == nil {
		manifest := bv.validateManifest(manifestPath, name, verbose)
		bv.inventory.Blueprints = append(bv.inventory.Blueprints, manifest)
	}

	// Validate state files
	statesDir := filepath.Join(path, "states")
	if _, err := os.Stat(statesDir); err == nil {
		bv.validateStateFiles(statesDir, name, verbose)
	}
}

func (bv *BlueprintValidator) validateManifest(path, blueprint string, verbose bool) BlueprintManifest {
	manifest := BlueprintManifest{
		Path: path,
	}

	content, err := os.ReadFile(path)
	if err != nil {
		manifest.Errors = append(manifest.Errors, fmt.Sprintf("Could not read file: %v", err))
		return manifest
	}

	var bp BlueprintYAML
	if err := yaml.Unmarshal(content, &bp); err != nil {
		manifest.Errors = append(manifest.Errors, fmt.Sprintf("YAML parse error: %v", err))
		return manifest
	}

	// Extract name and version from metadata section
	manifest.Name = bp.Metadata.Name
	manifest.Version = bp.Metadata.Version
	manifest.Description = bp.Metadata.Description

	// Parameters and features can be at top-level or in spec section
	paramCount := len(bp.Parameters)
	if paramCount == 0 && len(bp.Spec.Parameters) > 0 {
		paramCount = len(bp.Spec.Parameters)
	}
	manifest.Parameters = paramCount

	featureCount := len(bp.Features)
	if featureCount == 0 && len(bp.Spec.Features) > 0 {
		featureCount = len(bp.Spec.Features)
	}
	manifest.Features = featureCount

	// Validate required fields
	if bp.Metadata.Name == "" {
		manifest.Errors = append(manifest.Errors, "Missing required field: metadata.name")
	}
	if bp.Metadata.Version == "" {
		manifest.Errors = append(manifest.Errors, "Missing required field: metadata.version")
	}

	manifest.Valid = len(manifest.Errors) == 0

	if verbose {
		status := "✓"
		if !manifest.Valid {
			status = "✗"
		}
		fmt.Printf("    %s blueprint.yaml (params: %d, features: %d)\n", status, manifest.Parameters, manifest.Features)
	}

	return manifest
}

func (bv *BlueprintValidator) validateStateFiles(statesDir, blueprint string, verbose bool) {
	filepath.Walk(statesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}

		stateFile := bv.validateStateFile(path, blueprint, verbose)
		bv.inventory.StateFiles = append(bv.inventory.StateFiles, stateFile)

		return nil
	})
}

func (bv *BlueprintValidator) validateStateFile(path, blueprint string, verbose bool) BlueprintFile {
	relPath, _ := filepath.Rel(bv.rootDir, path)

	stateFile := BlueprintFile{
		Path:      path,
		RelPath:   relPath,
		Blueprint: blueprint,
	}

	content, err := os.ReadFile(path)
	if err != nil {
		stateFile.Errors = append(stateFile.Errors, fmt.Sprintf("Could not read file: %v", err))
		return stateFile
	}

	contentStr := string(content)

	// Check for Go template usage
	goTemplatePattern := regexp.MustCompile(`\{\{\s*\.`)
	stateFile.UsesGoTemplate = goTemplatePattern.MatchString(contentStr)

	// Check for Salt/Jinja remnants
	// Note: Jinja uses {% if %} and {% for %}, Go templates use {{ if }} and {{ range }}
	// We detect Jinja-style block tags with {% %}
	saltPatterns := []struct {
		pattern *regexp.Regexp
		message string
	}{
		{regexp.MustCompile(`\{%\s*if\s`), "Jinja if statement found (use Go template {{ if }})"},
		{regexp.MustCompile(`\{%\s*for\s`), "Jinja for loop found (use Go template {{ range }})"},
		{regexp.MustCompile(`\{%\s*endif\s*%\}`), "Jinja endif found (use Go template {{ end }})"},
		{regexp.MustCompile(`\{%\s*endfor\s*%\}`), "Jinja endfor found (use Go template {{ end }})"},
		{regexp.MustCompile(`\{\{\s*salt\[`), "Salt function call found"},
		{regexp.MustCompile(`\{\{\s*grains\[`), "Salt grains reference found (use .Facts)"},
		{regexp.MustCompile(`\{\{\s*pillar\[`), "Salt pillar reference found (use .Vars)"},
		{regexp.MustCompile(`\{\{\s*parameters\.\w+\s*\}\}`), "Salt parameters.X found (use .Params.X)"},
		{regexp.MustCompile(`template://`), "template:// protocol found (use inline contents:)"},
		{regexp.MustCompile(`file://`), "file:// protocol found (use inline contents:)"},
		{regexp.MustCompile(`\.managed:`), "Salt .managed state found (use module: file)"},
		{regexp.MustCompile(`\.installed:`), "Salt .installed state found (use module: package)"},
		{regexp.MustCompile(`\.running:`), "Salt .running state found (use module: service)"},
	}

	for _, sp := range saltPatterns {
		if sp.pattern.MatchString(contentStr) {
			stateFile.Errors = append(stateFile.Errors, sp.message)
		}
	}

	// Parse YAML
	var stateContent StateFileContent
	if err := yaml.Unmarshal(content, &stateContent); err != nil {
		stateFile.Errors = append(stateFile.Errors, fmt.Sprintf("YAML parse error: %v", err))
		return stateFile
	}

	// Extract metadata
	stateFile.Name = stateContent.Metadata.Name
	stateFile.Description = stateContent.Metadata.Description
	stateFile.Version = stateContent.Metadata.Version

	// Check for required metadata
	if stateContent.Metadata.Name == "" {
		stateFile.Warnings = append(stateFile.Warnings, "Missing metadata.name")
	}

	// Count states
	stateFile.StateCount = len(stateContent.States)
	stateFile.HasIncludes = len(stateContent.Include) > 0 || len(stateContent.IncludeIf) > 0

	// Validate state structure
	for stateName, stateDef := range stateContent.States {
		stateMap, ok := stateDef.(map[string]interface{})
		if !ok {
			stateFile.Errors = append(stateFile.Errors, fmt.Sprintf("State '%s' is not a valid mapping", stateName))
			continue
		}

		// Check for required fields
		if _, hasModule := stateMap["module"]; !hasModule {
			stateFile.Errors = append(stateFile.Errors, fmt.Sprintf("State '%s': missing 'module' field", stateName))
		}
		if _, hasState := stateMap["state"]; !hasState {
			stateFile.Errors = append(stateFile.Errors, fmt.Sprintf("State '%s': missing 'state' field", stateName))
		}
	}

	// If no states and no includes, warn
	if stateFile.StateCount == 0 && !stateFile.HasIncludes {
		stateFile.Warnings = append(stateFile.Warnings, "No states or includes defined")
	}

	stateFile.Valid = len(stateFile.Errors) == 0

	if verbose {
		status := "✓"
		if !stateFile.Valid {
			status = "✗"
		} else if len(stateFile.Warnings) > 0 {
			status = "⚠"
		}
		fmt.Printf("    %s %s (states: %d)\n", status, filepath.Base(path), stateFile.StateCount)
	}

	return stateFile
}

func (bv *BlueprintValidator) calculateSummary() {
	summary := &bv.inventory.Summary
	summary.TotalBlueprints = len(bv.inventory.Blueprints)
	summary.TotalStateFiles = len(bv.inventory.StateFiles)

	for _, sf := range bv.inventory.StateFiles {
		summary.TotalStates += sf.StateCount
		if sf.Valid {
			summary.ValidFiles++
		} else {
			summary.InvalidFiles++
		}
		if len(sf.Warnings) > 0 {
			summary.FilesWithWarnings++
		}
		if sf.UsesGoTemplate {
			summary.GoTemplateUsage++
		}
	}
}

// GenerateBlueprintReport generates a markdown report
func GenerateBlueprintReport(inv *BlueprintInventory) string {
	var sb strings.Builder

	sb.WriteString("# Blueprint Validation Report\n\n")

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Blueprints**: %d\n", inv.Summary.TotalBlueprints))
	sb.WriteString(fmt.Sprintf("- **State Files**: %d\n", inv.Summary.TotalStateFiles))
	sb.WriteString(fmt.Sprintf("- **Total States**: %d\n", inv.Summary.TotalStates))
	sb.WriteString(fmt.Sprintf("- **Valid Files**: %d\n", inv.Summary.ValidFiles))
	sb.WriteString(fmt.Sprintf("- **Invalid Files**: %d\n", inv.Summary.InvalidFiles))
	sb.WriteString(fmt.Sprintf("- **Files with Warnings**: %d\n", inv.Summary.FilesWithWarnings))
	sb.WriteString(fmt.Sprintf("- **Go Template Usage**: %d files\n\n", inv.Summary.GoTemplateUsage))

	// Blueprint details
	sb.WriteString("## Blueprints\n\n")
	sb.WriteString("| Blueprint | Version | Parameters | Features | Valid |\n")
	sb.WriteString("|-----------|---------|------------|----------|-------|\n")
	for _, bp := range inv.Blueprints {
		valid := "✓"
		if !bp.Valid {
			valid = "✗"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %s |\n",
			bp.Name, bp.Version, bp.Parameters, bp.Features, valid))
	}
	sb.WriteString("\n")

	// State files by blueprint
	sb.WriteString("## State Files by Blueprint\n\n")

	// Group by blueprint
	byBlueprint := make(map[string][]BlueprintFile)
	for _, sf := range inv.StateFiles {
		byBlueprint[sf.Blueprint] = append(byBlueprint[sf.Blueprint], sf)
	}

	for blueprint, files := range byBlueprint {
		sb.WriteString(fmt.Sprintf("### %s\n\n", blueprint))
		sb.WriteString("| File | States | Valid | Template |\n")
		sb.WriteString("|------|--------|-------|----------|\n")
		for _, sf := range files {
			valid := "✓"
			if !sf.Valid {
				valid = "✗"
			} else if len(sf.Warnings) > 0 {
				valid = "⚠"
			}
			template := ""
			if sf.UsesGoTemplate {
				template = "Go"
			}
			sb.WriteString(fmt.Sprintf("| %s | %d | %s | %s |\n",
				filepath.Base(sf.Path), sf.StateCount, valid, template))
		}
		sb.WriteString("\n")
	}

	// Errors
	hasErrors := false
	for _, sf := range inv.StateFiles {
		if len(sf.Errors) > 0 {
			hasErrors = true
			break
		}
	}

	if hasErrors {
		sb.WriteString("## Errors\n\n")
		for _, sf := range inv.StateFiles {
			if len(sf.Errors) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("### %s\n\n", sf.RelPath))
			for _, err := range sf.Errors {
				sb.WriteString(fmt.Sprintf("- %s\n", err))
			}
			sb.WriteString("\n")
		}
	}

	// Warnings
	hasWarnings := false
	for _, sf := range inv.StateFiles {
		if len(sf.Warnings) > 0 {
			hasWarnings = true
			break
		}
	}

	if hasWarnings {
		sb.WriteString("## Warnings\n\n")
		for _, sf := range inv.StateFiles {
			if len(sf.Warnings) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("### %s\n\n", sf.RelPath))
			for _, warn := range sf.Warnings {
				sb.WriteString(fmt.Sprintf("- %s\n", warn))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
