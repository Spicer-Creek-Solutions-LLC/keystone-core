package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FilesystemBackend implements Backend for local filesystem storage.
type FilesystemBackend struct {
	config *FilesystemConfig
}

// FilesystemConfig configures the filesystem backend.
type FilesystemConfig struct {
	Config `yaml:",inline"`

	// Root directory for file storage.
	Root string `yaml:"root"`

	// CreateDirs creates directories as needed.
	CreateDirs bool `yaml:"create_dirs,omitempty"`

	// Permissions for created files (default 0644).
	FileMode os.FileMode `yaml:"file_mode,omitempty"`

	// Permissions for created directories (default 0755).
	DirMode os.FileMode `yaml:"dir_mode,omitempty"`
}

// NewFilesystemBackend creates a new filesystem backend.
func NewFilesystemBackend(config *FilesystemConfig) (*FilesystemBackend, error) {
	if config.Root == "" {
		return nil, fmt.Errorf("filesystem backend: root directory is required")
	}

	// Ensure root exists
	info, err := os.Stat(config.Root)
	if err != nil {
		if os.IsNotExist(err) && config.CreateDirs {
			mode := config.DirMode
			if mode == 0 {
				mode = 0755
			}
			if err := os.MkdirAll(config.Root, mode); err != nil {
				return nil, fmt.Errorf("filesystem backend: failed to create root: %w", err)
			}
		} else {
			return nil, fmt.Errorf("filesystem backend: root directory error: %w", err)
		}
	} else if !info.IsDir() {
		return nil, fmt.Errorf("filesystem backend: root is not a directory")
	}

	// Set defaults
	if config.FileMode == 0 {
		config.FileMode = 0644
	}
	if config.DirMode == 0 {
		config.DirMode = 0755
	}

	return &FilesystemBackend{config: config}, nil
}

// Name returns the backend name.
func (b *FilesystemBackend) Name() string {
	return b.config.Name
}

// Type returns the backend type.
func (b *FilesystemBackend) Type() BackendType {
	return BackendTypeFilesystem
}

// BaseConfig returns the base configuration for path matching and priority.
func (b *FilesystemBackend) BaseConfig() *Config {
	return &b.config.Config
}

// Get retrieves a file from the filesystem.
func (b *FilesystemBackend) Get(ctx context.Context, path string, opts *GetOptions) (*GetResult, error) {
	fullPath := b.fullPath(path)

	// Check context
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Open file
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &BackendError{Backend: b.config.Name, Op: "get", Path: path, Err: errNotFound{}}
		}
		if os.IsPermission(err) {
			return nil, &BackendError{Backend: b.config.Name, Op: "get", Path: path, Err: errAccessDenied{}}
		}
		return nil, &BackendError{Backend: b.config.Name, Op: "get", Path: path, Err: err}
	}

	// Get file info
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, &BackendError{Backend: b.config.Name, Op: "stat", Path: path, Err: err}
	}

	if info.IsDir() {
		file.Close()
		return nil, &BackendError{Backend: b.config.Name, Op: "get", Path: path, Err: fmt.Errorf("path is a directory")}
	}

	// Calculate checksum
	checksum, err := calculateFileChecksum(file)
	if err != nil {
		file.Close()
		return nil, &BackendError{Backend: b.config.Name, Op: "checksum", Path: path, Err: err}
	}

	// Check conditional GET
	if opts != nil && opts.IfNoneMatch != "" && opts.IfNoneMatch == checksum {
		file.Close()
		return &GetResult{
			NotModified: true,
			Info: &FileInfo{
				Path:         path,
				Name:         filepath.Base(path),
				Size:         info.Size(),
				Checksum:     checksum,
				ContentType:  detectContentType(path),
				ModifiedTime: info.ModTime(),
			},
		}, nil
	}

	// Reset file position after checksum calculation
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, &BackendError{Backend: b.config.Name, Op: "seek", Path: path, Err: err}
	}

	// Handle range request
	var reader io.ReadCloser = file
	size := info.Size()

	if opts != nil && opts.Range != nil {
		start := opts.Range.Start
		end := opts.Range.End
		if end == 0 {
			end = size
		}

		if start < 0 || start >= size {
			file.Close()
			return nil, &BackendError{Backend: b.config.Name, Op: "get", Path: path, Err: fmt.Errorf("invalid range start")}
		}

		if _, err := file.Seek(start, io.SeekStart); err != nil {
			file.Close()
			return nil, &BackendError{Backend: b.config.Name, Op: "seek", Path: path, Err: err}
		}

		reader = &limitedReadCloser{
			rc:    file,
			limit: end - start,
		}
		size = end - start
	}

	return &GetResult{
		Reader: reader,
		Info: &FileInfo{
			Path:         path,
			Name:         filepath.Base(path),
			Size:         size,
			Checksum:     checksum,
			ContentType:  detectContentType(path),
			ModifiedTime: info.ModTime(),
		},
	}, nil
}

