// SPDX-License-Identifier: Apache-2.0

// kscore-agent is the Keystone Core agent daemon.
//
// Default invocation runs the daemon: connects to the cluster's
// NATS, registers, runs heartbeat + metadata loops, executes
// commands, exits cleanly on SIGTERM/SIGINT.
//
// Subcommands:
//   - bootstrap — interactive TUI wizard or `--non-interactive`
//     flag set that drives the Detect → Configure → Validate →
//     Install → Verify FSM and writes a usable agent config.
//     v1.0 supports demo mode end-to-end; production / enterprise
//     modes are deferred to v0.x (see docs/project/ROADMAP.md).
//   - service install|uninstall|status — manage the
//     keystone-core-agent systemd unit. Linux-only in v1.0.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/agent/bootstrap"
	"go.keystone-core.io/keystone-core/internal/agent/bootstrap/tui"
	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/logging"
	natsmgr "go.keystone-core.io/keystone-core/internal/nats"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// shutdownTimeout matches the §4.6 "drain pending, exit" budget.
// Heartbeat / metadata loops exit on ctx cancellation in
// microseconds; only the command goroutine (Task 5+) realistically
// consumes the budget.
const shutdownTimeout = 30 * time.Second

func main() {
	if err := newCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	root := cli.RootCommand(cli.Options{
		Name:  "kscore-agent",
		Short: "Keystone Core agent daemon",
		Run:   run,
	})
	root.AddCommand(newBootstrapCommand())
	root.AddCommand(newServiceCommand())
	return root
}

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	if cfg.Agent.AgentID == "" {
		return errors.New("agent: cfg.Agent.AgentID is required (set via config or bootstrap — Task 6)")
	}
	if cfg.NATS.Mode != config.NATSModeExternal {
		// v1.0 agents always run in external mode — they connect to
		// the control plane's NATS, never embed their own. Embedded
		// mode would be a v2.x+ hybrid topology.
		return fmt.Errorf("agent: NATS mode must be external, got %q", string(cfg.NATS.Mode))
	}

	natsManager, err := natsmgr.New(cfg.NATS, log)
	if err != nil {
		return fmt.Errorf("nats: %w", err)
	}
	if err := natsManager.Start(ctx); err != nil {
		return fmt.Errorf("nats start: %w", err)
	}
	natsClosed := false
	defer func() {
		if !natsClosed {
			stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			_ = natsManager.Shutdown(stopCtx)
		}
	}()

	enforcer, err := agent.NewSecurityEnforcer(securityPolicyFromConfig(cfg.Security), log)
	if err != nil {
		return fmt.Errorf("security enforcer: %w", err)
	}
	executor := agent.NewExecutor(agent.ExecutorConfig{
		Logger:         log,
		DefaultTimeout: cfg.Agent.CommandTimeout,
	})

	a, err := agent.New(agent.Config{
		AgentID:           cfg.Agent.AgentID,
		HeartbeatInterval: cfg.Agent.HeartbeatInterval,
		MetadataInterval:  cfg.Agent.MetadataInterval,
		CommandTimeout:    cfg.Agent.CommandTimeout,
		Labels:            cfg.Agent.Labels,
		BootstrapPSK:      cfg.Agent.BootstrapPSK,
	}, natsAdapter{m: natsManager}, natsManager.Subjects(), agent.NewGopsutilCollector(log), executor, enforcer, log)
	if err != nil {
		return fmt.Errorf("agent: %w", err)
	}
	if err := a.Start(ctx); err != nil {
		return fmt.Errorf("agent start: %w", err)
	}

	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	agentErr := a.Shutdown(stopCtx)
	natsErr := natsManager.Shutdown(stopCtx)
	natsClosed = true

	if agentErr != nil {
		return fmt.Errorf("agent shutdown: %w", agentErr)
	}
	if natsErr != nil {
		return fmt.Errorf("nats shutdown: %w", natsErr)
	}
	return nil
}

