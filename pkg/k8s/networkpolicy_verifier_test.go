package k8s

import (
	"context"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewNetworkPolicyVerifier(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	verifier := NewNetworkPolicyVerifier(client)

	if verifier == nil {
		t.Fatal("Expected verifier to be non-nil")
	}
	if len(verifier.rules) == 0 {
		t.Error("Expected default rules to be registered")
	}
}

func TestNetworkPolicyVerifier_Verify(t *testing.T) {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "frontend"},
							},
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(np)
	client := NewClientWithInterface(fakeClient, ClusterConfig{})
	verifier := NewNetworkPolicyVerifier(client)

	policy := FromK8sNetworkPolicy(np)

	result, err := verifier.Verify(context.Background(), policy)
	if err != nil {
		t.Fatalf("Verify error = %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if len(result.Checks) == 0 {
		t.Error("Expected checks to be run")
	}

	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}

	// Log check results for debugging
	for _, check := range result.Checks {
		t.Logf("Check %s: passed=%v, message=%s", check.Name, check.Passed, check.Message)
	}
}

func TestNetworkPolicyVerifier_AddRule(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	client := NewClientWithInterface(fakeClient, ClusterConfig{})
	verifier := NewNetworkPolicyVerifier(client)

	initialCount := len(verifier.rules)

	customRule := VerificationRule{
		Name:        "custom-rule",
		Description: "A custom test rule",
		Severity:    "low",
		Check: func(ctx context.Context, policy *NetworkPolicy, client *Client) (*VerificationCheck, error) {
			return &VerificationCheck{
				Name:    "custom-rule",
				Passed:  true,
				Message: "Custom check passed",
			}, nil
		},
	}

	verifier.AddRule(customRule)

	if len(verifier.rules) != initialCount+1 {
		t.Errorf("Rules count = %d, want %d", len(verifier.rules), initialCount+1)
	}
}

func TestCheckPolicyExists(t *testing.T) {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-policy",
			Namespace: "default",
		},
	}

	fakeClient := fake.NewSimpleClientset(np)
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	// Test existing policy
	policy := &NetworkPolicy{Name: "existing-policy", Namespace: "default"}
	check, err := checkPolicyExists(context.Background(), policy, client)
	if err != nil {
		t.Fatalf("checkPolicyExists error = %v", err)
	}
	if !check.Passed {
		t.Errorf("Expected check to pass for existing policy, got: %s", check.Message)
	}

	// Test non-existing policy
	policy = &NetworkPolicy{Name: "nonexistent", Namespace: "default"}
	check, err = checkPolicyExists(context.Background(), policy, client)
	if err != nil {
		t.Fatalf("checkPolicyExists error = %v", err)
	}
	if check.Passed {
		t.Error("Expected check to fail for non-existing policy")
	}

	// Test with nil client
	check, err = checkPolicyExists(context.Background(), policy, nil)
	if err != nil {
		t.Fatalf("checkPolicyExists error = %v", err)
	}
	if !check.Passed {
		t.Error("Expected check to pass (skip) with nil client")
	}
}

func TestCheckSelectorValid(t *testing.T) {
	tests := []struct {
		name           string
		policy         *NetworkPolicy
		expectPassed   bool
		expectMatchAll bool
	}{
		{
			name: "empty selector",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PodSelector: LabelSelector{},
				},
			},
			expectPassed:   true,
			expectMatchAll: true,
		},
		{
			name: "with labels",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PodSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			},
			expectPassed:   true,
			expectMatchAll: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check, err := checkSelectorValid(context.Background(), tt.policy, nil)
			if err != nil {
				t.Fatalf("checkSelectorValid error = %v", err)
			}
			if check.Passed != tt.expectPassed {
				t.Errorf("Passed = %v, want %v", check.Passed, tt.expectPassed)
			}
			if tt.expectMatchAll {
				if check.Details == nil || check.Details["matches_all"] != true {
					t.Error("Expected matches_all to be true for empty selector")
				}
			}
		})
	}
}

