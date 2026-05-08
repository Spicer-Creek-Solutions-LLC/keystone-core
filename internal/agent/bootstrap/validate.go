package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// Check is one validation outcome. Aggregated into ValidationResult
// + VerifyResult for operator inspection.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// ValidationResult is the Validate phase's output. Operator-facing
// — Engine persists it in State so a failed bootstrap surfaces a
// readable list of what passed / what didn't.
type ValidationResult struct {
	Checks []Check `json:"checks"`
}

// AllOK reports whether every check passed.
func (r *ValidationResult) AllOK() bool {
	for _, c := range r.Checks {
		if !c.OK {
			return false
		}
	}
	return true
}

// Validator runs config-shape + reachability checks before Install
// makes any side effects. v1.0 covers the demo-mode ground truth;
// production/enterprise checks layer in via Tasks 9+.
type Validator interface {
	Validate(ctx context.Context, cfg *Configuration) (*ValidationResult, error)
}

// natsDialTimeout bounds the join-URL reachability probe. 1s is
// long enough for a healthy LAN reach + short enough to fail fast
// when the operator typo'd the URL.
const natsDialTimeout = 1 * time.Second

// NewDefaultValidator returns the v1.0 demo-mode validator. Probes:
//   - Configuration.Validate (mode, cluster_name, agent_id, config_path)
//   - JoinURL parses (when set)
//   - JoinURL host:port TCP-dialable (when set)
//   - ConfigPath parent directory exists or is creatable
func NewDefaultValidator(log *slog.Logger) Validator {
	if log == nil {
		log = slog.Default()
	}
	return &defaultValidator{log: log}
}

type defaultValidator struct {
	log *slog.Logger
}

func (v *defaultValidator) Validate(ctx context.Context, cfg *Configuration) (*ValidationResult, error) {
	out := &ValidationResult{}

	if err := cfg.Validate(); err != nil {
		out.Checks = append(out.Checks, Check{Name: "configuration", OK: false, Detail: err.Error()})
	} else {
		out.Checks = append(out.Checks, Check{Name: "configuration", OK: true})
	}

	if cfg.JoinURL != "" {
		out.Checks = append(out.Checks, v.checkJoinURL(ctx, cfg.JoinURL))
	}

	out.Checks = append(out.Checks, v.checkConfigPath(cfg.ConfigPath))

	return out, nil
}

func (v *defaultValidator) checkJoinURL(ctx context.Context, raw string) Check {
	c := Check{Name: "join_url"}
	u, err := url.Parse(raw)
	if err != nil {
		c.Detail = fmt.Sprintf("parse %q: %v", raw, err)
		return c
	}
	if u.Host == "" {
		c.Detail = fmt.Sprintf("join_url %q has no host", raw)
		return c
	}
	dialer := &net.Dialer{Timeout: natsDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		c.Detail = fmt.Sprintf("dial %q: %v", u.Host, err)
		return c
	}
	_ = conn.Close()
	c.OK = true
	return c
}

func (v *defaultValidator) checkConfigPath(path string) Check {
	c := Check{Name: "config_path"}
	if path == "" {
		c.Detail = "config_path is empty"
		return c
	}
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			c.Detail = fmt.Sprintf("%q exists but is not a directory", dir)
			return c
		}
		c.OK = true
		return c
	} else if os.IsNotExist(err) {
		// Parent absent — Install creates it. We still pass the check
		// here because the create is a known-safe side effect.
		c.OK = true
		c.Detail = "parent dir absent; will be created at Install"
		return c
	} else {
		c.Detail = fmt.Sprintf("stat %q: %v", dir, err)
		return c
	}
}
