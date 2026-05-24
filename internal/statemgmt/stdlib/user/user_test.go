// SPDX-License-Identifier: Apache-2.0

package user

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// fakeProvider records every call so tests can pin which Provider
// method fires on which transition. Lookup uses an in-memory
// table; mutations update it.
type fakeProvider struct {
	users         map[string]UserInfo
	addErr        error
	modErr        error
	delErr        error
	setGroupsErr  error
	addCalls      []AddOptions
	modCalls      []ModOptions
	delCalls      []delCall
	setGroupsCalls []setGroupsCall
}

type delCall struct {
	Name       string
	RemoveHome bool
}
type setGroupsCall struct {
	Name   string
	Groups []string
}

func newFake(seed ...UserInfo) *fakeProvider {
	f := &fakeProvider{users: map[string]UserInfo{}}
	for _, u := range seed {
		f.users[u.Name] = u
	}
	return f
}

func (f *fakeProvider) Lookup(name string) (*UserInfo, error) {
	u, ok := f.users[name]
	if !ok {
		return nil, nil
	}
	cp := u
	cp.Groups = append([]string(nil), u.Groups...)
	return &cp, nil
}

func (f *fakeProvider) Add(_ context.Context, opts AddOptions) error {
	f.addCalls = append(f.addCalls, opts)
	if f.addErr != nil {
		return f.addErr
	}
	uid := 5000
	gid := 5000
	if opts.UID != nil {
		uid = *opts.UID
	}
	if opts.GID != nil {
		gid = *opts.GID
	}
	f.users[opts.Name] = UserInfo{
		Name: opts.Name, UID: uid, GID: gid,
		Home: opts.Home, Shell: opts.Shell, Comment: opts.Comment,
		Groups: append([]string(nil), opts.Groups...),
	}
	return nil
}

func (f *fakeProvider) Mod(_ context.Context, opts ModOptions) error {
	f.modCalls = append(f.modCalls, opts)
	if f.modErr != nil {
		return f.modErr
	}
	u, ok := f.users[opts.Name]
	if !ok {
		return errors.New("usermod: no such user")
	}
	if opts.UID != nil {
		u.UID = *opts.UID
	}
	if opts.GID != nil {
		u.GID = *opts.GID
	}
	if opts.Home != "" {
		u.Home = opts.Home
	}
	if opts.Shell != "" {
		u.Shell = opts.Shell
	}
	if opts.Comment != "" {
		u.Comment = opts.Comment
	}
	f.users[opts.Name] = u
	return nil
}

func (f *fakeProvider) Del(_ context.Context, name string, removeHome bool) error {
	f.delCalls = append(f.delCalls, delCall{Name: name, RemoveHome: removeHome})
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.users, name)
	return nil
}

func (f *fakeProvider) SetGroups(_ context.Context, name string, groups []string) error {
	f.setGroupsCalls = append(f.setGroupsCalls, setGroupsCall{Name: name, Groups: append([]string(nil), groups...)})
	if f.setGroupsErr != nil {
		return f.setGroupsErr
	}
	u, ok := f.users[name]
	if ok {
		u.Groups = append([]string(nil), groups...)
		f.users[name] = u
	}
	return nil
}

func declFor(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID: "user:" + name, Module: "user", Name: name, State: state, Params: params,
	}
}

func newModuleWith(p Provider) *Module { return &Module{provider: p} }

// ---- parseParams / validate ---------------------------------------

func TestParseParams_RejectsUnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("alice", StatePresent, map[string]any{"badkey": "x"}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param error", err)
	}
}

func TestValidate_GIDGroupMutex(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("alice", StatePresent, map[string]any{
		"gid": 1500, "group": "wheel",
	}))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("want gid+group mutex error, got %v", err)
	}
}

func TestValidate_HomeMustBeAbsolute(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("alice", StatePresent, map[string]any{"home": "relative/path"}))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("want home-absolute error, got %v", err)
	}
}

func TestValidate_ShellMustBeAbsolute(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("alice", StatePresent, map[string]any{"shell": "bash"}))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("want shell-absolute error, got %v", err)
	}
}

