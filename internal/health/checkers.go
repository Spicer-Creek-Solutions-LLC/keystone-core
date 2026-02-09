package health

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSChecker checks the health of a NATS connection
type NATSChecker struct {
	name string
	nc   *nats.Conn
}

// NewNATSChecker creates a new NATS health checker
func NewNATSChecker(nc *nats.Conn) *NATSChecker {
	return &NATSChecker{
		name: "nats",
		nc:   nc,
	}
}

// Name returns the name of the checker
func (c *NATSChecker) Name() string {
	return c.name
}

// Check performs the health check
func (c *NATSChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if c.nc == nil {
		result.Status = StatusUnhealthy
		result.Message = "NATS connection is nil"
		result.Duration = time.Since(start)
		return result
	}

	// Check connection status
	if !c.nc.IsConnected() {
		result.Status = StatusUnhealthy
		result.Message = "NATS not connected"
		result.Duration = time.Since(start)
		return result
	}

	// Get connection stats
	stats := c.nc.Stats()
	result.Details["in_msgs"] = stats.InMsgs
	result.Details["out_msgs"] = stats.OutMsgs
	result.Details["in_bytes"] = stats.InBytes
	result.Details["out_bytes"] = stats.OutBytes
	result.Details["reconnects"] = stats.Reconnects

	// Check if there are too many reconnects (indicates instability)
	if stats.Reconnects > 10 {
		result.Status = StatusDegraded
		result.Message = fmt.Sprintf("High reconnect count: %d", stats.Reconnects)
	} else {
		result.Status = StatusHealthy
		result.Message = "NATS connection healthy"
	}

	// Add server information
	if c.nc.ConnectedUrl() != "" {
		result.Details["connected_url"] = c.nc.ConnectedUrl()
	}

	result.Duration = time.Since(start)
	return result
}

// DatabaseChecker checks the health of a database connection
type DatabaseChecker struct {
	name string
	db   *sql.DB
}

// NewDatabaseChecker creates a new database health checker
func NewDatabaseChecker(db *sql.DB, name string) *DatabaseChecker {
	if name == "" {
		name = "state_backend"
	}
	return &DatabaseChecker{
		name: name,
		db:   db,
	}
}

// Name returns the name of the checker
func (c *DatabaseChecker) Name() string {
	return c.name
}

