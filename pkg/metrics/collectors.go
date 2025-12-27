package metrics

import "time"

// Keystone Core metric names as constants
const (
	// Control Plane Metrics
	MetricAPIRequestsTotal           = "titan_api_requests_total"
	MetricAPIRequestDuration         = "titan_api_request_duration_seconds"
	MetricAgentsConnected            = "titan_agents_connected"
	MetricAgentsDisconnected         = "titan_agents_disconnected_total"
	MetricCommandExecutionsTotal     = "titan_command_executions_total"
	MetricCommandExecutionDuration   = "titan_command_execution_duration_seconds"
	MetricStateApplicationsTotal     = "titan_state_applications_total"
	MetricStateApplicationDuration   = "titan_state_application_duration_seconds"
	MetricPolicyEvaluationsTotal     = "titan_policy_evaluations_total"
	MetricEventsPublishedTotal       = "titan_events_published_total"
	MetricEventsProcessedTotal       = "titan_events_processed_total"

	// Agent Metrics
	MetricAgentHeartbeat             = "titan_agent_heartbeat_seconds"
	MetricAgentCPUUsage              = "titan_agent_cpu_usage_percent"
	MetricAgentMemoryUsage           = "titan_agent_memory_usage_bytes"
	MetricAgentDiskUsage             = "titan_agent_disk_usage_bytes"
	MetricAgentCommandsExecuted      = "titan_agent_commands_executed_total"
	MetricAgentStatesApplied         = "titan_agent_states_applied_total"

	// State Management Metrics
	MetricStateResourcesTotal        = "titan_state_resources_total"
	MetricStateDriftDetected         = "titan_state_drift_detected_total"
	MetricStateChangesApplied        = "titan_state_changes_applied_total"

	// GitOps Metrics
	MetricGitOpsWebhooksReceived     = "titan_gitops_webhooks_received_total"
	MetricGitOpsDeploymentsVerified  = "titan_gitops_deployments_verified_total"
	MetricGitOpsRollbacksTriggered   = "titan_gitops_rollbacks_triggered_total"

	// Policy Metrics
	MetricPolicyViolationsTotal      = "titan_policy_violations_total"
	MetricPolicyRemediationsTotal    = "titan_policy_remediations_total"
	MetricComplianceScore            = "titan_compliance_score"
)

// InitializeStandardMetrics registers all standard Keystone Core metrics
func InitializeStandardMetrics(collector *PrometheusCollector) error {
	metrics := []MetricDefinition{
		// Control Plane Metrics
		{
			Name:   MetricAPIRequestsTotal,
			Type:   MetricTypeCounter,
			Help:   "Total number of API requests",
			Labels: []string{"method", "endpoint", "status"},
		},
		{
			Name:   MetricAPIRequestDuration,
			Type:   MetricTypeHistogram,
			Help:   "API request duration in seconds",
			Labels: []string{"method", "endpoint"},
			Buckets: DefaultBuckets,
		},
		{
			Name:   MetricAgentsConnected,
			Type:   MetricTypeGauge,
			Help:   "Number of currently connected agents",
			Labels: []string{"datacenter", "role"},
		},
		{
			Name:   MetricAgentsDisconnected,
			Type:   MetricTypeCounter,
			Help:   "Total number of agent disconnections",
			Labels: []string{},
		},
		{
			Name:   MetricCommandExecutionsTotal,
			Type:   MetricTypeCounter,
			Help:   "Total number of command executions",
			Labels: []string{"status"},
		},
		{
			Name:   MetricCommandExecutionDuration,
			Type:   MetricTypeHistogram,
			Help:   "Command execution duration in seconds",
			Labels: []string{"status"},
			Buckets: DefaultBuckets,
		},
		{
			Name:   MetricStateApplicationsTotal,
			Type:   MetricTypeCounter,
			Help:   "Total number of state applications",
			Labels: []string{"status"},
		},
		{
			Name:   MetricStateApplicationDuration,
			Type:   MetricTypeHistogram,
			Help:   "State application duration in seconds",
			Labels: []string{"status"},
			Buckets: DefaultBuckets,
		},
		{
			Name:   MetricPolicyEvaluationsTotal,
			Type:   MetricTypeCounter,
			Help:   "Total number of policy evaluations",
			Labels: []string{"policy", "result"},
		},
		{
			Name:   MetricEventsPublishedTotal,
			Type:   MetricTypeCounter,
			Help:   "Total number of events published",
			Labels: []string{"type"},
		},
		{
			Name:   MetricEventsProcessedTotal,
			Type:   MetricTypeCounter,
			Help:   "Total number of events processed",
			Labels: []string{"type"},
		},

		// Agent Metrics
		{
			Name:   MetricAgentHeartbeat,
			Type:   MetricTypeGauge,
			Help:   "Timestamp of last heartbeat from agent",
			Labels: []string{"agent_id"},
		},
		{
			Name:   MetricAgentCPUUsage,
			Type:   MetricTypeGauge,
			Help:   "Agent CPU usage percentage",
			Labels: []string{"agent_id"},
		},
		{
			Name:   MetricAgentMemoryUsage,
			Type:   MetricTypeGauge,
			Help:   "Agent memory usage in bytes",
			Labels: []string{"agent_id"},
		},
		{
			Name:   MetricAgentDiskUsage,
			Type:   MetricTypeGauge,
			Help:   "Agent disk usage in bytes",
			Labels: []string{"agent_id"},
		},
		{
			Name:   MetricAgentCommandsExecuted,
			Type:   MetricTypeCounter,
			Help:   "Total number of commands executed on agent",
			Labels: []string{"agent_id", "status"},
		},
		{
			Name:   MetricAgentStatesApplied,
			Type:   MetricTypeCounter,
			Help:   "Total number of states applied on agent",
			Labels: []string{"agent_id", "status"},
		},

		// State Management Metrics
		{
			Name:   MetricStateResourcesTotal,
			Type:   MetricTypeGauge,
			Help:   "Total number of state resources",
			Labels: []string{"type", "status"},
		},
		{
			Name:   MetricStateDriftDetected,
			Type:   MetricTypeCounter,
			Help:   "Total number of drift detections",
			Labels: []string{"resource"},
		},
		{
			Name:   MetricStateChangesApplied,
			Type:   MetricTypeCounter,
			Help:   "Total number of state changes applied",
			Labels: []string{"module"},
		},

		// GitOps Metrics
		{
			Name:   MetricGitOpsWebhooksReceived,
			Type:   MetricTypeCounter,
			Help:   "Total number of webhooks received",
			Labels: []string{"source"},
		},
		{
			Name:   MetricGitOpsDeploymentsVerified,
			Type:   MetricTypeCounter,
			Help:   "Total number of deployments verified",
			Labels: []string{"status"},
		},
		{
			Name:   MetricGitOpsRollbacksTriggered,
			Type:   MetricTypeCounter,
			Help:   "Total number of rollbacks triggered",
			Labels: []string{},
		},

		// Policy Metrics
		{
			Name:   MetricPolicyViolationsTotal,
			Type:   MetricTypeCounter,
			Help:   "Total number of policy violations",
			Labels: []string{"policy", "severity"},
		},
		{
			Name:   MetricPolicyRemediationsTotal,
			Type:   MetricTypeCounter,
			Help:   "Total number of policy remediations",
			Labels: []string{"policy", "status"},
		},
		{
			Name:   MetricComplianceScore,
			Type:   MetricTypeGauge,
			Help:   "Compliance score by framework",
			Labels: []string{"framework"},
		},
	}

	for _, metric := range metrics {
		if err := collector.RegisterMetric(metric); err != nil {
			return err
		}
	}

	return nil
}

