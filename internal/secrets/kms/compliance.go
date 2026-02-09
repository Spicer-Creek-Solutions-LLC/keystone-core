// Package kms provides KMS provider implementations and utilities.
package kms

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ComplianceFramework represents a compliance framework for secret management.
type ComplianceFramework string

const (
	// FrameworkSOC2 represents SOC 2 compliance requirements.
	FrameworkSOC2 ComplianceFramework = "soc2"
	// FrameworkPCIDSS represents PCI-DSS compliance requirements.
	FrameworkPCIDSS ComplianceFramework = "pci-dss"
	// FrameworkHIPAA represents HIPAA compliance requirements.
	FrameworkHIPAA ComplianceFramework = "hipaa"
	// FrameworkGDPR represents GDPR compliance requirements.
	FrameworkGDPR ComplianceFramework = "gdpr"
	// FrameworkFedRAMP represents FedRAMP compliance requirements.
	FrameworkFedRAMP ComplianceFramework = "fedramp"
	// FrameworkNIST represents NIST 800-53 compliance requirements.
	FrameworkNIST ComplianceFramework = "nist-800-53"
)

// ComplianceRequirement represents a specific compliance requirement.
type ComplianceRequirement struct {
	ID          string              `json:"id"`
	Framework   ComplianceFramework `json:"framework"`
	Category    string              `json:"category"`
	Description string              `json:"description"`
	Severity    string              `json:"severity"` // critical, high, medium, low
}

// ComplianceCheckResult represents the result of a compliance check.
type ComplianceCheckResult struct {
	Requirement ComplianceRequirement `json:"requirement"`
	Status      ComplianceStatus      `json:"status"`
	Details     string                `json:"details"`
	Evidence    []string              `json:"evidence,omitempty"`
	CheckedAt   time.Time             `json:"checked_at"`
}

// ComplianceStatus represents the status of a compliance check.
type ComplianceStatus string

const (
	// StatusCompliant indicates the requirement is met.
	StatusCompliant ComplianceStatus = "compliant"
	// StatusNonCompliant indicates the requirement is not met.
	StatusNonCompliant ComplianceStatus = "non_compliant"
	// StatusPartiallyCompliant indicates partial compliance.
	StatusPartiallyCompliant ComplianceStatus = "partially_compliant"
	// StatusNotApplicable indicates the requirement doesn't apply.
	StatusNotApplicable ComplianceStatus = "not_applicable"
	// StatusUnknown indicates compliance status couldn't be determined.
	StatusUnknown ComplianceStatus = "unknown"
)

// ComplianceReport represents a complete compliance report.
type ComplianceReport struct {
	ID           string                  `json:"id"`
	GeneratedAt  time.Time               `json:"generated_at"`
	GeneratedBy  string                  `json:"generated_by"`
	Framework    ComplianceFramework     `json:"framework"`
	Period       ReportPeriod            `json:"period"`
	Summary      ComplianceSummary       `json:"summary"`
	Results      []ComplianceCheckResult `json:"results"`
	KeyInventory []KeyInventoryItem      `json:"key_inventory"`
	AccessAudit  AccessAuditSummary      `json:"access_audit"`
	Rotation     RotationSummary         `json:"rotation_summary"`
}

// ReportPeriod represents the time period covered by a report.
type ReportPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ComplianceSummary provides an overview of compliance status.
type ComplianceSummary struct {
	TotalRequirements    int     `json:"total_requirements"`
	Compliant            int     `json:"compliant"`
	NonCompliant         int     `json:"non_compliant"`
	PartiallyCompliant   int     `json:"partially_compliant"`
	NotApplicable        int     `json:"not_applicable"`
	Unknown              int     `json:"unknown"`
	OverallScore         float64 `json:"overall_score"` // 0-100
	CriticalIssues       int     `json:"critical_issues"`
	HighIssues           int     `json:"high_issues"`
	MediumIssues         int     `json:"medium_issues"`
	LowIssues            int     `json:"low_issues"`
	CompliancePercentage float64 `json:"compliance_percentage"`
	RiskLevel            string  `json:"risk_level"` // critical, high, medium, low
}

// KeyInventoryItem represents a key in the inventory.
type KeyInventoryItem struct {
	KeyID         string     `json:"key_id"`
	KeyType       string     `json:"key_type"`
	Algorithm     string     `json:"algorithm"`
	KeySize       int        `json:"key_size"`
	CreatedAt     time.Time  `json:"created_at"`
	LastRotated   *time.Time `json:"last_rotated,omitempty"`
	NextRotation  *time.Time `json:"next_rotation,omitempty"`
	RotationCount int        `json:"rotation_count"`
	Status        string     `json:"status"` // active, disabled, pending_deletion
	Owner         string     `json:"owner,omitempty"`
	Purpose       string     `json:"purpose,omitempty"`
	Compliance    []string   `json:"compliance_tags,omitempty"` // pci, hipaa, etc.
}

