package statemgmt

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// Loader reads a state file from an fs.FS and recursively expands its
// includes into one merged StateFile. The contract is:
//
//   - Each include path is resolved relative to the file that
//     contains it (Salt convention).
//
//   - Variables merge depth-first post-order. Includes layer first
//     (later includes override earlier siblings); the including file's
//     own variables overlay last. The root file you call Load on wins
//     over any transitive include.
//
//   - Declarations flatten depth-first preorder: each include's
//     declarations land first (in include-list order), then the
//     including file's own declarations. A duplicate ID anywhere in
//     the merged tree is rejected with both file paths in the error
//     (no extend/override in v1.0).
//
//   - The same file reached via two paths in a diamond loads exactly
//     once; cycles (direct or transitive) are rejected with the full
//     cycle path in the error.
//
//   - Only the root file's Metadata is kept.
//
//   - The returned StateFile.Includes is zero — the field has been
//     consumed; downstream phases must not re-expand.
type Loader struct {
	FS fs.FS
}

// NewLoader returns a Loader rooted at filesystem. Production callers
// pass os.DirFS(stateDir); tests can pass fstest.MapFS.
func NewLoader(filesystem fs.FS) *Loader {
	return &Loader{FS: filesystem}
}

// Load reads rootPath, expands its includes recursively, and returns
// the merged StateFile.
func (l *Loader) Load(rootPath string) (*StateFile, error) {
	if l.FS == nil {
		return nil, errors.New("statemgmt: load: Loader.FS is nil")
	}
	rootPath = path.Clean(rootPath)
	state := loaderState{
		fs:    l.FS,
		cache: make(map[string]*StateFile),
	}
	merged, _, err := state.load(rootPath, nil)
	if err != nil {
		return nil, err
	}
	merged.Includes = nil
	return merged, nil
}

// loaderState carries cross-recursion bookkeeping for one Load
// invocation. cache memoises parsed leaves so a diamond does not
// reparse them; the visiting stack is per-call-stack and detects
// cycles.
type loaderState struct {
	fs    fs.FS
	cache map[string]*StateFile
}

// load returns the fully-merged StateFile rooted at filePath plus a
// parallel map (declID → source file path) so the caller can detect
// duplicates and surface both paths in the error message. visiting is
// the chain of files we are mid-load on; a re-entry into it is a
// cycle.
func (s *loaderState) load(filePath string, visiting []string) (*StateFile, map[string]string, error) {
	for _, ancestor := range visiting {
		if ancestor == filePath {
			return nil, nil, fmt.Errorf("statemgmt: load: include cycle: %s", formatCycle(visiting, filePath))
		}
	}

	parsed, err := s.parseOnce(filePath)
	if err != nil {
		return nil, nil, err
	}

	nextVisiting := append(visiting, filePath) //nolint:gocritic // intentional new slice
	includeDir := path.Dir(filePath)
	if includeDir == "." {
		includeDir = ""
	}

	mergedVars := map[string]any{}
	var mergedDecls []*Declaration
	declOrigin := map[string]string{}

	for _, inc := range parsed.Includes {
		incPath := resolveInclude(includeDir, inc)
		childSF, childOrigin, err := s.load(incPath, nextVisiting)
		if err != nil {
			if isFileNotFound(err) {
				return nil, nil, fmt.Errorf("statemgmt: load %s: include %q not found", filePath, inc)
			}
			return nil, nil, err
		}
		layerVars(mergedVars, childSF.Variables)
		for _, d := range childSF.Declarations {
			if prev, dup := declOrigin[d.ID]; dup {
				return nil, nil, fmt.Errorf("statemgmt: load: declaration %q declared in both %s and %s", d.ID, prev, childOrigin[d.ID])
			}
			declOrigin[d.ID] = childOrigin[d.ID]
			mergedDecls = append(mergedDecls, d)
		}
	}

	// Current file's variables overlay last (root-most wins).
	layerVars(mergedVars, parsed.Variables)
	for _, d := range parsed.Declarations {
		if prev, dup := declOrigin[d.ID]; dup {
			return nil, nil, fmt.Errorf("statemgmt: load: declaration %q declared in both %s and %s", d.ID, prev, filePath)
		}
		declOrigin[d.ID] = filePath
		mergedDecls = append(mergedDecls, d)
	}

	merged := &StateFile{
		Metadata:     parsed.Metadata,
		Includes:     parsed.Includes,
		Variables:    nilIfEmpty(mergedVars),
		Declarations: mergedDecls,
	}
	return merged, declOrigin, nil
}

// parseOnce reads and parses filePath through the cache.
func (s *loaderState) parseOnce(filePath string) (*StateFile, error) {
	if cached, ok := s.cache[filePath]; ok {
		return cached, nil
	}
	data, err := fs.ReadFile(s.fs, filePath)
	if err != nil {
		return nil, err
	}
	parsed, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%w in %s", err, filePath)
	}
	s.cache[filePath] = parsed
	return parsed, nil
}

func resolveInclude(dir, inc string) string {
	if dir == "" {
		return path.Clean(inc)
	}
	return path.Clean(path.Join(dir, inc))
}

func layerVars(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

func nilIfEmpty(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	return m
}

func formatCycle(stack []string, repeated string) string {
	// Walk from the first occurrence of repeated so the printed
	// cycle is exactly the loop, not the lead-in.
	start := 0
	for i, p := range stack {
		if p == repeated {
			start = i
			break
		}
	}
	chain := append([]string{}, stack[start:]...)
	chain = append(chain, repeated)
	return strings.Join(chain, " → ")
}

func isFileNotFound(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
