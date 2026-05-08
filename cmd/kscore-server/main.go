// kscore-server is the Keystone Core control-plane daemon.
//
// Wires the runtime pieces from internal/cli + internal/config +
// internal/state + pkg/api/server into a daemon that runs the §4.4
// 21-step init, serves until SIGTERM/SIGINT, and shuts down in
// reverse-of-init.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	natsmgr "go.keystone-core.io/keystone-core/internal/nats"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
	"go.keystone-core.io/keystone-core/pkg/api/server"
)

// shutdownTimeout matches PROJECT-DETAILS §4.4 — 30s ceiling on graceful
// shutdown. Task 8 may make this configurable.
const shutdownTimeout = 30 * time.Second

func main() {
	if err := newCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	return cli.RootCommand(cli.Options{
		Name:  "kscore-server",
		Short: "Keystone Core control-plane server",
		Run:   run,
	})
}

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	stateCfg, err := cfg.Storage.ToStateConfig()
	if err != nil {
		return fmt.Errorf("storage config: %w", err)
	}
	store, err := state.NewStore(stateCfg)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	// store.Close runs inside server.Stop; the local defer is the
	// safety net for the case where New fails after store creation.
	storeClosed := false
	defer func() {
		if !storeClosed {
			_ = store.Close()
		}
	}()

	// Auth chain: API key (Bearer token) → RBAC → rate-limit. The
	// cmd binary wires this unconditionally; integrators wanting auth
	// disabled build their own server.Options with AuthInterceptor=nil.
	verifier := apikeys.NewStoreVerifier(store)
	authInterceptor := &auth.InterceptorConfig{
		Authenticator: auth.NewAPIKeyAuthenticator(verifier),
		Authorizer:    auth.NewRBACAuthorizer(),
		RateLimiter:   auth.NewRateLimiter(auth.RateLimitConfig{}),
	}

	// Dev-mode bootstrap: ensure a default admin API key exists so
	// out-of-the-box `kscore-server run` is immediately usable. The
	// cleartext is logged exactly once at WARN — store it now; it
	// cannot be recovered. Production mode skips this — operators
	// must provision keys via /api/v1/apikeys.
	if cfg.Mode == config.ModeDevelopment {
		cleartext, generated, err := apikeys.EnsureDevKey(ctx, store)
		if err != nil {
			return fmt.Errorf("dev key bootstrap: %w", err)
		}
		if generated {
			log.WarnContext(ctx,
				"DEV API KEY GENERATED — store this now; it cannot be recovered",
				"name", apikeys.DevKeyName,
				"role", "admin",
				"key", cleartext,
			)
		}
	}

	natsManager, err := natsmgr.New(cfg.NATS, log)
	if err != nil {
		return fmt.Errorf("nats: %w", err)
	}

	opts := server.Options{
		Config:          cfg,
		Logger:          log,
		Store:           store,
		NATSManager:     natsManager,
		Subjects:        natsManager.Subjects(),
		AuthInterceptor: authInterceptor,
	}

	if cfg.NATS.Bootstrap.Enabled {
		entries, err := controlplane.DecodeConfigPSKs(toConfigPSKs(cfg.NATS.Bootstrap.PSKs))
		if err != nil {
			return fmt.Errorf("bootstrap psk decode: %w", err)
		}
		issuer, err := controlplane.NewAPIKeyIssuer(controlplane.APIKeyIssuerConfig{
			Keys: store,
		})
		if err != nil {
			return fmt.Errorf("bootstrap issuer: %w", err)
		}
		opts.Subscriber = natsSubscriberAdapter{m: natsManager}
		opts.BootstrapValidator = controlplane.NewPSKValidator(controlplane.PSKValidatorConfig{
			Entries: entries,
		})
		opts.CredentialIssuer = issuer
	}

	srv, err := server.New(opts)
	if err != nil {
		return fmt.Errorf("server init: %w", err)
	}
	storeClosed = true // server.Stop now owns the store

	if err := srv.Start(ctx); err != nil {
		return fmt.Errorf("server start: %w", err)
	}

	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Stop(stopCtx)
}

// natsSubscriberAdapter bridges internal/nats.Manager (which uses
// internal/nats.MessageHandler / Subscription) into controlplane's
// equivalent named types. Function-type aliases don't unify across
// packages in Go interfaces, so we adapt explicitly.
type natsSubscriberAdapter struct{ m *natsmgr.Manager }

func (a natsSubscriberAdapter) Subscribe(subject string, h controlplane.MessageHandler) (controlplane.Subscription, error) {
	sub, err := a.m.Subscribe(subject, natsmgr.MessageHandler(h))
	if err != nil {
		return nil, err
	}
	return sub, nil // *natsSubscription satisfies controlplane.Subscription via its Unsubscribe method
}

// toConfigPSKs translates from the config-shaped BootstrapPSK to the
// controlplane-shaped ConfigPSK. Internal/controlplane stays free of
// internal/config imports; this binary is the wiring layer that
// crosses both sides.
func toConfigPSKs(in []config.BootstrapPSK) []controlplane.ConfigPSK {
	out := make([]controlplane.ConfigPSK, 0, len(in))
	for _, p := range in {
		out = append(out, controlplane.ConfigPSK{
			AgentID:   p.AgentID,
			Secret:    p.Secret,
			ExpiresAt: p.ExpiresAt,
		})
	}
	return out
}
