package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ArtifactBuilder builds backup artifacts
type ArtifactBuilder struct {
	sourceDir string
	logger    Logger
}

// NewArtifactBuilder creates a new artifact builder
func NewArtifactBuilder(sourceDir string, logger Logger) (*ArtifactBuilder, error) {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &ArtifactBuilder{
		sourceDir: sourceDir,
		logger:    logger,
	}, nil
}

// Build creates a backup artifact from the source directory
func (b *ArtifactBuilder) Build(ctx context.Context, outputPath string, manifest *BackupManifest, compression CompressionType) error {
	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Create gzip writer if compression is enabled
	var tarWriter *tar.Writer
	var gzWriter *gzip.Writer

	switch compression {
	case CompressionTypeGzip:
		gzWriter = gzip.NewWriter(outFile)
		defer gzWriter.Close()
		tarWriter = tar.NewWriter(gzWriter)
	case CompressionTypeNone:
		tarWriter = tar.NewWriter(outFile)
	default:
		gzWriter = gzip.NewWriter(outFile)
		defer gzWriter.Close()
		tarWriter = tar.NewWriter(gzWriter)
	}
	defer tarWriter.Close()

	// Track files for manifest
	manifest.Files = make([]ManifestFile, 0)

	// Walk source directory and add files to archive
	err = filepath.Walk(b.sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check context
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip the root directory
		if path == b.sourceDir {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(b.sourceDir, path)
		if err != nil {
			return err
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			header.Linkname = link
		}

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Write file content if regular file
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			// Calculate checksum while copying
			h := newHash()
			tee := io.TeeReader(file, h)

			if _, err := io.Copy(tarWriter, tee); err != nil {
				return err
			}

			// Add to manifest
			manifest.Files = append(manifest.Files, ManifestFile{
				Path:     relPath,
				Size:     info.Size(),
				Mode:     info.Mode(),
				ModTime:  info.ModTime(),
				Checksum: fmt.Sprintf("sha256:%x", h.Sum(nil)),
			})
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to add files to archive: %w", err)
	}

	// Add manifest to archive
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	manifestHeader := &tar.Header{
		Name:    "manifest.json",
		Size:    int64(len(manifestData)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	if err := tarWriter.WriteHeader(manifestHeader); err != nil {
		return fmt.Errorf("failed to write manifest header: %w", err)
	}
	if _, err := tarWriter.Write(manifestData); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	b.logger.Debug("built backup artifact", "output", outputPath, "files", len(manifest.Files))
	return nil
}

// ArtifactReader reads backup artifacts
type ArtifactReader struct {
	artifactPath string
	logger       Logger
}

// NewArtifactReader creates a new artifact reader
func NewArtifactReader(artifactPath string, logger Logger) *ArtifactReader {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &ArtifactReader{
		artifactPath: artifactPath,
		logger:       logger,
	}
}

// ReadManifest reads the manifest from a backup artifact
func (r *ArtifactReader) ReadManifest(ctx context.Context) (*BackupManifest, error) {
	file, err := os.Open(r.artifactPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open artifact: %w", err)
	}
	defer file.Close()

	// Detect if gzipped
	gzReader, err := gzip.NewReader(file)
	var tarReader *tar.Reader
	if err != nil {
		// Not gzipped, seek back and use raw tar
		file.Seek(0, 0)
		tarReader = tar.NewReader(file)
	} else {
		defer gzReader.Close()
		tarReader = tar.NewReader(gzReader)
	}

	// Find manifest.json
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("manifest.json not found in artifact")
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		if header.Name == "manifest.json" {
			var manifest BackupManifest
			if err := json.NewDecoder(tarReader).Decode(&manifest); err != nil {
				return nil, fmt.Errorf("failed to decode manifest: %w", err)
			}
			return &manifest, nil
		}
	}
}

// Extract extracts the backup artifact to a directory
func (r *ArtifactReader) Extract(ctx context.Context, destDir string) error {
	file, err := os.Open(r.artifactPath)
	if err != nil {
		return fmt.Errorf("failed to open artifact: %w", err)
	}
	defer file.Close()

	// Detect if gzipped
	gzReader, err := gzip.NewReader(file)
	var tarReader *tar.Reader
	if err != nil {
		// Not gzipped, seek back and use raw tar
		file.Seek(0, 0)
		tarReader = tar.NewReader(file)
	} else {
		defer gzReader.Close()
		tarReader = tar.NewReader(gzReader)
	}

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Extract files
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Construct target path (prevent path traversal)
		target := filepath.Join(destDir, header.Name)
		if !isSubPath(destDir, target) {
			return fmt.Errorf("illegal path in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

		case tar.TypeReg:
			// Create parent directory
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			// Create file
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			outFile.Close()

		case tar.TypeSymlink:
			// Remove existing symlink
			os.Remove(target)
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("failed to create symlink: %w", err)
			}
		}
	}

	r.logger.Debug("extracted backup artifact", "dest", destDir)
	return nil
}

// VerifyIntegrity verifies the integrity of files in the artifact
func (r *ArtifactReader) VerifyIntegrity(ctx context.Context) (*VerificationResult, error) {
	manifest, err := r.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}

	result := &VerificationResult{
		Valid:       true,
		TotalFiles:  len(manifest.Files),
		VerifiedAt:  time.Now(),
		FileResults: make([]FileVerificationResult, 0, len(manifest.Files)),
	}

	// Open artifact again for verification
	file, err := os.Open(r.artifactPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open artifact: %w", err)
	}
	defer file.Close()

	// Detect if gzipped
	gzReader, err := gzip.NewReader(file)
	var tarReader *tar.Reader
	if err != nil {
		file.Seek(0, 0)
		tarReader = tar.NewReader(file)
	} else {
		defer gzReader.Close()
		tarReader = tar.NewReader(gzReader)
	}

	// Build map of expected checksums
	expectedChecksums := make(map[string]string)
	for _, f := range manifest.Files {
		expectedChecksums[f.Path] = f.Checksum
	}

	// Verify each file
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		// Skip manifest
		if header.Name == "manifest.json" {
			continue
		}

		// Only verify regular files
		if header.Typeflag != tar.TypeReg {
			continue
		}

		// Calculate checksum
		h := newHash()
		if _, err := io.Copy(h, tarReader); err != nil {
			return nil, fmt.Errorf("failed to read file for verification: %w", err)
		}
		actualChecksum := fmt.Sprintf("sha256:%x", h.Sum(nil))

		fileResult := FileVerificationResult{
			Path:             header.Name,
			ExpectedChecksum: expectedChecksums[header.Name],
			ActualChecksum:   actualChecksum,
			Valid:            actualChecksum == expectedChecksums[header.Name],
		}

		if !fileResult.Valid {
			result.Valid = false
			result.FailedFiles++
		} else {
			result.ValidFiles++
		}

		result.FileResults = append(result.FileResults, fileResult)
	}

	return result, nil
}

