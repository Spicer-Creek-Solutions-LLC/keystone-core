// SPDX-License-Identifier: Apache-2.0

package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func timePlusHour() time.Time { return time.Now().Add(time.Hour) }

// sampleBz2 is a tar.bz2 built offline (compress/bzip2 is
// decompress-only in the stdlib) with the same two entries as the
// in-memory archives below: payload/hello.txt = "hello\n",
// payload/sub/world.txt = "world\n".
//
//go:embed testdata/sample.tar.bz2
var sampleBz2 []byte

// canonicalEntries is the content every test archive carries.
var canonicalEntries = map[string]string{
	"payload/hello.txt":     "hello\n",
	"payload/sub/world.txt": "world\n",
}

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "archive:" + name,
		Module: "archive",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- archive builders -------------------------------------------------

type tarEntry struct {
	name     string
	content  string // for regular files
	mode     int64
	typeflag byte
	linkname string // for symlinks
}

func writeTarBytes(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		tf := e.typeflag
		if tf == 0 {
			tf = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			if tf == tar.TypeDir {
				mode = 0o755
			} else {
				mode = 0o644
			}
		}
		hdr := &tar.Header{Name: e.name, Mode: mode, Typeflag: tf, Linkname: e.linkname}
		if tf == tar.TypeReg {
			hdr.Size = int64(len(e.content))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header %q: %v", e.name, err)
		}
		if tf == tar.TypeReg {
			if _, err := tw.Write([]byte(e.content)); err != nil {
				t.Fatalf("tar body %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func canonicalTarEntries() []tarEntry {
	keys := make([]string, 0, len(canonicalEntries))
	for k := range canonicalEntries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]tarEntry, 0, len(keys))
	for _, k := range keys {
		out = append(out, tarEntry{name: k, content: canonicalEntries[k]})
	}
	return out
}

func writeFile(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeZipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w, err := zw.Create(k)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entries[k])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// archiveFor writes a canonical archive of the given format into dir
// and returns its path.
func archiveFor(t *testing.T, dir, format string) string {
	t.Helper()
	switch format {
	case FormatTar:
		return writeFile(t, filepath.Join(dir, "a.tar"), writeTarBytes(t, canonicalTarEntries()))
	case FormatTarGz:
		return writeFile(t, filepath.Join(dir, "a.tar.gz"), gzipBytes(t, writeTarBytes(t, canonicalTarEntries())))
	case FormatTarBz2:
		return writeFile(t, filepath.Join(dir, "a.tar.bz2"), sampleBz2)
	case FormatZip:
		return writeFile(t, filepath.Join(dir, "a.zip"), writeZipBytes(t, canonicalEntries))
	default:
		t.Fatalf("unknown format %q", format)
		return ""
	}
}

func assertCanonical(t *testing.T, target string) {
	t.Helper()
	for name, want := range canonicalEntries {
		full := filepath.Join(target, filepath.FromSlash(name))
		got, err := os.ReadFile(full)
		if err != nil {
			t.Errorf("expected %s: %v", full, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", full, got, want)
		}
	}
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("/a.tar", StatePresent, map[string]any{"targets": "/x"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_FormatAliases(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"tgz": FormatTarGz, "TAR.GZ": FormatTarGz, "tbz2": FormatTarBz2, "ZIP": FormatZip, "auto": FormatAuto,
	} {
		p, err := parseParams(decl("/a", StatePresent, map[string]any{"target": "/t", "format": in}))
		if err != nil {
			t.Errorf("format %q: %v", in, err)
			continue
		}
		if p.Format != want {
			t.Errorf("format %q → %q, want %q", in, p.Format, want)
		}
	}
	if _, err := parseParams(decl("/a", StatePresent, map[string]any{"target": "/t", "format": "rar"})); err == nil {
		t.Error("unknown format should be rejected")
	}
}

func TestParse_StripComponents(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		raw any
		ok  bool
		n   int
	}{{2, true, 2}, {int64(0), true, 0}, {float64(1), true, 1}, {"3", true, 3}, {-1, false, 0}, {float64(1.5), false, 0}, {"x", false, 0}, {true, false, 0}} {
		p, err := parseParams(decl("/a", StatePresent, map[string]any{"target": "/t", "strip_components": c.raw}))
		if c.ok {
			if err != nil || p.StripComponents != c.n {
				t.Errorf("strip %v: err=%v n=%v", c.raw, err, p)
			}
		} else if err == nil {
			t.Errorf("strip %v: expected error", c.raw)
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
		{"ok", decl("/a.tar", StatePresent, map[string]any{"target": "/t"}), false},
		{"needs target", decl("/a.tar", StatePresent, nil), true},
		{"empty target", decl("/a.tar", StatePresent, map[string]any{"target": "  "}), true},
		{"absent not supported", decl("/a.tar", "absent", map[string]any{"target": "/t"}), true},
		{"weird state", decl("/a.tar", "frob", map[string]any{"target": "/t"}), true},
		{"multiline creates", decl("/a.tar", StatePresent, map[string]any{"target": "/t", "creates": "a\nb"}), true},
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

// --- format detection + path safety -----------------------------------

func TestDetectFormat(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"/x/y.tar.gz": FormatTarGz, "/x.tgz": FormatTarGz,
		"/x.tar.bz2": FormatTarBz2, "/x.tbz2": FormatTarBz2, "/x.tbz": FormatTarBz2,
		"/x.tar": FormatTar, "/x.zip": FormatZip,
	} {
		got, err := detectFormat(in, FormatAuto)
		if err != nil || got != want {
			t.Errorf("detectFormat(%q) = %q,%v want %q", in, got, err, want)
		}
	}
	if _, err := detectFormat("/x.weird", FormatAuto); err == nil {
		t.Error("undetectable extension should error")
	}
	// declared format overrides
	if got, _ := detectFormat("/x.weird", FormatZip); got != FormatZip {
		t.Errorf("declared format ignored: %q", got)
	}
}

func TestSanitizeEntryPath(t *testing.T) {
	t.Parallel()
	target := "/tmp/t"
	cases := []struct {
		raw     string
		strip   int
		want    string
		skip    bool
		wantErr bool
	}{
		{"a/b.txt", 0, filepath.Join(target, "a", "b.txt"), false, false},
		{"/a/b.txt", 0, filepath.Join(target, "a", "b.txt"), false, false}, // leading slash stripped
		{"./a/./b.txt", 0, filepath.Join(target, "a", "b.txt"), false, false},
		{"", 0, "", true, false},
		{"./", 0, "", true, false},
		{"top/a/b.txt", 1, filepath.Join(target, "a", "b.txt"), false, false},
		{"top/a.txt", 1, filepath.Join(target, "a.txt"), false, false},
		{"top", 1, "", true, false}, // single segment, fully stripped
		{"a/b", 5, "", true, false}, // fewer segments than strip
		{"../escape.txt", 0, "", false, true},
		{"a/../../escape", 0, "", false, true},
	}
	for _, c := range cases {
		got, skip, err := sanitizeEntryPath(target, c.raw, c.strip)
		if (err != nil) != c.wantErr {
			t.Errorf("%q strip=%d: err=%v wantErr=%v", c.raw, c.strip, err, c.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if skip != c.skip {
			t.Errorf("%q: skip=%v want %v", c.raw, skip, c.skip)
			continue
		}
		if !skip && got != c.want {
			t.Errorf("%q: got %q want %q", c.raw, got, c.want)
		}
	}
}

// --- Check / Apply: all formats ---------------------------------------

func TestExtract_AllFormats(t *testing.T) {
	for _, format := range []string{FormatTar, FormatTarGz, FormatTarBz2, FormatZip} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			archivePath := archiveFor(t, dir, format)
			target := filepath.Join(dir, "out")
			m := New()
			d := decl(archivePath, StatePresent, map[string]any{"target": target, "format": format})

			r, err := m.Check(context.Background(), d)
			if err != nil {
				t.Fatal(err)
			}
			if r.Matches {
				t.Error("nothing extracted yet → should drift")
			}
			sr, err := m.Apply(context.Background(), d)
			if err != nil {
				t.Fatal(err)
			}
			if !sr.Changed {
				t.Error("first apply should change")
			}
			assertCanonical(t, target)

			// idempotent — sentinel matches
			r, _ = m.Check(context.Background(), d)
			if !r.Matches {
				t.Errorf("should match after extract, diff=%q", r.Diff)
			}
			sr, _ = m.Apply(context.Background(), d)
			if sr.Changed || sr.Comment != "already converged" {
				t.Errorf("second apply: changed=%v comment=%q", sr.Changed, sr.Comment)
			}
		})
	}
}

func TestExtract_FormatAuto(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	archivePath := writeFile(t, filepath.Join(dir, "release.tar.gz"), gzipBytes(t, writeTarBytes(t, canonicalTarEntries())))
	target := filepath.Join(dir, "out")
	if _, err := New().Apply(context.Background(), decl(archivePath, StatePresent, map[string]any{"target": target})); err != nil {
		t.Fatal(err)
	}
	assertCanonical(t, target)
}

func TestExtract_StripComponents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	archivePath := archiveFor(t, dir, FormatTar)
	target := filepath.Join(dir, "out")
	if _, err := New().Apply(context.Background(), decl(archivePath, StatePresent, map[string]any{"target": target, "strip_components": 1})); err != nil {
		t.Fatal(err)
	}
	// "payload/hello.txt" → "hello.txt"; "payload/sub/world.txt" → "sub/world.txt"
	if b, err := os.ReadFile(filepath.Join(target, "hello.txt")); err != nil || string(b) != "hello\n" {
		t.Errorf("hello.txt after strip: %q %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(target, "sub", "world.txt")); err != nil || string(b) != "world\n" {
		t.Errorf("sub/world.txt after strip: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(target, "payload")); !os.IsNotExist(err) {
		t.Error("the 'payload' prefix should have been stripped")
	}
}

func TestExtract_CreatesShortCircuit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	archivePath := archiveFor(t, dir, FormatTar)
	target := filepath.Join(dir, "out")
	createsRel := "payload/hello.txt"
	m := New()
	d := decl(archivePath, StatePresent, map[string]any{"target": target, "creates": createsRel})

	// not extracted yet, creates path missing → drift
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("creates path missing → should drift")
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	assertCanonical(t, target)
	// no sentinel should have been written (creates is the check)
	if _, err := os.Stat(sentinelPath(target, archivePath)); !os.IsNotExist(err) {
		t.Error("no sentinel should be written when creates is set")
	}

	// now DELETE the archive file — creates still exists → converged,
	// archive never consulted
	if err := os.Remove(archivePath); err != nil {
		t.Fatal(err)
	}
	r, err := m.Check(context.Background(), d)
	if err != nil || !r.Matches {
		t.Errorf("creates-exists should be converged even with no archive: matches=%v err=%v", r.Matches, err)
	}
	sr, _ := m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("apply should be a no-op when creates exists")
	}

	// creates missing AND archive missing → Check errors
	d2 := decl(filepath.Join(dir, "gone.tar"), StatePresent, map[string]any{"target": target, "creates": filepath.Join(dir, "nope")})
	if _, err := m.Check(context.Background(), d2); err == nil {
		t.Error("creates missing + archive missing should error")
	}
}

func TestExtract_ReExtractOnSourceChange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	archivePath := archiveFor(t, dir, FormatTar)
	target := filepath.Join(dir, "out")
	m := New()
	d := decl(archivePath, StatePresent, map[string]any{"target": target})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Fatal("should be converged after first extract")
	}

	// rewrite the archive with different content
	newRaw := writeTarBytes(t, []tarEntry{{name: "payload/hello.txt", content: "CHANGED\n"}})
	if err := os.WriteFile(archivePath, newRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	// bump mtime well past the old one so the size+mtime identity changes
	if err := os.Chtimes(archivePath, timePlusHour(), timePlusHour()); err != nil {
		t.Fatal(err)
	}

	r, _ = m.Check(context.Background(), d)
	if r.Matches {
		t.Error("archive changed → should drift")
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(target, "payload", "hello.txt")); err != nil || string(b) != "CHANGED\n" {
		t.Errorf("re-extracted content: %q %v", b, err)
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("should be converged again after re-extract")
	}
}

func TestExtract_PathTraversalRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	raw := writeTarBytes(t, []tarEntry{
		{name: "payload/ok.txt", content: "ok\n"},
		{name: "../escape.txt", content: "PWNED\n"},
	})
	archivePath := writeFile(t, filepath.Join(dir, "evil.tar"), raw)
	target := filepath.Join(dir, "out")
	if _, err := New().Apply(context.Background(), decl(archivePath, StatePresent, map[string]any{"target": target})); err == nil {
		t.Fatal("a '..' entry should make the extraction fail")
	}
	// the escape file must not exist next to target
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); !os.IsNotExist(err) {
		t.Error("escape.txt was written outside the target!")
	}
}

