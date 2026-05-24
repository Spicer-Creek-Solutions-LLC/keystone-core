// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// pluginExecutor is the slice of Executor Dispatch needs (injected
// for tests).
type pluginExecutor interface {
	Execute(ctx context.Context, p Plugin, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

// cobraBuiltins are never treated as plugin candidates even if a
// like-named binary exists.
var cobraBuiltins = map[string]bool{
	"help": true, "completion": true,
	"__complete": true, "__completeNoDesc": true,
}

// Dispatch implements the Git/kubectl plugin model: `kscorectl
// <name> <args…>` where <name> is the first argument. It delegates
// to a `kscore-<name>` plugin only when <name> is NOT a registered
// subcommand / cobra builtin and IS discoverable; otherwise it
// returns handled=false so cobra runs normally (and emits its own
// "unknown command" error when appropriate).
//
// Plugins receive every argument after <name>; kscorectl global
// flags must follow the plugin name (the standard git-plugin
// convention) — leading flags route to cobra, not a plugin.
func Dispatch(ctx context.Context, root *cobra.Command, args []string,
	d *Discovery, e pluginExecutor, stdin io.Reader, stdout, stderr io.Writer) (handled bool, code int, err error) {

	if len(args) == 0 {
		return false, 0, nil
	}
	name := args[0]
	if name == "" || name[0] == '-' { // a root flag (--help/--version/--config)
		return false, 0, nil
	}
	if cobraBuiltins[name] || isRegistered(root, name) {
		return false, 0, nil
	}
	p, ok := d.Lookup(name)
	if !ok {
		return false, 0, nil // unknown → let cobra report it
	}
	c, eerr := e.Execute(ctx, p, args[1:], stdin, stdout, stderr)
	return true, c, eerr
}

func isRegistered(root *cobra.Command, name string) bool {
	for _, c := range root.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return true
		}
	}
	return false
}
