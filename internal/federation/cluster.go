// Package federation provides cluster federation capabilities for Keystone.
package federation

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ClusterState represents the state of a federated cluster.
type ClusterState string

const (
	// StateUnknown indicates unknown state.
	StateUnknown ClusterState = "unknown"
	// StatePending indicates the cluster is pending join.
	StatePending ClusterState = "pending"
	// StateJoining indicates the cluster is joining.
	StateJoining ClusterState = "joining"
	// StateActive indicates the cluster is active.
	StateActive ClusterState = "active"
	// StateDegraded indicates the cluster is degraded.
	StateDegraded ClusterState = "degraded"
	// StateUnreachable indicates the cluster is unreachable.
	StateUnreachable ClusterState = "unreachable"
	// StateLeaving indicates the cluster is leaving.
	StateLeaving ClusterState = "leaving"
	// StateLeft indicates the cluster has left.
	StateLeft ClusterState = "left"
)

// ClusterRole represents the role of a cluster in the federation.
type ClusterRole string

const (
	// RoleLeader indicates the cluster is the leader.
	RoleLeader ClusterRole = "leader"
	// RoleMember indicates the cluster is a member.
	RoleMember ClusterRole = "member"
	// RoleObserver indicates the cluster is an observer.
	RoleObserver ClusterRole = "observer"
)

// Cluster represents a federated cluster.
type Cluster struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Endpoint    string            `json:"endpoint"`
	State       ClusterState      `json:"state"`
	Role        ClusterRole       `json:"role"`
	Region      string            `json:"region,omitempty"`
	Zone        string            `json:"zone,omitempty"`
	Provider    string            `json:"provider,omitempty"` // aws, gcp, azure, on-prem
	Version     string            `json:"version,omitempty"`
	Capacity    *ClusterCapacity  `json:"capacity,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	JoinedAt    *time.Time        `json:"joinedAt,omitempty"`
	LastSeenAt  time.Time         `json:"lastSeenAt"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// ClusterCapacity represents the capacity of a cluster.
type ClusterCapacity struct {
	Nodes        int   `json:"nodes"`
	CPUMillis    int64 `json:"cpuMillis"`
	MemoryBytes  int64 `json:"memoryBytes"`
	StorageBytes int64 `json:"storageBytes"`
	Pods         int   `json:"pods"`
	GPUs         int   `json:"gpus,omitempty"`
}

// ClusterHealth represents the health status of a cluster.
type ClusterHealth struct {
	Healthy       bool               `json:"healthy"`
	Ready         bool               `json:"ready"`
	Conditions    []ClusterCondition `json:"conditions,omitempty"`
	LastCheckedAt time.Time          `json:"lastCheckedAt"`
}

