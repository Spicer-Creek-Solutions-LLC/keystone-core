// Package kms provides anomaly detection for secret access patterns.
package kms

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// AnomalyDetector detects unusual secret access patterns.
type AnomalyDetector struct {
	config        *AnomalyConfig
	accessHistory map[string]*AccessHistory
	alerts        []AnomalyAlert
	alertHandlers []AnomalyAlertHandler
	mu            sync.RWMutex
	alertMu       sync.Mutex
}

// AnomalyConfig contains configuration for anomaly detection.
type AnomalyConfig struct {
	// WindowSize is the time window for calculating baselines.
	WindowSize time.Duration `json:"window_size,omitempty"`

	// AccessThreshold is the maximum normal access rate per minute.
	AccessThreshold float64 `json:"access_threshold,omitempty"`

	// BurstThreshold is the maximum burst access count.
	BurstThreshold int `json:"burst_threshold,omitempty"`

	// BurstWindow is the time window for burst detection.
	BurstWindow time.Duration `json:"burst_window,omitempty"`

	// EnumerationThreshold is the number of unique secrets accessed to trigger enumeration alert.
	EnumerationThreshold int `json:"enumeration_threshold,omitempty"`

	// EnumerationWindow is the time window for enumeration detection.
	EnumerationWindow time.Duration `json:"enumeration_window,omitempty"`

	// OffHoursStart defines the start of off-hours (UTC hour).
	OffHoursStart int `json:"off_hours_start,omitempty"`

	// OffHoursEnd defines the end of off-hours (UTC hour).
	OffHoursEnd int `json:"off_hours_end,omitempty"`

	// UnusualSourceEnabled enables detection of unusual source IPs.
	UnusualSourceEnabled bool `json:"unusual_source_enabled,omitempty"`

	// FailureThreshold is the number of failures to trigger alert.
	FailureThreshold int `json:"failure_threshold,omitempty"`

	// FailureWindow is the time window for failure detection.
	FailureWindow time.Duration `json:"failure_window,omitempty"`

	// RetentionPeriod is how long to keep access history.
	RetentionPeriod time.Duration `json:"retention_period,omitempty"`
}

// DefaultAnomalyConfig returns default anomaly detection configuration.
func DefaultAnomalyConfig() *AnomalyConfig {
	return &AnomalyConfig{
		WindowSize:           1 * time.Hour,
		AccessThreshold:      100,
		BurstThreshold:       50,
		BurstWindow:          1 * time.Minute,
		EnumerationThreshold: 20,
		EnumerationWindow:    5 * time.Minute,
		OffHoursStart:        22,
		OffHoursEnd:          6,
		UnusualSourceEnabled: true,
		FailureThreshold:     10,
		FailureWindow:        5 * time.Minute,
		RetentionPeriod:      24 * time.Hour,
	}
}

// AccessHistory tracks access patterns for a principal.
type AccessHistory struct {
	Principal       string          `json:"principal"`
	TotalAccesses   int64           `json:"total_accesses"`
	SuccessCount    int64           `json:"success_count"`
	FailureCount    int64           `json:"failure_count"`
	LastAccess      time.Time       `json:"last_access"`
	AccessTimes     []time.Time     `json:"-"`
	SecretsAccessed map[string]int  `json:"secrets_accessed"`
	SourceIPs       map[string]int  `json:"source_ips"`
	KnownSources    map[string]bool `json:"-"`
	HourlyPattern   [24]int         `json:"hourly_pattern"`
	Baseline        *AccessBaseline `json:"baseline,omitempty"`
}

// AccessBaseline contains baseline statistics for normal behavior.
type AccessBaseline struct {
	AvgAccessRate    float64   `json:"avg_access_rate"`
	StdDevAccessRate float64   `json:"stddev_access_rate"`
	AvgSecretsPerDay float64   `json:"avg_secrets_per_day"`
	CommonHours      []int     `json:"common_hours"`
	CommonSources    []string  `json:"common_sources"`
	LastUpdated      time.Time `json:"last_updated"`
}

