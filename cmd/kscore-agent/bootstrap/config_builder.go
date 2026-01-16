package bootstrap

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	serverConfigPath = "/etc/kscore/server.yaml"
	agentConfigPath  = "/etc/kscore/agent.yaml"
)

func buildServerConfig(cfg *BootstrapConfig) ([]byte, error) {
	config := map[string]any{
		"server":  buildServerSection(cfg),
		"nats":    buildNATSSection(cfg, true),
		"storage": buildStorageSection(cfg),
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
			embedded := map[string]any{}
			if cfg.BindAddress != "" {
				embedded["host"] = cfg.BindAddress
			}
			if len(embedded) > 0 {
				nats["embedded"] = embedded
			}
		} else {
			nats["mode"] = "external"
			nats["url"] = defaultNATSURL(cfg)
		}
	case "leaf":
		nats["mode"] = "leaf"
		embedded := map[string]any{}
		if cfg.BindAddress != "" {
			embedded["host"] = cfg.BindAddress
		}
		if len(cfg.NATSURLs) > 0 {
			embedded["leafnodeurls"] = cfg.NATSURLs
		}
		if len(embedded) > 0 {
			nats["embedded"] = embedded
		}
	case "external":
		nats["mode"] = "external"
		nats["url"] = strings.Join(cfg.NATSURLs, ",")
	default:
		if forServer {
			nats["mode"] = "embedded"
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
