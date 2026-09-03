// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package single

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	natsserver "github.com/nats-io/nats-server/v2/server"
)

// startNativeStack brings the single-topology test stack up using host
// subprocesses + an embedded NATS server. Postgres is supplied by the
// caller via KSCORE_TEST_POSTGRES_DSN (same env var the integration job
// uses). Returns a cleanup func that stops the processes and removes
// the scratch directory; safe to call even on partial-startup failure.
//
// Pinned ports (must be free on the host running the tests):
//   - 8080 server REST / health
//   - 5397 server gRPC
//   - 8081 gitops webhook receiver
//
// Dynamic ports (looked up via the OS, threaded into config templates):
//   - NATS client port
//   - NATS HTTP monitor port
func startNativeStack() (cleanup func(), err error) {
	dsn := os.Getenv("KSCORE_TEST_POSTGRES_DSN")
	if dsn == "" {
		return nil, fmt.Errorf("native e2e requires KSCORE_TEST_POSTGRES_DSN " +
			"(same env var used by `make test-integration`; set it to a reachable " +
			"postgres for this test run, or set KSCORE_E2E_USE_DOCKER=1 to use the " +
			"docker-compose stack instead)")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("locate repo root: %w", err)
	}

	// Fail loudly if the pinned server ports are already in use — a
	// leftover docker-compose stack would otherwise let the test's
	// /health/ready probe succeed against the orphan, then surface as
	// a confusing 401 when the test hits any authed endpoint with the
	// new server's dev key against the orphan's API key store.
	for _, port := range []int{8080, 5397, 8081} {
		if err := assertPortFree(port); err != nil {
			return nil, fmt.Errorf("native mode requires port %d free: %w "+
				"(hint: `docker compose -f test/e2e/single/docker-compose.yml down -v` "+
				"if you previously ran `make e2e-test-docker`)", port, err)
		}
	}

	// Wipe the target DB so the server's first-run dev-key bootstrap
	// (EnsureDevKey) actually generates + logs a fresh cleartext key.
	// Without this, a stale dev key from a previous run leaves the
	// WARN line absent and extractAdminAPIKey times out.
	if err := wipePostgresSchema(dsn); err != nil {
		return nil, fmt.Errorf("wipe postgres schema: %w", err)
	}

	scratch, err := os.MkdirTemp("", "kscore-e2e-native-")
	if err != nil {
		return nil, fmt.Errorf("scratch dir: %w", err)
	}

	// Track everything we need to dismantle; the closure runs in
	// reverse order on error or test completion.
	var (
		shutdown []func()
		procs    []*exec.Cmd
	)
	cleanup = func() {
		// Kill subprocesses first so they don't keep file handles
		// open against the scratch dir.
		for _, p := range procs {
			if p.Process != nil {
				_ = p.Process.Kill()
				_, _ = p.Process.Wait()
			}
		}
		for i := len(shutdown) - 1; i >= 0; i-- {
			shutdown[i]()
		}
		_ = os.RemoveAll(scratch)
	}
	// On any error before we return, run cleanup to avoid leaks.
	defer func() {
		if err != nil {
			cleanup()
		}
	}()

	// --- embedded NATS (with JetStream + HTTP monitor) ----------------------
	natsPort, err := freeLoopbackPort()
	if err != nil {
		return cleanup, fmt.Errorf("alloc nats port: %w", err)
	}
	natsMonPort, err := freeLoopbackPort()
	if err != nil {
		return cleanup, fmt.Errorf("alloc nats monitor port: %w", err)
	}
	natsStore := filepath.Join(scratch, "nats")
	if err := os.MkdirAll(natsStore, 0o700); err != nil {
		return cleanup, fmt.Errorf("nats store dir: %w", err)
	}
	// Deliberately NO JetStreamMaxStore / JetStreamMaxMemory here.
	//
	// Bounding the embedded server's JetStream budget looks like the
	// obvious way to stop nats-server deriving one from free disk, and it
	// does stop that — but it also stops the agents connecting at all.
	// Every scenario then fails with "agent-1 not yet connected" after a
	// 60s wait, with no error logged on either side. Measured, not
	// assumed:
	//
	//   budget none  + 64 MiB streams -> pass
	//   budget 512M  + 64 MiB streams -> agents never connect
	//   budget 8 GiB + 64 MiB streams -> agents never connect
	//
	// Not a size threshold: 8 GiB fails the same way 512 MiB does, so it
	// is setting the limit at all that does it. The mechanism is not
	// understood — plausibly nats-server configures the global account
	// differently once explicit JetStream limits are present.
	//
	// Bounding the STREAMS instead (test/e2e/single/config/server.yaml)
	// achieves the same goal without this: the streams ask for 64 MiB
	// rather than the production 10 GiB, so any plausible derived budget
	// is enough.
	natsSrv, err := natsserver.NewServer(&natsserver.Options{
		Host:      "127.0.0.1",
		Port:      natsPort,
		HTTPPort:  natsMonPort,
		JetStream: true,
		StoreDir:  natsStore,
		NoSigs:    true,
		NoLog:     true,
	})
	if err != nil {
		return cleanup, fmt.Errorf("nats new server: %w", err)
	}
	go natsSrv.Start()
	if !natsSrv.ReadyForConnections(10 * time.Second) {
		return cleanup, fmt.Errorf("nats not ready within 10s")
	}
	shutdown = append(shutdown, func() {
		natsSrv.Shutdown()
		natsSrv.WaitForShutdown()
	})
	natsURL := fmt.Sprintf("nats://127.0.0.1:%d", natsPort)
	natsMonAddr = fmt.Sprintf("http://127.0.0.1:%d", natsMonPort)
	postgresDSN = dsn
	// Native mode runs the agent as a host subprocess; pick a command
	// that exists on any Linux/macOS host. The test only cares about
	// exit-0; payload is irrelevant.
	commandExecBin = "/bin/echo"
	commandExecArgs = []string{"native-e2e"}
	// Server runs on the host (not in a container), so the outbound
	// webhook receiver is directly reachable on loopback.
	webhookReceiverHost = "127.0.0.1"

	// --- build kscore-server + kscore-agent binaries -----------------------
	binDir := filepath.Join(scratch, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return cleanup, fmt.Errorf("bin dir: %w", err)
	}
	serverBin := filepath.Join(binDir, "kscore-server")
	agentBin := filepath.Join(binDir, "kscore-agent")
	if err := buildBinary(repoRoot, "./cmd/kscore-server", serverBin); err != nil {
		return cleanup, fmt.Errorf("build kscore-server: %w", err)
	}
	if err := buildBinary(repoRoot, "./cmd/kscore-agent", agentBin); err != nil {
		return cleanup, fmt.Errorf("build kscore-agent: %w", err)
	}

	// --- render config files for native paths/ports ------------------------
	configDir := filepath.Join(scratch, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return cleanup, fmt.Errorf("config dir: %w", err)
	}
	dataDir := filepath.Join(scratch, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return cleanup, fmt.Errorf("data dir: %w", err)
	}

	serverData := filepath.Join(dataDir, "server")
	if err := os.MkdirAll(serverData, 0o700); err != nil {
		return cleanup, fmt.Errorf("server data dir: %w", err)
	}
	agent1Data := filepath.Join(dataDir, "agent-1")
	if err := os.MkdirAll(agent1Data, 0o700); err != nil {
		return cleanup, fmt.Errorf("agent-1 data dir: %w", err)
	}
	agent2Data := filepath.Join(dataDir, "agent-2")
	if err := os.MkdirAll(agent2Data, 0o700); err != nil {
		return cleanup, fmt.Errorf("agent-2 data dir: %w", err)
	}

	blueprintsSrc := filepath.Join(repoRoot, "test", "e2e", "single", "blueprints")
	configsSrc := filepath.Join(repoRoot, "test", "e2e", "single", "config")

	serverCfg := filepath.Join(configDir, "server.yaml")
	if err := renderConfig(filepath.Join(configsSrc, "server.yaml"), serverCfg, map[string]string{
		"nats://nats:4222": natsURL,
		"postgres://kscore:kscore@postgres:5432/kscore?sslmode=disable": dsn,
		"/data/jetstream": filepath.Join(serverData, "jetstream"),
		"/data/secrets":   filepath.Join(serverData, "secrets"),
		"/blueprints":     blueprintsSrc,
	}); err != nil {
		return cleanup, err
	}
	agent1Cfg := filepath.Join(configDir, "agent-1.yaml")
	if err := renderConfig(filepath.Join(configsSrc, "agent-1.yaml"), agent1Cfg, map[string]string{
		"nats://nats:4222": natsURL,
		"/data/agent.db":   filepath.Join(agent1Data, "agent.db"),
	}); err != nil {
		return cleanup, err
	}
	agent2Cfg := filepath.Join(configDir, "agent-2.yaml")
	if err := renderConfig(filepath.Join(configsSrc, "agent-2.yaml"), agent2Cfg, map[string]string{
		"nats://nats:4222": natsURL,
		"/data/agent.db":   filepath.Join(agent2Data, "agent.db"),
	}); err != nil {
		return cleanup, err
	}

	// --- launch kscore-server (capture stdout/stderr into a shared buffer) -
	serverBuf := newSafeBuffer()
	serverLogReader = serverBuf.String
	serverCmd, err := spawn(serverBin, serverCfg, serverBuf)
	if err != nil {
		return cleanup, fmt.Errorf("spawn kscore-server: %w", err)
	}
	procs = append(procs, serverCmd)

	// Wait until /health/ready returns 200. The kscore-server boots
	// non-trivial subsystems (jetstream consumer, secrets, blueprints,
	// gitops listener) so give it a generous budget.
	readyCtx, readyCancel := context.WithTimeout(context.Background(), composeReadyBudget)
	defer readyCancel()
	if err := waitForHTTP(readyCtx, serverHTTPAddr+"/health/ready", 200); err != nil {
		logs, _ := serverBuf.String()
		return cleanup, fmt.Errorf("kscore-server /health/ready: %w\n--- server logs ---\n%s",
			err, logs)
	}

	// --- launch the two kscore-agents --------------------------------------
	agent1Buf := newSafeBuffer()
	agent1Cmd, err := spawn(agentBin, agent1Cfg, agent1Buf)
	if err != nil {
		return cleanup, fmt.Errorf("spawn agent-1: %w", err)
	}
	procs = append(procs, agent1Cmd)

	agent2Buf := newSafeBuffer()
	agent2Cmd, err := spawn(agentBin, agent2Cfg, agent2Buf)
	if err != nil {
		return cleanup, fmt.Errorf("spawn agent-2: %w", err)
	}
	procs = append(procs, agent2Cmd)

	return cleanup, nil
}