// ControlPlaneCollector provides metrics collection for control plane operations
type ControlPlaneCollector struct {
	collector Collector
}

// NewControlPlaneCollector creates a new control plane metrics collector
func NewControlPlaneCollector(collector Collector) *ControlPlaneCollector {
	return &ControlPlaneCollector{collector: collector}
}

// RecordAPIRequest records an API request
func (c *ControlPlaneCollector) RecordAPIRequest(method, endpoint, status string, duration time.Duration) {
	c.collector.IncCounter(MetricAPIRequestsTotal, map[string]string{
		"method":   method,
		"endpoint": endpoint,
		"status":   status,
	})
	c.collector.RecordDuration(MetricAPIRequestDuration, duration, map[string]string{
		"method":   method,
		"endpoint": endpoint,
	})
}

// SetAgentsConnected sets the number of connected agents
func (c *ControlPlaneCollector) SetAgentsConnected(datacenter, role string, count float64) {
	c.collector.SetGauge(MetricAgentsConnected, count, map[string]string{
		"datacenter": datacenter,
		"role":       role,
	})
}

// RecordAgentDisconnect records an agent disconnection
func (c *ControlPlaneCollector) RecordAgentDisconnect() {
	c.collector.IncCounter(MetricAgentsDisconnected, map[string]string{})
}

// RecordCommandExecution records a command execution
func (c *ControlPlaneCollector) RecordCommandExecution(status string, duration time.Duration) {
	c.collector.IncCounter(MetricCommandExecutionsTotal, map[string]string{
		"status": status,
	})
	c.collector.RecordDuration(MetricCommandExecutionDuration, duration, map[string]string{
		"status": status,
	})
}

// RecordStateApplication records a state application
func (c *ControlPlaneCollector) RecordStateApplication(status string, duration time.Duration) {
	c.collector.IncCounter(MetricStateApplicationsTotal, map[string]string{
		"status": status,
	})
	c.collector.RecordDuration(MetricStateApplicationDuration, duration, map[string]string{
		"status": status,
	})
}

// RecordPolicyEvaluation records a policy evaluation
func (c *ControlPlaneCollector) RecordPolicyEvaluation(policy, result string) {
	c.collector.IncCounter(MetricPolicyEvaluationsTotal, map[string]string{
		"policy": policy,
		"result": result,
	})
}

// RecordEventPublished records an event publication
func (c *ControlPlaneCollector) RecordEventPublished(eventType string) {
	c.collector.IncCounter(MetricEventsPublishedTotal, map[string]string{
		"type": eventType,
	})
}