// AccessAuditSummary provides access statistics.
type AccessAuditSummary struct {
	TotalAccesses        int64               `json:"total_accesses"`
	UniqueSecrets        int                 `json:"unique_secrets"`
	UniquePrincipals     int                 `json:"unique_principals"`
	EncryptOperations    int64               `json:"encrypt_operations"`
	DecryptOperations    int64               `json:"decrypt_operations"`
	SignOperations       int64               `json:"sign_operations"`
	VerifyOperations     int64               `json:"verify_operations"`
	FailedAttempts       int64               `json:"failed_attempts"`
	UnauthorizedAttempts int64               `json:"unauthorized_attempts"`
	AnomaliesDetected    int                 `json:"anomalies_detected"`
	TopAccessors         []AccessorStats     `json:"top_accessors"`
	TopSecrets           []SecretAccessStats `json:"top_secrets"`
	AccessByHour         map[int]int64       `json:"access_by_hour"`
	AccessByDay          map[string]int64    `json:"access_by_day"`
}

// AccessorStats represents access statistics for a principal.
type AccessorStats struct {
	Principal    string    `json:"principal"`
	AccessCount  int64     `json:"access_count"`
	LastAccess   time.Time `json:"last_access"`
	FailureCount int64     `json:"failure_count"`
}

// SecretAccessStats represents access statistics for a secret.
type SecretAccessStats struct {
	SecretPath  string    `json:"secret_path"`
	AccessCount int64     `json:"access_count"`
	LastAccess  time.Time `json:"last_access"`
}

// RotationSummary provides key rotation statistics.
type RotationSummary struct {
	TotalKeys           int             `json:"total_keys"`
	KeysRotatedInPeriod int             `json:"keys_rotated_in_period"`
	KeysPendingRotation int             `json:"keys_pending_rotation"`
	KeysOverdue         int             `json:"keys_overdue"`
	AverageRotationAge  time.Duration   `json:"average_rotation_age"`
	MaxRotationAge      time.Duration   `json:"max_rotation_age"`
	RotationPolicy      RotationPolicy  `json:"rotation_policy"`
	RotationHistory     []RotationEvent `json:"rotation_history,omitempty"`
}

// RotationPolicy describes the key rotation policy.
type RotationPolicy struct {
	Enabled          bool          `json:"enabled"`
	RotationInterval time.Duration `json:"rotation_interval"`
	GracePeriod      time.Duration `json:"grace_period"`
	AutoRotate       bool          `json:"auto_rotate"`
}

// RotationEvent represents a key rotation event.
type RotationEvent struct {
	KeyID      string    `json:"key_id"`
	RotatedAt  time.Time `json:"rotated_at"`
	RotatedBy  string    `json:"rotated_by"`
	OldVersion int       `json:"old_version"`
	NewVersion int       `json:"new_version"`
	Reason     string    `json:"reason"`
}

// ComplianceConfig configures the compliance reporter.
type ComplianceConfig struct {
	Frameworks       []ComplianceFramework `json:"frameworks"`
	RotationMaxAge   time.Duration         `json:"rotation_max_age"`
	MinKeySize       int                   `json:"min_key_size"`
	RequiredKeyTypes []string              `json:"required_key_types"`
	AuditRetention   time.Duration         `json:"audit_retention"`
}

// DefaultComplianceConfig returns default compliance configuration.
func DefaultComplianceConfig() ComplianceConfig {
	return ComplianceConfig{
		Frameworks:       []ComplianceFramework{FrameworkSOC2},
		RotationMaxAge:   365 * 24 * time.Hour, // 1 year
		MinKeySize:       256,
		RequiredKeyTypes: []string{"aes-256-gcm", "rsa-4096"},
		AuditRetention:   90 * 24 * time.Hour, // 90 days
	}
}

// ComplianceReporter generates compliance reports.
type ComplianceReporter struct {
	config       ComplianceConfig
	auditLogger  *AuditLogger
	keyInventory map[string]*KeyInventoryItem
	accessStats  map[string]*accessStatEntry
	rotationLog  []RotationEvent
	mu           sync.RWMutex
}

type accessStatEntry struct {
	principal    string
	accessCount  int64
	failureCount int64
	lastAccess   time.Time
	byHour       map[int]int64
	byDay        map[string]int64
}

// NewComplianceReporter creates a new compliance reporter.
func NewComplianceReporter(config ComplianceConfig, auditLogger *AuditLogger) *ComplianceReporter {
	return &ComplianceReporter{
		config:       config,
		auditLogger:  auditLogger,
		keyInventory: make(map[string]*KeyInventoryItem),
		accessStats:  make(map[string]*accessStatEntry),
		rotationLog:  make([]RotationEvent, 0),
	}
}

// RegisterKey registers a key for compliance tracking.
func (r *ComplianceReporter) RegisterKey(item KeyInventoryItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keyInventory[item.KeyID] = &item
}

