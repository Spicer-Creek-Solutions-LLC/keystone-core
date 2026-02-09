// Package main implements the kscore-module CLI for module management operations.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
)

var (
	buildOutput   string
	buildNoVerify bool
)

var buildCmd = &cobra.Command{
	Use:   "build [path]",
	Short: "Build module for distribution",
	Long: `Build a module into a distributable ZIP archive.

The build process:
  1. Validates module.yaml
  2. Includes all necessary files
  3. Computes SHA256 hash
  4. Creates versioned ZIP archive

Output format: <name>-<version>.zip

Examples:
  # Build current directory
  kscorectl module build

  # Build specific directory
  kscorectl module build ./my-module

  # Specify output file
  kscorectl module build --output ./dist/my-module.zip`,
	Args: cobra.MaximumNArgs(1),
	RunE: buildExecute,
}

func init() {
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Output file path")
	buildCmd.Flags().BoolVar(&buildNoVerify, "no-verify", false, "Skip pre-build validation")
}

func buildExecute(cmd *cobra.Command, args []string) error {
	// Determine source path
	sourcePath := "."
	if len(args) > 0 {
		sourcePath = args[0]
	}

	// Make absolute
	absPath, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Find and parse module.yaml
	manifestPath := filepath.Join(absPath, "module.yaml")
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to parse module.yaml: %w", err)
	}

	fmt.Printf("Building module: %s v%s\n", m.Name, m.Version)

	// Validate unless skipped
	if !buildNoVerify {
		if err := validateManifest(m); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
		fmt.Println("✓ Validation passed")
	}

	// Determine output filename
	outputFile := buildOutput
	if outputFile == "" {
		// Use module name (replace / with -)
		safeName := strings.ReplaceAll(m.Name, "/", "-")
		outputFile = fmt.Sprintf("%s-%s.zip", safeName, m.Version)
	}

	// Collect files to include
	files, err := collectBuildFiles(absPath, m)
	if err != nil {
		return fmt.Errorf("failed to collect files: %w", err)
	}

	fmt.Printf("Including %d files\n", len(files))

	// Create ZIP archive
	if err := createZipArchive(outputFile, absPath, files); err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

	// Compute hash
	hash, err := computeFileHash(outputFile)
	if err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	// Get file size
	info, err := os.Stat(outputFile)
	if err != nil {
		return fmt.Errorf("failed to stat output file: %w", err)
	}

	fmt.Printf("\n✓ Build complete!\n\n")
	fmt.Printf("Output:   %s\n", outputFile)
	fmt.Printf("Size:     %s\n", formatSize(info.Size()))
	fmt.Printf("SHA256:   %s\n", hash)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  kscorectl module verify %s\n", outputFile)
	fmt.Printf("  kscorectl module sign %s --key private.pem\n", outputFile)
	fmt.Printf("  kscorectl module publish %s\n", outputFile)

	return nil
}

func validateManifest(m *manifest.Manifest) error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if m.Type == "" {
		return fmt.Errorf("type is required")
	}
	return nil
}

func collectBuildFiles(basePath string, m *manifest.Manifest) ([]string, error) {
	var files []string

	// Always include module.yaml
	files = append(files, "module.yaml")

	// Include lock file if exists
	if _, err := os.Stat(filepath.Join(basePath, "module.lock")); err == nil {
		files = append(files, "module.lock")
	}

	// Include README if exists
	for _, readme := range []string{"README.md", "README.txt", "README"} {
		if _, err := os.Stat(filepath.Join(basePath, readme)); err == nil {
			files = append(files, readme)
			break
		}
	}

	// Include LICENSE if exists
	for _, license := range []string{"LICENSE", "LICENSE.md", "LICENSE.txt"} {
		if _, err := os.Stat(filepath.Join(basePath, license)); err == nil {
			files = append(files, license)
			break
		}
	}

	// Include based on module type
	switch m.Type {
	case "starlark":
		// Include states directory
		if err := includeDirectory(basePath, "states", &files); err != nil {
			return nil, err
		}
		// Include tests directory
		if err := includeDirectory(basePath, "tests", &files); err != nil {
			return nil, err
		}
	case "wasm":
		// Include WASM file
		if m.Entrypoint != "" {
			if _, err := os.Stat(filepath.Join(basePath, m.Entrypoint)); err == nil {
				files = append(files, m.Entrypoint)
			}
		}
	}

	// Include entrypoint if not already included
	if m.Entrypoint != "" {
		found := false
		for _, f := range files {
			if f == m.Entrypoint {
				found = true
				break
			}
		}
		if !found {
			if _, err := os.Stat(filepath.Join(basePath, m.Entrypoint)); err == nil {
				files = append(files, m.Entrypoint)
			}
		}
	}

	return files, nil
}

func includeDirectory(basePath, dirName string, files *[]string) error {
	dirPath := filepath.Join(basePath, dirName)
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return nil // Directory doesn't exist, skip
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}

	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}

		// Skip hidden files
		if strings.HasPrefix(filepath.Base(relPath), ".") {
			return nil
		}

		*files = append(*files, relPath)
		return nil
	})
}

func createZipArchive(outputPath, basePath string, files []string) error {
	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Create ZIP writer
	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	// Add files
	for _, file := range files {
		srcPath := filepath.Join(basePath, file)

		// Open source file
		srcFile, err := os.Open(srcPath)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", file, err)
		}

		// Get file info
		info, err := srcFile.Stat()
		if err != nil {
			srcFile.Close()
			return fmt.Errorf("failed to stat %s: %w", file, err)
		}

		// Create ZIP header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			srcFile.Close()
			return fmt.Errorf("failed to create header for %s: %w", file, err)
		}

		// Use forward slashes for ZIP compatibility
		header.Name = filepath.ToSlash(file)
		header.Method = zip.Deflate
		header.Modified = time.Now()

		// Create entry
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			srcFile.Close()
			return fmt.Errorf("failed to create entry for %s: %w", file, err)
		}

		// Copy content
		if _, err := io.Copy(writer, srcFile); err != nil {
			srcFile.Close()
			return fmt.Errorf("failed to write %s: %w", file, err)
		}

		srcFile.Close()
	}

	return nil
}

func computeFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