// ClusterCondition represents a condition of a cluster.
type ClusterCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"` // True, False, Unknown
	LastTransitionTime time.Time `json:"lastTransitionTime"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
}

// FederationConfig configures the federation.
type FederationConfig struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	HealthInterval   time.Duration `json:"healthInterval"`
	SyncInterval     time.Duration `json:"syncInterval"`
	HeartbeatTimeout time.Duration `json:"heartbeatTimeout"`
	TLSConfig        *TLSConfig    `json:"tlsConfig,omitempty"`
}

// TLSConfig configures TLS for federation.
type TLSConfig struct {
	CACert     string `json:"caCert,omitempty"`
	ClientCert string `json:"clientCert,omitempty"`
	ClientKey  string `json:"clientKey,omitempty"`
	SkipVerify bool   `json:"skipVerify,omitempty"`
	MinVersion string `json:"minVersion,omitempty"`
}

// DefaultFederationConfig returns a default federation configuration.
func DefaultFederationConfig() *FederationConfig {
	return &FederationConfig{
		HealthInterval:   30 * time.Second,
		SyncInterval:     time.Minute,
		HeartbeatTimeout: 2 * time.Minute,
	}
}

// ClusterStore is the interface for storing clusters.
type ClusterStore interface {
	Get(ctx context.Context, id string) (*Cluster, error)
	List(ctx context.Context) ([]*Cluster, error)
	ListByState(ctx context.Context, state ClusterState) ([]*Cluster, error)
	ListByRegion(ctx context.Context, region string) ([]*Cluster, error)
	Save(ctx context.Context, cluster *Cluster) error
	Delete(ctx context.Context, id string) error
}

// Federation manages a cluster federation.
type Federation struct {
	config     *FederationConfig
	store      ClusterStore
	localID    string
	listeners  []FederationListener
	httpClient *http.Client
	mu         sync.RWMutex
	stopCh     chan struct{}
	running    bool
}

// FederationListener is called when federation events occur.
type FederationListener func(event *FederationEvent)

// FederationEvent represents a federation event.
type FederationEvent struct {
	Type      string                 `json:"type"`
	ClusterID string                 `json:"clusterId,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Message   string                 `json:"message"`
	Error     string                 `json:"error,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// NewFederation creates a new federation.
func NewFederation(config *FederationConfig, store ClusterStore, localID string) *Federation {
	if config == nil {
		config = DefaultFederationConfig()
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	minVersion := parseTLSMinVersion("")
	if config.TLSConfig != nil {
		minVersion = parseTLSMinVersion(config.TLSConfig.MinVersion)
	}

	tlsConfig := &tls.Config{
		MinVersion: minVersion,
	}

	if config.TLSConfig != nil && config.TLSConfig.SkipVerify {
		// #nosec G402 -- SkipVerify is user-configured for dev/test federation setups.
		tlsConfig.InsecureSkipVerify = true
	}

	httpClient.Transport = &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &Federation{
		config:     config,
		store:      store,
		localID:    localID,
		httpClient: httpClient,
		stopCh:     make(chan struct{}),
	}
}

func parseTLSMinVersion(value string) uint16 {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1.3", "tls1.3", "tls13":
		return tls.VersionTLS13
	case "1.2", "tls1.2", "tls12":
		return tls.VersionTLS12
	default:
		return tls.VersionTLS13
	}
}

// AddListener adds a federation event listener.
func (f *Federation) AddListener(listener FederationListener) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listeners = append(f.listeners, listener)
}

// emit sends an event to all listeners.
func (f *Federation) emit(event *FederationEvent) {
	f.mu.RLock()
	listeners := make([]FederationListener, len(f.listeners))
	copy(listeners, f.listeners)
	f.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// Start starts the federation.
func (f *Federation) Start(ctx context.Context) error {
	f.mu.Lock()
	if f.running {
		f.mu.Unlock()
		return fmt.Errorf("federation already running")
	}
	f.running = true
	f.stopCh = make(chan struct{})
	f.mu.Unlock()

	f.emit(&FederationEvent{
		Type:      "federation_started",
		Timestamp: time.Now(),
		Message:   "Federation started",
	})

	// Start health check loop
	go f.healthCheckLoop(ctx)

	// Start sync loop
	go f.syncLoop(ctx)

	return nil
}

// Stop stops the federation.
func (f *Federation) Stop() {
	f.mu.Lock()
	if !f.running {
		f.mu.Unlock()
		return
	}
	close(f.stopCh)
	f.running = false
	f.mu.Unlock()

	f.emit(&FederationEvent{
		Type:      "federation_stopped",
		Timestamp: time.Now(),
		Message:   "Federation stopped",
	})
}

// healthCheckLoop periodically checks cluster health.
func (f *Federation) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(f.config.HealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-f.stopCh:
			return
		case <-ticker.C:
			f.checkAllClusters(ctx)
		}
	}
}

// syncLoop periodically syncs cluster information.
func (f *Federation) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(f.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-f.stopCh:
			return
		case <-ticker.C:
			f.syncAllClusters(ctx)
		}
	}
}

// checkAllClusters checks the health of all clusters.
func (f *Federation) checkAllClusters(ctx context.Context) {
	clusters, err := f.store.ListByState(ctx, StateActive)
	if err != nil {
		return
	}

	for _, cluster := range clusters {
		if cluster.ID == f.localID {
			continue
		}

		health, err := f.CheckClusterHealth(ctx, cluster.ID)
		if err != nil || !health.Healthy {
			f.handleUnhealthyCluster(ctx, cluster, err)
		}
	}
}

func (f *Federation) handleUnhealthyCluster(ctx context.Context, cluster *Cluster, err error) {
	// Check if cluster has been unreachable for too long
	if time.Since(cluster.LastSeenAt) > f.config.HeartbeatTimeout {
		cluster.State = StateUnreachable
		cluster.UpdatedAt = time.Now()
		f.store.Save(ctx, cluster)

		f.emit(&FederationEvent{
			Type:      "cluster_unreachable",
			ClusterID: cluster.ID,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("Cluster %s is unreachable", cluster.Name),
			Error:     err.Error(),
		})
	}
}

// syncAllClusters syncs information from all clusters.
func (f *Federation) syncAllClusters(ctx context.Context) {
	clusters, err := f.store.List(ctx)
	if err != nil {
		return
	}

	for _, cluster := range clusters {
		if cluster.ID == f.localID || cluster.State != StateActive {
			continue
		}

		if err := f.SyncCluster(ctx, cluster.ID); err != nil {
			f.emit(&FederationEvent{
				Type:      "sync_failed",
				ClusterID: cluster.ID,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("Failed to sync cluster %s", cluster.Name),
				Error:     err.Error(),
			})
		}
	}
}

// JoinCluster adds a cluster to the federation.
func (f *Federation) JoinCluster(ctx context.Context, cluster *Cluster) error {
	cluster.State = StateJoining
	cluster.CreatedAt = time.Now()
	cluster.UpdatedAt = time.Now()

	if err := f.store.Save(ctx, cluster); err != nil {
		return err
	}

	f.emit(&FederationEvent{
		Type:      "cluster_joining",
		ClusterID: cluster.ID,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Cluster %s is joining the federation", cluster.Name),
	})

	// Verify connectivity
	_, err := f.CheckClusterHealth(ctx, cluster.ID)
	if err != nil {
		cluster.State = StateUnreachable
		f.store.Save(ctx, cluster)
		return fmt.Errorf("cluster unreachable: %w", err)
	}

	// Mark as active
	now := time.Now()
	cluster.State = StateActive
	cluster.JoinedAt = &now
	cluster.LastSeenAt = now
	cluster.UpdatedAt = now

	if err := f.store.Save(ctx, cluster); err != nil {
		return err
	}

	f.emit(&FederationEvent{
		Type:      "cluster_joined",
		ClusterID: cluster.ID,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Cluster %s has joined the federation", cluster.Name),
	})

	return nil
}

// LeaveCluster removes a cluster from the federation.
func (f *Federation) LeaveCluster(ctx context.Context, clusterID string) error {
	cluster, err := f.store.Get(ctx, clusterID)
	if err != nil {
		return err
	}

	cluster.State = StateLeaving
	cluster.UpdatedAt = time.Now()

	if err := f.store.Save(ctx, cluster); err != nil {
		return err
	}

	f.emit(&FederationEvent{
		Type:      "cluster_leaving",
		ClusterID: cluster.ID,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Cluster %s is leaving the federation", cluster.Name),
	})

	// Mark as left
	cluster.State = StateLeft
	cluster.UpdatedAt = time.Now()

	if err := f.store.Save(ctx, cluster); err != nil {
		return err
	}

	f.emit(&FederationEvent{
		Type:      "cluster_left",
		ClusterID: cluster.ID,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Cluster %s has left the federation", cluster.Name),
	})

	return nil
}

// CheckClusterHealth checks the health of a cluster.
func (f *Federation) CheckClusterHealth(ctx context.Context, clusterID string) (*ClusterHealth, error) {
	cluster, err := f.store.Get(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	// Make health check request
	healthURL := fmt.Sprintf("%s/health", cluster.Endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return &ClusterHealth{
			Healthy:       false,
			Ready:         false,
			LastCheckedAt: time.Now(),
			Conditions: []ClusterCondition{
				{
					Type:               "Reachable",
					Status:             "False",
					LastTransitionTime: time.Now(),
					Reason:             "ConnectionError",
					Message:            err.Error(),
				},
			},
		}, err
	}
	defer resp.Body.Close()

	health := &ClusterHealth{
		Healthy:       resp.StatusCode == http.StatusOK,
		Ready:         resp.StatusCode == http.StatusOK,
		LastCheckedAt: time.Now(),
		Conditions: []ClusterCondition{
			{
				Type:               "Reachable",
				Status:             "True",
				LastTransitionTime: time.Now(),
			},
		},
	}

	// Update last seen
	cluster.LastSeenAt = time.Now()
	cluster.UpdatedAt = time.Now()
	f.store.Save(ctx, cluster)

	return health, nil
}

// SyncCluster syncs information from a cluster.
func (f *Federation) SyncCluster(ctx context.Context, clusterID string) error {
	cluster, err := f.store.Get(ctx, clusterID)
	if err != nil {
		return err
	}

	// Make info request
	infoURL := fmt.Sprintf("%s/info", cluster.Endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", infoURL, nil)
	if err != nil {
		return err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var info struct {
		Version  string           `json:"version"`
		Capacity *ClusterCapacity `json:"capacity"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return err
	}

	// Update cluster info
	cluster.Version = info.Version
	cluster.Capacity = info.Capacity
	cluster.LastSeenAt = time.Now()
	cluster.UpdatedAt = time.Now()

	return f.store.Save(ctx, cluster)
}