// natsAdapter bridges internal/nats.Manager (which uses internal/
// nats.MessageHandler / Subscription) into agent.MessageHandler /
// Subscription. Function-type aliases don't unify across packages
// in Go interfaces, so we adapt explicitly. Same wiring-layer
// pattern as cmd/kscore-server's natsSubscriberAdapter.
type natsAdapter struct{ m *natsmgr.Manager }

func (a natsAdapter) PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error {
	return a.m.PublishEnvelope(ctx, subject, env)
}

func (a natsAdapter) Subscribe(subject string, h agent.MessageHandler) (agent.Subscription, error) {
	sub, err := a.m.Subscribe(subject, natsmgr.MessageHandler(h))
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (a natsAdapter) Health(ctx context.Context) error {
	return a.m.Health(ctx)
}

// securityPolicyFromConfig translates internal/config.SecurityConfig
// into the internal/agent.SecurityPolicy shape. Same translator
// lives in cmd/kscore-server — both binaries read the same
// security.* section of the config so the HMAC secret + rules stay
// in lockstep.
func securityPolicyFromConfig(c config.SecurityConfig) agent.SecurityPolicy {
	policy := agent.SecurityPolicy{
		HMACSecret:         c.DecodedHMACSecret(),
		PrincipalAllowlist: c.PrincipalAllowlist,
		CommandRules: agent.CommandRules{
			AllowGlobs:   c.CommandAllowGlobs,
			AllowRegexes: c.CommandAllowRegexes,
			DenyGlobs:    c.CommandDenyGlobs,
			DenyRegexes:  c.CommandDenyRegexes,
		},
		EnvVarAllowlist: c.EnvVarAllowlist,
		MaxArgsBytes:    c.MaxArgsBytes,
	}
	switch c.DefaultPolicy {
	case "allow":
		policy.DefaultPolicy = agent.PolicyAllow
	case "deny":
		policy.DefaultPolicy = agent.PolicyDeny
	}
	return policy
}

// bootstrapFlags holds the parsed --non-interactive flag set.
// Each field has a corresponding KSCORE_BOOTSTRAP_* env var
// fallback; the flag wins when both are set. Defaults are filled
// in by applyDefaults from the loaded daemon config + sensible
// fallbacks.
type bootstrapFlags struct {
	NonInteractive bool
	Mode           string
	ClusterName    string
	AgentID        string
	NodeRole       string
	Join           string
	JoinToken      string
	ConfigPath     string
	StatePath      string
	DryRun         bool
}

// newBootstrapCommand builds the `kscore-agent bootstrap`
// subcommand. Default invocation runs the TUI wizard (Task 7);
// `--non-interactive` (or KSCORE_BOOTSTRAP_NON_INTERACTIVE=1)
// switches to a flag-driven bootstrap suitable for CI and
// config-management tooling.
//
// v1.0 ships demo mode end-to-end. Production / enterprise modes
// are rejected by bootstrap.ValidateForV10 in both paths — see
// docs/project/ROADMAP.md.
//
// We don't reuse cli.RootCommand's daemon RunE — bootstrap has
// its own lifecycle (no NATS, no heartbeat). Config + logger +
// signal handling are loaded inline.
func newBootstrapCommand() *cobra.Command {
	flags := &bootstrapFlags{}
	cmd := &cobra.Command{
		Use:           "bootstrap",
		Short:         "Bootstrap the agent (interactive wizard or --non-interactive flags)",
		Long:          "Walks through the Detect → Configure → Validate → Install → Verify state machine and writes a usable agent config. Default is the TUI wizard; pass --non-interactive (or set KSCORE_BOOTSTRAP_NON_INTERACTIVE=1) to drive entirely from flags / env vars. v1.0 supports demo mode; production and enterprise modes are deferred (see docs/project/ROADMAP.md).",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBootstrap(cmd, flags)
		},
	}
	registerBootstrapFlags(cmd, flags)
	return cmd
}

