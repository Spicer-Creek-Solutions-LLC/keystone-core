// kscore-server is the Keystone Core control-plane daemon.
//
// Wires the runtime pieces from internal/cli + internal/config +
// internal/state + pkg/api/server into a daemon that runs the §4.4
// 21-step init, serves until SIGTERM/SIGINT, and shuts down in
// reverse-of-init.
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
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/identity"
	natsmgr "go.keystone-core.io/keystone-core/internal/nats"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib"
	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
	"go.keystone-core.io/keystone-core/pkg/api/server"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
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

	enforcer, err := agent.NewSecurityEnforcer(securityPolicyFromConfig(cfg.Security), log)
	if err != nil {
		return fmt.Errorf("security enforcer: %w", err)
	}

	// Epic 09 task 12 — start identity provider BEFORE the server
	// builds its Options so the provider can supply the server cert
	// when cfg.Server.TLS.Enabled (task 13). The provider stop
	// runs after server.Stop on shutdown.
	var identityProvider *identity.EmbeddedProvider
	if cfg.Identity.Enabled {
		identityProvider, err = startIdentityProvider(ctx, cfg.Identity, store, log)
		if err != nil {
			return fmt.Errorf("identity provider: %w", err)
		}
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			_ = identityProvider.Stop(stopCtx)
		}()
	}

	// Epic 09 task 13 — derive *tls.Config from the identity
	// provider when cfg.Server.TLS.Enabled. Returns (nil, nop, nil)
	// when TLS is off; cancel terminates the watcher goroutine that
	// rebuilds the cert + bundle on signing-CA rotation.
	tlsConfig, tlsCancel, err := buildIdentityTLSConfig(ctx, cfg.Server, identityProvider, log)
	if err != nil {
		return fmt.Errorf("identity tls config: %w", err)
	}
	defer tlsCancel()

	opts := server.Options{
		Config:          cfg,
		Logger:          log,
		Store:           store,
		NATSManager:     natsManager,
		Subjects:        natsManager.Subjects(),
		Signer:          commandSignerAdapter{enf: enforcer},
		AuthInterceptor: authInterceptor,
		TLSConfig:       tlsConfig,
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

	batchDisp, err := controlplane.NewBatchDispatcher(controlplane.BatchDispatcherConfig{
		Store:  store,
		Logger: log,
	})
	if err != nil {
		return fmt.Errorf("batch dispatcher: %w", err)
	}
	cpGRPC, err := controlplane.NewGRPCServer(controlplane.GRPCServerConfig{
		Dispatcher: batchDisp,
		Store:      store,
		Logger:     log,
		// Executor injected after Start once CommandDispatcher exists.
	})
	if err != nil {
		return fmt.Errorf("controlplane grpc: %w", err)
	}
	srv.RegisterService(&v1.ControlPlaneService_ServiceDesc, cpGRPC)

	// Epic 08: register stdlib modules into DefaultRegistry, then
	// wire StateService. RegisterAll(nil) targets DefaultRegistry,
	// which is what NewStateGRPCServer reads when its own Registry
	// is nil. DefaultRegistry is process-global, so a re-entrant
	// run() (the in-process test harness calls run() once per case)
	// finds the modules already registered — that is fine, not an
	// error.
	if err := stdlib.RegisterAll(nil); err != nil && !errors.Is(err, statemgmt.ErrDuplicateModule) {
		return fmt.Errorf("stdlib register: %w", err)
	}
	stateGRPC := controlplane.NewStateGRPCServer(nil, store)
	srv.RegisterService(&v1.StateService_ServiceDesc, stateGRPC)

	// Epic 09 task 12 — register IdentityService gRPC. The provider
	// itself is started above (so it can supply TLS material for
	// task 13's --tls.enabled path); we just bind the gRPC service.
	if identityProvider != nil {
		identityGRPC := controlplane.NewIdentityGRPCServer(identityProvider)
		srv.RegisterService(&v1.IdentityService_ServiceDesc, identityGRPC)
	}

	if err := srv.Start(ctx); err != nil {
		return fmt.Errorf("server start: %w", err)
	}

	// Task 12 — response router + NATS-backed BatchExecutor wired
	// post-Start so the CommandDispatcher exists. ExecuteCommand /
	// BatchExecuteCommand flip from Unavailable to live here.
	router, err := controlplane.NewResponseRouter(controlplane.ResponseRouterConfig{
		Subscriber: natsSubscriberAdapter{m: natsManager},
		Subjects:   natsManager.Subjects(),
		Dispatcher: srv.CommandDispatcher(),
		Logger:     log,
	})
	if err != nil {
		return fmt.Errorf("response router: %w", err)
	}
	if err := router.Start(ctx); err != nil {
		return fmt.Errorf("response router start: %w", err)
	}
	defer func() { _ = router.Stop() }()

	natsExec, err := controlplane.NewNATSBatchExecutor(controlplane.NATSBatchExecutorConfig{
		Dispatcher: srv.CommandDispatcher(),
		Router:     router,
	})
	if err != nil {
		return fmt.Errorf("nats batch executor: %w", err)
	}
	cpGRPC.SetExecutor(natsExec)

	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Stop(stopCtx)
}

// commandSignerAdapter bridges internal/agent.SecurityEnforcer
// into controlplane.Signer. The enforcer's ComputeHMAC takes an
// agent.CommandRequest; the dispatcher's Signer takes a
// controlplane.CommandMessage. Same field set, different package
// — adapter is one line.
type commandSignerAdapter struct{ enf *agent.SecurityEnforcer }

func (a commandSignerAdapter) SignCommand(msg controlplane.CommandMessage) string {
	return a.enf.ComputeHMAC(agent.CommandRequest{
		MessageID:      msg.MessageID,
		Principal:      msg.Principal,
		Command:        msg.Command,
		Args:           msg.Args,
		Env:            msg.Env,
		WorkingDir:     msg.WorkingDir,
		User:           msg.User,
		TimeoutSeconds: msg.TimeoutSeconds,
	})
}

// securityPolicyFromConfig translates internal/config.SecurityConfig
// into the internal/agent.SecurityPolicy shape.
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
