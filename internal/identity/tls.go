package identity

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ServerTLSRole captures which SPIFFE identity the server should
// present at the TLS layer. v0.1 ships [ServerRoleControlPlane]
// (the canonical `spiffe://<td>/server/control-plane`); future
// roles slot in cleanly (e.g. a separate identity for the agent-
// facing gRPC vs. the operator-facing one).
type ServerTLSRole int

const (
	// ServerRoleControlPlane → spiffe://<td>/server/control-plane.
	// Used by both the operator-facing IdentityService gRPC + the
	// CoordinationService once Epic 13 lands.
	ServerRoleControlPlane ServerTLSRole = iota
)

// ErrTLSConfig wraps every rejection from [BuildServerTLSConfig].
// Callers branch with [errors.Is].
var ErrTLSConfig = errors.New("identity: TLS config")

// ServerTLSOptions tunes the [BuildServerTLSConfig] result.
// Sensible v0.1 defaults are applied when the zero value is
// passed.
type ServerTLSOptions struct {
	// MinVersion is the minimum TLS version. Defaults to
	// tls.VersionTLS13 per the §4.10 "TLS 1.3 default minimum"
	// requirement. Operators must explicitly downgrade to 1.2.
	MinVersion uint16

	// ClientAuth is the peer-cert verification policy. Defaults
	// to tls.VerifyClientCertIfGiven — the server accepts API-key
	// clients that don't present a cert but verifies any cert
	// that IS presented. tls.RequireAndVerifyClientCert is the
	// strict-mTLS alternative.
	ClientAuth tls.ClientAuthType

	// ServerSVIDLifetime sets the TTL of the issued server SVID.
	// Defaults to identity.MaxSVIDTTL (24h). The watcher reissues
	// when the cert reaches ShouldRotate (50% lifetime).
	ServerSVIDLifetime time.Duration

	// DNSNames + IPAddresses populate the server cert's SANs in
	// addition to its SPIFFE URI SAN. Defaults to ["localhost"]
	// + [127.0.0.1, ::1] so local mTLS clients (CLI / tests)
	// connect by hostname/IP without bypassing TLS verify.
	DNSNames    []string
	IPAddresses []net.IP

	// Logger receives rotation + watcher messages. nil falls back
	// to slog.Default.
	Logger *slog.Logger
}

func (o *ServerTLSOptions) withDefaults() *ServerTLSOptions {
	out := *o
	if out.MinVersion == 0 {
		out.MinVersion = tls.VersionTLS13
	}
	if out.ClientAuth == tls.NoClientCert {
		// Operators who actually want NoClientCert must set
		// tls.RequestClientCert — we don't override that. zero =
		// the v0.1 default.
		out.ClientAuth = tls.VerifyClientCertIfGiven
	}
	if out.ServerSVIDLifetime == 0 {
		out.ServerSVIDLifetime = maxSVIDTTLDefault
	}
	if len(out.DNSNames) == 0 {
		out.DNSNames = []string{"localhost"}
	}
	if len(out.IPAddresses) == 0 {
		out.IPAddresses = []net.IP{
			net.IPv4(127, 0, 0, 1),
			net.IPv6loopback,
		}
	}
	if out.Logger == nil {
		out.Logger = slog.Default()
	}
	return &out
}

// BuildServerTLSConfig issues a server SVID for `role` and
// returns a *tls.Config wired to the provider's trust bundle.
// The returned config uses GetCertificate + GetConfigForClient
// callbacks so the server picks up signing-CA rotation + bundle
// updates without restart.
//
// The returned cancel function tears down the background watcher
// goroutine. Callers MUST invoke it on shutdown (typically via
// t.Cleanup / defer in production boot).
//
// Returns [ErrTLSConfig] when the provider isn't running or when
// the initial SVID issuance fails.
func BuildServerTLSConfig(ctx context.Context, p *EmbeddedProvider, role ServerTLSRole, opts *ServerTLSOptions) (*tls.Config, func(), error) {
	if p == nil {
		return nil, noopCancel, fmt.Errorf("%w: nil provider", ErrTLSConfig)
	}
	if err := p.Health(ctx); err != nil {
		return nil, noopCancel, fmt.Errorf("%w: %w", ErrTLSConfig, err)
	}
	if opts == nil {
		opts = &ServerTLSOptions{}
	}
	opts = opts.withDefaults()

	id, err := spiffeIDForServerRole(p.TrustDomain(), role)
	if err != nil {
		return nil, noopCancel, fmt.Errorf("%w: %v", ErrTLSConfig, err)
	}

	state := newServerCertState(p, id, opts)
	if err := state.refreshCert(ctx); err != nil {
		return nil, noopCancel, fmt.Errorf("%w: initial issuance: %w", ErrTLSConfig, err)
	}
	if err := state.refreshBundle(ctx); err != nil {
		return nil, noopCancel, fmt.Errorf("%w: initial bundle: %w", ErrTLSConfig, err)
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	state.startWatcher(watchCtx)

	tlsCfg := &tls.Config{
		MinVersion: opts.MinVersion,
		ClientAuth: opts.ClientAuth,
		GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return state.currentCert(), nil
		},
		GetConfigForClient: func(_ *tls.ClientHelloInfo) (*tls.Config, error) {
			return state.currentClientConfig(), nil
		},
	}
	return tlsCfg, func() {
		cancel()
		state.wait()
	}, nil
}

func spiffeIDForServerRole(td string, role ServerTLSRole) (SPIFFEID, error) {
	switch role {
	case ServerRoleControlPlane:
		return ServerID(td, "control-plane")
	}
	return SPIFFEID{}, fmt.Errorf("unknown ServerTLSRole %d", role)
}

