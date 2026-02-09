package servicemesh

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestNewIstioCRDClientFromDynamic(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)

	client := NewIstioCRDClientFromDynamic(fakeClient)
	if client == nil {
		t.Fatal("Expected client to be non-nil")
	}
	if client.client != fakeClient {
		t.Error("Expected client.client to match provided dynamic client")
	}
}

func TestIstioCRDClient_ListPeerAuthentications(t *testing.T) {
	scheme := runtime.NewScheme()

	// Create test PeerAuthentication objects
	pa1 := createPeerAuthenticationUnstructured("test-pa-1", "default", "STRICT", "my-service")
	pa2 := createPeerAuthenticationUnstructured("test-pa-2", "default", "PERMISSIVE", "")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR: "PeerAuthenticationList",
		},
		pa1, pa2,
	)

	client := NewIstioCRDClientFromDynamic(fakeClient)

	// Test listing in a specific namespace
	policies, err := client.ListPeerAuthentications(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListPeerAuthentications error = %v", err)
	}

	if len(policies) != 2 {
		t.Errorf("Expected 2 policies, got %d", len(policies))
	}

	// Verify first policy
	var foundStrict, foundPermissive bool
	for _, p := range policies {
		if p.Name == "test-pa-1" {
			foundStrict = true
			if p.Mode != PolicyModeStrict {
				t.Errorf("Expected STRICT mode for test-pa-1, got %s", p.Mode)
			}
			if p.Service != "my-service" {
				t.Errorf("Expected service 'my-service', got %q", p.Service)
			}
		}
		if p.Name == "test-pa-2" {
			foundPermissive = true
			if p.Mode != PolicyModePermissive {
				t.Errorf("Expected PERMISSIVE mode for test-pa-2, got %s", p.Mode)
			}
		}
	}

	if !foundStrict {
		t.Error("test-pa-1 not found in results")
	}
	if !foundPermissive {
		t.Error("test-pa-2 not found in results")
	}
}

func TestIstioCRDClient_ListPeerAuthentications_AllNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()

	pa1 := createPeerAuthenticationUnstructured("test-pa-1", "ns1", "STRICT", "svc1")
	pa2 := createPeerAuthenticationUnstructured("test-pa-2", "ns2", "PERMISSIVE", "svc2")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR: "PeerAuthenticationList",
		},
		pa1, pa2,
	)

	client := NewIstioCRDClientFromDynamic(fakeClient)

	// Test listing all namespaces (empty string)
	policies, err := client.ListPeerAuthentications(context.Background(), "")
	if err != nil {
		t.Fatalf("ListPeerAuthentications error = %v", err)
	}

	if len(policies) != 2 {
		t.Errorf("Expected 2 policies across all namespaces, got %d", len(policies))
	}
}

func TestIstioCRDClient_GetPeerAuthentication(t *testing.T) {
	scheme := runtime.NewScheme()

	pa := createPeerAuthenticationUnstructured("test-pa", "default", "STRICT", "my-service")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR: "PeerAuthenticationList",
		},
		pa,
	)

	client := NewIstioCRDClientFromDynamic(fakeClient)

	// Test getting a specific resource
	policy, err := client.GetPeerAuthentication(context.Background(), "default", "test-pa")
	if err != nil {
		t.Fatalf("GetPeerAuthentication error = %v", err)
	}

	if policy.Name != "test-pa" {
		t.Errorf("Expected name 'test-pa', got %q", policy.Name)
	}
	if policy.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got %q", policy.Namespace)
	}
	if policy.Mode != PolicyModeStrict {
		t.Errorf("Expected STRICT mode, got %s", policy.Mode)
	}
}

func TestIstioCRDClient_GetPeerAuthentication_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)

	client := NewIstioCRDClientFromDynamic(fakeClient)

	_, err := client.GetPeerAuthentication(context.Background(), "default", "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent resource")
	}
}

