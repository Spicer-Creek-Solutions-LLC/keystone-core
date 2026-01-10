// Package cluster provides etcd-based high-availability clustering for Keystone Core.
// It implements:
//   - Cluster membership management with heartbeat monitoring
//   - Leader election using etcd concurrency primitives
//   - Distributed state storage and coordination
//   - Work distribution and agent sharding
//   - Automatic failover and recovery
//
// The cluster package supports both embedded and external etcd deployments,
// enabling single-node development and multi-node production configurations.
package cluster

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// EtcdClient wraps the etcd client with additional functionality.
type EtcdClient struct {
	config         *EtcdConfig
	client         *clientv3.Client
	session        *concurrency.Session
	embeddedServer *EmbeddedEtcdServer
	memberID       string
	mu             sync.RWMutex
	closed         bool
	leaseID        clientv3.LeaseID
	stopChan       chan struct{}
}

// NewEtcdClient creates a new etcd client.
// The memberID is used for embedded mode to identify this node in the cluster.
func NewEtcdClient(config *EtcdConfig, memberID string) (*EtcdClient, error) {
	if config == nil {
		return nil, fmt.Errorf("etcd config is required")
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid etcd config: %w", err)
	}

	return &EtcdClient{
		config:   config,
		memberID: memberID,
		stopChan: make(chan struct{}),
	}, nil
}

// Connect establishes a connection to etcd.
// For embedded mode, this will start an embedded etcd server first.
func (c *EtcdClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrShutdown
	}

	if c.client != nil {
		return nil // Already connected
	}

	endpoints := c.config.Endpoints
	if c.config.Mode == EtcdModeEmbedded {
		// Start embedded etcd server if not already running
		if c.embeddedServer == nil {
			server, err := NewEmbeddedEtcdServer(c.config, c.memberID)
			if err != nil {
				return fmt.Errorf("failed to create embedded etcd server: %w", err)
			}
			c.embeddedServer = server
		}

		if !c.embeddedServer.IsRunning() {
			log.Printf("[INFO] Starting embedded etcd server on port %d...", c.config.Embedded.ClientPort)
			if err := c.embeddedServer.Start(ctx); err != nil {
				return fmt.Errorf("failed to start embedded etcd server: %w", err)
			}
			log.Printf("[INFO] Embedded etcd server started successfully")
		}

		// Connect to the embedded server
		endpoints = []string{c.embeddedServer.ClientEndpoint()}
	}

	clientConfig := clientv3.Config{
		Endpoints:            endpoints,
		DialTimeout:          c.config.DialTimeout,
		AutoSyncInterval:     c.config.AutoSyncInterval,
		Username:             c.config.Username,
		Password:             c.config.Password,
		PermitWithoutStream:  true,
		RejectOldCluster:     true,
		MaxCallSendMsgSize:   0, // Use default
		MaxCallRecvMsgSize:   0, // Use default
	}

	// Configure TLS if enabled
	if c.config.TLS != nil && c.config.TLS.Enabled {
		tlsConfig, err := c.buildTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to build TLS config: %w", err)
		}
		clientConfig.TLS = tlsConfig
	}

	client, err := clientv3.New(clientConfig)
	if err != nil {
		return fmt.Errorf("failed to create etcd client: %w", err)
	}

	// Verify connection with a status check
	statusCtx, cancel := context.WithTimeout(ctx, c.config.DialTimeout)
	defer cancel()

	for _, ep := range endpoints {
		_, err := client.Status(statusCtx, ep)
		if err == nil {
			break
		}
		// Try next endpoint
	}

	c.client = client
	return nil
}

