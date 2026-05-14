package main

import (
	"context"
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
