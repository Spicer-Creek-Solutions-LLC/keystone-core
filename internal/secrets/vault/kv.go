// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	vaultapi "github.com/hashicorp/vault/api"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// splitMountAndKey splits a Vault path into (mount, key). The mount
// is the first path segment; the key is everything after. Examples:
//
//	"secret/app/db"   → ("secret",    "app/db")
//	"kv-legacy/x"     → ("kv-legacy", "x")
//	"justmount"       → ("justmount", "")
//
// Returns an error wrapping [secrets.ErrInvalidBackend] for empty input.
func splitMountAndKey(path string) (string, string, error) {
	if path == "" {
		return "", "", errInvalid("kv: path is required")
	}
	idx := strings.IndexByte(path, '/')
	if idx < 0 {
		return path, "", nil
	}
	return path[:idx], path[idx+1:], nil
}

// kvGet dispatches the read by KV engine version.
func (b *Backend) kvGet(ctx context.Context, req secrets.GetSecretRequest) (*secrets.Secret, error) {
	mount, key, err := splitMountAndKey(req.Path)
	if err != nil {
		return nil, err
	}
	version := b.cfg.resolveKVVersion(mount)
	switch version {
	case 1:
		return b.kvGetV1(ctx, mount, key, req.Path)
	case 2:
		return b.kvGetV2(ctx, mount, key, req)
	default:
		return nil, errInvalid(fmt.Sprintf("kv: unsupported version %d for mount %q", version, mount))
	}
}

func (b *Backend) kvGetV1(ctx context.Context, mount, key, fullPath string) (*secrets.Secret, error) {
	secret, err := b.client.KVv1(mount).Get(ctx, key)
	if err != nil {
		return nil, translateError("kv-v1 get", fullPath, err)
	}
	if secret == nil {
		return nil, fmt.Errorf("%w: %q", secrets.ErrSecretNotFound, fullPath)
	}
	return &secrets.Secret{
		Path: fullPath,
		Data: secret.Data,
	}, nil
}

func (b *Backend) kvGetV2(ctx context.Context, mount, key string, req secrets.GetSecretRequest) (*secrets.Secret, error) {
	kv := b.client.KVv2(mount)
	var secret *vaultapi.KVSecret
	var err error
	if req.Version != 0 {
		secret, err = kv.GetVersion(ctx, key, int(req.Version))
	} else {
		secret, err = kv.Get(ctx, key)
	}
	if err != nil {
		if errors.Is(err, vaultapi.ErrSecretNotFound) {
			return nil, fmt.Errorf("%w: %q", secrets.ErrSecretNotFound, req.Path)
		}
		return nil, translateError("kv-v2 get", req.Path, err)
	}
	if secret == nil {
		return nil, fmt.Errorf("%w: %q", secrets.ErrSecretNotFound, req.Path)
	}

	out := &secrets.Secret{
		Path: req.Path,
		Data: secret.Data,
	}
	if secret.VersionMetadata != nil {
		out.Version = uint64(secret.VersionMetadata.Version) // #nosec G115 -- Vault versions are small positive ints
		out.CreatedAt = secret.VersionMetadata.CreatedTime
	}
	if secret.CustomMetadata != nil {
		out.Metadata = make(map[string]string, len(secret.CustomMetadata))
		for k, v := range secret.CustomMetadata {
			if s, ok := v.(string); ok {
				out.Metadata[k] = s
			} else {
				out.Metadata[k] = fmt.Sprintf("%v", v)
			}
		}
	}
	return out, nil
}

// kvWrite dispatches the write by engine version.
func (b *Backend) kvWrite(ctx context.Context, req secrets.WriteSecretRequest) (*secrets.Secret, error) {
	mount, key, err := splitMountAndKey(req.Path)
	if err != nil {
		return nil, err
	}
	version := b.cfg.resolveKVVersion(mount)
	switch version {
	case 1:
		return b.kvWriteV1(ctx, mount, key, req)
	case 2:
		return b.kvWriteV2(ctx, mount, key, req)
	default:
		return nil, errInvalid(fmt.Sprintf("kv: unsupported version %d for mount %q", version, mount))
	}
}