func TestIstioCRDClient_ListAuthorizationPolicies(t *testing.T) {
	scheme := runtime.NewScheme()

	ap1 := createAuthorizationPolicyUnstructured("test-ap-1", "default", "ALLOW")
	ap2 := createAuthorizationPolicyUnstructured("test-ap-2", "default", "DENY")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
		},
		ap1, ap2,
	)

	client := NewIstioCRDClientFromDynamic(fakeClient)

	policies, err := client.ListAuthorizationPolicies(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListAuthorizationPolicies error = %v", err)
	}

	if len(policies) != 2 {
		t.Errorf("Expected 2 policies, got %d", len(policies))
	}

	var foundAllow, foundDeny bool
	for _, p := range policies {
		if p.Name == "test-ap-1" && p.Action == AuthorizationActionAllow {
			foundAllow = true
		}
		if p.Name == "test-ap-2" && p.Action == AuthorizationActionDeny {
			foundDeny = true
		}
	}

	if !foundAllow {
		t.Error("ALLOW policy not found")
	}
	if !foundDeny {
		t.Error("DENY policy not found")
	}
}

func TestIstioCRDClient_GetAuthorizationPolicy(t *testing.T) {
	scheme := runtime.NewScheme()

	ap := createAuthorizationPolicyUnstructured("test-ap", "default", "ALLOW")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
		},
		ap,
	)

	client := NewIstioCRDClientFromDynamic(fakeClient)

	policy, err := client.GetAuthorizationPolicy(context.Background(), "default", "test-ap")
	if err != nil {
		t.Fatalf("GetAuthorizationPolicy error = %v", err)
	}

	if policy.Name != "test-ap" {
		t.Errorf("Expected name 'test-ap', got %q", policy.Name)
	}
	if policy.Action != AuthorizationActionAllow {
		t.Errorf("Expected ALLOW action, got %s", policy.Action)
	}
}

func TestIstioCRDClient_ListDestinationRules(t *testing.T) {
	scheme := runtime.NewScheme()

	dr := createDestinationRuleUnstructured("test-dr", "default", "my-service.default.svc.cluster.local")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			DestinationRuleGVR: "DestinationRuleList",
		},
		dr,
	)

	client := NewIstioCRDClientFromDynamic(fakeClient)

	rules, err := client.ListDestinationRules(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListDestinationRules error = %v", err)
	}

	if len(rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rules))
	}

	if rules[0].Host != "my-service.default.svc.cluster.local" {
		t.Errorf("Expected host 'my-service.default.svc.cluster.local', got %q", rules[0].Host)
	}
}

func TestParsePeerAuthentication(t *testing.T) {
	tests := []struct {
		name         string
		obj          *unstructured.Unstructured
		expectedMode PolicyMode
		expectedSvc  string
	}{
		{
			name:         "strict mode with service",
			obj:          createPeerAuthenticationUnstructured("pa1", "default", "STRICT", "my-service"),
			expectedMode: PolicyModeStrict,
			expectedSvc:  "my-service",
		},
		{
			name:         "permissive mode namespace-wide",
			obj:          createPeerAuthenticationUnstructured("pa2", "default", "PERMISSIVE", ""),
			expectedMode: PolicyModePermissive,
			expectedSvc:  "",
		},
		{
			name:         "disable mode",
			obj:          createPeerAuthenticationUnstructured("pa3", "default", "DISABLE", "svc"),
			expectedMode: PolicyModeDisable,
			expectedSvc:  "svc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := parsePeerAuthentication(tt.obj)
			if err != nil {
				t.Fatalf("parsePeerAuthentication error = %v", err)
			}

			if policy.Mode != tt.expectedMode {
				t.Errorf("Mode = %s, want %s", policy.Mode, tt.expectedMode)
			}
			if policy.Service != tt.expectedSvc {
				t.Errorf("Service = %q, want %q", policy.Service, tt.expectedSvc)
			}
		})
	}
}