// buildTLSConfig builds a TLS configuration from the etcd TLS config.
func (c *EtcdClient) buildTLSConfig() (*tls.Config, error) {
	// Log critical security warning if InsecureSkipVerify is enabled
	if c.config.TLS.InsecureSkipVerify {
		log.Printf("[SECURITY WARNING] etcd TLS certificate verification is DISABLED (InsecureSkipVerify=true). " +
			"This allows man-in-the-middle attacks on cluster communication. " +
			"This should NEVER be used in production environments.")
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.config.TLS.InsecureSkipVerify,
		ServerName:         c.config.TLS.ServerName,
	}

	// Load client certificate if provided
	if c.config.TLS.CertFile != "" && c.config.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.config.TLS.CertFile, c.config.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate if provided
	if c.config.TLS.CAFile != "" {
		caCert, err := os.ReadFile(c.config.TLS.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// Close closes the etcd client connection and stops the embedded server if running.
func (c *EtcdClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	close(c.stopChan)

	var errs []error

	if c.session != nil {
		if err := c.session.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close session: %w", err))
		}
		c.session = nil
	}

	if c.client != nil {
		if err := c.client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close client: %w", err))
		}
		c.client = nil
	}

	// Stop embedded server if running
	if c.embeddedServer != nil {
		log.Printf("[INFO] Stopping embedded etcd server...")
		if err := c.embeddedServer.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop embedded server: %w", err))
		}
		c.embeddedServer = nil
		log.Printf("[INFO] Embedded etcd server stopped")
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// IsConnected returns true if the client is connected.
func (c *EtcdClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client != nil && !c.closed
}

// Client returns the underlying etcd client.
// Returns nil if not connected.
func (c *EtcdClient) Client() *clientv3.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

// CreateSession creates a concurrency session with the given TTL.
func (c *EtcdClient) CreateSession(ctx context.Context, ttl int) (*concurrency.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return nil, ErrEtcdNotConnected
	}

	if c.closed {
		return nil, ErrShutdown
	}

	session, err := concurrency.NewSession(c.client, concurrency.WithTTL(ttl))
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	c.session = session
	c.leaseID = clientv3.LeaseID(session.Lease())
	return session, nil
}

// Session returns the current session, or nil if not created.
func (c *EtcdClient) Session() *concurrency.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

// LeaseID returns the current lease ID.
func (c *EtcdClient) LeaseID() clientv3.LeaseID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leaseID
}

// Get retrieves a value by key.
func (c *EtcdClient) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	client := c.client
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return nil, ErrShutdown
	}

	if client == nil {
		return nil, ErrEtcdNotConnected
	}

	fullKey := c.fullKey(key)
	resp, err := client.Get(ctx, fullKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get key %s: %w", key, err)
	}

	if len(resp.Kvs) == 0 {
		return nil, nil
	}

	return resp.Kvs[0].Value, nil
}

// Put stores a value with optional TTL.
func (c *EtcdClient) Put(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.RLock()
	client := c.client
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return ErrShutdown
	}

	if client == nil {
		return ErrEtcdNotConnected
	}

	fullKey := c.fullKey(key)
	var opts []clientv3.OpOption

	if ttl > 0 {
		// Create a lease for TTL
		leaseResp, err := client.Grant(ctx, int64(ttl.Seconds()))
		if err != nil {
			return fmt.Errorf("failed to create lease: %w", err)
		}
		opts = append(opts, clientv3.WithLease(leaseResp.ID))
	}

	_, err := client.Put(ctx, fullKey, string(value), opts...)
	if err != nil {
		return fmt.Errorf("failed to put key %s: %w", key, err)
	}

	return nil
}

// PutWithLease stores a value with the session's lease.
func (c *EtcdClient) PutWithLease(ctx context.Context, key string, value []byte) error {
	c.mu.RLock()
	client := c.client
	leaseID := c.leaseID
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return ErrShutdown
	}

	if client == nil {
		return ErrEtcdNotConnected
	}

	if leaseID == 0 {
		return fmt.Errorf("no session lease available")
	}

	fullKey := c.fullKey(key)
	_, err := client.Put(ctx, fullKey, string(value), clientv3.WithLease(leaseID))
	if err != nil {
		return fmt.Errorf("failed to put key %s with lease: %w", key, err)
	}

	return nil
}

// Delete removes a key.
func (c *EtcdClient) Delete(ctx context.Context, key string) error {
	c.mu.RLock()
	client := c.client
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return ErrShutdown
	}

	if client == nil {
		return ErrEtcdNotConnected
	}

	fullKey := c.fullKey(key)
	_, err := client.Delete(ctx, fullKey)
	if err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}

	return nil
}