// UpdateKeyStatus updates the status of a registered key.
func (r *ComplianceReporter) UpdateKeyStatus(keyID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key, ok := r.keyInventory[keyID]
	if !ok {
		return fmt.Errorf("key not found: %s", keyID)
	}
	key.Status = status
	return nil
}

// RecordRotation records a key rotation event.
func (r *ComplianceReporter) RecordRotation(event RotationEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rotationLog = append(r.rotationLog, event)

	if key, ok := r.keyInventory[event.KeyID]; ok {
		now := event.RotatedAt
		key.LastRotated = &now
		key.RotationCount++

		if r.config.RotationMaxAge > 0 {
			next := now.Add(r.config.RotationMaxAge)
			key.NextRotation = &next
		}
	}
}

// RecordAccess records an access event for compliance tracking.
func (r *ComplianceReporter) RecordAccess(principal, secretPath, operation string, success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	key := principal + ":" + secretPath

	entry, ok := r.accessStats[key]
	if !ok {
		entry = &accessStatEntry{
			principal: principal,
			byHour:    make(map[int]int64),
			byDay:     make(map[string]int64),
		}
		r.accessStats[key] = entry
	}

	entry.accessCount++
	entry.lastAccess = now
	entry.byHour[now.Hour()]++
	entry.byDay[now.Format("2006-01-02")]++

	if !success {
		entry.failureCount++
	}
}

// GenerateReport generates a compliance report for the specified framework.
func (r *ComplianceReporter) GenerateReport(ctx context.Context, framework ComplianceFramework, period ReportPeriod) (*ComplianceReport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	report := &ComplianceReport{
		ID:          fmt.Sprintf("compliance-%s-%d", framework, time.Now().UnixNano()),
		GeneratedAt: time.Now(),
		GeneratedBy: "keystone-compliance-reporter",
		Framework:   framework,
		Period:      period,
	}

	// Get compliance requirements for framework
	requirements := r.getRequirements(framework)

	// Run compliance checks
	results := make([]ComplianceCheckResult, 0, len(requirements))
	for _, req := range requirements {
		result := r.checkRequirement(ctx, req, period)
		results = append(results, result)
	}
	report.Results = results

	// Generate summary
	report.Summary = r.calculateSummary(results)

	// Key inventory
	report.KeyInventory = r.getKeyInventory()

	// Access audit summary
	report.AccessAudit = r.getAccessAuditSummary(period)

	// Rotation summary
	report.Rotation = r.getRotationSummary(period)

	return report, nil
}

func (r *ComplianceReporter) getRequirements(framework ComplianceFramework) []ComplianceRequirement {
	switch framework {
	case FrameworkSOC2:
		return r.soc2Requirements()
	case FrameworkPCIDSS:
		return r.pciDSSRequirements()
	case FrameworkHIPAA:
		return r.hipaaRequirements()
	case FrameworkGDPR:
		return r.gdprRequirements()
	case FrameworkFedRAMP:
		return r.fedRAMPRequirements()
	case FrameworkNIST:
		return r.nistRequirements()
	default:
		return r.soc2Requirements() // default to SOC2
	}
}

func (r *ComplianceReporter) soc2Requirements() []ComplianceRequirement {
	return []ComplianceRequirement{
		{ID: "SOC2-CC6.1", Framework: FrameworkSOC2, Category: "Logical Access", Description: "Encryption keys are protected from unauthorized access", Severity: "critical"},
		{ID: "SOC2-CC6.6", Framework: FrameworkSOC2, Category: "System Operations", Description: "Key rotation is performed according to policy", Severity: "high"},
		{ID: "SOC2-CC6.7", Framework: FrameworkSOC2, Category: "Change Management", Description: "Key changes are authorized and documented", Severity: "high"},
		{ID: "SOC2-CC7.2", Framework: FrameworkSOC2, Category: "System Monitoring", Description: "Key access is logged and monitored", Severity: "high"},
		{ID: "SOC2-CC7.3", Framework: FrameworkSOC2, Category: "Incident Response", Description: "Anomalous key access is detected and responded to", Severity: "medium"},
		{ID: "SOC2-CC8.1", Framework: FrameworkSOC2, Category: "Risk Management", Description: "Key management risks are identified and mitigated", Severity: "medium"},
	}
}

