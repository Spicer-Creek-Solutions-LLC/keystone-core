package cluster

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// CoordinationClient manages connections to peer control plane servers
// for cluster coordination when NATS is unavailable.
type CoordinationClient struct {
	localID string
	config  *CoordinationClientConfig

	// Connection pool to peer servers
	mu    sync.RWMutex
	peers map[string]*peerConnection

	// Heartbeat tracking
	heartbeatSeq int64
}

// peerConnection holds a connection to a single peer server.
type peerConnection struct {
	memberID string
	address  string
	conn     *grpc.ClientConn
	client   pb.CoordinationServiceClient
	lastSeen time.Time
	healthy  bool
}

// CoordinationClientConfig holds configuration for the coordination client.
type CoordinationClientConfig struct {
	// LocalID is this server's member ID
	LocalID string
	// TLSConfig for mTLS authentication (optional, nil = insecure)
	TLSConfig *tls.Config
	// DialTimeout is the timeout for establishing connections
	DialTimeout time.Duration
	// RequestTimeout is the default timeout for RPC requests
	RequestTimeout time.Duration
	// KeepaliveInterval is how often to send keepalive pings
	KeepaliveInterval time.Duration
	// KeepaliveTimeout is how long to wait for keepalive response
	KeepaliveTimeout time.Duration
	// MaxRetries is the maximum number of retries for failed requests
	MaxRetries int
	// RetryBackoff is the initial backoff between retries
	RetryBackoff time.Duration
}

// DefaultCoordinationClientConfig returns default configuration.
func DefaultCoordinationClientConfig(localID string) *CoordinationClientConfig {
	return &CoordinationClientConfig{
		LocalID:           localID,
		DialTimeout:       5 * time.Second,
		RequestTimeout:    10 * time.Second,
		KeepaliveInterval: 10 * time.Second,
		KeepaliveTimeout:  5 * time.Second,
		MaxRetries:        3,
		RetryBackoff:      time.Second,
	}
}

// NewCoordinationClient creates a new coordination client.
func NewCoordinationClient(config *CoordinationClientConfig) (*CoordinationClient, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.LocalID == "" {
		return nil, fmt.Errorf("local_id is required")
	}

	return &CoordinationClient{
		localID: config.LocalID,
		config:  config,
		peers:   make(map[string]*peerConnection),
	}, nil
}

// AddPeer adds a peer server to the connection pool.
func (c *CoordinationClient) AddPeer(memberID, address string) error {
	if memberID == "" {
		return fmt.Errorf("member_id is required")
	}
	if address == "" {
		return fmt.Errorf("address is required")
	}
	if memberID == c.localID {
		return nil // Don't connect to ourselves
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if peer already exists
	if existing, ok := c.peers[memberID]; ok {
		if existing.address == address {
			return nil // Already connected to same address
		}
		// Address changed, close old connection
		if existing.conn != nil {
			existing.conn.Close()
		}
	}

	// Establish new connection
	conn, err := c.dial(address)
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s at %s: %w", memberID, address, err)
	}

	c.peers[memberID] = &peerConnection{
		memberID: memberID,
		address:  address,
		conn:     conn,
		client:   pb.NewCoordinationServiceClient(conn),
		lastSeen: time.Now(),
		healthy:  true,
	}

	return nil
}

// RemovePeer removes a peer server from the connection pool.
func (c *CoordinationClient) RemovePeer(memberID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if peer, ok := c.peers[memberID]; ok {
		if peer.conn != nil {
			peer.conn.Close()
		}
		delete(c.peers, memberID)
	}
}

// GetPeerCount returns the number of connected peers.
func (c *CoordinationClient) GetPeerCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.peers)
}

// GetHealthyPeerCount returns the number of healthy peers.
func (c *CoordinationClient) GetHealthyPeerCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for _, peer := range c.peers {
		if peer.healthy {
			count++
		}
	}
	return count
}

// Close closes all peer connections.
func (c *CoordinationClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error
	for _, peer := range c.peers {
		if peer.conn != nil {
			if err := peer.conn.Close(); err != nil {
				lastErr = err
			}
		}
	}
	c.peers = make(map[string]*peerConnection)

	return lastErr
}

