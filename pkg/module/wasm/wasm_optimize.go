// Package wasm provides WASM module build utilities with size optimization.
package wasm

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OptimizationLevel represents WASM size optimization level
type OptimizationLevel int

const (
	// OptNone performs no optimization
	OptNone OptimizationLevel = iota
	// OptSize optimizes for smaller binary size
	OptSize
	// OptAggressive uses aggressive size optimization (may impact performance)
	OptAggressive
)

// BuildConfig configures WASM module compilation
type BuildConfig struct {
	// SourcePath is the Go source file or directory
	SourcePath string

	// OutputPath is the output WASM file path
	OutputPath string

	// OptLevel is the optimization level
	OptLevel OptimizationLevel

	// StripDebug removes debug information
	StripDebug bool

	// TinyGo uses TinyGo compiler instead of standard Go (smaller output)
	TinyGo bool

	// GOOS target (default: wasip1)
	GOOS string

	// GOARCH target (default: wasm)
	GOARCH string

	// LDFlags are additional linker flags
	LDFlags []string

	// Tags are build tags to include
	Tags []string

	// TrimPath removes file system paths from binary
	TrimPath bool

	// Env contains additional environment variables
	Env map[string]string
}

// DefaultBuildConfig returns a configuration optimized for size
func DefaultBuildConfig() *BuildConfig {
	return &BuildConfig{
		OptLevel:   OptSize,
		StripDebug: true,
		TinyGo:     false,
		GOOS:       "wasip1",
		GOARCH:     "wasm",
		TrimPath:   true,
		LDFlags:    []string{},
		Tags:       []string{},
		Env:        make(map[string]string),
	}
}

// TinyGoBuildConfig returns a configuration using TinyGo for minimal size
func TinyGoBuildConfig() *BuildConfig {
	return &BuildConfig{
		OptLevel:   OptAggressive,
		StripDebug: true,
		TinyGo:     true,
		GOOS:       "wasip1",
		GOARCH:     "wasm",
		TrimPath:   true,
		LDFlags:    []string{},
		Tags:       []string{},
		Env:        make(map[string]string),
	}
}

// Builder compiles Go code to optimized WASM modules
type Builder struct {
	config *BuildConfig
}

// NewBuilder creates a new WASM builder
func NewBuilder(config *BuildConfig) *Builder {
	if config == nil {
		config = DefaultBuildConfig()
	}
	return &Builder{config: config}
}

// Build compiles the WASM module
func (b *Builder) Build() (*BuildResult, error) {
	if b.config.TinyGo {
		return b.buildTinyGo()
	}
	return b.buildGo()
}

func (b *Builder) buildGo() (*BuildResult, error) {
	// Build ldflags
	ldflags := b.buildLDFlags()

	// Build arguments
	args := []string{"build"}

	// Add optimization flags
	if b.config.TrimPath {
		args = append(args, "-trimpath")
	}

	// Add ldflags
	if len(ldflags) > 0 {
		args = append(args, "-ldflags", strings.Join(ldflags, " "))
	}

	// Add tags
	if len(b.config.Tags) > 0 {
		args = append(args, "-tags", strings.Join(b.config.Tags, ","))
	}

	// Add output
	args = append(args, "-o", b.config.OutputPath)

	// Add source
	args = append(args, b.config.SourcePath)

	// Setup environment
	env := b.buildEnv()

	// Execute build
	cmd := exec.Command("go", args...)
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go build failed: %w\nstderr: %s", err, stderr.String())
	}

	return b.buildResult()
}

