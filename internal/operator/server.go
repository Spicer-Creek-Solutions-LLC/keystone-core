package operator

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/store"
)

const (
	SocketDir  = "/run/keystone"
	SocketPath = "/run/keystone/operator.sock"

	ActionAuthorizationDenied = "operator.authorization.denied"
	ActionFaultPrefix         = "fault.enabled:"
)

// Error codes, C04-A's frozen set and the one C05 adds. Each is the only member
// of an error frame.
const (
	ErrAuthorizationDenied = "authorization_denied"
	ErrUnknownOperation    = "unknown_operation"
	ErrFrameTooLarge       = "frame_too_large"
	ErrConnectionLimit     = "connection_limit"
	// ErrInvalidRequest answers a known operation whose arguments it refuses.
	ErrInvalidRequest = "invalid_request"
)

type membershipRequest struct {
	uid   uint32
	delay time.Duration
	done  chan membershipResult
}

type membershipResult struct {
	name       *string
	authorized bool
	err        error
}

type Server struct {
	cfg        config.Server
	store      *store.Store
	handlers   map[string]Handler
	groupID    int
	listener   *net.UnixListener
	unauth     chan struct{}
	authorized chan struct{}
	lookups    chan membershipRequest
}

// Actor is the authorized peer, as the kernel and the membership check
// established it. No request member can change it (ADR-0009 § 7).
type Actor struct {
	UID              uint32
	UsernameSnapshot *string
}

// Handler answers one operation for an authorized actor. It returns the
// response object, or an error code to send instead. A nil response with an
// empty code closes the connection unanswered: the handler could not establish
// its outcome, and no answer is better than a wrong one.
type Handler func(ctx context.Context, actor Actor, request []byte) (response any, errorCode string)

// Handle registers an operation. It must be called before Serve.
func (s *Server) Handle(op string, h Handler) { s.handlers[op] = h }

func New(cfg config.Server, st *store.Store) (*Server, error) {
	g, err := user.LookupGroup(cfg.Operator.AdminGroup)
	if err != nil {
		return nil, fmt.Errorf("lookup admin group %q: %w", cfg.Operator.AdminGroup, err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return nil, fmt.Errorf("admin group %q has invalid gid %q", cfg.Operator.AdminGroup, g.Gid)
	}
	s := &Server{
		cfg: cfg, store: st, groupID: gid, handlers: map[string]Handler{},
		unauth:     make(chan struct{}, cfg.Operator.MaxUnauthorizedConnections),
		authorized: make(chan struct{}, cfg.Operator.MaxAuthorizedConnections),
		lookups:    make(chan membershipRequest, cfg.Operator.MaxUnauthorizedConnections),
	}
	return s, nil
}

func (s *Server) Listen() error {
	if os.Geteuid() != 0 {
		return errors.New("operator listener must run as root")
	}
	if err := s.prepareDirectory(); err != nil {
		return err
	}
	if err := recoverStaleSocket(); err != nil {
		return err
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: SocketPath, Net: "unix"})
	if err != nil {
		return err
	}
	if err := os.Chown(SocketPath, 0, s.groupID); err != nil {
		l.Close()
		return err
	}
	if err := os.Chmod(SocketPath, 0o660); err != nil {
		l.Close()
		return err
	}
	s.listener = l
	for i := 0; i < s.cfg.Operator.MaxUnauthorizedConnections; i++ {
		go s.membershipWorker()
	}
	return nil
}

func (s *Server) Serve() error {
	if s.listener == nil {
		return errors.New("operator listener is not open")
	}
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		select {
		case s.unauth <- struct{}{}:
			go s.handle(conn)
		default:
			conn.Close()
		}
	}
}

func (s *Server) Close() error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *Server) prepareDirectory() error {
	if err := os.MkdirAll(SocketDir, 0o750); err != nil {
		return err
	}
	info, err := os.Lstat(SocketDir)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || st.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s must be a root-owned directory writable only by root", SocketDir)
	}
	if err := os.Chown(SocketDir, 0, s.groupID); err != nil {
		return err
	}
	return os.Chmod(SocketDir, 0o750)
}

