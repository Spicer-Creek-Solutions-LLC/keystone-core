package transfer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

func TestNewImporter_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  ImportConfig
		wantErr bool
	}{
		{"valid", ImportConfig{PackagePath: "/tmp/pkg.tar.gz", OutputDir: "/tmp/out"}, false},
		{"missing package", ImportConfig{OutputDir: "/tmp/out"}, true},
		{"missing output non-preview", ImportConfig{PackagePath: "/tmp/pkg.tar.gz"}, true},
		{"preview no output ok", ImportConfig{PackagePath: "/tmp/pkg.tar.gz", PreviewOnly: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewImporter(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewImporter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func exportTestPackage(t *testing.T, signed bool) (archivePath string, kp *signing.KeyPair) {
	t.Helper()
	outDir := t.TempDir()
	archivePath = filepath.Join(outDir, "export.tar.gz")

	config := ExporterConfig{
		ExportType: ExportTypeAudit,
		OutputPath: archivePath,
		CreatedBy:  "test",
	}

	if signed {
		var err error
		kp, err = signing.GenerateKeyPair(signing.KeyTypeEd25519, 0)
		if err != nil {
			t.Fatalf("GenerateKeyPair: %v", err)
		}
		config.SigningKey = kp.PrivateKey
	}

	exp, err := NewExporter(config)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	exp.AddCollector(&staticCollector{
		name:  "audit",
		lines: []string{`{"action":"login"}`, `{"action":"logout"}`},
	})

	_, err = exp.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	return archivePath, kp
}

func TestImporter_Import_PreviewMode(t *testing.T) {
	archivePath, _ := exportTestPackage(t, false)

	imp, err := NewImporter(ImportConfig{
		PackagePath: archivePath,
		PreviewOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if result.Manifest == nil {
		t.Fatal("Manifest should not be nil")
	}
	if result.Manifest.ExportType != ExportTypeAudit {
		t.Errorf("ExportType = %q, want %q", result.Manifest.ExportType, ExportTypeAudit)
	}
	if len(result.DataSets) != 1 {
		t.Fatalf("DataSets len = %d, want 1", len(result.DataSets))
	}
	if result.DataSets[0].Name != "audit" {
		t.Errorf("DataSet name = %q, want %q", result.DataSets[0].Name, "audit")
	}
	if result.DataSets[0].Path != "" {
		t.Error("Path should be empty in preview mode")
	}
	if !result.ChecksumsValid {
		t.Error("ChecksumsValid should be true")
	}
}

func TestImporter_Import_FullExtraction(t *testing.T) {
	archivePath, _ := exportTestPackage(t, false)
	outputDir := t.TempDir()

	imp, err := NewImporter(ImportConfig{
		PackagePath: archivePath,
		OutputDir:   outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if !result.Verified {
		t.Errorf("Verified = false, want true; warnings: %v", result.Warnings)
	}
	if len(result.DataSets) != 1 {
		t.Fatalf("DataSets len = %d, want 1", len(result.DataSets))
	}

	dsPath := result.DataSets[0].Path
	if dsPath == "" {
		t.Fatal("DataSet path should be set for full extraction")
	}
	if _, err := os.Stat(dsPath); err != nil {
		t.Errorf("extracted data file not found: %v", err)
	}
}

func TestImporter_Import_SignedVerified(t *testing.T) {
	archivePath, kp := exportTestPackage(t, true)
	outputDir := t.TempDir()

	imp, err := NewImporter(ImportConfig{
		PackagePath: archivePath,
		OutputDir:   outputDir,
		TrustedKeys: [][]byte{kp.PublicKey},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if !result.Verified {
		t.Errorf("Verified = false, want true; warnings: %v", result.Warnings)
	}
	if !result.SignatureValid {
		t.Error("SignatureValid should be true")
	}
	if !result.ChecksumsValid {
		t.Error("ChecksumsValid should be true")
	}
}

func TestImporter_Import_WrongKey(t *testing.T) {
	archivePath, _ := exportTestPackage(t, true)
	outputDir := t.TempDir()

	wrongKP, err := signing.GenerateKeyPair(signing.KeyTypeEd25519, 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	imp, err := NewImporter(ImportConfig{
		PackagePath: archivePath,
		OutputDir:   outputDir,
		TrustedKeys: [][]byte{wrongKP.PublicKey},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if result.SignatureValid {
		t.Error("SignatureValid should be false with wrong key")
	}
	if result.Verified {
		t.Error("Verified should be false with wrong key")
	}
	if len(result.Warnings) == 0 {
		t.Error("should have warnings about signature verification")
	}
}

func TestImporter_Import_TamperedChecksum(t *testing.T) {
	archivePath, _ := exportTestPackage(t, false)

	// Extract, tamper with data, re-archive
	tmpDir := t.TempDir()
	if err := extractArchive(archivePath, tmpDir); err != nil {
		t.Fatalf("extractArchive: %v", err)
	}

	// Tamper with the data file
	dataPath := filepath.Join(tmpDir, "data", "audit", "audit-data.jsonl.gz")
	if err := writeTestFile(dataPath, []byte("tampered")); err != nil {
		t.Fatal(err)
	}

	tamperedArchive := filepath.Join(t.TempDir(), "tampered.tar.gz")
	if err := createArchive(tmpDir, tamperedArchive); err != nil {
		t.Fatal(err)
	}

	outputDir := t.TempDir()
	imp, err := NewImporter(ImportConfig{
		PackagePath: tamperedArchive,
		OutputDir:   outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if result.ChecksumsValid {
		t.Error("ChecksumsValid should be false after tampering")
	}
	if result.Verified {
		t.Error("Verified should be false after tampering")
	}
}

func TestImporter_Import_SelectiveDatasets(t *testing.T) {
	outDir := t.TempDir()
	archivePath := filepath.Join(outDir, "multi.tar.gz")

	exp, err := NewExporter(ExporterConfig{
		ExportType: ExportTypeFull,
		OutputPath: archivePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	exp.AddCollector(&staticCollector{name: "audit", lines: []string{`{"a":1}`}})
	exp.AddCollector(&staticCollector{name: "events", lines: []string{`{"e":1}`}})
	exp.AddCollector(&staticCollector{name: "state", lines: []string{`{"s":1}`}})

	if _, err := exp.Export(context.Background()); err != nil {
		t.Fatalf("Export: %v", err)
	}

	outputDir := t.TempDir()
	imp, err := NewImporter(ImportConfig{
		PackagePath: archivePath,
		OutputDir:   outputDir,
		DataSets:    []string{"audit", "state"},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if len(result.DataSets) != 2 {
		t.Fatalf("DataSets len = %d, want 2", len(result.DataSets))
	}

	names := map[string]bool{}
	for _, ds := range result.DataSets {
		names[ds.Name] = true
	}
	if !names["audit"] {
		t.Error("expected audit dataset")
	}
	if !names["state"] {
		t.Error("expected state dataset")
	}
	if names["events"] {
		t.Error("events dataset should have been filtered out")
	}
}

func TestImporter_Import_MissingPackage(t *testing.T) {
	imp, err := NewImporter(ImportConfig{
		PackagePath: "/nonexistent/path.tar.gz",
		OutputDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = imp.Import(context.Background())
	if err == nil {
		t.Error("expected error for missing package")
	}
}

func TestRoundtrip_ExportImport(t *testing.T) {
	kp, err := signing.GenerateKeyPair(signing.KeyTypeEd25519, 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "roundtrip.tar.gz")
	exp, err := NewExporter(ExporterConfig{
		ExportType: ExportTypeFull,
		OutputPath: archivePath,
		CreatedBy:  "roundtrip-test",
		SigningKey:  kp.PrivateKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	exp.AddCollector(&staticCollector{name: "audit", lines: []string{`{"a":1}`, `{"a":2}`}})
	exp.AddCollector(&staticCollector{name: "events", lines: []string{`{"e":1}`}})

	exportManifest, err := exp.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	outputDir := t.TempDir()
	imp, err := NewImporter(ImportConfig{
		PackagePath: archivePath,
		OutputDir:   outputDir,
		TrustedKeys: [][]byte{kp.PublicKey},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := imp.Import(context.Background())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if !result.Verified {
		t.Errorf("Verified = false; warnings: %v", result.Warnings)
	}
	if result.Manifest.CreatedBy != exportManifest.CreatedBy {
		t.Errorf("CreatedBy = %q, want %q", result.Manifest.CreatedBy, exportManifest.CreatedBy)
	}
	if len(result.DataSets) != 2 {
		t.Fatalf("DataSets len = %d, want 2", len(result.DataSets))
	}

	for _, ds := range result.DataSets {
		if ds.Path == "" {
			t.Errorf("DataSet %q: path should be set", ds.Name)
		}
		if _, err := os.Stat(ds.Path); err != nil {
			t.Errorf("DataSet %q: file not found: %v", ds.Name, err)
		}
	}
}
