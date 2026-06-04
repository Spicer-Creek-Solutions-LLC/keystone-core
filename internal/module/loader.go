// SPDX-License-Identifier: Apache-2.0

package module

import (
	"log/slog"

	"go.keystone-core.io/keystone-core/pkg/module/loader"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/runtime/starlark"
	"go.keystone-core.io/keystone-core/pkg/module/runtime/starlark/capbuiltins"
	"go.keystone-core.io/keystone-core/pkg/module/verify"
)

// LoaderOptions configures [BuildLoader].
type LoaderOptions struct {
	// Verifier checks module signatures against the trusted keys. nil
	// is allowed but then every Load must set SkipVerification (the
	// loader otherwise fails closed).
	Verifier *verify.Verifier
	// Logger receives module log-capability output. nil → slog.Default.
	Logger *slog.Logger
}

// BuildLoader assembles the production module loader for local (CLI)
// execution: signature verification against the supplied verifier, the
// real local capability hosts ([LocalHosts]), and the Starlark runtime
// with the capability builtins ([capbuiltins.Provider]) registered for
// manifest.TypeStarlark.
//
// Policy is left nil (allow-all): for the standalone CLI the security
// boundary is signature verification plus the manifest-declared,
// scope-enforced capability layer. The internal/policy PolicyChecker is
// a server-side concern (no policy engine exists in a local CLI).
func BuildLoader(opts LoaderOptions) *loader.ModuleLoader {
	reg := loader.NewRuntimeRegistry()
	reg.Register(manifest.TypeStarlark, starlark.New(starlark.Config{Builtins: capbuiltins.Provider}))

	cfg := loader.Config{
		Hosts:    LocalHosts(opts.Logger),
		Runtimes: reg,
	}
	// Only set Verifier when non-nil: the field is an interface, so a
	// typed-nil *verify.Verifier would read as non-nil and panic on the
	// verify path.
	if opts.Verifier != nil {
		cfg.Verifier = opts.Verifier
	}
	return loader.New(cfg)
}
