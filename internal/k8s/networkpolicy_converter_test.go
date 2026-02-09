package k8s

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestToK8sNetworkPolicy(t *testing.T) {
	// Test nil input
	if ToK8sNetworkPolicy(nil) != nil {
		t.Error("Expected nil for nil input")
	}

	// Test basic conversion
	policy := &NetworkPolicy{
		Name:      "test-policy",
		Namespace: "default",
		Labels: map[string]string{
			"app": "test",
		},
		Annotations: map[string]string{
			"description": "Test policy",
		},
		Spec: NetworkPolicySpec{
			PodSelector: LabelSelector{
				MatchLabels: map[string]string{
					"role": "db",
				},
			},
			PolicyTypes: []PolicyType{PolicyTypeIngress, PolicyTypeEgress},
		},
	}

	k8sPolicy := ToK8sNetworkPolicy(policy)

	if k8sPolicy.Name != "test-policy" {
		t.Errorf("Name = %q, want %q", k8sPolicy.Name, "test-policy")
	}
	if k8sPolicy.Namespace != "default" {
		t.Errorf("Namespace = %q, want %q", k8sPolicy.Namespace, "default")
	}
	if k8sPolicy.Labels["app"] != "test" {
		t.Error("Labels not preserved")
	}
	if k8sPolicy.Annotations["description"] != "Test policy" {
		t.Error("Annotations not preserved")
	}
	if len(k8sPolicy.Spec.PolicyTypes) != 2 {
		t.Errorf("PolicyTypes length = %d, want 2", len(k8sPolicy.Spec.PolicyTypes))
	}
}

func TestFromK8sNetworkPolicy(t *testing.T) {
	// Test nil input
	if FromK8sNetworkPolicy(nil) != nil {
		t.Error("Expected nil for nil input")
	}

	// Test basic conversion
	k8sPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-policy",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now()),
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"role": "db",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	policy := FromK8sNetworkPolicy(k8sPolicy)

	if policy.Name != "test-policy" {
		t.Errorf("Name = %q, want %q", policy.Name, "test-policy")
	}
	if policy.Namespace != "default" {
		t.Errorf("Namespace = %q, want %q", policy.Namespace, "default")
	}
	if policy.Labels["app"] != "test" {
		t.Error("Labels not preserved")
	}
	if len(policy.Spec.PolicyTypes) != 2 {
		t.Errorf("PolicyTypes length = %d, want 2", len(policy.Spec.PolicyTypes))
	}
}

func TestRoundTripConversion(t *testing.T) {
	original := &NetworkPolicy{
		Name:      "round-trip-test",
		Namespace: "test-ns",
		Labels: map[string]string{
			"app":  "myapp",
			"tier": "backend",
		},
		Spec: NetworkPolicySpec{
			PodSelector: LabelSelector{
				MatchLabels: map[string]string{
					"role": "db",
				},
				MatchExpressions: []LabelSelectorRequirement{
					{
						Key:      "environment",
						Operator: "In",
						Values:   []string{"production", "staging"},
					},
				},
			},
			PolicyTypes: []PolicyType{PolicyTypeIngress, PolicyTypeEgress},
			Ingress: []NetworkPolicyIngressRule{
				{
					Ports: []NetworkPolicyPort{
						{Protocol: ProtocolTCP, Port: 5432},
					},
					From: []NetworkPolicyPeer{
						{
							PodSelector: &LabelSelector{
								MatchLabels: map[string]string{"app": "backend"},
							},
						},
						{
							NamespaceSelector: &LabelSelector{
								MatchLabels: map[string]string{"name": "production"},
							},
						},
						{
							IPBlock: &IPBlock{
								CIDR:   "10.0.0.0/8",
								Except: []string{"10.0.1.0/24"},
							},
						},
					},
				},
			},
			Egress: []NetworkPolicyEgressRule{
				{
					Ports: []NetworkPolicyPort{
						{Protocol: ProtocolTCP, Port: 443},
						{Protocol: ProtocolUDP, Port: 53},
					},
					To: []NetworkPolicyPeer{
						{
							IPBlock: &IPBlock{
								CIDR: "0.0.0.0/0",
							},
						},
					},
				},
			},
		},
	}

	// Convert to K8s and back
	k8sPolicy := ToK8sNetworkPolicy(original)
	roundTripped := FromK8sNetworkPolicy(k8sPolicy)

	// Verify key fields
	if roundTripped.Name != original.Name {
		t.Errorf("Name = %q, want %q", roundTripped.Name, original.Name)
	}
	if roundTripped.Namespace != original.Namespace {
		t.Errorf("Namespace = %q, want %q", roundTripped.Namespace, original.Namespace)
	}
	if len(roundTripped.Spec.Ingress) != len(original.Spec.Ingress) {
		t.Errorf("Ingress rules count = %d, want %d", len(roundTripped.Spec.Ingress), len(original.Spec.Ingress))
	}
	if len(roundTripped.Spec.Egress) != len(original.Spec.Egress) {
		t.Errorf("Egress rules count = %d, want %d", len(roundTripped.Spec.Egress), len(original.Spec.Egress))
	}

	// Verify ingress rule details
	if len(roundTripped.Spec.Ingress[0].Ports) != 1 {
		t.Error("Ingress ports not preserved")
	}
	if roundTripped.Spec.Ingress[0].Ports[0].Port != 5432 {
		t.Errorf("Ingress port = %d, want 5432", roundTripped.Spec.Ingress[0].Ports[0].Port)
	}

	// Verify IP block
	if len(roundTripped.Spec.Ingress[0].From) != 3 {
		t.Error("Ingress from peers not preserved")
	}
	ipBlock := roundTripped.Spec.Ingress[0].From[2].IPBlock
	if ipBlock == nil || ipBlock.CIDR != "10.0.0.0/8" {
		t.Error("IP block not preserved")
	}
	if len(ipBlock.Except) != 1 || ipBlock.Except[0] != "10.0.1.0/24" {
		t.Error("IP block except not preserved")
	}
}