// AnomalyAlert represents a detected anomaly.
type AnomalyAlert struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	Type         AnomalyType            `json:"type"`
	Severity     AlertSeverity          `json:"severity"`
	Principal    string                 `json:"principal"`
	ResourceID   string                 `json:"resource_id,omitempty"`
	SourceIP     string                 `json:"source_ip,omitempty"`
	Description  string                 `json:"description"`
	Details      map[string]interface{} `json:"details,omitempty"`
	Acknowledged bool                   `json:"acknowledged"`
}

// AnomalyType categorizes anomaly types.
type AnomalyType string

// AnomalyTypeExcessiveAccess constants define the supported types.
const (
	AnomalyTypeExcessiveAccess   AnomalyType = "excessive_access"
	AnomalyTypeBurstAccess       AnomalyType = "burst_access"
	AnomalyTypeEnumeration       AnomalyType = "enumeration"
	AnomalyTypeOffHoursAccess    AnomalyType = "off_hours_access"
	AnomalyTypeUnusualSource     AnomalyType = "unusual_source"
	AnomalyTypeExcessiveFailures AnomalyType = "excessive_failures"
	AnomalyTypeFirstTimeAccess   AnomalyType = "first_time_access"
	AnomalyTypeSensitiveAccess   AnomalyType = "sensitive_access"
	AnomalyTypePatternDeviation  AnomalyType = "pattern_deviation"
)

// AlertSeverity indicates alert severity.
type AlertSeverity string

// AlertSeverity constants define the severity levels.
const (
	AlertSeverityLow      AlertSeverity = "low"
	AlertSeverityMedium   AlertSeverity = "medium"
	AlertSeverityHigh     AlertSeverity = "high"
	AlertSeverityCritical AlertSeverity = "critical"
)

// AnomalyAlertHandler handles anomaly alerts.
type AnomalyAlertHandler func(ctx context.Context, alert *AnomalyAlert) error

// NewAnomalyDetector creates a new anomaly detector.
func NewAnomalyDetector(config *AnomalyConfig) *AnomalyDetector {
	if config == nil {
		config = DefaultAnomalyConfig()
	}

	ad := &AnomalyDetector{
		config:        config,
		accessHistory: make(map[string]*AccessHistory),
		alerts:        make([]AnomalyAlert, 0),
		alertHandlers: make([]AnomalyAlertHandler, 0),
	}

	go ad.cleanupLoop()

	return ad
}

// AddAlertHandler adds an alert handler.
func (ad *AnomalyDetector) AddAlertHandler(handler AnomalyAlertHandler) {
	ad.mu.Lock()
	ad.alertHandlers = append(ad.alertHandlers, handler)
	ad.mu.Unlock()
}

// RecordAccess records a secret access for anomaly detection.
func (ad *AnomalyDetector) RecordAccess(ctx context.Context, access *SecretAccess) []AnomalyAlert {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	history := ad.getOrCreateHistory(access.Principal)
	now := time.Now()

	// Update history
	history.TotalAccesses++
	history.LastAccess = now
	history.AccessTimes = append(history.AccessTimes, now)

	if access.Success {
		history.SuccessCount++
	} else {
		history.FailureCount++
	}

	if access.SecretID != "" {
		if history.SecretsAccessed == nil {
			history.SecretsAccessed = make(map[string]int)
		}
		history.SecretsAccessed[access.SecretID]++
	}

	if access.SourceIP != "" {
		if history.SourceIPs == nil {
			history.SourceIPs = make(map[string]int)
		}
		history.SourceIPs[access.SourceIP]++
	}

	history.HourlyPattern[now.Hour()]++

	// Run anomaly checks
	alerts := ad.checkAnomalies(ctx, history, access)

	// Fire alerts
	for i := range alerts {
		ad.fireAlert(ctx, &alerts[i])
	}

	return alerts
}

// SecretAccess represents a secret access event for anomaly detection.
type SecretAccess struct {
	Principal string    `json:"principal"`
	SecretID  string    `json:"secret_id"`
	Action    string    `json:"action"`
	SourceIP  string    `json:"source_ip"`
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
	Sensitive bool      `json:"sensitive"`
}

// getOrCreateHistory gets or creates access history for a principal.
func (ad *AnomalyDetector) getOrCreateHistory(principal string) *AccessHistory {
	history, exists := ad.accessHistory[principal]
	if !exists {
		history = &AccessHistory{
			Principal:       principal,
			SecretsAccessed: make(map[string]int),
			SourceIPs:       make(map[string]int),
			KnownSources:    make(map[string]bool),
			AccessTimes:     make([]time.Time, 0),
		}
		ad.accessHistory[principal] = history
	}
	return history
}

