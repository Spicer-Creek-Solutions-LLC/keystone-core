// SPDX-License-Identifier: Apache-2.0

package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// declFor returns a Declaration for path/state/params. ID is
// recomputed to match Module:Name, matching what the engine does at
// resolver time.
func declFor(path, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "file:" + path,
		Module: "file",
		Name:   path,
		State:  state,
		Params: params,
	}
}

func newModule() *Module { return &Module{} }

// ---- parseParams / validate ----------------------------------------

func TestParseParams_RejectsUnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("/x", StatePresent, map[string]any{
		"contnet": "oops", // typo
	}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Fatalf("err = %v, want unknown-param error", err)
	}
}

func TestParseParams_ModeFormats(t *testing.T) {
	t.Parallel()
	cases := map[string]uint32{
		"0644": 0o644,
		"644":  0o644,
		"0755": 0o755,
		"4755": 0o4755, // setuid + 0755
	}
	for in, want := range cases {
		p, err := parseParams(declFor("/x", StatePresent, map[string]any{"mode": in}))
		if err != nil {
			t.Errorf("mode %q: %v", in, err)
			continue
		}
		if p.Mode != want {
			t.Errorf("mode %q = %#o, want %#o", in, p.Mode, want)
		}
	}
}

func TestParseParams_ModeInvalid(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"abc", "999", "1000000", ""} {
		_, err := parseParams(declFor("/x", StatePresent, map[string]any{"mode": bad}))
		if err == nil {
			t.Errorf("mode %q should be rejected", bad)
		}
	}
}

func TestParseParams_ContentSourceMutex(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("/x", StatePresent, map[string]any{
		"content": "hello",
		"source":  "/tmp/x",
	}))
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected content/source mutex error, got %v", err)
	}
}

func TestParseParams_AbsentRejectsAttributes(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("/x", StateAbsent, map[string]any{
		"mode": "0644",
	}))
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "absent") {
		t.Errorf("expected absent-leaked-attrs error, got %v", err)
	}
}

func TestParseParams_SymlinkRequiresTarget(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("/x", StateSymlink, map[string]any{}))
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "target") {
		t.Errorf("expected symlink-target-required error, got %v", err)
	}
}

func TestParseParams_DirectoryRejectsContent(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("/x", StateDirectory, map[string]any{"content": "no"}))
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if err := p.validate(); err == nil {
		t.Error("expected directory + content error")
	}
}

func TestParseParams_NonStringValues(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("/x", StatePresent, map[string]any{"mode": 644}))
	if err == nil || !strings.Contains(err.Error(), "octal string") {
		t.Errorf("expected non-string mode error, got %v", err)
	}
}

// ---- Module surface -----------------------------------------------

func TestModule_NameAndValidStates(t *testing.T) {
	t.Parallel()
	m := newModule()
	if m.Name() != "file" {
		t.Errorf("Name = %q, want file", m.Name())
	}
	want := []string{StatePresent, StateAbsent, StateDirectory, StateSymlink}
	got := m.ValidStates()
	if len(got) != len(want) {
		t.Fatalf("ValidStates len = %d, want %d", len(got), len(want))
	}
}

func TestModule_ImplementsValidatableAndDriftSeverity(t *testing.T) {
	t.Parallel()
	var _ statemgmt.ValidatableModule = newModule()
	var _ statemgmt.DriftSeverityModule = newModule()
}

func TestModule_DriftSeverity_AbsentHigh(t *testing.T) {
	t.Parallel()
	m := newModule()
	if got := m.DriftSeverity(declFor("/x", StateAbsent, nil), nil); got != statemgmt.DriftSeverityHigh {
		t.Errorf("absent drift severity = %v, want high", got)
	}
}

func TestModule_DriftSeverity_PresentMedium(t *testing.T) {
	t.Parallel()
	m := newModule()
	if got := m.DriftSeverity(declFor("/x", StatePresent, nil), nil); got != statemgmt.DriftSeverityMedium {
		t.Errorf("present drift severity = %v, want medium", got)
	}
}

// ---- present -------------------------------------------------------

