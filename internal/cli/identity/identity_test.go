// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"

	identitypkg "go.keystone-core.io/keystone-core/internal/identity"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// shadow names so the fake-server helper doesn't depend on
// import order in this file.
var (
	grpcCodesNotFound = codes.NotFound
	fakeStatusError   = status.Error
)

// ---- fixture: bufconn-backed Provider + IdentityGRPCServer -----

// cliFixture runs the production IdentityGRPCServer (from
// internal/controlplane) over bufconn so the CLI tests exercise
// the full server path — not a hand-rolled fake. Importing
// controlplane from a test file under internal/cli/identity would
// create a cyclic graph (controlplane imports identity, identity
// imports …); to avoid that we build a minimal fake server
// inline that satisfies the methods the CLI actually calls.
//
// Each test gets its own fixture so state doesn't leak across
// tests.
type cliFixture struct {
	t       *testing.T
	server  *fakeIdentityServer
	listen  *bufconn.Listener
	closeFn func()
}

func newCLIFixture(t *testing.T) *cliFixture {
	t.Helper()
	srv := &fakeIdentityServer{
		tokens: make(map[string]*v1.JoinToken),
	}
	listener := bufconn.Listen(1 << 20)
	grpcSrv := grpc.NewServer()
	v1.RegisterIdentityServiceServer(grpcSrv, srv)
	go func() {
		if err := grpcSrv.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("grpc serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcSrv.Stop()
		_ = listener.Close()
	})
	return &cliFixture{
		t:      t,
		server: srv,
		listen: listener,
		closeFn: func() {
			grpcSrv.Stop()
			_ = listener.Close()
		},
	}
}

// dial returns a Deps wired to talk to the fixture's bufconn.
func (f *cliFixture) deps() Deps {
	return Deps{
		Dial: func(_ context.Context, _, _ string) (v1.IdentityServiceClient, io.Closer, error) {
			conn, err := grpc.NewClient(
				"passthrough://bufnet",
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return f.listen.DialContext(ctx)
				}),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return nil, nil, err
			}
			return v1.NewIdentityServiceClient(conn), conn, nil
		},
	}
}

// runCmd executes the CLI's NewCommand with the fixture's Deps +
// supplied args, capturing stdout. Returns the captured output
// and the cobra error.
func (f *cliFixture) runCmd(args ...string) (string, error) {
	f.t.Helper()
	root := NewCommand(f.deps())
	root.SetArgs(args)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetContext(context.Background())
	err := root.Execute()
	return buf.String(), err
}

// ---- fake IdentityServiceServer ---------------------------------

// fakeIdentityServer is a minimal IdentityServiceServer that
// records each call + serves canned responses. CLI tests use it
// instead of the real EmbeddedProvider — that gives us full
// control over edge cases (rotation outcomes, error paths) without
// also booting a CA. The IdentityGRPCServer impl is tested
// separately in internal/controlplane/grpc_identity_server_test.go.
type fakeIdentityServer struct {
	v1.UnimplementedIdentityServiceServer

	tokens map[string]*v1.JoinToken

	// Per-method canned errors + counters for assertion.
	createErr error
	listErr   error
	deleteErr error
	cleanupN  int32
	rotateKID string
	exportPEM map[v1.ExportCARequest_What][]byte
	caInfo    *v1.GetCAInfoResponse
	status    *v1.GetStatusResponse
}

func (f *fakeIdentityServer) CreateJoinToken(_ context.Context, req *v1.CreateJoinTokenRequest) (*v1.CreateJoinTokenResponse, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	tok := &v1.JoinToken{
		Id:         "fake-id-" + req.GetAgentId(),
		Token:      "kscore-join-FAKEPREFIX0000000000RANDOMTAIL",
		Prefix:     "kscore-join-FAKEPREF",
		AgentId:    req.GetAgentId(),
		TtlSeconds: req.GetTtlSeconds(),
		MaxUses:    req.GetMaxUses(),
		Metadata:   req.GetMetadata(),
	}
	if tok.MaxUses == 0 {
		tok.MaxUses = 1
	}
	f.tokens[tok.GetId()] = tok
	return &v1.CreateJoinTokenResponse{Token: tok}, nil
}

