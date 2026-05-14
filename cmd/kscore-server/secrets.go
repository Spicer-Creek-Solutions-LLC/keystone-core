package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/secrets/file"
	"go.keystone-core.io/keystone-core/internal/secrets/vault"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// secretsRuntime carries the live secrets infrastructure that the
// kscore-server boot wiring constructs from [config.SecretsConfig]
// and that the server passes through to its API layer via
// [server.Options].
type secretsRuntime struct {
	Broker   *secrets.Broker
	Transit  secrets.TransitBackend
	LeaseMgr *secrets.LeaseManager
	Cache    *secrets.SecretCache
	Backends []secrets.SecretBackend
}

// stop tears down the runtime in reverse order. Logs every error +
// continues — best-effort shutdown.
func (r *secretsRuntime) stop(ctx context.Context, log *slog.Logger) {
	if r == nil {
		return
	}
	if r.Broker != nil {
		if err := r.Broker.Stop(ctx); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "secrets: broker stop", slog.String("err", err.Error()))
		}
	}
	if r.LeaseMgr != nil {
		if err := r.LeaseMgr.Stop(ctx); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "secrets: lease manager stop", slog.String("err", err.Error()))
		}
	}
	if r.Cache != nil {
		if err := r.Cache.Stop(ctx); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "secrets: cache stop", slog.String("err", err.Error()))
		}
	}
}

// startSecrets builds the broker + lease manager + cache + per-config
// backends from [config.SecretsConfig], wires them, and starts each
// component. Returns nil + nil error when secrets is disabled — the
// caller treats nil as "no secrets surface configured."
func startSecrets(ctx context.Context, cfg config.SecretsConfig, store state.Store, log *slog.Logger) (*secretsRuntime, error) {
	if !cfg.Enabled {
		log.LogAttrs(ctx, slog.LevelInfo, "secrets: disabled in config; skipping")
		return nil, nil
	}

	rt := &secretsRuntime{}

	// Construct every configured backend.
	var transit secrets.TransitBackend
	for i, bc := range cfg.Backends {
		switch bc.Type {
		case config.SecretsBackendTypeFile:
			fb, err := file.NewBackend(file.Config{
				Path:            bc.File.Path,
				MasterKeySource: bc.File.MasterKey,
				Name:            bc.Name,
				Logger:          log,
				EnsureParentDir: true,
			})
			if err != nil {
				return nil, fmt.Errorf("secrets: backends[%d] %q: %w", i, bc.Name, err)
			}
			rt.Backends = append(rt.Backends, fb)
		case config.SecretsBackendTypeVault:
			vb, err := buildVaultBackend(bc)
			if err != nil {
				return nil, fmt.Errorf("secrets: backends[%d] %q: %w", i, bc.Name, err)
			}
			rt.Backends = append(rt.Backends, vb)
			if transit == nil {
				transit = vb
			}
		default:
			return nil, fmt.Errorf("secrets: backends[%d] %q: unknown type %q", i, bc.Name, bc.Type)
		}
	}

	// Build the router.
	routes := make([]secrets.Route, 0, len(cfg.Routing))
	for _, r := range cfg.Routing {
		routes = append(routes, secrets.Route{Prefix: r.Prefix, Backend: r.Backend})
	}
	router, err := secrets.NewRouter(routes)
	if err != nil {
		return nil, fmt.Errorf("secrets: router: %w", err)
	}

	// Lease manager (always — even when no dynamic-capable backend
	// is configured, the manager handles the LeaseDirectory contract).
	lmCfg := secrets.LeaseManagerConfig{
		Store:        store,
		PollInterval: cfg.Lease.PollInterval,
		Jitter:       cfg.Lease.Jitter,
		RenewTimeout: cfg.Lease.RenewTimeout,
		Logger:       log,
	}
	if cfg.Lease.DefaultStrategy != "" {
		strategy, err := secrets.ParseRenewStrategy(cfg.Lease.DefaultStrategy)
		if err != nil {
			return nil, fmt.Errorf("secrets: lease.default_strategy: %w", err)
		}
		lmCfg.DefaultStrategy = strategy
	}
	lm, err := secrets.NewLeaseManager(lmCfg)
	if err != nil {
		return nil, fmt.Errorf("secrets: lease manager: %w", err)
	}
	rt.LeaseMgr = lm

	// Cache (optional).
	var cache secrets.Cache
	if cfg.Cache.Enabled {
		sc, err := secrets.NewSecretCache(secrets.SecretCacheConfig{
			DefaultTTL:    cfg.Cache.DefaultTTL,
			MaxEntries:    cfg.Cache.MaxEntries,
			SweepInterval: cfg.Cache.SweepInterval,
			Logger:        log,
		})
		if err != nil {
			return nil, fmt.Errorf("secrets: cache: %w", err)
		}
		rt.Cache = sc
		cache = sc
	}

	// Auditor — v1.0 fallback is the slog-backed log auditor; the
	// SQLite audit store ships with Epic 12.
	auditor := secrets.NewLogAuditor(log)

	broker, err := secrets.NewBroker(secrets.BrokerConfig{
		Router:           router,
		Backends:         rt.Backends,
		DefaultBackend:   cfg.DefaultBackend,
		Cache:            cache,
		Auditor:          auditor,
		LeaseDirectory:   lm,
		ExtractPrincipal: extractSecretsPrincipal,
		Logger:           log,
	})
	if err != nil {
		return nil, fmt.Errorf("secrets: broker: %w", err)
	}
	rt.Broker = broker
	rt.Transit = transit

	// Wire the lease manager's renewer back to the broker AFTER the
	// broker is constructed (the LeaseDirectory-vs-renewer cycle is
	// broken by the post-hoc setter).
	lm.SetRenewer(broker.RenewLease)

	// Start every backend, then the broker (which manages backend
	// lifecycles), then the lease manager + cache.
	if err := broker.Start(ctx); err != nil {
		return nil, fmt.Errorf("secrets: broker start: %w", err)
	}
	if rt.Cache != nil {
		if err := rt.Cache.Start(ctx); err != nil {
			_ = broker.Stop(ctx)
			return nil, fmt.Errorf("secrets: cache start: %w", err)
		}
	}
	if err := lm.Start(ctx); err != nil {
		if rt.Cache != nil {
			_ = rt.Cache.Stop(ctx)
		}
		_ = broker.Stop(ctx)
		return nil, fmt.Errorf("secrets: lease manager start: %w", err)
	}

	log.LogAttrs(ctx, slog.LevelInfo, "secrets: enabled",
		slog.Int("backends", len(rt.Backends)),
		slog.Int("routes", router.Len()),
		slog.String("default_backend", cfg.DefaultBackend),
		slog.Bool("cache_enabled", cfg.Cache.Enabled),
		slog.Bool("transit_enabled", transit != nil),
	)
	return rt, nil
}

