// SPDX-License-Identifier: Apache-2.0

package dest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalDestination_Open_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.tar")
	d := &LocalDestination{Path: path}

	wc, err := d.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	payload := []byte("backup-bytes")
	if _, err := wc.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("file = %q, want %q", got, payload)
	}
}

func TestLocalDestination_Open_MissingParent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-such-subdir", "backup.tar")
	d := &LocalDestination{Path: path}

	_, err := d.Open(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "open") {
		t.Errorf("err = %v, want open error", err)
	}
}

func TestLocalDestination_Open_EmptyPath(t *testing.T) {
	d := &LocalDestination{}
	if _, err := d.Open(context.Background()); err == nil {
		t.Fatal("want error for empty Path")
	}
}

func TestLocalDestination_Open_Truncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.tar")
	if err := os.WriteFile(path, []byte("existing content much longer than overwrite"), 0o600); err != nil {
		t.Fatal(err)
	}

	d := &LocalDestination{Path: path}
	wc, err := d.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := wc.Write([]byte("new")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("file = %q, want \"new\" (truncation failed)", got)
	}
}
