// kscore-agent is the Keystone Core agent daemon.
//
// Default invocation runs the daemon: connects to the cluster's
// NATS, registers, runs heartbeat + metadata loops, executes
// commands, exits cleanly on SIGTERM/SIGINT.
//
// Subcommands:
//   - bootstrap — interactive TUI wizard that drives the
//     Detect → Configure → Validate → Install → Verify FSM
//     and writes a usable agent config. v1.0 supports demo mode
//     end-to-end; production / enterprise modes are deferred to
//     v1.x (see docs/project/V1X-BACKLOG.md).
//
// Subsequent tasks layer on top:
//   - Task 8: --non-interactive flag set covering CLI-only bootstrap.
//   - Task 9: systemd unit install.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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
	return root
}

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	if cfg.Agent.AgentID == "" {
		return errors.New("agent: cfg.Agent.AgentID is required (set via config or bootstrap — Task 6)")
	}
	if cfg.NATS.Mode != config.NATSModeExternal {
		// v1.0 agents always run in external mode — they connect to
		// the control plane's NATS, never embed their own. Embedded
		// mode would be a v2.0 hybrid topology.
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

// newBootstrapCommand builds the `kscore-agent bootstrap`
// subcommand. It runs the bootstrap FSM (Task 6) with the TUI
// Configurer (Task 7) + default Detector / Validator / Installer
// / Verifier. Non-interactive flag coverage (Task 8) layers on
// top; the v1.0-demo-only mode-gate lives in tui.Configurer.
//
// We don't reuse cli.RootCommand's daemon RunE — bootstrap has
// its own lifecycle (no NATS, no heartbeat). Config + logger +
// signal handling are loaded inline; the duplication is small
// and keeps cli's surface clean.
func newBootstrapCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "bootstrap",
		Short:         "Interactive bootstrap wizard (demo mode)",
		Long:          "Walks through the Detect → Configure → Validate → Install → Verify state machine and writes a usable agent config. v1.0 supports demo mode; production and enterprise modes are deferred (see docs/project/V1X-BACKLOG.md).",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
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

			defaults := tui.Defaults{
				ClusterName: cfg.NATS.ClusterName,
				AgentID:     cfg.Agent.AgentID,
				JoinURL:     firstURL(cfg.NATS.URLs),
			}
			configurer := tui.NewConfigurer(defaults, log)

			engine, err := bootstrap.NewEngine(bootstrap.EngineConfig{
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
		},
	}
}

func firstURL(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

func configPathFromState(state *bootstrap.State) string {
	if state == nil || state.Config == nil {
		return ""
	}
	return state.Config.ConfigPath
}