// checkAnomalies checks for various anomaly types.
func (ad *AnomalyDetector) checkAnomalies(ctx context.Context, history *AccessHistory, access *SecretAccess) []AnomalyAlert {
	var alerts []AnomalyAlert

	// Check excessive access rate
	if alert := ad.checkExcessiveAccess(history, access); alert != nil {
		alerts = append(alerts, *alert)
	}

	// Check burst access
	if alert := ad.checkBurstAccess(history, access); alert != nil {
		alerts = append(alerts, *alert)
	}

	// Check enumeration
	if alert := ad.checkEnumeration(history, access); alert != nil {
		alerts = append(alerts, *alert)
	}

	// Check off-hours access
	if alert := ad.checkOffHoursAccess(history, access); alert != nil {
		alerts = append(alerts, *alert)
	}

	// Check unusual source
	if alert := ad.checkUnusualSource(history, access); alert != nil {
		alerts = append(alerts, *alert)
	}

	// Check excessive failures
	if alert := ad.checkExcessiveFailures(history, access); alert != nil {
		alerts = append(alerts, *alert)
	}

	// Check sensitive access
	if alert := ad.checkSensitiveAccess(history, access); alert != nil {
		alerts = append(alerts, *alert)
	}

	return alerts
}

// checkExcessiveAccess checks for excessive access rate.
func (ad *AnomalyDetector) checkExcessiveAccess(history *AccessHistory, access *SecretAccess) *AnomalyAlert {
	now := time.Now()
	windowStart := now.Add(-ad.config.WindowSize)

	// Count accesses in window
	var count int
	for _, t := range history.AccessTimes {
		if t.After(windowStart) {
			count++
		}
	}

	// Calculate rate per minute
	minutes := ad.config.WindowSize.Minutes()
	rate := float64(count) / minutes

	if rate > ad.config.AccessThreshold {
		return &AnomalyAlert{
			ID:          fmt.Sprintf("excessive-%s-%d", access.Principal, now.UnixNano()),
			Timestamp:   now,
			Type:        AnomalyTypeExcessiveAccess,
			Severity:    AlertSeverityMedium,
			Principal:   access.Principal,
			Description: fmt.Sprintf("Excessive access rate: %.2f/min (threshold: %.2f/min)", rate, ad.config.AccessThreshold),
			Details: map[string]interface{}{
				"rate":      rate,
				"threshold": ad.config.AccessThreshold,
				"window":    ad.config.WindowSize.String(),
			},
		}
	}

	return nil
}

// checkBurstAccess checks for burst access patterns.
func (ad *AnomalyDetector) checkBurstAccess(history *AccessHistory, access *SecretAccess) *AnomalyAlert {
	now := time.Now()
	windowStart := now.Add(-ad.config.BurstWindow)

	var count int
	for _, t := range history.AccessTimes {
		if t.After(windowStart) {
			count++
		}
	}

	if count > ad.config.BurstThreshold {
		return &AnomalyAlert{
			ID:          fmt.Sprintf("burst-%s-%d", access.Principal, now.UnixNano()),
			Timestamp:   now,
			Type:        AnomalyTypeBurstAccess,
			Severity:    AlertSeverityHigh,
			Principal:   access.Principal,
			Description: fmt.Sprintf("Burst access detected: %d accesses in %s", count, ad.config.BurstWindow),
			Details: map[string]interface{}{
				"count":     count,
				"threshold": ad.config.BurstThreshold,
				"window":    ad.config.BurstWindow.String(),
			},
		}
	}

	return nil
}