// VerificationResult holds the result of artifact verification
type VerificationResult struct {
	Valid       bool                     `json:"valid"`
	TotalFiles  int                      `json:"total_files"`
	ValidFiles  int                      `json:"valid_files"`
	FailedFiles int                      `json:"failed_files"`
	VerifiedAt  time.Time                `json:"verified_at"`
	FileResults []FileVerificationResult `json:"file_results,omitempty"`
}

// FileVerificationResult holds verification result for a single file
type FileVerificationResult struct {
	Path             string `json:"path"`
	ExpectedChecksum string `json:"expected_checksum"`
	ActualChecksum   string `json:"actual_checksum"`
	Valid            bool   `json:"valid"`
	Error            string `json:"error,omitempty"`
}

// isSubPath checks if target is a subpath of base (prevents path traversal)
func isSubPath(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return !filepath.IsAbs(rel) && rel != ".." && !hasPathPrefix(rel, "..")
}

// hasPathPrefix checks if path starts with prefix
func hasPathPrefix(path, prefix string) bool {
	pathParts := filepath.SplitList(path)
	if len(pathParts) > 0 && pathParts[0] == prefix {
		return true
	}
	// Also check for "../" prefix in string form
	return len(path) >= 3 && path[:3] == "../"
}

// ExtractFile extracts a single file from the artifact
func (r *ArtifactReader) ExtractFile(ctx context.Context, fileName string, w io.Writer) error {
	file, err := os.Open(r.artifactPath)
	if err != nil {
		return fmt.Errorf("failed to open artifact: %w", err)
	}
	defer file.Close()

	// Detect if gzipped
	gzReader, err := gzip.NewReader(file)
	var tarReader *tar.Reader
	if err != nil {
		file.Seek(0, 0)
		tarReader = tar.NewReader(file)
	} else {
		defer gzReader.Close()
		tarReader = tar.NewReader(gzReader)
	}

	// Find the file
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			return fmt.Errorf("file not found in artifact: %s", fileName)
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		if header.Name == fileName {
			if _, err := io.Copy(w, tarReader); err != nil {
				return fmt.Errorf("failed to extract file: %w", err)
			}
			return nil
		}
	}
}

// ListFiles lists all files in the artifact
func (r *ArtifactReader) ListFiles(ctx context.Context) ([]string, error) {
	file, err := os.Open(r.artifactPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open artifact: %w", err)
	}
	defer file.Close()

	// Detect if gzipped
	gzReader, err := gzip.NewReader(file)
	var tarReader *tar.Reader
	if err != nil {
		file.Seek(0, 0)
		tarReader = tar.NewReader(file)
	} else {
		defer gzReader.Close()
		tarReader = tar.NewReader(gzReader)
	}

	var files []string
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		files = append(files, header.Name)
	}

	return files, nil
}