func TestModule_Present_CreatesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "hello")
	m := newModule()
	decl := declFor(path, StatePresent, map[string]any{
		"content": "hello world\n",
		"mode":    "0644",
	})

	check, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if check.Matches {
		t.Error("Check should report missing file as drift")
	}

	res, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Error("first Apply should report Changed=true")
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "hello world\n" {
		t.Errorf("content = %q, want hello world", body)
	}

	ok, err := m.Test(context.Background(), decl)
	if err != nil || !ok {
		t.Errorf("Test = %v err=%v, want true/nil", ok, err)
	}
}

func TestModule_Present_Idempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "idem")
	m := newModule()
	decl := declFor(path, StatePresent, map[string]any{
		"content": "x",
		"mode":    "0644",
	})

	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	res, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if res.Changed {
		t.Error("second Apply should report Changed=false (idempotent)")
	}
	if res.Comment != "already converged" {
		t.Errorf("Comment = %q, want \"already converged\"", res.Comment)
	}
}

func TestModule_Present_ContentChange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "update")
	m := newModule()

	first := declFor(path, StatePresent, map[string]any{"content": "v1"})
	if _, err := m.Apply(context.Background(), first); err != nil {
		t.Fatalf("first: %v", err)
	}

	second := declFor(path, StatePresent, map[string]any{"content": "v2"})
	check, err := m.Check(context.Background(), second)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if check.Matches {
		t.Error("changed content should report drift")
	}
	if !strings.Contains(check.Diff, "content sha") {
		t.Errorf("Diff should mention content sha; got %q", check.Diff)
	}

	if _, err := m.Apply(context.Background(), second); err != nil {
		t.Fatalf("apply v2: %v", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "v2" {
		t.Errorf("content after update = %q, want v2", body)
	}
}

func TestModule_Present_ModeChange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "modechange")
	m := newModule()

	first := declFor(path, StatePresent, map[string]any{
		"content": "x",
		"mode":    "0600",
	})
	if _, err := m.Apply(context.Background(), first); err != nil {
		t.Fatalf("first: %v", err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("initial mode = %#o, want 0600", st.Mode().Perm())
	}

	second := declFor(path, StatePresent, map[string]any{
		"content": "x",
		"mode":    "0644",
	})
	check, _ := m.Check(context.Background(), second)
	if check.Matches {
		t.Error("mode change should report drift")
	}
	if !strings.Contains(check.Diff, "mode") {
		t.Errorf("Diff should mention mode; got %q", check.Diff)
	}

	res, err := m.Apply(context.Background(), second)
	if err != nil {
		t.Fatalf("apply mode: %v", err)
	}
	if !res.Changed {
		t.Error("mode change should report Changed=true")
	}
	st, _ = os.Stat(path)
	if st.Mode().Perm() != 0o644 {
		t.Errorf("mode after change = %#o, want 0644", st.Mode().Perm())
	}
}

func TestModule_Present_Source(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("from source"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	path := filepath.Join(dir, "dest")
	m := newModule()
	decl := declFor(path, StatePresent, map[string]any{"source": source})

	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "from source" {
		t.Errorf("content = %q, want from source", body)
	}
}

func TestModule_Present_SourceMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "dest")
	m := newModule()
	decl := declFor(path, StatePresent, map[string]any{"source": "/no/such/source"})
	_, err := m.Apply(context.Background(), decl)
	if err == nil || !strings.Contains(err.Error(), "read source") {
		t.Errorf("expected source-read error, got %v", err)
	}
}

func TestModule_Present_CollidesWithDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	collidingPath := filepath.Join(dir, "asdir")
	if err := os.Mkdir(collidingPath, 0o755); err != nil {
		t.Fatalf("setup mkdir: %v", err)
	}
	m := newModule()
	decl := declFor(collidingPath, StatePresent, map[string]any{"content": "x"})
	_, err := m.Apply(context.Background(), decl)
	if err == nil || !strings.Contains(err.Error(), "exists as directory") {
		t.Errorf("expected collision error, got %v", err)
	}
}

// ---- absent --------------------------------------------------------

func TestModule_Absent_RemovesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "doomed")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := newModule()
	decl := declFor(path, StateAbsent, nil)

	check, _ := m.Check(context.Background(), decl)
	if check.Matches {
		t.Error("existing file should be drift for state=absent")
	}

	res, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Error("first absent Apply should be Changed=true")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file should be gone; stat err = %v", err)
	}
}

func TestModule_Absent_Idempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "never-existed")
	m := newModule()
	decl := declFor(path, StateAbsent, nil)
	res, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply on missing: %v", err)
	}
	if res.Changed {
		t.Error("absent + missing should be Changed=false")
	}
}

