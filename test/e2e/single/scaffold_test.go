//go:build e2e

// Package single — Epic 19 task 2a shared scaffold for the
// single-topology E2E scenarios. TestMain brings the docker-compose
// topology up once before any test runs and tears it down at the end,
// so each TestE2E_* file only contains scenario logic.
//
// Configuration:
//   - KSCORE_E2E_NO_COMPOSE=1 skips the compose lifecycle (compose is
//     expected to already be up). `make e2e-test` sets this because
//     the make target manages the lifecycle itself.
//
// Helpers exported to scenario files (package-internal):
//   - serverHTTPAddr / serverGRPCAddr / postgresDSN / natsMonAddr: pinned
//     loopback ports matching docker-compose.yml.
//   - extractAdminAPIKey(t): scrapes `docker logs kscore-e2e-server`
//     for the dev-default API key the server logs at boot.
//   - dialControlPlane(t, apiKey): authenticated gRPC client.
//   - waitForHTTP / waitForCondition: polling utilities used by every
//     scenario.
package single

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

const (
	serverHTTPAddr = "http://127.0.0.1:8080"
	serverGRPCAddr = "127.0.0.1:9090"
	natsMonAddr    = "http://127.0.0.1:8222"
	postgresDSN    = "postgres://kscore:kscore@127.0.0.1:5432/kscore?sslmode=disable"

	composeReadyBudget = 90 * time.Second
	scenarioBudget     = 60 * time.Second
	pollInterval       = 1 * time.Second
)

// composeFile resolves the absolute path to docker-compose.yml so the
// test runs from any directory.
func composeFile(t testing.TB) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(cwd, "docker-compose.yml")
}

func runCompose(t testing.TB, args ...string) ([]byte, error) {
	t.Helper()
	full := append([]string{"compose", "-f", composeFile(t)}, args...)
	return exec.Command("docker", full...).CombinedOutput()
}

func TestMain(m *testing.M) {
	if os.Getenv("KSCORE_E2E_NO_COMPOSE") != "" {
		os.Exit(m.Run())
	}

	// External-driver mode: the make target sets KSCORE_E2E_NO_COMPOSE.
	// In every other case the test owns the lifecycle.
	t := &lifecycleT{}
	defer func() {
		out, err := runCompose(t, "down", "-v")
		if err != nil {
			fmt.Fprintf(os.Stderr, "compose down failed: %v\n%s\n", err, out)
		}
	}()
	out, err := runCompose(t, "up", "-d", "--wait")
	if err != nil {
		fmt.Fprintf(os.Stderr, "compose up failed: %v\n%s\n", err, out)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// lifecycleT implements testing.TB just enough for the compose helpers
// to log out of TestMain. Only Helper / Errorf / Fatalf are used.
type lifecycleT struct{ testing.TB }

func (l *lifecycleT) Helper()                            {}
func (l *lifecycleT) Errorf(f string, a ...interface{})  { fmt.Fprintf(os.Stderr, f+"\n", a...) }
func (l *lifecycleT) Fatalf(f string, a ...interface{})  { fmt.Fprintf(os.Stderr, f+"\n", a...); os.Exit(1) }
func (l *lifecycleT) Logf(f string, a ...interface{})    { fmt.Fprintf(os.Stderr, f+"\n", a...) }

// waitForHTTP polls url until it returns wantStatus or the context
// expires. Used to assert /health/* readiness.
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

// waitForCondition polls fn until it returns nil or the context
// expires. The last error from fn is wrapped in the timeout error.
func waitForCondition(ctx context.Context, what string, fn func() error) error {
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr == nil {
				lastErr = fmt.Errorf("condition never observed")
			}
			return fmt.Errorf("timed out waiting for %s: %w", what, lastErr)
		default:
		}
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(pollInterval)
	}
}

// extractAdminAPIKey scrapes `docker logs kscore-e2e-server` for the
// dev-default WARN line and pulls the cleartext API key out of it.
// The server emits this once at boot per pkg/api/apikeys.EnsureDevKey;
// it cannot be recovered any other way.
//
// Polls until a key is found or the budget expires — the line may not
// appear instantly if the server is still booting.
func extractAdminAPIKey(ctx context.Context, t testing.TB) string {
	t.Helper()
	const target = "DEV API KEY GENERATED"
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("extractAdminAPIKey: timed out (last err=%v)", lastErr)
		default:
		}
		out, err := exec.Command("docker", "logs", "kscore-e2e-server").CombinedOutput()
		if err != nil {
			lastErr = fmt.Errorf("docker logs: %v: %s", err, out)
			time.Sleep(pollInterval)
			continue
		}
		key := parseDevKey(string(out), target)
		if key != "" {
			return key
		}
		lastErr = fmt.Errorf("WARN line %q not yet present", target)
		time.Sleep(pollInterval)
	}
}

// parseDevKey scans server logs (JSON, one per line) for the line
// containing the dev-key WARN message and returns the "key" field.
func parseDevKey(logs, marker string) string {
	for _, line := range strings.Split(logs, "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		var record struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if record.Key != "" {
			return record.Key
		}
	}
	return ""
}

// dialControlPlane returns a gRPC ControlPlaneServiceClient
// authenticated with the supplied admin API key. Caller closes the
// returned io.Closer at end of test.
func dialControlPlane(t testing.TB, apiKey string) (v1.ControlPlaneServiceClient, io.Closer) {
	t.Helper()
	conn, err := grpc.NewClient(serverGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial %s: %v", serverGRPCAddr, err)
	}
	return v1.NewControlPlaneServiceClient(conn), conn
}

// dialStateService is the StateService counterpart of
// dialControlPlane. Shares the same connection target.
func dialStateService(t testing.TB, apiKey string) (v1.StateServiceClient, io.Closer) {
	t.Helper()
	conn, err := grpc.NewClient(serverGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial %s: %v", serverGRPCAddr, err)
	}
	return v1.NewStateServiceClient(conn), conn
}

// dialBlueprintService returns a BlueprintServiceClient. Used by
// scenario 4.
func dialBlueprintService(t testing.TB, apiKey string) (v1.BlueprintServiceClient, io.Closer) {
	t.Helper()
	conn, err := grpc.NewClient(serverGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial %s: %v", serverGRPCAddr, err)
	}
	return v1.NewBlueprintServiceClient(conn), conn
}

// dialPolicyService returns a PolicyServiceClient. Used by scenario 7
// (audit log query + compliance report).
func dialPolicyService(t testing.TB, apiKey string) (v1.PolicyServiceClient, io.Closer) {
	t.Helper()
	conn, err := grpc.NewClient(serverGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial %s: %v", serverGRPCAddr, err)
	}
	return v1.NewPolicyServiceClient(conn), conn
}

// adminHTTPRequest builds an HTTP request against the kscore-server
// REST surface with the admin API key attached as Bearer auth.
// Used by REST-only scenarios (8, 9).
func adminHTTPRequest(ctx context.Context, t testing.TB, apiKey, method, path string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, serverHTTPAddr+path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// dialSecretsService returns a SecretsServiceClient. Used by scenario 6.
func dialSecretsService(t testing.TB, apiKey string) (v1.SecretsServiceClient, io.Closer) {
	t.Helper()
	conn, err := grpc.NewClient(serverGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial %s: %v", serverGRPCAddr, err)
	}
	return v1.NewSecretsServiceClient(conn), conn
}

// authContext attaches the admin API key as `authorization: Bearer
// …` on outbound gRPC metadata. Matches pkg/api/auth.APIKey
// Authenticator's extractor.
func authContext(ctx context.Context, apiKey string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+apiKey)
}