func TestCheckNoAllowAllIngress(t *testing.T) {
	tests := []struct {
		name         string
		policy       *NetworkPolicy
		expectPassed bool
	}{
		{
			name: "no ingress policy type",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeEgress},
				},
			},
			expectPassed: true,
		},
		{
			name: "ingress with specific source",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeIngress},
					Ingress: []NetworkPolicyIngressRule{
						{
							From: []NetworkPolicyPeer{
								{
									PodSelector: &LabelSelector{
										MatchLabels: map[string]string{"app": "frontend"},
									},
								},
							},
						},
					},
				},
			},
			expectPassed: true,
		},
		{
			name: "allow-all ingress",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeIngress},
					Ingress: []NetworkPolicyIngressRule{
						{
							From: []NetworkPolicyPeer{}, // Empty = allow all
						},
					},
				},
			},
			expectPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check, err := checkNoAllowAllIngress(context.Background(), tt.policy, nil)
			if err != nil {
				t.Fatalf("checkNoAllowAllIngress error = %v", err)
			}
			if check.Passed != tt.expectPassed {
				t.Errorf("Passed = %v, want %v; message: %s", check.Passed, tt.expectPassed, check.Message)
			}
		})
	}
}

func TestCheckNoAllowAllEgress(t *testing.T) {
	tests := []struct {
		name         string
		policy       *NetworkPolicy
		expectPassed bool
	}{
		{
			name: "no egress policy type",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeIngress},
				},
			},
			expectPassed: true,
		},
		{
			name: "egress with specific destination",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeEgress},
					Egress: []NetworkPolicyEgressRule{
						{
							To: []NetworkPolicyPeer{
								{
									IPBlock: &IPBlock{CIDR: "10.0.0.0/8"},
								},
							},
						},
					},
				},
			},
			expectPassed: true,
		},
		{
			name: "allow-all egress",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeEgress},
					Egress: []NetworkPolicyEgressRule{
						{
							To:    []NetworkPolicyPeer{}, // Empty
							Ports: []NetworkPolicyPort{}, // Empty = allow all
						},
					},
				},
			},
			expectPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check, err := checkNoAllowAllEgress(context.Background(), tt.policy, nil)
			if err != nil {
				t.Fatalf("checkNoAllowAllEgress error = %v", err)
			}
			if check.Passed != tt.expectPassed {
				t.Errorf("Passed = %v, want %v; message: %s", check.Passed, tt.expectPassed, check.Message)
			}
		})
	}
}

func TestCheckDNSEgressAllowed(t *testing.T) {
	tests := []struct {
		name         string
		policy       *NetworkPolicy
		expectPassed bool
	}{
		{
			name: "no egress restriction",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeIngress},
				},
			},
			expectPassed: true,
		},
		{
			name: "egress restricted but no rules",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeEgress},
					Egress:      []NetworkPolicyEgressRule{},
				},
			},
			expectPassed: false,
		},
		{
			name: "egress allows DNS",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeEgress},
					Egress: []NetworkPolicyEgressRule{
						{
							Ports: []NetworkPolicyPort{
								{Protocol: ProtocolUDP, Port: 53},
							},
						},
					},
				},
			},
			expectPassed: true,
		},
		{
			name: "egress blocks DNS",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeEgress},
					Egress: []NetworkPolicyEgressRule{
						{
							Ports: []NetworkPolicyPort{
								{Protocol: ProtocolTCP, Port: 443},
							},
						},
					},
				},
			},
			expectPassed: false,
		},
		{
			name: "egress no port restriction",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeEgress},
					Egress: []NetworkPolicyEgressRule{
						{
							To: []NetworkPolicyPeer{
								{IPBlock: &IPBlock{CIDR: "0.0.0.0/0"}},
							},
							// No Ports = all ports allowed
						},
					},
				},
			},
			expectPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check, err := checkDNSEgressAllowed(context.Background(), tt.policy, nil)
			if err != nil {
				t.Fatalf("checkDNSEgressAllowed error = %v", err)
			}
			if check.Passed != tt.expectPassed {
				t.Errorf("Passed = %v, want %v; message: %s", check.Passed, tt.expectPassed, check.Message)
			}
		})
	}
}

