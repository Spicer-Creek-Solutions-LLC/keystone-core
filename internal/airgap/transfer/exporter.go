package transfer

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

// ExporterConfig configures an export operation.
type ExporterConfig struct {
	ExportType   ExportType
	OutputPath   string
	TimeRange    *TimeRange
	CreatedBy    string
	SigningKey   []byte // PEM private key; nil = unsigned
	Encrypt      bool
	AgeRecipient string // age public key for encryption
}

// Exporter builds export packages from registered data collectors.
type Exporter struct {
	config     ExporterConfig
	collectors []DataCollector
}

// NewExporter creates a new exporter.
func NewExporter(config ExporterConfig) (*Exporter, error) {
	if config.ExportType == "" {
		return nil, fmt.Errorf("export_type is required")
	}
	if config.OutputPath == "" {
		return nil, fmt.Errorf("output_path is required")
	}
	if config.Encrypt && config.AgeRecipient == "" {
		return nil, fmt.Errorf("age recipient required when encryption is enabled")
	}
	return &Exporter{config: config}, nil
}

// AddCollector registers a data collector.
func (e *Exporter) AddCollector(c DataCollector) {
	e.collectors = append(e.collectors, c)
}

// Export runs all collectors and produces a signed, optionally encrypted archive.
func (e *Exporter) Export(ctx context.Context) (*ExportManifest, error) {
	if len(e.collectors) == 0 {
		return nil, fmt.Errorf("no collectors registered")
	}

	staging, err := os.MkdirTemp("", "kscore-transfer-*")
	if err != nil {
		return nil, fmt.Errorf("creating staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	manifest := &ExportManifest{
		SchemaVersion:        SchemaVersion,
		ExportType:           e.config.ExportType,
		Created:              time.Now().UTC(),
		CreatedBy:            e.config.CreatedBy,
		RequiresVerification: e.config.SigningKey != nil,
		Encrypted:            e.config.Encrypt,
	}
	if e.config.Encrypt {
		manifest.EncryptionRecipient = e.config.AgeRecipient
	}

	var opts *CollectOptions
	if e.config.TimeRange != nil {
		opts = &CollectOptions{TimeRange: e.config.TimeRange}
	}

	var earliest, latest time.Time
	firstDataset := true

	for _, c := range e.collectors {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		dsDir := filepath.Join(staging, "data", c.Name())
		if err := os.MkdirAll(dsDir, 0o750); err != nil {
			return nil, fmt.Errorf("creating data dir for %s: %w", c.Name(), err)
		}

		jsonlPath := filepath.Join(dsDir, c.Name()+"-data.jsonl")
		gzPath := jsonlPath + ".gz"

		result, err := collectToGzip(ctx, c, gzPath, opts)
		if err != nil {
			return nil, fmt.Errorf("collecting %s: %w", c.Name(), err)
		}

		hash, err := hashFile(gzPath)
		if err != nil {
			return nil, fmt.Errorf("hashing %s: %w", c.Name(), err)
		}

		info, err := os.Stat(gzPath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", c.Name(), err)
		}

		relPath := filepath.Join("data", c.Name(), c.Name()+"-data.jsonl.gz")
		manifest.DataSets = append(manifest.DataSets, DataSetInfo{
			Name:      c.Name(),
			Path:      relPath,
			SHA256:    hash,
			Size:      info.Size(),
			Count:     result.Count,
			TimeRange: result.TimeRange,
		})

		if result.Count > 0 {
			if firstDataset {
				earliest = result.TimeRange.Start
				latest = result.TimeRange.End
				firstDataset = false
			} else {
				if result.TimeRange.Start.Before(earliest) {
					earliest = result.TimeRange.Start
				}
				if result.TimeRange.End.After(latest) {
					latest = result.TimeRange.End
				}
			}
		}
	}

	if !firstDataset {
		manifest.TimeRange = TimeRange{Start: earliest, End: latest}
	}

	checksum, err := calculateChecksum(staging)
	if err != nil {
		return nil, fmt.Errorf("calculating checksum: %w", err)
	}
	manifest.Checksum = checksum

	manifestPath := filepath.Join(staging, "manifest.json")
	if err := WriteManifest(manifest, manifestPath); err != nil {
		return nil, fmt.Errorf("writing manifest: %w", err)
	}

	if e.config.SigningKey != nil {
		if err := signManifest(ctx, e.config.SigningKey, staging); err != nil {
			return nil, err
		}
	}

	archivePath := e.config.OutputPath
	if err := createArchive(staging, archivePath); err != nil {
		return nil, fmt.Errorf("creating archive: %w", err)
	}

	if e.config.Encrypt {
		encPath := archivePath + ".age"
		if err := encryptFile(ctx, e.config.AgeRecipient, archivePath, encPath); err != nil {
			return nil, fmt.Errorf("encrypting archive: %w", err)
		}
		if err := os.Remove(archivePath); err != nil {
			return nil, fmt.Errorf("removing unencrypted archive: %w", err)
		}
	}

	return manifest, nil
}

func collectToGzip(ctx context.Context, c DataCollector, gzPath string, opts *CollectOptions) (*CollectResult, error) {
	f, err := os.Create(gzPath) //nolint:gosec // G304: path from staging dir
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	result, err := c.Collect(ctx, gw, opts)
	if closeErr := gw.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return result, err
}

func signManifest(ctx context.Context, privateKeyPEM []byte, packageDir string) error {
	signer, err := signing.NewKeySigner(&signing.KeySignerConfig{
		PrivateKeyPEM: privateKeyPEM,
		HashAlgorithm: signing.HashSHA256,
		Format:        signing.FormatBase64,
	})
	if err != nil {
		return fmt.Errorf("creating signer: %w", err)
	}

	manifestPath := filepath.Join(packageDir, "manifest.json")
	sigDir := filepath.Join(packageDir, "signatures")
	if err := os.MkdirAll(sigDir, 0o750); err != nil {
		return fmt.Errorf("creating signatures directory: %w", err)
	}

	result, err := signer.SignFile(ctx, manifestPath)
	if err != nil {
		return fmt.Errorf("signing manifest: %w", err)
	}

	sigPath := filepath.Join(sigDir, "manifest.json.sig")
	//nolint:gosec // G306: signature files need to be readable for verification
	if err := os.WriteFile(sigPath, []byte(result.SignatureBase64), 0o644); err != nil {
		return fmt.Errorf("writing signature: %w", err)
	}

	pubKey, err := signer.PublicKey()
	if err != nil {
		return fmt.Errorf("getting public key: %w", err)
	}
	pubPath := filepath.Join(sigDir, "cosign.pub")
	//nolint:gosec // G306: public keys need to be readable for verification
	if err := os.WriteFile(pubPath, pubKey, 0o644); err != nil {
		return fmt.Errorf("writing public key: %w", err)
	}

	return nil
}

func encryptFile(ctx context.Context, recipient, inputPath, outputPath string) error {
	//nolint:gosec // G204: age CLI execution is intentional for export encryption
	cmd := exec.CommandContext(ctx, "age", "-r", recipient, "-o", outputPath, inputPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("age encryption failed: %w", err)
	}
	return nil
}

func decryptFile(ctx context.Context, identityPath, inputPath, outputPath string) error {
	//nolint:gosec // G204: age CLI execution is intentional for export decryption
	cmd := exec.CommandContext(ctx, "age", "-d", "-i", identityPath, "-o", outputPath, inputPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("age decryption failed: %w", err)
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // G304: caller controls path
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func calculateChecksum(dir string) (string, error) {
	var paths []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "manifest.json" {
			return nil
		}
		if rel == "signatures" || (len(rel) > 11 && rel[:11] == "signatures/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking directory: %w", err)
	}

	sort.Strings(paths)

	h := sha256.New()
	for _, rel := range paths {
		fmt.Fprintf(h, "%s\n", rel)
		f, err := os.Open(filepath.Join(dir, rel)) //nolint:gosec // G304: paths from filepath.Walk within dir
		if err != nil {
			return "", fmt.Errorf("opening %s: %w", rel, err)
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", fmt.Errorf("hashing %s: %w", rel, err)
		}
		f.Close()
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