// buildVaultBackend translates [config.SecretsVaultBackendConfig]
// into the vault package's Config + constructs the backend.
func buildVaultBackend(bc config.SecretsBackendConfig) (*vault.Backend, error) {
	v := bc.Vault
	cfg := vault.Config{
		Address:   v.Address,
		Namespace: v.Namespace,
		Name:      bc.Name,
		Timeout:   v.Timeout,
		TLS: vault.TLSConfig{
			CACert:     v.TLS.CACert,
			ClientCert: v.TLS.ClientCert,
			ClientKey:  v.TLS.ClientKey,
			ServerName: v.TLS.ServerName,
			Insecure:   v.TLS.Insecure,
		},
	}
	for _, m := range v.Mounts {
		cfg.Mounts = append(cfg.Mounts, vault.MountConfig{Path: m.Path, KVVersion: m.KVVersion})
	}
	switch v.AuthMethod {
	case "token":
		cfg.Auth = vault.AuthConfig{Method: vault.AuthMethodToken, Token: &vault.TokenAuthConfig{Token: v.Token}}
	case "approle":
		cfg.Auth = vault.AuthConfig{Method: vault.AuthMethodAppRole, AppRole: &vault.AppRoleAuthConfig{
			RoleID:                  v.AppRole.RoleID,
			SecretID:                v.AppRole.SecretID,
			SecretIDIsWrappingToken: v.AppRole.SecretIDIsWrappingToken,
			Mount:                   v.AppRole.Mount,
		}}
	case "kubernetes":
		cfg.Auth = vault.AuthConfig{Method: vault.AuthMethodKubernetes, Kubernetes: &vault.KubernetesAuthConfig{
			Role:      v.Kubernetes.Role,
			TokenPath: v.Kubernetes.TokenPath,
			Mount:     v.Kubernetes.Mount,
		}}
	case "ldap":
		cfg.Auth = vault.AuthConfig{Method: vault.AuthMethodLDAP, LDAP: &vault.LDAPAuthConfig{
			Username: v.LDAP.Username,
			Password: v.LDAP.Password,
			Mount:    v.LDAP.Mount,
		}}
	}
	return vault.NewBackend(cfg)
}

// extractSecretsPrincipal converts a [pkg/api/auth.Principal] from
// the request context into a [secrets.Principal] for audit events.
// The mTLS auth path populates `Metadata["spiffe_id"]` (Epic 09 task
// 13); API-key + JWT paths leave it empty.
func extractSecretsPrincipal(ctx context.Context) secrets.Principal {
	p := auth.PrincipalFromContext(ctx)
	if p == nil {
		return secrets.Principal{}
	}
	out := secrets.Principal{User: p.ID}
	if p.Metadata != nil {
		out.AgentID = p.Metadata["agent_id"]
		out.SPIFFEID = p.Metadata["spiffe_id"]
	}
	return out
}

// stopSecretsCtx is a small helper for the shutdown path — bounds
// the per-component stop wait at 10s so a stuck backend doesn't
// hang the whole shutdown.
func stopSecretsCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}
