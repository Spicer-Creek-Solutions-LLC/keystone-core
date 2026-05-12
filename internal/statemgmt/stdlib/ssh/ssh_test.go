package ssh

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
		ID:     "ssh:" + name,
		Module: "ssh",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// withHome points homeDirFor at a tempdir and the current uid/gid
// (so the chown step is a no-op). Callers must NOT t.Parallel().
func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	old := homeDirFor
	homeDirFor = func(string) (string, int, int, error) { return home, os.Getuid(), os.Getgid(), nil }
	t.Cleanup(func() { homeDirFor = old })
	return home
}

func readAK(t *testing.T, home string) string {
	t.Helper()
	b, err := os.ReadFile(authorizedKeysPath(home))
	if err != nil {
		return ""
	}
	return string(b)
}

const (
	kt    = "ssh-ed25519"
	blobA = "AAAAC3NzaC1lZDI1NTE5AAAAITESTBLOBAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	blobB = "AAAAC3NzaC1lZDI1NTE5AAAAITESTBLOBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
)

func keyA(comment string) string {
	if comment == "" {
		return kt + " " + blobA
	}
	return kt + " " + blobA + " " + comment
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("lbl", StatePresent, map[string]any{"keys": "x", "user": "u"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParseKeyParam(t *testing.T) {
	t.Parallel()
	kt2, b, c, err := parseKeyParam("ssh-rsa AAAB3+/test== alice@laptop")
	if err != nil || kt2 != "ssh-rsa" || b != "AAAB3+/test==" || c != "alice@laptop" {
		t.Fatalf("parse: %q %q %q %v", kt2, b, c, err)
	}
	_, _, c, _ = parseKeyParam("ssh-ed25519 " + blobA)
	if c != "" {
		t.Errorf("no comment expected, got %q", c)
	}
	for _, bad := range []string{"ssh-rsa", "ssh rsa AAAA", "BAD/TYPE AAAA", "ssh-rsa !!!notbase64!!!", "ssh-rsa AAAA\nx"} {
		if _, _, _, err := parseKeyParam(bad); err == nil {
			t.Errorf("parseKeyParam(%q) should error", bad)
		}
	}
}

func TestParseOptions(t *testing.T) {
	t.Parallel()
	if o, err := parseOptions("no-pty,no-X11-forwarding"); err != nil || o != "no-pty,no-X11-forwarding" {
		t.Errorf("string: %q %v", o, err)
	}
	if o, err := parseOptions([]any{"no-pty", `command="/bin/foo"`}); err != nil || o != `no-pty,command="/bin/foo"` {
		t.Errorf("list: %q %v", o, err)
	}
	for _, bad := range []any{"", "no pty", []any{"a", ""}, []any{1}, "a\nb", 7} {
		if _, err := parseOptions(bad); err == nil {
			t.Errorf("parseOptions(%v) should error", bad)
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
		{"present ok", decl("l", StatePresent, map[string]any{"key": keyA("bot"), "user": "deploy"}), false},
		{"present ok options+comment", decl("l", StatePresent, map[string]any{"key": keyA(""), "user": "deploy", "options": "no-pty", "comment": "ci"}), false},
		{"present needs key", decl("l", StatePresent, map[string]any{"user": "deploy"}), true},
		{"needs user", decl("l", StatePresent, map[string]any{"key": keyA("")}), true},
		{"bad user", decl("l", StatePresent, map[string]any{"key": keyA(""), "user": "../etc"}), true},
		{"bad key", decl("l", StatePresent, map[string]any{"key": "ssh-rsa", "user": "deploy"}), true},
		{"absent ok", decl("l", StateAbsent, map[string]any{"key": keyA(""), "user": "deploy"}), false},
		{"absent rejects options", decl("l", StateAbsent, map[string]any{"key": keyA(""), "user": "deploy", "options": "no-pty"}), true},
		{"absent rejects comment", decl("l", StateAbsent, map[string]any{"key": keyA(""), "user": "deploy", "comment": "x"}), true},
		{"bad state", decl("l", "frob", map[string]any{"key": keyA(""), "user": "deploy"}), true},
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

// --- authkeys.go (pure) -----------------------------------------------

func TestAuthKeys_Parse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line string
		ok   bool
		k    authKey
	}{
		{kt + " " + blobA, true, authKey{KeyType: kt, Blob: blobA}},
		{kt + " " + blobA + " alice@host", true, authKey{KeyType: kt, Blob: blobA, Comment: "alice@host"}},
		{"no-pty " + kt + " " + blobA, true, authKey{Options: "no-pty", KeyType: kt, Blob: blobA}},
		{`command="/bin/x",no-pty ` + kt + " " + blobA + " bot", true, authKey{Options: `command="/bin/x",no-pty`, KeyType: kt, Blob: blobA, Comment: "bot"}},
		{"# comment", false, authKey{}},
		{"", false, authKey{}},
		{"   ", false, authKey{}},
		{"just some words here", false, authKey{}},
	}
	for _, c := range cases {
		got, ok := parseAuthLine(c.line)
		if ok != c.ok {
			t.Errorf("parseAuthLine(%q): ok=%v want %v", c.line, ok, c.ok)
			continue
		}
		if ok && got != c.k {
			t.Errorf("parseAuthLine(%q) = %+v, want %+v", c.line, got, c.k)
		}
	}
	// render round-trips
	k := authKey{Options: "no-pty", KeyType: kt, Blob: blobA, Comment: "bot"}
	if got, _ := parseAuthLine(k.render()); got != k {
		t.Errorf("render/parse round-trip: %+v != %+v", got, k)
	}
}

func TestAuthKeys_FindUpsertRemove(t *testing.T) {
	t.Parallel()
	base := "# my keys\n" + kt + " " + blobB + " other-key\n\n" + "no-pty " + kt + " " + blobA + " old-comment\n"
	// find keyed on key material (ignores options/comment)
	if k, ok := findLine(base, kt, blobA); !ok || k.Comment != "old-comment" || k.Options != "no-pty" {
		t.Fatalf("find: %+v ok=%v", k, ok)
	}
	if _, ok := findLine(base, kt, "AAAA"); ok {
		t.Error("a missing blob should not be found")
	}
	// upsert: same key material, new options/comment → replace in place
	want := authKey{KeyType: kt, Blob: blobA, Comment: "new-comment"}
	c1 := upsertLine(base, want)
	if k, _ := findLine(c1, kt, blobA); k.Comment != "new-comment" || k.Options != "" {
		t.Errorf("upsert replace: %+v", k)
	}
	if strings.Count(c1, blobA) != 1 {
		t.Errorf("upsert duplicated the entry: %s", c1)
	}
	if !strings.Contains(c1, blobB) || !strings.Contains(c1, "# my keys") {
		t.Errorf("upsert clobbered other content: %s", c1)
	}
	// upsert a brand-new key → appended
	c2 := upsertLine(c1, authKey{KeyType: kt, Blob: "AAAANEWKEY", Comment: "fresh"})
	if k, ok := findLine(c2, kt, "AAAANEWKEY"); !ok || k.Comment != "fresh" {
		t.Errorf("upsert add: %+v ok=%v", k, ok)
	}
	// remove
	c3 := removeLines(c2, kt, blobA)
	if _, ok := findLine(c3, kt, blobA); ok {
		t.Error("entry not removed")
	}
	if _, ok := findLine(c3, kt, blobB); !ok {
		t.Error("removal clobbered the other key")
	}
	if removeLines(c3, kt, "AAAANOPE") != c3 {
		t.Error("removing a missing entry should be a no-op")
	}
}

// --- Check / Apply -----------------------------------------------------

func TestCheckApply_Present(t *testing.T) {
	home := withHome(t)
	m := New()
	d := decl("bot-key", StatePresent, map[string]any{"key": keyA("build-bot"), "user": "deploy", "options": "no-pty"})

	// nothing yet → drift
	r, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches {
		t.Error("should drift")
	}

	// Apply: creates ~/.ssh (0700) + authorized_keys (0600) + the line
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("first apply should change")
	}
	if fi, err := os.Stat(filepath.Join(home, ".ssh")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("~/.ssh mode: %v %v", fi, err)
	}
	if fi, err := os.Stat(authorizedKeysPath(home)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("authorized_keys mode: %v %v", fi, err)
	}
	got := readAK(t, home)
	if !strings.Contains(got, "no-pty "+kt+" "+blobA+" build-bot") {
		t.Fatalf("authorized_keys line wrong: %q", got)
	}

	// converged
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("should match after apply, diff=%q", r.Diff)
	}
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed || sr.Comment != "already converged" {
		t.Errorf("second apply: changed=%v comment=%q", sr.Changed, sr.Comment)
	}

	// change the comment/options → rewrite in place
	d2 := decl("bot-key", StatePresent, map[string]any{"key": keyA(""), "user": "deploy", "comment": "renamed"})
	r, _ = m.Check(context.Background(), d2)
	if r.Matches {
		t.Error("options/comment change should drift")
	}
	sr, _ = m.Apply(context.Background(), d2)
	if !sr.Changed {
		t.Error("rewrite should change")
	}
	got = readAK(t, home)
	if !strings.Contains(got, kt+" "+blobA+" renamed") || strings.Contains(got, "no-pty") || strings.Contains(got, "build-bot") {
		t.Errorf("line not rewritten cleanly: %q", got)
	}
	if strings.Count(got, blobA) != 1 {
		t.Error("rewrite duplicated the entry")
	}
}

func TestApply_Present_CommentFromKey(t *testing.T) {
	home := withHome(t)
	// comment carried in the key string, no separate comment param
	if _, err := New().Apply(context.Background(), decl("k", StatePresent, map[string]any{"key": keyA("alice@laptop"), "user": "deploy"})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readAK(t, home), kt+" "+blobA+" alice@laptop") {
		t.Errorf("comment-from-key not used: %q", readAK(t, home))
	}
}

func TestApply_Present_PreservesOtherKeys(t *testing.T) {
	home := withHome(t)
	// seed the file with another key + a comment line
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	seed := "# operator's keys\n" + kt + " " + blobB + " other@host\n"
	if err := os.WriteFile(authorizedKeysPath(home), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New().Apply(context.Background(), decl("k", StatePresent, map[string]any{"key": keyA("new"), "user": "deploy"})); err != nil {
		t.Fatal(err)
	}
	got := readAK(t, home)
	if !strings.Contains(got, blobB) || !strings.Contains(got, "# operator's keys") {
		t.Errorf("other content lost: %q", got)
	}
	if !strings.Contains(got, kt+" "+blobA+" new") {
		t.Errorf("new key not appended: %q", got)
	}
}

func TestCheckApply_Absent(t *testing.T) {
	home := withHome(t)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// file has the target key twice (defensive) + another key
	seed := kt + " " + blobA + " one\nno-pty " + kt + " " + blobA + " two\n" + kt + " " + blobB + " keepme\n"
	if err := os.WriteFile(authorizedKeysPath(home), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	d := decl("k", StateAbsent, map[string]any{"key": keyA(""), "user": "deploy"})
	r, _ := New().Check(context.Background(), d)
	if r.Matches {
		t.Error("key present → drift from absent")
	}
	sr, err := New().Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("removal should change")
	}
	got := readAK(t, home)
	if strings.Contains(got, blobA) {
		t.Errorf("not all matching lines removed: %q", got)
	}
	if !strings.Contains(got, blobB) {
		t.Error("the other key was removed")
	}
	// already absent → no-op + match
	sr, _ = New().Apply(context.Background(), d)
	if sr.Changed {
		t.Error("absent on a missing key should be a no-op")
	}
	r, _ = New().Check(context.Background(), d)
	if !r.Matches {
		t.Error("absent should match once the key is gone")
	}
}

func TestCheckApply_Absent_NoFile(t *testing.T) {
	withHome(t) // no ~/.ssh at all
	d := decl("k", StateAbsent, map[string]any{"key": keyA(""), "user": "deploy"})
	r, err := New().Check(context.Background(), d)
	if err != nil || !r.Matches {
		t.Errorf("absent with no authorized_keys file should match: %v %v", r, err)
	}
	sr, _ := New().Apply(context.Background(), d)
	if sr.Changed {
		t.Error("absent apply with no file should be a no-op")
	}
}

func TestApply_UserLookupError(t *testing.T) {
	// not parallel: mutates the package-global homeDirFor.
	old := homeDirFor
	homeDirFor = func(string) (string, int, int, error) { return "", 0, 0, errors.New("no such user") }
	t.Cleanup(func() { homeDirFor = old })
	if _, err := New().Check(context.Background(), decl("k", StatePresent, map[string]any{"key": keyA(""), "user": "ghost"})); err == nil {
		t.Error("a user-lookup error should propagate from Check")
	}
	if _, err := New().Apply(context.Background(), decl("k", StatePresent, map[string]any{"key": keyA(""), "user": "ghost"})); err == nil {
		t.Error("a user-lookup error should propagate from Apply")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "ssh" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("ssh should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("k", StatePresent, map[string]any{"key": keyA(""), "user": "u"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("present drift → HIGH")
	}
	if dsm.DriftSeverity(decl("k", StateAbsent, map[string]any{"key": keyA(""), "user": "u"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("k", StatePresent, map[string]any{"key": keyA(""), "user": "deploy"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("k", StatePresent, map[string]any{"user": "deploy"})); err == nil {
		t.Error("present without key should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	home := withHome(t)
	m := New()
	d := decl("k", StatePresent, map[string]any{"key": keyA(""), "user": "deploy"})
	if ok, err := m.Test(context.Background(), d); err != nil || ok {
		t.Errorf("Test before apply should be false: ok=%v err=%v", ok, err)
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.Test(context.Background(), d); err != nil || !ok {
		t.Errorf("Test after apply should be true: ok=%v err=%v", ok, err)
	}
	_ = home
}