// checkEnumeration checks for secret enumeration attempts.
func (ad *AnomalyDetector) checkEnumeration(history *AccessHistory, access *SecretAccess) *AnomalyAlert {
	now := time.Now()
	_ = now.Add(-ad.config.EnumerationWindow) // reserved for future timestamp-based filtering

	// Count unique secrets accessed in window
	recentSecrets := make(map[string]bool)
	for secretID := range history.SecretsAccessed {
		// We'd need timestamps per secret for accurate check
		// For now, use total unique secrets
		recentSecrets[secretID] = true
	}

	if len(recentSecrets) > ad.config.EnumerationThreshold {
		return &AnomalyAlert{
			ID:          fmt.Sprintf("enum-%s-%d", access.Principal, now.UnixNano()),
			Timestamp:   now,
			Type:        AnomalyTypeEnumeration,
			Severity:    AlertSeverityCritical,
			Principal:   access.Principal,
			Description: fmt.Sprintf("Possible secret enumeration: %d unique secrets accessed", len(recentSecrets)),
			Details: map[string]interface{}{
				"unique_secrets": len(recentSecrets),
				"threshold":      ad.config.EnumerationThreshold,
				"window":         ad.config.EnumerationWindow.String(),
			},
		}
	}

	return nil
}

// checkOffHoursAccess checks for access during off-hours.
func (ad *AnomalyDetector) checkOffHoursAccess(history *AccessHistory, access *SecretAccess) *AnomalyAlert {
	now := time.Now().UTC()
	hour := now.Hour()

	var isOffHours bool
	if ad.config.OffHoursStart > ad.config.OffHoursEnd {
		// Spans midnight (e.g., 22:00 to 06:00)
		isOffHours = hour >= ad.config.OffHoursStart || hour < ad.config.OffHoursEnd
	} else {
		isOffHours = hour >= ad.config.OffHoursStart && hour < ad.config.OffHoursEnd
	}

	if isOffHours {
		// Check if this is unusual for the principal
		dayAccesses := 0
		nightAccesses := 0
		for h, count := range history.HourlyPattern {
			if h >= ad.config.OffHoursStart || h < ad.config.OffHoursEnd {
				nightAccesses += count
			} else {
				dayAccesses += count
			}
		}

		// Alert if mostly day activity and now accessing at night
		if dayAccesses > nightAccesses*3 && nightAccesses < 10 {
			return &AnomalyAlert{
				ID:          fmt.Sprintf("offhours-%s-%d", access.Principal, now.UnixNano()),
				Timestamp:   now,
				Type:        AnomalyTypeOffHoursAccess,
				Severity:    AlertSeverityMedium,
				Principal:   access.Principal,
				ResourceID:  access.SecretID,
				Description: fmt.Sprintf("Off-hours access at %02d:00 UTC (off-hours: %02d:00-%02d:00)", hour, ad.config.OffHoursStart, ad.config.OffHoursEnd),
				Details: map[string]interface{}{
					"hour":           hour,
					"day_accesses":   dayAccesses,
					"night_accesses": nightAccesses,
				},
			}
		}
	}

	return nil
}

// checkUnusualSource checks for access from unusual source IPs.
func (ad *AnomalyDetector) checkUnusualSource(history *AccessHistory, access *SecretAccess) *AnomalyAlert {
	if !ad.config.UnusualSourceEnabled || access.SourceIP == "" {
		return nil
	}

	// If this is a new source IP for an established principal
	if history.TotalAccesses > 10 && !history.KnownSources[access.SourceIP] {
		history.KnownSources[access.SourceIP] = true

		return &AnomalyAlert{
			ID:          fmt.Sprintf("source-%s-%d", access.Principal, time.Now().UnixNano()),
			Timestamp:   time.Now(),
			Type:        AnomalyTypeUnusualSource,
			Severity:    AlertSeverityMedium,
			Principal:   access.Principal,
			SourceIP:    access.SourceIP,
			Description: fmt.Sprintf("Access from new source IP: %s", access.SourceIP),
			Details: map[string]interface{}{
				"new_source":     access.SourceIP,
				"known_sources":  len(history.KnownSources),
				"total_accesses": history.TotalAccesses,
			},
		}
	}

	return nil
}