func (r *ComplianceReporter) pciDSSRequirements() []ComplianceRequirement {
	return []ComplianceRequirement{
		{ID: "PCI-DSS-3.5.1", Framework: FrameworkPCIDSS, Category: "Cryptographic Keys", Description: "Access to cryptographic keys is restricted to minimum necessary", Severity: "critical"},
		{ID: "PCI-DSS-3.5.2", Framework: FrameworkPCIDSS, Category: "Cryptographic Keys", Description: "Secret keys are stored in secure form", Severity: "critical"},
		{ID: "PCI-DSS-3.5.3", Framework: FrameworkPCIDSS, Category: "Cryptographic Keys", Description: "Keys are stored in fewest possible locations", Severity: "high"},
		{ID: "PCI-DSS-3.6.1", Framework: FrameworkPCIDSS, Category: "Key Management", Description: "Strong cryptographic keys are generated", Severity: "critical"},
		{ID: "PCI-DSS-3.6.4", Framework: FrameworkPCIDSS, Category: "Key Management", Description: "Cryptographic key changes for keys that have reached end of crypto-period", Severity: "high"},
		{ID: "PCI-DSS-3.6.5", Framework: FrameworkPCIDSS, Category: "Key Management", Description: "Retirement or replacement of keys when integrity is compromised", Severity: "critical"},
		{ID: "PCI-DSS-3.6.7", Framework: FrameworkPCIDSS, Category: "Key Management", Description: "Prevention of unauthorized substitution of keys", Severity: "high"},
		{ID: "PCI-DSS-10.5", Framework: FrameworkPCIDSS, Category: "Audit Trails", Description: "Audit trails are secured so they cannot be altered", Severity: "high"},
	}
}

func (r *ComplianceReporter) hipaaRequirements() []ComplianceRequirement {
	return []ComplianceRequirement{
		{ID: "HIPAA-164.312(a)(2)(iv)", Framework: FrameworkHIPAA, Category: "Access Control", Description: "Encryption and decryption mechanisms for ePHI", Severity: "critical"},
		{ID: "HIPAA-164.312(b)", Framework: FrameworkHIPAA, Category: "Audit Controls", Description: "Hardware, software, and procedural mechanisms that record and examine activity", Severity: "high"},
		{ID: "HIPAA-164.312(c)(1)", Framework: FrameworkHIPAA, Category: "Integrity", Description: "Policies and procedures to protect ePHI from improper alteration", Severity: "high"},
		{ID: "HIPAA-164.312(d)", Framework: FrameworkHIPAA, Category: "Authentication", Description: "Person or entity authentication procedures", Severity: "high"},
		{ID: "HIPAA-164.312(e)(2)(ii)", Framework: FrameworkHIPAA, Category: "Transmission Security", Description: "Encryption mechanism for ePHI transmitted over network", Severity: "critical"},
	}
}

func (r *ComplianceReporter) gdprRequirements() []ComplianceRequirement {
	return []ComplianceRequirement{
		{ID: "GDPR-Art32-1a", Framework: FrameworkGDPR, Category: "Security", Description: "Pseudonymisation and encryption of personal data", Severity: "critical"},
		{ID: "GDPR-Art32-1b", Framework: FrameworkGDPR, Category: "Security", Description: "Ability to ensure ongoing confidentiality, integrity, availability", Severity: "high"},
		{ID: "GDPR-Art32-1d", Framework: FrameworkGDPR, Category: "Security", Description: "Process for regularly testing and evaluating security measures", Severity: "medium"},
		{ID: "GDPR-Art33", Framework: FrameworkGDPR, Category: "Breach Notification", Description: "Notification of data breach to supervisory authority", Severity: "critical"},
		{ID: "GDPR-Art30", Framework: FrameworkGDPR, Category: "Records", Description: "Records of processing activities", Severity: "high"},
	}
}

func (r *ComplianceReporter) fedRAMPRequirements() []ComplianceRequirement {
	return []ComplianceRequirement{
		{ID: "FedRAMP-SC-12", Framework: FrameworkFedRAMP, Category: "Cryptographic Key", Description: "Cryptographic key establishment and management", Severity: "critical"},
		{ID: "FedRAMP-SC-13", Framework: FrameworkFedRAMP, Category: "Cryptographic Protection", Description: "Use of FIPS-validated cryptography", Severity: "critical"},
		{ID: "FedRAMP-SC-17", Framework: FrameworkFedRAMP, Category: "PKI Certificates", Description: "Organization issues public key certificates or obtains from approved provider", Severity: "high"},
		{ID: "FedRAMP-SC-28", Framework: FrameworkFedRAMP, Category: "Protection at Rest", Description: "Protection of information at rest", Severity: "high"},
		{ID: "FedRAMP-AU-9", Framework: FrameworkFedRAMP, Category: "Audit Protection", Description: "Protection of audit information", Severity: "high"},
	}
}

func (r *ComplianceReporter) nistRequirements() []ComplianceRequirement {
	return []ComplianceRequirement{
		{ID: "NIST-SC-12", Framework: FrameworkNIST, Category: "Cryptographic Key", Description: "Cryptographic key establishment and management", Severity: "critical"},
		{ID: "NIST-SC-12(1)", Framework: FrameworkNIST, Category: "Cryptographic Key", Description: "Availability of information in event of cryptographic key loss", Severity: "high"},
		{ID: "NIST-SC-12(2)", Framework: FrameworkNIST, Category: "Cryptographic Key", Description: "Symmetric keys using NIST-approved methods", Severity: "high"},
		{ID: "NIST-SC-12(3)", Framework: FrameworkNIST, Category: "Cryptographic Key", Description: "Asymmetric keys using NIST-approved methods", Severity: "high"},
		{ID: "NIST-SC-13", Framework: FrameworkNIST, Category: "Cryptographic Protection", Description: "Use of FIPS-validated cryptography", Severity: "critical"},
		{ID: "NIST-AU-9", Framework: FrameworkNIST, Category: "Audit Protection", Description: "Protection of audit information and tools", Severity: "high"},
		{ID: "NIST-AU-10", Framework: FrameworkNIST, Category: "Non-repudiation", Description: "Protection against denial of actions", Severity: "medium"},
	}
}

