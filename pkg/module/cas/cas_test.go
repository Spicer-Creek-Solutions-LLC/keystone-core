package cas_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/module/cas"
)

func TestHashFormatParse(t *testing.T) {
	h := cas.HashBytes([]byte("hello"))
	// sha256("hello") well-known digest.
	const want = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if h != want {
		t.Fatalf("HashBytes = %q, want %q", h, want)
	}
	sh, err := cas.Hash(strings.NewReader("hello"))
	if err != nil || sh != want {
		t.Fatalf("Hash = %q,%v", sh, err)
	}
	d, err := cas.ParseHash(want)
	if err != nil || d != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("ParseHash = %q,%v", d, err)
	}
	if f, err := cas.FormatHash(d); err != nil || f != want {
		t.Fatalf("FormatHash = %q,%v", f, err)
	}
	for _, bad := range []string{"", "sha256:", "sha256:zzz", d, "md5:" + d, "sha256:" + strings.ToUpper(d)} {
		if _, err := cas.ParseHash(bad); !errors.Is(err, cas.ErrInvalidHash) {
			t.Errorf("ParseHash(%q) = %v, want ErrInvalidHash", bad, err)
		}
	}
	if _, err := cas.FormatHash("nothex"); !errors.Is(err, cas.ErrInvalidHash) {
		t.Errorf("FormatHash(nothex): want ErrInvalidHash")
	}
}

func newStore(t *testing.T) *cas.Store {
	t.Helper()
	s, err := cas.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestPutGetRoundTrip(t *testing.T) {
	s := newStore(t)
	content := []byte("the module zip bytes")
	h, err := s.Put(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if h != cas.HashBytes(content) {
		t.Fatalf("Put hash = %q", h)
	}
	if !s.Has(h) {
		t.Fatal("Has = false after Put")
	}
	rc, err := s.Get(h)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, content) {
		t.Fatalf("Get content = %q", got)
	}
	if err := s.Verify(h); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	p, _ := s.Path(h)
	if !strings.HasSuffix(p, filepath.Join(strings.TrimPrefix(h, "sha256:"), "content")) {
		t.Fatalf("Path layout = %q", p)
	}
}

func TestPutIdempotent(t *testing.T) {
	s := newStore(t)
	c := []byte("dup")
	h1, _ := s.Put(bytes.NewReader(c))
	h2, err := s.Put(bytes.NewReader(c))
	if err != nil || h1 != h2 {
		t.Fatalf("idempotent Put: %q %q %v", h1, h2, err)
	}
}

func TestPutExpected(t *testing.T) {
	s := newStore(t)
	c := []byte("payload")
	good := cas.HashBytes(c)
	if h, err := s.PutExpected(bytes.NewReader(c), good); err != nil || h != good {
		t.Fatalf("PutExpected good = %q,%v", h, err)
	}
	wrong := cas.HashBytes([]byte("other"))
	if _, err := s.PutExpected(bytes.NewReader(c), wrong); !errors.Is(err, cas.ErrHashMismatch) {
		t.Fatalf("PutExpected mismatch = %v, want ErrHashMismatch", err)
	}
	// Mismatched content must NOT have been committed under `wrong`.
	if s.Has(wrong) {
		t.Fatal("mismatched content was committed")
	}
	if _, err := s.PutExpected(bytes.NewReader(c), "bogus"); !errors.Is(err, cas.ErrInvalidHash) {
		t.Fatalf("PutExpected bad hash = %v, want ErrInvalidHash", err)
	}
}

func TestGetMissingAndDelete(t *testing.T) {
	s := newStore(t)
	missing := cas.HashBytes([]byte("nope"))
	if _, err := s.Get(missing); !errors.Is(err, cas.ErrNotFound) {
		t.Fatalf("Get missing = %v, want ErrNotFound", err)
	}
	if err := s.Verify(missing); !errors.Is(err, cas.ErrNotFound) {
		t.Fatalf("Verify missing = %v, want ErrNotFound", err)
	}
	h, _ := s.Put(strings.NewReader("x"))
	if err := s.Delete(h); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.Has(h) {
		t.Fatal("Has = true after Delete")
	}
	if err := s.Delete(h); err != nil { // idempotent
		t.Fatalf("Delete idempotent: %v", err)
	}
	if _, err := s.Get("bad"); !errors.Is(err, cas.ErrInvalidHash) {
		t.Fatalf("Get(bad) = %v, want ErrInvalidHash", err)
	}
}

func TestVerifyDetectsCorruption(t *testing.T) {
	s := newStore(t)
	h, _ := s.Put(strings.NewReader("trusted content"))
	p, _ := s.Path(h)
	if err := os.WriteFile(p, []byte("tampered!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(h); !errors.Is(err, cas.ErrCorrupted) {
		t.Fatalf("Verify tampered = %v, want ErrCorrupted", err)
	}
}

func TestConcurrentPutSameContent(t *testing.T) {
	s := newStore(t)
	c := []byte(strings.Repeat("concurrent", 100))
	want := cas.HashBytes(c)
	var wg sync.WaitGroup
	errs := make([]error, 16)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h, err := s.Put(bytes.NewReader(c))
			if err == nil && h != want {
				err = errors.New("hash mismatch under concurrency")
			}
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Put #%d: %v", i, err)
		}
	}
	if err := s.Verify(want); err != nil {
		t.Fatalf("post-concurrency Verify: %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestReaderErrorsPropagate(t *testing.T) {
	if _, err := cas.Hash(errReader{}); err == nil {
		t.Fatal("Hash(errReader): want error")
	}
	s := newStore(t)
	if _, err := s.Put(errReader{}); err == nil {
		t.Fatal("Put(errReader): want error")
	}
	// The failed Put must not have left a temp file behind.
	ents, _ := os.ReadDir(filepathRoot(t, s))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".put-") {
			t.Fatalf("leaked temp file %q after failed Put", e.Name())
		}
	}
}

