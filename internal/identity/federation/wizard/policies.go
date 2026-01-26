// Package wizard provides an interactive TUI for trust federation setup.
package wizard

import (
	"path/filepath"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/identity/federation"
)

// PolicyTemplate represents a pre-defined trust policy configuration.
type PolicyTemplate struct {
	// Name is the template identifier.
	Name string

	// DisplayName is the human-readable name.
	DisplayName string

	// Description explains what the policy allows.
	Description string

	// Policy is the actual trust policy configuration.
	// Nil for "custom" template which is built interactively.
	Policy *federation.TrustPolicy

	// Recommended indicates if this is the recommended choice.
	Recommended bool
}

// PolicyTemplates contains the available policy templates.
var PolicyTemplates = []PolicyTemplate{
	{
		Name:        "services-only",
		DisplayName: "Services Only",
		Description: "Allow /service/** paths, deny /admin/** and /internal/**",
		Recommended: true,
		Policy: &federation.TrustPolicy{
			Name:         "services-only",
			Description:  "Only allow service identities from the federated domain",
			AllowedPaths: []string{"/service/**"},
			DeniedPaths:  []string{"/admin/**", "/internal/**"},
			RequireMTLS:  true,
			AuditLevel:   "standard",
		},
	},
	{
		Name:        "allow-all",
		DisplayName: "Allow All",
		Description: "Trust all identities from the partner domain",
		Policy: &federation.TrustPolicy{
			Name:         "allow-all",
			Description:  "Trust all identities from the federated domain",
			AllowedPaths: []string{"/**"},
			AuditLevel:   "standard",
		},
	},
	{
		Name:        "agents-only",
		DisplayName: "Agents Only",
		Description: "Only allow /agent/** paths",
		Policy: &federation.TrustPolicy{
			Name:         "agents-only",
			Description:  "Only allow agent identities from the federated domain",
			AllowedPaths: []string{"/agent/**"},
			DeniedPaths:  []string{"/service/**", "/admin/**"},
			RequireMTLS:  true,
			AuditLevel:   "standard",
		},
	},
	{
		Name:        "kubernetes",
		DisplayName: "Kubernetes Workloads",
		Description: "Allow Kubernetes service account paths (/ns/*/sa/*)",
		Policy: &federation.TrustPolicy{
			Name:         "kubernetes",
			Description:  "Allow Kubernetes workload identities",
			AllowedPaths: []string{"/ns/*/sa/*"},
			DeniedPaths:  []string{"/ns/kube-system/**", "/admin/**"},
			RequireMTLS:  true,
			AuditLevel:   "standard",
		},
	},
	{
		Name:        "custom",
		DisplayName: "Custom",
		Description: "Define your own allowed/denied paths",
		Policy:      nil, // Built interactively
	},
}

// GetPolicyTemplate returns a policy template by name.
func GetPolicyTemplate(name string) *PolicyTemplate {
	for i := range PolicyTemplates {
		if PolicyTemplates[i].Name == name {
			return &PolicyTemplates[i]
		}
	}
	return nil
}

// PolicyTestResult represents the result of testing a policy against a SPIFFE ID.
type PolicyTestResult struct {
	// SPIFFEID is the tested SPIFFE ID.
	SPIFFEID string

	// Allowed indicates whether the identity would be allowed.
	Allowed bool

	// Reason explains why the identity was allowed or denied.
	Reason string

	// MatchedRule is the rule that matched (if any).
	MatchedRule string
}

// TestPolicy tests a policy against a SPIFFE ID.
func TestPolicy(policy *federation.TrustPolicy, spiffeID string) PolicyTestResult {
	result := PolicyTestResult{
		SPIFFEID: spiffeID,
	}

	// Parse SPIFFE ID to extract path
	path := extractPath(spiffeID)
	if path == "" {
		result.Allowed = false
		result.Reason = "Invalid SPIFFE ID format"
		return result
	}

	// Check denied paths first (takes precedence)
	for _, pattern := range policy.DeniedPaths {
		if matchPath(path, pattern) {
			result.Allowed = false
			result.Reason = "Denied by policy"
			result.MatchedRule = "denied: " + pattern
			return result
		}
	}

	// Check allowed paths
	if len(policy.AllowedPaths) > 0 {
		for _, pattern := range policy.AllowedPaths {
			if matchPath(path, pattern) {
				result.Allowed = true
				result.Reason = "Allowed by policy"
				result.MatchedRule = "allowed: " + pattern
				return result
			}
		}
		// No allowed path matched
		result.Allowed = false
		result.Reason = "No matching allow rule"
		return result
	}

	// If no allow rules specified, allow by default
	result.Allowed = true
	result.Reason = "No restrictions configured"
	return result
}

