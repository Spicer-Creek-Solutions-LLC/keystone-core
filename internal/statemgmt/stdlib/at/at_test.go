// SPDX-License-Identifier: Apache-2.0

package at

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "at:" + name,
		Module: "at",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider ------------------------------------------------------

// fakeJob is a queued at job in the fake.
type fakeJob struct {
	id     string
	script string
}

type fakeProvider struct {
	jobs    []fakeJob
	nextID  int
	submits []struct{ queue, timeSpec, script string }
	removed []string

	listErr   error
	scriptErr error
	submitErr error
	removeErr error
}

func newFake(jobs ...fakeJob) *fakeProvider {
	return &fakeProvider{jobs: jobs, nextID: 100}
}
func (f *fakeProvider) ListJobs(context.Context) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	ids := make([]string, 0, len(f.jobs))
	for _, j := range f.jobs {
		ids = append(ids, j.id)
	}
	return ids, nil
}
func (f *fakeProvider) JobScript(_ context.Context, id string) (string, error) {
	if f.scriptErr != nil {
		return "", f.scriptErr
	}
	for _, j := range f.jobs {
		if j.id == id {
			return j.script, nil
		}
	}
	return "", fmt.Errorf("no such job %s", id)
}
func (f *fakeProvider) Submit(_ context.Context, queue, timeSpec, script string) error {
	if f.submitErr != nil {
		return f.submitErr
	}
	f.submits = append(f.submits, struct{ queue, timeSpec, script string }{queue, timeSpec, script})
	id := fmt.Sprintf("%d", f.nextID)
	f.nextID++
	f.jobs = append(f.jobs, fakeJob{id: id, script: atPreamble + script})
	return nil
}
func (f *fakeProvider) Remove(_ context.Context, id string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, id)
	out := f.jobs[:0]
	for _, j := range f.jobs {
		if j.id != id {
			out = append(out, j)
		}
	}
	f.jobs = out
	return nil
}

// atPreamble is the environment block `at` prepends to a submitted
// script — the marker comment lives below it.
const atPreamble = "#!/bin/sh\n# atrun uid=0 gid=0\nexport PATH=/usr/bin\ncd /root || exit 1\n"

func taggedScript(name, body string) string {
	return atPreamble + markerLine(name) + "\n" + body + "\n"
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("j", StatePresent, map[string]any{"comand": "x"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_TypeErrors(t *testing.T) {
	t.Parallel()
	for _, p := range []map[string]any{{"command": 1}, {"time": 2}, {"queue": 3}} {
		if _, err := parseParams(decl("j", StatePresent, p)); err == nil {
			t.Errorf("%v: expected type error", p)
		}
	}
}

func TestParse_DefaultQueue(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(decl("j", StatePresent, map[string]any{"command": "x", "time": "now"}))
	if p.Queue != defaultQueue {
		t.Errorf("default queue = %q, want %q", p.Queue, defaultQueue)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"present ok", decl("j", StatePresent, map[string]any{"command": "/bin/x", "time": "now + 1 hour"}), false},
		{"present multiline command ok", decl("j", StatePresent, map[string]any{"command": "a\nb", "time": "10:30"}), false},
		{"present needs command", decl("j", StatePresent, map[string]any{"time": "now"}), true},
		{"present needs time", decl("j", StatePresent, map[string]any{"command": "/bin/x"}), true},
		{"multiline time", decl("j", StatePresent, map[string]any{"command": "/bin/x", "time": "a\nb"}), true},
		{"newline in id", decl("a\nb", StatePresent, map[string]any{"command": "/bin/x", "time": "now"}), true},
		{"hash id", decl("#sneaky", StatePresent, map[string]any{"command": "/bin/x", "time": "now"}), true},
		{"bad queue (multichar)", decl("j", StatePresent, map[string]any{"command": "/bin/x", "time": "now", "queue": "ab"}), true},
		{"bad queue (digit)", decl("j", StatePresent, map[string]any{"command": "/bin/x", "time": "now", "queue": "1"}), true},
		{"queue letter ok", decl("j", StatePresent, map[string]any{"command": "/bin/x", "time": "now", "queue": "z"}), false},
		{"absent ok", decl("j", StateAbsent, nil), false},
		{"absent allows queue", decl("j", StateAbsent, map[string]any{"queue": "b"}), false},
		{"absent rejects command", decl("j", StateAbsent, map[string]any{"command": "/bin/x"}), true},
		{"absent rejects time", decl("j", StateAbsent, map[string]any{"time": "now"}), true},
		{"bad state", decl("j", "frob", nil), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parseParams(tc.d)
			if err == nil {
				err = p.validate()
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestOneline(t *testing.T) {
	t.Parallel()
	if got := oneline("  hi  "); got != "hi" {
		t.Errorf("oneline single = %q", got)
	}
	if got := oneline("first\nsecond"); got != "first …" {
		t.Errorf("oneline multi = %q", got)
	}
}

// --- Check / Apply -----------------------------------------------------

func TestCheckApply_Present(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("backup-reminder", StatePresent, map[string]any{"command": "/usr/bin/remind backup", "time": "now + 1 hour", "queue": "b"})

	// nothing queued → drift
	r, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches {
		t.Error("no tagged job → should drift")
	}

	// Apply: submits a tagged job
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("first apply should change")
	}
	if len(f.submits) != 1 {
		t.Fatalf("expected one Submit, got %d", len(f.submits))
	}
	s := f.submits[0]
	if s.queue != "b" || s.timeSpec != "now + 1 hour" {
		t.Errorf("Submit args wrong: %+v", s)
	}
	if !strings.Contains(s.script, markerLine("backup-reminder")) || !strings.Contains(s.script, "/usr/bin/remind backup") {
		t.Errorf("submitted script wrong: %q", s.script)
	}

	// Check: now matches
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged job should match, diff=%q", r.Diff)
	}

	// Apply again: no-op (no second Submit)
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed || sr.Comment != "already converged" {
		t.Errorf("second apply: changed=%v comment=%q", sr.Changed, sr.Comment)
	}
	if len(f.submits) != 1 {
		t.Error("no-op apply should not Submit again")
	}
}

func TestCheckApply_Present_ExistingJobUntouched(t *testing.T) {
	t.Parallel()
	// a tagged job already in the queue (with a *different* command) —
	// the module matches by name only and leaves it alone.
	f := newFake(fakeJob{id: "7", script: taggedScript("nightly", "/old/command")})
	m := NewWithProvider(f)
	d := decl("nightly", StatePresent, map[string]any{"command": "/new/command", "time": "midnight"})

	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("an existing tagged job should match regardless of command/time")
	}
	sr, _ := m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("apply should not re-queue an existing tagged job")
	}
	if len(f.submits) != 0 {
		t.Error("no Submit expected")
	}
}

func TestCheckApply_Absent(t *testing.T) {
	t.Parallel()
	f := newFake(
		fakeJob{id: "3", script: taggedScript("cleanup", "/bin/cleanup")},
		fakeJob{id: "4", script: atPreamble + "/bin/unrelated\n"},
		fakeJob{id: "9", script: taggedScript("cleanup", "/bin/cleanup")}, // a second job with the same tag
	)
	m := NewWithProvider(f)
	d := decl("cleanup", StateAbsent, nil)

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("a queued tagged job should drift from absent")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("removal should change")
	}
	// both tagged jobs removed, the unrelated one survives
	if len(f.removed) != 2 {
		t.Errorf("expected 2 removals, got %v", f.removed)
	}
	if len(f.jobs) != 1 || f.jobs[0].id != "4" {
		t.Errorf("unrelated job should survive: %+v", f.jobs)
	}

	// already absent → no-op
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("absent on a missing job should be a no-op")
	}

	// absent on an empty queue → match, no-op
	m2 := NewWithProvider(newFake())
	r, _ = m2.Check(context.Background(), decl("ghost", StateAbsent, nil))
	if !r.Matches {
		t.Error("absent on an empty queue should match")
	}
}