func TestValidate_AbsentRejectsAttrs(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("alice", StateAbsent, map[string]any{
		"uid": 1500, "home": "/home/alice",
	}))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "absent") {
		t.Errorf("want absent-rejects-attrs error, got %v", err)
	}
}

func TestValidate_AbsentAllowsRemoveHome(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("alice", StateAbsent, map[string]any{
		"remove_home": true,
	}))
	if err := p.validate(); err != nil {
		t.Errorf("remove_home should be allowed on absent; got %v", err)
	}
}

func TestValidate_RemoveHomeRejectedOnPresent(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("alice", StatePresent, map[string]any{
		"remove_home": true,
	}))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "remove_home") {
		t.Errorf("want remove_home-rejected error, got %v", err)
	}
}

func TestValidate_BadUserName(t *testing.T) {
	t.Parallel()
	bad := []string{"UPPER", "1leadingdigit", "with spaces", "evil@user", ""}
	for _, name := range bad {
		decl := &statemgmt.Declaration{
			ID: "user:" + name, Module: "user", Name: name, State: StatePresent,
		}
		p, err := parseParams(decl)
		if err != nil {
			continue
		}
		if err := p.validate(); err == nil {
			t.Errorf("name %q should be rejected", name)
		}
	}
}

func TestParseParams_GroupsListParsing(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("alice", StatePresent, map[string]any{
		"groups": []any{"wheel", "docker"},
	}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !p.HasGroups || len(p.Groups) != 2 {
		t.Errorf("Groups = %+v HasGroups=%v, want 2 entries", p.Groups, p.HasGroups)
	}
}

func TestParseParams_GroupsRejectsNonList(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("alice", StatePresent, map[string]any{"groups": "wheel"}))
	if err == nil || !strings.Contains(err.Error(), "groups") {
		t.Errorf("want groups-list error, got %v", err)
	}
}

func TestParseParams_GroupsRejectsNonStringEntries(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("alice", StatePresent, map[string]any{
		"groups": []any{"wheel", 42},
	}))
	if err == nil {
		t.Error("expected groups-entry-type error")
	}
}

func TestParseParams_GroupsEmptyListCountsAsExplicit(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("alice", StatePresent, map[string]any{
		"groups": []any{},
	}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !p.HasGroups {
		t.Error("explicit empty list should set HasGroups (operator wants no supp groups)")
	}
}

// ---- Module surface ----------------------------------------------

func TestModule_NameAndStates(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "user" {
		t.Errorf("Name = %q", m.Name())
	}
	if len(m.ValidStates()) != 2 {
		t.Errorf("ValidStates = %v", m.ValidStates())
	}
}

func TestModule_ImplementsOptionalInterfaces(t *testing.T) {
	t.Parallel()
	var _ statemgmt.ValidatableModule = &Module{}
	var _ statemgmt.DriftSeverityModule = &Module{}
}

func TestModule_DriftSeverity(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	if got := m.DriftSeverity(declFor("alice", StateAbsent, nil), nil); got != statemgmt.DriftSeverityHigh {
		t.Errorf("absent severity = %v, want high", got)
	}
	if got := m.DriftSeverity(declFor("alice", StatePresent, nil), nil); got != statemgmt.DriftSeverityMedium {
		t.Errorf("present severity = %v, want medium", got)
	}
}

func TestNew_DefaultProvider(t *testing.T) {
	t.Parallel()
	m := New()
	if m == nil {
		t.Fatal("New returned nil")
	}
}

func TestNewWithProvider_UsesInjected(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500})
	m := NewWithProvider(f)
	res, err := m.Check(context.Background(), declFor("alice", StatePresent, nil))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.Matches {
		t.Errorf("seeded user should match; diff = %q", res.Diff)
	}
}

// ---- Check -------------------------------------------------------

func TestCheck_PresentMissing(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	res, _ := m.Check(context.Background(), declFor("alice", StatePresent, nil))
	if res.Matches || !strings.Contains(res.Diff, "missing") {
		t.Errorf("expected missing-drift; got %+v", res)
	}
}