func TestToLabelSelector(t *testing.T) {
	ls := LabelSelector{
		MatchLabels: map[string]string{
			"app":  "test",
			"tier": "backend",
		},
		MatchExpressions: []LabelSelectorRequirement{
			{
				Key:      "environment",
				Operator: "NotIn",
				Values:   []string{"dev"},
			},
		},
	}

	k8sLS := toLabelSelector(ls)

	if len(k8sLS.MatchLabels) != 2 {
		t.Errorf("MatchLabels length = %d, want 2", len(k8sLS.MatchLabels))
	}
	if k8sLS.MatchLabels["app"] != "test" {
		t.Error("MatchLabels not preserved")
	}
	if len(k8sLS.MatchExpressions) != 1 {
		t.Errorf("MatchExpressions length = %d, want 1", len(k8sLS.MatchExpressions))
	}
	if k8sLS.MatchExpressions[0].Operator != metav1.LabelSelectorOpNotIn {
		t.Error("MatchExpressions operator not preserved")
	}
}

func TestFromLabelSelector(t *testing.T) {
	k8sLS := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "test",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "tier",
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	}

	ls := fromLabelSelector(k8sLS)

	if len(ls.MatchLabels) != 1 {
		t.Errorf("MatchLabels length = %d, want 1", len(ls.MatchLabels))
	}
	if len(ls.MatchExpressions) != 1 {
		t.Errorf("MatchExpressions length = %d, want 1", len(ls.MatchExpressions))
	}
	if ls.MatchExpressions[0].Operator != "Exists" {
		t.Errorf("Operator = %q, want 'Exists'", ls.MatchExpressions[0].Operator)
	}
}

func TestToPolicyTypes(t *testing.T) {
	// Test nil/empty
	if toPolicyTypes(nil) != nil {
		t.Error("Expected nil for nil input")
	}
	if toPolicyTypes([]PolicyType{}) != nil {
		t.Error("Expected nil for empty input")
	}

	// Test conversion
	types := []PolicyType{PolicyTypeIngress, PolicyTypeEgress}
	k8sTypes := toPolicyTypes(types)

	if len(k8sTypes) != 2 {
		t.Errorf("Length = %d, want 2", len(k8sTypes))
	}
	if k8sTypes[0] != networkingv1.PolicyTypeIngress {
		t.Error("First type not Ingress")
	}
	if k8sTypes[1] != networkingv1.PolicyTypeEgress {
		t.Error("Second type not Egress")
	}
}