func (f *fakeIdentityServer) ListJoinTokens(_ context.Context, req *v1.ListJoinTokensRequest) (*v1.ListJoinTokensResponse, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := &v1.ListJoinTokensResponse{}
	for _, tok := range f.tokens {
		if req.GetAgentId() != "" && tok.GetAgentId() != req.GetAgentId() {
			continue
		}
		// Always return without cleartext — the real server's
		// invariant. Build a fresh proto rather than copying the
		// existing one to avoid copying the proto runtime's lock.
		clone := &v1.JoinToken{
			Id:         tok.GetId(),
			Prefix:     tok.GetPrefix(),
			AgentId:    tok.GetAgentId(),
			TtlSeconds: tok.GetTtlSeconds(),
			MaxUses:    tok.GetMaxUses(),
			UsedCount:  tok.GetUsedCount(),
			Metadata:   tok.GetMetadata(),
		}
		out.Tokens = append(out.Tokens, clone)
	}
	return out, nil
}

func (f *fakeIdentityServer) DeleteJoinToken(_ context.Context, req *v1.DeleteJoinTokenRequest) (*v1.DeleteJoinTokenResponse, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	if _, ok := f.tokens[req.GetId()]; !ok {
		// Match the real IdentityGRPCServer's NotFound semantics —
		// the controlplane impl maps identity.ErrJoinTokenNotFound
		// to codes.NotFound. CLI's revoke command treats this as
		// idempotent (exits 0 with a notice).
		return nil, fakeNotFoundError(req.GetId())
	}
	delete(f.tokens, req.GetId())
	return &v1.DeleteJoinTokenResponse{}, nil
}

func fakeNotFoundError(id string) error {
	return fakeStatusError(grpcCodesNotFound, "join token not found: "+id)
}

func (f *fakeIdentityServer) CleanupJoinTokens(context.Context, *v1.CleanupJoinTokensRequest) (*v1.CleanupJoinTokensResponse, error) {
	return &v1.CleanupJoinTokensResponse{Removed: f.cleanupN}, nil
}

func (f *fakeIdentityServer) GetCAInfo(context.Context, *v1.GetCAInfoRequest) (*v1.GetCAInfoResponse, error) {
	if f.caInfo != nil {
		return f.caInfo, nil
	}
	return &v1.GetCAInfoResponse{
		TrustDomain: identitypkg.DefaultTrustDomain,
		Root: &v1.CACertInfo{
			Subject: "CN=kscore root CA",
			KeyType: "ECDSA-P-256",
		},
		JwtKid: "ks-signing-fake0001",
	}, nil
}

func (f *fakeIdentityServer) RotateSigningCA(context.Context, *v1.RotateSigningCARequest) (*v1.RotateSigningCAResponse, error) {
	kid := f.rotateKID
	if kid == "" {
		kid = "ks-signing-rotated00"
	}
	return &v1.RotateSigningCAResponse{NewJwtKid: kid}, nil
}

func (f *fakeIdentityServer) ExportCA(_ context.Context, req *v1.ExportCARequest) (*v1.ExportCAResponse, error) {
	if pem, ok := f.exportPEM[req.GetWhat()]; ok {
		return &v1.ExportCAResponse{Pem: pem}, nil
	}
	// Default: an obviously fake but well-formed PEM block so the
	// CLI's parse step exercises real code without us minting a
	// real cert in each test.
	pem := []byte("-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----\n")
	return &v1.ExportCAResponse{Pem: pem}, nil
}

func (f *fakeIdentityServer) GetStatus(context.Context, *v1.GetStatusRequest) (*v1.GetStatusResponse, error) {
	if f.status != nil {
		return f.status, nil
	}
	return &v1.GetStatusResponse{
		Started:      true,
		TrustDomain:  identitypkg.DefaultTrustDomain,
		WatcherCount: 0,
		TokenTotal:   int32(len(f.tokens)), //nolint:gosec // bounded
	}, nil
}

// ---- root command ----------------------------------------------

