package steps

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.keystone-core.io/keystone-core/internal/runbook"
)

// ErrStepNotConfigured is returned by a step whose required port was
// not supplied in [Deps].
var ErrStepNotConfigured = errors.New("steps: step type not configured")

// ErrStepConfig wraps a malformed/missing step Config value.
var ErrStepConfig = errors.New("steps: invalid step config")

// CommandRunner runs a single command (used by `command` and
// `script`). Implementations honour ctx cancellation and report
// command-level failure via CommandResult.ExitCode, not a Go error
// (a Go error means the runner itself failed to launch).
type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}

// CommandRequest is the input to a [CommandRunner].
type CommandRequest struct {
	Command    string
	Args       []string
	Env        map[string]string
	WorkingDir string
	Stdin      []byte
	Timeout    time.Duration
}

// CommandResult is the output of a [CommandRunner].
type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// StateApplier applies a state collection (used by `state`).
type StateApplier interface {
	Apply(ctx context.Context, req StateRequest) (StateResult, error)
}

// StateRequest carries raw state-file YAML to apply.
type StateRequest struct {
	Source []byte
}

// StateResult summarises a state apply.
type StateResult struct {
	Changed int
	Failed  int
	Summary string
}

// HTTPClient is the subset of *http.Client the `api` step needs.
// *http.Client satisfies it.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Notifier delivers a notification (used by `notification`).
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// Notification is one outbound message.
type Notification struct {
	Channel  string
	Message  string
	Severity string
	Fields   map[string]any
}

// Querier runs a read-only lookup (used by `query`).
type Querier interface {
	Query(ctx context.Context, query string, args ...any) ([]map[string]any, error)
}

// Deps carries the injected ports + test seams. A nil port disables
// its step type (ErrStepNotConfigured). HTTP defaults to
// http.DefaultClient; Clock/Sleep default to wall-clock.
type Deps struct {
	Command  CommandRunner
	State    StateApplier
	HTTP     HTTPClient
	Notifier Notifier
	Querier  Querier
	Clock    func() time.Time
	Sleep    func(ctx context.Context, d time.Duration) error
}

func (d Deps) sleep(ctx context.Context, dur time.Duration) error {
	if d.Sleep != nil {
		return d.Sleep(ctx, dur)
	}
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (d Deps) httpClient() HTTPClient {
	if d.HTTP != nil {
		return d.HTTP
	}
	return http.DefaultClient
}

// RegisterAll binds all 9 v1.0 step types into reg using deps.
func RegisterAll(reg *runbook.Registry, deps Deps) error {
	bindings := map[string]runbook.StepExecutor{
		"noop":         runbook.StepFunc(noopStep),
		"fail":         runbook.StepFunc(failStep),
		"wait":         runbook.StepFunc(deps.waitStep),
		"command":      runbook.StepFunc(deps.commandStep),
		"script":       runbook.StepFunc(deps.scriptStep),
		"state":        runbook.StepFunc(deps.stateStep),
		"api":          runbook.StepFunc(deps.apiStep),
		"notification": runbook.StepFunc(deps.notificationStep),
		"query":        runbook.StepFunc(deps.queryStep),
	}
	for typ, ex := range bindings {
		if err := reg.Register(typ, ex); err != nil {
			return err
		}
	}
	return nil
}

// --- Config accessors -------------------------------------------------

func cfgString(cfg map[string]any, key string) (string, error) {
	v, ok := cfg[key]
	if !ok {
		return "", fmt.Errorf("%w: %q is required", ErrStepConfig, key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: %q must be a string, got %T", ErrStepConfig, key, v)
	}
	return s, nil
}

func cfgStringOpt(cfg map[string]any, key, def string) string {
	if v, ok := cfg[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func cfgStringSlice(cfg map[string]any, key string) ([]string, error) {
	v, ok := cfg[key]
	if !ok {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %q must be a list", ErrStepConfig, key)
	}
	out := make([]string, 0, len(list))
	for i, e := range list {
		s, ok := e.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %q[%d] must be a string, got %T", ErrStepConfig, key, i, e)
		}
		out = append(out, s)
	}
	return out, nil
}

func cfgStringMap(cfg map[string]any, key string) (map[string]string, error) {
	v, ok := cfg[key]
	if !ok {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %q must be a map", ErrStepConfig, key)
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %q[%q] must be a string, got %T", ErrStepConfig, key, k, val)
		}
		out[k] = s
	}
	return out, nil
}

func cfgDurationOpt(cfg map[string]any, key string) (time.Duration, error) {
	s := cfgStringOpt(cfg, key, "")
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not a valid duration: %v", ErrStepConfig, key, err)
	}
	return d, nil
}
