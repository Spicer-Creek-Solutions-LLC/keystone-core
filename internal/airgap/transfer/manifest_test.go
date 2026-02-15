package transfer

import (
	"path/filepath"
	"testing"
	"time"
)

func validManifest() *ExportManifest {
	return &ExportManifest{
		SchemaVersion: SchemaVersion,
		ExportType:    ExportTypeAudit,
		Created:       time.Now().UTC(),
		TimeRange: TimeRange{
			Start: time.Now().Add(-24 * time.Hour),
			End:   time.Now(),
		},
		DataSets: []DataSetInfo{
			{
				Name:   "audit",
				Path:   "data/audit/audit-data.jsonl.gz",
				SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				Size:   1024,
				Count:  100,
			},
		},
		Checksum: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
}

func TestExportManifest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(m *ExportManifest)
		wantErr bool
	}{
		{"valid", func(m *ExportManifest) {}, false},
		{"missing schema_version", func(m *ExportManifest) { m.SchemaVersion = "" }, true},
		{"missing export_type", func(m *ExportManifest) { m.ExportType = "" }, true},
		{"unknown export_type", func(m *ExportManifest) { m.ExportType = "badtype" }, true},
		{"no data_sets", func(m *ExportManifest) { m.DataSets = nil }, true},
		{"dataset missing name", func(m *ExportManifest) { m.DataSets[0].Name = "" }, true},
		{"dataset missing path", func(m *ExportManifest) { m.DataSets[0].Path = "" }, true},
		{"dataset absolute path", func(m *ExportManifest) { m.DataSets[0].Path = "/etc/passwd" }, true},
		{"dataset path traversal", func(m *ExportManifest) { m.DataSets[0].Path = "../escape" }, true},
		{"dataset invalid sha256", func(m *ExportManifest) { m.DataSets[0].SHA256 = "not-a-hash" }, true},
		{"dataset duplicate name", func(m *ExportManifest) {
			m.DataSets = append(m.DataSets, DataSetInfo{
				Name: "audit", Path: "data/audit/other.jsonl.gz",
			})
		}, true},
		{"invalid checksum", func(m *ExportManifest) { m.Checksum = "badhash" }, true},
		{"empty checksum ok", func(m *ExportManifest) { m.Checksum = "" }, false},
		{"empty sha256 ok", func(m *ExportManifest) { m.DataSets[0].SHA256 = "" }, false},
		{"events type", func(m *ExportManifest) { m.ExportType = ExportTypeEvents }, false},
		{"state type", func(m *ExportManifest) { m.ExportType = ExportTypeState }, false},
		{"inventory type", func(m *ExportManifest) { m.ExportType = ExportTypeInventory }, false},
		{"full type", func(m *ExportManifest) { m.ExportType = ExportTypeFull }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifest()
			tt.modify(m)
			err := m.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWriteReadManifest(t *testing.T) {
	m := validManifest()
	m.CreatedBy = "test-user"
	m.Encrypted = true
	m.EncryptionRecipient = "age1testkey"
	m.RequiresVerification = true

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := WriteManifest(m, path); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	got, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}

	if got.SchemaVersion != m.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", got.SchemaVersion, m.SchemaVersion)
	}
	if got.ExportType != m.ExportType {
		t.Errorf("ExportType = %q, want %q", got.ExportType, m.ExportType)
	}
	if got.CreatedBy != m.CreatedBy {
		t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, m.CreatedBy)
	}
	if len(got.DataSets) != len(m.DataSets) {
		t.Errorf("DataSets len = %d, want %d", len(got.DataSets), len(m.DataSets))
	}
	if got.Encrypted != m.Encrypted {
		t.Errorf("Encrypted = %v, want %v", got.Encrypted, m.Encrypted)
	}
	if got.RequiresVerification != m.RequiresVerification {
		t.Errorf("RequiresVerification = %v, want %v", got.RequiresVerification, m.RequiresVerification)
	}
}

func TestReadManifest_NotFound(t *testing.T) {
	_, err := ReadManifest(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadManifest_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := writeTestFile(path, []byte("not json")); err != nil {
		t.Fatal(err)
	}
	_, err := ReadManifest(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
