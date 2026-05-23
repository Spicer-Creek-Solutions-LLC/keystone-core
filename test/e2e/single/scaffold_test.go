//go:build e2e

// Package single — Epic 19 task 2a shared scaffold for the
// single-topology E2E scenarios. TestMain brings the topology up once
// before any test runs and tears it down at the end, so each
// TestE2E_* file only contains scenario logic.
//
// Two lifecycle modes, selected by env var:
//   - native (default): builds the kscore binaries, embeds NATS, uses
//     the Postgres pointed to by KSCORE_TEST_POSTGRES_DSN, and runs
//     the binaries as host subprocesses. No docker required.
//   - docker (KSCORE_E2E_USE_DOCKER=1): brings docker-compose.yml up
//     and exercises the production-shaped Dockerfile.kscore images.
//     For local dev that wants container-image coverage.
//   - external (KSCORE_E2E_NO_COMPOSE=1): assumes the topology is
//     already running (back-compat with the legacy make target that
//     managed compose externally).
//
// Helpers exported to scenario files (package-internal):
//   - serverHTTPAddr / serverGRPCAddr: pinned ports (same in both modes).
//   - postgresDSN / natsMonAddr: set at startup by TestMain (native
//     uses KSCORE_TEST_POSTGRES_DSN + the embedded NATS monitor port).
//   - extractAdminAPIKey(t): reads the dev-default API key the server
//     logs at boot (docker mode: docker logs; native mode: captured stdout).
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

	composeReadyBudget = 90 * time.Second
	scenarioBudget     = 60 * time.Second
	pollInterval       = 1 * time.Second
)

// postgresDSN and natsMonAddr are populated by TestMain depending on
// the active lifecycle mode. Scenario code reads them as if they were
// pinned constants.
var (
	postgresDSN string
	natsMonAddr string
)

// serverLogReader returns the kscore-server's structured-JSON log
// stream. Populated by TestMain for the active mode (docker mode reads
// `docker logs kscore-e2e-server`; native mode reads the captured
// subprocess stdout/stderr buffer).
var serverLogReader func() (string, error)

// Per-mode command + host vars. Set by TestMain. Docker mode targets
// the distroless container layout (/usr/local/bin/kscore) and uses the
// docker host-gateway alias; native mode runs on the host so any
// universal command works and the receiver lives on loopback.
var (
	// commandExecBin + commandExecArgs are what TestE2E_CommandExec
	// asks the agent to execute. The test only cares about exit-0.
	commandExecBin  string
	commandExecArgs []string

	// webhookReceiverHost is the hostname/IP the outbound-webhook
	// scenario tells the kscore-server to POST back to. In docker the
	// server reaches the host via host-gateway; in native the server
	// IS on the host.
	webhookReceiverHost string
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
	// External-driver mode: caller already brought the topology up.
	// Defaults to docker-mode addresses for back-compat with the legacy
	// `make e2e-test` workflow that managed compose externally.
	if os.Getenv("KSCORE_E2E_NO_COMPOSE") != "" {
		setDockerModeAddresses()
		serverLogReader = readDockerServerLogs
		os.Exit(m.Run())
	}

	if os.Getenv("KSCORE_E2E_USE_DOCKER") != "" {
		setDockerModeAddresses()
		serverLogReader = readDockerServerLogs
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

	// Default: native-process mode. No docker required.
	cleanup, err := startNativeStack()
	if err != nil {
		fmt.Fprintf(os.Stderr, "native stack startup failed: %v\n", err)
		if cleanup != nil {
			cleanup()
		}
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// setDockerModeAddresses pins the addresses + per-mode command/host
// values for docker-compose mode. Mirrors the previous package-const
// values.
func setDockerModeAddresses() {
	postgresDSN = "postgres://kscore:kscore@127.0.0.1:5432/kscore?sslmode=disable"
	natsMonAddr = "http://127.0.0.1:8222"
	commandExecBin = "/usr/local/bin/kscore"
	commandExecArgs = []string{"--version"}
	webhookReceiverHost = "host.docker.internal"
}

// readDockerServerLogs is the docker-mode serverLogReader: it shells
// out to `docker logs` on the named server container.
func readDockerServerLogs() (string, error) {
	out, err := exec.Command("docker", "logs", "kscore-e2e-server").CombinedOutput()
	return string(out), err
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

// extractAdminAPIKey reads the kscore-server's log stream for the
// dev-default WARN line and pulls the cleartext API key out of it.
// The server emits this once at boot per pkg/api/apikeys.EnsureDevKey;
// it cannot be recovered any other way. The log-source indirection
// lives in serverLogReader so docker-mode (docker logs) and
// native-mode (captured subprocess stdout) share this helper.
//
// Polls until a key is found or the budget expires — the line may not
// appear instantly if the server is still booting.
func extractAdminAPIKey(ctx context.Context, t testing.TB) string {
	t.Helper()
	if serverLogReader == nil {
		t.Fatalf("extractAdminAPIKey: serverLogReader not initialized (TestMain bug)")
	}
	const target = "DEV API KEY GENERATED"
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("extractAdminAPIKey: timed out (last err=%v)", lastErr)
		default:
		}
		logs, err := serverLogReader()
		if err != nil {
			lastErr = fmt.Errorf("read server logs: %v", err)
			time.Sleep(pollInterval)
			continue
		}
		key := parseDevKey(logs, target)
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