// DeletePrefix removes all keys with the given prefix.
func (c *EtcdClient) DeletePrefix(ctx context.Context, prefix string) error {
	c.mu.RLock()
	client := c.client
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return ErrShutdown
	}

	if client == nil {
		return ErrEtcdNotConnected
	}

	fullPrefix := c.fullKey(prefix)
	_, err := client.Delete(ctx, fullPrefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to delete prefix %s: %w", prefix, err)
	}

	return nil
}

// List returns all key-value pairs with the given prefix.
func (c *EtcdClient) List(ctx context.Context, prefix string) (map[string][]byte, error) {
	c.mu.RLock()
	client := c.client
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return nil, ErrShutdown
	}

	if client == nil {
		return nil, ErrEtcdNotConnected
	}

	fullPrefix := c.fullKey(prefix)
	resp, err := client.Get(ctx, fullPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list prefix %s: %w", prefix, err)
	}

	result := make(map[string][]byte, len(resp.Kvs))
	prefixLen := len(c.config.KeyPrefix)
	for _, kv := range resp.Kvs {
		// Strip the key prefix from the returned key
		key := string(kv.Key)
		if len(key) > prefixLen {
			key = key[prefixLen:]
		}
		result[key] = kv.Value
	}

	return result, nil
}

// Watch watches for changes to a key prefix.
func (c *EtcdClient) Watch(ctx context.Context, prefix string, handler func(key string, value []byte, deleted bool)) error {
	c.mu.RLock()
	client := c.client
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return ErrShutdown
	}

	if client == nil {
		return ErrEtcdNotConnected
	}

	fullPrefix := c.fullKey(prefix)
	watchChan := client.Watch(ctx, fullPrefix, clientv3.WithPrefix())

	go func() {
		prefixLen := len(c.config.KeyPrefix)
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.stopChan:
				return
			case wresp, ok := <-watchChan:
				if !ok {
					return
				}
				for _, event := range wresp.Events {
					key := string(event.Kv.Key)
					if len(key) > prefixLen {
						key = key[prefixLen:]
					}
					deleted := event.Type == clientv3.EventTypeDelete
					handler(key, event.Kv.Value, deleted)
				}
			}
		}
	}()

	return nil
}

// CompareAndSwap atomically updates a key if it matches the expected value.
func (c *EtcdClient) CompareAndSwap(ctx context.Context, key string, expected, value []byte) (bool, error) {
	c.mu.RLock()
	client := c.client
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return false, ErrShutdown
	}

	if client == nil {
		return false, ErrEtcdNotConnected
	}

	fullKey := c.fullKey(key)

	var cmp clientv3.Cmp
	if expected == nil {
		// Key should not exist
		cmp = clientv3.Compare(clientv3.CreateRevision(fullKey), "=", 0)
	} else {
		// Key should have the expected value
		cmp = clientv3.Compare(clientv3.Value(fullKey), "=", string(expected))
	}

	txn := client.Txn(ctx).If(cmp).Then(
		clientv3.OpPut(fullKey, string(value)),
	)

	resp, err := txn.Commit()
	if err != nil {
		return false, fmt.Errorf("failed to compare and swap key %s: %w", key, err)
	}

	return resp.Succeeded, nil
}

// KeepAlive starts keeping the session lease alive.
// Returns a channel that receives keep-alive responses.
func (c *EtcdClient) KeepAlive(ctx context.Context) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
	c.mu.RLock()
	client := c.client
	leaseID := c.leaseID
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return nil, ErrShutdown
	}

	if client == nil {
		return nil, ErrEtcdNotConnected
	}

	if leaseID == 0 {
		return nil, fmt.Errorf("no session lease available")
	}

	ch, err := client.KeepAlive(ctx, leaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to start keep-alive: %w", err)
	}

	return ch, nil
}

// RevokeSession revokes the current session's lease.
func (c *EtcdClient) RevokeSession(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return ErrEtcdNotConnected
	}

	if c.leaseID == 0 {
		return nil
	}

	_, err := c.client.Revoke(ctx, c.leaseID)
	if err != nil {
		return fmt.Errorf("failed to revoke lease: %w", err)
	}

	c.leaseID = 0
	c.session = nil
	return nil
}

