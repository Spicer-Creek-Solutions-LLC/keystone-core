package cron

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "cron:" + name,
		Module: "cron",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider ------------------------------------------------------

type fakeProvider struct {
	crontabs map[string]string // user → content
	reads    int
	writes   int
	readErr  error
	writeErr error
}

func newFake(initial map[string]string) *fakeProvider {
	if initial == nil {
		initial = map[string]string{}
	}
	return &fakeProvider{crontabs: initial}
}
func (f *fakeProvider) Read(_ context.Context, user string) (string, error) {
	f.reads++
	if f.readErr != nil {
		return "", f.readErr
	}
	return f.crontabs[user], nil
}
func (f *fakeProvider) Write(_ context.Context, user, content string) error {
	f.writes++
	if f.writeErr != nil {
		return f.writeErr
	}
	f.crontabs[user] = content
	return nil
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("job", StatePresent, map[string]any{"comand": "x"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_TypeErrors(t *testing.T) {
	t.Parallel()
	for _, p := range []map[string]any{
		{"command": 1}, {"schedule": 2}, {"user": 3},
	} {
		if _, err := parseParams(decl("job", StatePresent, p)); err == nil {
			t.Errorf("%v: expected type error", p)
		}
	}
}

func TestParse_DefaultUser(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(decl("job", StatePresent, map[string]any{"command": "x", "schedule": "@daily"}))
	if p.User != defaultCronUser {
		t.Errorf("default user = %q, want %q", p.User, defaultCronUser)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"present ok 5-field", decl("j", StatePresent, map[string]any{"command": "/bin/x", "schedule": "*/5 * * * *"}), false},
		{"present ok shortcut", decl("j", StatePresent, map[string]any{"command": "/bin/x", "schedule": "@reboot"}), false},
		{"present needs command", decl("j", StatePresent, map[string]any{"schedule": "@daily"}), true},
		{"present needs schedule", decl("j", StatePresent, map[string]any{"command": "/bin/x"}), true},
		{"bad field count", decl("j", StatePresent, map[string]any{"command": "/bin/x", "schedule": "* * * *"}), true},
		{"unknown shortcut", decl("j", StatePresent, map[string]any{"command": "/bin/x", "schedule": "@often"}), true},
		{"multiline command", decl("j", StatePresent, map[string]any{"command": "a\nb", "schedule": "@daily"}), true},
		{"newline in id", decl("a\nb", StatePresent, map[string]any{"command": "/bin/x", "schedule": "@daily"}), true},
		{"hash id", decl("#sneaky", StatePresent, map[string]any{"command": "/bin/x", "schedule": "@daily"}), true},
		{"bad user", decl("j", StatePresent, map[string]any{"command": "/bin/x", "schedule": "@daily", "user": "../root"}), true},
		{"absent ok", decl("j", StateAbsent, nil), false},
		{"absent rejects command", decl("j", StateAbsent, map[string]any{"command": "/bin/x"}), true},
		{"absent rejects schedule", decl("j", StateAbsent, map[string]any{"schedule": "@daily"}), true},
		{"absent allows user", decl("j", StateAbsent, map[string]any{"user": "alice"}), false},
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

func TestCanonSchedule(t *testing.T) {
	t.Parallel()
	if got := canonSchedule("*/5  *   *  *  *"); got != "*/5 * * * *" {
		t.Errorf("canonSchedule = %q", got)
	}
	if got := canonSchedule("@daily"); got != "@daily" {
		t.Errorf("canonSchedule shortcut = %q", got)
	}
}

// --- crontab manipulation (pure) --------------------------------------

func TestCrontab_FindUpsertRemove(t *testing.T) {
	t.Parallel()
	base := "# user comment\n0 0 * * * /bin/legacy\n"

	// not found
	if _, found := findJob(base, "job1"); found {
		t.Error("findJob should not find a missing job")
	}

	// upsert appends a fresh block, leaving the user's line alone
	c1 := upsertJob(base, "job1", "*/5 * * * * /bin/job1")
	got, found := findJob(c1, "job1")
	if !found || got != "*/5 * * * * /bin/job1" {
		t.Fatalf("after upsert: found=%v got=%q\ncontent=%q", found, got, c1)
	}
	if !strings.Contains(c1, "0 0 * * * /bin/legacy") || !strings.Contains(c1, "# user comment") {
		t.Errorf("upsert clobbered existing lines: %q", c1)
	}
	if !strings.Contains(c1, markerLine("job1")) {
		t.Errorf("marker missing: %q", c1)
	}

	// upsert again with a new line replaces in place (no duplicate block)
	c2 := upsertJob(c1, "job1", "@daily /bin/job1-v2")
	got, _ = findJob(c2, "job1")
	if got != "@daily /bin/job1-v2" {
		t.Errorf("after re-upsert: got=%q", got)
	}
	if strings.Count(c2, markerLine("job1")) != 1 {
		t.Errorf("re-upsert duplicated the marker: %q", c2)
	}

	// add a second job; remove the first; second + legacy survive
	c3 := upsertJob(c2, "job2", "0 3 * * * /bin/job2")
	c4 := removeJob(c3, "job1")
	if _, found := findJob(c4, "job1"); found {
		t.Error("job1 should be gone after removeJob")
	}
	if got, found := findJob(c4, "job2"); !found || got != "0 3 * * * /bin/job2" {
		t.Errorf("job2 should survive: found=%v got=%q", found, got)
	}
	if !strings.Contains(c4, "0 0 * * * /bin/legacy") {
		t.Errorf("legacy line lost: %q", c4)
	}

	// remove a missing job → unchanged
	if removeJob(c4, "nope") != c4 {
		t.Error("removeJob of a missing job should be a no-op")
	}
}

func TestCrontab_EmptyAndDanglingMarker(t *testing.T) {
	t.Parallel()
	// empty crontab → upsert produces just the block, trailing newline
	c := upsertJob("", "j", "@daily /bin/x")
	if c != markerLine("j")+"\n@daily /bin/x\n" {
		t.Errorf("upsert into empty: %q", c)
	}
	// dangling marker (last line) → upsert appends the entry below it
	dangling := "foo\n" + markerLine("j")
	c2 := upsertJob(dangling, "j", "@daily /bin/x")
	if got, found := findJob(c2, "j"); !found || got != "@daily /bin/x" {
		t.Errorf("dangling-marker upsert: found=%v got=%q content=%q", found, got, c2)
	}
	// removeJob of a dangling marker drops just the marker
	c3 := removeJob(dangling, "j")
	if c3 != "foo\n" {
		t.Errorf("removeJob dangling: %q", c3)
	}
}

// --- Check / Apply -----------------------------------------------------

func TestCheckApply_Present(t *testing.T) {
	t.Parallel()
	f := newFake(nil)
	m := NewWithProvider(f)
	d := decl("backup", StatePresent, map[string]any{"command": "/usr/bin/backup", "schedule": "0 2 * * *", "user": "root"})

	// Check: missing → drift
	r, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches {
		t.Error("missing job should drift")
	}

	// Apply: creates it
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("first apply should change")
	}
	got, found := findJob(f.crontabs["root"], "backup")
	if !found || got != "0 2 * * * /usr/bin/backup" {
		t.Fatalf("installed entry wrong: found=%v got=%q", found, got)
	}

	// Check: now matches
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged job should match, diff=%q", r.Diff)
	}

	// Apply again: no-op
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("second apply should be a no-op")
	}
	if sr.Comment != "already converged" {
		t.Errorf("comment=%q", sr.Comment)
	}

	// change the schedule → drift + rewrite in place
	d2 := decl("backup", StatePresent, map[string]any{"command": "/usr/bin/backup", "schedule": "@hourly", "user": "root"})
	r, _ = m.Check(context.Background(), d2)
	if r.Matches {
		t.Error("reschedule should drift")
	}
	sr, _ = m.Apply(context.Background(), d2)
	if !sr.Changed {
		t.Error("reschedule apply should change")
	}
	got, _ = findJob(f.crontabs["root"], "backup")
	if got != "@hourly /usr/bin/backup" {
		t.Errorf("after reschedule: got=%q", got)
	}
	if strings.Count(f.crontabs["root"], markerLine("backup")) != 1 {
		t.Error("reschedule duplicated the entry")
	}
}

func TestCheckApply_NormalisesWhitespace(t *testing.T) {
	t.Parallel()
	// crontab already has the job but with sloppy spacing — Check
	// should drift once, Apply rewrites canonically, then stable.
	f := newFake(map[string]string{"root": markerLine("j") + "\n  */5   *  *  *  *   /bin/x\n"})
	m := NewWithProvider(f)
	d := decl("j", StatePresent, map[string]any{"command": "/bin/x", "schedule": "*/5 * * * *"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("sloppy spacing should drift")
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	got, _ := findJob(f.crontabs["root"], "j")
	if got != "*/5 * * * * /bin/x" {
		t.Errorf("not canonicalised: %q", got)
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("should be stable after canonicalisation")
	}
}

func TestCheckApply_Absent(t *testing.T) {
	t.Parallel()
	f := newFake(map[string]string{"root": "0 0 * * * /bin/legacy\n" + markerLine("old") + "\n@daily /bin/old\n"})
	m := NewWithProvider(f)
	d := decl("old", StateAbsent, nil)

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("present job should drift from absent")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("removal should change")
	}
	if _, found := findJob(f.crontabs["root"], "old"); found {
		t.Error("job not removed")
	}
	if !strings.Contains(f.crontabs["root"], "0 0 * * * /bin/legacy") {
		t.Error("legacy line lost on removal")
	}

	// already absent → no-op
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("absent on a missing job should be a no-op")
	}

	// absent on a totally empty crontab → match, no-op
	f2 := newFake(nil)
	m2 := NewWithProvider(f2)
	r, _ = m2.Check(context.Background(), decl("ghost", StateAbsent, nil))
	if !r.Matches {
		t.Error("absent on empty crontab should match")
	}
}

func TestApply_ReadAndWriteErrors(t *testing.T) {
	t.Parallel()
	f := newFake(nil)
	f.readErr = errors.New("crontab -l blew up")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("j", StatePresent, map[string]any{"command": "x", "schedule": "@daily"})); err == nil {
		t.Error("read error should propagate from Apply")
	}
	f2 := newFake(nil)
	f2.writeErr = errors.New("crontab install failed")
	sr, err := NewWithProvider(f2).Apply(context.Background(), decl("j", StatePresent, map[string]any{"command": "x", "schedule": "@daily"}))
	if err == nil {
		t.Fatal("write error should propagate from Apply")
	}
	if sr == nil || sr.Success {
		t.Error("result should report failure")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "cron" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("cron should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("j", StateAbsent, nil), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(decl("j", StatePresent, map[string]any{"command": "x", "schedule": "@daily"}), nil) != statemgmt.DriftSeverityMedium {
		t.Error("present drift → MEDIUM")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}

	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("j", StatePresent, map[string]any{"command": "x", "schedule": "@daily"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("j", StatePresent, nil)); err == nil {
		t.Error("present-without-command should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(newFake(nil))
	ok, err := m.Test(context.Background(), decl("ghost", StateAbsent, nil))
	if err != nil || !ok {
		t.Errorf("Test on absent-and-missing: ok=%v err=%v", ok, err)
	}
	ok, err = m.Test(context.Background(), decl("j", StatePresent, map[string]any{"command": "x", "schedule": "@daily"}))
	if err != nil || ok {
		t.Errorf("Test on a not-yet-present job should be false: ok=%v err=%v", ok, err)
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || IsUnsupportedOS(errors.New("x")) {
		t.Error("IsUnsupportedOS")
	}
	if !IsNoCrontab(ErrNoCrontab) || IsNoCrontab(errors.New("x")) {
		t.Error("IsNoCrontab")
	}
}
