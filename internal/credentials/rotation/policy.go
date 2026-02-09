package rotation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

// PolicyEngine evaluates credentials against rotation policies to determine
// which credentials need rotation.
type PolicyEngine struct {
	mu       sync.RWMutex
	policies map[string]*Policy
}

// NewPolicyEngine creates a new policy engine.
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{
		policies: make(map[string]*Policy),
	}
}

// AddPolicy adds a rotation policy.
func (pe *PolicyEngine) AddPolicy(policy *Policy) error {
	if policy.ID == "" {
		return fmt.Errorf("policy ID is required")
	}
	if policy.MaxAge <= 0 {
		return fmt.Errorf("max age must be positive")
	}
	if len(policy.CredentialTypes) == 0 {
		return fmt.Errorf("at least one credential type is required")
	}

	pe.mu.Lock()
	defer pe.mu.Unlock()

	if _, exists := pe.policies[policy.ID]; exists {
		return fmt.Errorf("policy %s already exists", policy.ID)
	}

	pe.policies[policy.ID] = policy
	return nil
}

// UpdatePolicy updates an existing policy.
func (pe *PolicyEngine) UpdatePolicy(policy *Policy) error {
	if policy.ID == "" {
		return fmt.Errorf("policy ID is required")
	}

	pe.mu.Lock()
	defer pe.mu.Unlock()

	if _, exists := pe.policies[policy.ID]; !exists {
		return fmt.Errorf("policy %s not found", policy.ID)
	}

	pe.policies[policy.ID] = policy
	return nil
}

// RemovePolicy removes a policy by ID.
func (pe *PolicyEngine) RemovePolicy(id string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	if _, exists := pe.policies[id]; !exists {
		return fmt.Errorf("policy %s not found", id)
	}

	delete(pe.policies, id)
	return nil
}

// GetPolicy returns a policy by ID.
func (pe *PolicyEngine) GetPolicy(id string) (*Policy, bool) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	p, ok := pe.policies[id]
	return p, ok
}

// ListPolicies returns all policies.
func (pe *PolicyEngine) ListPolicies() []*Policy {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	result := make([]*Policy, 0, len(pe.policies))
	for _, p := range pe.policies {
		result = append(result, p)
	}
	return result
}

// Evaluate checks a credential against all matching policies and returns the
// most urgent result.
func (pe *PolicyEngine) Evaluate(cred credentials.Credential, now time.Time) *PolicyResult {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	var mostUrgent *PolicyResult

	for _, policy := range pe.policies {
		if !policy.Enabled {
			continue
		}
		if !policyMatchesType(policy, cred.Type()) {
			continue
		}

		result := evaluateCredential(policy, cred, now)
		if mostUrgent == nil || actionPriority(result.Action) > actionPriority(mostUrgent.Action) {
			mostUrgent = result
		}
	}

	if mostUrgent == nil {
		return &PolicyResult{
			CredentialID: cred.ID(),
			Action:       PolicyActionOK,
			Message:      "no matching policy",
		}
	}

	return mostUrgent
}

// FindDueCredentials scans the credential store and returns jobs for credentials
// that need rotation according to enabled policies.
func (pe *PolicyEngine) FindDueCredentials(ctx context.Context, store credentials.CredentialStore, now time.Time) ([]*Job, error) {
	ids, err := store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list credentials: %w", err)
	}

	var jobs []*Job

	for _, id := range ids {
		cred, err := store.Get(ctx, id)
		if err != nil {
			continue // Skip credentials that can't be read (expired, etc.)
		}

		result := pe.Evaluate(cred, now)
		if result.Action == PolicyActionRotate || result.Action == PolicyActionExpired {
			policy, ok := pe.GetPolicy(result.PolicyID)
			rollback := true
			if ok {
				rollback = policy.RollbackOnFail
			}

			jobs = append(jobs, &Job{
				ID:             fmt.Sprintf("auto-%s-%d", id, now.UnixNano()),
				CredentialID:   id,
				CredentialType: cred.Type(),
				RollbackOnFail: rollback,
				OldCredential:  cred,
			})
		}
	}

	return jobs, nil
}

func policyMatchesType(policy *Policy, credType credentials.CredentialType) bool {
	for _, t := range policy.CredentialTypes {
		if t == credType {
			return true
		}
	}
	return false
}

func evaluateCredential(policy *Policy, cred credentials.Credential, now time.Time) *PolicyResult {
	result := &PolicyResult{
		PolicyID:     policy.ID,
		CredentialID: cred.ID(),
		MaxAge:       policy.MaxAge,
	}

	// Check if the credential has a creation/update time via BaseCredential
	var createdAt time.Time
	switch c := cred.(type) {
	case *credentials.SSHPasswordCredential:
		createdAt = c.CreatedAt
	case *credentials.SSHKeyCredential:
		createdAt = c.CreatedAt
	case *credentials.SNMPv2cCredential:
		createdAt = c.CreatedAt
	case *credentials.SNMPv3Credential:
		createdAt = c.CreatedAt
	case *credentials.WinRMCredential:
		createdAt = c.CreatedAt
	case *credentials.RESTBasicCredential:
		createdAt = c.CreatedAt
	case *credentials.RESTBearerCredential:
		createdAt = c.CreatedAt
	case *credentials.RESTAPIKeyCredential:
		createdAt = c.CreatedAt
	case *credentials.RESTOAuth2Credential:
		createdAt = c.CreatedAt
	case *credentials.GNMICredential:
		createdAt = c.CreatedAt
	}

	// If credential is already expired
	if cred.IsExpired() {
		result.Action = PolicyActionExpired
		result.Message = "credential has expired"
		return result
	}

	// If no creation time, we can't determine age
	if createdAt.IsZero() {
		result.Action = PolicyActionOK
		result.Message = "no creation time available"
		return result
	}

	age := now.Sub(createdAt)
	result.Age = age

	if age >= policy.MaxAge {
		result.Action = PolicyActionRotate
		result.Message = fmt.Sprintf("credential age %s exceeds max age %s", age.Round(time.Hour), policy.MaxAge.Round(time.Hour))
		return result
	}

	if policy.WarningAge > 0 && age >= policy.WarningAge {
		result.Action = PolicyActionWarning
		result.Message = fmt.Sprintf("credential age %s exceeds warning threshold %s", age.Round(time.Hour), policy.WarningAge.Round(time.Hour))
		return result
	}

	result.Action = PolicyActionOK
	result.Message = fmt.Sprintf("credential age %s within policy limits", age.Round(time.Hour))
	return result
}

func actionPriority(action PolicyAction) int {
	switch action {
	case PolicyActionExpired:
		return 4
	case PolicyActionRotate:
		return 3
	case PolicyActionWarning:
		return 2
	case PolicyActionOK:
		return 1
	default:
		return 0
	}
}
