// Package k8s provides Kubernetes integration for Keystone.
package k8s

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ToK8sNetworkPolicy converts a Keystone NetworkPolicy to a Kubernetes NetworkPolicy.
func ToK8sNetworkPolicy(policy *NetworkPolicy) *networkingv1.NetworkPolicy {
	if policy == nil {
		return nil
	}

	k8sPolicy := &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "NetworkPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        policy.Name,
			Namespace:   policy.Namespace,
			Labels:      policy.Labels,
			Annotations: policy.Annotations,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: toLabelSelector(policy.Spec.PodSelector),
			PolicyTypes: toPolicyTypes(policy.Spec.PolicyTypes),
		},
	}

	// Convert ingress rules
	if len(policy.Spec.Ingress) > 0 {
		k8sPolicy.Spec.Ingress = make([]networkingv1.NetworkPolicyIngressRule, len(policy.Spec.Ingress))
		for i, rule := range policy.Spec.Ingress {
			k8sPolicy.Spec.Ingress[i] = toIngressRule(rule)
		}
	}

	// Convert egress rules
	if len(policy.Spec.Egress) > 0 {
		k8sPolicy.Spec.Egress = make([]networkingv1.NetworkPolicyEgressRule, len(policy.Spec.Egress))
		for i, rule := range policy.Spec.Egress {
			k8sPolicy.Spec.Egress[i] = toEgressRule(rule)
		}
	}

	return k8sPolicy
}

// FromK8sNetworkPolicy converts a Kubernetes NetworkPolicy to a Keystone NetworkPolicy.
func FromK8sNetworkPolicy(np *networkingv1.NetworkPolicy) *NetworkPolicy {
	if np == nil {
		return nil
	}

	policy := &NetworkPolicy{
		Name:        np.Name,
		Namespace:   np.Namespace,
		Labels:      np.Labels,
		Annotations: np.Annotations,
		Spec: NetworkPolicySpec{
			PodSelector: fromLabelSelector(np.Spec.PodSelector),
			PolicyTypes: fromPolicyTypes(np.Spec.PolicyTypes),
		},
		CreatedAt: np.CreationTimestamp.Time,
		UpdatedAt: time.Now(),
	}

	// Convert ingress rules
	if len(np.Spec.Ingress) > 0 {
		policy.Spec.Ingress = make([]NetworkPolicyIngressRule, len(np.Spec.Ingress))
		for i, rule := range np.Spec.Ingress {
			policy.Spec.Ingress[i] = fromIngressRule(rule)
		}
	}

	// Convert egress rules
	if len(np.Spec.Egress) > 0 {
		policy.Spec.Egress = make([]NetworkPolicyEgressRule, len(np.Spec.Egress))
		for i, rule := range np.Spec.Egress {
			policy.Spec.Egress[i] = fromEgressRule(rule)
		}
	}

	return policy
}

// toLabelSelector converts a Keystone LabelSelector to a Kubernetes LabelSelector.
func toLabelSelector(ls LabelSelector) metav1.LabelSelector {
	result := metav1.LabelSelector{
		MatchLabels: ls.MatchLabels,
	}

	if len(ls.MatchExpressions) > 0 {
		result.MatchExpressions = make([]metav1.LabelSelectorRequirement, len(ls.MatchExpressions))
		for i, expr := range ls.MatchExpressions {
			result.MatchExpressions[i] = metav1.LabelSelectorRequirement{
				Key:      expr.Key,
				Operator: metav1.LabelSelectorOperator(expr.Operator),
				Values:   expr.Values,
			}
		}
	}

	return result
}

// fromLabelSelector converts a Kubernetes LabelSelector to a Keystone LabelSelector.
func fromLabelSelector(ls metav1.LabelSelector) LabelSelector {
	result := LabelSelector{
		MatchLabels: ls.MatchLabels,
	}

	if len(ls.MatchExpressions) > 0 {
		result.MatchExpressions = make([]LabelSelectorRequirement, len(ls.MatchExpressions))
		for i, expr := range ls.MatchExpressions {
			result.MatchExpressions[i] = LabelSelectorRequirement{
				Key:      expr.Key,
				Operator: string(expr.Operator),
				Values:   expr.Values,
			}
		}
	}

	return result
}