// checkExcessiveFailures checks for excessive access failures.
func (ad *AnomalyDetector) checkExcessiveFailures(history *AccessHistory, access *SecretAccess) *AnomalyAlert {
	if access.Success {
		return nil
	}

	now := time.Now()

	// Simple check: if failure rate is high
	if history.TotalAccesses > 5 {
		failureRate := float64(history.FailureCount) / float64(history.TotalAccesses)
		if failureRate > 0.5 && history.FailureCount >= int64(ad.config.FailureThreshold) {
			return &AnomalyAlert{
				ID:          fmt.Sprintf("failures-%s-%d", access.Principal, now.UnixNano()),
				Timestamp:   now,
				Type:        AnomalyTypeExcessiveFailures,
				Severity:    AlertSeverityHigh,
				Principal:   access.Principal,
				Description: fmt.Sprintf("High failure rate: %.1f%% (%d failures)", failureRate*100, history.FailureCount),
				Details: map[string]interface{}{
					"failure_count": history.FailureCount,
					"total_count":   history.TotalAccesses,
					"failure_rate":  failureRate,
				},
			}
		}
	}

	return nil
}

// checkSensitiveAccess checks for access to sensitive secrets.
func (ad *AnomalyDetector) checkSensitiveAccess(history *AccessHistory, access *SecretAccess) *AnomalyAlert {
	if !access.Sensitive {
		return nil
	}

	// Alert on first-time access to sensitive secrets
	if history.SecretsAccessed[access.SecretID] == 1 {
		return &AnomalyAlert{
			ID:          fmt.Sprintf("sensitive-%s-%d", access.Principal, time.Now().UnixNano()),
			Timestamp:   time.Now(),
			Type:        AnomalyTypeSensitiveAccess,
			Severity:    AlertSeverityMedium,
			Principal:   access.Principal,
			ResourceID:  access.SecretID,
			Description: "First-time access to sensitive secret",
			Details: map[string]interface{}{
				"secret_id": MaskSecretID(access.SecretID),
				"action":    access.Action,
			},
		}
	}

	return nil
}

// fireAlert fires an alert to all handlers.
func (ad *AnomalyDetector) fireAlert(ctx context.Context, alert *AnomalyAlert) {
	ad.alertMu.Lock()
	ad.alerts = append(ad.alerts, *alert)
	ad.alertMu.Unlock()

	for _, handler := range ad.alertHandlers {
		go func(h func(context.Context, *AnomalyAlert) error) { _ = h(ctx, alert) }(handler) //nolint:errcheck // best-effort async alert
	}
}

// GetAlerts returns all alerts.
func (ad *AnomalyDetector) GetAlerts(limit int) []AnomalyAlert {
	ad.alertMu.Lock()
	defer ad.alertMu.Unlock()

	if limit <= 0 || limit > len(ad.alerts) {
		limit = len(ad.alerts)
	}

	start := len(ad.alerts) - limit
	if start < 0 {
		start = 0
	}

	result := make([]AnomalyAlert, limit)
	copy(result, ad.alerts[start:])
	return result
}

// GetAlertsByType returns alerts of a specific type.
func (ad *AnomalyDetector) GetAlertsByType(alertType AnomalyType) []AnomalyAlert {
	ad.alertMu.Lock()
	defer ad.alertMu.Unlock()

	var result []AnomalyAlert
	for i := range ad.alerts {
		if ad.alerts[i].Type == alertType {
			result = append(result, ad.alerts[i])
		}
	}
	return result
}

// GetAlertsBySeverity returns alerts at or above a severity level.
func (ad *AnomalyDetector) GetAlertsBySeverity(minSeverity AlertSeverity) []AnomalyAlert {
	ad.alertMu.Lock()
	defer ad.alertMu.Unlock()

	severityOrder := map[AlertSeverity]int{
		AlertSeverityLow:      1,
		AlertSeverityMedium:   2,
		AlertSeverityHigh:     3,
		AlertSeverityCritical: 4,
	}

	minLevel := severityOrder[minSeverity]
	var result []AnomalyAlert
	for i := range ad.alerts {
		if severityOrder[ad.alerts[i].Severity] >= minLevel {
			result = append(result, ad.alerts[i])
		}
	}
	return result
}

// AcknowledgeAlert acknowledges an alert.
func (ad *AnomalyDetector) AcknowledgeAlert(alertID string) bool {
	ad.alertMu.Lock()
	defer ad.alertMu.Unlock()

	for i := range ad.alerts {
		if ad.alerts[i].ID == alertID {
			ad.alerts[i].Acknowledged = true
			return true
		}
	}
	return false
}

