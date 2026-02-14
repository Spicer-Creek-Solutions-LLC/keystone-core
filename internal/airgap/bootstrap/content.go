package bootstrap

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxExtractSize limits individual file extraction to 1 GiB to prevent decompression bombs.
const maxExtractSize = 1 << 30

// BundleModules copies module archives from srcDir to stagingDir/modules/
// and returns content entries with checksums.
func BundleModules(srcDir, stagingDir string) ([]ContentEntry, error) {
	return bundleDir(srcDir, filepath.Join(stagingDir, "modules"), "modules")
}

// BundleBlueprints copies blueprint directories from srcDir to stagingDir/blueprints/
// and returns content entries with checksums.
func BundleBlueprints(srcDir, stagingDir string) ([]ContentEntry, error) {
	return bundleBlueprintDirs(srcDir, filepath.Join(stagingDir, "blueprints"))
}

// BundlePolicies copies .rego and .cel policy files from srcDir to stagingDir/policies/
// and returns content entries with checksums.
func BundlePolicies(srcDir, stagingDir string) ([]ContentEntry, error) {
	dstDir := filepath.Join(stagingDir, "policies")
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating policies dir: %w", err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading policies dir: %w", err)
	}

	var result []ContentEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".rego" && ext != ".cel" {
			continue
		}

		srcPath := filepath.Join(srcDir, e.Name())
		dstPath := filepath.Join(dstDir, e.Name())
		if err := copyFile(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("copying policy %s: %w", e.Name(), err)
		}

		hash, err := HashFile(dstPath)
		if err != nil {
			return nil, fmt.Errorf("hashing policy %s: %w", e.Name(), err)
		}
		info, _ := os.Stat(dstPath)

		result = append(result, ContentEntry{
			Name:   e.Name(),
			Path:   "policies/" + e.Name(),
			SHA256: hash,
			Size:   info.Size(),
		})
	}

	return result, nil
}

// BundleDocs creates a nested docs archive from srcDir into stagingDir/docs/offline-docs.tar.gz
// and returns the content entry.
func BundleDocs(srcDir, stagingDir string) (*ContentEntry, error) {
	dstDir := filepath.Join(stagingDir, "docs")
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating docs dir: %w", err)
	}

	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil, nil
	}

	archivePath := filepath.Join(dstDir, "offline-docs.tar.gz")
	if err := createArchive(srcDir, archivePath); err != nil {
		return nil, fmt.Errorf("creating docs archive: %w", err)
	}

	hash, err := HashFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("hashing docs archive: %w", err)
	}
	info, _ := os.Stat(archivePath)

	return &ContentEntry{
		Name:   "offline-docs",
		Path:   "docs/offline-docs.tar.gz",
		SHA256: hash,
		Size:   info.Size(),
	}, nil
}

func bundleDir(srcDir, dstDir, prefix string) ([]ContentEntry, error) {
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating %s dir: %w", prefix, err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s dir: %w", prefix, err)
	}

	var result []ContentEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		srcPath := filepath.Join(srcDir, e.Name())
		dstPath := filepath.Join(dstDir, e.Name())
		if err := copyFile(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("copying %s %s: %w", prefix, e.Name(), err)
		}

		hash, err := HashFile(dstPath)
		if err != nil {
			return nil, fmt.Errorf("hashing %s %s: %w", prefix, e.Name(), err)
		}
		info, _ := os.Stat(dstPath)

		result = append(result, ContentEntry{
			Name:   e.Name(),
			Path:   prefix + "/" + e.Name(),
			SHA256: hash,
			Size:   info.Size(),
		})
	}

	return result, nil
}

func bundleBlueprintDirs(srcDir, dstDir string) ([]ContentEntry, error) {
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating blueprints dir: %w", err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading blueprints dir: %w", err)
	}

	var result []ContentEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		srcPath := filepath.Join(srcDir, e.Name())
		archiveName := e.Name() + ".tar.gz"
		dstPath := filepath.Join(dstDir, archiveName)

		if err := createArchive(srcPath, dstPath); err != nil {
			return nil, fmt.Errorf("archiving blueprint %s: %w", e.Name(), err)
		}

		hash, err := HashFile(dstPath)
		if err != nil {
			return nil, fmt.Errorf("hashing blueprint %s: %w", e.Name(), err)
		}
		info, _ := os.Stat(dstPath)

		result = append(result, ContentEntry{
			Name:   e.Name(),
			Path:   "blueprints/" + archiveName,
			SHA256: hash,
			Size:   info.Size(),
		})
	}

	return result, nil
}

// ExtractArchive extracts a tar.gz archive to the destination directory.
func ExtractArchive(archivePath, destDir string) error {
	f, err := os.Open(archivePath) //nolint:gosec // G304: caller controls path
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		target := filepath.Join(destDir, header.Name) //nolint:gosec // G305: validated below
		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar entry: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return fmt.Errorf("creating directory %s: %w", header.Name, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return fmt.Errorf("creating parent dir for %s: %w", header.Name, err)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)) //nolint:gosec // G304: target validated above
			if err != nil {
				return fmt.Errorf("creating file %s: %w", header.Name, err)
			}
			if _, err := io.Copy(out, io.LimitReader(tr, maxExtractSize)); err != nil {
				out.Close()
				return fmt.Errorf("writing file %s: %w", header.Name, err)
			}
			out.Close()
		}
	}

	return nil
}