func (b *Builder) buildTinyGo() (*BuildResult, error) {
	// Check if TinyGo is installed
	if _, err := exec.LookPath("tinygo"); err != nil {
		return nil, fmt.Errorf("TinyGo not found: install from https://tinygo.org")
	}

	// Build arguments
	args := []string{"build"}

	// Target
	args = append(args, "-target", "wasip1")

	// Optimization
	switch b.config.OptLevel {
	case OptSize:
		args = append(args, "-opt", "s")
	case OptAggressive:
		args = append(args, "-opt", "z")
	default:
		args = append(args, "-opt", "2")
	}

	// Disable features for smaller size
	args = append(args, "-no-debug")

	// Add tags
	if len(b.config.Tags) > 0 {
		args = append(args, "-tags", strings.Join(b.config.Tags, ","))
	}

	// Add output
	args = append(args, "-o", b.config.OutputPath)

	// Add source
	args = append(args, b.config.SourcePath)

	// Execute build
	cmd := exec.Command("tinygo", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tinygo build failed: %w\nstderr: %s", err, stderr.String())
	}

	return b.buildResult()
}

func (b *Builder) buildLDFlags() []string {
	var flags []string

	// Strip debug info
	if b.config.StripDebug {
		flags = append(flags, "-s", "-w")
	}

	// Add custom ldflags
	flags = append(flags, b.config.LDFlags...)

	return flags
}

func (b *Builder) buildEnv() []string {
	// Start with current environment
	env := os.Environ()

	// Set GOOS and GOARCH
	env = append(env, fmt.Sprintf("GOOS=%s", b.config.GOOS))
	env = append(env, fmt.Sprintf("GOARCH=%s", b.config.GOARCH))

	// Add custom environment
	for k, v := range b.config.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}

func (b *Builder) buildResult() (*BuildResult, error) {
	info, err := os.Stat(b.config.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output: %w", err)
	}

	return &BuildResult{
		OutputPath: b.config.OutputPath,
		Size:       info.Size(),
		Compiler:   b.compilerName(),
		OptLevel:   b.config.OptLevel,
	}, nil
}

func (b *Builder) compilerName() string {
	if b.config.TinyGo {
		return "tinygo"
	}
	return "go"
}

// BuildResult contains the result of a WASM build
type BuildResult struct {
	// OutputPath is the path to the built WASM file
	OutputPath string

	// Size is the file size in bytes
	Size int64

	// Compiler is the compiler used (go or tinygo)
	Compiler string

	// OptLevel is the optimization level used
	OptLevel OptimizationLevel
}

// SizeString returns a human-readable size
func (r *BuildResult) SizeString() string {
	return formatBytes(r.Size)
}

// Optimize runs wasm-opt on the output file if available
func (b *Builder) Optimize() error {
	// Check if wasm-opt is available
	if _, err := exec.LookPath("wasm-opt"); err != nil {
		return nil // wasm-opt not available, skip
	}

	// Build arguments based on optimization level
	var args []string
	switch b.config.OptLevel {
	case OptSize:
		args = []string{"-Os"}
	case OptAggressive:
		args = []string{"-Oz"}
	default:
		args = []string{"-O2"}
	}

	// Add output file (in-place)
	args = append(args, b.config.OutputPath, "-o", b.config.OutputPath)

	// Execute wasm-opt
	cmd := exec.Command("wasm-opt", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wasm-opt failed: %w\nstderr: %s", err, stderr.String())
	}

	return nil
}

// Strip removes custom sections from WASM file to reduce size
func (b *Builder) Strip() error {
	// Check if wasm-strip is available
	if _, err := exec.LookPath("wasm-strip"); err != nil {
		return nil // wasm-strip not available, skip
	}

	cmd := exec.Command("wasm-strip", b.config.OutputPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wasm-strip failed: %w\nstderr: %s", err, stderr.String())
	}

	return nil
}

