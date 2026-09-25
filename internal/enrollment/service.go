package enrollment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"go.keystone-core.io/keystone-core/internal/natsauth"
	"go.keystone-core.io/keystone-core/internal/protocol"
	"go.keystone-core.io/keystone-core/internal/store"
)

// Audit actions for the enrollment lifecycle (ARCH-OBS-001).
const (
	ActionCredentialIssued = "enrollment.credential.issued"
	ActionActivated        = "enrollment.identity.activated"
	ActionDenied           = "enrollment.request.denied"
)

// KindDenied answers a request the service refused. It carries nothing but the
// fact: expired, spent and invalid are not distinguished to the caller
// (ADR-0003 § 5), and the reason is in the audit record only.
const KindDenied = "denied"

// ConnectTimeout bounds how long the server tries to reach the broker at
// startup before it refuses to run.
const ConnectTimeout = 15 * time.Second

// Service is the enrollment service and the presence observation that
// completes S4. It runs on two connections, one per service credential.
type Service struct {
	srv      *Server
	store    *store.Store
	minter   *natsauth.Minter
	now      func() time.Time
	enroll   *nats.Conn
	presence *nats.Conn
}

// Start connects both service identities, subscribes, and returns once both
// subscriptions are established at the broker. A server that cannot reach the
// broker, or cannot verify it, does not run.
func (s *Server) Start(ctx context.Context) (*Service, error) {
	svc := &Service{srv: s, store: s.Issuer.store, minter: s.Issuer.minter, now: time.Now}
	var err error
	if svc.enroll, err = s.connect(ctx, s.Enrollment, "keystone-server enrollment-service"); err != nil {
		return nil, err
	}
	if svc.presence, err = s.connect(ctx, s.Presence, "keystone-server presence-consumer"); err != nil {
		svc.enroll.Close()
		return nil, err
	}
	if _, err := svc.enroll.Subscribe(RequestSubjects, svc.onRequest); err != nil {
		svc.Close()
		return nil, err
	}
	if _, err := svc.presence.Subscribe(PresenceSubjects, svc.onPresence); err != nil {
		svc.Close()
		return nil, err
	}
	for _, c := range []*nats.Conn{svc.enroll, svc.presence} {
		if err := c.FlushTimeout(ConnectTimeout); err != nil {
			svc.Close()
			return nil, err
		}
	}
	return svc, nil
}

func (s *Service) Close() {
	for _, c := range []*nats.Conn{s.enroll, s.presence} {
		if c != nil {
			c.Close()
		}
	}
}

