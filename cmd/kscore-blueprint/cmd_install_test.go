package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func buildTarGz(t *testing.T, name string, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o644,
		Size: int64(len(data)),
	}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("write data: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	return buf.Bytes()
}

func TestExtractBlueprint_EntrySizeLimit(t *testing.T) {
	originalLimit := maxBlueprintArchiveEntrySize
	maxBlueprintArchiveEntrySize = 10
	t.Cleanup(func() { maxBlueprintArchiveEntrySize = originalLimit })

	archive := buildTarGz(t, "file.txt", []byte("01234567890"))
	err := extractBlueprint(archive, t.TempDir())
	if err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestExtractBlueprint_WithinSizeLimit(t *testing.T) {
	originalLimit := maxBlueprintArchiveEntrySize
	maxBlueprintArchiveEntrySize = 10
	t.Cleanup(func() { maxBlueprintArchiveEntrySize = originalLimit })

	data := []byte("0123456789")
	dest := t.TempDir()
	archive := buildTarGz(t, "file.txt", data)

	if err := extractBlueprint(archive, dest); err != nil {
		t.Fatalf("extractBlueprint: %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(dest, "file.txt"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(contents) != string(data) {
		t.Fatalf("content mismatch: got %q want %q", contents, data)
	}
}