// Put stores a file in the filesystem.
func (b *FilesystemBackend) Put(ctx context.Context, path string, reader io.Reader, opts *PutOptions) (*PutResult, error) {
	if b.config.ReadOnly {
		return nil, &BackendError{Backend: b.config.Name, Op: "put", Path: path, Err: fmt.Errorf("backend is read-only")}
	}

	fullPath := b.fullPath(path)

	// Check context
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Check if file exists and overwrite is not allowed
	if opts != nil && !opts.Overwrite {
		if _, err := os.Stat(fullPath); err == nil {
			return nil, &BackendError{Backend: b.config.Name, Op: "put", Path: path, Err: fmt.Errorf("file already exists")}
		}
	}

	// Create parent directories if needed
	dir := filepath.Dir(fullPath)
	if b.config.CreateDirs {
		if err := os.MkdirAll(dir, b.config.DirMode); err != nil {
			return nil, &BackendError{Backend: b.config.Name, Op: "mkdir", Path: dir, Err: err}
		}
	}

	// Create temporary file in same directory for atomic write
	tmpFile, err := os.CreateTemp(dir, ".kscore-upload-*")
	if err != nil {
		return nil, &BackendError{Backend: b.config.Name, Op: "create", Path: path, Err: err}
	}
	tmpPath := tmpFile.Name()
	defer func() {
		// Clean up temp file on error
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	// Copy data and calculate checksum
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmpFile, hash), reader)
	if err != nil {
		return nil, &BackendError{Backend: b.config.Name, Op: "write", Path: path, Err: err}
	}

	checksum := hex.EncodeToString(hash.Sum(nil))

	// Verify checksum if provided
	if opts != nil && opts.Checksum != "" && opts.Checksum != checksum {
		return nil, &BackendError{Backend: b.config.Name, Op: "put", Path: path, Err: fmt.Errorf("checksum mismatch: expected %s, got %s", opts.Checksum, checksum)}
	}

	// Close temp file before rename
	if err := tmpFile.Close(); err != nil {
		return nil, &BackendError{Backend: b.config.Name, Op: "close", Path: path, Err: err}
	}

	// Set file permissions
	if err := os.Chmod(tmpPath, b.config.FileMode); err != nil {
		return nil, &BackendError{Backend: b.config.Name, Op: "chmod", Path: path, Err: err}
	}

	// Atomic rename
	if err := os.Rename(tmpPath, fullPath); err != nil {
		return nil, &BackendError{Backend: b.config.Name, Op: "rename", Path: path, Err: err}
	}

	// Clear tmpFile so deferred cleanup doesn't try to delete
	tmpFile = nil

	return &PutResult{
		Path:     path,
		Checksum: checksum,
		Size:     written,
	}, nil
}

