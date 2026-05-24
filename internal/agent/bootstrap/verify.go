// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"

	"go.keystone-core.io/keystone-core/internal/config"
)

// VerifyResult is the Verify phase's output. Mirrors
// ValidationResult's shape so operator dashboards can render both
// the same way.
type VerifyResult struct {
	Checks []Check `json:"checks"`
}

// AllOK reports whether every check passed.
func (r *VerifyResult) AllOK() bool {
	for _, c := range r.Checks {
		if !c.OK {
			return false
		}
	}
	return true
}

// Verifier runs the post-Install sanity checks. v1.0 demo mode
// confirms the written config file parses + the NATS endpoint is
// still reachable (a more thorough Verify happens once kscore-agent
// boots and registers, but that's outside bootstrap's scope).
type Verifier interface {
	Verify(ctx context.Context, cfg *Configuration) (*VerifyResult, error)
}

// NewDefaultVerifier returns the v1.0 demo-mode verifier.
func NewDefaultVerifier(log *slog.Logger) Verifier {
	if log == nil {
		log = slog.Default()
	}
	return &defaultVerifier{log: log}
}

type defaultVerifier struct {
	log *slog.Logger
}

func (v *defaultVerifier) Verify(ctx context.Context, cfg *Configuration) (*VerifyResult, error) {
	out := &VerifyResult{}
	out.Checks = append(out.Checks, v.checkConfigFile(cfg))
	if cfg.JoinURL != "" {
		out.Checks = append(out.Checks, v.checkJoinURLReachable(ctx, cfg.JoinURL))
	}
	return out, nil
}

func (v *defaultVerifier) checkConfigFile(cfg *Configuration) Check {
	c := Check{Name: "config_file_parses"}
	if cfg.ConfigPath == "" {
		c.Detail = "config_path is empty"
		return c
	}
	if _, err := os.Stat(cfg.ConfigPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.Detail = fmt.Sprintf("config file %q does not exist after Install", cfg.ConfigPath)
			return c
		}
		c.Detail = fmt.Sprintf("stat config file: %v", err)
		return c
	}
	if _, err := config.Load(cfg.ConfigPath); err != nil {
		c.Detail = fmt.Sprintf("parse config %q: %v", cfg.ConfigPath, err)
		return c
	}
	c.OK = true
	return c
}

func (v *defaultVerifier) checkJoinURLReachable(ctx context.Context, raw string) Check {
	c := Check{Name: "join_url_reachable"}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		c.Detail = fmt.Sprintf("invalid join_url %q", raw)
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