// Check performs the health check
func (c *DatabaseChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if c.db == nil {
		result.Status = StatusUnhealthy
		result.Message = "Database connection is nil"
		result.Duration = time.Since(start)
		return result
	}

	// Ping the database with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	pingStart := time.Now()
	err := c.db.PingContext(pingCtx)
	pingDuration := time.Since(pingStart)

	result.Details["latency_ms"] = pingDuration.Milliseconds()

	if err != nil {
		result.Status = StatusUnhealthy
		result.Message = fmt.Sprintf("Database ping failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// Check connection pool stats
	stats := c.db.Stats()
	result.Details["open_connections"] = stats.OpenConnections
	result.Details["in_use"] = stats.InUse
	result.Details["idle"] = stats.Idle
	result.Details["wait_count"] = stats.WaitCount
	result.Details["wait_duration_ms"] = stats.WaitDuration.Milliseconds()

	// Determine status based on latency and wait count
	switch {
	case pingDuration > 100*time.Millisecond:
		result.Status = StatusDegraded
		result.Message = fmt.Sprintf("High database latency: %v", pingDuration)
	case stats.WaitCount > 1000:
		result.Status = StatusDegraded
		result.Message = fmt.Sprintf("High connection wait count: %d", stats.WaitCount)
	default:
		result.Status = StatusHealthy
		result.Message = "Database connection healthy"
	}

	result.Duration = time.Since(start)
	return result
}

// AgentPoolChecker checks the health of the agent pool
type AgentPoolChecker struct {
	name                string
	getConnectedAgents  func() int
	getTotalAgents      func() int
	minHealthyThreshold float64
}

// NewAgentPoolChecker creates a new agent pool health checker
func NewAgentPoolChecker(getConnected, getTotal func() int, minThreshold float64) *AgentPoolChecker {
	if minThreshold == 0 {
		minThreshold = 0.8 // 80% default
	}
	return &AgentPoolChecker{
		name:                "agents",
		getConnectedAgents:  getConnected,
		getTotalAgents:      getTotal,
		minHealthyThreshold: minThreshold,
	}
}

// Name returns the name of the checker
func (c *AgentPoolChecker) Name() string {
	return c.name
}

// Check performs the health check
func (c *AgentPoolChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if c.getConnectedAgents == nil || c.getTotalAgents == nil {
		result.Status = StatusUnknown
		result.Message = "Agent pool functions not configured"
		result.Duration = time.Since(start)
		return result
	}

	connected := c.getConnectedAgents()
	total := c.getTotalAgents()

	result.Details["connected"] = connected
	result.Details["total"] = total

	if total == 0 {
		// No agents registered yet - not necessarily unhealthy
		result.Status = StatusHealthy
		result.Message = "No agents registered"
		result.Duration = time.Since(start)
		return result
	}

	availability := float64(connected) / float64(total)
	result.Details["availability"] = fmt.Sprintf("%.1f%%", availability*100)

	switch {
	case availability < 0.5:
		result.Status = StatusUnhealthy
		result.Message = fmt.Sprintf("Critical agent availability: %.1f%%", availability*100)
	case availability < c.minHealthyThreshold:
		result.Status = StatusDegraded
		result.Message = fmt.Sprintf("Low agent availability: %.1f%%", availability*100)
	default:
		result.Status = StatusHealthy
		result.Message = fmt.Sprintf("Agent availability: %.1f%%", availability*100)
	}

	result.Duration = time.Since(start)
	return result
}

// FunctionChecker wraps a custom health check function
type FunctionChecker struct {
	name    string
	checkFn func(ctx context.Context) CheckResult
}

// NewFunctionChecker creates a new function-based health checker
func NewFunctionChecker(name string, fn func(ctx context.Context) CheckResult) *FunctionChecker {
	return &FunctionChecker{
		name:    name,
		checkFn: fn,
	}
}

// Name returns the name of the checker
func (c *FunctionChecker) Name() string {
	return c.name
}

// Check performs the health check
func (c *FunctionChecker) Check(ctx context.Context) CheckResult {
	if c.checkFn == nil {
		return CheckResult{
			Status:    StatusUnknown,
			Message:   "Check function not configured",
			Timestamp: time.Now(),
		}
	}
	return c.checkFn(ctx)
}

// ListenerInfo represents information about a network listener
type ListenerInfo struct {
	Address   string `json:"address"`
	Protocol  string `json:"protocol"`   // grpc, http
	IPVersion string `json:"ip_version"` // ipv4, ipv6
	Active    bool   `json:"active"`
}

// NetworkChecker checks the health of network listeners
type NetworkChecker struct {
	name      string
	listeners []ListenerInfo
}

// NewNetworkChecker creates a new network health checker
func NewNetworkChecker() *NetworkChecker {
	return &NetworkChecker{
		name:      "network",
		listeners: make([]ListenerInfo, 0),
	}
}

// Name returns the name of the checker
func (c *NetworkChecker) Name() string {
	return c.name
}

// AddListener adds a listener to be monitored
func (c *NetworkChecker) AddListener(info ListenerInfo) {
	c.listeners = append(c.listeners, info)
}

// ClearListeners clears all monitored listeners
func (c *NetworkChecker) ClearListeners() {
	c.listeners = make([]ListenerInfo, 0)
}

// Check performs the health check
func (c *NetworkChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Timestamp: start,
		Details:   make(map[string]interface{}),
	}

	if len(c.listeners) == 0 {
		result.Status = StatusUnknown
		result.Message = "No listeners configured"
		result.Duration = time.Since(start)
		return result
	}

	// Count listeners by type and IP version
	var activeCount, inactiveCount int
	var ipv4Count, ipv6Count int
	var grpcCount, httpCount int
	listenerDetails := make([]map[string]interface{}, 0, len(c.listeners))

	for _, listener := range c.listeners {
		if listener.Active {
			activeCount++
		} else {
			inactiveCount++
		}

		switch listener.IPVersion {
		case "ipv4":
			ipv4Count++
		case "ipv6":
			ipv6Count++
		}

		switch listener.Protocol {
		case "grpc":
			grpcCount++
		case "http":
			httpCount++
		}

		listenerDetails = append(listenerDetails, map[string]interface{}{
			"address":    listener.Address,
			"protocol":   listener.Protocol,
			"ip_version": listener.IPVersion,
			"active":     listener.Active,
		})
	}

	result.Details["listeners"] = listenerDetails
	result.Details["total"] = len(c.listeners)
	result.Details["active"] = activeCount
	result.Details["inactive"] = inactiveCount
	result.Details["ipv4_listeners"] = ipv4Count
	result.Details["ipv6_listeners"] = ipv6Count
	result.Details["grpc_listeners"] = grpcCount
	result.Details["http_listeners"] = httpCount
	result.Details["dual_stack"] = ipv4Count > 0 && ipv6Count > 0

	// Determine overall status
	switch {
	case activeCount == 0:
		result.Status = StatusUnhealthy
		result.Message = "No active listeners"
	case inactiveCount > 0:
		result.Status = StatusDegraded
		result.Message = fmt.Sprintf("%d of %d listeners active", activeCount, len(c.listeners))
	default:
		result.Status = StatusHealthy
		switch {
		case ipv4Count > 0 && ipv6Count > 0:
			result.Message = fmt.Sprintf("All %d listeners active (dual-stack)", activeCount)
		case ipv6Count > 0:
			result.Message = fmt.Sprintf("All %d listeners active (IPv6)", activeCount)
		default:
			result.Message = fmt.Sprintf("All %d listeners active (IPv4)", activeCount)
		}
	}

	result.Duration = time.Since(start)
	return result
}

// AlwaysHealthyChecker always returns healthy (for testing)
type AlwaysHealthyChecker struct {
	name string
}

// NewAlwaysHealthyChecker creates a checker that always returns healthy
func NewAlwaysHealthyChecker(name string) *AlwaysHealthyChecker {
	return &AlwaysHealthyChecker{name: name}
}

// Name returns the name of the checker
func (c *AlwaysHealthyChecker) Name() string {
	return c.name
}

// Check always returns healthy
func (c *AlwaysHealthyChecker) Check(ctx context.Context) CheckResult {
	return CheckResult{
		Status:    StatusHealthy,
		Message:   "Always healthy",
		Timestamp: time.Now(),
		Duration:  0,
	}
}
