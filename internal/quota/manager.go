// Package quota provides resource quota management for Keystone.
package quota

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ResourceType represents a type of resource.
type ResourceType string

const (
	// ResourceCPU represents CPU resources.
	ResourceCPU ResourceType = "cpu"
	// ResourceMemory represents memory resources.
	ResourceMemory ResourceType = "memory"
	// ResourceStorage represents storage resources.
	ResourceStorage ResourceType = "storage"
	// ResourcePods represents the number of pods.
	ResourcePods ResourceType = "pods"
	// ResourceSecrets represents the number of secrets.
	ResourceSecrets ResourceType = "secrets"
	// ResourceConfigMaps represents the number of configmaps.
	ResourceConfigMaps ResourceType = "configmaps"
	// ResourceServices represents the number of services.
	ResourceServices ResourceType = "services"
	// ResourcePVCs represents the number of persistent volume claims.
	ResourcePVCs ResourceType = "pvcs"
	// ResourceGPU represents GPU resources.
	ResourceGPU ResourceType = "gpu"
	// ResourceLoadBalancers represents load balancer resources.
	ResourceLoadBalancers ResourceType = "loadbalancers"
	// ResourceNodePorts represents node port resources.
	ResourceNodePorts ResourceType = "nodeports"
	// ResourceCustom represents custom resource types.
	ResourceCustom ResourceType = "custom"
)

// Quantity represents a resource quantity.
type Quantity struct {
	Value int64  `json:"value"`
	Unit  string `json:"unit,omitempty"` // m, Ki, Mi, Gi, etc.
}

// String returns a string representation of the quantity.
func (q Quantity) String() string {
	if q.Unit == "" {
		return fmt.Sprintf("%d", q.Value)
	}
	return fmt.Sprintf("%d%s", q.Value, q.Unit)
}

// Quota defines resource limits for a scope.
type Quota struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Scope       *Scope               `json:"scope"`
	Hard        map[ResourceType]Quantity `json:"hard"`
	Used        map[ResourceType]Quantity `json:"used,omitempty"`
	Labels      map[string]string         `json:"labels,omitempty"`
	Annotations map[string]string         `json:"annotations,omitempty"`
	CreatedAt   time.Time                 `json:"createdAt"`
	UpdatedAt   time.Time                 `json:"updatedAt"`
}

// Scope defines the scope of a quota.
type Scope struct {
	Type      string `json:"type"` // namespace, cluster, team, project
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Cluster   string `json:"cluster,omitempty"`
}

// Available returns the available resources.
func (q *Quota) Available() map[ResourceType]Quantity {
	available := make(map[ResourceType]Quantity)
	for rt, hard := range q.Hard {
		used, ok := q.Used[rt]
		if !ok {
			available[rt] = hard
		} else {
			available[rt] = Quantity{
				Value: hard.Value - used.Value,
				Unit:  hard.Unit,
			}
		}
	}
	return available
}

// UsagePercentage returns the usage percentage for each resource.
func (q *Quota) UsagePercentage() map[ResourceType]float64 {
	percentages := make(map[ResourceType]float64)
	for rt, hard := range q.Hard {
		if hard.Value == 0 {
			percentages[rt] = 0
			continue
		}
		used, ok := q.Used[rt]
		if !ok {
			percentages[rt] = 0
		} else {
			percentages[rt] = float64(used.Value) / float64(hard.Value) * 100
		}
	}
	return percentages
}

// IsExceeded checks if the quota is exceeded for any resource.
func (q *Quota) IsExceeded() bool {
	for rt, hard := range q.Hard {
		used, ok := q.Used[rt]
		if ok && used.Value > hard.Value {
			return true
		}
	}
	return false
}

// ExceededResources returns resources that are exceeded.
func (q *Quota) ExceededResources() []ResourceType {
	var exceeded []ResourceType
	for rt, hard := range q.Hard {
		used, ok := q.Used[rt]
		if ok && used.Value > hard.Value {
			exceeded = append(exceeded, rt)
		}
	}
	return exceeded
}