// toLabelSelectorPtr converts a Keystone LabelSelector pointer to a Kubernetes LabelSelector pointer.
func toLabelSelectorPtr(ls *LabelSelector) *metav1.LabelSelector {
	if ls == nil {
		return nil
	}
	result := toLabelSelector(*ls)
	return &result
}

// fromLabelSelectorPtr converts a Kubernetes LabelSelector pointer to a Keystone LabelSelector pointer.
func fromLabelSelectorPtr(ls *metav1.LabelSelector) *LabelSelector {
	if ls == nil {
		return nil
	}
	result := fromLabelSelector(*ls)
	return &result
}

// toPolicyTypes converts Keystone PolicyTypes to Kubernetes PolicyTypes.
func toPolicyTypes(types []PolicyType) []networkingv1.PolicyType {
	if len(types) == 0 {
		return nil
	}

	result := make([]networkingv1.PolicyType, len(types))
	for i, t := range types {
		result[i] = networkingv1.PolicyType(t)
	}
	return result
}

// fromPolicyTypes converts Kubernetes PolicyTypes to Keystone PolicyTypes.
func fromPolicyTypes(types []networkingv1.PolicyType) []PolicyType {
	if len(types) == 0 {
		return nil
	}

	result := make([]PolicyType, len(types))
	for i, t := range types {
		result[i] = PolicyType(t)
	}
	return result
}

// toIngressRule converts a Keystone NetworkPolicyIngressRule to a Kubernetes one.
func toIngressRule(rule NetworkPolicyIngressRule) networkingv1.NetworkPolicyIngressRule {
	result := networkingv1.NetworkPolicyIngressRule{}

	// Convert ports
	if len(rule.Ports) > 0 {
		result.Ports = make([]networkingv1.NetworkPolicyPort, len(rule.Ports))
		for i, port := range rule.Ports {
			result.Ports[i] = toNetworkPolicyPort(port)
		}
	}

	// Convert from peers
	if len(rule.From) > 0 {
		result.From = make([]networkingv1.NetworkPolicyPeer, len(rule.From))
		for i, peer := range rule.From {
			result.From[i] = toNetworkPolicyPeer(peer)
		}
	}

	return result
}

// fromIngressRule converts a Kubernetes NetworkPolicyIngressRule to a Keystone one.
func fromIngressRule(rule networkingv1.NetworkPolicyIngressRule) NetworkPolicyIngressRule {
	result := NetworkPolicyIngressRule{}

	// Convert ports
	if len(rule.Ports) > 0 {
		result.Ports = make([]NetworkPolicyPort, len(rule.Ports))
		for i, port := range rule.Ports {
			result.Ports[i] = fromNetworkPolicyPort(port)
		}
	}

	// Convert from peers
	if len(rule.From) > 0 {
		result.From = make([]NetworkPolicyPeer, len(rule.From))
		for i, peer := range rule.From {
			result.From[i] = fromNetworkPolicyPeer(peer)
		}
	}

	return result
}

// toEgressRule converts a Keystone NetworkPolicyEgressRule to a Kubernetes one.
func toEgressRule(rule NetworkPolicyEgressRule) networkingv1.NetworkPolicyEgressRule {
	result := networkingv1.NetworkPolicyEgressRule{}

	// Convert ports
	if len(rule.Ports) > 0 {
		result.Ports = make([]networkingv1.NetworkPolicyPort, len(rule.Ports))
		for i, port := range rule.Ports {
			result.Ports[i] = toNetworkPolicyPort(port)
		}
	}

	// Convert to peers
	if len(rule.To) > 0 {
		result.To = make([]networkingv1.NetworkPolicyPeer, len(rule.To))
		for i, peer := range rule.To {
			result.To[i] = toNetworkPolicyPeer(peer)
		}
	}

	return result
}

// fromEgressRule converts a Kubernetes NetworkPolicyEgressRule to a Keystone one.
func fromEgressRule(rule networkingv1.NetworkPolicyEgressRule) NetworkPolicyEgressRule {
	result := NetworkPolicyEgressRule{}

	// Convert ports
	if len(rule.Ports) > 0 {
		result.Ports = make([]NetworkPolicyPort, len(rule.Ports))
		for i, port := range rule.Ports {
			result.Ports[i] = fromNetworkPolicyPort(port)
		}
	}

	// Convert to peers
	if len(rule.To) > 0 {
		result.To = make([]NetworkPolicyPeer, len(rule.To))
		for i, peer := range rule.To {
			result.To[i] = fromNetworkPolicyPeer(peer)
		}
	}

	return result
}

