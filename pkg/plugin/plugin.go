// Package plugin implements Git/kubectl-style plugin discovery and
// execution (Epic 14 task 13, PROJECT-DETAILS §4.18): any
// `kscore-*` executable on $PATH becomes a `kscorectl <name>`
// subcommand.
//
// Discovery scans $PATH for the prefix; Executor runs a discovered
// plugin via exec.CommandContext with stdio piping and context
// cancellation. Dispatch is the cobra glue that delegates an
// unknown subcommand to a plugin without ever shadowing a
// registered command.
//
// Pure standard library; no new dependency.
package plugin

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// killWaitDelay bounds how long Run blocks after ctx-cancel kills
// the plugin: without it, Wait hangs on stdout/stderr copier
// goroutines a grandchild may have inherited (the standard
// os/exec + CommandContext caveat).
const killWaitDelay = 3 * time.Second

// DefaultPrefix is the plugin binary name prefix.
const DefaultPrefix = "kscore-"

// ErrPluginNotFound — no discovered plugin for the requested name.
var ErrPluginNotFound = errors.New("plugin: not found")

// Plugin is a discovered external subcommand.
type Plugin struct {
	Name string // the part after the prefix (e.g. "module")
	Path string // absolute executable path
}

// Discovery finds prefix-named executables on $PATH. Results are
// cached until Refresh.
type Discovery struct {
	prefix string

	mu      sync.Mutex
	cached  bool
	plugins []Plugin
	byName  map[string]Plugin
}

// New returns a Discovery (empty prefix → DefaultPrefix).
func New(prefix string) *Discovery {
	if prefix == "" {
		prefix = DefaultPrefix
	}
	return &Discovery{prefix: prefix}
}

// Discover returns the discovered plugins (sorted by name, first
// PATH match wins — shell semantics). Cached.
func (d *Discovery) Discover() []Plugin {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cached {
		return d.plugins
	}
	byName := map[string]Plugin{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, d.prefix) || len(name) == len(d.prefix) {
				continue
			}
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil || !isExecutable(info.Mode()) {
				continue
			}
			short := strings.TrimPrefix(name, d.prefix)
			if _, seen := byName[short]; seen {
				continue // first PATH match wins
			}
			byName[short] = Plugin{Name: short, Path: filepath.Join(dir, name)}
		}
	}
	plugins := make([]Plugin, 0, len(byName))
	for _, p := range byName {
		plugins = append(plugins, p)
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })

	d.plugins = plugins
	d.byName = byName
	d.cached = true
	return plugins
}

// Lookup returns the plugin for name, if discovered.
func (d *Discovery) Lookup(name string) (Plugin, bool) {
	d.Discover()
	d.mu.Lock()
	defer d.mu.Unlock()
	p, ok := d.byName[name]
	return p, ok
}

// Refresh clears the cache so the next Discover re-scans $PATH.
func (d *Discovery) Refresh() {
	d.mu.Lock()
	d.cached = false
	d.plugins = nil
	d.byName = nil
	d.mu.Unlock()
}

func isExecutable(m os.FileMode) bool {
	return !m.IsDir() && m.Perm()&0o111 != 0
}

// Executor runs discovered plugins.
type Executor struct{}

// Execute runs p with args, wiring stdio. Returns the child's exit
// code (0 on success; the process exit code on a non-zero exit;
// -1 if the process could not be started). ctx cancellation kills
// the child.
func (Executor) Execute(ctx context.Context, p Plugin, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if p.Path == "" {
		return -1, ErrPluginNotFound
	}
	cmd := exec.CommandContext(ctx, p.Path, args...) //nolint:gosec // p.Path is a discovered kscore-* binary on $PATH
	cmd.WaitDelay = killWaitDelay
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil // the plugin ran and chose a non-zero exit
	}
	return -1, err // spawn failure
}
