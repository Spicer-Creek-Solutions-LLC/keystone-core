package netconf

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// Session manages a NETCONF session over a Transport.
type Session struct {
	transport  *Transport
	sessionID  uint32
	clientCaps []Capability
	serverCaps []Capability
	messageID  atomic.Uint64
	mu         sync.RWMutex
	closed     bool
}

// NewSession creates a NETCONF session by exchanging hello messages.
func NewSession(transport *Transport) (*Session, error) {
	s := &Session{
		transport:  transport,
		clientCaps: ClientCapabilities(),
	}

	if err := s.exchangeHello(); err != nil {
		return nil, fmt.Errorf("hello exchange: %w", err)
	}

	return s, nil
}

func (s *Session) exchangeHello() error {
	hello := newHello(s.clientCaps)
	data, err := encodeHello(hello)
	if err != nil {
		return err
	}

	if err := s.transport.Send(data); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	reply, err := s.transport.Receive()
	if err != nil {
		return fmt.Errorf("receive hello: %w", err)
	}

	serverHello, err := parseHello(reply)
	if err != nil {
		return err
	}

	s.sessionID = serverHello.SessionID
	s.serverCaps = make([]Capability, len(serverHello.Capabilities))
	for i, c := range serverHello.Capabilities {
		s.serverCaps[i] = Capability(c)
	}

	if !s.HasCapability(BaseCapability10) && !s.HasCapability(BaseCapability11) {
		return fmt.Errorf("server does not support NETCONF base:1.0 or base:1.1")
	}

	if s.HasCapability(BaseCapability11) {
		s.transport.SetFraming(FramingChunked)
	}

	return nil
}

// SendRPC sends an RPC request and returns the parsed reply.
func (s *Session) SendRPC(body interface{}) (*RPCReply, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("session closed")
	}
	s.mu.RUnlock()

	msgID := s.nextMessageID()

	data, err := encodeRPC(msgID, body)
	if err != nil {
		return nil, err
	}

	if err := s.transport.Send(data); err != nil {
		return nil, fmt.Errorf("send rpc %s: %w", msgID, err)
	}

	raw, err := s.transport.Receive()
	if err != nil {
		return nil, fmt.Errorf("receive rpc-reply %s: %w", msgID, err)
	}

	reply, err := decodeReply(raw)
	if err != nil {
		return nil, err
	}

	if reply.MessageID != msgID {
		return nil, fmt.Errorf("message-id mismatch: sent %s, got %s", msgID, reply.MessageID)
	}

	return reply, nil
}

func (s *Session) nextMessageID() string {
	id := s.messageID.Add(1)
	return fmt.Sprintf("%d", id)
}

// SessionID returns the session ID assigned by the server.
func (s *Session) SessionID() uint32 {
	return s.sessionID
}

// ServerCapabilities returns the server's advertised capabilities.
func (s *Session) ServerCapabilities() []Capability {
	return s.serverCaps
}

// HasCapability checks if the server advertises the given capability.
func (s *Session) HasCapability(cap Capability) bool {
	target := string(cap)
	for _, c := range s.serverCaps {
		if string(c) == target || strings.HasPrefix(string(c), target+"?") {
			return true
		}
	}
	return false
}

// Close sends close-session and marks the session as closed.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	data, err := encodeRPC(s.nextMessageID(), &rpcCloseSession{})
	if err != nil {
		return s.transport.Close()
	}

	// Best-effort close-session; ignore send/receive errors.
	_ = s.transport.Send(data)

	return s.transport.Close()
}
