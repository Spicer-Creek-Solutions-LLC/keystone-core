// SPDX-License-Identifier: Apache-2.0

package secrets_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	cli "go.keystone-core.io/keystone-core/internal/cli/secrets"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	internalsecrets "go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// cliRig boots the real SecretsGRPCServer over bufconn and exposes
// a Deps that the CLI's NewCommand can use.
type cliRig struct {
	listen   *bufconn.Listener
	grpc     *grpc.Server
	backend  *cliFakeBackend
	transit  *cliFakeTransit
}

func newCLIRig(t *testing.T) *cliRig {
	t.Helper()

	store, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "store.db")},
	})
	if err != nil {
		t.Fatalf("state.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	backend := &cliFakeBackend{
		name: "test",
		caps: []internalsecrets.BackendCapability{
			internalsecrets.CapKV, internalsecrets.CapList,
			internalsecrets.CapDynamic, internalsecrets.CapLeaseRenew, internalsecrets.CapLeaseRevoke,
		},
		entries: make(map[string]*internalsecrets.Secret),
		leases:  make(map[string]struct{}),
	}

	router, err := internalsecrets.NewRouter([]internalsecrets.Route{
		{Prefix: "kv/", Backend: "test"},
		{Prefix: "database/", Backend: "test"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	lm, err := internalsecrets.NewLeaseManager(internalsecrets.LeaseManagerConfig{Store: store})
	if err != nil {
		t.Fatalf("NewLeaseManager: %v", err)
	}
	broker, err := internalsecrets.NewBroker(internalsecrets.BrokerConfig{
		Router:         router,
		Backends:       []internalsecrets.SecretBackend{backend},
		DefaultBackend: "test",
		LeaseDirectory: lm,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	lm.SetRenewer(broker.RenewLease)

	transit := &cliFakeTransit{}
	server := controlplane.NewSecretsGRPCServer(broker, transit, lm)

	listener := bufconn.Listen(1 << 20)
	grpcSrv := grpc.NewServer()
	v1.RegisterSecretsServiceServer(grpcSrv, server)
	go func() {
		if err := grpcSrv.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("grpc serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcSrv.Stop()
		_ = listener.Close()
	})

	return &cliRig{
		listen:  listener,
		grpc:    grpcSrv,
		backend: backend,
		transit: transit,
	}
}

// deps returns a Deps wired to talk to the rig's bufconn.
func (r *cliRig) deps() cli.Deps {
	return cli.Deps{
		Dial: func(_ context.Context, _, _ string) (v1.SecretsServiceClient, io.Closer, error) {
			conn, err := grpc.NewClient(
				"passthrough://bufnet",
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return r.listen.DialContext(ctx)
				}),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return nil, nil, err
			}
			return v1.NewSecretsServiceClient(conn), conn, nil
		},
	}
}

// runCmd executes the CLI with args + the rig's Deps, capturing
// stdout + stderr.
func (r *cliRig) runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewCommand(r.deps())
	cmd.SetArgs(args)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	err := cmd.Execute()
	return buf.String(), err
}

// ---- get / put / delete / list ----------------------------------

func TestCLI_PutGet_RoundTrip(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)

	out, err := r.runCmd(t, "put", "kv/app/db", "--data", "password=hunter2", "--label", "env=prod")
	if err != nil {
		t.Fatalf("put: %v\n%s", err, out)
	}
	if !strings.Contains(out, "kv/app/db") {
		t.Errorf("put output missing path: %s", out)
	}

	// Default get hides cleartext.
	out, err = r.runCmd(t, "get", "kv/app/db")
	if err != nil {
		t.Fatalf("get: %v\n%s", err, out)
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("default get leaked cleartext: %s", out)
	}
	if !strings.Contains(out, "password=***") {
		t.Errorf("masked output missing: %s", out)
	}

	// --show-cleartext prints the value.
	out, err = r.runCmd(t, "get", "kv/app/db", "--show-cleartext")
	if err != nil {
		t.Fatalf("get cleartext: %v\n%s", err, out)
	}
	if !strings.Contains(out, "password=hunter2") {
		t.Errorf("cleartext output missing: %s", out)
	}
}

func TestCLI_Get_NotFound(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)

	_, err := r.runCmd(t, "get", "kv/missing")
	if err == nil {
		t.Errorf("get on missing returned nil err")
	}
}