// GetCluster retrieves a cluster by ID.
func (f *Federation) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	return f.store.Get(ctx, id)
}

// ListClusters lists all clusters.
func (f *Federation) ListClusters(ctx context.Context) ([]*Cluster, error) {
	return f.store.List(ctx)
}

// ListActiveClusters lists all active clusters.
func (f *Federation) ListActiveClusters(ctx context.Context) ([]*Cluster, error) {
	return f.store.ListByState(ctx, StateActive)
}

// ListClustersByRegion lists clusters in a region.
func (f *Federation) ListClustersByRegion(ctx context.Context, region string) ([]*Cluster, error) {
	return f.store.ListByRegion(ctx, region)
}

// UpdateCluster updates a cluster.
func (f *Federation) UpdateCluster(ctx context.Context, cluster *Cluster) error {
	cluster.UpdatedAt = time.Now()
	return f.store.Save(ctx, cluster)
}

// GetFederationStats returns statistics about the federation.
func (f *Federation) GetFederationStats(ctx context.Context) (*FederationStats, error) {
	clusters, err := f.store.List(ctx)
	if err != nil {
		return nil, err
	}

	stats := &FederationStats{
		TotalClusters: len(clusters),
		ByState:       make(map[ClusterState]int),
		ByRole:        make(map[ClusterRole]int),
		ByRegion:      make(map[string]int),
		ByProvider:    make(map[string]int),
		TotalCapacity: &ClusterCapacity{},
	}

	for _, c := range clusters {
		stats.ByState[c.State]++
		stats.ByRole[c.Role]++

		if c.Region != "" {
			stats.ByRegion[c.Region]++
		}
		if c.Provider != "" {
			stats.ByProvider[c.Provider]++
		}

		if c.Capacity != nil {
			stats.TotalCapacity.Nodes += c.Capacity.Nodes
			stats.TotalCapacity.CPUMillis += c.Capacity.CPUMillis
			stats.TotalCapacity.MemoryBytes += c.Capacity.MemoryBytes
			stats.TotalCapacity.StorageBytes += c.Capacity.StorageBytes
			stats.TotalCapacity.Pods += c.Capacity.Pods
			stats.TotalCapacity.GPUs += c.Capacity.GPUs
		}
	}

	stats.ActiveClusters = stats.ByState[StateActive]

	return stats, nil
}

