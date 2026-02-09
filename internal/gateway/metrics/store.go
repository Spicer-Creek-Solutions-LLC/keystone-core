// Package metrics provides the metrics gateway for aggregating agent metrics.
package metrics

import (
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
)

// StoreConfig holds configuration for the metrics store.
type StoreConfig struct {
	// MaxAge is the maximum age for metrics before they're considered stale
	MaxAge time.Duration

	// MaxSeries limits total metric series stored
	MaxSeries int

	// MaxLabelsPerSeries limits labels per series
	MaxLabelsPerSeries int

	// DropHighCardinality automatically drops high cardinality metrics
	DropHighCardinality bool

	// HighCardinalityThreshold defines when a metric is considered high cardinality
	HighCardinalityThreshold int
}

// DefaultStoreConfig returns a configuration with sensible defaults.
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		MaxAge:                   60 * time.Second,
		MaxSeries:                100000,
		MaxLabelsPerSeries:       20,
		DropHighCardinality:      false,
		HighCardinalityThreshold: 10000,
	}
}

// AgentMetrics holds metrics for a single agent.
type AgentMetrics struct {
	// AgentID is the unique identifier for the agent
	AgentID string

	// Labels are additional labels for this agent
	Labels map[string]string

	// Families contains the metric families received from this agent
	Families map[string]*dto.MetricFamily

	// LastSeen is when metrics were last received from this agent
	LastSeen time.Time

	// SeriesCount tracks the number of series for this agent
	SeriesCount int
}

// Store stores metrics from multiple agents.
type Store struct {
	agents map[string]*AgentMetrics
	mu     sync.RWMutex
	config StoreConfig

	// Callbacks
	onAgentAdded   func(agentID string)
	onAgentRemoved func(agentID string)
	onMetricsAdded func(agentID string, count int)

	// Stats
	totalSeries   int
	droppedSeries int
}

// NewMetricsStore creates a new metrics store.
func NewMetricsStore(config StoreConfig) *Store {
	return &Store{
		agents: make(map[string]*AgentMetrics),
		config: config,
	}
}

// SetOnAgentAdded sets the callback for when an agent is added.
func (s *Store) SetOnAgentAdded(fn func(agentID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onAgentAdded = fn
}

// SetOnAgentRemoved sets the callback for when an agent is removed.
func (s *Store) SetOnAgentRemoved(fn func(agentID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onAgentRemoved = fn
}

// SetOnMetricsAdded sets the callback for when metrics are added.
func (s *Store) SetOnMetricsAdded(fn func(agentID string, count int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onMetricsAdded = fn
}

// Store stores metrics for an agent.
func (s *Store) Store(agentID string, labels map[string]string, families []*dto.MetricFamily) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if this is a new agent
	isNew := false
	agent, exists := s.agents[agentID]
	if !exists {
		isNew = true
		agent = &AgentMetrics{
			AgentID:  agentID,
			Labels:   labels,
			Families: make(map[string]*dto.MetricFamily),
		}
		s.agents[agentID] = agent
	}

	// Update agent labels
	agent.Labels = labels
	agent.LastSeen = time.Now()

	// Store metrics
	seriesAdded := 0
	for _, family := range families {
		if family == nil || family.Name == nil {
			continue
		}

		name := *family.Name

		// Check cardinality limits
		seriesCount := len(family.Metric)
		if s.config.DropHighCardinality && seriesCount > s.config.HighCardinalityThreshold {
			s.droppedSeries += seriesCount
			continue
		}

		// Check label count per series
		if s.config.MaxLabelsPerSeries > 0 {
			for _, m := range family.Metric {
				if len(m.Label) > s.config.MaxLabelsPerSeries {
					// Truncate labels
					m.Label = m.Label[:s.config.MaxLabelsPerSeries]
				}
			}
		}

		// Check total series limit
		if s.config.MaxSeries > 0 && s.totalSeries+seriesCount > s.config.MaxSeries {
			s.droppedSeries += seriesCount
			continue
		}

		// Update series count
		if existing, ok := agent.Families[name]; ok {
			s.totalSeries -= len(existing.Metric)
		}
		s.totalSeries += seriesCount
		seriesAdded += seriesCount

		agent.Families[name] = family
	}

	agent.SeriesCount = 0
	for _, f := range agent.Families {
		agent.SeriesCount += len(f.Metric)
	}

	// Notify callbacks
	if isNew && s.onAgentAdded != nil {
		go s.onAgentAdded(agentID)
	}
	if seriesAdded > 0 && s.onMetricsAdded != nil {
		go s.onMetricsAdded(agentID, seriesAdded)
	}

	return nil
}

// Get returns metrics for an agent.
func (s *Store) Get(agentID string) (*AgentMetrics, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agent, exists := s.agents[agentID]
	if !exists {
		return nil, false
	}
	return agent, true
}

// GetAll returns all agent metrics.
func (s *Store) GetAll() map[string]*AgentMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*AgentMetrics, len(s.agents))
	for k, v := range s.agents {
		result[k] = v
	}
	return result
}

