// Package ssh provides an SSH protocol adapter for proxy agents.
package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// TunnelAdapter implements the TunnelAdapter interface for SSH.
type TunnelAdapter struct {
	*Adapter
	tunnels   map[string]*activeTunnel
	tunnelsMu sync.Mutex
}

// activeTunnel represents an active SSH tunnel.
type activeTunnel struct {
	id            string
	tunnelType    string // "local", "remote", "dynamic"
	localAddr     string
	remoteAddr    string
	listener      net.Listener
	active        bool
	bytesSent     int64
	bytesReceived int64
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// NewTunnelAdapter creates a new tunnel adapter.
func NewTunnelAdapter(config *Config) *TunnelAdapter {
	return &TunnelAdapter{
		Adapter: NewAdapter(config),
		tunnels: make(map[string]*activeTunnel),
	}
}

// Connect establishes an SSH connection for tunneling.
func (a *TunnelAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	return a.Adapter.Connect(ctx, device, cred)
}

// Disconnect closes all tunnels and the SSH connection.
func (a *TunnelAdapter) Disconnect(ctx context.Context) error {
	// Close all tunnels first
	a.tunnelsMu.Lock()
	for _, tunnel := range a.tunnels {
		if tunnel.cancel != nil {
			tunnel.cancel()
		}
		if tunnel.listener != nil {
			tunnel.listener.Close()
		}
		tunnel.active = false
	}
	a.tunnels = make(map[string]*activeTunnel)
	a.tunnelsMu.Unlock()

	return a.Adapter.Disconnect(ctx)
}

// LocalForward creates a local port forward.
// Traffic to local address is forwarded through SSH to the remote address.
func (a *TunnelAdapter) LocalForward(ctx context.Context, req *protocols.ForwardRequest) (*protocols.Tunnel, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	localAddr := fmt.Sprintf("%s:%d", req.LocalHost, req.LocalPort)
	remoteAddr := fmt.Sprintf("%s:%d", req.RemoteHost, req.RemotePort)

	// Create local listener
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create local listener: %w", err)
	}

	// Get actual local address (in case port was 0)
	actualLocalAddr := listener.Addr().String()

	// Create tunnel context
	ctx, cancel := context.WithCancel(ctx)

	tunnel := &activeTunnel{
		id:         uuid.New().String(),
		tunnelType: "local",
		localAddr:  actualLocalAddr,
		remoteAddr: remoteAddr,
		listener:   listener,
		active:     true,
		cancel:     cancel,
	}

	// Store tunnel
	a.tunnelsMu.Lock()
	a.tunnels[tunnel.id] = tunnel
	a.tunnelsMu.Unlock()

	// Start accepting connections
	tunnel.wg.Add(1)
	go func() {
		defer tunnel.wg.Done()
		a.acceptLocalConnections(ctx, tunnel, client)
	}()

	return &protocols.Tunnel{
		ID:         tunnel.id,
		Type:       "local",
		LocalAddr:  actualLocalAddr,
		RemoteAddr: remoteAddr,
		Active:     true,
		Close: func() error {
			return a.closeTunnel(tunnel.id)
		},
	}, nil
}

// acceptLocalConnections accepts local connections and forwards them.
func (a *TunnelAdapter) acceptLocalConnections(ctx context.Context, tunnel *activeTunnel, client interface{ Dial(string, string) (net.Conn, error) }) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := tunnel.listener.Accept()
		if err != nil {
			// Check if we're closing
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}

		// Forward this connection
		go a.forwardLocalConnection(ctx, tunnel, conn, client)
	}
}

// forwardLocalConnection forwards a single local connection through SSH.
func (a *TunnelAdapter) forwardLocalConnection(ctx context.Context, tunnel *activeTunnel, localConn net.Conn, client interface{ Dial(string, string) (net.Conn, error) }) {
	defer localConn.Close()

	// Connect to remote through SSH
	remoteConn, err := client.Dial("tcp", tunnel.remoteAddr)
	if err != nil {
		return
	}
	defer remoteConn.Close()

	// Forward data in both directions
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		sent, _ := io.Copy(remoteConn, localConn)
		atomic.AddInt64(&tunnel.bytesSent, sent)
	}()

	go func() {
		defer wg.Done()
		received, _ := io.Copy(localConn, remoteConn)
		atomic.AddInt64(&tunnel.bytesReceived, received)
	}()

	// Wait for either direction to close
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