// FederationStats contains statistics about the federation.
type FederationStats struct {
	TotalClusters  int                  `json:"totalClusters"`
	ActiveClusters int                  `json:"activeClusters"`
	ByState        map[ClusterState]int `json:"byState"`
	ByRole         map[ClusterRole]int  `json:"byRole"`
	ByRegion       map[string]int       `json:"byRegion"`
	ByProvider     map[string]int       `json:"byProvider"`
	TotalCapacity  *ClusterCapacity     `json:"totalCapacity"`
}

// InMemoryClusterStore is an in-memory implementation of ClusterStore.
type InMemoryClusterStore struct {
	clusters map[string]*Cluster
	mu       sync.RWMutex
}

// NewInMemoryClusterStore creates a new in-memory cluster store.
func NewInMemoryClusterStore() *InMemoryClusterStore {
	return &InMemoryClusterStore{
		clusters: make(map[string]*Cluster),
	}
}

// Get retrieves a cluster by ID.
func (s *InMemoryClusterStore) Get(_ context.Context, id string) (*Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.clusters[id]
	if !ok {
		return nil, fmt.Errorf("cluster not found: %s", id)
	}
	return copyCluster(c), nil
}

// List lists all clusters.
func (s *InMemoryClusterStore) List(_ context.Context) ([]*Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Cluster, 0, len(s.clusters))
	for _, c := range s.clusters {
		result = append(result, copyCluster(c))
	}
	return result, nil
}