func TestFromPolicyTypes(t *testing.T) {
	// Test nil/empty
	if fromPolicyTypes(nil) != nil {
		t.Error("Expected nil for nil input")
	}

	// Test conversion
	k8sTypes := []networkingv1.PolicyType{
		networkingv1.PolicyTypeIngress,
		networkingv1.PolicyTypeEgress,
	}
	types := fromPolicyTypes(k8sTypes)

	if len(types) != 2 {
		t.Errorf("Length = %d, want 2", len(types))
	}
	if types[0] != PolicyTypeIngress {
		t.Error("First type not Ingress")
	}
	if types[1] != PolicyTypeEgress {
		t.Error("Second type not Egress")
	}
}

func TestToNetworkPolicyPort(t *testing.T) {
	port := NetworkPolicyPort{
		Protocol: ProtocolTCP,
		Port:     8080,
		EndPort:  8090,
	}

	k8sPort := toNetworkPolicyPort(port)

	if k8sPort.Protocol == nil || *k8sPort.Protocol != corev1.ProtocolTCP {
		t.Error("Protocol not set correctly")
	}
	if k8sPort.Port == nil || k8sPort.Port.IntVal != 8080 {
		t.Error("Port not set correctly")
	}
	if k8sPort.EndPort == nil || *k8sPort.EndPort != 8090 {
		t.Error("EndPort not set correctly")
	}
}

func TestFromNetworkPolicyPort(t *testing.T) {
	protocol := corev1.ProtocolUDP
	portVal := intstr.FromInt32(53)
	endPort := int32(60)

	k8sPort := networkingv1.NetworkPolicyPort{
		Protocol: &protocol,
		Port:     &portVal,
		EndPort:  &endPort,
	}

	port := fromNetworkPolicyPort(k8sPort)

	if port.Protocol != ProtocolUDP {
		t.Errorf("Protocol = %q, want UDP", port.Protocol)
	}
	if port.Port != 53 {
		t.Errorf("Port = %d, want 53", port.Port)
	}
	if port.EndPort != 60 {
		t.Errorf("EndPort = %d, want 60", port.EndPort)
	}
}

func TestToNetworkPolicyPeer(t *testing.T) {
	// Test with all peer types
	peer := NetworkPolicyPeer{
		PodSelector: &LabelSelector{
			MatchLabels: map[string]string{"app": "backend"},
		},
		NamespaceSelector: &LabelSelector{
			MatchLabels: map[string]string{"name": "production"},
		},
		IPBlock: &IPBlock{
			CIDR:   "192.168.0.0/16",
			Except: []string{"192.168.1.0/24"},
		},
	}

	k8sPeer := toNetworkPolicyPeer(peer)

	if k8sPeer.PodSelector == nil {
		t.Error("PodSelector not set")
	}
	if k8sPeer.NamespaceSelector == nil {
		t.Error("NamespaceSelector not set")
	}
	if k8sPeer.IPBlock == nil {
		t.Error("IPBlock not set")
	}
	if k8sPeer.IPBlock.CIDR != "192.168.0.0/16" {
		t.Errorf("IPBlock CIDR = %q, want '192.168.0.0/16'", k8sPeer.IPBlock.CIDR)
	}
}

func TestFromNetworkPolicyPeer(t *testing.T) {
	k8sPeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "frontend"},
		},
		IPBlock: &networkingv1.IPBlock{
			CIDR:   "10.0.0.0/8",
			Except: []string{"10.10.0.0/16"},
		},
	}

	peer := fromNetworkPolicyPeer(k8sPeer)

	if peer.PodSelector == nil {
		t.Error("PodSelector not set")
	}
	if peer.PodSelector.MatchLabels["app"] != "frontend" {
		t.Error("PodSelector labels not preserved")
	}
	if peer.NamespaceSelector != nil {
		t.Error("NamespaceSelector should be nil")
	}
	if peer.IPBlock == nil || peer.IPBlock.CIDR != "10.0.0.0/8" {
		t.Error("IPBlock not preserved")
	}
}

func TestToK8sNetworkPolicyList(t *testing.T) {
	// Test nil/empty
	if ToK8sNetworkPolicyList(nil) != nil {
		t.Error("Expected nil for nil input")
	}
	if ToK8sNetworkPolicyList([]*NetworkPolicy{}) != nil {
		t.Error("Expected nil for empty input")
	}

	// Test conversion
	policies := []*NetworkPolicy{
		{Name: "policy1", Namespace: "ns1"},
		{Name: "policy2", Namespace: "ns2"},
	}

	k8sPolicies := ToK8sNetworkPolicyList(policies)

	if len(k8sPolicies) != 2 {
		t.Errorf("Length = %d, want 2", len(k8sPolicies))
	}
	if k8sPolicies[0].Name != "policy1" {
		t.Error("First policy name not preserved")
	}
	if k8sPolicies[1].Name != "policy2" {
		t.Error("Second policy name not preserved")
	}
}

