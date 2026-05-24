// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "git:" + name,
		Module: "git",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider ------------------------------------------------------

type fakeProvider struct {
	state     *RepoState
	stateErr  error
	remoteSHA string
	remoteErr error

	clones  []CloneOptions
	syncs   []SyncOptions
	removed []string

	cloneErr error
	syncErr  error
}

func (f *fakeProvider) Inspect(_ context.Context, _, _ string) (*RepoState, error) {
	if f.stateErr != nil {
		return nil, f.stateErr
	}
	return f.state, nil
}
func (f *fakeProvider) RemoteSHA(_ context.Context, _, _, _ string) (string, error) {
	if f.remoteErr != nil {
		return "", f.remoteErr
	}
	return f.remoteSHA, nil
}
func (f *fakeProvider) Clone(_ context.Context, opts CloneOptions) error {
	f.clones = append(f.clones, opts)
	return f.cloneErr
}
func (f *fakeProvider) Sync(_ context.Context, opts SyncOptions) error {
	f.syncs = append(f.syncs, opts)
	return f.syncErr
}
func (f *fakeProvider) Remove(dir string) error {
	f.removed = append(f.removed, dir)
	return nil
}

const sha40a = "1111111111111111111111111111111111111111"
const sha40b = "2222222222222222222222222222222222222222"

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("/r", StatePresent, map[string]any{"ur1": "x"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_Defaults(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("/r", StatePresent, map[string]any{"url": "u"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Rev != revHEAD || p.Remote != defaultRemote || p.Depth != 0 {
		t.Errorf("unexpected defaults: %+v", p)
	}
	if p.Force {
		t.Error("present should not default force=true")
	}
}

func TestParse_LatestForceDefault(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(decl("/r", StateLatest, map[string]any{"url": "u"}))
	if !p.Force {
		t.Error("latest should default force=true")
	}
	p, _ = parseParams(decl("/r", StateLatest, map[string]any{"url": "u", "force": false}))
	if p.Force {
		t.Error("explicit force:false should be honoured for latest")
	}
}

func TestParse_Depth(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  any
		want int
		bad  bool
	}{
		{1, 1, false},
		{int64(5), 5, false},
		{float64(3), 3, false},
		{"7", 7, false},
		{float64(2.5), 0, true},
		{-1, 0, true},
		{"x", 0, true},
		{true, 0, true},
	}
	for _, c := range cases {
		p, err := parseParams(decl("/r", StatePresent, map[string]any{"url": "u", "depth": c.raw}))
		if c.bad {
			if err == nil {
				t.Errorf("depth %v: expected error", c.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("depth %v: %v", c.raw, err)
			continue
		}
		if p.Depth != c.want {
			t.Errorf("depth %v: got %d want %d", c.raw, p.Depth, c.want)
		}
	}
}

func TestParse_TypeErrors(t *testing.T) {
	t.Parallel()
	for _, p := range []map[string]any{
		{"url": 1}, {"rev": 2}, {"remote": 3}, {"force": "yes"},
	} {
		if _, err := parseParams(decl("/r", StatePresent, p)); err == nil {
			t.Errorf("%v: expected type error", p)
		}
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"present needs url", decl("/r", StatePresent, nil), true},
		{"present ok", decl("/r", StatePresent, map[string]any{"url": "u"}), false},
		{"latest ok", decl("/r", StateLatest, map[string]any{"url": "u"}), false},
		{"absent ok", decl("/r", StateAbsent, nil), false},
		{"absent rejects url", decl("/r", StateAbsent, map[string]any{"url": "u"}), true},
		{"absent rejects rev", decl("/r", StateAbsent, map[string]any{"rev": "main"}), true},
		{"absent rejects depth", decl("/r", StateAbsent, map[string]any{"depth": 1}), true},
		{"absent rejects remote", decl("/r", StateAbsent, map[string]any{"remote": "upstream"}), true},
		{"bad state", decl("/r", "frob", nil), true},
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

func TestIsFullSHA(t *testing.T) {
	t.Parallel()
	if !isFullSHA(sha40a) {
		t.Error("40 hex chars should be a full SHA")
	}
	if isFullSHA("main") || isFullSHA("abc") || isFullSHA(sha40a+"0") {
		t.Error("non-SHA strings should not match")
	}
}

// --- Check -------------------------------------------------------------

func newWith(f *fakeProvider) statemgmt.Module { return NewWithProvider(f) }

func TestCheck_Absent(t *testing.T) {
	t.Parallel()
	m := newWith(&fakeProvider{state: &RepoState{Exists: false}})
	r, err := m.Check(context.Background(), decl("/r", StateAbsent, nil))
	if err != nil || !r.Matches {
		t.Fatalf("no repo should match absent: %v %v", r, err)
	}
	m = newWith(&fakeProvider{state: &RepoState{Exists: true}})
	r, _ = m.Check(context.Background(), decl("/r", StateAbsent, nil))
	if r.Matches {
		t.Error("present repo should drift from absent")
	}
}

func TestCheck_Present(t *testing.T) {
	t.Parallel()
	d := decl("/r", StatePresent, map[string]any{"url": "git://u"})

	r, _ := newWith(&fakeProvider{state: &RepoState{Exists: false}}).Check(context.Background(), d)
	if r.Matches {
		t.Error("not-cloned should drift")
	}
	r, _ = newWith(&fakeProvider{state: &RepoState{Exists: true, RemoteURL: "git://other"}}).Check(context.Background(), d)
	if r.Matches {
		t.Error("wrong remote url should drift")
	}
	r, err := newWith(&fakeProvider{state: &RepoState{Exists: true, RemoteURL: "git://u", HeadSHA: sha40a}}).Check(context.Background(), d)
	if err != nil || !r.Matches {
		t.Errorf("matching url should match for present: %v %v", r, err)
	}
}

func TestCheck_Latest(t *testing.T) {
	t.Parallel()
	d := decl("/r", StateLatest, map[string]any{"url": "u", "rev": "main"})

	// not cloned
	r, _ := newWith(&fakeProvider{state: &RepoState{Exists: false}}).Check(context.Background(), d)
	if r.Matches {
		t.Error("not cloned should drift")
	}
	// behind
	f := &fakeProvider{state: &RepoState{Exists: true, RemoteURL: "u", HeadSHA: sha40a}, remoteSHA: sha40b}
	r, _ = newWith(f).Check(context.Background(), d)
	if r.Matches {
		t.Error("behind should drift")
	}
	// up to date
	f = &fakeProvider{state: &RepoState{Exists: true, RemoteURL: "u", HeadSHA: sha40b}, remoteSHA: sha40b}
	r, err := newWith(f).Check(context.Background(), d)
	if err != nil || !r.Matches {
		t.Errorf("up to date should match: %v %v", r, err)
	}
	// rev is a full SHA → no remote lookup; remoteErr would fire if it did
	dsha := decl("/r", StateLatest, map[string]any{"url": "u", "rev": sha40b})
	f = &fakeProvider{state: &RepoState{Exists: true, RemoteURL: "u", HeadSHA: sha40b}, remoteErr: errors.New("must not be called")}
	r, err = newWith(f).Check(context.Background(), dsha)
	if err != nil || !r.Matches {
		t.Errorf("SHA-pinned latest should match without ls-remote: %v %v", r, err)
	}
	// RemoteSHA error propagates
	f = &fakeProvider{state: &RepoState{Exists: true, RemoteURL: "u", HeadSHA: sha40a}, remoteErr: errors.New("network")}
	if _, err := newWith(f).Check(context.Background(), d); err == nil {
		t.Error("RemoteSHA error should propagate from Check")
	}
}

// --- Apply -------------------------------------------------------------

func TestApply_AbsentRemovesRepo(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: &RepoState{Exists: true}}
	r, err := newWith(f).Apply(context.Background(), decl("/r", StateAbsent, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Changed || len(f.removed) != 1 || f.removed[0] != "/r" {
		t.Errorf("expected Remove(/r); changed=%v removed=%v", r.Changed, f.removed)
	}

	f = &fakeProvider{state: &RepoState{Exists: false}}
	r, _ = newWith(f).Apply(context.Background(), decl("/r", StateAbsent, nil))
	if r.Changed || len(f.removed) != 0 {
		t.Error("absent on a missing repo should be a no-op")
	}
}

func TestApply_PresentClones(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo") // does not exist yet
	f := &fakeProvider{state: &RepoState{Exists: false}}
	d := decl(repo, StatePresent, map[string]any{"url": "git://u", "rev": "v1.2.0", "depth": 1, "remote": "upstream"})
	r, err := newWith(f).Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Changed || len(f.clones) != 1 {
		t.Fatalf("expected one Clone; changed=%v clones=%v", r.Changed, f.clones)
	}
	got := f.clones[0]
	if got.URL != "git://u" || got.Dir != repo || got.Rev != "v1.2.0" || got.Depth != 1 || got.Remote != "upstream" {
		t.Errorf("Clone opts wrong: %+v", got)
	}
}

func TestApply_PresentRefusesNonEmptyNonRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// dir is non-empty and not a repo
	if err := os.WriteFile(filepath.Join(dir, "stuff"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeProvider{state: &RepoState{Exists: false}}
	_, err := newWith(f).Apply(context.Background(), decl(dir, StatePresent, map[string]any{"url": "u"}))
	if err == nil || !errors.Is(err, ErrNotARepo) {
		t.Fatalf("expected ErrNotARepo, got %v", err)
	}
	if len(f.clones) != 0 {
		t.Error("should not have cloned into a non-empty directory")
	}
}

func TestApply_PresentRefusesWrongRemote(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: &RepoState{Exists: true, RemoteURL: "git://different"}}
	_, err := newWith(f).Apply(context.Background(), decl("/r", StatePresent, map[string]any{"url": "git://u"}))
	if err == nil {
		t.Fatal("expected an error for a repo with the wrong remote")
	}
}

func TestApply_LatestSyncs(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: &RepoState{Exists: true, RemoteURL: "u", HeadSHA: sha40a}, remoteSHA: sha40b}
	d := decl("/r", StateLatest, map[string]any{"url": "u", "rev": "main", "depth": 2})
	r, err := newWith(f).Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Changed || len(f.syncs) != 1 {
		t.Fatalf("expected one Sync; changed=%v syncs=%v", r.Changed, f.syncs)
	}
	s := f.syncs[0]
	if s.Dir != "/r" || s.Rev != "main" || s.Depth != 2 || s.Remote != defaultRemote || !s.Force {
		t.Errorf("Sync opts wrong: %+v", s)
	}
}

func TestApply_IdempotentWhenConverged(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: &RepoState{Exists: true, RemoteURL: "u", HeadSHA: sha40b}, remoteSHA: sha40b}
	r, err := newWith(f).Apply(context.Background(), decl("/r", StateLatest, map[string]any{"url": "u", "rev": "main"}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Changed || r.Comment != "already converged" {
		t.Errorf("expected no-op apply, got changed=%v comment=%q", r.Changed, r.Comment)
	}
	if len(f.syncs) != 0 {
		t.Error("converged repo should not be synced")
	}
}

func TestApply_CloneErrorSurfaces(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := &fakeProvider{state: &RepoState{Exists: false}, cloneErr: errors.New("boom")}
	r, err := newWith(f).Apply(context.Background(), decl(filepath.Join(dir, "x"), StatePresent, map[string]any{"url": "u"}))
	if err == nil {
		t.Fatal("expected clone error")
	}
	if r == nil || r.Success {
		t.Error("result should report failure")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "git" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 3 {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("git should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("/r", StateAbsent, nil), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent → HIGH")
	}
	if dsm.DriftSeverity(decl("/r", StatePresent, map[string]any{"url": "u"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("present → HIGH")
	}
	if dsm.DriftSeverity(decl("/r", StateLatest, map[string]any{"url": "u"}), &statemgmt.ModuleCheckResult{Diff: "not cloned; want u"}) != statemgmt.DriftSeverityHigh {
		t.Error("latest+not-cloned → HIGH")
	}
	if dsm.DriftSeverity(decl("/r", StateLatest, map[string]any{"url": "u"}), &statemgmt.ModuleCheckResult{Diff: "HEAD aaa → bbb"}) != statemgmt.DriftSeverityMedium {
		t.Error("latest+behind → MEDIUM")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
}

func TestValidate_ViaModule(t *testing.T) {
	t.Parallel()
	vm := New().(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("/r", StatePresent, map[string]any{"url": "u"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("/r", StatePresent, nil)); err == nil {
		t.Error("present-without-url should be rejected")
	}
}

// --- gitcli: arg formation + parsing -----------------------------------

type recordedCall struct {
	args []string
}

func recorder(out map[int]string) (commandRunner, *[]recordedCall) {
	var calls []recordedCall
	run := func(_ context.Context, _ string, args ...string) (string, error) {
		i := len(calls)
		calls = append(calls, recordedCall{args: args})
		return out[i], nil
	}
	return run, &calls
}

func gitDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCLI_Inspect(t *testing.T) {
	t.Parallel()
	dir := gitDir(t)
	run, calls := recorder(map[int]string{0: "git://configured\n", 1: sha40a + "\n"})
	p := &cliProvider{git: "git", run: run}
	st, err := p.Inspect(context.Background(), dir, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Exists || st.RemoteURL != "git://configured" || st.HeadSHA != sha40a {
		t.Errorf("RepoState wrong: %+v", st)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 git calls, got %d", len(*calls))
	}
	if got := strings.Join((*calls)[0].args, " "); got != "-C "+dir+" config --get remote.origin.url" {
		t.Errorf("config call args: %q", got)
	}
	if got := strings.Join((*calls)[1].args, " "); got != "-C "+dir+" rev-parse HEAD" {
		t.Errorf("rev-parse call args: %q", got)
	}
}

func TestCLI_Inspect_NotARepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // no .git
	run, calls := recorder(nil)
	p := &cliProvider{git: "git", run: run}
	st, err := p.Inspect(context.Background(), dir, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if st.Exists {
		t.Error("dir without .git should report Exists=false")
	}
	if len(*calls) != 0 {
		t.Error("Inspect on a non-repo should not shell out to git")
	}
}

func TestCLI_Clone_ArgForms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		opts CloneOptions
		want []string // first call (clone); second call (checkout) checked separately
	}{
		{
			CloneOptions{URL: "u", Dir: "d", Rev: revHEAD},
			[]string{"clone", "--", "u", "d"},
		},
		{
			CloneOptions{URL: "u", Dir: "d", Rev: "v1.0", Depth: 1, Remote: "upstream"},
			[]string{"clone", "--depth", "1", "--origin", "upstream", "--branch", "v1.0", "--", "u", "d"},
		},
		{
			CloneOptions{URL: "u", Dir: "d", Rev: sha40a},
			[]string{"clone", "--", "u", "d"},
		},
	}
	for _, c := range cases {
		run, calls := recorder(nil)
		p := &cliProvider{git: "git", run: run}
		if err := p.Clone(context.Background(), c.opts); err != nil {
			t.Fatalf("Clone(%+v): %v", c.opts, err)
		}
		if got := strings.Join((*calls)[0].args, " "); got != strings.Join(c.want, " ") {
			t.Errorf("Clone(%+v) args = %q, want %q", c.opts, got, strings.Join(c.want, " "))
		}
		if isFullSHA(c.opts.Rev) {
			if len(*calls) != 2 {
				t.Fatalf("SHA rev should trigger a checkout call, got %d calls", len(*calls))
			}
			if got := strings.Join((*calls)[1].args, " "); got != "-C d checkout "+sha40a {
				t.Errorf("checkout args = %q", got)
			}
		}
	}
}

func TestCLI_Sync_ArgForms(t *testing.T) {
	t.Parallel()
	// force → reset --hard
	run, calls := recorder(nil)
	p := &cliProvider{git: "git", run: run}
	if err := p.Sync(context.Background(), SyncOptions{Dir: "d", Rev: "main", Depth: 3, Remote: "origin", Force: true}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join((*calls)[0].args, " "); got != "-C d fetch --depth 3 -- origin main" {
		t.Errorf("fetch args = %q", got)
	}
	if got := strings.Join((*calls)[1].args, " "); got != "-C d reset --hard FETCH_HEAD" {
		t.Errorf("reset args = %q", got)
	}
	// no force → merge --ff-only
	run, calls = recorder(nil)
	p = &cliProvider{git: "git", run: run}
	if err := p.Sync(context.Background(), SyncOptions{Dir: "d", Rev: "main", Remote: "origin", Force: false}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join((*calls)[0].args, " "); got != "-C d fetch -- origin main" {
		t.Errorf("fetch args = %q", got)
	}
	if got := strings.Join((*calls)[1].args, " "); got != "-C d merge --ff-only FETCH_HEAD" {
		t.Errorf("merge args = %q", got)
	}
}

func TestCLI_RemoteSHA(t *testing.T) {
	t.Parallel()
	run, _ := recorder(map[int]string{0: sha40a + "\trefs/heads/main\n"})
	p := &cliProvider{git: "git", run: run}
	got, err := p.RemoteSHA(context.Background(), "d", "origin", "main")
	if err != nil || got != sha40a {
		t.Fatalf("RemoteSHA = %q, %v", got, err)
	}

	// empty output → error
	run, _ = recorder(map[int]string{0: "\n"})
	p = &cliProvider{git: "git", run: run}
	if _, err := p.RemoteSHA(context.Background(), "d", "origin", "nope"); err == nil {
		t.Error("empty ls-remote output should error")
	}
}

func TestParseLsRemote(t *testing.T) {
	t.Parallel()
	if got := parseLsRemote(sha40a+"\trefs/heads/main\n"+sha40b+"\trefs/heads/dev\n", "main"); got != sha40a {
		t.Errorf("first-line pick: got %q", got)
	}
	out := sha40a + "\trefs/tags/v1\n" + sha40b + "\trefs/tags/v1^{}\n"
	if got := parseLsRemote(out, "v1"); got != sha40b {
		t.Errorf("annotated-tag deref: got %q want %q", got, sha40b)
	}
	headOut := sha40a + "\tHEAD\n" + sha40b + "\trefs/heads/main\n"
	if got := parseLsRemote(headOut, revHEAD); got != sha40a {
		t.Errorf("HEAD pick: got %q", got)
	}
	if got := parseLsRemote("garbage line with no tab\n", "x"); got != "" {
		t.Errorf("unparseable → empty, got %q", got)
	}
}

// --- notInstalledProvider ---------------------------------------------

func TestNotInstalledProvider(t *testing.T) {
	t.Parallel()
	p := &notInstalledProvider{}
	if err := p.Clone(context.Background(), CloneOptions{}); !errors.Is(err, ErrGitNotFound) {
		t.Errorf("Clone err = %v", err)
	}
	if err := p.Sync(context.Background(), SyncOptions{}); !errors.Is(err, ErrGitNotFound) {
		t.Errorf("Sync err = %v", err)
	}
	if _, err := p.RemoteSHA(context.Background(), "", "", ""); !errors.Is(err, ErrGitNotFound) {
		t.Errorf("RemoteSHA err = %v", err)
	}
	// Inspect via filesystem
	dir := gitDir(t)
	st, err := p.Inspect(context.Background(), dir, "origin")
	if err != nil || !st.Exists {
		t.Errorf("Inspect on a .git dir: %+v %v", st, err)
	}
	st, _ = p.Inspect(context.Background(), t.TempDir(), "origin")
	if st.Exists {
		t.Error("Inspect on a bare dir should report not-exists")
	}
	// Remove works without git
	rm := t.TempDir()
	if err := p.Remove(rm); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rm); !os.IsNotExist(err) {
		t.Error("Remove should have deleted the directory")
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsGitNotFound(ErrGitNotFound) || IsGitNotFound(errors.New("x")) {
		t.Error("IsGitNotFound")
	}
	if !IsNotARepo(ErrNotARepo) || IsNotARepo(errors.New("x")) {
		t.Error("IsNotARepo")
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	// Whether git is installed or not, defaultProvider must return a
	// usable Provider.
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}