// dial establishes a gRPC connection to a peer.
func (c *CoordinationClient) dial(address string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.DialTimeout)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                c.config.KeepaliveInterval,
			Timeout:             c.config.KeepaliveTimeout,
			PermitWithoutStream: true,
		}),
	}

	// Configure TLS
	if c.config.TLSConfig != nil {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(c.config.TLSConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return grpc.DialContext(ctx, address, opts...)
}

// ClusterHealth queries cluster health from a specific peer.
func (c *CoordinationClient) ClusterHealth(ctx context.Context, memberID string, includeMembers, includeNATS bool) (*pb.ClusterHealthResponse, error) {
	peer, err := c.getPeer(memberID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	return peer.client.ClusterHealth(ctx, &pb.ClusterHealthRequest{
		RequestId:      c.newRequestID(),
		IncludeMembers: includeMembers,
		IncludeNats:    includeNATS,
	})
}

// ClusterHealthAll queries cluster health from all peers and returns combined results.
func (c *CoordinationClient) ClusterHealthAll(ctx context.Context, includeMembers, includeNATS bool) (map[string]*pb.ClusterHealthResponse, error) {
	c.mu.RLock()
	peers := make([]*peerConnection, 0, len(c.peers))
	for _, p := range c.peers {
		peers = append(peers, p)
	}
	c.mu.RUnlock()

	results := make(map[string]*pb.ClusterHealthResponse)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, peer := range peers {
		wg.Add(1)
		go func(p *peerConnection) {
			defer wg.Done()

			ctx, cancel := c.withTimeout(ctx)
			defer cancel()

			resp, err := p.client.ClusterHealth(ctx, &pb.ClusterHealthRequest{
				RequestId:      c.newRequestID(),
				IncludeMembers: includeMembers,
				IncludeNats:    includeNATS,
			})

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				c.markPeerUnhealthy(p.memberID)
			} else {
				c.markPeerHealthy(p.memberID)
				results[p.memberID] = resp
			}
		}(peer)
	}

	wg.Wait()
	return results, nil
}

// GetLeader queries leader info from a specific peer.
func (c *CoordinationClient) GetLeader(ctx context.Context, memberID string) (*pb.GetLeaderResponse, error) {
	peer, err := c.getPeer(memberID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	return peer.client.GetLeader(ctx, &pb.GetLeaderRequest{
		RequestId: c.newRequestID(),
	})
}

// NATSStatus queries NATS status from a specific peer.
func (c *CoordinationClient) NATSStatus(ctx context.Context, memberID string) (*pb.NATSStatusResponse, error) {
	peer, err := c.getPeer(memberID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	return peer.client.NATSStatus(ctx, &pb.NATSStatusRequest{
		RequestId:   c.newRequestID(),
		RequesterId: c.localID,
	})
}

// NATSStatusAll queries NATS status from all peers.
func (c *CoordinationClient) NATSStatusAll(ctx context.Context) (map[string]*pb.NATSStatusResponse, error) {
	c.mu.RLock()
	peers := make([]*peerConnection, 0, len(c.peers))
	for _, p := range c.peers {
		peers = append(peers, p)
	}
	c.mu.RUnlock()

	results := make(map[string]*pb.NATSStatusResponse)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, peer := range peers {
		wg.Add(1)
		go func(p *peerConnection) {
			defer wg.Done()

			ctx, cancel := c.withTimeout(ctx)
			defer cancel()

			resp, err := p.client.NATSStatus(ctx, &pb.NATSStatusRequest{
				RequestId:   c.newRequestID(),
				RequesterId: c.localID,
			})

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				c.markPeerUnhealthy(p.memberID)
			} else {
				c.markPeerHealthy(p.memberID)
				results[p.memberID] = resp
			}
		}(peer)
	}

	wg.Wait()
	return results, nil
}

// RecoveryCoordinate sends a recovery coordination request to a peer.
func (c *CoordinationClient) RecoveryCoordinate(ctx context.Context, memberID string, action pb.RecoveryAction, params map[string]string) (*pb.RecoveryCoordinateResponse, error) {
	peer, err := c.getPeer(memberID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	return peer.client.RecoveryCoordinate(ctx, &pb.RecoveryCoordinateRequest{
		RequestId:   c.newRequestID(),
		InitiatorId: c.localID,
		Action:      action,
		Parameters:  params,
	})
}

// RecoveryCoordinateAll sends a recovery coordination request to all peers.
func (c *CoordinationClient) RecoveryCoordinateAll(ctx context.Context, action pb.RecoveryAction, params map[string]string) (map[string]*pb.RecoveryCoordinateResponse, error) {
	c.mu.RLock()
	peers := make([]*peerConnection, 0, len(c.peers))
	for _, p := range c.peers {
		peers = append(peers, p)
	}
	c.mu.RUnlock()

	results := make(map[string]*pb.RecoveryCoordinateResponse)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, peer := range peers {
		wg.Add(1)
		go func(p *peerConnection) {
			defer wg.Done()

			ctx, cancel := c.withTimeout(ctx)
			defer cancel()

			resp, err := p.client.RecoveryCoordinate(ctx, &pb.RecoveryCoordinateRequest{
				RequestId:   c.newRequestID(),
				InitiatorId: c.localID,
				Action:      action,
				Parameters:  params,
			})

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				c.markPeerUnhealthy(p.memberID)
			} else {
				c.markPeerHealthy(p.memberID)
				results[p.memberID] = resp
			}
		}(peer)
	}

	wg.Wait()
	return results, nil
}

// Heartbeat sends a heartbeat to a specific peer.
func (c *CoordinationClient) Heartbeat(ctx context.Context, memberID string) (*pb.ServerHeartbeatResponse, error) {
	peer, err := c.getPeer(memberID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	c.mu.Lock()
	seq := c.heartbeatSeq
	c.heartbeatSeq++
	c.mu.Unlock()

	resp, err := peer.client.Heartbeat(ctx, &pb.ServerHeartbeatRequest{
		SenderId:  c.localID,
		Timestamp: timestamppb.Now(),
		Sequence:  seq,
	})

	if err != nil {
		c.markPeerUnhealthy(memberID)
		return nil, err
	}

	c.markPeerHealthy(memberID)
	return resp, nil
}

// HeartbeatAll sends heartbeats to all peers.
func (c *CoordinationClient) HeartbeatAll(ctx context.Context) (map[string]*pb.ServerHeartbeatResponse, map[string]error) {
	c.mu.RLock()
	peers := make([]*peerConnection, 0, len(c.peers))
	for _, p := range c.peers {
		peers = append(peers, p)
	}
	c.mu.RUnlock()

	results := make(map[string]*pb.ServerHeartbeatResponse)
	errors := make(map[string]error)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, peer := range peers {
		wg.Add(1)
		go func(p *peerConnection) {
			defer wg.Done()

			resp, err := c.Heartbeat(ctx, p.memberID)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errors[p.memberID] = err
			} else {
				results[p.memberID] = resp
			}
		}(peer)
	}

	wg.Wait()
	return results, errors
}

// PropagateState propagates state changes to a specific peer.
func (c *CoordinationClient) PropagateState(ctx context.Context, memberID string, updateType pb.StateUpdateType, data []byte, version int64) (*pb.PropagateStateResponse, error) {
	peer, err := c.getPeer(memberID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	return peer.client.PropagateState(ctx, &pb.PropagateStateRequest{
		RequestId:      c.newRequestID(),
		SenderId:       c.localID,
		UpdateType:     updateType,
		StateData:      data,
		Version:        version,
		StateTimestamp: timestamppb.Now(),
	})
}

// PropagateStateAll propagates state changes to all peers.
func (c *CoordinationClient) PropagateStateAll(ctx context.Context, updateType pb.StateUpdateType, data []byte, version int64) (map[string]*pb.PropagateStateResponse, map[string]error) {
	c.mu.RLock()
	peers := make([]*peerConnection, 0, len(c.peers))
	for _, p := range c.peers {
		peers = append(peers, p)
	}
	c.mu.RUnlock()

	results := make(map[string]*pb.PropagateStateResponse)
	errors := make(map[string]error)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, peer := range peers {
		wg.Add(1)
		go func(p *peerConnection) {
			defer wg.Done()

			resp, err := c.PropagateState(ctx, p.memberID, updateType, data, version)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errors[p.memberID] = err
			} else {
				results[p.memberID] = resp
			}
		}(peer)
	}

	wg.Wait()
	return results, errors
}

// getPeer returns the peer connection for a given member ID.
func (c *CoordinationClient) getPeer(memberID string) (*peerConnection, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	peer, ok := c.peers[memberID]
	if !ok {
		return nil, fmt.Errorf("peer %s not found", memberID)
	}

	return peer, nil
}

// markPeerHealthy marks a peer as healthy.
func (c *CoordinationClient) markPeerHealthy(memberID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if peer, ok := c.peers[memberID]; ok {
		peer.healthy = true
		peer.lastSeen = time.Now()
	}
}

// markPeerUnhealthy marks a peer as unhealthy.
func (c *CoordinationClient) markPeerUnhealthy(memberID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if peer, ok := c.peers[memberID]; ok {
		peer.healthy = false
	}
}

// withTimeout creates a context with the configured request timeout.
func (c *CoordinationClient) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.config.RequestTimeout > 0 {
		return context.WithTimeout(ctx, c.config.RequestTimeout)
	}
	return ctx, func() {}
}

// newRequestID generates a new unique request ID.
func (c *CoordinationClient) newRequestID() string {
	return uuid.New().String()
}

// IsPeerHealthy returns true if the peer is connected and healthy.
func (c *CoordinationClient) IsPeerHealthy(memberID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	peer, ok := c.peers[memberID]
	if !ok {
		return false
	}
	return peer.healthy
}

// GetPeerLastSeen returns the last time a peer was successfully contacted.
func (c *CoordinationClient) GetPeerLastSeen(memberID string) (time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	peer, ok := c.peers[memberID]
	if !ok {
		return time.Time{}, fmt.Errorf("peer %s not found", memberID)
	}
	return peer.lastSeen, nil
}

// ListPeers returns a list of all peer member IDs.
func (c *CoordinationClient) ListPeers() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	peers := make([]string, 0, len(c.peers))
	for id := range c.peers {
		peers = append(peers, id)
	}
	return peers
}
