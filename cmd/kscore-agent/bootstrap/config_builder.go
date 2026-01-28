package bootstrap

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	serverConfigPath  = "/etc/kscore/server.yaml"
	agentConfigPath   = "/etc/kscore/agent.yaml"
	natsStoreDir      = "/var/lib/kscore/nats"
	jetstreamStoreDir = "/var/lib/kscore/jetstream"
)

func buildServerConfig(cfg *BootstrapConfig) ([]byte, error) {
	config := map[string]any{
		"server":  buildServerSection(cfg),
		"nats":    buildNATSSection(cfg, true),
		"storage": buildStorageSection(cfg),
		"auth":    buildAuthSection(cfg),
	}

	if tls := buildTLSSection(cfg); len(tls) > 0 {
		config["tls"] = tls
	}

	return yaml.Marshal(config)
}

func buildAgentConfig(cfg *BootstrapConfig) ([]byte, error) {
	config := map[string]any{
		"nats":  buildNATSSection(cfg, false),
		"agent": buildAgentSection(cfg),
	}

	if tls := buildTLSSection(cfg); len(tls) > 0 {
		config["tls"] = tls
	}

	return yaml.Marshal(config)
}

func buildServerSection(cfg *BootstrapConfig) map[string]any {
	server := map[string]any{}
	if cfg.BindAddress != "" {
		server["listenaddr"] = cfg.BindAddress
	}
	return server
}

func buildAgentSection(cfg *BootstrapConfig) map[string]any {
	agent := map[string]any{}
	if cfg.NodeName != "" {
		agent["id"] = cfg.NodeName
	}
	if len(cfg.NodeLabels) > 0 {
		agent["labels"] = cfg.NodeLabels
	}
	if cfg.Advertise != "" {
		agent["advertiseaddrs"] = []string{cfg.Advertise}
	}
	return agent
}

func buildNATSSection(cfg *BootstrapConfig, forServer bool) map[string]any {
	nats := map[string]any{}

	mode := strings.ToLower(cfg.NATSMode)
	switch mode {
	case "embedded", "cluster":
		if forServer {
			nats["mode"] = "embedded"
			embedded := map[string]any{
				"storedir": natsStoreDir,
			}
			if cfg.BindAddress != "" {
				embedded["host"] = cfg.BindAddress
			}
			nats["embedded"] = embedded
			nats["jetstream"] = map[string]any{
				"storedir": jetstreamStoreDir,
			}
		} else {
			nats["mode"] = "external"
			nats["url"] = defaultNATSURL(cfg)
		}
	case "leaf":
		nats["mode"] = "leaf"
		embedded := map[string]any{
			"storedir": natsStoreDir,
		}
		if cfg.BindAddress != "" {
			embedded["host"] = cfg.BindAddress
		}
		if len(cfg.NATSURLs) > 0 {
			embedded["leafnodeurls"] = cfg.NATSURLs
		}
		nats["embedded"] = embedded
		nats["jetstream"] = map[string]any{
			"storedir": jetstreamStoreDir,
		}
	case "external":
		nats["mode"] = "external"
		nats["url"] = strings.Join(cfg.NATSURLs, ",")
	default:
		if forServer {
			nats["mode"] = "embedded"
			nats["embedded"] = map[string]any{
				"storedir": natsStoreDir,
			}
			nats["jetstream"] = map[string]any{
				"storedir": jetstreamStoreDir,
			}
		} else {
			nats["mode"] = "external"
			nats["url"] = defaultNATSURL(cfg)
		}
	}

	if cfg.NATSCredsFile != "" {
		nats["credential"] = cfg.NATSCredsFile
	}

	return nats
}

func defaultNATSURL(cfg *BootstrapConfig) string {
	address := cfg.Advertise
	if address == "" {
		address = cfg.BindAddress
	}
	if address == "" {
		address = "127.0.0.1"
	}
	return fmt.Sprintf("nats://%s:4222", address)
}

func buildStorageSection(cfg *BootstrapConfig) map[string]any {
	storage := map[string]any{}
	switch strings.ToLower(cfg.Storage) {
	case "postgres":
		storage["backend"] = "postgresql"
		storage["postgresql"] = map[string]any{
			"dsn": buildPostgresDSN(cfg),
		}
	default:
		storage["backend"] = "sqlite"
		storage["sqlite"] = map[string]any{
			"path": sqliteDatabasePath(cfg),
		}
	}
	return storage
}

func buildTLSSection(cfg *BootstrapConfig) map[string]any {
	tls := map[string]any{}
	certPath, keyPath, caPath := resolveTLSPaths(cfg)
	if cfg.GenerateCerts || certPath != "" || keyPath != "" || caPath != "" {
		tls["enabled"] = true
	}
	if certPath != "" {
		tls["certfile"] = certPath
	}
	if keyPath != "" {
		tls["keyfile"] = keyPath
	}
	if caPath != "" {
		tls["cafile"] = caPath
	}
	return tls
}

func buildPostgresDSN(cfg *BootstrapConfig) string {
	sslMode := cfg.PostgresSSLMode
	if sslMode == "" {
		sslMode = "prefer"
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.PostgresUser,
		cfg.PostgresPassword,
		cfg.PostgresHost,
		cfg.PostgresPort,
		cfg.PostgresDatabase,
		sslMode,
	)
}

func buildAuthSection(cfg *BootstrapConfig) map[string]any {
	auth := map[string]any{}

	// Demo mode disables auth for easy testing/evaluation
	mode := strings.ToLower(cfg.Mode)
	if mode == "demo" {
		auth["enabled"] = false
		return auth
	}

	// Production modes require auth - generate a bootstrap API key
	auth["enabled"] = true
	auth["type"] = "apikey"
	auth["apikey"] = map[string]any{
		"keys": map[string]any{
			generateAPIKey(): map[string]any{
				"name": "bootstrap-admin",
				"role": "admin",
			},
		},
	}

	return auth
}

func generateAPIKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback to a fixed key if random fails (should not happen)
		return "ks-bootstrap-fallback-key-please-change"
	}
	return "ks-" + hex.EncodeToString(b)
}