func TestCheck_PresentMatchesAllFields(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{
		Name: "alice", UID: 1500, GID: 1500,
		Home: "/home/alice", Shell: "/bin/bash", Comment: "Alice",
		Groups: []string{"docker", "wheel"},
	})
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("alice", StatePresent, map[string]any{
		"uid": 1500, "gid": 1500,
		"home": "/home/alice", "shell": "/bin/bash", "comment": "Alice",
		"groups": []any{"wheel", "docker"}, // order-independent
	}))
	if !res.Matches {
		t.Errorf("expected match; diff = %q", res.Diff)
	}
}

func TestCheck_UIDMismatch(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500})
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("alice", StatePresent, map[string]any{"uid": 1600}))
	if res.Matches {
		t.Error("UID drift not detected")
	}
	if !strings.Contains(res.Diff, "1500") || !strings.Contains(res.Diff, "1600") {
		t.Errorf("diff should cite both UIDs; got %q", res.Diff)
	}
}

func TestCheck_GIDMismatch(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500})
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("alice", StatePresent, map[string]any{"gid": 1600}))
	if res.Matches {
		t.Error("GID drift not detected")
	}
}

func TestCheck_HomeMismatch(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500, Home: "/home/alice"})
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("alice", StatePresent, map[string]any{
		"home": "/var/lib/alice",
	}))
	if res.Matches {
		t.Error("home drift not detected")
	}
}

func TestCheck_ShellMismatch(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500, Shell: "/bin/sh"})
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("alice", StatePresent, map[string]any{
		"shell": "/bin/bash",
	}))
	if res.Matches {
		t.Error("shell drift not detected")
	}
}

func TestCheck_CommentMismatch(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500, Comment: "Old"})
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("alice", StatePresent, map[string]any{
		"comment": "New",
	}))
	if res.Matches {
		t.Error("comment drift not detected")
	}
}

func TestCheck_GroupsAddedToLive(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500, Groups: []string{"wheel"}})
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("alice", StatePresent, map[string]any{
		"groups": []any{"wheel", "docker"},
	}))
	if res.Matches {
		t.Error("group-added drift not detected")
	}
	if !strings.Contains(res.Diff, "groups") {
		t.Errorf("diff should cite groups; got %q", res.Diff)
	}
}

func TestCheck_GroupsRemovedFromLive(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500, Groups: []string{"docker", "wheel"}})
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("alice", StatePresent, map[string]any{
		"groups": []any{"wheel"},
	}))
	if res.Matches {
		t.Error("group-removed drift not detected")
	}
}

func TestCheck_AbsentMissing(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	res, _ := m.Check(context.Background(), declFor("alice", StateAbsent, nil))
	if !res.Matches {
		t.Error("absent+missing should match")
	}
}

func TestCheck_AbsentPresent(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake(UserInfo{Name: "alice", UID: 1500}))
	res, _ := m.Check(context.Background(), declFor("alice", StateAbsent, nil))
	if res.Matches {
		t.Error("absent+present should drift")
	}
}

// ---- Apply -------------------------------------------------------

func TestApply_Missing_CallsAdd(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	uid := 1500
	_, err := m.Apply(context.Background(), declFor("alice", StatePresent, map[string]any{
		"uid":         uid,
		"home":        "/home/alice",
		"shell":       "/bin/bash",
		"groups":      []any{"wheel"},
		"create_home": true,
	}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.addCalls) != 1 {
		t.Fatalf("Add calls = %d, want 1", len(f.addCalls))
	}
	got := f.addCalls[0]
	if got.UID == nil || *got.UID != uid || got.Home != "/home/alice" || got.Shell != "/bin/bash" {
		t.Errorf("Add args lost: %+v", got)
	}
	if !got.CreateHome {
		t.Error("CreateHome should be true")
	}
	if len(got.Groups) != 1 || got.Groups[0] != "wheel" {
		t.Errorf("Groups lost: %+v", got.Groups)
	}
}

func TestApply_ScalarChangeOnly_CallsModNotSetGroups(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500, Shell: "/bin/sh", Groups: []string{"wheel"}})
	m := newModuleWith(f)
	_, err := m.Apply(context.Background(), declFor("alice", StatePresent, map[string]any{
		"shell":  "/bin/bash",
		"groups": []any{"wheel"}, // unchanged
	}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.modCalls) != 1 {
		t.Errorf("Mod calls = %d, want 1", len(f.modCalls))
	}
	if len(f.setGroupsCalls) != 0 {
		t.Errorf("SetGroups should NOT fire on unchanged group set; got %d calls", len(f.setGroupsCalls))
	}
}

