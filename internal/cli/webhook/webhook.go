// Package webhook implements the `kscore-webhook` CLI (Epic 16 task
// 16). Reachable as `kscorectl webhook ...` via the Epic-14 plugin
// dispatch. Today only the `outbound` subcommand tree is wired —
// inbound non-GitOps webhooks are v1.x per FEATURES.md.
package webhook

import (
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/webhook/outbound"
)

// Deps bundles the production seams the CLI needs. Production main
// constructs them with the real implementations; tests inject fakes
// (a fixed clock, a counting Dispatcher, etc.).
type Deps struct {
	IDGen      func() string
	Now        func() time.Time
	Dispatcher outbound.Dispatcher // nil → CLI builds an HTTPDispatcher with a 30s client
}

// NewCommand returns the root `kscore-webhook` cobra command.
func NewCommand(d Deps) *cobra.Command {
	if d.IDGen == nil {
		d.IDGen = uuid.NewString
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	root := &cobra.Command{
		Use:           "kscore-webhook",
		Short:         "Keystone Core webhook subscriptions CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newOutboundCmd(d))
	cli.AddVersion(root)
	return root
}