func filepathRoot(t *testing.T, s *cas.Store) string {
	t.Helper()
	// Path() of any hash is <root>/<hex>/content — go up two.
	p, err := s.Path(cas.HashBytes(nil))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(p))
}

func TestNewOnNonDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := cas.New(f); err == nil {
		t.Fatal("New(filepath of a regular file): want error")
	}
}

func TestHasAndDeleteEdgeCases(t *testing.T) {
	s := newStore(t)
	if s.Has("bad") {
		t.Fatal("Has(bad hash) = true")
	}
	if err := s.Delete("bad"); !errors.Is(err, cas.ErrInvalidHash) {
		t.Fatalf("Delete(bad) = %v, want ErrInvalidHash", err)
	}
	// A directory (not a regular file) at the blob path → Has=false.
	h := cas.HashBytes([]byte("dir-not-file"))
	p, _ := s.Path(h)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if s.Has(h) {
		t.Fatal("Has = true for a directory at the blob path")
	}
}

func TestEntries(t *testing.T) {
	s := newStore(t)
	root := filepathRoot(t, s)

	if es, err := s.Entries(); err != nil || len(es) != 0 {
		t.Fatalf("empty Entries = %v,%v", es, err)
	}
	ha, _ := s.Put(strings.NewReader("aaaa"))
	hb, _ := s.Put(strings.NewReader("bbbbbb"))

	// Junk in the root must be skipped, not error.
	if err := os.WriteFile(filepath.Join(root, ".put-leftover"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "not-a-hex-dir"), 0o750); err != nil {
		t.Fatal(err)
	}

	es, err := s.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(es) != 2 {
		t.Fatalf("Entries len = %d, want 2 (junk skipped): %+v", len(es), es)
	}
	bySize := map[int64]string{}
	for _, e := range es {
		bySize[e.Size] = e.Hash
		if e.ModTime.IsZero() {
			t.Fatalf("entry %s has zero ModTime", e.Hash)
		}
	}
	if bySize[4] != ha || bySize[6] != hb {
		t.Fatalf("Entries size/hash mismatch: %+v (ha=%s hb=%s)", es, ha, hb)
	}
}

func TestDefaultRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := cas.DefaultRoot()
	if !strings.HasSuffix(r, filepath.Join(".kscore", "modules")) {
		t.Fatalf("DefaultRoot = %q", r)
	}
	s, err := cas.New("") // empty → DefaultRoot, created
	if err != nil {
		t.Fatalf("New(\"\"): %v", err)
	}
	if _, err := s.Put(strings.NewReader("via default root")); err != nil {
		t.Fatalf("Put under default root: %v", err)
	}
}
