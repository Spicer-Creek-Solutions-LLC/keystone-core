// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestVersionAndHelp(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"version"}, {"--help"}, {"serve", "--help"}} {
		cmd := newCommand()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if buf.Len() == 0 {
			t.Fatalf("%v: empty output", args)
		}
	}
	// version output carries the binary name.
	cmd := newCommand()
	var b bytes.Buffer
	cmd.SetOut(&b)
	cmd.SetArgs([]string{"version"})
	_ = cmd.Execute()
	if !strings.Contains(b.String(), "kscore-registry") {
		t.Fatalf("version = %q", b.String())
	}
}

func TestServe_PublishThenListThenGracefulShutdown(t *testing.T) {
	dir := t.TempDir()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // serve() re-binds; we just needed a free port

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, testLogger(t), addr, dir, 0)
	}()

	base := "http://" + addr
	waitReady(t, base)

	// Publish via the multipart form the CLI will use.
	ct, body := publishForm(t, []byte(manifestYAML), []byte("ZIPBYTES"))
	resp, err := http.Post(base+"/publish", ct, body) //nolint:noctx // test
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("publish status = %d, want 201", resp.StatusCode)
	}

	// The read endpoint reflects it.
	lr, err := http.Get(base + "/acme/widget/@v/list") //nolint:noctx // test
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	lb, _ := io.ReadAll(lr.Body)
	_ = lr.Body.Close()
	if !strings.Contains(string(lb), "1.0.0") {
		t.Fatalf("list = %q", lb)
	}

	// Cancel → graceful shutdown returns nil.
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatalf("serve returned %v, want nil on graceful shutdown", e)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not shut down within 15s")
	}
}

func TestServe_BadDir(t *testing.T) {
	f := t.TempDir() + "/afile"
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A storage root under a regular file → serve fails fast.
	if err := serve(context.Background(), testLogger(t), "127.0.0.1:0", f+"/sub", 0); err == nil {
		t.Fatal("serve with bad --dir: want error")
	}
}

const manifestYAML = "name: acme/widget\nversion: 1.0.0\ntype: starlark\nentrypoint: main.star\n"

func publishForm(t *testing.T, man, zip []byte) (string, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range []struct {
		field, name string
		data        []byte
	}{{"manifest", "manifest.yaml", man}, {"module", "module.zip", zip}} {
		fw, err := mw.CreateFormFile(p.field, p.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(p.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return mw.FormDataContentType(), &buf
}

func waitReady(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/healthz-probe/@v/list") //nolint:noctx // readiness poll
		if err == nil {
			_ = resp.Body.Close()
			return // server is accepting connections (any HTTP reply)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not become ready within 10s")
}
