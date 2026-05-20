package dest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// LocalSource reads an artifact from a local filesystem path.
type LocalSource struct {
	Path string
}

// Open opens the file for reading. ctx is accepted for interface
// symmetry; local reads are synchronous.
func (l *LocalSource) Open(_ context.Context) (io.ReadCloser, error) {
	if l.Path == "" {
		return nil, fmt.Errorf("dest: LocalSource.Path must not be empty")
	}
	f, err := os.Open(l.Path) //nolint:gosec // operator-supplied artifact path
	if err != nil {
		return nil, fmt.Errorf("dest: open %q: %w", l.Path, err)
	}
	return f, nil
}

// LocalLister enumerates `.tar` artifacts in a local directory. Only
// regular files matching `*.tar` are returned; non-matching names and
// subdirectories are ignored.
type LocalLister struct {
	Dir string
}

// localArtifactExt is the suffix [LocalLister] filters on. Encrypted
// artifacts (age envelope around tar) keep the same `.tar` name
// because the wrapper is transparent on disk — operators can always
// distinguish encrypted/plain by reading the manifest.
const localArtifactExt = ".tar"

// List globs `Dir/*.tar` and returns one [Entry] per match, sorted by
// Name. A missing directory is a real error; an empty matching set is
// an empty result, not an error.
func (l *LocalLister) List(_ context.Context) ([]Entry, error) {
	if l.Dir == "" {
		return nil, fmt.Errorf("dest: LocalLister.Dir must not be empty")
	}
	entries, err := os.ReadDir(l.Dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("dest: directory %q does not exist", l.Dir)
		}
		return nil, fmt.Errorf("dest: read dir %q: %w", l.Dir, err)
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != localArtifactExt {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("dest: stat %q: %w", e.Name(), err)
		}
		out = append(out, Entry{
			Name:         e.Name(),
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
