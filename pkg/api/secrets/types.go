// Package secrets provides HTTP handlers for secrets management REST API endpoints.
package secrets

import "time"

// SecretResponse is the response for GET /api/v1/secrets/{path}.
type SecretResponse struct {
	Path      string                 `json:"path"`
	Backend   string                 `json:"backend"`
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Metadata  map[string]string      `json:"metadata,omitempty"`
	Version   int                    `json:"version,omitempty"`
	CreatedAt time.Time              `json:"created_at,omitempty"`
	ExpiresAt time.Time              `json:"expires_at,omitempty"`
	Renewable bool                   `json:"renewable,omitempty"`
	LeaseID   string                 `json:"lease_id,omitempty"`
}

// SecretListResponse is the response for GET /api/v1/secrets/{path}?list=true.
type SecretListResponse struct {
	Path string   `json:"path"`
	Keys []string `json:"keys"`
}

// SecretWriteRequest is the request body for POST /api/v1/secrets/{path}.
type SecretWriteRequest struct {
	Data     map[string]interface{} `json:"data"`
	Metadata map[string]string      `json:"metadata,omitempty"`
}

// LeaseResponse is the response for lease endpoints.
type LeaseResponse struct {
	ID           string            `json:"id"`
	SecretPath   string            `json:"secret_path"`
	Backend      string            `json:"backend"`
	State        string            `json:"state"`
	TTLSeconds   int64             `json:"ttl_seconds"`
	MaxTTLSeconds int64            `json:"max_ttl_seconds,omitempty"`
	IssuedAt     time.Time         `json:"issued_at"`
	ExpiresAt    time.Time         `json:"expires_at"`
	LastRenewal  time.Time         `json:"last_renewal,omitempty"`
	RenewalCount int               `json:"renewal_count"`
	Renewable    bool              `json:"renewable"`
	Revocable    bool              `json:"revocable"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// LeaseListResponse is the response for GET /api/v1/leases.
type LeaseListResponse struct {
	Leases []LeaseResponse `json:"leases"`
	Total  int             `json:"total"`
}

// LeaseRenewRequest is the request body for POST /api/v1/leases/{id}/renew.
type LeaseRenewRequest struct {
	IncrementSeconds int64 `json:"increment_seconds"`
}

// BulkRevokeRequest is the request body for POST /api/v1/leases/bulk-revoke.
type BulkRevokeRequest struct {
	Path string `json:"path,omitempty"`
}

// BulkRevokeResponse is the response for POST /api/v1/leases/bulk-revoke.
type BulkRevokeResponse struct {
	Revoked int    `json:"revoked"`
	Path    string `json:"path,omitempty"`
}

// StartRotationRequest is the request body for POST /api/v1/rotations.
type StartRotationRequest struct {
	ID         string            `json:"id"`
	SecretPath string            `json:"secret_path"`
	Strategy   string            `json:"strategy"`
	Targets    []RotationTargetRequest `json:"targets"`
	Config     RotationConfigRequest   `json:"config"`
}

// RotationTargetRequest is a target in a rotation start request.
type RotationTargetRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	AgentID  string `json:"agent_id,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

// RotationConfigRequest is the config in a rotation start request.
type RotationConfigRequest struct {
	Strategy          string `json:"strategy"`
	BatchSize         int    `json:"batch_size,omitempty"`
	BatchDelaySeconds int64  `json:"batch_delay_seconds,omitempty"`
	RollbackOnFailure bool   `json:"rollback_on_failure,omitempty"`
	FailureThreshold  float64 `json:"failure_threshold,omitempty"`
	GracePeriodSeconds int64  `json:"grace_period_seconds,omitempty"`
}

// RotationResponse is the response for rotation endpoints.
type RotationResponse struct {
	ID             string    `json:"id"`
	SecretPath     string    `json:"secret_path"`
	State          string    `json:"state"`
	Strategy       string    `json:"strategy"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
	TotalTargets   int       `json:"total_targets"`
	UpdatedTargets int       `json:"updated_targets"`
	FailedTargets  int       `json:"failed_targets"`
	Error          string    `json:"error,omitempty"`
}

// RotationListResponse is the response for GET /api/v1/rotations.
type RotationListResponse struct {
	Rotations []RotationResponse `json:"rotations"`
	Total     int                `json:"total"`
}

// RotationActionResponse is the response for rotation action endpoints (pause, resume, trigger, cancel, rollback).
type RotationActionResponse struct {
	RotationID string `json:"rotation_id"`
	Action     string `json:"action"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
}

// RotationPolicyResponse is the response for rotation policy endpoints.
type RotationPolicyResponse struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	CredentialTypes []string `json:"credential_types"`
	MaxAge         string   `json:"max_age"`
	WarningAge     string   `json:"warning_age,omitempty"`
	Schedule       string   `json:"schedule,omitempty"`
	AutoRotate     bool     `json:"auto_rotate"`
	RollbackOnFail bool     `json:"rollback_on_fail"`
	Enabled        bool     `json:"enabled"`
}