func TestNewCommand_HasSubcommands(t *testing.T) {
	t.Parallel()
	cmd := NewCommand(Deps{})
	names := map[string]bool{}
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	for _, want := range []string{"token", "ca", "status"} {
		if !names[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
}

// ---- token create ----------------------------------------------

func TestTokenCreate_HappyPath_Table(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	out, err := f.runCmd("token", "create", "--agent-id", "agent-1")
	if err != nil {
		t.Fatalf("err = %v\nout:\n%s", err, out)
	}
	if !strings.Contains(out, "shown ONCE") {
		t.Errorf("missing one-shot banner in output:\n%s", out)
	}
	if !strings.Contains(out, "kscore-join-") {
		t.Errorf("missing cleartext token:\n%s", out)
	}
	if !strings.Contains(out, "agent-1") {
		t.Errorf("missing AgentID:\n%s", out)
	}
}

func TestTokenCreate_JSON(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	out, err := f.runCmd("token", "create", "--agent-id", "agent-json", "-o", "json")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	var resp struct {
		Token struct {
			Token   string `json:"token"`
			AgentId string `json:"agentId"`
		} `json:"token"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json decode: %v\noutput: %s", err, out)
	}
	if resp.Token.Token == "" {
		t.Error("JSON cleartext Token empty")
	}
	if resp.Token.AgentId != "agent-json" {
		t.Errorf("AgentId = %q", resp.Token.AgentId)
	}
}

func TestTokenCreate_MissingAgentID(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	out, err := f.runCmd("token", "create")
	if err == nil {
		t.Fatalf("missing --agent-id should fail; got out:\n%s", out)
	}
}

func TestTokenCreate_BadOutputFormat(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	_, err := f.runCmd("token", "create", "--agent-id", "x", "-o", "yaml")
	if err == nil {
		t.Fatal("invalid --output should fail")
	}
}

func TestTokenCreate_BadMetadata(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	_, err := f.runCmd("token", "create", "--agent-id", "x", "--metadata", "no-equals")
	if err == nil {
		t.Fatal("malformed --metadata should fail")
	}
	if !strings.Contains(err.Error(), "key=value") {
		t.Errorf("err = %v; want \"key=value\" hint", err)
	}
}

func TestTokenCreate_MetadataParses(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	out, err := f.runCmd("token", "create", "--agent-id", "agent-md",
		"--metadata", "role=web", "--metadata", "env=prod")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "role=web") || !strings.Contains(out, "env=prod") {
		t.Errorf("metadata missing from output:\n%s", out)
	}
}

// ---- token list -------------------------------------------------

func TestTokenList_Empty(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	out, err := f.runCmd("token", "list")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "no join tokens") {
		t.Errorf("empty list output:\n%s", out)
	}
}

func TestTokenList_Populated_NoCleartext(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	if _, err := f.runCmd("token", "create", "--agent-id", "a"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out, err := f.runCmd("token", "list")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(out, "kscore-join-FAKEPREFIX0000000000RANDOMTAIL") {
		t.Errorf("cleartext leaked in list output:\n%s", out)
	}
	// Prefix-only is OK (it's the operator-readable handle).
	if !strings.Contains(out, "kscore-join-FAKEPREF") {
		t.Errorf("prefix missing:\n%s", out)
	}
}

func TestTokenList_JSON(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	if _, err := f.runCmd("token", "create", "--agent-id", "a"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out, err := f.runCmd("token", "list", "-o", "json")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, `"tokens"`) {
		t.Errorf("JSON output missing tokens key:\n%s", out)
	}
}

// ---- token revoke -----------------------------------------------

func TestTokenRevoke_HappyPath(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	if _, err := f.runCmd("token", "create", "--agent-id", "rev"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out, err := f.runCmd("token", "revoke", "fake-id-rev")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "revoked") {
		t.Errorf("missing revoke confirmation:\n%s", out)
	}
}

func TestTokenRevoke_NotFound_IsIdempotent(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	out, err := f.runCmd("token", "revoke", "no-such-id")
	if err != nil {
		t.Errorf("revoke of missing id should be idempotent, got err = %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("missing not-found notice:\n%s", out)
	}
}

// ---- token cleanup ----------------------------------------------

func TestTokenCleanup(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	f.server.cleanupN = 5
	out, err := f.runCmd("token", "cleanup")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "removed 5") {
		t.Errorf("missing count:\n%s", out)
	}
}

// ---- ca info ----------------------------------------------------

func TestCAInfo_TableUsesRealCert(t *testing.T) {
	t.Parallel()
	cert, _ := mintTestCAForCLI(t)
	f := newCLIFixture(t)
	f.server.exportPEM = map[v1.ExportCARequest_What][]byte{
		v1.ExportCARequest_WHAT_SIGNING: cert,
	}
	out, err := f.runCmd("ca", "info")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, identitypkg.DefaultTrustDomain) {
		t.Errorf("missing trust domain:\n%s", out)
	}
	if !strings.Contains(out, "ECDSA-P-256") {
		t.Errorf("missing signing key type:\n%s", out)
	}
}

func TestCAInfo_BadSigningPEM(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	f.server.exportPEM = map[v1.ExportCARequest_What][]byte{
		v1.ExportCARequest_WHAT_SIGNING: []byte("not pem"),
	}
	_, err := f.runCmd("ca", "info")
	if err == nil {
		t.Fatal("malformed signing PEM should fail")
	}
}

// ---- ca rotate-signing ------------------------------------------

func TestCARotateSigning(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	f.server.rotateKID = "ks-signing-cli-test01"
	out, err := f.runCmd("ca", "rotate-signing")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "ks-signing-cli-test01") {
		t.Errorf("missing kid:\n%s", out)
	}
}

// ---- ca export --------------------------------------------------

func TestCAExport_PEMRoot(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	pem := []byte("-----BEGIN CERTIFICATE-----\nDEADBEEF\n-----END CERTIFICATE-----\n")
	f.server.exportPEM = map[v1.ExportCARequest_What][]byte{
		v1.ExportCARequest_WHAT_ROOT: pem,
	}
	out, err := f.runCmd("ca", "export", "--what", "root")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "BEGIN CERTIFICATE") {
		t.Errorf("missing PEM:\n%s", out)
	}
}

func TestCAExport_BundleJSON(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	bundle := []byte(`{"keys":[]}`)
	f.server.exportPEM = map[v1.ExportCARequest_What][]byte{
		v1.ExportCARequest_WHAT_BUNDLE: bundle,
	}
	out, err := f.runCmd("ca", "export", "--what", "bundle")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, `"keys"`) {
		t.Errorf("missing bundle:\n%s", out)
	}
}

func TestCAExport_UnknownWhat(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	_, err := f.runCmd("ca", "export", "--what", "secret")
	if err == nil {
		t.Fatal("unknown --what should reject")
	}
}

func TestCAExport_JSONEnvelope(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	f.server.exportPEM = map[v1.ExportCARequest_What][]byte{
		v1.ExportCARequest_WHAT_ROOT: []byte("ROOT-PEM"),
	}
	out, err := f.runCmd("ca", "export", "--what", "root", "-o", "json")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	var got struct {
		What string `json:"what"`
		PEM  string `json:"pem"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json decode: %v\nout: %s", err, out)
	}
	if got.What != "root" || got.PEM != "ROOT-PEM" {
		t.Errorf("got = %+v", got)
	}
}

// ---- status -----------------------------------------------------

func TestStatus_HappyPath(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	out, err := f.runCmd("status")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "running") {
		t.Errorf("missing status:\n%s", out)
	}
	if !strings.Contains(out, identitypkg.DefaultTrustDomain) {
		t.Errorf("missing trust domain:\n%s", out)
	}
}

func TestStatus_NotRunning(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	f.server.status = &v1.GetStatusResponse{Started: false}
	out, err := f.runCmd("status")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "STOPPED") {
		t.Errorf("missing stopped marker:\n%s", out)
	}
}