func registerBootstrapFlags(cmd *cobra.Command, f *bootstrapFlags) {
	cmd.Flags().BoolVar(&f.NonInteractive, "non-interactive", false,
		"skip the TUI; drive bootstrap from flags / env vars (KSCORE_BOOTSTRAP_NON_INTERACTIVE)")
	cmd.Flags().StringVar(&f.Mode, "mode", "",
		"bootstrap mode: demo (KSCORE_BOOTSTRAP_MODE). production and enterprise are reserved for v1.x")
	cmd.Flags().StringVar(&f.ClusterName, "cluster-name", "",
		"cluster this agent joins (KSCORE_BOOTSTRAP_CLUSTER_NAME)")
	cmd.Flags().StringVar(&f.AgentID, "agent-id", "",
		"unique id for this agent within the cluster (KSCORE_BOOTSTRAP_AGENT_ID; default = hostname)")
	cmd.Flags().StringVar(&f.NodeRole, "node-role", "",
		"free-text role label for targeting (KSCORE_BOOTSTRAP_NODE_ROLE; default = worker)")
	cmd.Flags().StringVar(&f.Join, "join", "",
		"NATS join URL, e.g. nats://server:4222 (KSCORE_BOOTSTRAP_JOIN)")
	cmd.Flags().StringVar(&f.JoinToken, "join-token", "",
		"optional bootstrap PSK / token (KSCORE_BOOTSTRAP_JOIN_TOKEN)")
	cmd.Flags().StringVar(&f.ConfigPath, "config-path", "",
		"absolute path where keystone-core-agent.yaml is rendered (KSCORE_BOOTSTRAP_CONFIG_PATH; default = /etc/keystone-core/keystone-core-agent.yaml)")
	cmd.Flags().StringVar(&f.StatePath, "state-path", "",
		"absolute path for the bootstrap FSM state file (KSCORE_BOOTSTRAP_STATE_PATH; default = /var/lib/keystone-core/bootstrap.json)")
	cmd.Flags().BoolVar(&f.DryRun, "dry-run", false,
		"report what would happen without writing the config (KSCORE_BOOTSTRAP_DRY_RUN)")
}

// applyEnvFallback fills in flag values from KSCORE_BOOTSTRAP_*
// env vars when the flag wasn't passed on the command line.
// Cobra's Flag.Changed tells us whether the operator set the
// flag explicitly — if not, env wins; if so, flag wins. Boolean
// envs accept any value parseable by strconv.ParseBool.
func applyEnvFallback(cmd *cobra.Command, f *bootstrapFlags) error {
	if !cmd.Flags().Lookup("non-interactive").Changed {
		if v, ok := os.LookupEnv("KSCORE_BOOTSTRAP_NON_INTERACTIVE"); ok {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("KSCORE_BOOTSTRAP_NON_INTERACTIVE: %w", err)
			}
			f.NonInteractive = b
		}
	}
	if !cmd.Flags().Lookup("dry-run").Changed {
		if v, ok := os.LookupEnv("KSCORE_BOOTSTRAP_DRY_RUN"); ok {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("KSCORE_BOOTSTRAP_DRY_RUN: %w", err)
			}
			f.DryRun = b
		}
	}
	envForString := func(flagName, envName string, target *string) {
		if cmd.Flags().Lookup(flagName).Changed {
			return
		}
		if v, ok := os.LookupEnv(envName); ok {
			*target = v
		}
	}
	envForString("mode", "KSCORE_BOOTSTRAP_MODE", &f.Mode)
	envForString("cluster-name", "KSCORE_BOOTSTRAP_CLUSTER_NAME", &f.ClusterName)
	envForString("agent-id", "KSCORE_BOOTSTRAP_AGENT_ID", &f.AgentID)
	envForString("node-role", "KSCORE_BOOTSTRAP_NODE_ROLE", &f.NodeRole)
	envForString("join", "KSCORE_BOOTSTRAP_JOIN", &f.Join)
	envForString("join-token", "KSCORE_BOOTSTRAP_JOIN_TOKEN", &f.JoinToken)
	envForString("config-path", "KSCORE_BOOTSTRAP_CONFIG_PATH", &f.ConfigPath)
	envForString("state-path", "KSCORE_BOOTSTRAP_STATE_PATH", &f.StatePath)
	return nil
}