func TestParsePeerAuthentication_WithPortLevelMtls(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "security.istio.io/v1",
			"kind":       "PeerAuthentication",
			"metadata": map[string]interface{}{
				"name":              "pa-with-ports",
				"namespace":         "default",
				"creationTimestamp": time.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"mtls": map[string]interface{}{
					"mode": "PERMISSIVE",
				},
				"portLevelMtls": []interface{}{
					map[string]interface{}{
						"port": int64(443),
						"mtls": map[string]interface{}{
							"mode": "STRICT",
						},
					},
					map[string]interface{}{
						"port": int64(80),
						"mtls": map[string]interface{}{
							"mode": "DISABLE",
						},
					},
				},
			},
		},
	}

	policy, err := parsePeerAuthentication(obj)
	if err != nil {
		t.Fatalf("parsePeerAuthentication error = %v", err)
	}

	if policy.PeerAuthentication == nil {
		t.Fatal("Expected PeerAuthentication to be set")
	}

	if len(policy.PeerAuthentication.PortLevelMtls) != 2 {
		t.Errorf("Expected 2 port-level entries, got %d", len(policy.PeerAuthentication.PortLevelMtls))
	}

	if policy.PeerAuthentication.PortLevelMtls[443] != PolicyModeStrict {
		t.Errorf("Port 443 mode = %s, want STRICT", policy.PeerAuthentication.PortLevelMtls[443])
	}

	if policy.PeerAuthentication.PortLevelMtls[80] != PolicyModeDisable {
		t.Errorf("Port 80 mode = %s, want DISABLE", policy.PeerAuthentication.PortLevelMtls[80])
	}
}

func TestParseAuthorizationPolicy(t *testing.T) {
	obj := createAuthorizationPolicyUnstructured("test-ap", "default", "DENY")

	policy, err := parseAuthorizationPolicy(obj)
	if err != nil {
		t.Fatalf("parseAuthorizationPolicy error = %v", err)
	}

	if policy.Name != "test-ap" {
		t.Errorf("Name = %q, want 'test-ap'", policy.Name)
	}
	if policy.Action != AuthorizationActionDeny {
		t.Errorf("Action = %s, want DENY", policy.Action)
	}
}

func TestParseAuthorizationPolicy_WithRules(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "security.istio.io/v1",
			"kind":       "AuthorizationPolicy",
			"metadata": map[string]interface{}{
				"name":      "ap-with-rules",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"action": "ALLOW",
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": "my-app",
					},
				},
				"rules": []interface{}{
					map[string]interface{}{
						"from": []interface{}{
							map[string]interface{}{
								"source": map[string]interface{}{
									"principals":  []interface{}{"cluster.local/ns/default/sa/frontend"},
									"namespaces":  []interface{}{"default", "production"},
									"ipBlocks":    []interface{}{"10.0.0.0/8"},
									"notIpBlocks": []interface{}{"10.0.1.0/24"},
								},
							},
						},
						"to": []interface{}{
							map[string]interface{}{
								"operation": map[string]interface{}{
									"methods": []interface{}{"GET", "POST"},
									"paths":   []interface{}{"/api/*"},
									"ports":   []interface{}{"8080"},
								},
							},
						},
						"when": []interface{}{
							map[string]interface{}{
								"key":    "request.headers[x-custom-header]",
								"values": []interface{}{"allowed-value"},
							},
						},
					},
				},
			},
		},
	}

	policy, err := parseAuthorizationPolicy(obj)
	if err != nil {
		t.Fatalf("parseAuthorizationPolicy error = %v", err)
	}

	if policy.Selector["app"] != "my-app" {
		t.Errorf("Selector app = %q, want 'my-app'", policy.Selector["app"])
	}

	if len(policy.Rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(policy.Rules))
	}

	rule := policy.Rules[0]

	// Check from sources
	if len(rule.From) != 1 {
		t.Fatalf("Expected 1 source, got %d", len(rule.From))
	}
	source := rule.From[0]
	if len(source.Principals) != 1 || source.Principals[0] != "cluster.local/ns/default/sa/frontend" {
		t.Errorf("Principals = %v, want [cluster.local/ns/default/sa/frontend]", source.Principals)
	}
	if len(source.Namespaces) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(source.Namespaces))
	}
	if len(source.IPBlocks) != 1 || source.IPBlocks[0] != "10.0.0.0/8" {
		t.Errorf("IPBlocks = %v, want [10.0.0.0/8]", source.IPBlocks)
	}
	if len(source.NotIPBlocks) != 1 || source.NotIPBlocks[0] != "10.0.1.0/24" {
		t.Errorf("NotIPBlocks = %v, want [10.0.1.0/24]", source.NotIPBlocks)
	}

	// Check to destinations
	if len(rule.To) != 1 {
		t.Fatalf("Expected 1 destination, got %d", len(rule.To))
	}
	dest := rule.To[0]
	if len(dest.Methods) != 2 {
		t.Errorf("Expected 2 methods, got %d", len(dest.Methods))
	}
	if len(dest.Paths) != 1 || dest.Paths[0] != "/api/*" {
		t.Errorf("Paths = %v, want [/api/*]", dest.Paths)
	}

	// Check when conditions
	if len(rule.When) != 1 {
		t.Fatalf("Expected 1 condition, got %d", len(rule.When))
	}
	cond := rule.When[0]
	if cond.Key != "request.headers[x-custom-header]" {
		t.Errorf("Condition key = %q, want 'request.headers[x-custom-header]'", cond.Key)
	}
}

