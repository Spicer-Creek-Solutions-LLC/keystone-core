package backup

import (
	"fmt"
	"io"
)

// Encrypter wraps a destination [io.Writer] with an envelope cipher.
// Concrete implementations live in subpackages (e.g.
// internal/backup/age); the kscore-backup CLI composes one of them
// between the destination file and [BackupManager.CreateBackup].
//
// Wrap returns a [io.WriteCloser] — callers MUST Close to flush the
// cipher trailer, and Close errors MUST be checked. A nil Encrypter
// is the documented "no encryption" path; use it in tests or when an
// operator opts out explicitly.
type Encrypter interface {
	Wrap(dst io.Writer) (io.WriteCloser, error)
}

// Decrypter is the reader-side counterpart of [Encrypter]. The
// restore flow (Epic 18 task 6) wraps the source reader with this
// before handing the stream to the manifest+tar reader.
type Decrypter interface {
	Wrap(src io.Reader) (io.Reader, error)
}

// NewEncryptingWriter is sugar for the CLI compose path:
//
//	enc := age.Encrypter{Recipients: ...}
//	out, err := backup.NewEncryptingWriter(dest, enc)
//	defer out.Close()
//	manifest, err := mgr.CreateBackup(ctx, out)
//
// Passing a nil enc returns a [nopWriteCloser] around dst — the
// caller composes uniformly regardless of whether encryption is on.
func NewEncryptingWriter(dst io.Writer, enc Encrypter) (io.WriteCloser, error) {
	if dst == nil {
		return nil, fmt.Errorf("backup: NewEncryptingWriter: dst must not be nil")
	}
	if enc == nil {
		return nopWriteCloser{Writer: dst}, nil
	}
	wc, err := enc.Wrap(dst)
	if err != nil {
		return nil, fmt.Errorf("backup: wrap encrypter: %w", err)
	}
	return wc, nil
}

// NewDecryptingReader is the symmetric helper for Restore. A nil dec
// returns src unchanged.
func NewDecryptingReader(src io.Reader, dec Decrypter) (io.Reader, error) {
	if src == nil {
		return nil, fmt.Errorf("backup: NewDecryptingReader: src must not be nil")
	}
	if dec == nil {
		return src, nil
	}
	r, err := dec.Wrap(src)
	if err != nil {
		return nil, fmt.Errorf("backup: wrap decrypter: %w", err)
	}
	return r, nil
}

// nopWriteCloser turns an io.Writer into an io.WriteCloser with a
// no-op Close, used by [NewEncryptingWriter] when no encrypter is
// supplied.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
