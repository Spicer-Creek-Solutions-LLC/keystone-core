//go:build e2e

// Package single contains the Epic 19 task 1 single-topology E2E
// harness validation. The test asserts that the docker-compose
// topology (1× kscore-server + 2× kscore-agent + Postgres + NATS)
// comes up cleanly so that task 2 can layer the nine feature
// scenarios (agent registration, command exec, state apply, blueprint
// apply, module install, secrets, audit, outbound webhook, GitOps)
// on top of it.
//
// Scope discipline: this file proves the *harness* — task 1 — not
// any feature scenario. Specifically it asserts:
//
//   1. kscore-server /health/live returns 200 (process alive).
//   2. kscore-server /health/ready returns 200 within a budget
//      (NATS + Postgres reachable from the server).
//   3. Postgres schema was applied (the agents table exists),
//      proving state.NewStore succeeded inside the container.
//   4. NATS monitoring /healthz returns 200 (broker healthy).
//
// Agent registration — whether agent-1 and agent-2 appear in
// ListAgents — is scenario 1 of task 2 and is deliberately NOT
// asserted here. See README.md for the task 1 / task 2 boundary.
//
// The test brings the topology up only when KSCORE_E2E_NO_COMPOSE is
// unset; CI and `make e2e-test` set up + tear down at the make
// layer, in which case the test just exercises the running
// services. This keeps the test runnable two ways.
package single

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

const (
	serverReadyBudget = 90 * time.Second
	pollInterval      = 1 * time.Second

	serverHTTPAddr = "http://127.0.0.1:8080"
	natsMonAddr    = "http://127.0.0.1:8222"
	postgresDSN    = "postgres://kscore:kscore@127.0.0.1:5432/kscore?sslmode=disable"
)

// composeFile resolves to test/e2e/single/docker-compose.yml regardless
// of which directory `go test` is run from.
func composeFile(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(cwd, "docker-compose.yml")
}

// runCompose execs `docker compose -f <file> <args...>`. Returns the
// command's combined output for diagnostics.
func runCompose(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()
	full := append([]string{"compose", "-f", composeFile(t)}, args...)
	cmd := exec.Command("docker", full...)
	return cmd.CombinedOutput()
}

// manageCompose brings the topology up at test start and tears it
// down at test end. Skipped when KSCORE_E2E_NO_COMPOSE is set so
// `make e2e-test` (which manages compose lifecycle at the make
// layer) doesn't double-up.
func manageCompose(t *testing.T) {
	t.Helper()
	if os.Getenv("KSCORE_E2E_NO_COMPOSE") != "" {
		t.Log("KSCORE_E2E_NO_COMPOSE set — skipping compose up/down; topology must already be running")
		return
	}

	t.Log("compose up -d (this builds images on first run; can take 60-90s)")
	out, err := runCompose(t, "up", "-d", "--wait")
	if err != nil {
		t.Fatalf("compose up failed: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		t.Log("compose down -v")
		out, err := runCompose(t, "down", "-v")
		if err != nil {
			t.Errorf("compose down failed: %v\n%s", err, out)
		}
	})
}

func TestSingleTopologyBoots(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e: skipping under -short")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("e2e: docker not on PATH")
	}

	manageCompose(t)

	ctx, cancel := context.WithTimeout(context.Background(), serverReadyBudget)
	defer cancel()

	t.Run("server /health/live", func(t *testing.T) {
		if err := waitForHTTP(ctx, serverHTTPAddr+"/health/live", 200); err != nil {
			t.Fatalf("server /health/live: %v", err)
		}
	})

	t.Run("server /health/ready", func(t *testing.T) {
		if err := waitForHTTP(ctx, serverHTTPAddr+"/health/ready", 200); err != nil {
			t.Fatalf("server /health/ready: %v", err)
		}
	})

	t.Run("nats /healthz", func(t *testing.T) {
		if err := waitForHTTP(ctx, natsMonAddr+"/healthz", 200); err != nil {
			t.Fatalf("nats /healthz: %v", err)
		}
	})

	t.Run("postgres schema applied", func(t *testing.T) {
		if err := waitForPostgresSchema(ctx); err != nil {
			t.Fatalf("postgres schema check: %v", err)
		}
	})
}

// waitForHTTP polls url until it returns wantStatus or the context
// expires. The first error after the deadline is returned with the
// most recent response status for diagnostics.
func waitForHTTP(ctx context.Context, url string, wantStatus int) error {
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	var lastStatus int
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s (last status=%d, last err=%v)",
				url, lastStatus, lastErr)
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(pollInterval)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		lastStatus = resp.StatusCode
		if resp.StatusCode == wantStatus {
			return nil
		}
		time.Sleep(pollInterval)
	}
}

// waitForPostgresSchema polls until the agents table is present,
// proving state.NewStore inside kscore-server has run applySchema
// successfully against the shared Postgres.
func waitForPostgresSchema(ctx context.Context) error {
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for postgres schema (last err=%v)", lastErr)
		default:
		}

		db, err := sql.Open("postgres", postgresDSN)
		if err != nil {
			lastErr = err
			time.Sleep(pollInterval)
			continue
		}

		queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		var n int
		err = db.QueryRowContext(queryCtx,
			`SELECT 1 FROM information_schema.tables WHERE table_name = 'agents'`,
		).Scan(&n)
		cancel()
		_ = db.Close()

		switch {
		case err == nil && n == 1:
			return nil
		case err == sql.ErrNoRows:
			lastErr = fmt.Errorf("agents table not present yet")
		default:
			lastErr = err
		}
		time.Sleep(pollInterval)
	}
}