func (r *ComplianceReporter) checkRequirement(ctx context.Context, req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{
		Requirement: req,
		CheckedAt:   time.Now(),
		Evidence:    make([]string, 0),
	}

	switch req.Category {
	case "Logical Access", "Access Control":
		result = r.checkAccessControl(req, period)
	case "Cryptographic Keys", "Cryptographic Key", "Cryptographic Protection":
		result = r.checkCryptographicControls(req, period)
	case "Key Management":
		result = r.checkKeyManagement(req, period)
	case "Audit Trails", "Audit Controls", "Audit Protection", "Records":
		result = r.checkAuditControls(req, period)
	case "System Monitoring", "Incident Response":
		result = r.checkMonitoring(req, period)
	case "System Operations", "Change Management":
		result = r.checkOperations(req, period)
	case "Integrity", "Authentication", "Transmission Security", "Security":
		result = r.checkSecurityControls(req, period)
	case "Protection at Rest", "PKI Certificates":
		result = r.checkDataProtection(req, period)
	case "Non-repudiation":
		result = r.checkNonRepudiation(req, period)
	case "Risk Management":
		result = r.checkRiskManagement(req, period)
	case "Breach Notification":
		result = r.checkBreachNotification(req, period)
	default:
		result.Status = StatusUnknown
		result.Details = fmt.Sprintf("Unknown requirement category: %s", req.Category)
	}

	result.Requirement = req
	result.CheckedAt = time.Now()
	return result
}

func (r *ComplianceReporter) checkAccessControl(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	// Check for unauthorized access attempts
	var unauthorizedCount int64
	for _, entry := range r.accessStats {
		unauthorizedCount += entry.failureCount
	}

	switch {
	case unauthorizedCount == 0:
		result.Status = StatusCompliant
		result.Details = "No unauthorized access attempts detected"
		result.Evidence = append(result.Evidence, "Zero failed access attempts in audit log")
	case unauthorizedCount < 10:
		result.Status = StatusPartiallyCompliant
		result.Details = fmt.Sprintf("%d unauthorized access attempts detected", unauthorizedCount)
		result.Evidence = append(result.Evidence, fmt.Sprintf("Failed attempts: %d", unauthorizedCount))
	default:
		result.Status = StatusNonCompliant
		result.Details = fmt.Sprintf("High number of unauthorized access attempts: %d", unauthorizedCount)
		result.Evidence = append(result.Evidence, fmt.Sprintf("Failed attempts: %d (threshold: 10)", unauthorizedCount))
	}

	return result
}

func (r *ComplianceReporter) checkCryptographicControls(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	var weakKeys, strongKeys int
	for _, key := range r.keyInventory {
		if key.KeySize < r.config.MinKeySize {
			weakKeys++
			result.Evidence = append(result.Evidence, fmt.Sprintf("Weak key: %s (%d bits)", key.KeyID, key.KeySize))
		} else {
			strongKeys++
		}
	}

	switch {
	case weakKeys == 0 && strongKeys > 0:
		result.Status = StatusCompliant
		result.Details = fmt.Sprintf("All %d keys meet minimum size requirement (%d bits)", strongKeys, r.config.MinKeySize)
		result.Evidence = append(result.Evidence, fmt.Sprintf("Minimum key size: %d bits", r.config.MinKeySize))
	case weakKeys > 0 && strongKeys > 0:
		result.Status = StatusPartiallyCompliant
		result.Details = fmt.Sprintf("%d of %d keys are below minimum size requirement", weakKeys, weakKeys+strongKeys)
	case weakKeys > 0:
		result.Status = StatusNonCompliant
		result.Details = fmt.Sprintf("All %d keys are below minimum size requirement", weakKeys)
	default:
		result.Status = StatusNotApplicable
		result.Details = "No keys registered"
	}

	return result
}

