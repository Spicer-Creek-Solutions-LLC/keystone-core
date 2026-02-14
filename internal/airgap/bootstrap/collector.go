package bootstrap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CoreBinaries returns the names of binaries that should always be present.
func CoreBinaries() []string {
	return []string{
		"kscore-server",
		"kscore-agent",
		"kscorectl",
	}
}

// CollectResult holds the outcome of binary collection.
type CollectResult struct {
	Entries  []ComponentEntry
	Warnings []string
}

// CollectBinaries scans buildDir/{platform.OS}/{platform.Arch}/ for kscore-*
// and kscorectl binaries, copies them to stagingDir/bin/, and returns entries.
// Missing core binaries produce warnings but do not fail the collection.
func CollectBinaries(buildDir string, platform Platform, version, stagingDir string) (*CollectResult, error) {
	srcDir := filepath.Join(buildDir, platform.OS, platform.Arch)
	dstDir := filepath.Join(stagingDir, "bin")
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating bin dir: %w", err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("reading build directory %s: %w", srcDir, err)
	}

	result := &CollectResult{}
	collected := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isBinaryName(name) {
			continue
		}

		srcPath := filepath.Join(srcDir, name)
		dstPath := filepath.Join(dstDir, name)

		if err := copyFile(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("copying %s: %w", name, err)
		}

		hash, err := HashFile(dstPath)
		if err != nil {
			return nil, fmt.Errorf("hashing %s: %w", name, err)
		}

		info, err := os.Stat(dstPath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", name, err)
		}

		result.Entries = append(result.Entries, ComponentEntry{
			Name:    name,
			Version: version,
			Path:    "bin/" + name,
			SHA256:  hash,
			Size:    info.Size(),
		})
		collected[name] = true
	}

	for _, core := range CoreBinaries() {
		if !collected[core] {
			result.Warnings = append(result.Warnings, fmt.Sprintf("core binary %q not found in %s", core, srcDir))
		}
	}

	return result, nil
}

func isBinaryName(name string) bool {
	return name == "kscorectl" || strings.HasPrefix(name, "kscore-")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // G304: source path from controlled build dir
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode()) //nolint:gosec // G304: dst path from controlled staging dir
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