func TestStatus_JSON(t *testing.T) {
	t.Parallel()
	f := newCLIFixture(t)
	out, err := f.runCmd("status", "-o", "json")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	var resp v1.GetStatusResponse
	if err := unmarshalProtoJSON(out, &resp); err != nil {
		t.Fatalf("decode: %v\nout: %s", err, out)
	}
	if !resp.GetStarted() {
		t.Error("Started=false in JSON")
	}
}

// ---- helpers ----------------------------------------------------

// mintTestCAForCLI generates a real ECDSA-P256 self-signed CA cert
// + returns its PEM encoding for the `ca info` test to parse.
func mintTestCAForCLI(t *testing.T) ([]byte, time.Time) {
	t.Helper()
	cfg := identitypkg.DefaultCAConfig(identitypkg.DefaultTrustDomain)
	cfg.RootCATTL = 24 * time.Hour
	cfg.SigningCATTL = 12 * time.Hour
	cfg.RotateBefore = time.Hour
	cfg.DefaultSVIDTTL = 5 * time.Minute
	cfg.MaxSVIDTTL = time.Hour
	storage, err := identitypkg.NewFileCAStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	mgr, err := identitypkg.NewCAManager(cfg, storage)
	if err != nil {
		t.Fatalf("NewCAManager: %v", err)
	}
	if err := mgr.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	chain := mgr.GetTrustChain()
	if len(chain) == 0 {
		t.Fatal("no chain")
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: chain[0].Raw})
	return pemBytes, chain[0].NotAfter
}

// unmarshalProtoJSON decodes a protojson-emitted message string
// back into a proto.Message. Used by --output json round-trip
// tests so we exercise the actual encoder our CLI uses.
func unmarshalProtoJSON(s string, msg *v1.GetStatusResponse) error {
	return protojson.Unmarshal([]byte(s), msg)
}