// RecordEventProcessed records an event processing
func (c *ControlPlaneCollector) RecordEventProcessed(eventType string) {
	c.collector.IncCounter(MetricEventsProcessedTotal, map[string]string{
		"type": eventType,
	})
}

// AgentCollector provides metrics collection for agent operations
type AgentCollector struct {
	collector Collector
}

// NewAgentCollector creates a new agent metrics collector
func NewAgentCollector(collector Collector) *AgentCollector {
	return &AgentCollector{collector: collector}
}

// RecordHeartbeat records agent heartbeat timestamp
func (a *AgentCollector) RecordHeartbeat(agentID string) {
	a.collector.SetGauge(MetricAgentHeartbeat, float64(time.Now().Unix()), map[string]string{
		"agent_id": agentID,
	})
}

// RecordCPUUsage records agent CPU usage
func (a *AgentCollector) RecordCPUUsage(agentID string, percentage float64) {
	a.collector.SetGauge(MetricAgentCPUUsage, percentage, map[string]string{
		"agent_id": agentID,
	})
}

// RecordMemoryUsage records agent memory usage
func (a *AgentCollector) RecordMemoryUsage(agentID string, bytes float64) {
	a.collector.SetGauge(MetricAgentMemoryUsage, bytes, map[string]string{
		"agent_id": agentID,
	})
}

// RecordDiskUsage records agent disk usage
func (a *AgentCollector) RecordDiskUsage(agentID string, bytes float64) {
	a.collector.SetGauge(MetricAgentDiskUsage, bytes, map[string]string{
		"agent_id": agentID,
	})
}

// RecordCommandExecuted records a command execution on the agent
func (a *AgentCollector) RecordCommandExecuted(agentID, status string) {
	a.collector.IncCounter(MetricAgentCommandsExecuted, map[string]string{
		"agent_id": agentID,
		"status":   status,
	})
}

// RecordStateApplied records a state application on the agent
func (a *AgentCollector) RecordStateApplied(agentID, status string) {
	a.collector.IncCounter(MetricAgentStatesApplied, map[string]string{
		"agent_id": agentID,
		"status":   status,
	})
}

// StateCollector provides metrics collection for state management operations
type StateCollector struct {
	collector Collector
}

// NewStateCollector creates a new state metrics collector
func NewStateCollector(collector Collector) *StateCollector {
	return &StateCollector{collector: collector}
}

// SetResourceCount sets the number of resources
func (s *StateCollector) SetResourceCount(resourceType, status string, count float64) {
	s.collector.SetGauge(MetricStateResourcesTotal, count, map[string]string{
		"type":   resourceType,
		"status": status,
	})
}

// RecordDriftDetection records a drift detection
func (s *StateCollector) RecordDriftDetection(resource string) {
	s.collector.IncCounter(MetricStateDriftDetected, map[string]string{
		"resource": resource,
	})
}

// RecordStateChange records a state change
func (s *StateCollector) RecordStateChange(module string) {
	s.collector.IncCounter(MetricStateChangesApplied, map[string]string{
		"module": module,
	})
}

// GitOpsCollector provides metrics collection for GitOps operations
type GitOpsCollector struct {
	collector Collector
}

// NewGitOpsCollector creates a new GitOps metrics collector
func NewGitOpsCollector(collector Collector) *GitOpsCollector {
	return &GitOpsCollector{collector: collector}
}

// RecordWebhookReceived records a webhook reception
func (g *GitOpsCollector) RecordWebhookReceived(source string) {
	g.collector.IncCounter(MetricGitOpsWebhooksReceived, map[string]string{
		"source": source,
	})
}

// RecordDeploymentVerified records a deployment verification
func (g *GitOpsCollector) RecordDeploymentVerified(status string) {
	g.collector.IncCounter(MetricGitOpsDeploymentsVerified, map[string]string{
		"status": status,
	})
}

// RecordRollbackTriggered records a rollback trigger
func (g *GitOpsCollector) RecordRollbackTriggered() {
	g.collector.IncCounter(MetricGitOpsRollbacksTriggered, map[string]string{})
}

// PolicyCollector provides metrics collection for policy operations
type PolicyCollector struct {
	collector Collector
}

// NewPolicyCollector creates a new policy metrics collector
func NewPolicyCollector(collector Collector) *PolicyCollector {
	return &PolicyCollector{collector: collector}
}

// RecordViolation records a policy violation
func (p *PolicyCollector) RecordViolation(policy, severity string) {
	p.collector.IncCounter(MetricPolicyViolationsTotal, map[string]string{
		"policy":   policy,
		"severity": severity,
	})
}

// RecordRemediation records a policy remediation
func (p *PolicyCollector) RecordRemediation(policy, status string) {
	p.collector.IncCounter(MetricPolicyRemediationsTotal, map[string]string{
		"policy": policy,
		"status": status,
	})
}

// SetComplianceScore sets the compliance score
func (p *PolicyCollector) SetComplianceScore(framework string, score float64) {
	p.collector.SetGauge(MetricComplianceScore, score, map[string]string{
		"framework": framework,
	})
}