// extractPath extracts the path component from a SPIFFE ID.
// Example: spiffe://example.org/service/api -> /service/api
func extractPath(spiffeID string) string {
	// Remove scheme
	if !strings.HasPrefix(spiffeID, "spiffe://") {
		return ""
	}
	rest := strings.TrimPrefix(spiffeID, "spiffe://")

	// Find trust domain / path separator
	idx := strings.Index(rest, "/")
	if idx == -1 {
		return "/" // Root path
	}

	return rest[idx:]
}

// matchPath checks if a path matches a pattern.
// Supports glob patterns:
// - "*" matches any single path segment
// - "**" matches any number of path segments
func matchPath(path, pattern string) bool {
	// Handle special cases
	if pattern == "*" || pattern == "/**" {
		return true
	}

	// If pattern contains wildcards, use segment-based matching
	if strings.Contains(pattern, "*") {
		// Split pattern into segments
		patternParts := strings.Split(pattern, "/")
		pathParts := strings.Split(path, "/")

		return matchPathParts(pathParts, patternParts)
	}

	// Try exact match or filepath.Match
	matched, _ := filepath.Match(pattern, path)
	return matched || pattern == path
}

// matchPathParts matches path segments against pattern segments.
func matchPathParts(pathParts, patternParts []string) bool {
	pi := 0 // path index
	pp := 0 // pattern index

	for pp < len(patternParts) {
		if pi >= len(pathParts) {
			// No more path parts
			// Check if remaining pattern is all **
			for pp < len(patternParts) {
				if patternParts[pp] != "**" {
					return false
				}
				pp++
			}
			return true
		}

		pattern := patternParts[pp]

		switch pattern {
		case "**":
			// ** matches zero or more segments
			if pp == len(patternParts)-1 {
				// ** at end matches everything
				return true
			}
			// Try to match remaining pattern against remaining path
			for i := pi; i <= len(pathParts); i++ {
				if matchPathParts(pathParts[i:], patternParts[pp+1:]) {
					return true
				}
			}
			return false
		case "*":
			// * matches exactly one segment
			pi++
			pp++
		default:
			// Literal match (may contain single * within segment)
			if matched, _ := filepath.Match(pattern, pathParts[pi]); matched {
				pi++
				pp++
			} else {
				return false
			}
		}
	}

	// Pattern exhausted, path must also be exhausted
	return pi == len(pathParts)
}

// BuildCustomPolicy creates a policy from user-provided paths.
func BuildCustomPolicy(name, description string, allowedPaths, deniedPaths []string, requireMTLS bool) *federation.TrustPolicy {
	return &federation.TrustPolicy{
		Name:         name,
		Description:  description,
		AllowedPaths: allowedPaths,
		DeniedPaths:  deniedPaths,
		RequireMTLS:  requireMTLS,
		AuditLevel:   "standard",
	}
}

// ValidatePolicyPaths validates that policy paths are well-formed.
func ValidatePolicyPaths(paths []string) []string {
	var errors []string

	for _, path := range paths {
		// Path should start with /
		if !strings.HasPrefix(path, "/") {
			errors = append(errors, "Path must start with /: "+path)
			continue
		}

		// Check for invalid patterns
		if strings.Contains(path, "***") {
			errors = append(errors, "Invalid pattern (***): "+path)
		}

		// Check for empty segments
		if strings.Contains(path, "//") {
			errors = append(errors, "Path contains empty segment (//): "+path)
		}
	}

	return errors
}
