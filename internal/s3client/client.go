// SPDX-License-Identifier: Apache-2.0

package s3client

import (
	"errors"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config is the operator-supplied connection state. Static access-key
// auth is the only mode v1.0 ships; advanced auth (IRSA, instance
// profiles, web-identity) defers under the v1.x ROADMAP entry
// "Backup destinations: ... advanced S3 auth".
type Config struct {
	AccessKey string
	SecretKey string
	Region    string
	Endpoint  string // empty = AWS default endpoint; non-empty = MinIO/B2/etc.
	UseSSL    bool
}

// ErrMissingCredentials is returned by [NewClient] when AccessKey or
// SecretKey is empty. Callers can errors.Is against it to surface a
// friendlier message at the CLI layer.
var ErrMissingCredentials = errors.New("s3client: access key and secret key are required")

// defaultEndpoint is the host used when [Config.Endpoint] is empty.
const defaultEndpoint = "s3.amazonaws.com"

// NewClient builds a minio-go client from cfg. Both AccessKey and
// SecretKey are required — anonymous access has no v1.0 use case and
// silently producing an unauthenticated client would mask
// misconfiguration.
func NewClient(cfg Config) (*minio.Client, error) {
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, ErrMissingCredentials
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3client: minio.New: %w", err)
	}
	return client, nil
}