func (b *Backend) kvWriteV1(ctx context.Context, mount, key string, req secrets.WriteSecretRequest) (*secrets.Secret, error) {
	if req.CAS != nil {
		// KV v1 has no version concept; honoring CAS would be a lie.
		// Reject loudly so callers don't silently get unsafe writes.
		return nil, errInvalid(fmt.Sprintf("kv: CAS is not supported on KV v1 mount %q (use a KV v2 mount or omit CAS)", mount))
	}
	if err := b.client.KVv1(mount).Put(ctx, key, req.Data); err != nil {
		return nil, translateError("kv-v1 put", req.Path, err)
	}
	return &secrets.Secret{
		Path: req.Path,
		Data: req.Data,
	}, nil
}

func (b *Backend) kvWriteV2(ctx context.Context, mount, key string, req secrets.WriteSecretRequest) (*secrets.Secret, error) {
	opts := []vaultapi.KVOption{}
	if req.CAS != nil {
		opts = append(opts, vaultapi.WithCheckAndSet(int(*req.CAS))) // #nosec G115 -- caller-supplied version; Vault validates
	}
	secret, err := b.client.KVv2(mount).Put(ctx, key, req.Data, opts...)
	if err != nil {
		return nil, translateError("kv-v2 put", req.Path, err)
	}

	out := &secrets.Secret{
		Path: req.Path,
		Data: req.Data,
	}
	if secret != nil && secret.VersionMetadata != nil {
		out.Version = uint64(secret.VersionMetadata.Version) // #nosec G115 -- Vault versions are small positive ints
		out.CreatedAt = secret.VersionMetadata.CreatedTime
		out.UpdatedAt = secret.VersionMetadata.CreatedTime
	} else {
		out.UpdatedAt = time.Now().UTC()
	}
	if len(req.Metadata) > 0 {
		// Vault's KV v2 stores metadata separately. Best-effort:
		// silently degrade if the API rejects (older Vault servers).
		md := make(map[string]any, len(req.Metadata))
		for k, v := range req.Metadata {
			md[k] = v
		}
		if err := b.client.KVv2(mount).PutMetadata(ctx, key, vaultapi.KVMetadataPutInput{
			CustomMetadata: md,
		}); err != nil {
			b.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "vault: kv-v2 PutMetadata failed (continuing)",
				slog.String("err", err.Error()),
				slog.String("path", req.Path),
			)
		}
		out.Metadata = cloneStringMapForBackend(req.Metadata)
	}
	return out, nil
}

// kvList dispatches by engine version.
func (b *Backend) kvList(ctx context.Context, req secrets.ListSecretsRequest) (*secrets.ListSecretsResponse, error) {
	mount, key, err := splitMountAndKey(req.Prefix)
	if err != nil {
		return nil, err
	}
	version := b.cfg.resolveKVVersion(mount)
	switch version {
	case 1:
		return b.kvListV1(ctx, mount, key, req)
	case 2:
		return b.kvListV2(ctx, mount, key, req)
	default:
		return nil, errInvalid(fmt.Sprintf("kv: unsupported version %d for mount %q", version, mount))
	}
}

func (b *Backend) kvListV1(ctx context.Context, mount, key string, req secrets.ListSecretsRequest) (*secrets.ListSecretsResponse, error) {
	// vault/api's KVv1 doesn't expose List directly — use the
	// Logical client's List against `<mount>/<key>`.
	listPath := mount
	if key != "" {
		listPath = mount + "/" + key
	}
	secret, err := b.client.Logical().ListWithContext(ctx, listPath)
	if err != nil {
		return nil, translateError("kv-v1 list", req.Prefix, err)
	}
	return buildListResponse(secret, req)
}

func (b *Backend) kvListV2(ctx context.Context, mount, key string, req secrets.ListSecretsRequest) (*secrets.ListSecretsResponse, error) {
	// KV v2 LIST uses the metadata API endpoint:
	// `<mount>/metadata/<key>`.
	listPath := mount + "/metadata"
	if key != "" {
		listPath = listPath + "/" + key
	}
	secret, err := b.client.Logical().ListWithContext(ctx, listPath)
	if err != nil {
		return nil, translateError("kv-v2 list", req.Prefix, err)
	}
	return buildListResponse(secret, req)
}