func runBootstrap(cmd *cobra.Command, f *bootstrapFlags) error {
	if err := applyEnvFallback(cmd, f); err != nil {
		return err
	}
	cfgPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("flag: %w", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log, err := logging.New(logging.Options{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
		Output: cmd.ErrOrStderr(), // stdout is the TUI's canvas
	})
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	ctx, cancel := signal.NotifyContext(cmd.Context(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	configurer, err := buildConfigurer(cfg, f, log)
	if err != nil {
		return err
	}
	engine, err := bootstrap.NewEngine(bootstrap.EngineConfig{
		StatePath:  f.StatePath, // empty -> bootstrap default
		Logger:     log,
		Detector:   bootstrap.NewDefaultDetector(log),
		Configurer: configurer,
		Validator:  bootstrap.NewDefaultValidator(log),
		Installer:  bootstrap.NewDefaultInstaller(log),
		Verifier:   bootstrap.NewDefaultVerifier(log),
	})
	if err != nil {
		return fmt.Errorf("bootstrap engine: %w", err)
	}
	state, runErr := engine.Run(ctx)
	if runErr != nil {
		return fmt.Errorf("bootstrap: %w", runErr)
	}
	log.InfoContext(ctx, "bootstrap complete",
		"phase", string(state.Phase),
		"config_path", configPathFromState(state),
	)
	return nil
}

// buildConfigurer picks between non-interactive (StaticConfigurer
// from flags) and interactive (TUI from Task 7). Flags also seed
// the TUI's Defaults so an operator can pass "almost everything"
// and let the wizard fill in the rest.
func buildConfigurer(cfg *config.Config, f *bootstrapFlags, log *slog.Logger) (bootstrap.Configurer, error) {
	mode := f.Mode
	if mode == "" {
		mode = string(bootstrap.ModeDemo)
	}
	clusterName := firstNonEmpty(f.ClusterName, cfg.NATS.ClusterName, "default")
	agentID := firstNonEmpty(f.AgentID, cfg.Agent.AgentID, hostnameOr(""))
	nodeRole := firstNonEmpty(f.NodeRole, "worker")
	join := firstNonEmpty(f.Join, firstURL(cfg.NATS.URLs))
	cfgPath := firstNonEmpty(f.ConfigPath, "/etc/keystone-core/keystone-core-agent.yaml")

	if !f.NonInteractive {
		return tui.NewConfigurer(tui.Defaults{
			ClusterName: clusterName,
			AgentID:     agentID,
			NodeRole:    nodeRole,
			JoinURL:     join,
			ConfigPath:  cfgPath,
		}, log), nil
	}

	// Non-interactive path: a missing required value is a hard
	// error — we won't half-bootstrap a system based on guesses.
	if f.Mode == "" {
		return nil, errors.New("--non-interactive requires --mode (or KSCORE_BOOTSTRAP_MODE)")
	}
	if f.Join == "" {
		return nil, errors.New("--non-interactive requires --join (or KSCORE_BOOTSTRAP_JOIN)")
	}
	bootstrapCfg := &bootstrap.Configuration{
		Mode:        bootstrap.Mode(mode),
		ClusterName: clusterName,
		AgentID:     agentID,
		NodeRole:    nodeRole,
		JoinURL:     join,
		JoinToken:   f.JoinToken,
		ConfigPath:  cfgPath,
		DryRun:      f.DryRun,
	}
	if err := bootstrap.ValidateForV10(bootstrapCfg); err != nil {
		return nil, err
	}
	if err := bootstrapCfg.Validate(); err != nil {
		return nil, fmt.Errorf("--non-interactive: %w", err)
	}
	return bootstrap.StaticConfigurer{Config: bootstrapCfg}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstURL(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

func hostnameOr(fallback string) string {
	if hn, err := os.Hostname(); err == nil && hn != "" {
		return hn
	}
	return fallback
}

func configPathFromState(state *bootstrap.State) string {
	if state == nil || state.Config == nil {
		return ""
	}
	return state.Config.ConfigPath
}