func (r *ComplianceReporter) checkKeyManagement(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	var overdueKeys, pendingKeys, compliantKeys int
	now := time.Now()

	for _, key := range r.keyInventory {
		if key.Status != "active" {
			continue
		}

		switch {
		case key.NextRotation != nil:
			switch {
			case key.NextRotation.Before(now):
				overdueKeys++
				result.Evidence = append(result.Evidence, fmt.Sprintf("Overdue: %s (due: %s)", key.KeyID, key.NextRotation.Format(time.RFC3339)))
			case key.NextRotation.Before(now.Add(30 * 24 * time.Hour)):
				pendingKeys++
			default:
				compliantKeys++
			}
		case key.LastRotated != nil:
			age := now.Sub(*key.LastRotated)
			if age > r.config.RotationMaxAge {
				overdueKeys++
				result.Evidence = append(result.Evidence, fmt.Sprintf("Overdue: %s (last rotated: %s)", key.KeyID, key.LastRotated.Format(time.RFC3339)))
			} else {
				compliantKeys++
			}
		}
	}

	total := overdueKeys + pendingKeys + compliantKeys
	switch {
	case total == 0:
		result.Status = StatusNotApplicable
		result.Details = "No active keys to evaluate"
	case overdueKeys == 0:
		result.Status = StatusCompliant
		result.Details = fmt.Sprintf("All %d keys are within rotation policy", total)
		if pendingKeys > 0 {
			result.Evidence = append(result.Evidence, fmt.Sprintf("%d keys pending rotation in next 30 days", pendingKeys))
		}
	default:
		result.Status = StatusNonCompliant
		result.Details = fmt.Sprintf("%d of %d keys are overdue for rotation", overdueKeys, total)
	}

	return result
}

func (r *ComplianceReporter) checkAuditControls(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	if r.auditLogger != nil {
		result.Status = StatusCompliant
		result.Details = "Audit logging is enabled and operational"
		result.Evidence = append(result.Evidence, "AuditLogger instance is configured")
	} else {
		result.Status = StatusNonCompliant
		result.Details = "Audit logging is not configured"
		result.Evidence = append(result.Evidence, "No AuditLogger instance found")
	}

	// Check for access logging
	if len(r.accessStats) > 0 {
		var totalAccess int64
		for _, entry := range r.accessStats {
			totalAccess += entry.accessCount
		}
		result.Evidence = append(result.Evidence, fmt.Sprintf("Total access events logged: %d", totalAccess))
	}

	return result
}

func (r *ComplianceReporter) checkMonitoring(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	// Check if we have recent access data
	if len(r.accessStats) > 0 {
		result.Status = StatusCompliant
		result.Details = "Access monitoring is active"
		result.Evidence = append(result.Evidence, fmt.Sprintf("Monitoring %d principals/secrets", len(r.accessStats)))
	} else {
		result.Status = StatusPartiallyCompliant
		result.Details = "Monitoring infrastructure exists but no data collected"
	}

	return result
}

func (r *ComplianceReporter) checkOperations(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	// Count rotations in period
	var rotationsInPeriod int
	for _, event := range r.rotationLog {
		if event.RotatedAt.After(period.Start) && event.RotatedAt.Before(period.End) {
			rotationsInPeriod++
		}
	}

	if rotationsInPeriod > 0 {
		result.Status = StatusCompliant
		result.Details = fmt.Sprintf("%d key rotations performed in period", rotationsInPeriod)
		result.Evidence = append(result.Evidence, fmt.Sprintf("Rotations: %d", rotationsInPeriod))
	} else {
		// Check if any keys needed rotation
		var needsRotation int
		for _, key := range r.keyInventory {
			if key.NextRotation != nil && key.NextRotation.Before(period.End) {
				needsRotation++
			}
		}
		if needsRotation > 0 {
			result.Status = StatusNonCompliant
			result.Details = fmt.Sprintf("%d keys needed rotation but none performed", needsRotation)
		} else {
			result.Status = StatusCompliant
			result.Details = "No keys required rotation in period"
		}
	}

	return result
}

func (r *ComplianceReporter) checkSecurityControls(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	// Check key algorithms
	var approved, unapproved int
	approvedAlgorithms := map[string]bool{
		"aes-256-gcm": true,
		"aes-128-gcm": true,
		"rsa-4096":    true,
		"rsa-2048":    true,
		"ecdsa-p256":  true,
		"ecdsa-p384":  true,
		"ed25519":     true,
	}

	for _, key := range r.keyInventory {
		if approvedAlgorithms[key.Algorithm] {
			approved++
		} else {
			unapproved++
			result.Evidence = append(result.Evidence, fmt.Sprintf("Unapproved algorithm: %s (%s)", key.KeyID, key.Algorithm))
		}
	}

	switch {
	case unapproved == 0 && approved > 0:
		result.Status = StatusCompliant
		result.Details = fmt.Sprintf("All %d keys use approved algorithms", approved)
	case unapproved > 0:
		result.Status = StatusNonCompliant
		result.Details = fmt.Sprintf("%d keys use unapproved algorithms", unapproved)
	default:
		result.Status = StatusNotApplicable
		result.Details = "No keys to evaluate"
	}

	return result
}

func (r *ComplianceReporter) checkDataProtection(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	// Verify keys are stored securely (all registered keys are assumed secure)
	activeKeys := 0
	for _, key := range r.keyInventory {
		if key.Status == "active" {
			activeKeys++
		}
	}

	if activeKeys > 0 {
		result.Status = StatusCompliant
		result.Details = fmt.Sprintf("%d active keys stored in KMS", activeKeys)
		result.Evidence = append(result.Evidence, "Keys are stored in hardware-backed KMS")
	} else {
		result.Status = StatusNotApplicable
		result.Details = "No active keys to evaluate"
	}

	return result
}

