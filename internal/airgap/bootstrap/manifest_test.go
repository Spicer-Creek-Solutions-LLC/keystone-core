package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManifest_Validate(t *testing.T) {
	validManifest := func() *Manifest {
		return &Manifest{
			SchemaVersion: SchemaVersion,
			Version:       "0.1.0",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			Created:       time.Now(),
			Components: []ComponentEntry{
				{Name: "kscore-server", Version: "0.1.0", Path: "bin/kscore-server", SHA256: "a" + repeatChar('0', 63), Size: 100},
			},
		}
	}

	tests := []struct {
		name    string
		modify  func(m *Manifest)
		wantErr bool
	}{
		{"valid", func(m *Manifest) {}, false},
		{"missing schema_version", func(m *Manifest) { m.SchemaVersion = "" }, true},
		{"missing version", func(m *Manifest) { m.Version = "" }, true},
		{"invalid platform", func(m *Manifest) { m.Platform.OS = "freebsd" }, true},
		{"no components", func(m *Manifest) { m.Components = nil }, true},
		{"component missing name", func(m *Manifest) { m.Components[0].Name = "" }, true},
		{"component missing path", func(m *Manifest) { m.Components[0].Path = "" }, true},
		{"component absolute path", func(m *Manifest) { m.Components[0].Path = "/bin/kscore-server" }, true},
		{"component bad sha256", func(m *Manifest) { m.Components[0].SHA256 = "xyz" }, true},
		{"component empty sha256 ok", func(m *Manifest) { m.Components[0].SHA256 = "" }, false},
		{"duplicate component", func(m *Manifest) {
			m.Components = append(m.Components, m.Components[0])
		}, true},
		{"module missing path", func(m *Manifest) {
			m.Modules = []ContentEntry{{Name: "std/core"}}
		}, true},
		{"bad overall checksum", func(m *Manifest) { m.Checksum = "short" }, true},
		{"valid overall checksum", func(m *Manifest) {
			m.Checksum = repeatChar('a', 64)
		}, false},
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

func TestManifestJSONRoundtrip(t *testing.T) {
	original := &Manifest{
		SchemaVersion: SchemaVersion,
		Version:       "0.1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Created:       time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		CreatedBy:     "test",
		Components: []ComponentEntry{
			{Name: "kscore-server", Version: "0.1.0", Path: "bin/kscore-server", SHA256: repeatChar('a', 64), Size: 1024},
		},
		Modules: []ContentEntry{
			{Name: "std/core", Version: "0.1.0", Path: "modules/std-core.tar.gz", SHA256: repeatChar('b', 64), Size: 512},
		},
		Checksum:             repeatChar('c', 64),
		RequiresVerification: true,
	}

	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.SchemaVersion != original.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", decoded.SchemaVersion, original.SchemaVersion)
	}
	if decoded.Version != original.Version {
		t.Errorf("Version = %q, want %q", decoded.Version, original.Version)
	}
	if decoded.Platform != original.Platform {
		t.Errorf("Platform = %v, want %v", decoded.Platform, original.Platform)
	}
	if len(decoded.Components) != 1 {
		t.Fatalf("Components len = %d, want 1", len(decoded.Components))
	}
	if decoded.Components[0].Name != "kscore-server" {
		t.Errorf("Component name = %q, want %q", decoded.Components[0].Name, "kscore-server")
	}
	if len(decoded.Modules) != 1 {
		t.Fatalf("Modules len = %d, want 1", len(decoded.Modules))
	}
	if decoded.RequiresVerification != true {
		t.Error("RequiresVerification should be true")
	}
}

func TestWriteReadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	original := &Manifest{
		SchemaVersion: SchemaVersion,
		Version:       "0.1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Created:       time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Components: []ComponentEntry{
			{Name: "kscore-server", Version: "0.1.0", Path: "bin/kscore-server"},
		},
	}

	if err := WriteManifest(original, path); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	loaded, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}

	if loaded.Version != original.Version {
		t.Errorf("Version = %q, want %q", loaded.Version, original.Version)
	}
	if len(loaded.Components) != 1 {
		t.Fatalf("Components len = %d, want 1", len(loaded.Components))
	}
}

func TestReadManifest_NotFound(t *testing.T) {
	_, err := ReadManifest("/nonexistent/manifest.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestCalculateChecksum(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	writeTestFile(t, filepath.Join(dir, "bin", "server"), "server-binary")
	writeTestFile(t, filepath.Join(dir, "bin", "agent"), "agent-binary")

	sum1, err := CalculateChecksum(dir)
	if err != nil {
		t.Fatalf("CalculateChecksum: %v", err)
	}
	if len(sum1) != 64 {
		t.Errorf("checksum length = %d, want 64", len(sum1))
	}

	// Same content should produce same checksum
	sum2, err := CalculateChecksum(dir)
	if err != nil {
		t.Fatalf("CalculateChecksum: %v", err)
	}
	if sum1 != sum2 {
		t.Error("same dir should produce same checksum")
	}

	// Modify a file and verify checksum changes
	writeTestFile(t, filepath.Join(dir, "bin", "server"), "modified-binary")
	sum3, err := CalculateChecksum(dir)
	if err != nil {
		t.Fatalf("CalculateChecksum: %v", err)
	}
	if sum1 == sum3 {
		t.Error("modified dir should produce different checksum")
	}
}

func TestCalculateChecksum_ExcludesManifestAndSignatures(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "bin", "server"), "server")
	sum1, err := CalculateChecksum(dir)
	if err != nil {
		t.Fatalf("CalculateChecksum: %v", err)
	}

	// Adding manifest.json should not change the checksum
	writeTestFile(t, filepath.Join(dir, "manifest.json"), `{"version":"0.1.0"}`)
	sum2, err := CalculateChecksum(dir)
	if err != nil {
		t.Fatalf("CalculateChecksum: %v", err)
	}
	if sum1 != sum2 {
		t.Error("manifest.json should be excluded from checksum")
	}

	// Adding signatures/ should not change the checksum
	writeTestFile(t, filepath.Join(dir, "signatures", "manifest.json.sig"), "sig-data")
	sum3, err := CalculateChecksum(dir)
	if err != nil {
		t.Fatalf("CalculateChecksum: %v", err)
	}
	if sum1 != sum3 {
		t.Error("signatures/ should be excluded from checksum")
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	writeTestFile(t, path, "hello")

	hash, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}

	// Same content should produce same hash
	path2 := filepath.Join(dir, "test2.bin")
	writeTestFile(t, path2, "hello")
	hash2, err := HashFile(path2)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	if hash != hash2 {
		t.Error("same content should produce same hash")
	}
}

func TestHashFile_NotFound(t *testing.T) {
	_, err := HashFile("/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func repeatChar(c byte, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return string(b)
}