// GetAllFamilies returns all metric families across all agents.
// It merges metrics from different agents, adding agent labels.
func (s *Store) GetAllFamilies() []*dto.MetricFamily {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all unique family names
	familyNames := make(map[string]bool)
	for _, agent := range s.agents {
		for name := range agent.Families {
			familyNames[name] = true
		}
	}

	// Merge metrics from all agents for each family
	result := make([]*dto.MetricFamily, 0, len(familyNames))
	for name := range familyNames {
		merged := s.mergeFamilyAcrossAgents(name)
		if merged != nil {
			result = append(result, merged)
		}
	}

	return result
}

// mergeFamilyAcrossAgents merges a metric family across all agents.
func (s *Store) mergeFamilyAcrossAgents(name string) *dto.MetricFamily {
	var merged *dto.MetricFamily

	for _, agent := range s.agents {
		family, exists := agent.Families[name]
		if !exists {
			continue
		}

		if merged == nil {
			// Clone the first family
			merged = &dto.MetricFamily{
				Name:   family.Name,
				Help:   family.Help,
				Type:   family.Type,
				Metric: make([]*dto.Metric, 0, len(family.Metric)),
			}
		}

		// Add metrics with agent labels
		for _, m := range family.Metric {
			cloned := s.cloneMetricWithAgentLabels(m, agent)
			merged.Metric = append(merged.Metric, cloned)
		}
	}

	return merged
}

// cloneMetricWithAgentLabels clones a metric and adds agent labels.
func (s *Store) cloneMetricWithAgentLabels(m *dto.Metric, agent *AgentMetrics) *dto.Metric {
	cloned := &dto.Metric{
		Label:       make([]*dto.LabelPair, 0, len(m.Label)+len(agent.Labels)+1),
		Gauge:       m.Gauge,
		Counter:     m.Counter,
		Summary:     m.Summary,
		Untyped:     m.Untyped,
		Histogram:   m.Histogram,
		TimestampMs: m.TimestampMs,
	}

	// Copy existing labels
	cloned.Label = append(cloned.Label, m.Label...)

	// Add agent_id label
	agentIDKey := "agent_id"
	agentIDVal := agent.AgentID
	cloned.Label = append(cloned.Label, &dto.LabelPair{
		Name:  &agentIDKey,
		Value: &agentIDVal,
	})

	// Add agent custom labels
	for k, v := range agent.Labels {
		key := k
		val := v
		cloned.Label = append(cloned.Label, &dto.LabelPair{
			Name:  &key,
			Value: &val,
		})
	}

	return cloned
}

// Remove removes an agent from the store.
func (s *Store) Remove(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, exists := s.agents[agentID]
	if !exists {
		return
	}

	// Update series count
	for _, f := range agent.Families {
		s.totalSeries -= len(f.Metric)
	}

	delete(s.agents, agentID)

	if s.onAgentRemoved != nil {
		go s.onAgentRemoved(agentID)
	}
}

// RemoveStale removes agents that haven't been seen within maxAge.
func (s *Store) RemoveStale() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var removed []string

	for agentID, agent := range s.agents {
		if now.Sub(agent.LastSeen) > s.config.MaxAge {
			// Update series count
			for _, f := range agent.Families {
				s.totalSeries -= len(f.Metric)
			}

			delete(s.agents, agentID)
			removed = append(removed, agentID)

			if s.onAgentRemoved != nil {
				go s.onAgentRemoved(agentID)
			}
		}
	}

	return removed
}

// AgentCount returns the number of agents in the store.
func (s *Store) AgentCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.agents)
}

// SeriesCount returns the total number of metric series.
func (s *Store) SeriesCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalSeries
}

// DroppedSeriesCount returns the number of dropped series.
func (s *Store) DroppedSeriesCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.droppedSeries
}

// StoreStats holds store statistics.
type StoreStats struct {
	AgentCount    int
	TotalSeries   int
	DroppedSeries int
}

// Stats returns current store statistics.
func (s *Store) Stats() StoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StoreStats{
		AgentCount:    len(s.agents),
		TotalSeries:   s.totalSeries,
		DroppedSeries: s.droppedSeries,
	}
}
