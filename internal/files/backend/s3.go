package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/s3client"
)

// S3Store persists files in an S3-compatible bucket using the same
// data/meta split as [FilesystemStore]:
//
//	<prefix>/data/<path>           file body
//	<prefix>/meta/<path>.json      marshalled [files.FileMetadata]
//
// A per-store mutex serialises Put so the next-version assignment
// is race-free in process. Cross-process concurrent writers are
// outside v1.0 scope (the file service is a single writer).
type S3Store struct {
	bucket string
	prefix string
	cfg    s3client.Config
	mu     sync.Mutex
	now    func() time.Time
}

const (
	s3DataPrefix = "data/"
	s3MetaPrefix = "meta/"
	s3MetaSuffix = ".json"
)

// NewS3Store returns an [S3Store] writing under bucket + prefix.
// prefix may be empty (bucket root) or a slash-terminated prefix
// such as "files/" — the constructor normalises by trimming
// leading slashes and adding a trailing slash if missing.
// nowFunc lets tests inject deterministic timestamps.
func NewS3Store(bucket, prefix string, cfg s3client.Config, nowFunc func() time.Time) (*S3Store, error) {
	if bucket == "" {
		return nil, errors.New("backend: s3 bucket must not be empty")
	}
	if nowFunc == nil {
		nowFunc = time.Now
	}
	prefix = strings.TrimPrefix(prefix, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &S3Store{
		bucket: bucket,
		prefix: prefix,
		cfg:    cfg,
		now:    nowFunc,
	}, nil
}

func (s *S3Store) Put(ctx context.Context, meta files.FileMetadata, body io.Reader) (files.FileMetadata, error) {
	if err := validatePath(meta.Path); err != nil {
		return files.FileMetadata{}, err
	}
	// Buffer the body so we can compute size + hash before the PUT
	// and use the single-PUT minio-go path. v1.0 files fit in
	// memory (configs / blueprints); a streaming variant is a v1.x
	// scaling concern.
	buf, err := io.ReadAll(body)
	if err != nil {
		return files.FileMetadata{}, fmt.Errorf("backend: read body: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	client, err := s3client.NewClient(s.cfg)
	if err != nil {
		return files.FileMetadata{}, err
	}

	var nextVersion int64 = 1
	if prev, err := s.statWithClient(ctx, client, meta.Path); err == nil {
		nextVersion = prev.Version + 1
	} else if !errors.Is(err, ErrNotFound) {
		return files.FileMetadata{}, err
	}

	dataKey := s.dataKey(meta.Path)
	metaKey := s.metaKey(meta.Path)

	final := files.FileMetadata{
		Path:        meta.Path,
		Size:        int64(len(buf)),
		Hash:        files.HashOf(buf),
		ContentType: meta.ContentType,
		CreatedAt:   s.now().UTC(),
		Version:     nextVersion,
		Tags:        maps.Clone(meta.Tags),
	}

	ct := final.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	if _, err := client.PutObject(ctx, s.bucket, dataKey, bytes.NewReader(buf), int64(len(buf)), minio.PutObjectOptions{
		ContentType: ct,
	}); err != nil {
		return files.FileMetadata{}, fmt.Errorf("backend: s3 put body: %w", err)
	}

	metaBytes, err := json.MarshalIndent(final, "", "  ")
	if err != nil {
		return files.FileMetadata{}, fmt.Errorf("backend: marshal meta: %w", err)
	}
	if _, err := client.PutObject(ctx, s.bucket, metaKey, bytes.NewReader(metaBytes), int64(len(metaBytes)), minio.PutObjectOptions{
		ContentType: "application/json",
	}); err != nil {
		// Best-effort cleanup so an orphan body does not linger.
		_ = client.RemoveObject(ctx, s.bucket, dataKey, minio.RemoveObjectOptions{})
		return files.FileMetadata{}, fmt.Errorf("backend: s3 put meta: %w", err)
	}
	return final, nil
}

func (s *S3Store) Get(ctx context.Context, path string) (files.FileMetadata, io.ReadCloser, error) {
	if err := validatePath(path); err != nil {
		return files.FileMetadata{}, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	client, err := s3client.NewClient(s.cfg)
	if err != nil {
		return files.FileMetadata{}, nil, err
	}
	meta, err := s.statWithClient(ctx, client, path)
	if err != nil {
		return files.FileMetadata{}, nil, err
	}
	obj, err := client.GetObject(ctx, s.bucket, s.dataKey(path), minio.GetObjectOptions{})
	if err != nil {
		return files.FileMetadata{}, nil, fmt.Errorf("backend: s3 get body: %w", err)
	}
	// minio-go's *minio.Object defers the HTTP call until first
	// Read or Stat; trigger Stat here so a missing body surfaces
	// synchronously alongside the meta check.
	if _, statErr := obj.Stat(); statErr != nil {
		_ = obj.Close()
		if isS3NotFound(statErr) {
			return files.FileMetadata{}, nil, ErrNotFound
		}
		return files.FileMetadata{}, nil, fmt.Errorf("backend: s3 stat body: %w", statErr)
	}
	return meta, obj, nil
}

func (s *S3Store) Stat(ctx context.Context, path string) (files.FileMetadata, error) {
	if err := validatePath(path); err != nil {
		return files.FileMetadata{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	client, err := s3client.NewClient(s.cfg)
	if err != nil {
		return files.FileMetadata{}, err
	}
	return s.statWithClient(ctx, client, path)
}

func (s *S3Store) List(ctx context.Context, prefix string) ([]files.FileMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	client, err := s3client.NewClient(s.cfg)
	if err != nil {
		return nil, err
	}
	listPrefix := s.prefix + s3MetaPrefix + prefix
	out := []files.FileMetadata{}
	for obj := range client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    listPrefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("backend: s3 list: %w", obj.Err)
		}
		if !strings.HasSuffix(obj.Key, s3MetaSuffix) {
			continue
		}
		m, err := s.fetchMeta(ctx, client, obj.Key)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (s *S3Store) Delete(ctx context.Context, path string) error {
	if err := validatePath(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	client, err := s3client.NewClient(s.cfg)
	if err != nil {
		return err
	}
	// Verify presence first so the contract surfaces ErrNotFound;
	// S3 DELETE is otherwise silently idempotent.
	if _, err := s.statWithClient(ctx, client, path); err != nil {
		return err
	}
	if err := client.RemoveObject(ctx, s.bucket, s.metaKey(path), minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("backend: s3 remove meta: %w", err)
	}
	if err := client.RemoveObject(ctx, s.bucket, s.dataKey(path), minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("backend: s3 remove body: %w", err)
	}
	return nil
}

// statWithClient reads + unmarshals meta with the supplied client.
// It is the building block for both [Stat] and the next-version
// lookup inside [Put].
func (s *S3Store) statWithClient(ctx context.Context, client *minio.Client, path string) (files.FileMetadata, error) {
	return s.fetchMeta(ctx, client, s.metaKey(path))
}

// fetchMeta retrieves a meta object by its raw key.
func (s *S3Store) fetchMeta(ctx context.Context, client *minio.Client, key string) (files.FileMetadata, error) {
	obj, err := client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return files.FileMetadata{}, fmt.Errorf("backend: s3 get meta: %w", err)
	}
	defer func() { _ = obj.Close() }()
	if _, statErr := obj.Stat(); statErr != nil {
		if isS3NotFound(statErr) {
			return files.FileMetadata{}, ErrNotFound
		}
		return files.FileMetadata{}, fmt.Errorf("backend: s3 stat meta: %w", statErr)
	}
	b, err := io.ReadAll(obj)
	if err != nil {
		return files.FileMetadata{}, fmt.Errorf("backend: read meta: %w", err)
	}
	var m files.FileMetadata
	if err := json.Unmarshal(b, &m); err != nil {
		return files.FileMetadata{}, fmt.Errorf("backend: parse meta %s: %w", key, err)
	}
	return m, nil
}

func (s *S3Store) dataKey(p string) string {
	return s.prefix + s3DataPrefix + p
}

func (s *S3Store) metaKey(p string) string {
	return s.prefix + s3MetaPrefix + p + s3MetaSuffix
}

// isS3NotFound reports whether err is a 404 from minio-go. The SDK
// surfaces NoSuchKey via [minio.ErrorResponse].StatusCode = 404 or
// Code = "NoSuchKey"; both forms appear depending on the API.
func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	resp := minio.ToErrorResponse(err)
	if resp.StatusCode == 404 {
		return true
	}
	if resp.Code == "NoSuchKey" || resp.Code == "NoSuchBucket" {
		return true
	}
	return false
}