// buildListResponse projects Vault's `keys` listing into the
// metadata-only [secrets.ListSecretsResponse] shape with cursor-based
// pagination. Vault returns a flat list of keys under the prefix; we
// sort + slice.
func buildListResponse(secret *vaultapi.Secret, req secrets.ListSecretsRequest) (*secrets.ListSecretsResponse, error) {
	out := &secrets.ListSecretsResponse{}
	if secret == nil || secret.Data == nil {
		return out, nil
	}
	rawKeys, ok := secret.Data["keys"]
	if !ok {
		return out, nil
	}
	keys, ok := rawKeys.([]any)
	if !ok {
		return out, nil
	}

	entries := make([]string, 0, len(keys))
	for _, k := range keys {
		if s, ok := k.(string); ok {
			entries = append(entries, s)
		}
	}
	sortStrings(entries)

	if req.Cursor != "" {
		idx := searchStrings(entries, req.Cursor)
		if idx < len(entries) && entries[idx] == req.Cursor {
			idx++
		}
		entries = entries[idx:]
	}

	for _, name := range entries {
		if req.Limit > 0 && len(out.Entries) >= req.Limit {
			out.NextCursor = out.Entries[len(out.Entries)-1].Path
			break
		}
		out.Entries = append(out.Entries, secrets.ListEntry{
			// Vault returns just the leaf name; rejoin to a full
			// path so callers can use it directly with GetSecret.
			Path: rejoinListPath(req.Prefix, name),
		})
	}
	return out, nil
}

// rejoinListPath builds `<prefix>/<name>` (without a leading slash
// when prefix is empty). Vault list keys for "dir/" come back as
// child entries; we preserve a trailing `/` for directory-style
// children so callers can keep listing into them.
func rejoinListPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	if strings.HasSuffix(prefix, "/") {
		return prefix + name
	}
	return prefix + "/" + name
}

// kvDelete dispatches by engine version.
func (b *Backend) kvDelete(ctx context.Context, req secrets.DeleteSecretRequest) error {
	mount, key, err := splitMountAndKey(req.Path)
	if err != nil {
		return err
	}
	version := b.cfg.resolveKVVersion(mount)
	switch version {
	case 1:
		if err := b.client.KVv1(mount).Delete(ctx, key); err != nil {
			return translateError("kv-v1 delete", req.Path, err)
		}
		return nil
	case 2:
		kv := b.client.KVv2(mount)
		if req.Destroy {
			if req.Version != 0 {
				if err := kv.Destroy(ctx, key, []int{int(req.Version)}); err != nil { // #nosec G115 -- Vault versions are small positive ints
					return translateError("kv-v2 destroy", req.Path, err)
				}
				return nil
			}
			if err := kv.DeleteMetadata(ctx, key); err != nil {
				return translateError("kv-v2 delete-metadata", req.Path, err)
			}
			return nil
		}
		if req.Version != 0 {
			if err := kv.DeleteVersions(ctx, key, []int{int(req.Version)}); err != nil { // #nosec G115 -- Vault versions are small positive ints
				return translateError("kv-v2 delete-versions", req.Path, err)
			}
			return nil
		}
		if err := kv.Delete(ctx, key); err != nil {
			return translateError("kv-v2 delete", req.Path, err)
		}
		return nil
	default:
		return errInvalid(fmt.Sprintf("kv: unsupported version %d for mount %q", version, mount))
	}
}

// Local helpers — duplicates of small string utilities so the package
// stays self-contained and doesn't widen imports for the broker side.

func sortStrings(in []string) {
	// O(n²) on tiny lists is fine; vault list pages are bounded.
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j-1] > in[j]; j-- {
			in[j-1], in[j] = in[j], in[j-1]
		}
	}
}

func searchStrings(in []string, target string) int {
	lo, hi := 0, len(in)
	for lo < hi {
		mid := (lo + hi) / 2
		if in[mid] < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func cloneStringMapForBackend(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
