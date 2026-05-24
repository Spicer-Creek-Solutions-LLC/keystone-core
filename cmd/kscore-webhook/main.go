// SPDX-License-Identifier: Apache-2.0

// kscore-webhook is the Keystone Core webhook CLI (Epic 16 task 16).
// `outbound` subcommands (list/create/show/delete/history/test)
// manage subscriptions stored in a local SQLite at --store. Reachable
// as `kscorectl webhook ...` via the Epic-14 plugin dispatch.
package main

import (
	"io"
	"os"

	cliwh "go.keystone-core.io/keystone-core/internal/cli/webhook"
)

func run(args []string, stdout, stderr io.Writer) int {
	cmd := cliwh.NewCommand(cliwh.Deps{})
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		_, _ = io.WriteString(stderr, "error: "+err.Error()+"\n")
		return 1
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