// Request represents a resource request.
type Request struct {
	Resources map[ResourceType]Quantity `json:"resources"`
	Requester string                    `json:"requester,omitempty"`
	Reason    string                    `json:"reason,omitempty"`
}

// AllocationResult represents the result of an allocation request.
type AllocationResult struct {
	Allowed      bool                      `json:"allowed"`
	Reason       string                    `json:"reason,omitempty"`
	Requested    map[ResourceType]Quantity `json:"requested"`
	Available    map[ResourceType]Quantity `json:"available,omitempty"`
	Insufficient []ResourceType            `json:"insufficient,omitempty"`
}

// Store is the interface for storing quotas.
type Store interface {
	Get(ctx context.Context, id string) (*Quota, error)
	GetByScope(ctx context.Context, scope *Scope) (*Quota, error)
	List(ctx context.Context) ([]*Quota, error)
	ListByType(ctx context.Context, scopeType string) ([]*Quota, error)
	Save(ctx context.Context, quota *Quota) error
	Delete(ctx context.Context, id string) error
}

// Manager manages resource quotas.
type Manager struct {
	store     Store
	defaults  map[string]*Quota // Default quotas by scope type
	listeners []Listener
	mu        sync.RWMutex
}

// Listener is called when quota events occur.
type Listener func(event *Event)

