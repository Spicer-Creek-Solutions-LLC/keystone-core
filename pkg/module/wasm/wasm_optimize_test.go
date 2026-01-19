package wasm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultBuildConfig(t *testing.T) {
	cfg := DefaultBuildConfig()

	if cfg.OptLevel != OptSize {
		t.Errorf("OptLevel = %d, want OptSize", cfg.OptLevel)
	}
	if !cfg.StripDebug {
		t.Error("StripDebug should be true")
	}
	if cfg.TinyGo {
		t.Error("TinyGo should be false by default")
	}
	if cfg.GOOS != "wasip1" {
		t.Errorf("GOOS = %s, want wasip1", cfg.GOOS)
	}
	if cfg.GOARCH != "wasm" {
		t.Errorf("GOARCH = %s, want wasm", cfg.GOARCH)
	}
	if !cfg.TrimPath {
		t.Error("TrimPath should be true")
	}
}

func TestTinyGoBuildConfig(t *testing.T) {
	cfg := TinyGoBuildConfig()

	if cfg.OptLevel != OptAggressive {
		t.Errorf("OptLevel = %d, want OptAggressive", cfg.OptLevel)
	}
	if !cfg.TinyGo {
		t.Error("TinyGo should be true")
	}
}

func TestNewBuilder(t *testing.T) {
	// With nil config
	b := NewBuilder(nil)
	if b.config.GOOS != "wasip1" {
		t.Error("Should use default config when nil")
	}

	// With custom config
	cfg := &BuildConfig{GOOS: "js"}
	b = NewBuilder(cfg)
	if b.config.GOOS != "js" {
		t.Error("Should use provided config")
	}
}

func TestBuilder_buildLDFlags(t *testing.T) {
	cfg := &BuildConfig{
		StripDebug: true,
		LDFlags:    []string{"-X", "main.version=1.0.0"},
	}
	b := NewBuilder(cfg)

	flags := b.buildLDFlags()

	// Should include -s -w for stripping
	hasS := false
	hasW := false
	hasCustom := false

	for _, f := range flags {
		if f == "-s" {
			hasS = true
		}
		if f == "-w" {
			hasW = true
		}
		if f == "-X" {
			hasCustom = true
		}
	}

	if !hasS {
		t.Error("Should include -s flag")
	}
	if !hasW {
		t.Error("Should include -w flag")
	}
	if !hasCustom {
		t.Error("Should include custom ldflags")
	}
}

func TestBuilder_buildEnv(t *testing.T) {
	cfg := &BuildConfig{
		GOOS:   "wasip1",
		GOARCH: "wasm",
		Env: map[string]string{
			"CUSTOM_VAR": "value",
		},
	}
	b := NewBuilder(cfg)

	env := b.buildEnv()

	hasGOOS := false
	hasGOARCH := false
	hasCustom := false

	for _, e := range env {
		if strings.HasPrefix(e, "GOOS=wasip1") {
			hasGOOS = true
		}
		if strings.HasPrefix(e, "GOARCH=wasm") {
			hasGOARCH = true
		}
		if e == "CUSTOM_VAR=value" {
			hasCustom = true
		}
	}

	if !hasGOOS {
		t.Error("Should set GOOS")
	}
	if !hasGOARCH {
		t.Error("Should set GOARCH")
	}
	if !hasCustom {
		t.Error("Should include custom env vars")
	}
}

func TestBuilder_compilerName(t *testing.T) {
	// Go compiler
	b := NewBuilder(&BuildConfig{TinyGo: false})
	if b.compilerName() != "go" {
		t.Errorf("compilerName() = %s, want go", b.compilerName())
	}

	// TinyGo compiler
	b = NewBuilder(&BuildConfig{TinyGo: true})
	if b.compilerName() != "tinygo" {
		t.Errorf("compilerName() = %s, want tinygo", b.compilerName())
	}
}

func TestBuildResult_SizeString(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{5 * 1024 * 1024, "5.0 MiB"},
	}

	for _, tt := range tests {
		r := &BuildResult{Size: tt.size}
		if got := r.SizeString(); got != tt.expected {
			t.Errorf("SizeString() for %d = %s, want %s", tt.size, got, tt.expected)
		}
	}
}

func TestRecommendedFlags(t *testing.T) {
	flags := RecommendedFlags()

	if flags["GOOS"] != "wasip1" {
		t.Errorf("GOOS = %s, want wasip1", flags["GOOS"])
	}
	if flags["GOARCH"] != "wasm" {
		t.Errorf("GOARCH = %s, want wasm", flags["GOARCH"])
	}
	if flags["CGO_ENABLED"] != "0" {
		t.Errorf("CGO_ENABLED = %s, want 0", flags["CGO_ENABLED"])
	}
}

func TestRecommendedLDFlags(t *testing.T) {
	flags := RecommendedLDFlags()

	hasS := false
	hasW := false

	for _, f := range flags {
		if f == "-s" {
			hasS = true
		}
		if f == "-w" {
			hasW = true
		}
	}

	if !hasS {
		t.Error("Should recommend -s flag")
	}
	if !hasW {
		t.Error("Should recommend -w flag")
	}
}

func TestRecommendedTags(t *testing.T) {
	tags := RecommendedTags()

	hasPurego := false
	for _, tag := range tags {
		if tag == "purego" {
			hasPurego = true
		}
	}

	if !hasPurego {
		t.Error("Should recommend purego tag")
	}
}

