package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, path, name string, data []byte) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zipWriter := zip.NewWriter(file)

	entry, err := zipWriter.Create(name)
	if err != nil {
		zipWriter.Close()
		file.Close()
		t.Fatalf("create entry: %v", err)
	}
	if _, err := entry.Write(data); err != nil {
		zipWriter.Close()
		file.Close()
		t.Fatalf("write entry: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		file.Close()
		t.Fatalf("close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
}

func TestExtractModule_EntrySizeLimit(t *testing.T) {
	originalLimit := maxModuleArchiveEntrySize
	maxModuleArchiveEntrySize = 10
	t.Cleanup(func() { maxModuleArchiveEntrySize = originalLimit })

	zipPath := filepath.Join(t.TempDir(), "module.zip")
	writeZip(t, zipPath, "module.txt", []byte("01234567890"))

	err := extractModule(zipPath, "acme/demo", "1.0.0", t.TempDir())
	if err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestExtractModule_WithinSizeLimit(t *testing.T) {
	originalLimit := maxModuleArchiveEntrySize
	maxModuleArchiveEntrySize = 10
	t.Cleanup(func() { maxModuleArchiveEntrySize = originalLimit })

	zipPath := filepath.Join(t.TempDir(), "module.zip")
	data := []byte("0123456789")
	writeZip(t, zipPath, "module.txt", data)

	modulesDir := t.TempDir()
	if err := extractModule(zipPath, "acme/demo", "1.0.0", modulesDir); err != nil {
		t.Fatalf("extractModule: %v", err)
	}

	extracted := filepath.Join(modulesDir, "acme", "demo", "1.0.0", "module.txt")
	contents, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(contents) != string(data) {
		t.Fatalf("content mismatch: got %q want %q", contents, data)
	}
}