// RemoteForward creates a remote port forward.
// Traffic to remote address is forwarded through SSH to the local address.
func (a *TunnelAdapter) RemoteForward(ctx context.Context, req *protocols.ForwardRequest) (*protocols.Tunnel, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	remoteAddr := fmt.Sprintf("%s:%d", req.RemoteHost, req.RemotePort)
	localAddr := fmt.Sprintf("%s:%d", req.LocalHost, req.LocalPort)

	// Request remote port forward
	listener, err := client.Listen("tcp", remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create remote listener: %w", err)
	}

	// Get actual remote address
	actualRemoteAddr := listener.Addr().String()

	// Create tunnel context
	ctx, cancel := context.WithCancel(ctx)

	tunnel := &activeTunnel{
		id:         uuid.New().String(),
		tunnelType: "remote",
		localAddr:  localAddr,
		remoteAddr: actualRemoteAddr,
		listener:   listener,
		active:     true,
		cancel:     cancel,
	}

	// Store tunnel
	a.tunnelsMu.Lock()
	a.tunnels[tunnel.id] = tunnel
	a.tunnelsMu.Unlock()

	// Start accepting connections
	tunnel.wg.Add(1)
	go func() {
		defer tunnel.wg.Done()
		a.acceptRemoteConnections(ctx, tunnel)
	}()

	return &protocols.Tunnel{
		ID:         tunnel.id,
		Type:       "remote",
		LocalAddr:  localAddr,
		RemoteAddr: actualRemoteAddr,
		Active:     true,
		Close: func() error {
			return a.closeTunnel(tunnel.id)
		},
	}, nil
}

// acceptRemoteConnections accepts remote connections and forwards them locally.
func (a *TunnelAdapter) acceptRemoteConnections(ctx context.Context, tunnel *activeTunnel) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := tunnel.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}

		// Forward this connection
		go a.forwardRemoteConnection(ctx, tunnel, conn)
	}
}

// forwardRemoteConnection forwards a single remote connection to local.
func (a *TunnelAdapter) forwardRemoteConnection(ctx context.Context, tunnel *activeTunnel, remoteConn net.Conn) {
	defer remoteConn.Close()

	// Connect to local address
	localConn, err := net.Dial("tcp", tunnel.localAddr)
	if err != nil {
		return
	}
	defer localConn.Close()

	// Forward data in both directions
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		sent, _ := io.Copy(localConn, remoteConn)
		atomic.AddInt64(&tunnel.bytesSent, sent)
	}()

	go func() {
		defer wg.Done()
		received, _ := io.Copy(remoteConn, localConn)
		atomic.AddInt64(&tunnel.bytesReceived, received)
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

// DynamicForward creates a SOCKS proxy.
func (a *TunnelAdapter) DynamicForward(ctx context.Context, localAddr string) (*protocols.Tunnel, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Create local SOCKS listener
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS listener: %w", err)
	}

	actualLocalAddr := listener.Addr().String()

	// Create tunnel context
	ctx, cancel := context.WithCancel(ctx)

	tunnel := &activeTunnel{
		id:         uuid.New().String(),
		tunnelType: "dynamic",
		localAddr:  actualLocalAddr,
		remoteAddr: "SOCKS",
		listener:   listener,
		active:     true,
		cancel:     cancel,
	}

	// Store tunnel
	a.tunnelsMu.Lock()
	a.tunnels[tunnel.id] = tunnel
	a.tunnelsMu.Unlock()

	// Start accepting SOCKS connections
	tunnel.wg.Add(1)
	go func() {
		defer tunnel.wg.Done()
		a.acceptSOCKSConnections(ctx, tunnel, client)
	}()

	return &protocols.Tunnel{
		ID:         tunnel.id,
		Type:       "dynamic",
		LocalAddr:  actualLocalAddr,
		RemoteAddr: "SOCKS",
		Active:     true,
		Close: func() error {
			return a.closeTunnel(tunnel.id)
		},
	}, nil
}

// acceptSOCKSConnections accepts SOCKS connections.
func (a *TunnelAdapter) acceptSOCKSConnections(ctx context.Context, tunnel *activeTunnel, client interface{ Dial(string, string) (net.Conn, error) }) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := tunnel.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}

		// Handle SOCKS connection
		go a.handleSOCKSConnection(ctx, tunnel, conn, client)
	}
}