// Delete removes a file from the filesystem.
func (b *FilesystemBackend) Delete(ctx context.Context, path string) error {
	if b.config.ReadOnly {
		return &BackendError{Backend: b.config.Name, Op: "delete", Path: path, Err: fmt.Errorf("backend is read-only")}
	}

	fullPath := b.fullPath(path)

	// Check context
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := os.Remove(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &BackendError{Backend: b.config.Name, Op: "delete", Path: path, Err: errNotFound{}}
		}
		return &BackendError{Backend: b.config.Name, Op: "delete", Path: path, Err: err}
	}

	return nil
}

// Exists checks if a file exists.
func (b *FilesystemBackend) Exists(ctx context.Context, path string) (bool, error) {
	fullPath := b.fullPath(path)

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, &BackendError{Backend: b.config.Name, Op: "exists", Path: path, Err: err}
	}
	return true, nil
}

// Stat returns file metadata.
func (b *FilesystemBackend) Stat(ctx context.Context, path string) (*FileInfo, error) {
	fullPath := b.fullPath(path)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &BackendError{Backend: b.config.Name, Op: "stat", Path: path, Err: errNotFound{}}
		}
		return nil, &BackendError{Backend: b.config.Name, Op: "stat", Path: path, Err: err}
	}

	var checksum string
	if !info.IsDir() {
		file, err := os.Open(fullPath)
		if err == nil {
			checksum, _ = calculateFileChecksum(file)
			file.Close()
		}
	}

	return &FileInfo{
		Path:         path,
		Name:         filepath.Base(path),
		Size:         info.Size(),
		Checksum:     checksum,
		ContentType:  detectContentType(path),
		ModifiedTime: info.ModTime(),
		IsDirectory:  info.IsDir(),
	}, nil
}

// List returns files matching the path pattern.
func (b *FilesystemBackend) List(ctx context.Context, path string, opts *ListOptions) (*ListResult, error) {
	// Handle glob patterns
	isGlob := strings.ContainsAny(path, "*?[]")
	var searchPath string
	var pattern string

	if isGlob {
		// Find the non-glob prefix
		idx := strings.IndexAny(path, "*?[]")
		prefix := path[:idx]
		lastSlash := strings.LastIndex(prefix, "/")
		if lastSlash >= 0 {
			searchPath = b.fullPath(prefix[:lastSlash+1])
		} else {
			searchPath = b.config.Root
		}
		pattern = path
	} else {
		searchPath = b.fullPath(path)
		pattern = ""
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Check if path exists
	info, err := os.Stat(searchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ListResult{Files: []FileInfo{}}, nil
		}
		return nil, &BackendError{Backend: b.config.Name, Op: "list", Path: path, Err: err}
	}

	var files []FileInfo

	if info.IsDir() {
		// List directory contents
		err = b.walkDir(ctx, searchPath, path, pattern, opts, &files)
		if err != nil {
			return nil, err
		}
	} else {
		// Single file
		checksum := ""
		if opts != nil && opts.IncludeMetadata {
			file, err := os.Open(searchPath)
			if err == nil {
				checksum, _ = calculateFileChecksum(file)
				file.Close()
			}
		}

		files = append(files, FileInfo{
			Path:         path,
			Name:         filepath.Base(path),
			Size:         info.Size(),
			Checksum:     checksum,
			ContentType:  detectContentType(path),
			ModifiedTime: info.ModTime(),
		})
	}

	// Sort by path
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	// Apply pagination
	totalCount := len(files)
	offset := 0
	limit := len(files)

	if opts != nil {
		if opts.Offset > 0 {
			offset = opts.Offset
		}
		if opts.Limit > 0 {
			limit = opts.Limit
		}
	}

	if offset >= len(files) {
		files = []FileInfo{}
	} else {
		end := offset + limit
		if end > len(files) {
			end = len(files)
		}
		files = files[offset:end]
	}

	return &ListResult{
		Files:      files,
		TotalCount: totalCount,
		Truncated:  offset+len(files) < totalCount,
		NextOffset: offset + len(files),
	}, nil
}

