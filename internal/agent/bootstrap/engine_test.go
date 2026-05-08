package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// recorder captures the call order across the five phase impls so
// tests can assert FSM walks the right sequence.
type recorder struct {
	calls []string
}

func (r *recorder) record(phase string) { r.calls = append(r.calls, phase) }

type recordedDetector struct{ r *recorder }

func (d recordedDetector) Detect(_ context.Context) (*DetectionResult, error) {
	d.r.record("detect")
	return &DetectionResult{OS: "linux"}, nil
}

type recordedConfigurer struct {
	r   *recorder
	cfg *Configuration
}

func (c recordedConfigurer) Configure(_ context.Context, _ *DetectionResult) (*Configuration, error) {
	c.r.record("configure")
	if c.cfg == nil {
		return nil, errors.New("recordedConfigurer: nil cfg")
	}
	out := *c.cfg
	return &out, nil
}

type recordedValidator struct {
	r   *recorder
	res *ValidationResult
	err error
}

func (v recordedValidator) Validate(_ context.Context, _ *Configuration) (*ValidationResult, error) {
	v.r.record("validate")
	return v.res, v.err
}

type recordedInstaller struct {
	r   *recorder
	res *InstallResult
	err error
}

func (i recordedInstaller) Install(_ context.Context, _ *Configuration) (*InstallResult, error) {
	i.r.record("install")
	return i.res, i.err
}

type recordedVerifier struct {
	r   *recorder
	res *VerifyResult
	err error
}

func (v recordedVerifier) Verify(_ context.Context, _ *Configuration) (*VerifyResult, error) {
	v.r.record("verify")
	return v.res, v.err
}

func newTestEngine(t *testing.T, r *recorder, cfg *Configuration) (*Engine, string) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "bootstrap.json")
	allOK := &ValidationResult{Checks: []Check{{Name: "ok", OK: true}}}
	allOKVerify := &VerifyResult{Checks: []Check{{Name: "ok", OK: true}}}
	eng, err := NewEngine(EngineConfig{
		StatePath:  statePath,
		Logger:     discardLogger(),
		Now:        func() time.Time { return time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC) },
		Detector:   recordedDetector{r: r},
		Configurer: recordedConfigurer{r: r, cfg: cfg},
		Validator:  recordedValidator{r: r, res: allOK},
		Installer:  recordedInstaller{r: r, res: &InstallResult{ConfigPath: cfg.ConfigPath}},
		Verifier:   recordedVerifier{r: r, res: allOKVerify},
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng, statePath
}

func validConfiguration(t *testing.T) *Configuration {
	t.Helper()
	return &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  filepath.Join(t.TempDir(), "agent.yaml"),
	}
}

func TestEngine_HappyPath(t *testing.T) {
	r := &recorder{}
	eng, statePath := newTestEngine(t, r, validConfiguration(t))

	state, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if state.Phase != PhaseDone {
		t.Errorf("Phase = %q, want done", state.Phase)
	}
	if state.CompletedAt == nil {
		t.Error("CompletedAt nil after happy path")
	}
	want := []string{"detect", "configure", "validate", "install", "verify"}
	if len(r.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", r.calls, want)
	}
	for i, c := range want {
		if r.calls[i] != c {
			t.Errorf("calls[%d] = %q, want %q", i, r.calls[i], c)
		}
	}

	// State must be persisted to disk.
	loaded, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded == nil || loaded.Phase != PhaseDone {
		t.Errorf("loaded state = %+v", loaded)
	}
}

func TestEngine_ResumeSkipsCompletedPhases(t *testing.T) {
	r := &recorder{}
	cfg := validConfiguration(t)
	eng, statePath := newTestEngine(t, r, cfg)

	// Pre-seed a state file at PhaseInstall (Detect + Configure +
	// Validate already done in a previous run).
	pre := NewState(time.Now().UTC())
	pre.Phase = PhaseInstall
	pre.Detection = &DetectionResult{OS: "linux"}
	pre.Config = cfg
	pre.Validation = &ValidationResult{Checks: []Check{{Name: "ok", OK: true}}}
	if err := pre.Save(statePath); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	state, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if state.Phase != PhaseDone {
		t.Errorf("Phase = %q, want done", state.Phase)
	}
	// Only install + verify should have run.
	if got := r.calls; len(got) != 2 || got[0] != "install" || got[1] != "verify" {
		t.Errorf("resumed calls = %v, want [install verify]", got)
	}
}

func TestEngine_ReRunAfterDoneIsNoOp(t *testing.T) {
	r := &recorder{}
	eng, _ := newTestEngine(t, r, validConfiguration(t))

	if _, err := eng.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	r.calls = nil

	state, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if state.Phase != PhaseDone {
		t.Errorf("Phase = %q, want done", state.Phase)
	}
	if len(r.calls) != 0 {
		t.Errorf("second Run made phase calls: %v, want none", r.calls)
	}
}

