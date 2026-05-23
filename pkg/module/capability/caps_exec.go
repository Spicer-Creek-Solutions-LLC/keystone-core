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
// allowlist.
//
// Allowlist semantics (clarified per Phase B5 finding M2):
//
//   - **Absolute-path manifest entry** (starts with `/`) — matches
//     only the exact full path. A manifest with
//     `commands: ["/usr/bin/apt-get"]` permits a call with
//     `name="/usr/bin/apt-get"` but DENIES `name="/tmp/apt-get"`
//     or `name="apt-get"`. This is the strict-path-containment
//     mode; use this when the manifest author wants to pin the
//     exact binary.
//
//   - **Bare-name manifest entry** (no `/`) — matches the
//     supplied command's base name. A manifest with
//     `commands: ["apt-get"]` permits `name="apt-get"`,
//     `name="/usr/bin/apt-get"`, `name="/tmp/apt-get"`, and
//     anywhere-apt-get. This is the loose "command-on-PATH" mode;
//     use this when the manifest author trusts the host to resolve
//     the right binary.
//
// Operators who need path-based containment MUST use absolute
// paths in the manifest. Bare names deliberately let the host
// pick the binary; that's the design.
func (c *Exec) Run(ctx context.Context, name string, args []string) (stdout, stderr []byte, err error) {
	if c.host == nil {
		return nil, nil, fmt.Errorf("exec: %w", ErrHostUnavailable)
	}
	// Exact match handles both shapes: bare-to-bare and abs-to-abs.
	_, ok := c.allowed[name]
	if !ok {
		// Base-name fallback: only meaningful when the supplied
		// `name` is an absolute path and the allowlist has a bare
		// entry. For bare-supplied names, base(name) == name, so
		// this branch is a no-op and the exact match above was the
		// real check. Absolute-path allowlist entries do not fall
		// back here because base() of a supplied bare-name doesn't
		// match an absolute-path key.
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