func TestCheckNoWideCIDR(t *testing.T) {
	tests := []struct {
		name         string
		policy       *NetworkPolicy
		expectPassed bool
	}{
		{
			name: "no CIDR rules",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					Ingress: []NetworkPolicyIngressRule{
						{
							From: []NetworkPolicyPeer{
								{
									PodSelector: &LabelSelector{},
								},
							},
						},
					},
				},
			},
			expectPassed: true,
		},
		{
			name: "narrow CIDR",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					Ingress: []NetworkPolicyIngressRule{
						{
							From: []NetworkPolicyPeer{
								{
									IPBlock: &IPBlock{CIDR: "10.0.0.0/24"},
								},
							},
						},
					},
				},
			},
			expectPassed: true,
		},
		{
			name: "wide CIDR 0.0.0.0/0",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					Egress: []NetworkPolicyEgressRule{
						{
							To: []NetworkPolicyPeer{
								{
									IPBlock: &IPBlock{CIDR: "0.0.0.0/0"},
								},
							},
						},
					},
				},
			},
			expectPassed: false,
		},
		{
			name: "wide CIDR ::/0",
			policy: &NetworkPolicy{
				Spec: NetworkPolicySpec{
					Ingress: []NetworkPolicyIngressRule{
						{
							From: []NetworkPolicyPeer{
								{
									IPBlock: &IPBlock{CIDR: "::/0"},
								},
							},
						},
					},
				},
			},
			expectPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check, err := checkNoWideCIDR(context.Background(), tt.policy, nil)
			if err != nil {
				t.Fatalf("checkNoWideCIDR error = %v", err)
			}
			if check.Passed != tt.expectPassed {
				t.Errorf("Passed = %v, want %v; message: %s", check.Passed, tt.expectPassed, check.Message)
			}
		})
	}
}

func TestVerificationCheck_Types(t *testing.T) {
	check := &VerificationCheck{
		Name:        "test-check",
		Description: "A test verification check",
		Passed:      true,
		Message:     "Check passed successfully",
		Severity:    "high",
		Details: map[string]interface{}{
			"key": "value",
		},
	}

	if check.Name != "test-check" {
		t.Error("Name not set correctly")
	}
	if check.Severity != "high" {
		t.Error("Severity not set correctly")
	}
	if check.Details["key"] != "value" {
		t.Error("Details not set correctly")
	}
}

func TestNetworkPolicyVerificationResult_Types(t *testing.T) {
	policy := &NetworkPolicy{Name: "test", Namespace: "default"}
	result := &NetworkPolicyVerificationResult{
		Policy:  policy,
		Passed:  true,
		Message: "All checks passed",
		Checks: []VerificationCheck{
			{Name: "check1", Passed: true},
			{Name: "check2", Passed: true},
		},
	}

	if result.Policy.Name != "test" {
		t.Error("Policy not set correctly")
	}
	if len(result.Checks) != 2 {
		t.Error("Checks not set correctly")
	}
}

func TestNetworkPolicyVerifier_VerifyDenyAllPolicy(t *testing.T) {
	// Create a deny-all policy (common pattern)
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deny-all",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{}, // Empty = all pods
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			// No ingress/egress rules = deny all
		},
	}

	fakeClient := fake.NewSimpleClientset(np)
	client := NewClientWithInterface(fakeClient, ClusterConfig{})
	verifier := NewNetworkPolicyVerifier(client)

	policy := FromK8sNetworkPolicy(np)

	result, err := verifier.Verify(context.Background(), policy)
	if err != nil {
		t.Fatalf("Verify error = %v", err)
	}

	// Deny-all policy should fail DNS egress check
	var dnsFailed bool
	for _, check := range result.Checks {
		if check.Name == "dns-egress-allowed" && !check.Passed {
			dnsFailed = true
		}
	}

	if !dnsFailed {
		t.Error("Expected deny-all policy to fail DNS egress check")
	}
}
