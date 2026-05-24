// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"context"
	"net/http"
)

// Host seams. Each capability backend delegates the actual effect
// to a narrow injected host so pkg/module stays dependency-light;
// production wires the real internal/secrets / os-exec / net-http
// hosts at server boot (deferred — the module boot-wiring ROADMAP
// item, like every other Epic 14 host integration). Tests inject
// fakes.

// FSHost performs the unscoped filesystem effects.
type FSHost interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm uint32) error
	Stat(path string) (size int64, err error)
}

// HTTPHost performs the unscoped HTTP round-trip.
type HTTPHost interface {
	Do(req *http.Request) (*http.Response, error)
}

// ExecHost runs a process. dir may be empty (host default).
type ExecHost interface {
	Run(ctx context.Context, dir, name string, args []string) (stdout, stderr []byte, err error)
}

// SecretsHost is the secrets backend (boot wires
// internal/secrets.Broker).
type SecretsHost interface {
	Get(ctx context.Context, path string) (map[string]any, error)
	Set(ctx context.Context, path string, data map[string]any) error
}

// Logger is the module log sink (default: slog adapter).
type Logger interface {
	Log(level, msg string, kv map[string]string)
}

// Hosts bundles every host seam for [BuildCapabilities]. Any field
// may be nil — the corresponding capability then fails closed with
// a clear error if a module both requested and tries to use it.
type Hosts struct {
	FS      FSHost
	HTTP    HTTPHost
	Exec    ExecHost
	Secrets SecretsHost
	Logger  Logger
}