func TestModule_Absent_RefusesDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "asdir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	m := newModule()
	decl := declFor(target, StateAbsent, nil)
	_, err := m.Apply(context.Background(), decl)
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Errorf("absent of a directory should refuse; err = %v", err)
	}
}

// ---- directory ----------------------------------------------------

func TestModule_Directory_Creates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "subdir", "nested")
	m := newModule()
	decl := declFor(target, StateDirectory, map[string]any{"mode": "0750"})

	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !st.IsDir() {
		t.Error("not a directory")
	}
	if st.Mode().Perm() != 0o750 {
		t.Errorf("mode = %#o, want 0750", st.Mode().Perm())
	}
	// Idempotent.
	res, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("re-Apply: %v", err)
	}
	if res.Changed {
		t.Error("re-Apply on matching dir should be Changed=false")
	}
}

func TestModule_Directory_CollidesWithFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	collidingPath := filepath.Join(dir, "asfile")
	if err := os.WriteFile(collidingPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	m := newModule()
	decl := declFor(collidingPath, StateDirectory, nil)
	_, err := m.Apply(context.Background(), decl)
	if err == nil || !strings.Contains(err.Error(), "exists as regular") {
		t.Errorf("expected collision error; got %v", err)
	}
}

// ---- symlink ------------------------------------------------------

func TestModule_Symlink_Creates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("real"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	link := filepath.Join(dir, "link")
	m := newModule()
	decl := declFor(link, StateSymlink, map[string]any{"target": target})

	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if got != target {
		t.Errorf("readlink = %q, want %q", got, target)
	}
	// Idempotent.
	res, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("re-Apply: %v", err)
	}
	if res.Changed {
		t.Error("re-Apply on matching symlink should be Changed=false")
	}
}

func TestModule_Symlink_ReplacesWrongTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	oldTarget := filepath.Join(dir, "old-target")
	newTarget := filepath.Join(dir, "new-target")
	for _, p := range []string{oldTarget, newTarget} {
		if err := os.WriteFile(p, []byte(p), 0o644); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(oldTarget, link); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	m := newModule()
	decl := declFor(link, StateSymlink, map[string]any{"target": newTarget})

	check, _ := m.Check(context.Background(), decl)
	if check.Matches {
		t.Error("wrong target should be drift")
	}
	if !strings.Contains(check.Diff, "symlink target") {
		t.Errorf("Diff should mention target; got %q", check.Diff)
	}

	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, _ := os.Readlink(link)
	if got != newTarget {
		t.Errorf("readlink = %q, want %q", got, newTarget)
	}
}

func TestModule_Symlink_CollidesWithFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "regular")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := newModule()
	decl := declFor(path, StateSymlink, map[string]any{"target": "/anywhere"})
	_, err := m.Apply(context.Background(), decl)
	if err == nil || !strings.Contains(err.Error(), "exists as regular") {
		t.Errorf("expected collision; got %v", err)
	}
}

// ---- Validate ------------------------------------------------------

func TestModule_Validate_PassesGoodDeclaration(t *testing.T) {
	t.Parallel()
	m := newModule()
	if err := m.Validate(declFor("/etc/hosts", StatePresent, map[string]any{
		"content": "x", "mode": "0644",
	})); err != nil {
		t.Errorf("Validate should pass; got %v", err)
	}
}

func TestModule_Validate_RejectsContentAndSource(t *testing.T) {
	t.Parallel()
	m := newModule()
	err := m.Validate(declFor("/etc/hosts", StatePresent, map[string]any{
		"content": "x", "source": "/y",
	}))
	if err == nil {
		t.Error("Validate should reject content+source")
	}
}

// ---- Owner / Group (root-only) ------------------------------------

func TestModule_Present_OwnerChange_RequiresRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root for chown")
	}
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "owned")
	m := newModule()
	decl := declFor(path, StatePresent, map[string]any{
		"content": "x",
		"owner":   "nobody",
	})
	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Idempotent re-Check.
	check, _ := m.Check(context.Background(), decl)
	if !check.Matches {
		t.Errorf("re-Check should match; diff %q", check.Diff)
	}
}

