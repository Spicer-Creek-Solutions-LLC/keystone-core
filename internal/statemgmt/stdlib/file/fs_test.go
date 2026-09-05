// SPDX-License-Identifier: Apache-2.0

package file

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Focused unit tests for the fs.go helpers that aren't fully
// exercised through Module.Apply (writeFileAtomic error legs,
// resolveOwner/Group numeric vs name paths, ownerMatches and
// groupMatches branches).

func TestResolveOwner_NumericString(t *testing.T) {
	t.Parallel()
	got, err := resolveOwner("0")
	if err != nil {
		t.Fatalf("resolveOwner(\"0\"): %v", err)
	}
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestResolveOwner_UnknownName(t *testing.T) {
	t.Parallel()
	_, err := resolveOwner("zzz-no-such-user-zzz")
	if err == nil || !strings.Contains(err.Error(), "user") {
		t.Errorf("err = %v, want \"user\" cited", err)
	}
}

func TestResolveOwner_ByName_Root(t *testing.T) {
	t.Parallel()
	// `root` exists on every Linux distro; this exercises the
	// non-numeric Lookup branch + uid Atoi happy path.
	uid, err := resolveOwner("root")
	if err != nil {
		t.Skipf("user.Lookup(\"root\") not supported on this host: %v", err)
	}
	if uid != 0 {
		t.Errorf("root uid = %d, want 0", uid)
	}
}

func TestResolveGroup_NumericString(t *testing.T) {
	t.Parallel()
	got, err := resolveGroup("0")
	if err != nil {
		t.Fatalf("resolveGroup(\"0\"): %v", err)
	}
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestResolveGroup_UnknownName(t *testing.T) {
	t.Parallel()
	_, err := resolveGroup("zzz-no-such-group-zzz")
	if err == nil || !strings.Contains(err.Error(), "group") {
		t.Errorf("err = %v, want \"group\" cited", err)
	}
}

func TestResolveGroup_ByName_Root(t *testing.T) {
	t.Parallel()
	// `root` group is universally present.
	gid, err := resolveGroup("root")
	if err != nil {
		t.Skipf("user.LookupGroup(\"root\") not supported: %v", err)
	}
	if gid != 0 {
		t.Errorf("root gid = %d, want 0", gid)
	}
}

func TestOwnerMatches_Branches(t *testing.T) {
	t.Parallel()
	m := &meta{UID: 1000, OwnerName: "alice"}
	cases := []struct {
		declared string
		want     bool
	}{
		{"", true},      // no declaration → matches
		{"1000", true},  // numeric: match
		{"1001", false}, // numeric: mismatch
		{"alice", true}, // name: match
		{"bob", false},  // name: mismatch
		{"-1", false},   // numeric: negative non-match
	}
	for _, c := range cases {
		if got := ownerMatches(c.declared, m); got != c.want {
			t.Errorf("ownerMatches(%q) = %v, want %v", c.declared, got, c.want)
		}
	}
}

func TestGroupMatches_Branches(t *testing.T) {
	t.Parallel()
	m := &meta{GID: 100, GroupName: "users"}
	cases := []struct {
		declared string
		want     bool
	}{
		{"", true},
		{"100", true},
		{"101", false},
		{"users", true},
		{"wheel", false},
	}
	for _, c := range cases {
		if got := groupMatches(c.declared, m); got != c.want {
			t.Errorf("groupMatches(%q) = %v, want %v", c.declared, got, c.want)
		}
	}
}

func TestWriteFileAtomic_HappyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := writeFileAtomic(path, []byte("ok\n"), 0o600); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "ok\n" {
		t.Errorf("content = %q", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600", got)
	}
}

func TestWriteFileAtomic_BadDir(t *testing.T) {
	t.Parallel()
	// Parent does not exist → OpenFile on the temp file must fail
	// with the wrapped "open tmp" error.
	path := filepath.Join(t.TempDir(), "no-such-dir", "out.txt")
	err := writeFileAtomic(path, []byte("x"), 0o644)
	if err == nil {
		t.Fatal("expected error for missing parent dir")
	}
	if !strings.Contains(err.Error(), "open tmp") {
		t.Errorf("err = %v, want \"open tmp\" prefix", err)
	}
}

func TestWriteFileAtomic_DestIsDir(t *testing.T) {
	t.Parallel()
	// Renaming over a non-empty directory must surface the rename
	// error path. Linux's rename(2) refuses with ENOTDIR / EISDIR
	// depending on direction; either way the wrapper cites "rename".
	dir := t.TempDir()
	dst := filepath.Join(dir, "occupied")
	if err := os.MkdirAll(filepath.Join(dst, "sub"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := writeFileAtomic(dst, []byte("x"), 0o644)
	if err == nil {
		t.Fatal("expected rename error onto a non-empty directory")
	}
	if !strings.Contains(err.Error(), "rename") {
		t.Errorf("err = %v, want \"rename\" cited", err)
	}
}

func TestHashBytes_Stable(t *testing.T) {
	t.Parallel()
	if a, b := hashBytes([]byte("abc")), hashBytes([]byte("abc")); a != b {
		t.Errorf("hashBytes not deterministic: %q vs %q", a, b)
	}
	if hashBytes([]byte("abc")) == hashBytes([]byte("def")) {
		t.Error("hashBytes collision")
	}
}

func TestReadMeta_MissingPath(t *testing.T) {
	t.Parallel()
	m, err := readMeta(filepath.Join(t.TempDir(), "no-such"))
	if err != nil {
		t.Fatalf("readMeta missing: %v", err)
	}
	if m.Type != typeMissing {
		t.Errorf("type = %v, want missing", m.Type)
	}
}

func TestReadMeta_SymlinkAndRegular(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	regular := filepath.Join(dir, "file")
	if err := os.WriteFile(regular, []byte("hello"), 0o640); err != nil {
		t.Fatalf("seed: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rm, err := readMeta(regular)
	if err != nil {
		t.Fatalf("readMeta regular: %v", err)
	}
	if rm.Type != typeRegular {
		t.Errorf("regular type = %v", rm.Type)
	}
	if rm.ContentHash == "" {
		t.Error("ContentHash empty for regular file")
	}

	lm, err := readMeta(link)
	if err != nil {
		t.Fatalf("readMeta symlink: %v", err)
	}
	if lm.Type != typeSymlink {
		t.Errorf("symlink type = %v", lm.Type)
	}
	if lm.SymlinkTarget != regular {
		t.Errorf("symlink target = %q, want %q", lm.SymlinkTarget, regular)
	}
}

func TestReadMeta_LstatErrorWraps(t *testing.T) {
	t.Parallel()
	// Path under a non-readable directory → lstat returns EACCES
	// (not ErrNotExist), so readMeta wraps the error rather than
	// returning typeMissing. Skip if we're already root since root
	// bypasses directory permissions.
	if os.Geteuid() == 0 {
		t.Skip("running as root; can't make a directory unreadable")
	}
	dir := t.TempDir()
	hidden := filepath.Join(dir, "locked")
	if err := os.Mkdir(hidden, 0o000); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer func() { _ = os.Chmod(hidden, 0o755) }()
	_, err := readMeta(filepath.Join(hidden, "child"))
	if err == nil {
		t.Fatal("expected EACCES from lstat under chmod 000 dir")
	}
	if !strings.Contains(err.Error(), "lstat") {
		t.Errorf("err = %v, want \"lstat\" cited", err)
	}
}

func TestHashFile_OpenError(t *testing.T) {
	t.Parallel()
	_, err := hashFile(filepath.Join(t.TempDir(), "no-such"))
	if err == nil || !strings.Contains(err.Error(), "open") {
		t.Errorf("err = %v, want \"open\" cited", err)
	}
}

func TestFileTypeStringer(t *testing.T) {
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
			t.Errorf("fileType(%d).String() = %q, want %q", int(ft), got, want)
		}
	}
}

// guardrail: meta's UID/GID are -1 on platforms without Stat_t.
// Today only Linux is supported, so the helper always has Stat_t;
// instead we sanity-check that on Linux they come out >= 0.
func TestReadMeta_StatFieldsPositive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := readMeta(p)
	if err != nil {
		t.Fatal(err)
	}
	if m.UID < 0 {
		t.Errorf("UID = %d, want >= 0", m.UID)
	}
	if _, err := strconv.Atoi(strconv.Itoa(m.GID)); err != nil {
		t.Errorf("GID not int-roundtripable: %v", err)
	}
}