func TestFromK8sNetworkPolicyList(t *testing.T) {
	// Test nil/empty
	if FromK8sNetworkPolicyList(nil) != nil {
		t.Error("Expected nil for nil input")
	}
	if FromK8sNetworkPolicyList([]networkingv1.NetworkPolicy{}) != nil {
		t.Error("Expected nil for empty input")
	}

	// Test conversion
	k8sPolicies := []networkingv1.NetworkPolicy{
		{ObjectMeta: metav1.ObjectMeta{Name: "policy1", Namespace: "ns1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "policy2", Namespace: "ns2"}},
	}

	policies := FromK8sNetworkPolicyList(k8sPolicies)

	if len(policies) != 2 {
		t.Errorf("Length = %d, want 2", len(policies))
	}
	if policies[0].Name != "policy1" {
		t.Error("First policy name not preserved")
	}
	if policies[1].Name != "policy2" {
		t.Error("Second policy name not preserved")
	}
}

func TestIngressRuleConversion(t *testing.T) {
	rule := NetworkPolicyIngressRule{
		Ports: []NetworkPolicyPort{
			{Protocol: ProtocolTCP, Port: 80},
			{Protocol: ProtocolTCP, Port: 443},
		},
		From: []NetworkPolicyPeer{
			{
				PodSelector: &LabelSelector{
					MatchLabels: map[string]string{"app": "frontend"},
				},
			},
		},
	}

	k8sRule := toIngressRule(rule)

	if len(k8sRule.Ports) != 2 {
		t.Errorf("Ports length = %d, want 2", len(k8sRule.Ports))
	}
	if len(k8sRule.From) != 1 {
		t.Errorf("From length = %d, want 1", len(k8sRule.From))
	}

	// Convert back
	converted := fromIngressRule(k8sRule)
	if len(converted.Ports) != 2 {
		t.Errorf("Converted ports length = %d, want 2", len(converted.Ports))
	}
}

func TestEgressRuleConversion(t *testing.T) {
	rule := NetworkPolicyEgressRule{
		Ports: []NetworkPolicyPort{
			{Protocol: ProtocolUDP, Port: 53},
		},
		To: []NetworkPolicyPeer{
			{
				IPBlock: &IPBlock{
					CIDR: "0.0.0.0/0",
				},
			},
		},
	}

	k8sRule := toEgressRule(rule)

	if len(k8sRule.Ports) != 1 {
		t.Errorf("Ports length = %d, want 1", len(k8sRule.Ports))
	}
	if len(k8sRule.To) != 1 {
		t.Errorf("To length = %d, want 1", len(k8sRule.To))
	}

	// Convert back
	converted := fromEgressRule(k8sRule)
	if converted.To[0].IPBlock == nil || converted.To[0].IPBlock.CIDR != "0.0.0.0/0" {
		t.Error("Egress IP block not preserved")
	}
}

func TestEmptySelectorsAndRules(t *testing.T) {
	// Test policy with empty pod selector (matches all pods in namespace)
	policy := &NetworkPolicy{
		Name:      "deny-all",
		Namespace: "default",
		Spec: NetworkPolicySpec{
			PodSelector: LabelSelector{}, // Empty = match all
			PolicyTypes: []PolicyType{PolicyTypeIngress},
			Ingress:     []NetworkPolicyIngressRule{}, // Empty = deny all
		},
	}

	k8sPolicy := ToK8sNetworkPolicy(policy)

	if len(k8sPolicy.Spec.PodSelector.MatchLabels) > 0 {
		t.Error("Empty pod selector should have no match labels")
	}
	if len(k8sPolicy.Spec.Ingress) > 0 {
		t.Error("Empty ingress should deny all")
	}

	// Round trip
	roundTripped := FromK8sNetworkPolicy(k8sPolicy)
	if len(roundTripped.Spec.Ingress) != 0 {
		t.Error("Round tripped ingress should be empty")
	}
}