func TestCLI_Get_JSONOutput(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, _ = r.runCmd(t, "put", "kv/app", "--data", "k=v")

	out, err := r.runCmd(t, "-o", "json", "get", "kv/app")
	if err != nil {
		t.Fatalf("get json: %v\n%s", err, out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("JSON output not valid JSON: %v\n%s", err, out)
	}
	if _, ok := parsed["metadata"]; !ok {
		t.Errorf("JSON output missing metadata: %#v", parsed)
	}
}

func TestCLI_Put_RequiresData(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "put", "kv/x")
	if err == nil {
		t.Errorf("put without --data returned nil err")
	}
}

func TestCLI_Put_BadKeyVal(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "put", "kv/x", "--data", "no-equals-sign")
	if err == nil {
		t.Errorf("put with malformed --data returned nil err")
	}
}

func TestCLI_Put_WithTTL(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	out, err := r.runCmd(t, "put", "kv/x", "--data", "k=v", "--ttl", "60s")
	if err != nil {
		t.Fatalf("put with ttl: %v\n%s", err, out)
	}
	// TTL goes into Metadata; verify via backend state.
	r.backend.mu.Lock()
	stored := r.backend.entries["kv/x"]
	r.backend.mu.Unlock()
	if stored.Metadata["ttl_seconds"] != "60" {
		t.Errorf("ttl_seconds metadata = %q, want 60", stored.Metadata["ttl_seconds"])
	}
}

func TestCLI_Delete(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, _ = r.runCmd(t, "put", "kv/x", "--data", "k=v")

	out, err := r.runCmd(t, "delete", "kv/x")
	if err != nil {
		t.Fatalf("delete: %v\n%s", err, out)
	}
	if !strings.Contains(out, `deleted "kv/x"`) {
		t.Errorf("delete output: %s", out)
	}

	// Subsequent get errors with NotFound.
	if _, err := r.runCmd(t, "get", "kv/x"); err == nil {
		t.Errorf("get after delete returned nil err")
	}
}

func TestCLI_List(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	for _, p := range []string{"a", "b", "c"} {
		_, _ = r.runCmd(t, "put", "kv/"+p, "--data", "k=v")
	}

	out, err := r.runCmd(t, "list", "--prefix", "kv/")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	for _, p := range []string{"kv/a", "kv/b", "kv/c"} {
		if !strings.Contains(out, p) {
			t.Errorf("list missing %q\n%s", p, out)
		}
	}
}

func TestCLI_List_Empty(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	out, err := r.runCmd(t, "list", "--prefix", "kv/never")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no secrets") {
		t.Errorf("empty list output: %s", out)
	}
}

// ---- leases -------------------------------------------------------

func TestCLI_Leases_Flow(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)

	// Issue a dynamic secret via the in-process broker rig so the
	// lease shows up in the manager.
	r.backend.mu.Lock()
	r.backend.leases["lease-1"] = struct{}{}
	r.backend.mu.Unlock()

	// List (broker hasn't populated the lease directory yet — the
	// backend's IssueDynamicSecret never went through the CLI).
	// This still exercises the empty path.
	out, err := r.runCmd(t, "leases", "list")
	if err != nil {
		t.Fatalf("leases list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no leases") {
		t.Errorf("expected 'no leases' output: %s", out)
	}

	// GET on unknown lease — gRPC NotFound; the CLI surfaces it.
	if _, err := r.runCmd(t, "leases", "get", "ghost"); err == nil {
		t.Errorf("leases get ghost returned nil err")
	}

	// Renew ghost → error.
	if _, err := r.runCmd(t, "leases", "renew", "ghost"); err == nil {
		t.Errorf("leases renew ghost returned nil err")
	}

	// Revoke ghost → error (broker returns NotFound; the CLI
	// surfaces it; idempotency is internal to the backend).
	if _, err := r.runCmd(t, "leases", "revoke", "ghost"); err == nil {
		t.Errorf("leases revoke ghost returned nil err")
	}
}

// ---- transit ------------------------------------------------------

func TestCLI_Transit_EncryptDecrypt(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)

	out, err := r.runCmd(t, "transit", "encrypt", "k", "--plaintext", "hello")
	if err != nil {
		t.Fatalf("encrypt: %v\n%s", err, out)
	}
	ciphertext := strings.TrimSpace(out)
	if !strings.HasPrefix(ciphertext, "vault:v1:") {
		t.Fatalf("encrypt output does not look like vault wire format: %q", ciphertext)
	}

	out, err = r.runCmd(t, "transit", "decrypt", "k", "--ciphertext", ciphertext, "--as-string")
	if err != nil {
		t.Fatalf("decrypt: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Errorf("decrypt = %q, want hello", strings.TrimSpace(out))
	}
}