func TestApply_ErrorsPropagate(t *testing.T) {
	t.Parallel()
	// list error
	f := newFake()
	f.listErr = errors.New("at -l blew up")
	if _, err := NewWithProvider(f).Check(context.Background(), decl("j", StatePresent, map[string]any{"command": "x", "time": "now"})); err == nil {
		t.Error("list error should propagate from Check")
	}
	// script error during the scan
	f = newFake(fakeJob{id: "1", script: "whatever"})
	f.scriptErr = errors.New("at -c failed")
	if _, err := NewWithProvider(f).Check(context.Background(), decl("j", StatePresent, map[string]any{"command": "x", "time": "now"})); err == nil {
		t.Error("script error should propagate from Check")
	}
	// submit error
	f = newFake()
	f.submitErr = errors.New("Garbled time")
	sr, err := NewWithProvider(f).Apply(context.Background(), decl("j", StatePresent, map[string]any{"command": "x", "time": "bogus"}))
	if err == nil {
		t.Fatal("submit error should propagate")
	}
	if sr == nil || sr.Success {
		t.Error("result should report failure")
	}
	// remove error
	f = newFake(fakeJob{id: "5", script: taggedScript("j", "/bin/x")})
	f.removeErr = errors.New("atrm failed")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("j", StateAbsent, nil)); err == nil {
		t.Error("remove error should propagate")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "at" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("at should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("j", StateAbsent, nil), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(decl("j", StatePresent, map[string]any{"command": "x", "time": "now"}), nil) != statemgmt.DriftSeverityMedium {
		t.Error("present drift → MEDIUM")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("j", StatePresent, map[string]any{"command": "x", "time": "now"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("j", StatePresent, nil)); err == nil {
		t.Error("present-without-command should be rejected")
	}

	// Test() via a fake provider
	fm := NewWithProvider(newFake())
	ok, err := fm.Test(context.Background(), decl("ghost", StateAbsent, nil))
	if err != nil || !ok {
		t.Errorf("Test on absent-and-empty: ok=%v err=%v", ok, err)
	}
	ok, err = fm.Test(context.Background(), decl("j", StatePresent, map[string]any{"command": "x", "time": "now"}))
	if err != nil || ok {
		t.Errorf("Test on a not-yet-queued job should be false: ok=%v err=%v", ok, err)
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || IsUnsupportedOS(errors.New("x")) {
		t.Error("IsUnsupportedOS")
	}
	if !IsNoAt(ErrNoAt) || IsNoAt(errors.New("x")) {
		t.Error("IsNoAt")
	}
}
