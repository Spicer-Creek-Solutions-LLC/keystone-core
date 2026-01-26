// Package servicemesh provides service mesh integration for Keystone Core.
package servicemesh

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Istio security.istio.io GVRs
var (
	// PeerAuthenticationGVR is the GVR for PeerAuthentication CRD
	PeerAuthenticationGVR = schema.GroupVersionResource{
		Group:    "security.istio.io",
		Version:  "v1",
		Resource: "peerauthentications",
	}

	// AuthorizationPolicyGVR is the GVR for AuthorizationPolicy CRD
	AuthorizationPolicyGVR = schema.GroupVersionResource{
		Group:    "security.istio.io",
		Version:  "v1",
		Resource: "authorizationpolicies",
	}

	// DestinationRuleGVR is the GVR for DestinationRule CRD
	DestinationRuleGVR = schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1",
		Resource: "destinationrules",
	}
)

// IstioCRDClient provides access to Istio CRDs in Kubernetes
type IstioCRDClient struct {
	client dynamic.Interface
}

// NewIstioCRDClient creates a new Istio CRD client
func NewIstioCRDClient(kubeconfig string) (*IstioCRDClient, error) {
	var config *rest.Config
	var err error

	if kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &IstioCRDClient{client: client}, nil
}

// NewIstioCRDClientFromDynamic creates an Istio CRD client from an existing dynamic client
func NewIstioCRDClientFromDynamic(client dynamic.Interface) *IstioCRDClient {
	return &IstioCRDClient{client: client}
}

