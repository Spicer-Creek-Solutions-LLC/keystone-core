package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// detectFormat resolves the format to extract with: the declared
// value when it isn't `auto`, otherwise inferred from the archive
// filename's extension.
func detectFormat(archivePath, declared string) (string, error) {
	if declared != FormatAuto {
		return declared, nil
	}
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return FormatTarGz, nil
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"), strings.HasSuffix(lower, ".tbz"):
		return FormatTarBz2, nil
	case strings.HasSuffix(lower, ".tar"):
		return FormatTar, nil
	case strings.HasSuffix(lower, ".zip"):
		return FormatZip, nil
	default:
		return "", fmt.Errorf("cannot infer archive format from %q; set format: explicitly", archivePath)
	}
}

// sourceIdentity returns the archive file's size and modification
// time (Unix nanoseconds). A directory in place of the archive is an
// error.
func sourceIdentity(archivePath string) (size, mtime int64, err error) {
	fi, err := os.Stat(archivePath)
	if err != nil {
		return 0, 0, err
	}
	if fi.IsDir() {
		return 0, 0, fmt.Errorf("%s is a directory, not an archive file", archivePath)
	}
	return fi.Size(), fi.ModTime().UnixNano(), nil
}

// --- extraction-state sentinel ---------------------------------------

const sentinelHeader = "keystone-archive v1"

// sentinelPath returns the marker file the module writes (when
// `creates` is not set) to record that this archive was extracted
// here: <target>/.keystone-archive.<first-8-hex-of-sha256(archivePath)>.
func sentinelPath(target, archivePath string) string {
	sum := sha256.Sum256([]byte(archivePath))
	return filepath.Join(target, ".keystone-archive."+hex.EncodeToString(sum[:])[:8])
}

// readSentinel returns the recorded source size + mtime, and whether
// a well-formed sentinel was found.
func readSentinel(path string) (size, mtime int64, ok bool) {
	data, err := os.ReadFile(path) //nolint:gosec // path is under the operator-supplied target dir
	if err != nil {
		return 0, 0, false
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != sentinelHeader {
		return 0, 0, false
	}
	haveSize, haveMtime := false, false
	for _, ln := range lines[1:] {
		k, v, found := strings.Cut(strings.TrimSpace(ln), "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(k) {
		case "size":
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				size, haveSize = n, true
			}
		case "mtime":
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				mtime, haveMtime = n, true
			}
		}
	}
	if !haveSize || !haveMtime {
		return 0, 0, false
	}
	return size, mtime, true
}

func writeSentinel(path, archivePath string, size, mtime int64) error {
	content := fmt.Sprintf("%s\nsource=%s\nsize=%d\nmtime=%d\n", sentinelHeader, archivePath, size, mtime)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil { //nolint:gosec // marker file is world-readable
		return fmt.Errorf("write sentinel: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename sentinel: %w", err)
	}
	return nil
}

// --- path safety ------------------------------------------------------

// sanitizeEntryPath turns an archive entry name into a filesystem
// path under target, applying strip_components. skip is true for
// empty / fully-stripped entries; err is non-nil for entries that
// try to escape target (absolute paths, '..' components).
func sanitizeEntryPath(target, raw string, strip int) (full string, skip bool, err error) {
	name := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	name = strings.TrimPrefix(name, "/")
	cleaned := make([]string, 0, strings.Count(name, "/")+1)
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			return "", false, fmt.Errorf("archive entry %q contains a '..' path component", raw)
		}
		cleaned = append(cleaned, seg)
	}
	if len(cleaned) == 0 {
		return "", true, nil
	}
	if strip > 0 {
		if len(cleaned) <= strip {
			return "", true, nil
		}
		cleaned = cleaned[strip:]
	}
	full = filepath.Join(append([]string{target}, cleaned...)...)
	// belt-and-suspenders: the join above can't escape after the
	// '..' rejection, but confirm explicitly.
	if full != filepath.Clean(target) {
		rel, relErr := filepath.Rel(target, full)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", false, fmt.Errorf("archive entry %q escapes the target directory", raw)
		}
	}
	return full, false, nil
}

// --- extraction -------------------------------------------------------

func extract(archivePath, target, format string, strip int) error {
	if format == FormatZip {
		return extractZip(archivePath, target, strip)
	}
	f, err := os.Open(archivePath) //nolint:gosec // operator-supplied archive path
	if err != nil {
		return fmt.Errorf("open %s: %w", archivePath, err)
	}
	defer func() { _ = f.Close() }()

	var r io.Reader = f
	switch format {
	case FormatTarGz:
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("gzip %s: %w", archivePath, err)
		}
		defer func() { _ = gz.Close() }()
		r = gz
	case FormatTarBz2:
		r = bzip2.NewReader(f)
	case FormatTar:
		// r = f
	}
	return extractTar(r, target, strip)
}

func extractTar(r io.Reader, target string, strip int) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		full, skip, err := sanitizeEntryPath(target, hdr.Name, strip)
		if err != nil {
			return err
		}
		if skip {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := ensureDir(full); err != nil {
				return err
			}
			if m := fs.FileMode(hdr.Mode).Perm(); m != 0 {
				if err := os.Chmod(full, m); err != nil {
					return fmt.Errorf("chmod %s: %w", full, err)
				}
			}
		case tar.TypeReg:
			if err := writeEntry(full, tr, fs.FileMode(hdr.Mode).Perm()); err != nil {
				return err
			}
		default:
			// symlinks, hardlinks, char/block devices, fifos, and
			// extended-header records are skipped in v1.0 (V1X:
			// safe symlink extraction).
			continue
		}
	}
}

func extractZip(archivePath, target string, strip int) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip %s: %w", archivePath, err)
	}
	defer func() { _ = zr.Close() }()
	for _, zf := range zr.File {
		full, skip, err := sanitizeEntryPath(target, zf.Name, strip)
		if err != nil {
			return err
		}
		if skip {
			continue
		}
		mode := zf.FileInfo().Mode()
		switch {
		case zf.FileInfo().IsDir():
			if err := ensureDir(full); err != nil {
				return err
			}
			if m := mode.Perm(); m != 0 {
				if err := os.Chmod(full, m); err != nil {
					return fmt.Errorf("chmod %s: %w", full, err)
				}
			}
		case mode&os.ModeSymlink != 0:
			continue // skip symlinks (V1X)
		case mode.IsRegular():
			rc, err := zf.Open()
			if err != nil {
				return fmt.Errorf("open zip entry %q: %w", zf.Name, err)
			}
			werr := writeEntry(full, rc, mode.Perm())
			_ = rc.Close()
			if werr != nil {
				return werr
			}
		default:
			continue
		}
	}
	return nil
}

func writeEntry(full string, r io.Reader, mode fs.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	if err := ensureDir(filepath.Dir(full)); err != nil {
		return err
	}
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) //nolint:gosec // full is validated by sanitizeEntryPath to stay within target
	if err != nil {
		return fmt.Errorf("create %s: %w", full, err)
	}
	if _, err := io.Copy(f, r); err != nil { //nolint:gosec // G110: archive source is operator-supplied; extraction-size limits are a v1.x hardening item
		_ = f.Close()
		return fmt.Errorf("write %s: %w", full, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", full, err)
	}
	if err := os.Chmod(full, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", full, err)
	}
	return nil
}

// ensureDir creates dir (and any missing parents) at the conventional
// 0755; explicit directory entries in the archive are chmod'd to
// their archived mode afterward.
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // dir is validated by sanitizeEntryPath; 0755 is the conventional default for extracted directories
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return nil
}