// handleSOCKSConnection handles a single SOCKS connection.
// Implements SOCKS5 protocol (simplified).
func (a *TunnelAdapter) handleSOCKSConnection(ctx context.Context, tunnel *activeTunnel, localConn net.Conn, client interface{ Dial(string, string) (net.Conn, error) }) {
	defer localConn.Close()

	// Read SOCKS5 greeting
	buf := make([]byte, 256)
	n, err := localConn.Read(buf)
	if err != nil || n < 2 {
		return
	}

	// Check version (5)
	if buf[0] != 5 {
		return
	}

	// Reply with no authentication required
	_, err = localConn.Write([]byte{5, 0})
	if err != nil {
		return
	}

	// Read SOCKS5 request
	n, err = localConn.Read(buf)
	if err != nil || n < 7 {
		return
	}

	// Check version and command
	if buf[0] != 5 || buf[1] != 1 { // Only support CONNECT
		localConn.Write([]byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0}) // Command not supported
		return
	}

	// Parse destination
	var destAddr string
	var destPort int

	switch buf[3] {
	case 1: // IPv4
		if n < 10 {
			return
		}
		destAddr = fmt.Sprintf("%d.%d.%d.%d", buf[4], buf[5], buf[6], buf[7])
		destPort = int(buf[8])<<8 | int(buf[9])

	case 3: // Domain name
		domainLen := int(buf[4])
		if n < 5+domainLen+2 {
			return
		}
		destAddr = string(buf[5 : 5+domainLen])
		destPort = int(buf[5+domainLen])<<8 | int(buf[5+domainLen+1])

	case 4: // IPv6
		if n < 22 {
			return
		}
		destAddr = fmt.Sprintf("[%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x]",
			buf[4], buf[5], buf[6], buf[7], buf[8], buf[9], buf[10], buf[11],
			buf[12], buf[13], buf[14], buf[15], buf[16], buf[17], buf[18], buf[19])
		destPort = int(buf[20])<<8 | int(buf[21])

	default:
		localConn.Write([]byte{5, 8, 0, 1, 0, 0, 0, 0, 0, 0}) // Address type not supported
		return
	}

	// Connect through SSH
	remoteAddr := fmt.Sprintf("%s:%d", destAddr, destPort)
	remoteConn, err := client.Dial("tcp", remoteAddr)
	if err != nil {
		localConn.Write([]byte{5, 4, 0, 1, 0, 0, 0, 0, 0, 0}) // Host unreachable
		return
	}
	defer remoteConn.Close()

	// Send success response
	_, err = localConn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	if err != nil {
		return
	}

	// Forward data
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		sent, _ := io.Copy(remoteConn, localConn)
		atomic.AddInt64(&tunnel.bytesSent, sent)
	}()

	go func() {
		defer wg.Done()
		received, _ := io.Copy(localConn, remoteConn)
		atomic.AddInt64(&tunnel.bytesReceived, received)
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

// closeTunnel closes a specific tunnel.
func (a *TunnelAdapter) closeTunnel(id string) error {
	a.tunnelsMu.Lock()
	tunnel, ok := a.tunnels[id]
	if ok {
		delete(a.tunnels, id)
	}
	a.tunnelsMu.Unlock()

	if !ok {
		return fmt.Errorf("tunnel not found: %s", id)
	}

	if tunnel.cancel != nil {
		tunnel.cancel()
	}

	if tunnel.listener != nil {
		tunnel.listener.Close()
	}

	tunnel.active = false

	// Wait for goroutines to finish
	tunnel.wg.Wait()

	return nil
}

// ListTunnels returns all active tunnels.
func (a *TunnelAdapter) ListTunnels() []*protocols.Tunnel {
	a.tunnelsMu.Lock()
	defer a.tunnelsMu.Unlock()

	tunnels := make([]*protocols.Tunnel, 0, len(a.tunnels))
	for _, t := range a.tunnels {
		tunnels = append(tunnels, &protocols.Tunnel{
			ID:            t.id,
			Type:          t.tunnelType,
			LocalAddr:     t.localAddr,
			RemoteAddr:    t.remoteAddr,
			Active:        t.active,
			BytesSent:     atomic.LoadInt64(&t.bytesSent),
			BytesReceived: atomic.LoadInt64(&t.bytesReceived),
			Close: func() error {
				return a.closeTunnel(t.id)
			},
		})
	}

	return tunnels
}

// GetTunnel returns a specific tunnel by ID.
func (a *TunnelAdapter) GetTunnel(id string) (*protocols.Tunnel, error) {
	a.tunnelsMu.Lock()
	t, ok := a.tunnels[id]
	a.tunnelsMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("tunnel not found: %s", id)
	}

	return &protocols.Tunnel{
		ID:            t.id,
		Type:          t.tunnelType,
		LocalAddr:     t.localAddr,
		RemoteAddr:    t.remoteAddr,
		Active:        t.active,
		BytesSent:     atomic.LoadInt64(&t.bytesSent),
		BytesReceived: atomic.LoadInt64(&t.bytesReceived),
		Close: func() error {
			return a.closeTunnel(t.id)
		},
	}, nil
}

// NewTunnelAdapterFactory creates a tunnel adapter factory for SSH.
func NewTunnelAdapterFactory(config *Config) protocols.TunnelAdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.TunnelAdapter, error) {
		cfg := config
		if cfg == nil {
			cfg = DefaultConfig()
		}
		cfg.ConnectionConfig = connConfig
		return NewTunnelAdapter(cfg), nil
	}
}

// init registers the tunnel adapter with the default registry.
func init() {
	protocols.RegisterTunnel(protocols.ProtocolSSH, NewTunnelAdapterFactory(nil))
}