func TestExtract_SymlinkSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	raw := writeTarBytes(t, []tarEntry{
		{name: "payload/real.txt", content: "real\n"},
		{name: "payload/link", typeflag: tar.TypeSymlink, linkname: "real.txt"},
	})
	archivePath := writeFile(t, filepath.Join(dir, "withlink.tar"), raw)
	target := filepath.Join(dir, "out")
	if _, err := New().Apply(context.Background(), decl(archivePath, StatePresent, map[string]any{"target": target})); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(target, "payload", "real.txt")); err != nil || string(b) != "real\n" {
		t.Errorf("regular file: %q %v", b, err)
	}
	if _, err := os.Lstat(filepath.Join(target, "payload", "link")); !os.IsNotExist(err) {
		t.Error("symlink entry should have been skipped, not created")
	}
}

func TestExtract_ModesPreserved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	raw := writeTarBytes(t, []tarEntry{
		{name: "secret/", typeflag: tar.TypeDir, mode: 0o700},
		{name: "secret/key", content: "k\n", mode: 0o600},
	})
	archivePath := writeFile(t, filepath.Join(dir, "m.tar"), raw)
	target := filepath.Join(dir, "out")
	if _, err := New().Apply(context.Background(), decl(archivePath, StatePresent, map[string]any{"target": target})); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(target, "secret")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("dir mode: %v %v", fi, err)
	}
	if fi, err := os.Stat(filepath.Join(target, "secret", "key")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode: %v %v", fi, err)
	}
}