// Event represents a quota event.
type Event struct {
	Type      string                 `json:"type"` // created, updated, exceeded, warning
	QuotaID   string                 `json:"quotaId"`
	Scope     *Scope            `json:"scope"`
	Timestamp time.Time              `json:"timestamp"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// NewManager creates a new quota manager.
func NewManager(store Store) *Manager {
	return &Manager{
		store:    store,
		defaults: make(map[string]*Quota),
	}
}

// AddListener adds a quota event listener.
func (m *Manager) AddListener(listener Listener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, listener)
}

// emit sends an event to all listeners.
func (m *Manager) emit(event *Event) {
	m.mu.RLock()
	listeners := make([]Listener, len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// SetDefault sets a default quota for a scope type.
func (m *Manager) SetDefault(scopeType string, quota *Quota) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaults[scopeType] = quota
}

// GetDefault gets the default quota for a scope type.
func (m *Manager) GetDefault(scopeType string) (*Quota, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	q, ok := m.defaults[scopeType]
	return q, ok
}

// Create creates a new quota.
func (m *Manager) Create(ctx context.Context, quota *Quota) error {
	quota.CreatedAt = time.Now()
	quota.UpdatedAt = time.Now()
	if quota.Used == nil {
		quota.Used = make(map[ResourceType]Quantity)
	}

	if err := m.store.Save(ctx, quota); err != nil {
		return err
	}

	m.emit(&Event{
		Type:      "created",
		QuotaID:   quota.ID,
		Scope:     quota.Scope,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Quota %s created", quota.Name),
	})

	return nil
}

// Update updates an existing quota.
func (m *Manager) Update(ctx context.Context, quota *Quota) error {
	quota.UpdatedAt = time.Now()

	if err := m.store.Save(ctx, quota); err != nil {
		return err
	}

	m.emit(&Event{
		Type:      "updated",
		QuotaID:   quota.ID,
		Scope:     quota.Scope,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Quota %s updated", quota.Name),
	})

	return nil
}

// Delete deletes a quota.
func (m *Manager) Delete(ctx context.Context, id string) error {
	quota, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}

	if err := m.store.Delete(ctx, id); err != nil {
		return err
	}

	m.emit(&Event{
		Type:      "deleted",
		QuotaID:   id,
		Scope:     quota.Scope,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Quota %s deleted", quota.Name),
	})

	return nil
}

// Get retrieves a quota by ID.
func (m *Manager) Get(ctx context.Context, id string) (*Quota, error) {
	return m.store.Get(ctx, id)
}

// GetByScope retrieves a quota by scope.
func (m *Manager) GetByScope(ctx context.Context, scope *Scope) (*Quota, error) {
	return m.store.GetByScope(ctx, scope)
}

// List lists all quotas.
func (m *Manager) List(ctx context.Context) ([]*Quota, error) {
	return m.store.List(ctx)
}

// CheckAllocation checks if resources can be allocated.
func (m *Manager) CheckAllocation(ctx context.Context, scope *Scope, request *Request) (*AllocationResult, error) {
	quota, err := m.store.GetByScope(ctx, scope)
	if err != nil {
		// If no quota exists, check for default
		defaultQuota, ok := m.GetDefault(scope.Type)
		if !ok {
			return &AllocationResult{Allowed: true, Reason: "no quota defined"}, nil
		}
		quota = defaultQuota
	}

	result := &AllocationResult{
		Allowed:   true,
		Requested: request.Resources,
		Available: quota.Available(),
	}

	for rt, requested := range request.Resources {
		hard, ok := quota.Hard[rt]
		if !ok {
			continue
		}

		used := quota.Used[rt]
		available := hard.Value - used.Value

		if requested.Value > available {
			result.Allowed = false
			result.Insufficient = append(result.Insufficient, rt)
		}
	}

	if !result.Allowed {
		result.Reason = fmt.Sprintf("insufficient resources: %v", result.Insufficient)
	}

	return result, nil
}

// Allocate allocates resources.
func (m *Manager) Allocate(ctx context.Context, scope *Scope, request *Request) (*AllocationResult, error) {
	// First check if allocation is possible
	result, err := m.CheckAllocation(ctx, scope, request)
	if err != nil {
		return nil, err
	}

	if !result.Allowed {
		return result, nil
	}

	// Get the quota
	quota, err := m.store.GetByScope(ctx, scope)
	if err != nil {
		// No quota, nothing to track
		return result, nil //nolint:nilerr // no quota is not an error
	}

	// Update usage
	if quota.Used == nil {
		quota.Used = make(map[ResourceType]Quantity)
	}

	for rt, requested := range request.Resources {
		current := quota.Used[rt]
		quota.Used[rt] = Quantity{
			Value: current.Value + requested.Value,
			Unit:  requested.Unit,
		}
	}

	quota.UpdatedAt = time.Now()
	if err := m.store.Save(ctx, quota); err != nil {
		return nil, err
	}

	// Check for warnings
	percentages := quota.UsagePercentage()
	for rt, pct := range percentages {
		if pct >= 90 {
			m.emit(&Event{
				Type:      "warning",
				QuotaID:   quota.ID,
				Scope:     quota.Scope,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("Resource %s is at %.1f%% usage", rt, pct),
				Details: map[string]interface{}{
					"resource":   rt,
					"percentage": pct,
				},
			})
		}
	}

	return result, nil
}

// Release releases resources.
func (m *Manager) Release(ctx context.Context, scope *Scope, request *Request) error {
	quota, err := m.store.GetByScope(ctx, scope)
	if err != nil {
		return nil //nolint:nilerr // no quota is not an error
	}

	if quota.Used == nil {
		return nil
	}

	for rt, released := range request.Resources {
		current := quota.Used[rt]
		newValue := current.Value - released.Value
		if newValue < 0 {
			newValue = 0
		}
		quota.Used[rt] = Quantity{
			Value: newValue,
			Unit:  released.Unit,
		}
	}

	quota.UpdatedAt = time.Now()
	return m.store.Save(ctx, quota)
}

// GetUsageReport generates a usage report for a quota.
func (m *Manager) GetUsageReport(ctx context.Context, id string) (*UsageReport, error) {
	quota, err := m.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	report := &UsageReport{
		QuotaID:     quota.ID,
		QuotaName:   quota.Name,
		Scope:       quota.Scope,
		GeneratedAt: time.Now(),
		Resources:   make([]ResourceUsage, 0),
	}

	for rt, hard := range quota.Hard {
		used := quota.Used[rt]
		available := hard.Value - used.Value
		if available < 0 {
			available = 0
		}

		percentage := float64(0)
		if hard.Value > 0 {
			percentage = float64(used.Value) / float64(hard.Value) * 100
		}

		report.Resources = append(report.Resources, ResourceUsage{
			Type:       rt,
			Hard:       hard,
			Used:       used,
			Available:  Quantity{Value: available, Unit: hard.Unit},
			Percentage: percentage,
		})
	}

	return report, nil
}

// UsageReport contains quota usage information.
type UsageReport struct {
	QuotaID     string          `json:"quotaId"`
	QuotaName   string          `json:"quotaName"`
	Scope       *Scope     `json:"scope"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Resources   []ResourceUsage `json:"resources"`
}

