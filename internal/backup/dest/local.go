package dest

import (
	"context"
	"fmt"
	"io"
	"os"
)

// LocalDestination writes the artifact to a local file. The parent
// directory must already exist — operators get a real "no such file
// or directory" error rather than a silently-created path.
type LocalDestination struct {
	Path string
}

// Open creates (or truncates) the file at Path with mode 0o600. The
// returned writer is the underlying *os.File; Close closes the file.
// ctx is accepted for interface symmetry with the S3 backend but is
// not used — local writes are synchronous.
func (l *LocalDestination) Open(_ context.Context) (io.WriteCloser, error) {
	if l.Path == "" {
		return nil, fmt.Errorf("dest: LocalDestination.Path must not be empty")
	}
	f, err := os.OpenFile(l.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // operator-supplied backup path
	if err != nil {
		return nil, fmt.Errorf("dest: open %q: %w", l.Path, err)
	}
	return f, nil
}