func TestParseDestinationRule(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1",
			"kind":       "DestinationRule",
			"metadata": map[string]interface{}{
				"name":      "test-dr",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"host": "my-service.default.svc.cluster.local",
				"trafficPolicy": map[string]interface{}{
					"tls": map[string]interface{}{
						"mode":              "ISTIO_MUTUAL",
						"clientCertificate": "/certs/cert.pem",
						"privateKey":        "/certs/key.pem",
						"caCertificates":    "/certs/ca.pem",
						"sni":               "my-service.default.svc.cluster.local",
						"subjectAltNames":   []interface{}{"my-service"},
					},
					"connectionPool": map[string]interface{}{
						"tcp": map[string]interface{}{
							"maxConnections": int64(100),
							"connectTimeout": "5s",
						},
						"http": map[string]interface{}{
							"http1MaxPendingRequests":  int64(100),
							"http2MaxRequests":         int64(1000),
							"maxRequestsPerConnection": int64(10),
							"maxRetries":               int64(3),
						},
					},
					"outlierDetection": map[string]interface{}{
						"consecutive5xxErrors":     int64(5),
						"consecutiveGatewayErrors": int64(3),
						"interval":                 "10s",
						"baseEjectionTime":         "30s",
						"maxEjectionPercent":       int64(10),
					},
				},
			},
		},
	}

	rule, err := parseDestinationRule(obj)
	if err != nil {
		t.Fatalf("parseDestinationRule error = %v", err)
	}

	if rule.Host != "my-service.default.svc.cluster.local" {
		t.Errorf("Host = %q, want 'my-service.default.svc.cluster.local'", rule.Host)
	}

	if rule.TrafficPolicy == nil {
		t.Fatal("Expected TrafficPolicy to be set")
	}

	// Check TLS settings
	tls := rule.TrafficPolicy.TLS
	if tls == nil {
		t.Fatal("Expected TLS settings to be set")
	}
	if tls.Mode != "ISTIO_MUTUAL" {
		t.Errorf("TLS mode = %q, want 'ISTIO_MUTUAL'", tls.Mode)
	}
	if tls.ClientCertificate != "/certs/cert.pem" {
		t.Errorf("ClientCertificate = %q, want '/certs/cert.pem'", tls.ClientCertificate)
	}
	if tls.SNI != "my-service.default.svc.cluster.local" {
		t.Errorf("SNI = %q, want 'my-service.default.svc.cluster.local'", tls.SNI)
	}

	// Check connection pool
	cp := rule.TrafficPolicy.ConnectionPool
	if cp == nil {
		t.Fatal("Expected ConnectionPool to be set")
	}
	if cp.TCP == nil {
		t.Fatal("Expected TCP settings to be set")
	}
	if cp.TCP.MaxConnections != 100 {
		t.Errorf("TCP MaxConnections = %d, want 100", cp.TCP.MaxConnections)
	}
	if cp.TCP.ConnectTimeout != 5*time.Second {
		t.Errorf("TCP ConnectTimeout = %v, want 5s", cp.TCP.ConnectTimeout)
	}
	if cp.HTTP == nil {
		t.Fatal("Expected HTTP settings to be set")
	}
	if cp.HTTP.HTTP2MaxRequests != 1000 {
		t.Errorf("HTTP2MaxRequests = %d, want 1000", cp.HTTP.HTTP2MaxRequests)
	}

	// Check outlier detection
	od := rule.TrafficPolicy.OutlierDetection
	if od == nil {
		t.Fatal("Expected OutlierDetection to be set")
	}
	if od.Consecutive5xxErrors != 5 {
		t.Errorf("Consecutive5xxErrors = %d, want 5", od.Consecutive5xxErrors)
	}
	if od.Interval != 10*time.Second {
		t.Errorf("Interval = %v, want 10s", od.Interval)
	}
	if od.BaseEjectionTime != 30*time.Second {
		t.Errorf("BaseEjectionTime = %v, want 30s", od.BaseEjectionTime)
	}
}

func TestGetStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		key      string
		expected []string
	}{
		{
			name:     "existing slice",
			input:    map[string]interface{}{"values": []interface{}{"a", "b", "c"}},
			key:      "values",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "non-existent key",
			input:    map[string]interface{}{"other": []interface{}{"x"}},
			key:      "values",
			expected: nil,
		},
		{
			name:     "empty map",
			input:    map[string]interface{}{},
			key:      "values",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStringSlice(tt.input, tt.key)
			if len(result) != len(tt.expected) {
				t.Errorf("len(result) = %d, want %d", len(result), len(tt.expected))
			}
		})
	}
}

func TestGVRDefinitions(t *testing.T) {
	// Verify GVR definitions are correct
	if PeerAuthenticationGVR.Group != "security.istio.io" {
		t.Errorf("PeerAuthenticationGVR.Group = %q, want 'security.istio.io'", PeerAuthenticationGVR.Group)
	}
	if PeerAuthenticationGVR.Version != "v1" {
		t.Errorf("PeerAuthenticationGVR.Version = %q, want 'v1'", PeerAuthenticationGVR.Version)
	}
	if PeerAuthenticationGVR.Resource != "peerauthentications" {
		t.Errorf("PeerAuthenticationGVR.Resource = %q, want 'peerauthentications'", PeerAuthenticationGVR.Resource)
	}

	if AuthorizationPolicyGVR.Group != "security.istio.io" {
		t.Errorf("AuthorizationPolicyGVR.Group = %q, want 'security.istio.io'", AuthorizationPolicyGVR.Group)
	}
	if AuthorizationPolicyGVR.Resource != "authorizationpolicies" {
		t.Errorf("AuthorizationPolicyGVR.Resource = %q, want 'authorizationpolicies'", AuthorizationPolicyGVR.Resource)
	}

	if DestinationRuleGVR.Group != "networking.istio.io" {
		t.Errorf("DestinationRuleGVR.Group = %q, want 'networking.istio.io'", DestinationRuleGVR.Group)
	}
	if DestinationRuleGVR.Resource != "destinationrules" {
		t.Errorf("DestinationRuleGVR.Resource = %q, want 'destinationrules'", DestinationRuleGVR.Resource)
	}
}

// Helper functions to create test unstructured objects

func createPeerAuthenticationUnstructured(name, namespace, mode, service string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "security.istio.io/v1",
			"kind":       "PeerAuthentication",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         namespace,
				"creationTimestamp": metav1.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"mtls": map[string]interface{}{
					"mode": mode,
				},
			},
		},
	}

	if service != "" {
		spec := obj.Object["spec"].(map[string]interface{})
		spec["selector"] = map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": service,
			},
		}
	}

	return obj
}

func createAuthorizationPolicyUnstructured(name, namespace, action string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "security.istio.io/v1",
			"kind":       "AuthorizationPolicy",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         namespace,
				"creationTimestamp": metav1.Now().Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"action": action,
			},
		},
	}
}

func createDestinationRuleUnstructured(name, namespace, host string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1",
			"kind":       "DestinationRule",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"host": host,
			},
		},
	}
}