// ResourceUsage contains usage information for a single resource.
type ResourceUsage struct {
	Type       ResourceType `json:"type"`
	Hard       Quantity     `json:"hard"`
	Used       Quantity     `json:"used"`
	Available  Quantity     `json:"available"`
	Percentage float64      `json:"percentage"`
}

// InMemoryStore is an in-memory implementation of QuotaStore.
type InMemoryStore struct {
	quotas map[string]*Quota
	mu     sync.RWMutex
}

// NewInMemoryStore creates a new in-memory quota store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		quotas: make(map[string]*Quota),
	}
}

// Get retrieves a quota by ID.
func (s *InMemoryStore) Get(_ context.Context, id string) (*Quota, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q, ok := s.quotas[id]
	if !ok {
		return nil, fmt.Errorf("quota not found: %s", id)
	}
	return copyQuota(q), nil
}

// GetByScope retrieves a quota by scope.
func (s *InMemoryStore) GetByScope(_ context.Context, scope *Scope) (*Quota, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, q := range s.quotas {
		if q.Scope != nil && q.Scope.Type == scope.Type && q.Scope.Name == scope.Name {
			if scope.Namespace != "" && q.Scope.Namespace != scope.Namespace {
				continue
			}
			if scope.Cluster != "" && q.Scope.Cluster != scope.Cluster {
				continue
			}
			return copyQuota(q), nil
		}
	}
	return nil, fmt.Errorf("quota not found for scope: %s/%s", scope.Type, scope.Name)
}

// List lists all quotas.
func (s *InMemoryStore) List(_ context.Context) ([]*Quota, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Quota, 0, len(s.quotas))
	for _, q := range s.quotas {
		result = append(result, copyQuota(q))
	}
	return result, nil
}

// ListByType lists quotas by scope type.
func (s *InMemoryStore) ListByType(_ context.Context, scopeType string) ([]*Quota, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Quota
	for _, q := range s.quotas {
		if q.Scope != nil && q.Scope.Type == scopeType {
			result = append(result, copyQuota(q))
		}
	}
	return result, nil
}

// Save saves a quota.
func (s *InMemoryStore) Save(_ context.Context, quota *Quota) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.quotas[quota.ID] = copyQuota(quota)
	return nil
}

// Delete deletes a quota.
func (s *InMemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.quotas, id)
	return nil
}

func copyQuota(q *Quota) *Quota {
	copied := &Quota{
		ID:        q.ID,
		Name:      q.Name,
		CreatedAt: q.CreatedAt,
		UpdatedAt: q.UpdatedAt,
	}

	if q.Scope != nil {
		copied.Scope = &Scope{
			Type:      q.Scope.Type,
			Name:      q.Scope.Name,
			Namespace: q.Scope.Namespace,
			Cluster:   q.Scope.Cluster,
		}
	}

	if q.Hard != nil {
		copied.Hard = make(map[ResourceType]Quantity)
		for k, v := range q.Hard {
			copied.Hard[k] = v
		}
	}

	if q.Used != nil {
		copied.Used = make(map[ResourceType]Quantity)
		for k, v := range q.Used {
			copied.Used[k] = v
		}
	}

	if q.Labels != nil {
		copied.Labels = make(map[string]string)
		for k, v := range q.Labels {
			copied.Labels[k] = v
		}
	}

	if q.Annotations != nil {
		copied.Annotations = make(map[string]string)
		for k, v := range q.Annotations {
			copied.Annotations[k] = v
		}
	}

	return copied
}