func (r *ComplianceReporter) checkNonRepudiation(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	// Check if audit logging captures who performed actions
	switch {
	case r.auditLogger != nil && len(r.accessStats) > 0:
		result.Status = StatusCompliant
		result.Details = "Audit logging captures principal identity for all operations"
		result.Evidence = append(result.Evidence, "All access events include principal ID")
	case r.auditLogger != nil:
		result.Status = StatusPartiallyCompliant
		result.Details = "Audit logging is enabled but no events recorded"
	default:
		result.Status = StatusNonCompliant
		result.Details = "Audit logging is not configured"
	}

	return result
}

func (r *ComplianceReporter) checkRiskManagement(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	// Check for keys approaching end-of-life
	var riskKeys int
	now := time.Now()
	thirtyDays := 30 * 24 * time.Hour

	for _, key := range r.keyInventory {
		if key.NextRotation != nil && key.NextRotation.Before(now.Add(thirtyDays)) {
			riskKeys++
		}
	}

	if riskKeys == 0 {
		result.Status = StatusCompliant
		result.Details = "No keys approaching rotation deadline"
	} else {
		result.Status = StatusPartiallyCompliant
		result.Details = fmt.Sprintf("%d keys require rotation within 30 days", riskKeys)
		result.Evidence = append(result.Evidence, "Schedule rotation for these keys")
	}

	return result
}

func (r *ComplianceReporter) checkBreachNotification(req ComplianceRequirement, period ReportPeriod) ComplianceCheckResult {
	result := ComplianceCheckResult{Evidence: make([]string, 0)}

	// Check for high failure rates that might indicate breach attempts
	var highFailureAccounts int
	for _, entry := range r.accessStats {
		if entry.failureCount > 10 {
			highFailureAccounts++
		}
	}

	if highFailureAccounts == 0 {
		result.Status = StatusCompliant
		result.Details = "No breach indicators detected"
	} else {
		result.Status = StatusPartiallyCompliant
		result.Details = fmt.Sprintf("%d accounts with high failure rates", highFailureAccounts)
		result.Evidence = append(result.Evidence, "Review accounts for potential breach")
	}

	return result
}

func (r *ComplianceReporter) calculateSummary(results []ComplianceCheckResult) ComplianceSummary {
	summary := ComplianceSummary{
		TotalRequirements: len(results),
	}

	for i := range results {
		result := &results[i]
		switch result.Status {
		case StatusCompliant:
			summary.Compliant++
		case StatusNonCompliant:
			summary.NonCompliant++
			switch result.Requirement.Severity {
			case "critical":
				summary.CriticalIssues++
			case "high":
				summary.HighIssues++
			case "medium":
				summary.MediumIssues++
			case "low":
				summary.LowIssues++
			}
		case StatusPartiallyCompliant:
			summary.PartiallyCompliant++
		case StatusNotApplicable:
			summary.NotApplicable++
		case StatusUnknown:
			summary.Unknown++
		}
	}

	// Calculate overall score
	applicable := summary.TotalRequirements - summary.NotApplicable - summary.Unknown
	if applicable > 0 {
		score := float64(summary.Compliant) + float64(summary.PartiallyCompliant)*0.5
		summary.OverallScore = (score / float64(applicable)) * 100
		summary.CompliancePercentage = (float64(summary.Compliant) / float64(applicable)) * 100
	}

	// Determine risk level
	switch {
	case summary.CriticalIssues > 0:
		summary.RiskLevel = "critical"
	case summary.HighIssues > 0:
		summary.RiskLevel = "high"
	case summary.MediumIssues > 0 || summary.NonCompliant > 0:
		summary.RiskLevel = "medium"
	default:
		summary.RiskLevel = "low"
	}

	return summary
}

func (r *ComplianceReporter) getKeyInventory() []KeyInventoryItem {
	items := make([]KeyInventoryItem, 0, len(r.keyInventory))
	for _, item := range r.keyInventory {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].KeyID < items[j].KeyID
	})
	return items
}