// BuildAndOptimize builds and then optimizes the WASM module
func (b *Builder) BuildAndOptimize() (*BuildResult, error) {
	// Build
	result, err := b.Build()
	if err != nil {
		return nil, err
	}

	initialSize := result.Size

	// Run wasm-opt if available
	if err := b.Optimize(); err != nil {
		return nil, fmt.Errorf("optimization failed: %w", err)
	}

	// Run wasm-strip if available and debug stripping enabled
	if b.config.StripDebug {
		if err := b.Strip(); err != nil {
			return nil, fmt.Errorf("stripping failed: %w", err)
		}
	}

	// Update result with final size
	info, err := os.Stat(b.config.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output: %w", err)
	}

	result.Size = info.Size()

	// Log size reduction if significant
	if initialSize > result.Size {
		reduction := float64(initialSize-result.Size) / float64(initialSize) * 100
		fmt.Printf("Size reduced from %s to %s (%.1f%% reduction)\n",
			formatBytes(initialSize), formatBytes(result.Size), reduction)
	}

	return result, nil
}

// RecommendedFlags returns recommended build flags for WASM optimization
func RecommendedFlags() map[string]string {
	return map[string]string{
		"GOOS":        "wasip1",
		"GOARCH":      "wasm",
		"CGO_ENABLED": "0",
	}
}

// RecommendedLDFlags returns recommended linker flags
func RecommendedLDFlags() []string {
	return []string{
		"-s", // Strip symbol table
		"-w", // Strip DWARF debug info
	}
}

// RecommendedTags returns recommended build tags for smaller binaries
func RecommendedTags() []string {
	return []string{
		"purego", // Use pure Go implementations (no assembly)
	}
}

// EstimateSizeReduction returns estimated size reduction for each optimization
func EstimateSizeReduction() map[string]string {
	return map[string]string{
		"-s -w ldflags":         "10-20% reduction",
		"-trimpath":             "5-10% reduction",
		"TinyGo compiler":       "50-80% reduction vs Go",
		"wasm-opt -Os":          "5-15% additional reduction",
		"wasm-opt -Oz":          "10-20% additional reduction",
		"wasm-strip":            "1-5% additional reduction",
		"purego build tag":      "Varies by dependencies",
	}
}

// ValidateWASMFile checks if the file is a valid WASM module
func ValidateWASMFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Check WASM magic number
	magic := make([]byte, 4)
	if _, err := file.Read(magic); err != nil {
		return fmt.Errorf("failed to read magic: %w", err)
	}

	// WASM magic: 0x00 0x61 0x73 0x6D ("\0asm")
	expected := []byte{0x00, 0x61, 0x73, 0x6D}
	if !bytes.Equal(magic, expected) {
		return fmt.Errorf("invalid WASM magic number: got %x, expected %x", magic, expected)
	}

	return nil
}

