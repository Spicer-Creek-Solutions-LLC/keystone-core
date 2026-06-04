// SPDX-License-Identifier: Apache-2.0

// Package module wires the production module-execution stack — the
// real capability hosts and a loader builder — that pkg/module leaves
// as injected seams. It is the boot-side glue for kscore-module run
// (and, later, server-side module execution).
package module

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/exec"

	"go.keystone-core.io/keystone-core/pkg/module/capability"
)

// osFSHost is the production capability.FSHost backed by the local
// filesystem. The capability layer (FSRead/FSWrite) enforces the
// manifest's path globs + size limits before any call reaches here.
type osFSHost struct{}

func (osFSHost) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) } //nolint:gosec // path is capability-scoped upstream
func (osFSHost) WriteFile(path string, data []byte, perm uint32) error {
	return os.WriteFile(path, data, os.FileMode(perm)) //nolint:gosec // path + perm are capability-scoped upstream
}

func (osFSHost) Stat(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// httpHost is the production capability.HTTPHost backed by an
// *http.Client. The capability layer (HTTPCap) enforces the domain
// allowlist + size/rate/timeout before the request reaches here.
type httpHost struct{ client *http.Client }

func (h httpHost) Do(req *http.Request) (*http.Response, error) {
	// #nosec G704 -- httpHost is the deliberately-unscoped transport seam;
	// the module-supplied URL is gated by capability.HTTPCap's domain
	// allowlist (manifest-declared) before any request reaches here.
	return h.client.Do(req)
}

// execHost is the production capability.ExecHost backed by os/exec.
// The capability layer (Exec) enforces the command allowlist +
// working-dir + timeout; the ctx carries that timeout.
type execHost struct{}

func (execHost) Run(ctx context.Context, dir, name string, args []string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // name is capability-allowlisted upstream
	cmd.Dir = dir
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	return out.Bytes(), errBuf.Bytes(), runErr
}

// slogLogger is the production capability.Logger sink, forwarding
// module log lines to the host slog.Logger at the requested level.
type slogLogger struct{ log *slog.Logger }

func (l slogLogger) Log(level, msg string, kv map[string]string) {
	attrs := make([]any, 0, len(kv)*2)
	for k, v := range kv {
		attrs = append(attrs, k, v)
	}
	switch level {
	case "debug":
		l.log.Debug(msg, attrs...)
	case "warn", "warning":
		l.log.Warn(msg, attrs...)
	case "error":
		l.log.Error(msg, attrs...)
	default: // "info" and anything unrecognized
		l.log.Info(msg, attrs...)
	}
}

// LocalHosts returns the capability hosts for local (CLI) module
// execution: real filesystem, HTTP, process-exec and a slog log sink.
// Secrets is intentionally left nil — a standalone CLI has no secrets
// broker, so a module requesting secrets.* fails closed (the correct
// posture; the secrets host is wired server-side). All effects remain
// gated by the manifest-declared, scope-enforced capability layer.
func LocalHosts(log *slog.Logger) capability.Hosts {
	if log == nil {
		log = slog.Default()
	}
	return capability.Hosts{
		FS:     osFSHost{},
		HTTP:   httpHost{client: http.DefaultClient},
		Exec:   execHost{},
		Logger: slogLogger{log: log},
		// Secrets: nil — fails closed for the CLI.
	}
}