func TestEstimateSizeReduction(t *testing.T) {
	estimates := EstimateSizeReduction()

	if len(estimates) == 0 {
		t.Error("Should provide size reduction estimates")
	}

	// Check some expected entries
	if _, ok := estimates["-s -w ldflags"]; !ok {
		t.Error("Should have ldflags estimate")
	}
	if _, ok := estimates["TinyGo compiler"]; !ok {
		t.Error("Should have TinyGo estimate")
	}
}

func TestValidateWASMFile(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "wasm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test with valid WASM magic number
	validWasm := filepath.Join(tmpDir, "valid.wasm")
	// WASM magic: 0x00 0x61 0x73 0x6D ("\0asm") + version 1
	wasmBytes := []byte{0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00}
	if err := os.WriteFile(validWasm, wasmBytes, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := ValidateWASMFile(validWasm); err != nil {
		t.Errorf("ValidateWASMFile() should pass for valid WASM: %v", err)
	}

	// Test with invalid file
	invalidWasm := filepath.Join(tmpDir, "invalid.wasm")
	invalidBytes := []byte{0x50, 0x4B, 0x03, 0x04} // ZIP magic
	if err := os.WriteFile(invalidWasm, invalidBytes, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := ValidateWASMFile(invalidWasm); err == nil {
		t.Error("ValidateWASMFile() should fail for invalid WASM")
	}

	// Test with non-existent file
	if err := ValidateWASMFile("/nonexistent/file.wasm"); err == nil {
		t.Error("ValidateWASMFile() should fail for non-existent file")
	}
}

func TestGetWASMSize(t *testing.T) {
	// Create a temporary file
	tmpDir, err := os.MkdirTemp("", "wasm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.wasm")
	content := make([]byte, 1024)
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	size, err := GetWASMSize(testFile)
	if err != nil {
		t.Errorf("GetWASMSize() error: %v", err)
	}
	if size != 1024 {
		t.Errorf("GetWASMSize() = %d, want 1024", size)
	}

	// Non-existent file
	_, err = GetWASMSize("/nonexistent/file.wasm")
	if err == nil {
		t.Error("GetWASMSize() should fail for non-existent file")
	}
}

func TestCompareBuilds(t *testing.T) {
	a := &BuildResult{
		OutputPath: "a.wasm",
		Size:       100000,
		Compiler:   "go",
	}
	b := &BuildResult{
		OutputPath: "b.wasm",
		Size:       50000,
		Compiler:   "tinygo",
	}

	comparison := CompareBuilds(a, b)

	if !strings.Contains(comparison, "go") {
		t.Error("Comparison should mention go compiler")
	}
	if !strings.Contains(comparison, "tinygo") {
		t.Error("Comparison should mention tinygo compiler")
	}
	if !strings.Contains(comparison, "smaller") {
		t.Error("Comparison should indicate which is smaller")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{100, "100 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{1073741824, "1.0 GiB"},
	}

	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.expected {
			t.Errorf("formatBytes(%d) = %s, want %s", tt.bytes, got, tt.expected)
		}
	}
}

func TestInstallOptimizationTools(t *testing.T) {
	instructions := InstallOptimizationTools()

	if !strings.Contains(instructions, "wasm-opt") {
		t.Error("Should mention wasm-opt")
	}
	if !strings.Contains(instructions, "wasm-strip") {
		t.Error("Should mention wasm-strip")
	}
	if !strings.Contains(instructions, "TinyGo") {
		t.Error("Should mention TinyGo")
	}
	if !strings.Contains(instructions, "brew") {
		t.Error("Should include macOS instructions")
	}
	if !strings.Contains(instructions, "apt") {
		t.Error("Should include Ubuntu instructions")
	}
}

func TestMakefile(t *testing.T) {
	makefile := Makefile("mymodule")

	// Check for essential targets
	if !strings.Contains(makefile, ".PHONY: build") {
		t.Error("Should have build target")
	}
	if !strings.Contains(makefile, ".PHONY: build-tiny") {
		t.Error("Should have build-tiny target")
	}
	if !strings.Contains(makefile, ".PHONY: optimize") {
		t.Error("Should have optimize target")
	}
	if !strings.Contains(makefile, ".PHONY: strip") {
		t.Error("Should have strip target")
	}
	if !strings.Contains(makefile, ".PHONY: release") {
		t.Error("Should have release target")
	}

	// Check for WASM-specific settings
	if !strings.Contains(makefile, "GOOS = wasip1") {
		t.Error("Should set GOOS to wasip1")
	}
	if !strings.Contains(makefile, "GOARCH = wasm") {
		t.Error("Should set GOARCH to wasm")
	}
	if !strings.Contains(makefile, "-s -w") {
		t.Error("Should include strip ldflags")
	}
	if !strings.Contains(makefile, "-trimpath") {
		t.Error("Should include trimpath flag")
	}
}

func TestGenerateBuildScript(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "wasm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate build script
	if err := GenerateBuildScript(tmpDir, "testmodule"); err != nil {
		t.Errorf("GenerateBuildScript() error: %v", err)
	}

	// Check Makefile was created
	makefilePath := filepath.Join(tmpDir, "Makefile")
	if _, err := os.Stat(makefilePath); os.IsNotExist(err) {
		t.Error("Makefile should be created")
	}

	// Read and verify content
	content, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("Failed to read Makefile: %v", err)
	}

	if !strings.Contains(string(content), "testmodule.wasm") {
		t.Error("Makefile should use module name")
	}
}

func TestOptimizationLevel(t *testing.T) {
	levels := []OptimizationLevel{OptNone, OptSize, OptAggressive}

	// Just verify the constants are distinct
	seen := make(map[OptimizationLevel]bool)
	for _, level := range levels {
		if seen[level] {
			t.Errorf("Duplicate optimization level: %d", level)
		}
		seen[level] = true
	}
}