// ListByState lists clusters by state.
func (s *InMemoryClusterStore) ListByState(_ context.Context, state ClusterState) ([]*Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Cluster
	for _, c := range s.clusters {
		if c.State == state {
			result = append(result, copyCluster(c))
		}
	}
	return result, nil
}

// ListByRegion lists clusters by region.
func (s *InMemoryClusterStore) ListByRegion(_ context.Context, region string) ([]*Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Cluster
	for _, c := range s.clusters {
		if c.Region == region {
			result = append(result, copyCluster(c))
		}
	}
	return result, nil
}

// Save saves a cluster.
func (s *InMemoryClusterStore) Save(_ context.Context, cluster *Cluster) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clusters[cluster.ID] = copyCluster(cluster)
	return nil
}

// Delete deletes a cluster.
func (s *InMemoryClusterStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.clusters, id)
	return nil
}

func copyCluster(c *Cluster) *Cluster {
	copy := &Cluster{
		ID:         c.ID,
		Name:       c.Name,
		Endpoint:   c.Endpoint,
		State:      c.State,
		Role:       c.Role,
		Region:     c.Region,
		Zone:       c.Zone,
		Provider:   c.Provider,
		Version:    c.Version,
		LastSeenAt: c.LastSeenAt,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}

	if c.JoinedAt != nil {
		joinedAt := *c.JoinedAt
		copy.JoinedAt = &joinedAt
	}

	if c.Capacity != nil {
		copy.Capacity = &ClusterCapacity{
			Nodes:        c.Capacity.Nodes,
			CPUMillis:    c.Capacity.CPUMillis,
			MemoryBytes:  c.Capacity.MemoryBytes,
			StorageBytes: c.Capacity.StorageBytes,
			Pods:         c.Capacity.Pods,
			GPUs:         c.Capacity.GPUs,
		}
	}

	if c.Labels != nil {
		copy.Labels = make(map[string]string)
		for k, v := range c.Labels {
			copy.Labels[k] = v
		}
	}

	if c.Annotations != nil {
		copy.Annotations = make(map[string]string)
		for k, v := range c.Annotations {
			copy.Annotations[k] = v
		}
	}

	return copy
}

// PlacementPolicy defines how workloads are placed across clusters.
type PlacementPolicy struct {
	Name        string                `json:"name"`
	Type        PlacementType         `json:"type"`
	Clusters    []string              `json:"clusters,omitempty"`
	Regions     []string              `json:"regions,omitempty"`
	Providers   []string              `json:"providers,omitempty"`
	Selector    map[string]string     `json:"selector,omitempty"`
	Weights     map[string]int        `json:"weights,omitempty"`
	Constraints []PlacementConstraint `json:"constraints,omitempty"`
}

