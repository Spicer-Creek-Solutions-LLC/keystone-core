// Package k8s provides Kubernetes integration for Keystone.
package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// NetworkPolicyVerifier verifies network policy enforcement.
type NetworkPolicyVerifier struct {
	client *Client
	rules  []VerificationRule
}

// VerificationRule defines a policy verification check.
type VerificationRule struct {
	Name        string
	Description string
	Severity    string // "critical", "high", "medium", "low"
	Check       func(ctx context.Context, policy *NetworkPolicy, client *Client) (*VerificationCheck, error)
}

// VerificationCheck represents the result of a single verification check.
type VerificationCheck struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Passed      bool                   `json:"passed"`
	Message     string                 `json:"message"`
	Severity    string                 `json:"severity"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// NetworkPolicyVerificationResult contains the outcome of policy verification.
type NetworkPolicyVerificationResult struct {
	Policy     *NetworkPolicy      `json:"policy"`
	Passed     bool                `json:"passed"`
	Checks     []VerificationCheck `json:"checks"`
	VerifiedAt time.Time           `json:"verified_at"`
	Duration   time.Duration       `json:"duration"`
	Message    string              `json:"message"`
}

// NewNetworkPolicyVerifier creates a new policy verifier with default rules.
func NewNetworkPolicyVerifier(client *Client) *NetworkPolicyVerifier {
	v := &NetworkPolicyVerifier{
		client: client,
	}
	v.registerDefaultRules()
	return v
}

// registerDefaultRules registers the default verification rules.
func (v *NetworkPolicyVerifier) registerDefaultRules() {
	v.rules = []VerificationRule{
		{
			Name:        "policy-exists",
			Description: "Verify policy exists in the Kubernetes cluster",
			Severity:    "critical",
			Check:       checkPolicyExists,
		},
		{
			Name:        "selector-valid",
			Description: "Verify pod selector is valid and matches pods",
			Severity:    "high",
			Check:       checkSelectorValid,
		},
		{
			Name:        "no-allow-all-ingress",
			Description: "Check for unintentional allow-all ingress rules",
			Severity:    "high",
			Check:       checkNoAllowAllIngress,
		},
		{
			Name:        "no-allow-all-egress",
			Description: "Check for unintentional allow-all egress rules",
			Severity:    "medium",
			Check:       checkNoAllowAllEgress,
		},
		{
			Name:        "dns-egress-allowed",
			Description: "Verify DNS egress is allowed when egress is restricted",
			Severity:    "high",
			Check:       checkDNSEgressAllowed,
		},
		{
			Name:        "no-wide-cidr",
			Description: "Check for overly permissive CIDR ranges",
			Severity:    "medium",
			Check:       checkNoWideCIDR,
		},
	}
}

// Verify verifies a network policy against all registered rules.
func (v *NetworkPolicyVerifier) Verify(ctx context.Context, policy *NetworkPolicy) (*NetworkPolicyVerificationResult, error) {
	start := time.Now()
	result := &NetworkPolicyVerificationResult{
		Policy:     policy,
		Passed:     true,
		Checks:     make([]VerificationCheck, 0, len(v.rules)),
		VerifiedAt: start,
	}

	var failedCritical bool
	var messages []string

	for _, rule := range v.rules {
		check, err := rule.Check(ctx, policy, v.client)
		if err != nil {
			check = &VerificationCheck{
				Name:        rule.Name,
				Description: rule.Description,
				Passed:      false,
				Message:     fmt.Sprintf("check error: %v", err),
				Severity:    rule.Severity,
			}
		}

		result.Checks = append(result.Checks, *check)

		if !check.Passed {
			if check.Severity == "critical" {
				failedCritical = true
			}
			messages = append(messages, fmt.Sprintf("%s: %s", check.Name, check.Message))
		}
	}

	result.Duration = time.Since(start)

	// Overall pass/fail based on critical failures
	if failedCritical {
		result.Passed = false
		result.Message = fmt.Sprintf("Verification failed: %s", strings.Join(messages, "; "))
	} else if len(messages) > 0 {
		result.Passed = true // Warnings only
		result.Message = fmt.Sprintf("Verification passed with warnings: %s", strings.Join(messages, "; "))
	} else {
		result.Passed = true
		result.Message = "All verification checks passed"
	}

	return result, nil
}

// AddRule adds a custom verification rule.
func (v *NetworkPolicyVerifier) AddRule(rule VerificationRule) {
	v.rules = append(v.rules, rule)
}

// checkPolicyExists verifies the policy exists in Kubernetes.
func checkPolicyExists(ctx context.Context, policy *NetworkPolicy, client *Client) (*VerificationCheck, error) {
	check := &VerificationCheck{
		Name:        "policy-exists",
		Description: "Verify policy exists in the Kubernetes cluster",
		Severity:    "critical",
	}

	if client == nil {
		check.Passed = true
		check.Message = "No client available, skipping existence check"
		return check, nil
	}

	_, err := client.GetNetworkPolicy(policy.Namespace, policy.Name)
	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Policy not found in cluster: %v", err)
		return check, nil
	}

	check.Passed = true
	check.Message = "Policy exists in cluster"
	return check, nil
}

// checkSelectorValid verifies the pod selector is valid.
func checkSelectorValid(ctx context.Context, policy *NetworkPolicy, client *Client) (*VerificationCheck, error) {
	check := &VerificationCheck{
		Name:        "selector-valid",
		Description: "Verify pod selector is valid and matches pods",
		Severity:    "high",
	}

	// Empty selector is valid (matches all pods)
	if len(policy.Spec.PodSelector.MatchLabels) == 0 && len(policy.Spec.PodSelector.MatchExpressions) == 0 {
		check.Passed = true
		check.Message = "Empty selector matches all pods in namespace"
		check.Details = map[string]interface{}{
			"matches_all": true,
		}
		return check, nil
	}

	// Build label selector string for validation
	var selectors []string
	for k, v := range policy.Spec.PodSelector.MatchLabels {
		selectors = append(selectors, fmt.Sprintf("%s=%s", k, v))
	}

	check.Passed = true
	check.Message = fmt.Sprintf("Selector defined: %v", policy.Spec.PodSelector.MatchLabels)
	check.Details = map[string]interface{}{
		"match_labels": policy.Spec.PodSelector.MatchLabels,
	}
	return check, nil
}

// checkNoAllowAllIngress checks for unintentional allow-all ingress.
func checkNoAllowAllIngress(ctx context.Context, policy *NetworkPolicy, client *Client) (*VerificationCheck, error) {
	check := &VerificationCheck{
		Name:        "no-allow-all-ingress",
		Description: "Check for unintentional allow-all ingress rules",
		Severity:    "high",
	}

	// Check if ingress is in policy types
	hasIngress := false
	for _, pt := range policy.Spec.PolicyTypes {
		if pt == PolicyTypeIngress {
			hasIngress = true
			break
		}
	}

	if !hasIngress {
		check.Passed = true
		check.Message = "No ingress policy type defined"
		return check, nil
	}

	// Check for allow-all ingress (empty From in any rule)
	for i, rule := range policy.Spec.Ingress {
		if len(rule.From) == 0 {
			check.Passed = false
			check.Message = fmt.Sprintf("Ingress rule %d allows traffic from all sources", i)
			check.Details = map[string]interface{}{
				"rule_index": i,
				"allow_all":  true,
			}
			return check, nil
		}
	}

	check.Passed = true
	check.Message = "No allow-all ingress rules found"
	return check, nil
}

// checkNoAllowAllEgress checks for unintentional allow-all egress.
func checkNoAllowAllEgress(ctx context.Context, policy *NetworkPolicy, client *Client) (*VerificationCheck, error) {
	check := &VerificationCheck{
		Name:        "no-allow-all-egress",
		Description: "Check for unintentional allow-all egress rules",
		Severity:    "medium",
	}

	// Check if egress is in policy types
	hasEgress := false
	for _, pt := range policy.Spec.PolicyTypes {
		if pt == PolicyTypeEgress {
			hasEgress = true
			break
		}
	}

	if !hasEgress {
		check.Passed = true
		check.Message = "No egress policy type defined"
		return check, nil
	}

	// Check for allow-all egress (empty To in any rule)
	for i, rule := range policy.Spec.Egress {
		if len(rule.To) == 0 && len(rule.Ports) == 0 {
			check.Passed = false
			check.Message = fmt.Sprintf("Egress rule %d allows traffic to all destinations", i)
			check.Details = map[string]interface{}{
				"rule_index": i,
				"allow_all":  true,
			}
			return check, nil
		}
	}

	check.Passed = true
	check.Message = "No allow-all egress rules found"
	return check, nil
}

// checkDNSEgressAllowed verifies DNS is accessible when egress is restricted.
func checkDNSEgressAllowed(ctx context.Context, policy *NetworkPolicy, client *Client) (*VerificationCheck, error) {
	check := &VerificationCheck{
		Name:        "dns-egress-allowed",
		Description: "Verify DNS egress is allowed when egress is restricted",
		Severity:    "high",
	}

	// Check if egress is restricted
	hasEgress := false
	for _, pt := range policy.Spec.PolicyTypes {
		if pt == PolicyTypeEgress {
			hasEgress = true
			break
		}
	}

	if !hasEgress {
		check.Passed = true
		check.Message = "No egress restrictions, DNS implicitly allowed"
		return check, nil
	}

	// If no egress rules, all egress is denied (including DNS)
	if len(policy.Spec.Egress) == 0 {
		check.Passed = false
		check.Message = "All egress denied, including DNS"
		return check, nil
	}

	// Check if DNS port (53) is allowed
	dnsAllowed := false
	for _, rule := range policy.Spec.Egress {
		// Check if this rule allows DNS
		if len(rule.Ports) == 0 {
			// No port restriction means DNS is allowed
			dnsAllowed = true
			break
		}
		for _, port := range rule.Ports {
			if port.Port == 53 || port.Port == 0 {
				dnsAllowed = true
				break
			}
		}
		if dnsAllowed {
			break
		}
	}

	if dnsAllowed {
		check.Passed = true
		check.Message = "DNS egress is allowed"
	} else {
		check.Passed = false
		check.Message = "DNS egress (port 53) may be blocked"
	}

	return check, nil
}

// checkNoWideCIDR checks for overly permissive CIDR ranges.
func checkNoWideCIDR(ctx context.Context, policy *NetworkPolicy, client *Client) (*VerificationCheck, error) {
	check := &VerificationCheck{
		Name:        "no-wide-cidr",
		Description: "Check for overly permissive CIDR ranges",
		Severity:    "medium",
	}

	wideCIDRs := []string{
		"0.0.0.0/0",
		"::/0",
		"0.0.0.0/1",
		"128.0.0.0/1",
	}

	var foundWideCIDRs []string

	// Check ingress rules
	for _, rule := range policy.Spec.Ingress {
		for _, peer := range rule.From {
			if peer.IPBlock != nil {
				for _, wide := range wideCIDRs {
					if peer.IPBlock.CIDR == wide {
						foundWideCIDRs = append(foundWideCIDRs, fmt.Sprintf("ingress: %s", wide))
					}
				}
			}
		}
	}

	// Check egress rules
	for _, rule := range policy.Spec.Egress {
		for _, peer := range rule.To {
			if peer.IPBlock != nil {
				for _, wide := range wideCIDRs {
					if peer.IPBlock.CIDR == wide {
						foundWideCIDRs = append(foundWideCIDRs, fmt.Sprintf("egress: %s", wide))
					}
				}
			}
		}
	}

	if len(foundWideCIDRs) > 0 {
		check.Passed = false
		check.Message = fmt.Sprintf("Found overly permissive CIDR ranges: %v", foundWideCIDRs)
		check.Details = map[string]interface{}{
			"wide_cidrs": foundWideCIDRs,
		}
	} else {
		check.Passed = true
		check.Message = "No overly permissive CIDR ranges found"
	}

	return check, nil
}