func TestApply_GroupsChangeOnly_CallsSetGroupsNotMod(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500, Shell: "/bin/sh", Groups: []string{"wheel"}})
	m := newModuleWith(f)
	_, err := m.Apply(context.Background(), declFor("alice", StatePresent, map[string]any{
		"groups": []any{"wheel", "docker"},
	}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.setGroupsCalls) != 1 {
		t.Errorf("SetGroups calls = %d, want 1", len(f.setGroupsCalls))
	}
	if len(f.modCalls) != 0 {
		t.Errorf("Mod should NOT fire when only groups changed; got %d calls", len(f.modCalls))
	}
}

func TestApply_BothChange_CallsModAndSetGroups(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500, Shell: "/bin/sh", Groups: []string{"wheel"}})
	m := newModuleWith(f)
	_, err := m.Apply(context.Background(), declFor("alice", StatePresent, map[string]any{
		"shell":  "/bin/bash",
		"groups": []any{"docker"},
	}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.modCalls) != 1 || len(f.setGroupsCalls) != 1 {
		t.Errorf("got mod=%d set=%d, want 1/1", len(f.modCalls), len(f.setGroupsCalls))
	}
}

func TestApply_AlreadyConverged_NoCalls(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500, GID: 1500, Shell: "/bin/bash", Groups: []string{"wheel"}})
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("alice", StatePresent, map[string]any{
		"uid": 1500, "gid": 1500, "shell": "/bin/bash",
		"groups": []any{"wheel"},
	}))
	if res.Changed {
		t.Error("converged Apply should be Changed=false")
	}
	if len(f.addCalls)+len(f.modCalls)+len(f.delCalls)+len(f.setGroupsCalls) != 0 {
		t.Errorf("no provider calls expected; got add=%d mod=%d del=%d set=%d",
			len(f.addCalls), len(f.modCalls), len(f.delCalls), len(f.setGroupsCalls))
	}
}

func TestApply_AbsentPresent_CallsDelWithRemoveHome(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500})
	m := newModuleWith(f)
	_, err := m.Apply(context.Background(), declFor("alice", StateAbsent, map[string]any{
		"remove_home": true,
	}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.delCalls) != 1 || f.delCalls[0].Name != "alice" || !f.delCalls[0].RemoveHome {
		t.Errorf("Del call wrong: %+v", f.delCalls)
	}
}

func TestApply_AbsentMissing_NoCalls(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("alice", StateAbsent, nil))
	if res.Changed {
		t.Error("absent+missing should be Changed=false")
	}
	if len(f.delCalls) != 0 {
		t.Error("Del shouldn't fire on absent+missing")
	}
}

func TestApply_ProviderErrorPropagates(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.addErr = errors.New("useradd: name in use")
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("alice", StatePresent, nil))
	if err == nil || !strings.Contains(err.Error(), "name in use") {
		t.Errorf("err = %v, want underlying provider error", err)
	}
	if res.Success || res.Changed {
		t.Errorf("Success=%v Changed=%v, want false/false", res.Success, res.Changed)
	}
}

// ---- End-to-end --------------------------------------------------

func TestModule_EndToEnd_PresentLifecycle(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := newModuleWith(f)
	decl := declFor("alice", StatePresent, map[string]any{
		"uid": 1500, "home": "/home/alice", "shell": "/bin/bash",
		"groups": []any{"wheel"},
	})

	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	ok, _ := m.Test(context.Background(), decl)
	if !ok {
		t.Error("Test should match after Apply")
	}
	res, _ := m.Apply(context.Background(), decl)
	if res.Changed {
		t.Error("re-Apply should be Changed=false")
	}
}