// connect dials the broker as one service principal, verifying it with the
// configured CA and name, TLS-first (ADR-0002 § 12, D-C05-8).
func (s *Server) connect(ctx context.Context, cred natsauth.ServiceCredential, name string) (*nats.Conn, error) {
	opts, err := ClientOptions(s.CAPEM, s.ServerName, cred.File)
	if err != nil {
		return nil, err
	}
	opts = append(opts, nats.Name(name), nats.MaxReconnects(-1))
	deadline := time.Now().Add(ConnectTimeout)
	for {
		c, err := nats.Connect(s.URL, opts...)
		if err == nil {
			return c, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return nil, fmt.Errorf("connect to the broker as %s: %w", cred.Principal, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ClientOptions is how every Keystone NATS connection authenticates and
// verifies the broker: its own credential, and TLS-first against the
// configured CA and name. There is no option that turns verification off.
func ClientOptions(caPEM []byte, serverName string, creds []byte) ([]nats.Option, error) {
	tlsConfig, err := natsauth.ClientTLSConfig(caPEM, serverName)
	if err != nil {
		return nil, err
	}
	token, err := jwt.ParseDecoratedJWT(creds)
	if err != nil {
		return nil, err
	}
	kp, err := jwt.ParseDecoratedNKey(creds)
	if err != nil {
		return nil, err
	}
	seed, err := kp.Seed()
	if err != nil {
		return nil, err
	}
	return []nats.Option{
		nats.Secure(tlsConfig),
		nats.TLSHandshakeFirst(),
		nats.UserJWTAndSeed(token, string(seed)),
	}, nil
}

// tokenOf extracts the token from a request subject. The broker lets only the
// token's own bootstrap identity publish there (ADR-0003 § 3), which is how the
// server knows the presenting identity is the one it minted (§ 5).
func tokenOf(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) != 4 || parts[0] != "ks" || parts[1] != "enroll" || parts[3] != "request" {
		return ""
	}
	return parts[2]
}

func (s *Service) onRequest(m *nats.Msg) {
	token := tokenOf(m.Subject)
	if token == "" {
		return
	}
	ctx := context.Background()
	reply, reason := s.handle(ctx, token, m.Data)
	if reason != "" {
		s.deny(ctx, token, reason)
		reply = &Reply{Kind: KindDenied}
	}
	if reply == nil {
		return
	}
	s.reply(token, *reply)
}

// handle applies ADR-0003 § 5's checks in order, then S1 or S6. It returns the
// reply to send, a denial reason, or neither -- a confirmation asked for
// before S4, which the agent repeats.
func (s *Service) handle(ctx context.Context, token string, data []byte) (*Reply, string) {
	rec, err := s.store.Enrollment(ctx, token)
	if errors.Is(err, store.ErrNoEnrollment) {
		return nil, "unknown token"
	}
	if err != nil {
		log.Printf("enrollment: read token: %v", err)
		return nil, ""
	}
	e, err := OpenEnvelope(data, protocol.ClassEnrollmentRequest, rec.AgentID)
	if err != nil || e.CorrelationID != token {
		return nil, "malformed request"
	}
	var req Request
	if err := strictJSON(e.Payload, &req); err != nil {
		return nil, "malformed request"
	}
	if rec.State == store.TokenSpent && req.Kind != KindConfirmation {
		return nil, "token spent"
	}
	if !s.now().Before(rec.ExpiresAt) {
		return nil, "token expired"
	}
	halves, err := parseHalves(req)
	if err != nil {
		return nil, "incomplete public halves"
	}
	if err := Verify(halves.verifying, e); err != nil {
		return nil, "request signature"
	}
	if rec.PermanentJWT != "" && !halves.equal(rec) {
		return nil, "different public halves"
	}
	switch req.Kind {
	case KindCredential:
		return s.credential(ctx, rec, halves)
	case KindConfirmation:
		if rec.State != store.TokenSpent {
			return nil, ""
		}
		return &Reply{Kind: KindConfirmation, AgentID: rec.AgentID, Identity: StateActive, Token: StateSpent}, ""
	}
	return nil, "malformed request"
}

// credential is S1: record the halves and mint the permanent JWT once, and
// return the same JWT to every later request carrying the same halves.
func (s *Service) credential(ctx context.Context, rec store.EnrollmentRecord, h requestHalves) (*Reply, string) {
	if rec.PermanentJWT != "" {
		return &Reply{Kind: KindCredential, AgentID: rec.AgentID, PermanentJWT: rec.PermanentJWT}, ""
	}
	id, err := s.minter.Agent(natsauth.AgentKey{ID: rec.AgentID, PublicKey: h.natsKey})
	if err != nil {
		log.Printf("enrollment: mint %s: %v", rec.AgentID, err)
		return nil, ""
	}
	target := rec.AgentID
	err = s.store.RecordCredential(ctx, rec.TokenID, store.Credential{
		NATSPublicKey: h.natsKey, SigningPublicKey: h.signing, EncryptionPublicKey: h.encryption,
		PermanentJWT: id.JWT, At: s.now(),
	}, store.AuditRecord{Target: &target, Action: ActionCredentialIssued, Result: "issued", At: s.now()})
	if errors.Is(err, store.ErrAlreadyRecorded) {
		// A concurrent request recorded first. Its halves are authoritative.
		again, rerr := s.store.Enrollment(ctx, rec.TokenID)
		if rerr != nil || !h.equal(again) {
			return nil, "different public halves"
		}
		return &Reply{Kind: KindCredential, AgentID: again.AgentID, PermanentJWT: again.PermanentJWT}, ""
	}
	if err != nil {
		log.Printf("enrollment: record credential for %s: %v", rec.AgentID, err)
		return nil, ""
	}
	return &Reply{Kind: KindCredential, AgentID: rec.AgentID, PermanentJWT: id.JWT}, ""
}

func (s *Service) deny(ctx context.Context, token, reason string) {
	target := token
	if err := s.store.AppendAuditRecord(ctx, store.AuditRecord{
		CorrelationID: &target, Action: ActionDenied, Result: reason, At: s.now(),
	}); err != nil {
		log.Printf("enrollment: record denial: %v", err)
	}
}

func (s *Service) reply(token string, r Reply) {
	payload, err := json.Marshal(r)
	if err != nil {
		return
	}
	b, err := Seal(s.srv.Keys.Signing, protocol.ClassEnrollmentReply, protocol.SenderEnrollmentService, token, payload, s.now())
	if err != nil {
		log.Printf("enrollment: seal reply: %v", err)
		return
	}
	msg := nats.NewMsg(ReplySubject(token))
	msg.Data = b
	msg.Header.Set(protocol.HeaderVersion, fmt.Sprint(protocol.Version))
	msg.Header.Set(protocol.HeaderClass, string(protocol.ClassEnrollmentReply))
	if err := s.enroll.PublishMsg(msg); err != nil {
		log.Printf("enrollment: publish reply: %v", err)
	}
}

// onPresence is S4: the first presence envelope an agent signs with the key S1
// recorded, fresh, activates it and spends its token in one write.
func (s *Service) onPresence(m *nats.Msg) {
	parts := strings.Split(m.Subject, ".")
	if len(parts) != 4 {
		return
	}
	agent := parts[2]
	ctx := context.Background()
	rec, err := s.store.EnrollmentByAgent(ctx, agent)
	if err != nil || rec.State != store.TokenIssued || rec.PermanentJWT == "" {
		return
	}
	e, err := OpenEnvelope(m.Data, protocol.ClassPresence, agent)
	if err != nil {
		return
	}
	v, err := protocol.ParseVerifyingKey(rec.SigningPublicKey)
	if err != nil || Verify(v, e) != nil {
		return
	}
	if age := s.now().Sub(e.Timestamp); age > PresenceStaleness || age < -PresenceStaleness {
		return
	}
	target := agent
	err = s.store.Activate(ctx, rec.TokenID, s.now(), store.AuditRecord{
		Target: &target, Action: ActionActivated, Result: "active", At: s.now(),
	})
	if err != nil && !errors.Is(err, store.ErrNotActivatable) {
		log.Printf("enrollment: activate %s: %v", agent, err)
	}
}

type requestHalves struct {
	natsKey    string
	signing    []byte
	encryption []byte
	verifying  *protocol.VerifyingKey
}

// parseHalves requires all three public halves, each well formed (§ 5).
func parseHalves(r Request) (requestHalves, error) {
	if !nkeys.IsValidPublicUserKey(r.NATSPublicKey) {
		return requestHalves{}, errors.New("nats public key")
	}
	signing, err := DecodeKey(r.SigningPublicKey)
	if err != nil {
		return requestHalves{}, err
	}
	v, err := protocol.ParseVerifyingKey(signing)
	if err != nil {
		return requestHalves{}, err
	}
	encryption, err := DecodeKey(r.EncryptionPublicKey)
	if err != nil {
		return requestHalves{}, err
	}
	if _, err := protocol.ParseRecipientKey(encryption); err != nil {
		return requestHalves{}, err
	}
	return requestHalves{natsKey: r.NATSPublicKey, signing: signing, encryption: encryption, verifying: v}, nil
}

func (h requestHalves) equal(r store.EnrollmentRecord) bool {
	return h.natsKey == r.NATSPublicKey && bytes.Equal(h.signing, r.SigningPublicKey) &&
		bytes.Equal(h.encryption, r.EncryptionPublicKey)
}