func TestExtract_MissingSource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d := decl(filepath.Join(dir, "nope.tar"), StatePresent, map[string]any{"target": filepath.Join(dir, "out")})
	if _, err := New().Check(context.Background(), d); err == nil {
		t.Error("missing archive should error from Check")
	}
	if _, err := New().Apply(context.Background(), d); err == nil {
		t.Error("missing archive should error from Apply")
	}
}

func TestExtract_CorruptArchive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// invalid gzip → fails before extractTar
	gzPath := writeFile(t, filepath.Join(dir, "bad.tar.gz"), []byte("not actually gzip"))
	if _, err := New().Apply(context.Background(), decl(gzPath, StatePresent, map[string]any{"target": filepath.Join(dir, "out1")})); err == nil {
		t.Error("a corrupt gzip should error")
	}
	// garbage in a plain .tar → fails inside extractTar (tr.Next)
	tarPath := writeFile(t, filepath.Join(dir, "bad.tar"), bytes.Repeat([]byte("garbage!\x01\x02"), 64))
	if _, err := New().Apply(context.Background(), decl(tarPath, StatePresent, map[string]any{"target": filepath.Join(dir, "out2")})); err == nil {
		t.Error("a corrupt tar should error")
	}
}

func TestExtract_Zip_DirEntryAndSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// regular file
	w, _ := zw.Create("payload/file.txt")
	_, _ = w.Write([]byte("data\n"))
	// explicit dir entry with mode 0700
	dh := &zip.FileHeader{Name: "payload/secret/"}
	dh.SetMode(os.ModeDir | 0o700)
	if _, err := zw.CreateHeader(dh); err != nil {
		t.Fatal(err)
	}
	// symlink entry (content = link target)
	sh := &zip.FileHeader{Name: "payload/link"}
	sh.SetMode(os.ModeSymlink | 0o777)
	sw, err := zw.CreateHeader(sh)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = sw.Write([]byte("file.txt"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := writeFile(t, filepath.Join(dir, "z.zip"), buf.Bytes())
	target := filepath.Join(dir, "out")
	if _, err := New().Apply(context.Background(), decl(archivePath, StatePresent, map[string]any{"target": target})); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(target, "payload", "file.txt")); err != nil || string(b) != "data\n" {
		t.Errorf("regular file: %q %v", b, err)
	}
	if fi, err := os.Stat(filepath.Join(target, "payload", "secret")); err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Errorf("dir entry: %v %v", fi, err)
	}
	if _, err := os.Lstat(filepath.Join(target, "payload", "link")); !os.IsNotExist(err) {
		t.Error("zip symlink entry should be skipped")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "archive" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 1 || got[0] != StatePresent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("archive should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("/a.tar", StatePresent, map[string]any{"target": "/t"}), nil) != statemgmt.DriftSeverityMedium {
		t.Error("drift → MEDIUM")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("/a.tar", StatePresent, map[string]any{"target": "/t"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("/a.tar", StatePresent, nil)); err == nil {
		t.Error("missing target should be rejected")
	}

	// Test() round-trip
	dir := t.TempDir()
	archivePath := archiveFor(t, dir, FormatTar)
	target := filepath.Join(dir, "out")
	d := decl(archivePath, StatePresent, map[string]any{"target": target})
	ok, err := m.Test(context.Background(), d)
	if err != nil || ok {
		t.Errorf("Test before extract should be false: ok=%v err=%v", ok, err)
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	ok, err = m.Test(context.Background(), d)
	if err != nil || !ok {
		t.Errorf("Test after extract should be true: ok=%v err=%v", ok, err)
	}
}