func TestModule_Present_OwnerNumericString(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root for chown")
	}
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "numeric")
	m := newModule()
	decl := declFor(path, StatePresent, map[string]any{
		"content": "x",
		"owner":   "65534", // nobody on most distros
	})
	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

// ---- stdlib RegisterAll smoke ------------------------------------

func TestNew_ReturnsModule(t *testing.T) {
	t.Parallel()
	m := New()
	if m == nil {
		t.Fatal("New returned nil")
	}
	if m.Name() != "file" {
		t.Errorf("New().Name() = %q, want file", m.Name())
	}
}

func TestModule_Present_ModeOnly_NoContentChange(t *testing.T) {
	t.Parallel()
	// Pre-seed a file with content, then declare present without
	// content/source but with a different mode. Apply should chmod
	// without rewriting the file (content stays).
	dir := t.TempDir()
	path := filepath.Join(dir, "preserved")
	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := newModule()
	decl := declFor(path, StatePresent, map[string]any{"mode": "0644"})
	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "keep me" {
		t.Errorf("content lost: %q", body)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o644 {
		t.Errorf("mode = %#o, want 0644", st.Mode().Perm())
	}
}

func TestApplyOwnership_NonRootGetsClearError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test asserts non-root failure mode")
	}
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "owned")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := newModule()
	decl := declFor(path, StatePresent, map[string]any{
		"content": "x",
		"owner":   "root", // can't chown to root as non-root
	})
	_, err := m.Apply(context.Background(), decl)
	if err == nil || !strings.Contains(err.Error(), "chown") {
		t.Errorf("expected chown error; got %v", err)
	}
}

func TestApplyOwnership_UnknownUserSurfacesName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "owned")
	m := newModule()
	decl := declFor(path, StatePresent, map[string]any{
		"content": "x",
		"owner":   "no-such-user-name-zzz",
	})
	_, err := m.Apply(context.Background(), decl)
	if err == nil || !strings.Contains(err.Error(), "no-such-user-name-zzz") {
		t.Errorf("expected user-lookup error citing name; got %v", err)
	}
}

func TestApplyOwnership_UnknownGroupSurfacesName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "owned")
	m := newModule()
	decl := declFor(path, StatePresent, map[string]any{
		"content": "x",
		"group":   "no-such-group-name-zzz",
	})
	_, err := m.Apply(context.Background(), decl)
	if err == nil || !strings.Contains(err.Error(), "no-such-group-name-zzz") {
		t.Errorf("expected group-lookup error citing name; got %v", err)
	}
}

func TestDiffCheck_PresentWithoutContent_OnlyComparesAttributes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "noattr")
	if err := os.WriteFile(path, []byte("anything"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := newModule()
	// No content, no source, mode matches → Check matches.
	decl := declFor(path, StatePresent, map[string]any{"mode": "0644"})
	check, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !check.Matches {
		t.Errorf("Check should match (no content/source declared); diff = %q", check.Diff)
	}
}

func TestShort_TruncatesLongHashes(t *testing.T) {
	t.Parallel()
	long := "0123456789abcdef0123"
	if got := short(long); got != "01234567" {
		t.Errorf("short = %q, want 01234567", got)
	}
	if got := short("abc"); got != "abc" {
		t.Errorf("short short-input = %q, want abc", got)
	}
}

func TestFileType_StringFormatting(t *testing.T) {
	t.Parallel()
	cases := map[fileType]string{
		typeMissing:   "missing",
		typeRegular:   "regular",
		typeDirectory: "directory",
		typeSymlink:   "symlink",
		typeOther:     "other",
	}
	for ft, want := range cases {
		if got := ft.String(); got != want {
			t.Errorf("fileType(%d) = %q, want %q", ft, got, want)
		}
	}
}

func TestModule_HashFunctionsStable(t *testing.T) {
	t.Parallel()
	// Hash of empty content must be stable + non-empty.
	h := hashBytes(nil)
	if h == "" || len(h) != 64 {
		t.Errorf("hashBytes(nil) = %q, want 64-char hex", h)
	}
	// Hash determinism: same input bytes → same output across
	// two separate slices (staticcheck flags literal-vs-literal
	// comparisons, so allocate distinctly).
	a := hashBytes([]byte{'a'})
	b := hashBytes([]byte{'a'})
	if a != b {
		t.Errorf("hashBytes not deterministic: %s vs %s", a, b)
	}
}