// ListPeerAuthentications lists all PeerAuthentication resources in a namespace
func (c *IstioCRDClient) ListPeerAuthentications(ctx context.Context, namespace string) ([]*MTLSPolicy, error) {
	var list *unstructured.UnstructuredList
	var err error

	if namespace == "" {
		list, err = c.client.Resource(PeerAuthenticationGVR).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.client.Resource(PeerAuthenticationGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list PeerAuthentications: %w", err)
	}

	policies := make([]*MTLSPolicy, 0, len(list.Items))
	for _, item := range list.Items {
		policy, err := parsePeerAuthentication(&item)
		if err != nil {
			continue // Skip invalid entries
		}
		policies = append(policies, policy)
	}

	return policies, nil
}

// GetPeerAuthentication gets a specific PeerAuthentication resource
func (c *IstioCRDClient) GetPeerAuthentication(ctx context.Context, namespace, name string) (*MTLSPolicy, error) {
	obj, err := c.client.Resource(PeerAuthenticationGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get PeerAuthentication %s/%s: %w", namespace, name, err)
	}

	return parsePeerAuthentication(obj)
}

// ListAuthorizationPolicies lists all AuthorizationPolicy resources in a namespace
func (c *IstioCRDClient) ListAuthorizationPolicies(ctx context.Context, namespace string) ([]*AuthorizationPolicy, error) {
	var list *unstructured.UnstructuredList
	var err error

	if namespace == "" {
		list, err = c.client.Resource(AuthorizationPolicyGVR).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.client.Resource(AuthorizationPolicyGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list AuthorizationPolicies: %w", err)
	}

	policies := make([]*AuthorizationPolicy, 0, len(list.Items))
	for _, item := range list.Items {
		policy, err := parseAuthorizationPolicy(&item)
		if err != nil {
			continue // Skip invalid entries
		}
		policies = append(policies, policy)
	}

	return policies, nil
}

// GetAuthorizationPolicy gets a specific AuthorizationPolicy resource
func (c *IstioCRDClient) GetAuthorizationPolicy(ctx context.Context, namespace, name string) (*AuthorizationPolicy, error) {
	obj, err := c.client.Resource(AuthorizationPolicyGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get AuthorizationPolicy %s/%s: %w", namespace, name, err)
	}

	return parseAuthorizationPolicy(obj)
}

// ListDestinationRules lists all DestinationRule resources in a namespace
func (c *IstioCRDClient) ListDestinationRules(ctx context.Context, namespace string) ([]*DestinationRule, error) {
	var list *unstructured.UnstructuredList
	var err error

	if namespace == "" {
		list, err = c.client.Resource(DestinationRuleGVR).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.client.Resource(DestinationRuleGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list DestinationRules: %w", err)
	}

	rules := make([]*DestinationRule, 0, len(list.Items))
	for _, item := range list.Items {
		rule, err := parseDestinationRule(&item)
		if err != nil {
			continue // Skip invalid entries
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// parsePeerAuthentication parses an unstructured PeerAuthentication into MTLSPolicy
func parsePeerAuthentication(obj *unstructured.Unstructured) (*MTLSPolicy, error) {
	policy := &MTLSPolicy{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	// Parse spec
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return nil, fmt.Errorf("spec not found")
	}

	// Parse selector to determine target service
	if selector, found, _ := unstructured.NestedMap(spec, "selector", "matchLabels"); found {
		if app, ok := selector["app"].(string); ok {
			policy.Service = app
		}
	}

	// Parse mTLS mode
	if mtls, found, _ := unstructured.NestedMap(spec, "mtls"); found {
		if mode, ok := mtls["mode"].(string); ok {
			policy.Mode = PolicyMode(mode)
		}
	} else {
		// Default to PERMISSIVE if not specified
		policy.Mode = PolicyModePermissive
	}

	// Parse port-level mTLS
	if portLevelMtls, found, _ := unstructured.NestedSlice(spec, "portLevelMtls"); found {
		policy.PeerAuthentication = &PeerAuthentication{
			Mode:          policy.Mode,
			PortLevelMtls: make(map[int]PolicyMode),
		}
		for _, item := range portLevelMtls {
			if portMap, ok := item.(map[string]interface{}); ok {
				port := int64(0)
				if p, found, _ := unstructured.NestedInt64(portMap, "port"); found {
					port = p
				}
				if mtls, found, _ := unstructured.NestedMap(portMap, "mtls"); found {
					if mode, ok := mtls["mode"].(string); ok {
						policy.PeerAuthentication.PortLevelMtls[int(port)] = PolicyMode(mode)
					}
				}
			}
		}
	}

	// Parse timestamps
	if ts := obj.GetCreationTimestamp(); !ts.IsZero() {
		policy.CreatedAt = ts.Time
	}
	policy.UpdatedAt = time.Now()

	return policy, nil
}

// parseAuthorizationPolicy parses an unstructured AuthorizationPolicy
func parseAuthorizationPolicy(obj *unstructured.Unstructured) (*AuthorizationPolicy, error) {
	policy := &AuthorizationPolicy{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	// Parse spec
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return nil, fmt.Errorf("spec not found")
	}

	// Parse action
	if action, found, _ := unstructured.NestedString(spec, "action"); found {
		policy.Action = AuthorizationAction(action)
	} else {
		policy.Action = AuthorizationActionAllow // Default
	}

	// Parse selector
	if selector, found, _ := unstructured.NestedMap(spec, "selector", "matchLabels"); found {
		policy.Selector = make(map[string]string)
		for k, v := range selector {
			if vs, ok := v.(string); ok {
				policy.Selector[k] = vs
			}
		}
	}

	// Parse rules
	if rules, found, _ := unstructured.NestedSlice(spec, "rules"); found {
		policy.Rules = make([]AuthorizationRule, 0, len(rules))
		for _, ruleItem := range rules {
			if ruleMap, ok := ruleItem.(map[string]interface{}); ok {
				rule := parseAuthorizationRule(ruleMap)
				policy.Rules = append(policy.Rules, rule)
			}
		}
	}

	// Parse timestamps
	if ts := obj.GetCreationTimestamp(); !ts.IsZero() {
		policy.CreatedAt = ts.Time
	}
	policy.UpdatedAt = time.Now()

	return policy, nil
}

// parseAuthorizationRule parses a single authorization rule
func parseAuthorizationRule(ruleMap map[string]interface{}) AuthorizationRule {
	rule := AuthorizationRule{}

	// Parse from sources
	if fromList, found, _ := unstructured.NestedSlice(ruleMap, "from"); found {
		rule.From = make([]RuleSource, 0, len(fromList))
		for _, fromItem := range fromList {
			if fromMap, ok := fromItem.(map[string]interface{}); ok {
				source := parseRuleSource(fromMap)
				rule.From = append(rule.From, source)
			}
		}
	}

	// Parse to destinations
	if toList, found, _ := unstructured.NestedSlice(ruleMap, "to"); found {
		rule.To = make([]RuleDestination, 0, len(toList))
		for _, toItem := range toList {
			if toMap, ok := toItem.(map[string]interface{}); ok {
				dest := parseRuleDestination(toMap)
				rule.To = append(rule.To, dest)
			}
		}
	}

	// Parse when conditions
	if whenList, found, _ := unstructured.NestedSlice(ruleMap, "when"); found {
		rule.When = make([]RuleCondition, 0, len(whenList))
		for _, whenItem := range whenList {
			if whenMap, ok := whenItem.(map[string]interface{}); ok {
				cond := parseRuleCondition(whenMap)
				rule.When = append(rule.When, cond)
			}
		}
	}

	return rule
}

// parseRuleSource parses source matching criteria
func parseRuleSource(fromMap map[string]interface{}) RuleSource {
	source := RuleSource{}

	if src, found, _ := unstructured.NestedMap(fromMap, "source"); found {
		source.Principals = getStringSlice(src, "principals")
		source.NotPrincipals = getStringSlice(src, "notPrincipals")
		source.RequestPrincipals = getStringSlice(src, "requestPrincipals")
		source.NotRequestPrincipals = getStringSlice(src, "notRequestPrincipals")
		source.Namespaces = getStringSlice(src, "namespaces")
		source.NotNamespaces = getStringSlice(src, "notNamespaces")
		source.IPBlocks = getStringSlice(src, "ipBlocks")
		source.NotIPBlocks = getStringSlice(src, "notIpBlocks")
	}

	return source
}

// parseRuleDestination parses destination matching criteria
func parseRuleDestination(toMap map[string]interface{}) RuleDestination {
	dest := RuleDestination{}

	if operation, found, _ := unstructured.NestedMap(toMap, "operation"); found {
		dest.Hosts = getStringSlice(operation, "hosts")
		dest.NotHosts = getStringSlice(operation, "notHosts")
		dest.Ports = getStringSlice(operation, "ports")
		dest.NotPorts = getStringSlice(operation, "notPorts")
		dest.Methods = getStringSlice(operation, "methods")
		dest.NotMethods = getStringSlice(operation, "notMethods")
		dest.Paths = getStringSlice(operation, "paths")
		dest.NotPaths = getStringSlice(operation, "notPaths")
	}

	return dest
}

// parseRuleCondition parses a rule condition
func parseRuleCondition(whenMap map[string]interface{}) RuleCondition {
	cond := RuleCondition{}

	if key, found, _ := unstructured.NestedString(whenMap, "key"); found {
		cond.Key = key
	}
	cond.Values = getStringSlice(whenMap, "values")
	cond.NotValues = getStringSlice(whenMap, "notValues")

	return cond
}

// parseDestinationRule parses an unstructured DestinationRule
func parseDestinationRule(obj *unstructured.Unstructured) (*DestinationRule, error) {
	rule := &DestinationRule{}

	// Parse spec
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return nil, fmt.Errorf("spec not found")
	}

	// Parse host
	if host, found, _ := unstructured.NestedString(spec, "host"); found {
		rule.Host = host
	}

	// Parse traffic policy
	if tp, found, _ := unstructured.NestedMap(spec, "trafficPolicy"); found {
		rule.TrafficPolicy = parseTrafficPolicy(tp)
	}

	return rule, nil
}

// parseTrafficPolicy parses traffic policy settings
func parseTrafficPolicy(tp map[string]interface{}) *TrafficPolicy {
	policy := &TrafficPolicy{}

	// Parse TLS settings
	if tlsMap, found, _ := unstructured.NestedMap(tp, "tls"); found {
		policy.TLS = &TLSSettings{}
		if mode, found, _ := unstructured.NestedString(tlsMap, "mode"); found {
			policy.TLS.Mode = mode
		}
		if cert, found, _ := unstructured.NestedString(tlsMap, "clientCertificate"); found {
			policy.TLS.ClientCertificate = cert
		}
		if key, found, _ := unstructured.NestedString(tlsMap, "privateKey"); found {
			policy.TLS.PrivateKey = key
		}
		if ca, found, _ := unstructured.NestedString(tlsMap, "caCertificates"); found {
			policy.TLS.CaCertificates = ca
		}
		if sni, found, _ := unstructured.NestedString(tlsMap, "sni"); found {
			policy.TLS.SNI = sni
		}
		policy.TLS.SubjectAltNames = getStringSlice(tlsMap, "subjectAltNames")
	}

	// Parse connection pool
	if cp, found, _ := unstructured.NestedMap(tp, "connectionPool"); found {
		policy.ConnectionPool = &ConnectionPoolSettings{}
		if tcp, found, _ := unstructured.NestedMap(cp, "tcp"); found {
			policy.ConnectionPool.TCP = &TCPSettings{}
			if maxConn, found, _ := unstructured.NestedInt64(tcp, "maxConnections"); found {
				policy.ConnectionPool.TCP.MaxConnections = int(maxConn)
			}
			if timeout, found, _ := unstructured.NestedString(tcp, "connectTimeout"); found {
				if d, err := time.ParseDuration(timeout); err == nil {
					policy.ConnectionPool.TCP.ConnectTimeout = d
				}
			}
		}
		if http, found, _ := unstructured.NestedMap(cp, "http"); found {
			policy.ConnectionPool.HTTP = &HTTPSettings{}
			if val, found, _ := unstructured.NestedInt64(http, "http1MaxPendingRequests"); found {
				policy.ConnectionPool.HTTP.HTTP1MaxPendingRequests = int(val)
			}
			if val, found, _ := unstructured.NestedInt64(http, "http2MaxRequests"); found {
				policy.ConnectionPool.HTTP.HTTP2MaxRequests = int(val)
			}
			if val, found, _ := unstructured.NestedInt64(http, "maxRequestsPerConnection"); found {
				policy.ConnectionPool.HTTP.MaxRequestsPerConnection = int(val)
			}
			if val, found, _ := unstructured.NestedInt64(http, "maxRetries"); found {
				policy.ConnectionPool.HTTP.MaxRetries = int(val)
			}
		}
	}

	// Parse outlier detection
	if od, found, _ := unstructured.NestedMap(tp, "outlierDetection"); found {
		policy.OutlierDetection = &OutlierDetection{}
		if val, found, _ := unstructured.NestedInt64(od, "consecutive5xxErrors"); found {
			policy.OutlierDetection.Consecutive5xxErrors = int(val)
		}
		if val, found, _ := unstructured.NestedInt64(od, "consecutiveGatewayErrors"); found {
			policy.OutlierDetection.ConsecutiveGatewayErrors = int(val)
		}
		if interval, found, _ := unstructured.NestedString(od, "interval"); found {
			if d, err := time.ParseDuration(interval); err == nil {
				policy.OutlierDetection.Interval = d
			}
		}
		if baseTime, found, _ := unstructured.NestedString(od, "baseEjectionTime"); found {
			if d, err := time.ParseDuration(baseTime); err == nil {
				policy.OutlierDetection.BaseEjectionTime = d
			}
		}
		if val, found, _ := unstructured.NestedInt64(od, "maxEjectionPercent"); found {
			policy.OutlierDetection.MaxEjectionPercent = int(val)
		}
	}

	return policy
}

// getStringSlice extracts a string slice from an unstructured map
func getStringSlice(m map[string]interface{}, key string) []string {
	if val, found, _ := unstructured.NestedStringSlice(m, key); found {
		return val
	}
	return nil
}
