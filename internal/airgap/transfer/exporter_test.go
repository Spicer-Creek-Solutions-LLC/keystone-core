package transfer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

// staticCollector is a test DataCollector that writes fixed JSONL lines.
type staticCollector struct {
	name  string
	lines []string
}

func (c *staticCollector) Name() string { return c.name }
func (c *staticCollector) Collect(_ context.Context, w io.Writer, _ *CollectOptions) (*CollectResult, error) {
	result := &CollectResult{}
	for _, line := range c.lines {
		if _, err := w.Write([]byte(line + "\n")); err != nil {
			return nil, err
		}
		result.Count++
	}
	if result.Count > 0 {
		result.TimeRange = TimeRange{
			Start: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			End:   time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		}
	}
	return result, nil
}

func TestNewExporter_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  ExporterConfig
		wantErr bool
	}{
		{"valid", ExporterConfig{ExportType: ExportTypeAudit, OutputPath: "/tmp/test.tar.gz"}, false},
		{"missing type", ExporterConfig{OutputPath: "/tmp/test.tar.gz"}, true},
		{"missing output", ExporterConfig{ExportType: ExportTypeAudit}, true},
		{"encrypt without recipient", ExporterConfig{ExportType: ExportTypeAudit, OutputPath: "/tmp/test.tar.gz", Encrypt: true}, true},
		{"encrypt with recipient", ExporterConfig{ExportType: ExportTypeAudit, OutputPath: "/tmp/test.tar.gz", Encrypt: true, AgeRecipient: "age1key"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewExporter(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewExporter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExporter_Export_NoCollectors(t *testing.T) {
	exp, err := NewExporter(ExporterConfig{
		ExportType: ExportTypeAudit,
		OutputPath: filepath.Join(t.TempDir(), "test.tar.gz"),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = exp.Export(context.Background())
	if err == nil {
		t.Error("expected error for no collectors")
	}
}

func TestExporter_Export_Unsigned(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "export.tar.gz")
	exp, err := NewExporter(ExporterConfig{
		ExportType: ExportTypeAudit,
		OutputPath: outPath,
		CreatedBy:  "test-user",
	})
	if err != nil {
		t.Fatal(err)
	}

	exp.AddCollector(&staticCollector{
		name:  "audit",
		lines: []string{`{"action":"login"}`, `{"action":"logout"}`},
	})

	manifest, err := exp.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if manifest.ExportType != ExportTypeAudit {
		t.Errorf("ExportType = %q, want %q", manifest.ExportType, ExportTypeAudit)
	}
	if manifest.CreatedBy != "test-user" {
		t.Errorf("CreatedBy = %q, want %q", manifest.CreatedBy, "test-user")
	}
	if len(manifest.DataSets) != 1 {
		t.Fatalf("DataSets len = %d, want 1", len(manifest.DataSets))
	}
	if manifest.DataSets[0].Name != "audit" {
		t.Errorf("DataSet name = %q, want %q", manifest.DataSets[0].Name, "audit")
	}
	if manifest.DataSets[0].Count != 2 {
		t.Errorf("DataSet count = %d, want 2", manifest.DataSets[0].Count)
	}
	if manifest.RequiresVerification {
		t.Error("RequiresVerification should be false for unsigned export")
	}

	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("archive not created: %v", err)
	}
}

func TestExporter_Export_Signed(t *testing.T) {
	kp, err := signing.GenerateKeyPair(signing.KeyTypeEd25519, 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "signed-export.tar.gz")
	exp, err := NewExporter(ExporterConfig{
		ExportType: ExportTypeEvents,
		OutputPath: outPath,
		SigningKey:  kp.PrivateKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	exp.AddCollector(&staticCollector{
		name:  "events",
		lines: []string{`{"id":"evt-1"}`, `{"id":"evt-2"}`, `{"id":"evt-3"}`},
	})

	manifest, err := exp.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if !manifest.RequiresVerification {
		t.Error("RequiresVerification should be true for signed export")
	}
	if manifest.Checksum == "" {
		t.Error("Checksum should be set")
	}
}

func TestExporter_Export_MultipleCollectors(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "full-export.tar.gz")
	exp, err := NewExporter(ExporterConfig{
		ExportType: ExportTypeFull,
		OutputPath: outPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	exp.AddCollector(&staticCollector{name: "audit", lines: []string{`{"a":1}`}})
	exp.AddCollector(&staticCollector{name: "events", lines: []string{`{"e":1}`, `{"e":2}`}})
	exp.AddCollector(&staticCollector{name: "state", lines: []string{`{"s":1}`}})

	manifest, err := exp.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if len(manifest.DataSets) != 3 {
		t.Fatalf("DataSets len = %d, want 3", len(manifest.DataSets))
	}

	counts := map[string]int64{}
	for _, ds := range manifest.DataSets {
		counts[ds.Name] = ds.Count
	}
	if counts["audit"] != 1 {
		t.Errorf("audit count = %d, want 1", counts["audit"])
	}
	if counts["events"] != 2 {
		t.Errorf("events count = %d, want 2", counts["events"])
	}
	if counts["state"] != 1 {
		t.Errorf("state count = %d, want 1", counts["state"])
	}
}

func TestExporter_Export_EmptyCollector(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "empty-export.tar.gz")
	exp, err := NewExporter(ExporterConfig{
		ExportType: ExportTypeAudit,
		OutputPath: outPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	exp.AddCollector(&staticCollector{name: "audit", lines: nil})

	manifest, err := exp.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if manifest.DataSets[0].Count != 0 {
		t.Errorf("Count = %d, want 0", manifest.DataSets[0].Count)
	}
}

func TestExporter_Export_ContextCancelled(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "cancelled.tar.gz")
	exp, err := NewExporter(ExporterConfig{
		ExportType: ExportTypeAudit,
		OutputPath: outPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	exp.AddCollector(&staticCollector{name: "audit", lines: []string{`{}`}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = exp.Export(ctx)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestCollectToGzip(t *testing.T) {
	gzPath := filepath.Join(t.TempDir(), "test.jsonl.gz")
	c := &staticCollector{
		name:  "test",
		lines: []string{`{"a":1}`, `{"b":2}`},
	}

	result, err := collectToGzip(context.Background(), c, gzPath, nil)
	if err != nil {
		t.Fatalf("collectToGzip: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("Count = %d, want 2", result.Count)
	}

	if _, err := os.Stat(gzPath); err != nil {
		t.Errorf("gzip file not created: %v", err)
	}
}

func TestHashFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	if err := writeTestFile(path, []byte("hello")); err != nil {
		t.Fatal(err)
	}

	hash, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
}

func TestCalculateChecksum(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(filepath.Join(dataDir, "test.jsonl.gz"), []byte("data")); err != nil {
		t.Fatal(err)
	}

	checksum, err := calculateChecksum(dir)
	if err != nil {
		t.Fatalf("calculateChecksum: %v", err)
	}
	if len(checksum) != 64 {
		t.Errorf("checksum length = %d, want 64", len(checksum))
	}

	// Same content should produce same checksum
	checksum2, err := calculateChecksum(dir)
	if err != nil {
		t.Fatalf("calculateChecksum: %v", err)
	}
	if checksum != checksum2 {
		t.Error("checksum should be deterministic")
	}
}

func TestCalculateChecksum_ExcludesManifestAndSignatures(t *testing.T) {
	dir := t.TempDir()
	if err := writeTestFile(filepath.Join(dir, "data.jsonl.gz"), []byte("data")); err != nil {
		t.Fatal(err)
	}

	baseChecksum, err := calculateChecksum(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Adding manifest.json should not change checksum
	if err := writeTestFile(filepath.Join(dir, "manifest.json"), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	withManifest, err := calculateChecksum(dir)
	if err != nil {
		t.Fatal(err)
	}
	if withManifest != baseChecksum {
		t.Error("manifest.json should be excluded from checksum")
	}

	// Adding signatures/ should not change checksum
	sigDir := filepath.Join(dir, "signatures")
	if err := os.MkdirAll(sigDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(filepath.Join(sigDir, "manifest.json.sig"), []byte("sig")); err != nil {
		t.Fatal(err)
	}
	withSig, err := calculateChecksum(dir)
	if err != nil {
		t.Fatal(err)
	}
	if withSig != baseChecksum {
		t.Error("signatures/ should be excluded from checksum")
	}
}

// errCollector always returns an error.
type errCollector struct{ name string }

func (c *errCollector) Name() string { return c.name }
func (c *errCollector) Collect(_ context.Context, _ io.Writer, _ *CollectOptions) (*CollectResult, error) {
	return nil, fmt.Errorf("simulated error")
}

func TestExporter_Export_CollectorError(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "err.tar.gz")
	exp, err := NewExporter(ExporterConfig{
		ExportType: ExportTypeAudit,
		OutputPath: outPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	exp.AddCollector(&errCollector{name: "audit"})

	_, err = exp.Export(context.Background())
	if err == nil {
		t.Error("expected error from failing collector")
	}
}

var _ DataCollector = (*staticCollector)(nil)
var _ DataCollector = (*errCollector)(nil)