func TestCLI_Transit_Encrypt_RequiresPlaintext(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	if _, err := r.runCmd(t, "transit", "encrypt", "k"); err == nil {
		t.Errorf("encrypt without plaintext returned nil err")
	}
}

func TestCLI_Transit_Encrypt_ConflictingSources(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "transit", "encrypt", "k", "--plaintext", "a", "--plaintext-hex", "61")
	if err == nil {
		t.Errorf("two plaintext sources returned nil err")
	}
}

func TestCLI_Transit_Encrypt_HexInput(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	// "hello" = 68656c6c6f
	out, err := r.runCmd(t, "transit", "encrypt", "k", "--plaintext-hex", "68656c6c6f")
	if err != nil {
		t.Fatalf("encrypt hex: %v\n%s", err, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "vault:v1:") {
		t.Errorf("encrypt hex output: %s", out)
	}
}

func TestCLI_Transit_SignVerify(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)

	out, err := r.runCmd(t, "transit", "sign", "k", "--message", "p")
	if err != nil {
		t.Fatalf("sign: %v\n%s", err, out)
	}
	sig := strings.TrimSpace(out)

	out, err = r.runCmd(t, "transit", "verify", "k", "--message", "p", "--signature", sig)
	if err != nil {
		t.Fatalf("verify: %v\n%s", err, out)
	}
	if !strings.Contains(out, "valid: true") {
		t.Errorf("verify output = %s, want valid:true", out)
	}
}

func TestCLI_Transit_Verify_Mismatch(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	out, err := r.runCmd(t, "transit", "verify", "k",
		"--message", "p", "--signature", "vault:v1:WRONG")
	if err != nil {
		t.Fatalf("verify mismatch: %v\n%s", err, out)
	}
	if !strings.Contains(out, "valid: false") {
		t.Errorf("mismatch output = %s, want valid:false", out)
	}
}

// ---- output format -----------------------------------------------

func TestCLI_BadOutputFormat(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "-o", "xml", "list")
	if err == nil {
		t.Errorf("bad output format returned nil err")
	}
}

// ---- API key propagation -----------------------------------------

func TestCLI_APIKey_PassesAsBearer(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)

	// Run with an API key. The fake transit doesn't inspect headers,
	// but the round-trip succeeding proves the auth context didn't
	// break the request.
	out, err := r.runCmd(t, "--api-key", "test-key", "transit", "encrypt", "k", "--plaintext", "x")
	if err != nil {
		t.Fatalf("encrypt with --api-key: %v\n%s", err, out)
	}
}

// ---- fakes -------------------------------------------------------

type cliFakeBackend struct {
	mu      sync.Mutex
	name    string
	caps    []internalsecrets.BackendCapability
	entries map[string]*internalsecrets.Secret
	leases  map[string]struct{}
}

func (b *cliFakeBackend) Name() string                                      { return b.name }
func (b *cliFakeBackend) Capabilities() []internalsecrets.BackendCapability { return b.caps }
func (b *cliFakeBackend) Start(context.Context) error                       { return nil }
func (b *cliFakeBackend) Stop(context.Context) error                        { return nil }
func (b *cliFakeBackend) Health(context.Context) error                      { return nil }

func (b *cliFakeBackend) GetSecret(_ context.Context, req internalsecrets.GetSecretRequest) (*internalsecrets.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.entries[req.Path]
	if !ok {
		return nil, fmt.Errorf("%w: %q", internalsecrets.ErrSecretNotFound, req.Path)
	}
	return s, nil
}

func (b *cliFakeBackend) WriteSecret(_ context.Context, req internalsecrets.WriteSecretRequest) (*internalsecrets.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &internalsecrets.Secret{Path: req.Path, Data: req.Data, Metadata: req.Metadata, Version: 1}
	b.entries[req.Path] = s
	return s, nil
}

func (b *cliFakeBackend) ListSecrets(_ context.Context, req internalsecrets.ListSecretsRequest) (*internalsecrets.ListSecretsResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := &internalsecrets.ListSecretsResponse{}
	for path := range b.entries {
		if req.Prefix == "" || strings.HasPrefix(path, req.Prefix) {
			out.Entries = append(out.Entries, internalsecrets.ListEntry{Path: path})
		}
	}
	return out, nil
}

func (b *cliFakeBackend) DeleteSecret(_ context.Context, req internalsecrets.DeleteSecretRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entries, req.Path)
	return nil
}

func (b *cliFakeBackend) IssueDynamicSecret(context.Context, internalsecrets.IssueDynamicSecretRequest) (*internalsecrets.Secret, error) {
	return nil, internalsecrets.ErrNotImplementedYet
}