// Health checks etcd health.
func (c *EtcdClient) Health(ctx context.Context) error {
	c.mu.RLock()
	client := c.client
	closed := c.closed
	endpoints := c.config.Endpoints
	c.mu.RUnlock()

	if closed {
		return ErrShutdown
	}

	if client == nil {
		return ErrEtcdNotConnected
	}

	if c.config.Mode == EtcdModeEmbedded {
		// Use the configured client endpoint with IPv6 support
		endpoints = []string{c.config.Embedded.GetClientEndpoint()}
	}

	for _, ep := range endpoints {
		_, err := client.Status(ctx, ep)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("all etcd endpoints unhealthy")
}

// Endpoints returns the configured etcd endpoints.
// For embedded mode, returns the client endpoint with proper IPv6 formatting.
func (c *EtcdClient) Endpoints() []string {
	if c.config.Mode == EtcdModeEmbedded {
		return []string{c.config.Embedded.GetClientEndpoint()}
	}
	return c.config.Endpoints
}

// fullKey returns the full key with prefix.
func (c *EtcdClient) fullKey(key string) string {
	return c.config.KeyPrefix + key
}

// WithRetry executes a function with retry logic.
func (c *EtcdClient) WithRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	for i := 0; i < c.config.MaxRetries; i++ {
		if err := fn(); err != nil {
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.config.RetryInterval):
				continue
			}
		}
		return nil
	}
	return fmt.Errorf("operation failed after %d retries: %w", c.config.MaxRetries, lastErr)
}

// Transaction starts a new transaction.
func (c *EtcdClient) Transaction(ctx context.Context) *Transaction {
	return &Transaction{
		client: c,
		ctx:    ctx,
		ops:    make([]clientv3.Op, 0),
		cmps:   make([]clientv3.Cmp, 0),
	}
}

// Transaction represents an etcd transaction.
type Transaction struct {
	client *EtcdClient
	ctx    context.Context
	ops    []clientv3.Op
	cmps   []clientv3.Cmp
}

// If adds a comparison to the transaction.
func (t *Transaction) If(key string, cmp string, value string) *Transaction {
	fullKey := t.client.fullKey(key)
	var c clientv3.Cmp
	switch cmp {
	case "=":
		c = clientv3.Compare(clientv3.Value(fullKey), "=", value)
	case "!=":
		c = clientv3.Compare(clientv3.Value(fullKey), "!=", value)
	case ">":
		c = clientv3.Compare(clientv3.Value(fullKey), ">", value)
	case "<":
		c = clientv3.Compare(clientv3.Value(fullKey), "<", value)
	default:
		c = clientv3.Compare(clientv3.Value(fullKey), "=", value)
	}
	t.cmps = append(t.cmps, c)
	return t
}

// IfNotExists adds a condition that the key should not exist.
func (t *Transaction) IfNotExists(key string) *Transaction {
	fullKey := t.client.fullKey(key)
	t.cmps = append(t.cmps, clientv3.Compare(clientv3.CreateRevision(fullKey), "=", 0))
	return t
}

// IfExists adds a condition that the key should exist.
func (t *Transaction) IfExists(key string) *Transaction {
	fullKey := t.client.fullKey(key)
	t.cmps = append(t.cmps, clientv3.Compare(clientv3.CreateRevision(fullKey), ">", 0))
	return t
}

// Put adds a put operation to the transaction.
func (t *Transaction) Put(key string, value []byte) *Transaction {
	fullKey := t.client.fullKey(key)
	t.ops = append(t.ops, clientv3.OpPut(fullKey, string(value)))
	return t
}

// Delete adds a delete operation to the transaction.
func (t *Transaction) Delete(key string) *Transaction {
	fullKey := t.client.fullKey(key)
	t.ops = append(t.ops, clientv3.OpDelete(fullKey))
	return t
}

// Commit commits the transaction.
func (t *Transaction) Commit() (bool, error) {
	t.client.mu.RLock()
	client := t.client.client
	closed := t.client.closed
	t.client.mu.RUnlock()

	if closed {
		return false, ErrShutdown
	}

	if client == nil {
		return false, ErrEtcdNotConnected
	}

	txn := client.Txn(t.ctx)
	if len(t.cmps) > 0 {
		txn = txn.If(t.cmps...)
	}
	txn = txn.Then(t.ops...)

	resp, err := txn.Commit()
	if err != nil {
		return false, fmt.Errorf("transaction failed: %w", err)
	}

	return resp.Succeeded, nil
}