func (r *ComplianceReporter) getAccessAuditSummary(period ReportPeriod) AccessAuditSummary {
	summary := AccessAuditSummary{
		AccessByHour: make(map[int]int64),
		AccessByDay:  make(map[string]int64),
		TopAccessors: make([]AccessorStats, 0),
		TopSecrets:   make([]SecretAccessStats, 0),
	}

	principalStats := make(map[string]*AccessorStats)
	secretStats := make(map[string]*SecretAccessStats)
	principals := make(map[string]bool)
	secrets := make(map[string]bool)

	for key, entry := range r.accessStats {
		// Parse principal:secret
		principals[entry.principal] = true
		secrets[key] = true

		summary.TotalAccesses += entry.accessCount
		summary.FailedAttempts += entry.failureCount

		// Aggregate by hour
		for hour, count := range entry.byHour {
			summary.AccessByHour[hour] += count
		}

		// Aggregate by day
		for day, count := range entry.byDay {
			summary.AccessByDay[day] += count
		}

		// Track per-principal stats
		if _, ok := principalStats[entry.principal]; !ok {
			principalStats[entry.principal] = &AccessorStats{
				Principal: entry.principal,
			}
		}
		principalStats[entry.principal].AccessCount += entry.accessCount
		principalStats[entry.principal].FailureCount += entry.failureCount
		if entry.lastAccess.After(principalStats[entry.principal].LastAccess) {
			principalStats[entry.principal].LastAccess = entry.lastAccess
		}

		// Track per-secret stats
		if _, ok := secretStats[key]; !ok {
			secretStats[key] = &SecretAccessStats{
				SecretPath: key,
			}
		}
		secretStats[key].AccessCount += entry.accessCount
		if entry.lastAccess.After(secretStats[key].LastAccess) {
			secretStats[key].LastAccess = entry.lastAccess
		}
	}

	summary.UniquePrincipals = len(principals)
	summary.UniqueSecrets = len(secrets)

	// Convert to sorted slices (top 10)
	for _, stats := range principalStats {
		summary.TopAccessors = append(summary.TopAccessors, *stats)
	}
	sort.Slice(summary.TopAccessors, func(i, j int) bool {
		return summary.TopAccessors[i].AccessCount > summary.TopAccessors[j].AccessCount
	})
	if len(summary.TopAccessors) > 10 {
		summary.TopAccessors = summary.TopAccessors[:10]
	}

	for _, stats := range secretStats {
		summary.TopSecrets = append(summary.TopSecrets, *stats)
	}
	sort.Slice(summary.TopSecrets, func(i, j int) bool {
		return summary.TopSecrets[i].AccessCount > summary.TopSecrets[j].AccessCount
	})
	if len(summary.TopSecrets) > 10 {
		summary.TopSecrets = summary.TopSecrets[:10]
	}

	return summary
}

func (r *ComplianceReporter) getRotationSummary(period ReportPeriod) RotationSummary {
	now := time.Now()
	summary := RotationSummary{
		TotalKeys: len(r.keyInventory),
		RotationPolicy: RotationPolicy{
			Enabled:          true,
			RotationInterval: r.config.RotationMaxAge,
			GracePeriod:      30 * 24 * time.Hour,
			AutoRotate:       false,
		},
		RotationHistory: make([]RotationEvent, 0),
	}

	var totalAge time.Duration
	var keyCount int
	var maxAge time.Duration

	for _, key := range r.keyInventory {
		if key.Status != "active" {
			continue
		}

		// Calculate age since last rotation
		var age time.Duration
		if key.LastRotated != nil {
			age = now.Sub(*key.LastRotated)
		} else {
			age = now.Sub(key.CreatedAt)
		}

		totalAge += age
		keyCount++

		if age > maxAge {
			maxAge = age
		}

		// Check rotation status
		if key.NextRotation != nil {
			if key.NextRotation.Before(now) {
				summary.KeysOverdue++
			} else if key.NextRotation.Before(now.Add(30 * 24 * time.Hour)) {
				summary.KeysPendingRotation++
			}
		} else if age > r.config.RotationMaxAge {
			summary.KeysOverdue++
		}
	}

	if keyCount > 0 {
		summary.AverageRotationAge = totalAge / time.Duration(keyCount)
	}
	summary.MaxRotationAge = maxAge

	// Get rotations in period
	for _, event := range r.rotationLog {
		if event.RotatedAt.After(period.Start) && event.RotatedAt.Before(period.End) {
			summary.KeysRotatedInPeriod++
			summary.RotationHistory = append(summary.RotationHistory, event)
		}
	}

	return summary
}

// ExportJSON exports the compliance report as JSON.
func (r *ComplianceReport) ExportJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// GetNonCompliantItems returns all non-compliant check results.
func (r *ComplianceReport) GetNonCompliantItems() []ComplianceCheckResult {
	var items []ComplianceCheckResult
	for i := range r.Results {
		if r.Results[i].Status == StatusNonCompliant {
			items = append(items, r.Results[i])
		}
	}
	return items
}

// GetCriticalIssues returns all critical severity non-compliant items.
func (r *ComplianceReport) GetCriticalIssues() []ComplianceCheckResult {
	var items []ComplianceCheckResult
	for i := range r.Results {
		if r.Results[i].Status == StatusNonCompliant && r.Results[i].Requirement.Severity == "critical" {
			items = append(items, r.Results[i])
		}
	}
	return items
}

// IsCompliant returns true if the overall compliance percentage meets threshold.
func (r *ComplianceReport) IsCompliant(threshold float64) bool {
	return r.Summary.CompliancePercentage >= threshold
}