// Enforcer enforces quotas before resource creation.
type Enforcer struct {
	manager *Manager
	enabled bool
	mu      sync.RWMutex
}

// NewEnforcer creates a new quota enforcer.
func NewEnforcer(manager *Manager) *Enforcer {
	return &Enforcer{
		manager: manager,
		enabled: true,
	}
}

// Enable enables quota enforcement.
func (e *Enforcer) Enable() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = true
}

// Disable disables quota enforcement.
func (e *Enforcer) Disable() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = false
}

// IsEnabled returns whether enforcement is enabled.
func (e *Enforcer) IsEnabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enabled
}

// Enforce checks if resources can be allocated and returns an error if not.
func (e *Enforcer) Enforce(ctx context.Context, scope *Scope, request *Request) error {
	if !e.IsEnabled() {
		return nil
	}

	result, err := e.manager.CheckAllocation(ctx, scope, request)
	if err != nil {
		return err
	}

	if !result.Allowed {
		return &ExceededError{
			Scope:        scope,
			Insufficient: result.Insufficient,
			Message:      result.Reason,
		}
	}

	return nil
}

// ExceededError is returned when quota is exceeded.
type ExceededError struct {
	Scope        *Scope
	Insufficient []ResourceType
	Message      string
}

func (e *ExceededError) Error() string {
	return e.Message
}

// Aggregator aggregates quotas from multiple scopes.
type Aggregator struct {
	manager *Manager
}

// NewAggregator creates a new quota aggregator.
func NewAggregator(manager *Manager) *Aggregator {
	return &Aggregator{manager: manager}
}

// AggregatedQuota represents aggregated quota information.
type AggregatedQuota struct {
	ScopeType   string                    `json:"scopeType"`
	QuotaCount  int                       `json:"quotaCount"`
	TotalHard   map[ResourceType]Quantity `json:"totalHard"`
	TotalUsed   map[ResourceType]Quantity `json:"totalUsed"`
	Utilization map[ResourceType]float64  `json:"utilization"`
}

// Aggregate aggregates quotas by scope type.
func (a *Aggregator) Aggregate(ctx context.Context, scopeType string) (*AggregatedQuota, error) {
	quotas, err := a.manager.store.ListByType(ctx, scopeType)
	if err != nil {
		return nil, err
	}

	agg := &AggregatedQuota{
		ScopeType:   scopeType,
		QuotaCount:  len(quotas),
		TotalHard:   make(map[ResourceType]Quantity),
		TotalUsed:   make(map[ResourceType]Quantity),
		Utilization: make(map[ResourceType]float64),
	}

	for _, q := range quotas {
		for rt, hard := range q.Hard {
			current := agg.TotalHard[rt]
			agg.TotalHard[rt] = Quantity{
				Value: current.Value + hard.Value,
				Unit:  hard.Unit,
			}
		}
		for rt, used := range q.Used {
			current := agg.TotalUsed[rt]
			agg.TotalUsed[rt] = Quantity{
				Value: current.Value + used.Value,
				Unit:  used.Unit,
			}
		}
	}

	// Calculate utilization
	for rt, hard := range agg.TotalHard {
		if hard.Value == 0 {
			agg.Utilization[rt] = 0
			continue
		}
		used := agg.TotalUsed[rt]
		agg.Utilization[rt] = float64(used.Value) / float64(hard.Value) * 100
	}

	return agg, nil
}
