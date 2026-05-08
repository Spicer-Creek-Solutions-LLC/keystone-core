// kscore-agent is the Keystone Core agent daemon.
//
// v1.0 Epic 06 task 1: lifecycle skeleton. Constructs an external-
// mode NATS Manager from cfg.NATS, builds an Agent (subscribed to
// kscore.{cluster}.agent.{id}.command, heartbeat + metadata loops
// running), serves until SIGTERM/SIGINT, and shuts down in reverse.
//
// Subsequent tasks layer on top:
//   - Task 2: Executor (real os/exec).
//   - Task 3: MetadataCollector (gopsutil).
//   - Task 4: SecurityEnforcer (HMAC, allowlists).
//   - Task 5: full command-response handler.
//   - Task 6/7/8: bootstrap subcommand + TUI + non-interactive flags.
//   - Task 9: systemd unit install.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/config"
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
	return cli.RootCommand(cli.Options{
		Name:  "kscore-agent",
		Short: "Keystone Core agent daemon",
		Run:   run,
	})
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

	a, err := agent.New(agent.Config{
		AgentID:           cfg.Agent.AgentID,
		HeartbeatInterval: cfg.Agent.HeartbeatInterval,
		MetadataInterval:  cfg.Agent.MetadataInterval,
		CommandTimeout:    cfg.Agent.CommandTimeout,
		Labels:            cfg.Agent.Labels,
	}, natsAdapter{m: natsManager}, natsManager.Subjects(), log)
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