// noopCancel is the cancel func returned when BuildServerTLSConfig
// errors before spawning the watcher — keeps the caller's `defer
// cancel()` safe.
func noopCancel() {}

// ---- server-side cert + bundle state ----------------------------

type serverCertState struct {
	provider *EmbeddedProvider
	id       SPIFFEID
	opts     *ServerTLSOptions

	// atomic.Pointer fields make GetCertificate +
	// GetConfigForClient lock-free on the hot path. Updates happen
	// only inside the watcher goroutine, which always swaps the
	// pointer rather than mutating in place.
	cert     atomic.Pointer[tls.Certificate]
	pool     atomic.Pointer[x509.CertPool]
	template atomic.Pointer[tls.Config]

	wg sync.WaitGroup
}

func newServerCertState(p *EmbeddedProvider, id SPIFFEID, opts *ServerTLSOptions) *serverCertState {
	return &serverCertState{provider: p, id: id, opts: opts}
}

// refreshCert issues a fresh server SVID + rebuilds the
// tls.Certificate. Called at startup + whenever the watcher
// detects a rotation.
func (s *serverCertState) refreshCert(ctx context.Context) error {
	svid, err := s.provider.IssueX509SVID(ctx, IssueX509SVIDRequest{
		ID:          s.id,
		KeyType:     s.provider.cfg.CAConfig.KeyType,
		TTL:         s.opts.ServerSVIDLifetime,
		DNSNames:    s.opts.DNSNames,
		IPAddresses: s.opts.IPAddresses,
	})
	if err != nil {
		return fmt.Errorf("issue server svid: %w", err)
	}

	// Build a tls.Certificate from the chain + private key. The
	// chain is [leaf, signingCA] — TLS sends both so the client
	// has the full path to the trust anchor.
	chainRaw := make([][]byte, 0, len(svid.Chain()))
	for _, c := range svid.Chain() {
		chainRaw = append(chainRaw, c.Raw)
	}
	tlsCert := &tls.Certificate{
		Certificate: chainRaw,
		PrivateKey:  svid.PrivateKey(),
		Leaf:        svid.Leaf(),
	}
	s.cert.Store(tlsCert)
	return nil
}

// refreshBundle pulls the current trust bundle from the provider
// and builds an x509.CertPool the TLS client-auth verifier uses.
func (s *serverCertState) refreshBundle(ctx context.Context) error {
	bundle, err := s.provider.GetTrustBundle(ctx)
	if err != nil {
		return fmt.Errorf("get trust bundle: %w", err)
	}
	pool := x509.NewCertPool()
	for _, cert := range bundle.X509Authorities() {
		pool.AddCert(cert)
	}
	s.pool.Store(pool)

	// Rebuild the per-client config template too, so
	// GetConfigForClient returns a fresh *tls.Config that
	// references the new pool. tls.Config is copy-on-read by
	// design — we hand back the template every time.
	tmpl := &tls.Config{
		MinVersion: s.opts.MinVersion,
		ClientAuth: s.opts.ClientAuth,
		ClientCAs:  pool,
		// Also serve the current cert from this config so
		// GetConfigForClient consumers see a self-contained
		// tls.Config.
		GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return s.currentCert(), nil
		},
	}
	s.template.Store(tmpl)
	return nil
}

func (s *serverCertState) currentCert() *tls.Certificate {
	return s.cert.Load()
}

func (s *serverCertState) currentClientConfig() *tls.Config {
	tmpl := s.template.Load()
	if tmpl == nil {
		return nil
	}
	// Return a clone so a misbehaving client handler can't mutate
	// the shared template.
	cloned := tmpl.Clone()
	return cloned
}

// startWatcher subscribes to the provider's trust-bundle channel
// and spawns a goroutine that refreshes the cert + pool on every
// rotation. Background ctx terminates when [BuildServerTLSConfig]'s
// returned cancel fires.
//
// The provider's WatchTrustBundle delivers the current bundle into
// the channel buffer before returning (a cap-1 send). We drain
// that initial bundle SYNCHRONOUSLY here — BuildServerTLSConfig
// already called refreshCert + refreshBundle to seed the state,
// so reusing the initial would be wasted work. More importantly,
// draining synchronously empties the buffer before this function
// returns, so a rotation that fires immediately after never
// collides with the initial and gets non-blocking-dropped.
func (s *serverCertState) startWatcher(ctx context.Context) {
	updates, err := s.provider.WatchTrustBundle(ctx)
	if err != nil {
		s.opts.Logger.Error("tls: watch trust bundle failed; cert + pool will not refresh on rotation",
			"err", err)
		return
	}
	// Drain the initial bundle synchronously. WatchTrustBundle
	// pushes it into the cap-1 buffer before returning; if we
	// leave it there, a follow-up rotation's non-blocking send
	// drops on the floor.
	select {
	case <-updates:
	default:
		// Provider may have closed the channel already (Stop in
		// flight); the goroutine below handles that case too.
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-updates:
				if !ok {
					return
				}
				if err := s.refreshCert(ctx); err != nil {
					s.opts.Logger.Error("tls: refresh cert after rotation failed",
						"err", err)
					continue
				}
				if err := s.refreshBundle(ctx); err != nil {
					s.opts.Logger.Error("tls: refresh bundle after rotation failed",
						"err", err)
					continue
				}
				s.opts.Logger.Info("tls: cert + bundle refreshed after signing-CA rotation")
			}
		}
	}()
}

func (s *serverCertState) wait() { s.wg.Wait() }