// PlacementType represents the type of placement policy.
type PlacementType string

const (
	// PlacementAll places on all matching clusters.
	PlacementAll PlacementType = "all"
	// PlacementWeighted places based on weights.
	PlacementWeighted PlacementType = "weighted"
	// PlacementSpread places evenly across clusters.
	PlacementSpread PlacementType = "spread"
	// PlacementBinpack places to minimize cluster count.
	PlacementBinpack PlacementType = "binpack"
)

// PlacementConstraint defines a placement constraint.
type PlacementConstraint struct {
	Type     string   `json:"type"` // required, preferred, forbidden
	Key      string   `json:"key"`
	Operator string   `json:"operator"` // in, notIn, exists, doesNotExist
	Values   []string `json:"values,omitempty"`
}

// Scheduler schedules workloads across federated clusters.
type Scheduler struct {
	federation *Federation
	policies   map[string]*PlacementPolicy
	mu         sync.RWMutex
}

// NewScheduler creates a new scheduler.
func NewScheduler(federation *Federation) *Scheduler {
	return &Scheduler{
		federation: federation,
		policies:   make(map[string]*PlacementPolicy),
	}
}

// RegisterPolicy registers a placement policy.
func (s *Scheduler) RegisterPolicy(policy *PlacementPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[policy.Name] = policy
}

// GetPolicy retrieves a policy by name.
func (s *Scheduler) GetPolicy(name string) (*PlacementPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policies[name]
	return p, ok
}

// Schedule schedules a workload and returns target clusters.
func (s *Scheduler) Schedule(ctx context.Context, policyName string) ([]*Cluster, error) {
	policy, ok := s.GetPolicy(policyName)
	if !ok {
		return nil, fmt.Errorf("policy not found: %s", policyName)
	}

	// Get all active clusters
	clusters, err := s.federation.ListActiveClusters(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by policy criteria
	filtered := s.filterClusters(clusters, policy)

	// Apply placement type
	switch policy.Type {
	case PlacementAll:
		return filtered, nil
	case PlacementSpread:
		return s.spreadPlacement(filtered, policy)
	case PlacementWeighted:
		return s.weightedPlacement(filtered, policy)
	case PlacementBinpack:
		return s.binpackPlacement(filtered, policy)
	default:
		return filtered, nil
	}
}

func (s *Scheduler) filterClusters(clusters []*Cluster, policy *PlacementPolicy) []*Cluster {
	var result []*Cluster

	for _, c := range clusters {
		// Check if cluster is in allowed list
		if len(policy.Clusters) > 0 {
			found := false
			for _, id := range policy.Clusters {
				if c.ID == id {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Check if region is in allowed list
		if len(policy.Regions) > 0 {
			found := false
			for _, r := range policy.Regions {
				if c.Region == r {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Check if provider is in allowed list
		if len(policy.Providers) > 0 {
			found := false
			for _, p := range policy.Providers {
				if c.Provider == p {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Check label selector
		if len(policy.Selector) > 0 {
			matches := true
			for k, v := range policy.Selector {
				if c.Labels[k] != v {
					matches = false
					break
				}
			}
			if !matches {
				continue
			}
		}

		result = append(result, c)
	}

	return result
}

func (s *Scheduler) spreadPlacement(clusters []*Cluster, policy *PlacementPolicy) ([]*Cluster, error) {
	// Spread evenly - return all matching clusters
	return clusters, nil
}

func (s *Scheduler) weightedPlacement(clusters []*Cluster, policy *PlacementPolicy) ([]*Cluster, error) {
	// Sort by weight
	var result []*Cluster
	for _, c := range clusters {
		if weight, ok := policy.Weights[c.ID]; ok && weight > 0 {
			result = append(result, c)
		}
	}
	return result, nil
}

func (s *Scheduler) binpackPlacement(clusters []*Cluster, policy *PlacementPolicy) ([]*Cluster, error) {
	// Return first available cluster with capacity
	if len(clusters) > 0 {
		return []*Cluster{clusters[0]}, nil
	}
	return nil, nil
}
