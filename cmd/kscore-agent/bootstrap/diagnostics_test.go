package bootstrap

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderConfigSummaryRedactsSecrets(t *testing.T) {
	cfg := &BootstrapConfig{
		Mode:             "production",
		ClusterName:      "alpha",
		NodeRole:         "control-plane",
		Join:             "https://example.com:8443",
		JoinToken:        "super-secret-token",
		Storage:          "postgres",
		PostgresHost:     "db.local",
		PostgresUser:     "kscore",
		PostgresPassword: "db-secret",
		NATSMode:         "external",
		NATSURLs:         []string{"nats://nats.local:4222"},
		NATSPassword:     "nats-secret",
	}

	summary := renderConfigSummary(cfg)
	if strings.Contains(summary, cfg.JoinToken) {
		t.Fatalf("expected join token to be redacted, got: %s", summary)
	}
	if strings.Contains(summary, cfg.PostgresPassword) {
		t.Fatalf("expected postgres password to be redacted, got: %s", summary)
	}
	if strings.Contains(summary, cfg.NATSPassword) {
		t.Fatalf("expected nats password to be redacted, got: %s", summary)
	}
	if !strings.Contains(summary, "join token: configured") {
		t.Fatalf("expected join token hint, got: %s", summary)
	}
}

func TestDiagnosticHints(t *testing.T) {
	cfg := &BootstrapConfig{
		Storage:  "postgres",
		NATSMode: "external",
	}
	hints := diagnosticHints(cfg, errors.New("permission denied"))
	if !containsHint(hints, "run bootstrap as root or with sudo") {
		t.Fatalf("expected permission hint, got: %v", hints)
	}
	hints = diagnosticHints(cfg, errors.New("postgres host is required"))
	if !containsHint(hints, "set --postgres-host or KSCORE_POSTGRES_HOST for postgres storage") {
		t.Fatalf("expected postgres hint, got: %v", hints)
	}
}

func containsHint(hints []string, want string) bool {
	for _, hint := range hints {
		if hint == want {
			return true
		}
	}
	return false
}