// toNetworkPolicyPort converts a Keystone NetworkPolicyPort to a Kubernetes one.
func toNetworkPolicyPort(port NetworkPolicyPort) networkingv1.NetworkPolicyPort {
	result := networkingv1.NetworkPolicyPort{}

	// Convert protocol
	if port.Protocol != "" {
		proto := corev1.Protocol(port.Protocol)
		result.Protocol = &proto
	}

	// Convert port
	if port.Port > 0 {
		portVal := intstr.FromInt32(port.Port)
		result.Port = &portVal
	}

	// Convert end port (for port ranges)
	if port.EndPort > 0 {
		result.EndPort = &port.EndPort
	}

	return result
}

// fromNetworkPolicyPort converts a Kubernetes NetworkPolicyPort to a Keystone one.
func fromNetworkPolicyPort(port networkingv1.NetworkPolicyPort) NetworkPolicyPort {
	result := NetworkPolicyPort{}

	// Convert protocol
	if port.Protocol != nil {
		result.Protocol = Protocol(*port.Protocol)
	}

	// Convert port
	if port.Port != nil {
		result.Port = port.Port.IntVal
	}

	// Convert end port
	if port.EndPort != nil {
		result.EndPort = *port.EndPort
	}

	return result
}

// toNetworkPolicyPeer converts a Keystone NetworkPolicyPeer to a Kubernetes one.
func toNetworkPolicyPeer(peer NetworkPolicyPeer) networkingv1.NetworkPolicyPeer {
	result := networkingv1.NetworkPolicyPeer{}

	// Convert pod selector
	if peer.PodSelector != nil {
		result.PodSelector = toLabelSelectorPtr(peer.PodSelector)
	}

	// Convert namespace selector
	if peer.NamespaceSelector != nil {
		result.NamespaceSelector = toLabelSelectorPtr(peer.NamespaceSelector)
	}

	// Convert IP block
	if peer.IPBlock != nil {
		result.IPBlock = &networkingv1.IPBlock{
			CIDR:   peer.IPBlock.CIDR,
			Except: peer.IPBlock.Except,
		}
	}

	return result
}

// fromNetworkPolicyPeer converts a Kubernetes NetworkPolicyPeer to a Keystone one.
func fromNetworkPolicyPeer(peer networkingv1.NetworkPolicyPeer) NetworkPolicyPeer {
	result := NetworkPolicyPeer{}

	// Convert pod selector
	if peer.PodSelector != nil {
		result.PodSelector = fromLabelSelectorPtr(peer.PodSelector)
	}

	// Convert namespace selector
	if peer.NamespaceSelector != nil {
		result.NamespaceSelector = fromLabelSelectorPtr(peer.NamespaceSelector)
	}

	// Convert IP block
	if peer.IPBlock != nil {
		result.IPBlock = &IPBlock{
			CIDR:   peer.IPBlock.CIDR,
			Except: peer.IPBlock.Except,
		}
	}

	return result
}

// ToK8sNetworkPolicyList converts a slice of Keystone NetworkPolicies to Kubernetes ones.
func ToK8sNetworkPolicyList(policies []*NetworkPolicy) []networkingv1.NetworkPolicy {
	if len(policies) == 0 {
		return nil
	}

	result := make([]networkingv1.NetworkPolicy, len(policies))
	for i, policy := range policies {
		if k8sPolicy := ToK8sNetworkPolicy(policy); k8sPolicy != nil {
			result[i] = *k8sPolicy
		}
	}
	return result
}

// FromK8sNetworkPolicyList converts a slice of Kubernetes NetworkPolicies to Keystone ones.
func FromK8sNetworkPolicyList(policies []networkingv1.NetworkPolicy) []*NetworkPolicy {
	if len(policies) == 0 {
		return nil
	}

	result := make([]*NetworkPolicy, len(policies))
	for i := range policies {
		result[i] = FromK8sNetworkPolicy(&policies[i])
	}
	return result
}