// GetWASMSize returns the size of a WASM file
func GetWASMSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// CompareBuilds compares two WASM build results
func CompareBuilds(a, b *BuildResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Build A (%s): %s\n", a.Compiler, a.SizeString()))
	sb.WriteString(fmt.Sprintf("Build B (%s): %s\n", b.Compiler, b.SizeString()))

	diff := b.Size - a.Size
	if diff > 0 {
		sb.WriteString(fmt.Sprintf("Build A is %s smaller\n", formatBytes(diff)))
	} else if diff < 0 {
		sb.WriteString(fmt.Sprintf("Build B is %s smaller\n", formatBytes(-diff)))
	} else {
		sb.WriteString("Builds are the same size\n")
	}

	return sb.String()
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// InstallOptimizationTools prints instructions for installing WASM optimization tools
func InstallOptimizationTools() string {
	return `WASM Optimization Tools Installation:

1. wasm-opt (from Binaryen):
   - macOS: brew install binaryen
   - Ubuntu: apt install binaryen
   - Manual: https://github.com/WebAssembly/binaryen/releases

2. wasm-strip (from WABT):
   - macOS: brew install wabt
   - Ubuntu: apt install wabt
   - Manual: https://github.com/WebAssembly/wabt/releases

3. TinyGo (alternative Go compiler):
   - macOS: brew install tinygo
   - Ubuntu: See https://tinygo.org/getting-started/install/
   - Manual: https://tinygo.org/getting-started/install/

Recommended tools for maximum size reduction:
- TinyGo (50-80% smaller than Go)
- wasm-opt -Oz (10-20% additional reduction)
- wasm-strip (removes custom sections)
`
}

// Makefile returns a sample Makefile for WASM builds
func Makefile(moduleName string) string {
	return fmt.Sprintf(`# Makefile for %s WASM module

WASM_FILE = %s.wasm
GO_SOURCE = main.go

# Build flags
GOOS = wasip1
GOARCH = wasm
LDFLAGS = -s -w
BUILD_FLAGS = -trimpath

# Default target
.PHONY: all
all: build optimize

# Build with Go
.PHONY: build
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(WASM_FILE) $(GO_SOURCE)
	@echo "Built $(WASM_FILE): $$(stat -f%%z $(WASM_FILE) 2>/dev/null || stat -c%%s $(WASM_FILE)) bytes"

# Build with TinyGo (smaller output)
.PHONY: build-tiny
build-tiny:
	tinygo build -target wasip1 -opt z -no-debug -o $(WASM_FILE) $(GO_SOURCE)
	@echo "Built $(WASM_FILE): $$(stat -f%%z $(WASM_FILE) 2>/dev/null || stat -c%%s $(WASM_FILE)) bytes"

# Optimize with wasm-opt
.PHONY: optimize
optimize:
	@if command -v wasm-opt >/dev/null 2>&1; then \
		wasm-opt -Oz $(WASM_FILE) -o $(WASM_FILE); \
		echo "Optimized $(WASM_FILE): $$(stat -f%%z $(WASM_FILE) 2>/dev/null || stat -c%%s $(WASM_FILE)) bytes"; \
	else \
		echo "wasm-opt not found, skipping optimization"; \
	fi

# Strip debug sections
.PHONY: strip
strip:
	@if command -v wasm-strip >/dev/null 2>&1; then \
		wasm-strip $(WASM_FILE); \
		echo "Stripped $(WASM_FILE): $$(stat -f%%z $(WASM_FILE) 2>/dev/null || stat -c%%s $(WASM_FILE)) bytes"; \
	else \
		echo "wasm-strip not found, skipping"; \
	fi

# Full optimization pipeline
.PHONY: release
release: build-tiny optimize strip
	@echo "Release build complete"

# Clean
.PHONY: clean
clean:
	rm -f $(WASM_FILE)

# Validate WASM file
.PHONY: validate
validate:
	@if [ -f $(WASM_FILE) ]; then \
		head -c 4 $(WASM_FILE) | xxd | grep -q "0061 736d" && echo "Valid WASM file" || echo "Invalid WASM file"; \
	else \
		echo "$(WASM_FILE) not found"; \
	fi

# Show size comparison
.PHONY: compare
compare:
	@echo "Size comparison:"
	@echo "Go build:"; GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o /tmp/go.wasm $(GO_SOURCE); stat -f "  %%z bytes" /tmp/go.wasm 2>/dev/null || stat -c "  %%s bytes" /tmp/go.wasm
	@if command -v tinygo >/dev/null 2>&1; then \
		echo "TinyGo build:"; tinygo build -target wasip1 -opt z -no-debug -o /tmp/tinygo.wasm $(GO_SOURCE); stat -f "  %%z bytes" /tmp/tinygo.wasm 2>/dev/null || stat -c "  %%s bytes" /tmp/tinygo.wasm; \
	fi
	@rm -f /tmp/go.wasm /tmp/tinygo.wasm
`, moduleName, moduleName)
}

// GenerateBuildScript creates a build script for a WASM module
func GenerateBuildScript(outputDir, moduleName string) error {
	// Create Makefile
	makefilePath := filepath.Join(outputDir, "Makefile")
	if err := os.WriteFile(makefilePath, []byte(Makefile(moduleName)), 0644); err != nil {
		return fmt.Errorf("failed to write Makefile: %w", err)
	}

	return nil
}
