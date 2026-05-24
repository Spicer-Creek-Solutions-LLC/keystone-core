// SPDX-License-Identifier: Apache-2.0

package dest

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"

	"go.keystone-core.io/keystone-core/internal/s3client"
)

// S3Source reads an artifact from an S3-compatible object store.
type S3Source struct {
	Bucket string
	Key    string
	Config Config
}

// Open returns a reader for the object at Bucket/Key. minio-go's
// *minio.Object is itself an io.ReadCloser; we hand it straight back
// without any pipe-and-goroutine machinery (the read side is much
// simpler than the write side because the API is already
// reader-shaped).
func (s *S3Source) Open(ctx context.Context) (io.ReadCloser, error) {
	client, err := s3client.NewClient(s.Config)
	if err != nil {
		return nil, err
	}
	obj, err := client.GetObject(ctx, s.Bucket, s.Key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("dest: s3 GetObject %s/%s: %w", s.Bucket, s.Key, err)
	}
	return obj, nil
}

// S3Lister enumerates objects under a Bucket/Prefix using
// ListObjectsV2.
type S3Lister struct {
	Bucket string
	Prefix string
	Config Config
}

// List streams the prefix's objects into an in-memory slice. v1.0
// expects backup directories with tens-to-hundreds of artifacts; if
// an operator ever has six-digit object counts a paginated CLI is a
// v1.x concern.
func (l *S3Lister) List(ctx context.Context) ([]Entry, error) {
	client, err := s3client.NewClient(l.Config)
	if err != nil {
		return nil, err
	}
	opts := minio.ListObjectsOptions{
		Prefix:    l.Prefix,
		Recursive: true,
	}
	var out []Entry
	for obj := range client.ListObjects(ctx, l.Bucket, opts) {
		if obj.Err != nil {
			return nil, fmt.Errorf("dest: s3 ListObjects %s: %w", l.Bucket, obj.Err)
		}
		out = append(out, Entry{
			Name:         obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
		})
	}
	return out, nil
}

