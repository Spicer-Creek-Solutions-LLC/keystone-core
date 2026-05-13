// Package stdlib registers the v1.0 base stdlib state modules
// (PROJECT-DETAILS §4.8) into a statemgmt.Registry.
//
// Per-module subpackages live under internal/statemgmt/stdlib/<name>/.
// Each module exports New() statemgmt.Module; this umbrella package
// wires them through RegisterAll so the registration order, error
// handling, and test isolation surface are explicit (no per-module
// init() magic).
//
// Wiring:
//   - cmd/kscore-server calls stdlib.RegisterAll(nil) at boot to
//     populate statemgmt.DefaultRegistry — the StateGRPCServer
//     reads from DefaultRegistry when its own Registry is nil.
//   - Tests that want hermetic registration use
//     stdlib.RegisterAll(statemgmt.NewRegistry()) and pass the
//     fresh Registry to the Detector / Runner / Validator.
//
// Task 11a ships the file module; subsequent 11b/c/d PRs add the
// rest of the ~40 modules per the §11 category list.
package stdlib

import (
	"fmt"
	"sort"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	archivemod "go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/archive"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/at"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/cmd"
	configmod "go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/config"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/cron"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/file"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/firewalld"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/git"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/group"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/hostname"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/iptables"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/kmod"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/link"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/mount"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/nftables"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/pkg"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/pki"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/service"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/ssh"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/swap"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/sysctl"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/timer"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/timezone"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/user"
)

// modules is the canonical name → factory map. Adding a new stdlib
// module is a one-line addition here.
var modules = map[string]statemgmt.Factory{
	"archive":       archivemod.New,
	"at":            at.New,
	"cmd":           cmd.New,
	"config":        configmod.New,
	"cron":          cron.New,
	"file":          file.New,
	"firewalld":     firewalld.New,
	"git":           git.New,
	"group":         group.New,
	"hostname":      hostname.New,
	"iptables":      iptables.New,
	"kernel_module": kmod.New,
	"link":          link.New,
	"mount":         mount.New,
	"nftables":      nftables.New,
	"package":       pkg.New,
	"service":       service.New,
	"ssh":           ssh.New,
	"swap":          swap.New,
	"sysctl":        sysctl.New,
	"systemd_timer": timer.New,
	"timezone":      timezone.New,
	"user":          user.New,
	"x509":          pki.New,
}

// RegisterAll registers every stdlib module into reg. Pass nil to
// register into statemgmt.DefaultRegistry. Re-registering an
// already-registered name returns the underlying
// ErrDuplicateModule — RegisterAll is idempotent across processes
// but NOT within one (it expects a fresh-or-empty target).
func RegisterAll(reg *statemgmt.Registry) error {
	if reg == nil {
		reg = statemgmt.DefaultRegistry
	}
	for name, factory := range modules {
		if err := reg.Register(name, factory); err != nil {
			return fmt.Errorf("stdlib: register %s: %w", name, err)
		}
	}
	return nil
}

// ModuleNames returns the stdlib module names in stable order. Used
// by tests asserting which modules are registered.
func ModuleNames() []string {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
