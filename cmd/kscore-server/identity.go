package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/identity"
	"go.keystone-core.io/keystone-core/internal/state"
)

// startIdentityProvider wires the v0.1 embedded identity provider
// per the Epic 09 task 12 boot recipe:
//
//   - FileCAStorage at IdentityConfig.StoragePath
//   - CAConfig built from IdentityConfig + identity.DefaultCAConfig
//   - JoinTokenStore: StateJoinTokenStore (state.Store-backed) by
//     default; in-memory when cfg.JoinTokensInMemory is true (the
//     test / CI escape hatch)
//   - JoinTokenAttestor registered as the default attestor
//
// Returns a Started provider; the caller is responsible for
// invoking Stop on shutdown.
func startIdentityProvider(ctx context.Context, cfg config.IdentityConfig, store state.Store, log *slog.Logger) (*identity.EmbeddedProvider, error) {
	caStorage, err := identity.NewFileCAStorage(cfg.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("ca storage: %w", err)
	}

	caConfig := identity.DefaultCAConfig(cfg.TrustDomain)
	if cfg.KeyType != "" {
		caConfig.KeyType = identity.CAKeyType(cfg.KeyType)
	}

	var tokenStore identity.JoinTokenStore
	if cfg.JoinTokensInMemory {
		tokenStore = identity.NewInMemoryJoinTokenStore()
	} else {
		adapter, err := identity.NewStateJoinTokenStore(store)
		if err != nil {
			return nil, fmt.Errorf("state join-token store: %w", err)
		}
		tokenStore = adapter
	}

	attestor, err := identity.NewJoinTokenAttestor(identity.JoinTokenAttestorConfig{
		Store:       tokenStore,
		TrustDomain: cfg.TrustDomain,
	})
	if err != nil {
		return nil, fmt.Errorf("join-token attestor: %w", err)
	}

	provider, err := identity.NewEmbeddedProvider(identity.EmbeddedProviderConfig{
		CAConfig:       caConfig,
		Storage:        caStorage,
		Logger:         log,
		JoinTokenStore: tokenStore,
		Attestors:      []identity.Attestor{attestor},
	})
	if err != nil {
		return nil, fmt.Errorf("embedded provider: %w", err)
	}
	if err := provider.Start(ctx); err != nil {
		return nil, fmt.Errorf("embedded provider start: %w", err)
	}

	log.Info("identity provider started",
		"trust_domain", cfg.TrustDomain,
		"storage_path", cfg.StoragePath,
		"key_type", caConfig.KeyType,
		"tokens_in_memory", cfg.JoinTokensInMemory,
	)
	return provider, nil
}

// buildIdentityTLSConfig wires a [*tls.Config] from the running
// identity provider when cfg.Server.TLS.Enabled and the operator
// hasn't configured an explicit cert/key file pair. Returns
// (nil, nil) when TLS is sourced from files or disabled — the
// caller branches on `tlsConfig == nil` to decide what to pass to
// server.Options.TLSConfig.
//
// The returned cancel func tears down the background watcher
// goroutine; the caller MUST invoke it on shutdown.
func buildIdentityTLSConfig(ctx context.Context, serverCfg config.ServerConfig, provider *identity.EmbeddedProvider, log *slog.Logger) (*tls.Config, func(), error) {
	if !serverCfg.TLS.Enabled {
		return nil, func() {}, nil
	}
	if serverCfg.TLS.SourcedFromFiles() {
		// Operator configured an explicit cert/key pair —
		// pkg/api/server's older file-loader path will handle
		// it once that's wired. Until then, surface a clear
		// error so operators see the gap.
		return nil, func() {}, fmt.Errorf("identity tls: file-sourced TLS (CertFile + KeyFile) is not yet wired; leave both empty to derive from the identity provider")
	}
	if provider == nil {
		return nil, func() {}, fmt.Errorf("identity tls: cfg.Server.TLS.Enabled requires either CertFile/KeyFile OR cfg.Identity.Enabled=true")
	}
	tlsCfg, cancel, err := identity.BuildServerTLSConfig(ctx, provider, identity.ServerRoleControlPlane, &identity.ServerTLSOptions{
		Logger: log,
	})
	if err != nil {
		return nil, func() {}, fmt.Errorf("identity tls: %w", err)
	}
	log.Info("identity-sourced TLS config built",
		"role", "control-plane",
		"min_version", "TLS 1.3",
		"client_auth", "verify-if-given (mTLS + API-key fallback)",
	)
	return tlsCfg, cancel, nil
}