// findRepoRoot walks up from the test working directory until it finds
// a go.mod, matching what `go run` / `go test` does implicitly. Used
// to resolve config and blueprint asset paths in native mode.
func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in any parent of %s", cwd)
		}
		dir = parent
	}
}

// buildBinary compiles a cmd package into outPath, invoked from repoRoot.
// Equivalent to `go build -o outPath pkgPath` with the test process's
// Go toolchain. Errors carry compiler stdout/stderr.
func buildBinary(repoRoot, pkgPath, outPath string) error {
	cmd := exec.Command("go", "build", "-o", outPath, pkgPath)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build %s: %v: %s", pkgPath, err, out)
	}
	return nil
}

// renderConfig reads src, applies the supplied string-for-string
// substitutions (literal — no regex, no template syntax), and writes
// the result to dst. Used to retarget the docker-compose-shaped
// configs at native ports + host paths.
func renderConfig(src, dst string, subs map[string]string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	content := string(in)
	for find, replace := range subs {
		content = strings.ReplaceAll(content, find, replace)
	}
	if err := os.WriteFile(dst, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// spawn runs `bin --config=<cfg>` with stdout+stderr captured into the
// supplied buffer. Caller owns the *exec.Cmd lifecycle (Kill + Wait).
func spawn(bin, cfg string, out *safeBuffer) (*exec.Cmd, error) {
	cmd := exec.Command(bin, "--config="+cfg)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// wipePostgresSchema drops + recreates the `public` schema in the
// target database, returning it to a fresh state. Matches what the
// docker-compose path gets implicitly from a fresh postgres container.
func wipePostgresSchema(dsn string) error {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return err
	}
	return nil
}

// assertPortFree returns nil iff nothing is currently listening on the
// supplied loopback TCP port. Distinguishes "port in use" from other
// listen errors so the caller can surface a tailored hint.
func assertPortFree(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("already in use: %w", err)
	}
	return l.Close()
}

// freeLoopbackPort asks the kernel for a free TCP port on loopback by
// binding to :0 and immediately closing. A small TOCTOU window remains
// (another process could grab the port between Close and reuse), but
// it's good enough for E2E lifecycle.
func freeLoopbackPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

// safeBuffer is a mutex-guarded bytes.Buffer. Subprocesses write into
// it from one goroutine while the test reads via String() from another;
// bytes.Buffer alone isn't safe for that pattern.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func newSafeBuffer() *safeBuffer { return &safeBuffer{} }

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String(), nil
}
