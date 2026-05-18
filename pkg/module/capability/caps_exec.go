package capability

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// Exec is the scoped exec capability: the command (by base name)
// must be in the allowlist; it runs in the configured working dir
// with the configured timeout.
type Exec struct {
	allowed map[string]struct{}
	dir     string
	timeout time.Duration
	host    ExecHost
}

// NewExec builds the capability from its manifest config + host.
func NewExec(cfg manifest.CapabilityConfig, host ExecHost) (*Exec, error) {
	allowed := make(map[string]struct{}, len(cfg.Commands))
	for _, c := range cfg.Commands {
		allowed[c] = struct{}{}
	}
	var to time.Duration
	if cfg.Timeout != "" {
		var err error
		if to, err = time.ParseDuration(cfg.Timeout); err != nil {
			return nil, fmt.Errorf("exec timeout: %w", err)
		}
	}
	return &Exec{allowed: allowed, dir: cfg.WorkingDir, timeout: to, host: host}, nil
}

// Run executes name with args after enforcing the command
// allowlist. The allowlist matches on both the exact authored
// string and the path base, so `apt-get` permits `/usr/bin/apt-get`.
func (c *Exec) Run(ctx context.Context, name string, args []string) (stdout, stderr []byte, err error) {
	if c.host == nil {
		return nil, nil, fmt.Errorf("exec: %w", ErrHostUnavailable)
	}
	_, ok := c.allowed[name]
	if !ok {
		_, ok = c.allowed[filepath.Base(name)]
	}
	if !ok {
		return nil, nil, fmt.Errorf("%w: %q", ErrCommandDenied, name)
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	return c.host.Run(ctx, c.dir, name, args)
}
