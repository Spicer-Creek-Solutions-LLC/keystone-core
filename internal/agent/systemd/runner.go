package systemd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
)

// Runner abstracts systemctl invocations so tests can drive
// Install/Uninstall/Status against a fake. Production paths use
// defaultRunner; tests use NewFakeRunner.
type Runner interface {
	// Run invokes name with args under ctx. Returns stdout+stderr
	// merged (matches systemctl's "all output" behavior) and an
	// error wrapping non-zero exit codes.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// defaultRunner shells out via os/exec. We don't reuse a single
// *exec.Cmd across calls — Run is one-shot per systemctl op.
type defaultRunner struct{}

// NewDefaultRunner returns a Runner that invokes the real
// systemctl binary on the host. Linux-only — non-Linux callers
// should refuse before reaching the runner.
func NewDefaultRunner() Runner { return defaultRunner{} }

func (defaultRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // operator-controlled binary name (Install hard-codes "systemctl"); args validated upstream
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %v: %w (output: %s)", name, args, err, bytes.TrimSpace(out))
	}
	return out, nil
}

// FakeRunner records invocations + serves canned responses. Used
// in tests to assert the right systemctl commands fire in the
// right order.
type FakeRunner struct {
	mu    sync.Mutex
	Calls []FakeCall
	// Responses lets tests pre-program output for specific
	// "name args..." keys (joined with single spaces). Missing
	// keys return nil + nil.
	Responses map[string]FakeResponse
}

// FakeCall is one recorded Run invocation.
type FakeCall struct {
	Name string
	Args []string
}

// FakeResponse is the canned (output, error) for a given call.
type FakeResponse struct {
	Output []byte
	Err    error
}

// NewFakeRunner returns a Runner suitable for tests. Pre-program
// responses via .Responses[key] = FakeResponse{...}; key is the
// space-joined "name args..." string.
func NewFakeRunner() *FakeRunner {
	return &FakeRunner{Responses: map[string]FakeResponse{}}
}

func (f *FakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, FakeCall{Name: name, Args: append([]string(nil), args...)})
	key := name
	for _, a := range args {
		key += " " + a
	}
	if r, ok := f.Responses[key]; ok {
		return r.Output, r.Err
	}
	return nil, nil
}

// CallNames is a test-helper that returns each recorded call as
// "systemctl daemon-reload" / "systemctl is-active foo" — easy
// for assertion sets.
func (f *FakeRunner) CallNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.Calls))
	for _, c := range f.Calls {
		s := c.Name
		for _, a := range c.Args {
			s += " " + a
		}
		out = append(out, s)
	}
	return out
}