// GetPrincipalHistory returns access history for a principal.
func (ad *AnomalyDetector) GetPrincipalHistory(principal string) *AccessHistory {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	if history, exists := ad.accessHistory[principal]; exists {
		return history
	}
	return nil
}

// UpdateBaseline updates the baseline for a principal.
func (ad *AnomalyDetector) UpdateBaseline(principal string) error {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	history, exists := ad.accessHistory[principal]
	if !exists {
		return fmt.Errorf("no history for principal: %s", principal)
	}

	// Calculate baseline statistics
	baseline := &AccessBaseline{
		LastUpdated: time.Now(),
	}

	// Calculate average access rate
	if len(history.AccessTimes) > 0 {
		duration := time.Since(history.AccessTimes[0])
		if duration > 0 {
			baseline.AvgAccessRate = float64(len(history.AccessTimes)) / duration.Hours()
		}

		// Calculate standard deviation
		var sum, sumSq float64
		for i := 1; i < len(history.AccessTimes); i++ {
			interval := history.AccessTimes[i].Sub(history.AccessTimes[i-1]).Seconds()
			sum += interval
			sumSq += interval * interval
		}
		n := float64(len(history.AccessTimes) - 1)
		if n > 0 {
			mean := sum / n
			variance := (sumSq / n) - (mean * mean)
			if variance > 0 {
				baseline.StdDevAccessRate = math.Sqrt(variance)
			}
		}
	}

	// Calculate common hours
	maxCount := 0
	for _, count := range history.HourlyPattern {
		if count > maxCount {
			maxCount = count
		}
	}
	threshold := maxCount / 2
	for hour, count := range history.HourlyPattern {
		if count >= threshold {
			baseline.CommonHours = append(baseline.CommonHours, hour)
		}
	}

	// Calculate common sources
	for source, count := range history.SourceIPs {
		if count > 5 {
			baseline.CommonSources = append(baseline.CommonSources, source)
		}
	}

	history.Baseline = baseline
	return nil
}

// cleanupLoop periodically cleans up old data.
func (ad *AnomalyDetector) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		ad.cleanup()
	}
}

// cleanup removes old access history entries.
func (ad *AnomalyDetector) cleanup() {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	cutoff := time.Now().Add(-ad.config.RetentionPeriod)

	for principal, history := range ad.accessHistory {
		// Remove old access times
		var newTimes []time.Time
		for _, t := range history.AccessTimes {
			if t.After(cutoff) {
				newTimes = append(newTimes, t)
			}
		}
		history.AccessTimes = newTimes

		// Remove history if no recent activity
		if len(history.AccessTimes) == 0 && history.LastAccess.Before(cutoff) {
			delete(ad.accessHistory, principal)
		}
	}

	// Clean up old alerts
	ad.alertMu.Lock()
	var newAlerts []AnomalyAlert
	for i := range ad.alerts {
		if ad.alerts[i].Timestamp.After(cutoff) {
			newAlerts = append(newAlerts, ad.alerts[i])
		}
	}
	ad.alerts = newAlerts
	ad.alertMu.Unlock()
}

// Stats returns anomaly detection statistics.
func (ad *AnomalyDetector) Stats() AnomalyStats {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	ad.alertMu.Lock()
	defer ad.alertMu.Unlock()

	stats := AnomalyStats{
		TotalPrincipals:  len(ad.accessHistory),
		TotalAlerts:      len(ad.alerts),
		AlertsByType:     make(map[AnomalyType]int),
		AlertsBySeverity: make(map[AlertSeverity]int),
	}

	for i := range ad.alerts {
		stats.AlertsByType[ad.alerts[i].Type]++
		stats.AlertsBySeverity[ad.alerts[i].Severity]++
		if !ad.alerts[i].Acknowledged {
			stats.UnacknowledgedAlerts++
		}
	}

	return stats
}

// AnomalyStats contains anomaly detection statistics.
type AnomalyStats struct {
	TotalPrincipals      int                   `json:"total_principals"`
	TotalAlerts          int                   `json:"total_alerts"`
	UnacknowledgedAlerts int                   `json:"unacknowledged_alerts"`
	AlertsByType         map[AnomalyType]int   `json:"alerts_by_type"`
	AlertsBySeverity     map[AlertSeverity]int `json:"alerts_by_severity"`
}