// RotationPolicyListResponse is the response for GET /api/v1/secrets/rotation/policies.
type RotationPolicyListResponse struct {
	Policies []RotationPolicyResponse `json:"policies"`
	Total    int                      `json:"total"`
}

// CreateRotationPolicyRequest is the request for creating a rotation policy.
type CreateRotationPolicyRequest struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	CredentialTypes []string `json:"credential_types"`
	MaxAge         string   `json:"max_age"`
	WarningAge     string   `json:"warning_age,omitempty"`
	Schedule       string   `json:"schedule,omitempty"`
	AutoRotate     bool     `json:"auto_rotate"`
	RollbackOnFail bool     `json:"rollback_on_fail"`
	Enabled        bool     `json:"enabled"`
}

// RotationPolicyActionResponse is the response for policy enable/disable/delete.
type RotationPolicyActionResponse struct {
	PolicyID string `json:"policy_id"`
	Action   string `json:"action"`
	Success  bool   `json:"success"`
}

// TransitEncryptRequest is the request body for transit encrypt.
type TransitEncryptRequest struct {
	Plaintext  []byte `json:"plaintext"`
	Context    []byte `json:"context,omitempty"`
	KeyVersion int    `json:"key_version,omitempty"`
}

// TransitDecryptRequest is the request body for transit decrypt.
type TransitDecryptRequest struct {
	Ciphertext string `json:"ciphertext"`
	Context    []byte `json:"context,omitempty"`
}

// TransitBatchEncryptRequest is the request for batch encrypt.
type TransitBatchEncryptRequest struct {
	Items []TransitEncryptRequest `json:"items"`
}

// TransitBatchDecryptRequest is the request for batch decrypt.
type TransitBatchDecryptRequest struct {
	Items []TransitDecryptRequest `json:"items"`
}

// TransitSignRequest is the request body for transit sign.
type TransitSignRequest struct {
	Input      []byte `json:"input"`
	Algorithm  string `json:"algorithm,omitempty"`
	KeyVersion int    `json:"key_version,omitempty"`
}

// TransitVerifyRequest is the request body for transit verify.
type TransitVerifyRequest struct {
	Input     []byte `json:"input"`
	Signature string `json:"signature"`
	Algorithm string `json:"algorithm,omitempty"`
}

// TransitHMACRequest is the request body for transit HMAC.
type TransitHMACRequest struct {
	Input     []byte `json:"input"`
	Algorithm string `json:"algorithm,omitempty"`
}

// TransitEncryptResponse is the response for encrypt operations.
type TransitEncryptResponse struct {
	Ciphertext string `json:"ciphertext"`
	KeyVersion int    `json:"key_version,omitempty"`
}

// TransitDecryptResponse is the response for decrypt operations.
type TransitDecryptResponse struct {
	Plaintext  []byte `json:"plaintext"`
	KeyVersion int    `json:"key_version,omitempty"`
}