func recoverStaleSocket() error {
	info, err := os.Lstat(SocketPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeSocket == 0 || st.Uid != 0 {
		return fmt.Errorf("refusing to replace unsafe object at %s", SocketPath)
	}
	c, err := net.DialTimeout("unix", SocketPath, 250*time.Millisecond)
	if err == nil {
		c.Close()
		return fmt.Errorf("operator socket is already live")
	}
	var op *net.OpError
	if !errors.As(err, &op) || !errors.Is(op.Err, syscall.ECONNREFUSED) {
		return fmt.Errorf("cannot prove operator socket stale: %w", err)
	}
	return os.Remove(SocketPath)
}

func (s *Server) handle(conn *net.UnixConn) {
	defer conn.Close()
	unauthorizedSlotHeld := true
	defer func() {
		if unauthorizedSlotHeld {
			<-s.unauth
		}
	}()
	ucred, err := peerCredentials(conn)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Operator.AuthorizationTimeout)
	defer cancel()
	result := make(chan membershipResult, 1)
	req := membershipRequest{uid: ucred.Uid, delay: s.cfg.Faults.OperatorHoldBeforeDecision, done: result}
	select {
	case s.lookups <- req:
	case <-ctx.Done():
		return
	}
	var membership membershipResult
	select {
	case membership = <-result:
	case <-ctx.Done():
		return
	}
	if membership.err != nil || !membership.authorized {
		uid := ucred.Uid
		if err := s.store.AppendAuditRecord(context.Background(), store.AuditRecord{
			ActorUID: &uid, ActorUsernameSnapshot: membership.name,
			Action: ActionAuthorizationDenied, Result: "denied", At: time.Now(),
		}); err != nil {
			return
		}
		writeError(conn, ErrAuthorizationDenied)
		return
	}
	select {
	case s.authorized <- struct{}{}:
		defer func() { <-s.authorized }()
	default:
		writeError(conn, ErrConnectionLimit)
		return
	}
	// Authorization is complete; this connection no longer consumes a
	// pre-authorization slot while it remains open.
	<-s.unauth
	unauthorizedSlotHeld = false
	s.serveAuthorized(conn, Actor{UID: ucred.Uid, UsernameSnapshot: membership.name})
}

func (s *Server) membershipWorker() {
	for req := range s.lookups {
		if req.delay > 0 {
			time.Sleep(req.delay)
		}
		req.done <- currentMembership(req.uid, s.groupID)
	}
}

func currentMembership(uid uint32, adminGID int) membershipResult {
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return membershipResult{err: err}
	}
	groups, err := u.GroupIds()
	name := u.Username
	if err != nil {
		return membershipResult{name: &name, err: err}
	}
	want := strconv.Itoa(adminGID)
	for _, gid := range groups {
		if gid == want {
			return membershipResult{name: &name, authorized: true}
		}
	}
	return membershipResult{name: &name}
}

// serveAuthorized answers one request at a time. The next frame is not read
// until the current response is written, so a connection has exactly one
// request in flight and its replies are in request order (LIM-5). The reads are
// unbuffered for the same reason: a buffered reader would take the next frame
// off the socket early.
func (s *Server) serveAuthorized(conn net.Conn, actor Actor) {
	var prefix [4]byte
	for {
		if _, err := io.ReadFull(conn, prefix[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(prefix[:])
		if n > s.cfg.Operator.MaxFrameBytes {
			writeError(conn, ErrFrameTooLarge)
			return
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(conn, body); err != nil {
			return
		}
		response, code := s.dispatch(actor, body)
		if response == nil && code == "" {
			return
		}
		if d := s.cfg.Faults.OperatorHoldBeforeResponse; d > 0 {
			time.Sleep(d)
		}
		if code != "" {
			writeError(conn, code)
			continue
		}
		if err := writeFrame(conn, response); err != nil {
			return
		}
	}
}

func (s *Server) dispatch(actor Actor, body []byte) (any, string) {
	var request struct {
		Op string `json:"op"`
	}
	if json.Unmarshal(body, &request) != nil {
		return nil, ErrUnknownOperation
	}
	h, ok := s.handlers[request.Op]
	if !ok {
		return nil, ErrUnknownOperation
	}
	return h(context.Background(), actor, body)
}

func writeFrame(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(body)))
	_, err = w.Write(append(prefix[:], body...))
	return err
}

func writeError(w io.Writer, code string) {
	body, _ := json.Marshal(map[string]string{"error": code})
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(body)))
	_, _ = w.Write(append(prefix[:], body...))
}