// walkDir recursively walks a directory and collects file info.
func (b *FilesystemBackend) walkDir(ctx context.Context, fsPath, logicalPath, pattern string, opts *ListOptions, files *[]FileInfo) error {
	entries, err := os.ReadDir(fsPath)
	if err != nil {
		return &BackendError{Backend: b.config.Name, Op: "list", Path: logicalPath, Err: err}
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue // Skip hidden files
		}

		childFsPath := filepath.Join(fsPath, name)
		childLogicalPath := logicalPath
		if childLogicalPath == "/" {
			childLogicalPath = "/" + name
		} else {
			childLogicalPath = logicalPath + "/" + name
		}

		// Check pattern match
		if pattern != "" && !matchGlob(pattern, childLogicalPath) {
			if entry.IsDir() && (opts == nil || opts.Recursive) {
				// Might have matching files in subdirectory
				if err := b.walkDir(ctx, childFsPath, childLogicalPath, pattern, opts, files); err != nil {
					return err
				}
			}
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue // Skip files we can't stat
		}

		if entry.IsDir() {
			*files = append(*files, FileInfo{
				Path:         childLogicalPath,
				Name:         name,
				ModifiedTime: info.ModTime(),
				IsDirectory:  true,
			})

			if opts != nil && opts.Recursive {
				if err := b.walkDir(ctx, childFsPath, childLogicalPath, pattern, opts, files); err != nil {
					return err
				}
			}
		} else {
			checksum := ""
			if opts != nil && opts.IncludeMetadata {
				file, err := os.Open(childFsPath)
				if err == nil {
					checksum, _ = calculateFileChecksum(file)
					file.Close()
				}
			}

			*files = append(*files, FileInfo{
				Path:         childLogicalPath,
				Name:         name,
				Size:         info.Size(),
				Checksum:     checksum,
				ContentType:  detectContentType(name),
				ModifiedTime: info.ModTime(),
			})
		}
	}

	return nil
}

// Health checks if the backend is healthy.
func (b *FilesystemBackend) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check root directory is accessible
	_, err := os.Stat(b.config.Root)
	if err != nil {
		return &BackendError{Backend: b.config.Name, Op: "health", Err: err}
	}

	return nil
}

// Close releases any resources.
func (b *FilesystemBackend) Close() error {
	return nil
}

// fullPath converts a logical path to a filesystem path.
func (b *FilesystemBackend) fullPath(path string) string {
	// Remove leading slash and join with root
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	return filepath.Join(b.config.Root, path)
}

// calculateFileChecksum calculates the SHA-256 checksum of a file.
func calculateFileChecksum(file *os.File) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// detectContentType detects the content type from file extension.
func detectContentType(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return "application/octet-stream"
	}

	// Common extensions
	switch strings.ToLower(ext) {
	case ".yaml", ".yml":
		return "application/x-yaml"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".txt":
		return "text/plain"
	case ".sh":
		return "application/x-sh"
	case ".deb":
		return "application/vnd.debian.binary-package"
	case ".rpm":
		return "application/x-rpm"
	case ".tar":
		return "application/x-tar"
	case ".gz", ".tgz":
		return "application/gzip"
	case ".zip":
		return "application/zip"
	case ".exe":
		return "application/x-msdownload"
	case ".msi":
		return "application/x-msi"
	}

	// Use mime package for others
	contentType := mime.TypeByExtension(ext)
	if contentType != "" {
		return contentType
	}

	return "application/octet-stream"
}

// limitedReadCloser wraps a ReadCloser with a read limit.
type limitedReadCloser struct {
	rc    io.ReadCloser
	limit int64
	read  int64
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	if l.read >= l.limit {
		return 0, io.EOF
	}
	remaining := l.limit - l.read
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := l.rc.Read(p)
	l.read += int64(n)
	return n, err
}

func (l *limitedReadCloser) Close() error {
	return l.rc.Close()
}
