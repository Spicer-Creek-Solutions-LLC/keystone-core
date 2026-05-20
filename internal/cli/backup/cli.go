// Package backup implements the kscore-backup CLI (Epic 18 task 7).
// The root command exposes shared S3 + log-level flags and registers
// the verify + list subcommands shipped in task 7a. The create +
// restore subcommands land in task 7b.
package backup

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/backup/dest"
)

// globals holds shared flag values + log handle for every subcommand
// to access. Wired into each subcommand via closure so the cobra
// flag plumbing stays at the root.
type globals struct {
	// S3 connection state. AccessKey + SecretKey default to env
	// vars (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY) per the
	// standard S3 client convention; flags override.
	s3AccessKey string
	s3SecretKey string
	s3Region    string
	s3Endpoint  string
	s3SSL       bool

	logLevel string
	logger   *slog.Logger
}

// destConfig builds a [dest.Config] from the current globals so each
// subcommand can hand it to [dest.Resolve], [dest.ResolveSource], or
// [dest.ResolveLister].
func (g *globals) destConfig() dest.Config {
	return dest.Config{
		AccessKey: g.s3AccessKey,
		SecretKey: g.s3SecretKey,
		Region:    g.s3Region,
		Endpoint:  g.s3Endpoint,
		UseSSL:    g.s3SSL,
	}
}

// NewBackupCommand returns the kscore-backup cobra root with every
// subcommand registered. The binary entry point in
// cmd/kscore-backup/main.go calls this and Execute.
func NewBackupCommand() *cobra.Command {
	g := &globals{logger: slog.Default()}

	root := &cobra.Command{
		Use:   "kscore-backup",
		Short: "kscore-server backup + restore CLI",
		Long: `kscore-backup creates, verifies, lists, and restores
portable kscore-server backup artifacts. Task 7a ships the
read-only verify + list subcommands; create + restore land in
task 7b.`,
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			g.logger = newLogger(g.logLevel)
			// S3 creds default to env vars so operators can run with
			// AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY set without
			// putting credentials on the command line.
			if g.s3AccessKey == "" {
				g.s3AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
			}
			if g.s3SecretKey == "" {
				g.s3SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
			}
			if g.s3Region == "" {
				g.s3Region = os.Getenv("AWS_REGION")
			}
			if g.s3Endpoint == "" {
				g.s3Endpoint = os.Getenv("AWS_ENDPOINT_URL")
			}
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.StringVar(&g.s3AccessKey, "s3-access-key", "", "S3 access key (default $AWS_ACCESS_KEY_ID)")
	pf.StringVar(&g.s3SecretKey, "s3-secret-key", "", "S3 secret key (default $AWS_SECRET_ACCESS_KEY)")
	pf.StringVar(&g.s3Region, "s3-region", "", "S3 region (default $AWS_REGION)")
	pf.StringVar(&g.s3Endpoint, "s3-endpoint", "", "S3 endpoint (default $AWS_ENDPOINT_URL; empty = AWS)")
	pf.BoolVar(&g.s3SSL, "s3-ssl", true, "Use HTTPS for the S3 endpoint")
	pf.StringVar(&g.logLevel, "log-level", "info", "Log level: debug, info, warn, error")

	root.AddCommand(verifyCmd(g))
	root.AddCommand(listCmd(g))
	return root
}

// newLogger builds a slog.Logger filtered to the requested level.
// Unknown levels degrade gracefully to info — better to log
// "ratelimit" with a default level than to fail the whole command.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
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
