// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"fmt"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// BuildCapabilities constructs the scoped backend for every
// capability the (validated) manifest requested, wired to hosts.
// The returned map is keyed by capability name; each value is the
// concrete scoped type (*FSRead, *FSWrite, *HTTPCap, *Exec,
// *SecretsRead, *SecretsWrite, *KV, *Log). The task-10 loader hands
// this to the runtime; task-12 exposes them as Starlark builtins.
//
// A construction error (bad glob / size / rate / duration in the
// manifest) fails the whole build — a module with a malformed
// capability scope must not load.
func BuildCapabilities(m *manifest.Manifest, hosts Hosts) (map[string]any, error) {
	if m == nil {
		return nil, fmt.Errorf("capability: nil manifest")
	}
	out := make(map[string]any, len(m.Capabilities))
	for name, cfg := range m.Capabilities {
		var (
			c   any
			err error
		)
		switch name {
		case manifest.CapFSRead:
			c, err = NewFSRead(cfg, hosts.FS)
		case manifest.CapFSWrite:
			c, err = NewFSWrite(cfg, hosts.FS)
		case manifest.CapHTTPGet:
			c, err = NewHTTPGet(cfg, hosts.HTTP)
		case manifest.CapHTTPPost:
			c, err = NewHTTPPost(cfg, hosts.HTTP)
		case manifest.CapExec:
			c, err = NewExec(cfg, hosts.Exec)
		case manifest.CapSecretsRead:
			c, err = NewSecretsRead(cfg, hosts.Secrets)
		case manifest.CapSecretsWrite:
			c, err = NewSecretsWrite(cfg, hosts.Secrets)
		case manifest.CapKV:
			c, err = NewKV(cfg)
		case manifest.CapLog:
			c, err = NewLog(cfg, hosts.Logger)
		default:
			return nil, fmt.Errorf("%w: %q", ErrUnknownCapability, name)
		}
		if err != nil {
			return nil, fmt.Errorf("capability %q: %w", name, err)
		}
		out[name] = c
	}
	return out, nil
}
