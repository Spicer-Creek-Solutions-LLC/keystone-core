// Package files implements the kscore-files CLI (Epic 18 task 15).
// Subcommands wrap [transport.Client] over a NATS connection so
// operators can read + write files in the kscore file service
// from the command line.
//
//	kscore-files put <local> <remote>     upload via NATS chunks
//	kscore-files get <remote> <local>     download + verify hash
//	kscore-files delete <remote>          remove
//	kscore-files list [prefix]            enumerate
//	kscore-files stat <remote>            metadata only
//
// Remote paths accept both bare slash-delimited paths
// (`configs/app.yaml`) and the `kv://` URI form
// (`kv://configs/app.yaml`); both resolve to the same backend
// path. The `kv://` form makes intent visible in scripts that
// also pass local filesystem paths.
package files

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// globals holds shared flag values that every subcommand reads.
type globals struct {
	natsURL string
	cluster string

	principalID   string
	principalRole string

	timeout time.Duration

	logLevel string
	logger   *slog.Logger
}

// principal builds an [*auth.Principal] from the supplied flags.
// Returns nil if neither flag was set — the transport client
// then sends no identity headers and the server's ACL sees a
// nil principal.
func (g *globals) principal() (*auth.Principal, error) {
	if g.principalID == "" && g.principalRole == "" {
		return nil, nil
	}
	role, err := auth.ParseRole(g.principalRole)
	if err != nil {
		return nil, err
	}
	return &auth.Principal{ID: g.principalID, Role: role}, nil
}

// NewFilesCommand returns the kscore-files cobra root with every
// subcommand registered. The binary entry point in
// cmd/kscore-files/main.go calls this and Execute.
func NewFilesCommand() *cobra.Command {
	g := &globals{logger: slog.Default()}

	root := &cobra.Command{
		Use:   "kscore-files",
		Short: "Operator CLI for the kscore file service",
		Long: `kscore-files reads and writes files in the kscore
file service over NATS. Remote paths accept both bare paths
(configs/app.yaml) and kv:// URIs (kv://configs/app.yaml).

Connection defaults come from env vars when flags are absent:
KSCORE_NATS_URL, KSCORE_CLUSTER, KSCORE_PRINCIPAL_ID,
KSCORE_PRINCIPAL_ROLE.`,
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			g.logger = newLogger(g.logLevel)
			applyEnvDefaults(g)
			if g.natsURL == "" {
				return errors.New("nats-url is required (flag --nats-url or env KSCORE_NATS_URL)")
			}
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.StringVar(&g.natsURL, "nats-url", "", "NATS server URL (default $KSCORE_NATS_URL, else nats://localhost:4222)")
	pf.StringVar(&g.cluster, "cluster", "", "Cluster name for subject scoping (default $KSCORE_CLUSTER, else default)")
	pf.StringVar(&g.principalID, "principal-id", "", "Caller identity emitted in NATS headers (default $KSCORE_PRINCIPAL_ID)")
	pf.StringVar(&g.principalRole, "principal-role", "", "Caller role: admin|operator|readonly (default $KSCORE_PRINCIPAL_ROLE)")
	pf.DurationVar(&g.timeout, "timeout", 60*time.Second, "Per-request response timeout")
	pf.StringVar(&g.logLevel, "log-level", "info", "Log level: debug, info, warn, error")

	root.AddCommand(putCmd(g))
	root.AddCommand(getCmd(g))
	root.AddCommand(deleteCmd(g))
	root.AddCommand(listCmd(g))
	root.AddCommand(statCmd(g))
	return root
}

// applyEnvDefaults backfills unset flags from env vars. Called
// during PersistentPreRunE so it sees the parsed flag state.
// Flag values win when present; the empty string is treated as
// "unset" — operators who want to deliberately clear an env-
// supplied default can do so via an explicit empty assignment
// (the env var overrides only the default-empty case).
func applyEnvDefaults(g *globals) {
	if g.natsURL == "" {
		if v := os.Getenv("KSCORE_NATS_URL"); v != "" {
			g.natsURL = v
		} else {
			g.natsURL = "nats://localhost:4222"
		}
	}
	if g.cluster == "" {
		if v := os.Getenv("KSCORE_CLUSTER"); v != "" {
			g.cluster = v
		} else {
			g.cluster = "default"
		}
	}
	if g.principalID == "" {
		g.principalID = os.Getenv("KSCORE_PRINCIPAL_ID")
	}
	if g.principalRole == "" {
		g.principalRole = os.Getenv("KSCORE_PRINCIPAL_ROLE")
	}
}

// newLogger builds a slog.Logger filtered to the requested level.
// Unknown levels degrade gracefully to info.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
