package capability

import (
	"context"
	"fmt"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// secretScope wraps the shared pathScope but returns the
// secret-specific sentinel.
type secretScope struct{ sc *pathScope }

func newSecretScope(paths []string) (*secretScope, error) {
	sc, err := newPathScope(paths, nil)
	if err != nil {
		return nil, err
	}
	return &secretScope{sc: sc}, nil
}

func (s *secretScope) check(p string) error {
	if err := s.sc.check(p); err != nil {
		return fmt.Errorf("%w: %q", ErrSecretPathDenied, p)
	}
	return nil
}

// SecretsRead is the scoped secrets.read capability.
type SecretsRead struct {
	scope *secretScope
	host  SecretsHost
}

// NewSecretsRead builds the capability from its config + host.
func NewSecretsRead(cfg manifest.CapabilityConfig, host SecretsHost) (*SecretsRead, error) {
	sc, err := newSecretScope(cfg.SecretPaths)
	if err != nil {
		return nil, err
	}
	return &SecretsRead{scope: sc, host: host}, nil
}

// Get fetches the secret at path after enforcing secret-path scope.
func (c *SecretsRead) Get(ctx context.Context, path string) (map[string]any, error) {
	if c.host == nil {
		return nil, fmt.Errorf("secrets.read: %w", ErrHostUnavailable)
	}
	if err := c.scope.check(path); err != nil {
		return nil, err
	}
	return c.host.Get(ctx, path)
}

// SecretsWrite is the scoped secrets.write capability.
type SecretsWrite struct {
	scope *secretScope
	host  SecretsHost
}

// NewSecretsWrite builds the capability from its config + host.
func NewSecretsWrite(cfg manifest.CapabilityConfig, host SecretsHost) (*SecretsWrite, error) {
	sc, err := newSecretScope(cfg.SecretPaths)
	if err != nil {
		return nil, err
	}
	return &SecretsWrite{scope: sc, host: host}, nil
}

// Set writes the secret at path after enforcing secret-path scope.
func (c *SecretsWrite) Set(ctx context.Context, path string, data map[string]any) error {
	if c.host == nil {
		return fmt.Errorf("secrets.write: %w", ErrHostUnavailable)
	}
	if err := c.scope.check(path); err != nil {
		return err
	}
	return c.host.Set(ctx, path, data)
}
