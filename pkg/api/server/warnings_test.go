// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
	"go.keystone-core.io/keystone-core/pkg/api/server"
)

// productionConfig builds a Mode=production config that's risky in
// the dimensions task 9 surfaces (CORS=*, no TLS, SQLite). Tests
// override the bits they care about.
func productionConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := newTestConfig()
	cfg.Mode = config.ModeProduction
	// newTestConfig already enables CORS with [*]; leave as-is.
	return cfg
}

func TestServerProductionWarnings_DevModeEmpty(t *testing.T) {
	srv, _ := newServer(t)
	if got := srv.ProductionWarnings(); len(got) != 0 {
		t.Errorf("dev-mode warnings = %v, want none", got)
	}
}

func TestServerProductionWarnings_AuthDisabledInProd(t *testing.T) {
	cfg := productionConfig(t)
	srv, _ := newServer(t, func(o *server.Options) { o.Config = cfg })

	got := srv.ProductionWarnings()
	if !containsString(got, "auth is disabled in production (no AuthInterceptor wired)") {
		t.Errorf("warnings = %v; missing auth-disabled warning", got)
	}
}

func TestServerProductionWarnings_AuthEnabledInProd(t *testing.T) {
	cfg := productionConfig(t)
	ic := newAuthInterceptor(t,
		&stubAuthenticator{principal: &auth.Principal{ID: "x", Role: auth.RoleAdmin}},
		&stubAuthorizer{permissive: true}, nil,
	)
	srv, _ := newServer(t, func(o *server.Options) {
		o.Config = cfg
		o.AuthInterceptor = ic
	})

	got := srv.ProductionWarnings()
	for _, w := range got {
		if strings.Contains(w, "auth is disabled") {
			t.Errorf("auth-disabled warning emitted with interceptor wired: %v", got)
		}
	}
}

func TestServerProductionWarnings_BannerIncludesWarnings(t *testing.T) {
	log, buf := captureLogger(t)
	cfg := productionConfig(t)
	srv, _ := newServer(t, func(o *server.Options) {
		o.Config = cfg
		o.Logger = log
	})
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	// Banner is emitted via slog.Info(string) — captured as a single
	// message attr. Walk the lines and assert the banner contains a
	// WARNING line for auth.
	lines := parseLogLines(t, buf)
	var bannerMsg string
	for _, l := range lines {
		if msg, _ := l["msg"].(string); strings.HasPrefix(msg, "kscore-server ") {
			bannerMsg = msg
			break
		}
	}
	if bannerMsg == "" {
		t.Fatalf("banner not found in logs: %s", buf.String())
	}
	if !strings.Contains(bannerMsg, "WARNING") {
		t.Errorf("banner missing WARNING line: %s", bannerMsg)
	}
	if !strings.Contains(bannerMsg, "auth is disabled") {
		t.Errorf("banner missing auth-disabled warning: %s", bannerMsg)
	}
}

func TestAPIStatus_IncludesProductionWarningsAndAuthMode(t *testing.T) {
	cfg := productionConfig(t)
	// Use permissive auth so /api/status is reachable.
	ic := newAuthInterceptor(t,
		&stubAuthenticator{principal: &auth.Principal{ID: "x", Role: auth.RoleAdmin}},
		&stubAuthorizer{permissive: true}, nil,
	)
	srv, _ := newServer(t, func(o *server.Options) {
		o.Config = cfg
		o.AuthInterceptor = ic
	})
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	resp, err := http.Get("http://" + srv.Addrs().HTTP + "/api/status")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("body not JSON: %v\n%s", err, body)
	}

	if got, _ := payload["auth_mode"].(string); got != "enabled" {
		t.Errorf("auth_mode = %q, want enabled", got)
	}
	warnings, ok := payload["production_warnings"].([]any)
	if !ok {
		t.Fatalf("production_warnings missing or not array: %T %v", payload["production_warnings"], payload["production_warnings"])
	}
	if len(warnings) == 0 {
		t.Errorf("no production_warnings; expected CORS=*, TLS off, SQLite")
	}
}

func TestAPIStatus_ProductionWarningsAlwaysArrayInDev(t *testing.T) {
	// Dev mode → no warnings → field is []  (never null) so dashboards
	// can render without nil-checks.
	srv, _ := newServer(t)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	resp, err := http.Get("http://" + srv.Addrs().HTTP + "/api/status")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if !strings.Contains(string(body), `"production_warnings":[]`) {
		t.Errorf("body missing empty-array production_warnings: %s", body)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
