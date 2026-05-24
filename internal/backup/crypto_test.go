// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// stubEncrypter prepends a fixed marker and uppercases the payload.
// Not real encryption — it just lets the seam contract be tested
// without pulling in filippo.io/age (which has its own coverage in
// internal/backup/age).
type stubEncrypter struct {
	wrapErr  error
	closeErr error
}

func (e *stubEncrypter) Wrap(dst io.Writer) (io.WriteCloser, error) {
	if e.wrapErr != nil {
		return nil, e.wrapErr
	}
	_, _ = dst.Write([]byte("STUB::"))
	return &stubWriter{dst: dst, closeErr: e.closeErr}, nil
}

type stubWriter struct {
	dst      io.Writer
	closeErr error
}

func (w *stubWriter) Write(p []byte) (int, error) {
	return w.dst.Write(bytes.ToUpper(p))
}

func (w *stubWriter) Close() error {
	if _, err := w.dst.Write([]byte("::END")); err != nil {
		return err
	}
	return w.closeErr
}

type stubDecrypter struct {
	wrapErr error
}

func (d *stubDecrypter) Wrap(src io.Reader) (io.Reader, error) {
	if d.wrapErr != nil {
		return nil, d.wrapErr
	}
	body, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	// Reverse the stubEncrypter: strip "STUB::" prefix and "::END"
	// suffix; lowercase the middle.
	if !bytes.HasPrefix(body, []byte("STUB::")) || !bytes.HasSuffix(body, []byte("::END")) {
		return nil, errors.New("stub: bad envelope")
	}
	inner := body[len("STUB::") : len(body)-len("::END")]
	return bytes.NewReader(bytes.ToLower(inner)), nil
}

func TestNewEncryptingWriter_NilEncrypter(t *testing.T) {
	var dst bytes.Buffer
	wc, err := NewEncryptingWriter(&dst, nil)
	if err != nil {
		t.Fatalf("NewEncryptingWriter: %v", err)
	}
	if _, err := wc.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got, want := dst.String(), "hello"; got != want {
		t.Errorf("dst = %q, want %q (passthrough)", got, want)
	}
}

func TestNewEncryptingWriter_NilDst(t *testing.T) {
	if _, err := NewEncryptingWriter(nil, &stubEncrypter{}); err == nil {
		t.Fatal("want error")
	}
}

func TestNewEncryptingWriter_RealEncrypter(t *testing.T) {
	var dst bytes.Buffer
	wc, err := NewEncryptingWriter(&dst, &stubEncrypter{})
	if err != nil {
		t.Fatalf("NewEncryptingWriter: %v", err)
	}
	if _, err := wc.Write([]byte("hello world")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got, want := dst.String(), "STUB::HELLO WORLD::END"; got != want {
		t.Errorf("dst = %q, want %q", got, want)
	}
}

func TestNewEncryptingWriter_WrapError(t *testing.T) {
	wantErr := errors.New("wrap failed")
	var dst bytes.Buffer
	_, err := NewEncryptingWriter(&dst, &stubEncrypter{wrapErr: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want errors.Is(%v)", err, wantErr)
	}
}

func TestNewDecryptingReader_NilDecrypter(t *testing.T) {
	src := strings.NewReader("hello")
	r, err := NewDecryptingReader(src, nil)
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if got, want := string(body), "hello"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestNewDecryptingReader_NilSrc(t *testing.T) {
	if _, err := NewDecryptingReader(nil, &stubDecrypter{}); err == nil {
		t.Fatal("want error")
	}
}

func TestNewDecryptingReader_RealDecrypter(t *testing.T) {
	src := strings.NewReader("STUB::HELLO WORLD::END")
	r, err := NewDecryptingReader(src, &stubDecrypter{})
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if got, want := string(body), "hello world"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestNewDecryptingReader_WrapError(t *testing.T) {
	wantErr := errors.New("wrap failed")
	src := strings.NewReader("anything")
	_, err := NewDecryptingReader(src, &stubDecrypter{wrapErr: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want errors.Is(%v)", err, wantErr)
	}
}

// TestRoundTrip_StubWithStubDecrypter ensures the seam contract holds:
// what comes out of NewDecryptingReader on the dst of
// NewEncryptingWriter must equal the original payload.
func TestRoundTrip_StubWithStubDecrypter(t *testing.T) {
	original := []byte("the quick brown fox")
	var buf bytes.Buffer
	enc, err := NewEncryptingWriter(&buf, &stubEncrypter{})
	if err != nil {
		t.Fatalf("NewEncryptingWriter: %v", err)
	}
	if _, err := enc.Write(original); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	dec, err := NewDecryptingReader(&buf, &stubDecrypter{})
	if err != nil {
		t.Fatalf("NewDecryptingReader: %v", err)
	}
	got, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("round-trip got %q, want %q", got, original)
	}
}