func TestModule_EndToEnd_AbsentLifecycle(t *testing.T) {
	t.Parallel()
	f := newFake(UserInfo{Name: "alice", UID: 1500})
	m := newModuleWith(f)
	decl := declFor("alice", StateAbsent, nil)
	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	ok, _ := m.Test(context.Background(), decl)
	if !ok {
		t.Error("Test should match after Del")
	}
}

// ---- IsUnsupportedOS + Validate wrapper --------------------------

func TestModule_Validate_PassesGood(t *testing.T) {
	t.Parallel()
	m := &Module{}
	if err := m.Validate(declFor("alice", StatePresent, map[string]any{"uid": 1500})); err != nil {
		t.Errorf("Validate should pass; got %v", err)
	}
}

func TestModule_Validate_RejectsBad(t *testing.T) {
	t.Parallel()
	m := &Module{}
	if err := m.Validate(declFor("UPPER", StatePresent, nil)); err == nil {
		t.Error("uppercase name should be rejected")
	}
}

func TestIsUnsupportedOS(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) {
		t.Error("sentinel should match itself")
	}
	if IsUnsupportedOS(errors.New("other")) {
		t.Error("unrelated error matched")
	}
}

func TestOSLookup_MissingUser(t *testing.T) {
	t.Parallel()
	info, err := osLookup{}.Lookup("zzz-no-such-user-zzz")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info != nil {
		t.Errorf("expected nil; got %+v", info)
	}
}

func TestGroupsEqual_OrderIndependent(t *testing.T) {
	t.Parallel()
	if !groupsEqual([]string{"a", "b"}, []string{"b", "a"}) {
		t.Error("expected order-independent equality")
	}
	if groupsEqual([]string{"a"}, []string{"a", "b"}) {
		t.Error("different lengths should not match")
	}
}

func TestShellFromPasswd(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/passwd"
	content := "# comment\n" +
		"root:x:0:0:root:/root:/bin/bash\n" +
		"\n" +
		"alice:x:1500:1500:Alice:/home/alice:/bin/zsh\n"
	if err := writeFile(path, content); err != nil {
		t.Fatalf("seed: %v", err)
	}
	shell, err := shellFromPasswd(path, "alice")
	if err != nil {
		t.Fatalf("shellFromPasswd: %v", err)
	}
	if shell != "/bin/zsh" {
		t.Errorf("shell = %q, want /bin/zsh", shell)
	}
}

func TestShellFromPasswd_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/passwd"
	if err := writeFile(path, "root:x:0:0:root:/root:/bin/bash\n"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := shellFromPasswd(path, "absent")
	if err == nil {
		t.Error("expected not-found error")
	}
}

func TestShellFromPasswd_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := shellFromPasswd("/no/such/passwd", "alice")
	if err == nil {
		t.Error("expected file-open error")
	}
}

// writeFile is a tiny helper for the seed-file tests.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

func TestCoerceInt_AllShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v       any
		want    int
		wantErr bool
	}{
		{42, 42, false},
		{int64(42), 42, false},
		{float64(42), 42, false},
		{42.5, 0, true},      // fractional → reject
		{"not int", 0, true}, // wrong type
	}
	for _, c := range cases {
		got, err := coerceInt(c.v)
		if (err != nil) != c.wantErr {
			t.Errorf("coerceInt(%v) err=%v, wantErr=%v", c.v, err, c.wantErr)
		}
		if !c.wantErr && got != c.want {
			t.Errorf("coerceInt(%v) = %d, want %d", c.v, got, c.want)
		}
	}
}

func TestParseParams_AllScalarFields(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("alice", StatePresent, map[string]any{
		"uid":         1500,
		"gid":         int64(1500),
		"home":        "/home/alice",
		"shell":       "/bin/bash",
		"comment":     "Alice",
		"system":      true,
		"create_home": true,
	}))
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if p.UID == nil || *p.UID != 1500 || p.GID == nil || *p.GID != 1500 {
		t.Errorf("UID/GID lost: %+v / %+v", p.UID, p.GID)
	}
	if !p.System || !p.CreateHome {
		t.Errorf("bool flags lost: System=%v CreateHome=%v", p.System, p.CreateHome)
	}
}