func (b *cliFakeBackend) RenewLease(_ context.Context, req internalsecrets.RenewLeaseRequest) (*internalsecrets.LeaseInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.leases[req.LeaseID]; !ok {
		return nil, fmt.Errorf("%w: %q", internalsecrets.ErrLeaseNotFound, req.LeaseID)
	}
	return &internalsecrets.LeaseInfo{ID: req.LeaseID}, nil
}

func (b *cliFakeBackend) RevokeLease(_ context.Context, req internalsecrets.RevokeLeaseRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.leases, req.LeaseID)
	return nil
}

type cliFakeTransit struct{}

func (cliFakeTransit) Encrypt(_ context.Context, req internalsecrets.EncryptRequest) (*internalsecrets.EncryptResponse, error) {
	out := &internalsecrets.EncryptResponse{}
	for _, it := range req.Items {
		out.Results = append(out.Results, internalsecrets.EncryptResult{
			Ciphertext: "vault:v1:" + string(it.Plaintext),
			KeyVersion: 1,
		})
	}
	return out, nil
}

func (cliFakeTransit) Decrypt(_ context.Context, req internalsecrets.DecryptRequest) (*internalsecrets.DecryptResponse, error) {
	out := &internalsecrets.DecryptResponse{}
	for _, it := range req.Items {
		const prefix = "vault:v1:"
		if !strings.HasPrefix(it.Ciphertext, prefix) {
			out.Results = append(out.Results, internalsecrets.DecryptResult{Err: "bad ciphertext"})
			continue
		}
		out.Results = append(out.Results, internalsecrets.DecryptResult{Plaintext: []byte(it.Ciphertext[len(prefix):])})
	}
	return out, nil
}

func (cliFakeTransit) Sign(_ context.Context, req internalsecrets.SignRequest) (*internalsecrets.SignResponse, error) {
	out := &internalsecrets.SignResponse{}
	for _, it := range req.Items {
		out.Results = append(out.Results, internalsecrets.SignResult{Signature: "vault:v1:sig(" + string(it.Input) + ")"})
	}
	return out, nil
}

func (cliFakeTransit) Verify(_ context.Context, req internalsecrets.VerifyRequest) (*internalsecrets.VerifyResponse, error) {
	out := &internalsecrets.VerifyResponse{}
	for _, it := range req.Items {
		want := "vault:v1:sig(" + string(it.Input) + ")"
		out.Results = append(out.Results, internalsecrets.VerifyResult{Valid: it.Signature == want})
	}
	return out, nil
}

func (cliFakeTransit) HMAC(context.Context, internalsecrets.HMACRequest) (*internalsecrets.HMACResponse, error) {
	return &internalsecrets.HMACResponse{}, nil
}
func (cliFakeTransit) VerifyHMAC(context.Context, internalsecrets.VerifyHMACRequest) (*internalsecrets.VerifyResponse, error) {
	return &internalsecrets.VerifyResponse{}, nil
}
func (cliFakeTransit) Rewrap(context.Context, internalsecrets.RewrapRequest) (*internalsecrets.RewrapResponse, error) {
	return &internalsecrets.RewrapResponse{}, nil
}
func (cliFakeTransit) GenerateDataKey(context.Context, internalsecrets.GenerateDataKeyRequest) (*internalsecrets.GenerateDataKeyResponse, error) {
	return &internalsecrets.GenerateDataKeyResponse{}, nil
}

var _ internalsecrets.TransitBackend = cliFakeTransit{}

// `list` takes its prefix from --prefix, not from a positional. Cobra's
// default accepts arbitrary positionals and the RunE discards them, so
// `secrets list app` used to return the WHOLE listing while looking as
// though it had filtered to "app" — a silently wrong answer, which is
// the worst kind. Args: cobra.NoArgs makes it an error.
func TestCLI_List_RejectsPositionalPrefix(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	if _, err := r.runCmd(t, "list", "app"); err == nil {
		t.Error("list with a positional arg returned nil err — the prefix is --prefix, " +
			"and silently ignoring the argument reports the wrong secrets")
	}
}

func TestCLI_List_AcceptsPrefixFlag(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	if _, err := r.runCmd(t, "put", "app/db", "--data", "k=v"); err != nil {
		t.Fatalf("seed put: %v", err)
	}
	out, err := r.runCmd(t, "list", "--prefix", "app")
	if err != nil {
		t.Fatalf("list --prefix: %v", err)
	}
	if !strings.Contains(out, "app/db") {
		t.Errorf("list --prefix app = %q, want it to contain app/db", out)
	}
}