func TestEngine_ValidateFailureRecordsLastErrorAndPersists(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "bootstrap.json")
	cfg := validConfiguration(t)
	failingValidation := &ValidationResult{Checks: []Check{{Name: "x", OK: false, Detail: "boom"}}}
	eng, err := NewEngine(EngineConfig{
		StatePath:  statePath,
		Logger:     discardLogger(),
		Detector:   recordedDetector{r: &recorder{}},
		Configurer: recordedConfigurer{r: &recorder{}, cfg: cfg},
		Validator:  recordedValidator{r: &recorder{}, res: failingValidation},
		Installer:  recordedInstaller{r: &recorder{}},
		Verifier:   recordedVerifier{r: &recorder{}},
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	state, err := eng.Run(context.Background())
	if err == nil {
		t.Fatal("Run: expected error, got nil")
	}
	if state == nil {
		t.Fatal("state is nil after failure; expected populated")
	}
	if state.LastError == "" {
		t.Error("LastError empty after failure")
	}
	if state.Phase != PhaseValidate {
		t.Errorf("Phase = %q, want validate (paused at failure)", state.Phase)
	}

	// Persisted state should match in-memory state.
	loaded, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded.LastError == "" {
		t.Errorf("persisted LastError empty: %+v", loaded)
	}
}

func TestEngine_DetectErrorStopsImmediately(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "bootstrap.json")
	calls := atomic.Int32{}
	failingDetector := failingDetector{err: errors.New("synthetic detect failure"), counter: &calls}
	eng, err := NewEngine(EngineConfig{
		StatePath:  statePath,
		Logger:     discardLogger(),
		Detector:   failingDetector,
		Configurer: recordedConfigurer{r: &recorder{}, cfg: validConfiguration(t)},
		Validator:  recordedValidator{r: &recorder{}, res: &ValidationResult{}},
		Installer:  recordedInstaller{r: &recorder{}},
		Verifier:   recordedVerifier{r: &recorder{}},
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if _, err := eng.Run(context.Background()); err == nil {
		t.Fatal("expected detect error")
	}
	if calls.Load() != 1 {
		t.Errorf("detector called %d times, want 1", calls.Load())
	}
}

type failingDetector struct {
	err     error
	counter *atomic.Int32
}

func (d failingDetector) Detect(_ context.Context) (*DetectionResult, error) {
	d.counter.Add(1)
	return nil, d.err
}

func TestNewEngine_RequiresAllPhases(t *testing.T) {
	good := EngineConfig{
		Detector:   recordedDetector{r: &recorder{}},
		Configurer: recordedConfigurer{r: &recorder{}, cfg: validConfiguration(t)},
		Validator:  recordedValidator{r: &recorder{}},
		Installer:  recordedInstaller{r: &recorder{}},
		Verifier:   recordedVerifier{r: &recorder{}},
	}
	cases := []struct {
		name string
		mut  func(*EngineConfig)
	}{
		{"nil detector", func(c *EngineConfig) { c.Detector = nil }},
		{"nil configurer", func(c *EngineConfig) { c.Configurer = nil }},
		{"nil validator", func(c *EngineConfig) { c.Validator = nil }},
		{"nil installer", func(c *EngineConfig) { c.Installer = nil }},
		{"nil verifier", func(c *EngineConfig) { c.Verifier = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := good
			tc.mut(&cfg)
			if _, err := NewEngine(cfg); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestEngine_ResumeAfterFailureContinuesFromCheckpoint(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "bootstrap.json")
	cfg := validConfiguration(t)

	// Phase 1: install fails — state pauses at install with error.
	r1 := &recorder{}
	failOnce := failingInstaller{r: r1, err: errors.New("disk full")}
	eng1, err := NewEngine(EngineConfig{
		StatePath:  statePath,
		Logger:     discardLogger(),
		Detector:   recordedDetector{r: r1},
		Configurer: recordedConfigurer{r: r1, cfg: cfg},
		Validator:  recordedValidator{r: r1, res: &ValidationResult{Checks: []Check{{Name: "ok", OK: true}}}},
		Installer:  &failOnce,
		Verifier:   recordedVerifier{r: r1, res: &VerifyResult{Checks: []Check{{Name: "ok", OK: true}}}},
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if _, err := eng1.Run(context.Background()); err == nil {
		t.Fatal("first Run: expected install failure")
	}

	// Phase 2: re-run with a successful installer — only install +
	// verify execute. Detect/configure/validate are skipped.
	r2 := &recorder{}
	eng2, err := NewEngine(EngineConfig{
		StatePath:  statePath,
		Logger:     discardLogger(),
		Detector:   recordedDetector{r: r2},
		Configurer: recordedConfigurer{r: r2, cfg: cfg},
		Validator:  recordedValidator{r: r2, res: &ValidationResult{Checks: []Check{{Name: "ok", OK: true}}}},
		Installer:  recordedInstaller{r: r2, res: &InstallResult{ConfigPath: cfg.ConfigPath}},
		Verifier:   recordedVerifier{r: r2, res: &VerifyResult{Checks: []Check{{Name: "ok", OK: true}}}},
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	state, err := eng2.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if state.Phase != PhaseDone {
		t.Errorf("Phase = %q, want done", state.Phase)
	}
	if got := r2.calls; len(got) != 2 || got[0] != "install" || got[1] != "verify" {
		t.Errorf("resumed calls = %v, want [install verify]", got)
	}
}

type failingInstaller struct {
	r   *recorder
	err error
}

func (f *failingInstaller) Install(_ context.Context, _ *Configuration) (*InstallResult, error) {
	f.r.record("install")
	return nil, f.err
}