func TestParseParams_NonStringScalarsRejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key string
		val any
	}{
		{"group", 42},
		{"home", 1},
		{"shell", 1},
		{"comment", 1},
		{"system", "not-a-bool"},
		{"create_home", "not-a-bool"},
		{"remove_home", "not-a-bool"},
		{"gid", "not-a-number"},
		{"uid", "not-a-number"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			_, err := parseParams(declFor("alice", StatePresent, map[string]any{c.key: c.val}))
			if err == nil {
				t.Errorf("%s: expected type-rejection error", c.key)
			}
		})
	}
}

func TestValidate_OutOfRangeUIDGID(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"uid", "gid"} {
		t.Run(key+"-negative", func(t *testing.T) {
			p, _ := parseParams(declFor("alice", StatePresent, map[string]any{key: -1}))
			if err := p.validate(); err == nil {
				t.Errorf("%s: negative should be rejected", key)
			}
		})
	}
}

func TestGroupNameForGID_Resolves(t *testing.T) {
	t.Parallel()
	// gid 0 (root) is universally present.
	name, err := groupNameForGID("0")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if name == "" {
		t.Error("expected non-empty group name")
	}
}

func TestGroupNameForGID_Unresolvable(t *testing.T) {
	t.Parallel()
	_, err := groupNameForGID("9999999")
	if err == nil {
		t.Error("expected lookup error for unknown gid")
	}
}

func TestScalarDiff_NoChange(t *testing.T) {
	t.Parallel()
	p := &params{Name: "alice", State: StatePresent}
	live := &UserInfo{Name: "alice", UID: 1500, GID: 1500, Home: "/home/alice", Shell: "/bin/sh"}
	opts, changed := scalarDiff(p, live)
	if changed {
		t.Errorf("no fields declared → changed should be false; got opts=%+v", opts)
	}
}

func TestScalarDiff_MultiField(t *testing.T) {
	t.Parallel()
	uid := 1600
	p := &params{
		Name: "alice", State: StatePresent,
		UID: &uid, Home: "/var/lib/alice", Shell: "/bin/zsh", Comment: "Alice (new)",
	}
	live := &UserInfo{Name: "alice", UID: 1500, GID: 1500, Home: "/home/alice", Shell: "/bin/sh", Comment: "Alice"}
	opts, changed := scalarDiff(p, live)
	if !changed {
		t.Fatal("expected changed=true")
	}
	if opts.UID == nil || *opts.UID != 1600 {
		t.Errorf("UID not propagated; got %+v", opts.UID)
	}
	if opts.Home != "/var/lib/alice" || opts.Shell != "/bin/zsh" || opts.Comment != "Alice (new)" {
		t.Errorf("scalar fields lost: %+v", opts)
	}
}

func TestScalarDiff_GroupByName(t *testing.T) {
	t.Parallel()
	// Live GID is 0 (root). Declared group name "root" should be a
	// no-change verdict (no Mod call). Note: this relies on group
	// 0 being named "root", which is universally true on
	// Linux/macOS/BSD.
	p := &params{Name: "alice", State: StatePresent, Group: "root"}
	live := &UserInfo{Name: "alice", UID: 1500, GID: 0}
	_, changed := scalarDiff(p, live)
	if changed {
		t.Error("declared group name matches live GID → should not change")
	}
}

func TestScalarDiff_GroupByNumericString(t *testing.T) {
	t.Parallel()
	p := &params{Name: "alice", State: StatePresent, Group: "1600"}
	live := &UserInfo{Name: "alice", UID: 1500, GID: 1500}
	opts, changed := scalarDiff(p, live)
	if !changed {
		t.Fatal("numeric-string Group should diff against live.GID")
	}
	if opts.GID == nil || *opts.GID != 1600 {
		t.Errorf("GID = %v, want 1600", opts.GID)
	}
}