// TransitBatchEncryptResponse is the response for batch encrypt.
type TransitBatchEncryptResponse struct {
	Results []TransitEncryptResponse `json:"results"`
}

// TransitBatchDecryptResponse is the response for batch decrypt.
type TransitBatchDecryptResponse struct {
	Results []TransitDecryptResponse `json:"results"`
}

// TransitSignResponse is the response for sign operations.
type TransitSignResponse struct {
	Signature  string `json:"signature"`
	KeyVersion int    `json:"key_version,omitempty"`
}

// TransitVerifyResponse is the response for verify operations.
type TransitVerifyResponse struct {
	Valid bool `json:"valid"`
}

// TransitHMACResponse is the response for HMAC operations.
type TransitHMACResponse struct {
	HMAC string `json:"hmac"`
}

// TransitRewrapRequest is the request body for transit rewrap.
type TransitRewrapRequest struct {
	Ciphertext string `json:"ciphertext"`
	Context    []byte `json:"context,omitempty"`
}

// TransitRewrapResponse is the response for rewrap operations.
type TransitRewrapResponse struct {
	Ciphertext string `json:"ciphertext"`
	KeyVersion int    `json:"key_version,omitempty"`
}

// ComplianceReportRequest is the request for compliance report generation.
type ComplianceReportRequest struct {
	Type string `json:"type"`
}

// ComplianceReportResponse is the response for compliance reports.
type ComplianceReportResponse struct {
	Message string `json:"message"`
}

// AuditLogResponse is the response for audit log queries.
type AuditLogResponse struct {
	Events []*AuditEventResponse `json:"events"`
	Total  int                   `json:"total"`
}

// AuditEventResponse is a single audit event in the response.
type AuditEventResponse struct {
	SecretPath string                 `json:"secret_path"`
	Backend    string                 `json:"backend,omitempty"`
	SecretType string                 `json:"secret_type,omitempty"`
	AgentID    string                 `json:"agent_id,omitempty"`
	SPIFFEID   string                 `json:"spiffe_id,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
	LeaseID    string                 `json:"lease_id,omitempty"`
	Action     string                 `json:"action"`
	Timestamp  time.Time              `json:"timestamp"`
	DurationMS int64                  `json:"duration_ms,omitempty"`
	CacheHit   bool                   `json:"cache_hit,omitempty"`
	Success    bool                   `json:"success"`
	Error      string                 `json:"error,omitempty"`
	SourceIP   string                 `json:"source_ip,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

// HealthResponse is the response for GET /api/v1/health/secrets.
type HealthResponse struct {
	Healthy       bool            `json:"healthy"`
	BackendHealth map[string]bool `json:"backend_health"`
	BackendCount  int             `json:"backend_count"`
	CacheStats    interface{}     `json:"cache_stats,omitempty"`
	LeaseStats    interface{}     `json:"lease_stats,omitempty"`
}

// BackendInfoResponse is the response for a single backend.
type BackendInfoResponse struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Healthy bool   `json:"healthy"`
}

// BackendListResponse is the response for GET /api/v1/secrets/backends.
type BackendListResponse struct {
	Backends []*BackendInfoResponse `json:"backends"`
	Total    int                    `json:"total"`
}

// CacheStatsResponse is the response for GET /api/v1/secrets/cache/stats.
type CacheStatsResponse struct {
	Entries      int   `json:"entries"`
	MaxEntries   int   `json:"max_entries"`
	Hits         int64 `json:"hits"`
	Misses       int64 `json:"misses"`
	Evictions    int64 `json:"evictions"`
	ExpiredCount int64 `json:"expired_count"`
	MemoryBytes  int64 `json:"memory_bytes"`
}

// CacheClearResponse is the response for DELETE /api/v1/secrets/cache.
type CacheClearResponse struct {
	Message string `json:"message"`
	Cleared int    `json:"cleared"`
}
