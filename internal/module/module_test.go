// SPDX-License-Identifier: Apache-2.0

package module

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalHosts_Shape(t *testing.T) {
	h := LocalHosts(nil)
	if h.FS == nil || h.HTTP == nil || h.Exec == nil || h.Logger == nil {
		t.Fatalf("LocalHosts left a host nil: %+v", h)
	}
	if h.Secrets != nil {
		t.Error("LocalHosts.Secrets should be nil for the CLI (fails closed)")
	}
}

func TestOSFSHost_RoundTrip(t *testing.T) {
	var fs osFSHost
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := fs.WriteFile(p, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := fs.ReadFile(p)
	if err != nil || string(data) != "hello" {
		t.Fatalf("ReadFile = %q, %v; want hello", data, err)
	}
	size, err := fs.Stat(p)
	if err != nil || size != 5 {
		t.Fatalf("Stat = %d, %v; want 5", size, err)
	}
}

func TestExecHost_Run(t *testing.T) {
	stdout, _, err := execHost{}.Run(context.Background(), "", "echo", []string{"hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(string(stdout)) != "hi" {
		t.Errorf("stdout = %q, want hi", stdout)
	}
}

func TestHTTPHost_Do(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := httpHost{client: http.DefaultClient}.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestSlogLogger_Levels(t *testing.T) {
	var buf bytes.Buffer
	l := slogLogger{log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	l.Log("warn", "careful", map[string]string{"k": "v"})
	out := buf.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "careful") || !strings.Contains(out, "k=v") {
		t.Errorf("log output missing fields: %q", out)
	}
}

func TestBuildLoader_NotNil(t *testing.T) {
	if BuildLoader(LoaderOptions{}) == nil {
		t.Fatal("BuildLoader returned nil")
	}
}
