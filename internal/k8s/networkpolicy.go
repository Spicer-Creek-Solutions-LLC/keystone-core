// Package k8s provides Kubernetes integration for Keystone.
package k8s

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// PolicyType represents the type of network policy.
type PolicyType string

const (
	// PolicyTypeIngress controls incoming traffic.
	PolicyTypeIngress PolicyType = "Ingress"
	// PolicyTypeEgress controls outgoing traffic.
	PolicyTypeEgress PolicyType = "Egress"
)

// Protocol represents a network protocol.
type Protocol string

const (
	// ProtocolTCP represents TCP protocol.
	ProtocolTCP Protocol = "TCP"
	// ProtocolUDP represents UDP protocol.
	ProtocolUDP Protocol = "UDP"
	// ProtocolSCTP represents SCTP protocol.
	ProtocolSCTP Protocol = "SCTP"
)

// LabelSelector selects resources by labels.
type LabelSelector struct {
	MatchLabels      map[string]string          `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// LabelSelectorRequirement is a selector that contains values, a key, and an operator.
type LabelSelectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"` // In, NotIn, Exists, DoesNotExist
	Values   []string `json:"values,omitempty"`
}

// NetworkPolicyPort describes a port to allow traffic on.
type NetworkPolicyPort struct {
	Protocol Protocol `json:"protocol,omitempty"`
	Port     int32    `json:"port,omitempty"`
	EndPort  int32    `json:"endPort,omitempty"` // For port ranges
}

// IPBlock describes a particular CIDR.
type IPBlock struct {
	CIDR   string   `json:"cidr"`
	Except []string `json:"except,omitempty"`
}

// NetworkPolicyPeer describes a peer to allow traffic to/from.
type NetworkPolicyPeer struct {
	PodSelector       *LabelSelector `json:"podSelector,omitempty"`
	NamespaceSelector *LabelSelector `json:"namespaceSelector,omitempty"`
	IPBlock           *IPBlock       `json:"ipBlock,omitempty"`
}

// NetworkPolicyIngressRule describes a rule for ingress traffic.
type NetworkPolicyIngressRule struct {
	Ports []NetworkPolicyPort `json:"ports,omitempty"`
	From  []NetworkPolicyPeer `json:"from,omitempty"`
}

// NetworkPolicyEgressRule describes a rule for egress traffic.
type NetworkPolicyEgressRule struct {
	Ports []NetworkPolicyPort `json:"ports,omitempty"`
	To    []NetworkPolicyPeer `json:"to,omitempty"`
}

// NetworkPolicySpec defines the desired state of NetworkPolicy.
type NetworkPolicySpec struct {
	PodSelector LabelSelector              `json:"podSelector"`
	Ingress     []NetworkPolicyIngressRule `json:"ingress,omitempty"`
	Egress      []NetworkPolicyEgressRule  `json:"egress,omitempty"`
	PolicyTypes []PolicyType               `json:"policyTypes,omitempty"`
}

// NetworkPolicy represents a Kubernetes NetworkPolicy.
type NetworkPolicy struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Spec        NetworkPolicySpec `json:"spec"`
	CreatedAt   time.Time         `json:"createdAt,omitempty"`
	UpdatedAt   time.Time         `json:"updatedAt,omitempty"`
}

// Hash returns a hash of the policy spec for comparison.
func (np *NetworkPolicy) Hash() string {
	data, _ := json.Marshal(np.Spec)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:8])
}

// PolicyTemplate represents a reusable network policy template.
type PolicyTemplate struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"` // deny-all, allow-same-namespace, custom, etc.
	Template    NetworkPolicySpec      `json:"template"`
	Parameters  []TemplateParameter    `json:"parameters,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TemplateParameter defines a parameter for policy templates.
type TemplateParameter struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Type        string      `json:"type"` // string, int, bool, []string
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
}

// PolicyValidationResult contains the result of policy validation.
type PolicyValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// PolicyDiff represents changes between two policies.
type PolicyDiff struct {
	Added   []*NetworkPolicy `json:"added,omitempty"`
	Removed []*NetworkPolicy `json:"removed,omitempty"`
	Changed []PolicyChange   `json:"changed,omitempty"`
}

// PolicyChange represents a change to a single policy.
type PolicyChange struct {
	Name      string         `json:"name"`
	Namespace string         `json:"namespace"`
	Old       *NetworkPolicy `json:"old"`
	New       *NetworkPolicy `json:"new"`
}

// PolicyStore is the interface for storing and retrieving policies.
type PolicyStore interface {
	Get(ctx context.Context, namespace, name string) (*NetworkPolicy, error)
	List(ctx context.Context, namespace string) ([]*NetworkPolicy, error)
	Create(ctx context.Context, policy *NetworkPolicy) error
	Update(ctx context.Context, policy *NetworkPolicy) error
	Delete(ctx context.Context, namespace, name string) error
}

// PolicyManager manages network policies across namespaces.
type PolicyManager struct {
	store     PolicyStore
	templates map[string]*PolicyTemplate
	listeners []PolicyListener
	mu        sync.RWMutex
}

// PolicyListener is called when policy changes occur.
type PolicyListener func(event *PolicyEvent)

// PolicyEvent represents a policy change event.
type PolicyEvent struct {
	Type      string         `json:"type"` // created, updated, deleted
	Policy    *NetworkPolicy `json:"policy"`
	Namespace string         `json:"namespace"`
	Timestamp time.Time      `json:"timestamp"`
}

// NewPolicyManager creates a new policy manager.
func NewPolicyManager(store PolicyStore) *PolicyManager {
	pm := &PolicyManager{
		store:     store,
		templates: make(map[string]*PolicyTemplate),
	}

	// Register built-in templates
	pm.registerBuiltinTemplates()

	return pm
}

// registerBuiltinTemplates registers common policy templates.
func (pm *PolicyManager) registerBuiltinTemplates() {
	pm.templates["deny-all-ingress"] = &PolicyTemplate{
		Name:        "deny-all-ingress",
		Description: "Deny all ingress traffic to selected pods",
		Category:    "deny-all",
		Template: NetworkPolicySpec{
			PodSelector: LabelSelector{},
			PolicyTypes: []PolicyType{PolicyTypeIngress},
			Ingress:     []NetworkPolicyIngressRule{},
		},
	}

	pm.templates["deny-all-egress"] = &PolicyTemplate{
		Name:        "deny-all-egress",
		Description: "Deny all egress traffic from selected pods",
		Category:    "deny-all",
		Template: NetworkPolicySpec{
			PodSelector: LabelSelector{},
			PolicyTypes: []PolicyType{PolicyTypeEgress},
			Egress:      []NetworkPolicyEgressRule{},
		},
	}

	pm.templates["deny-all"] = &PolicyTemplate{
		Name:        "deny-all",
		Description: "Deny all ingress and egress traffic",
		Category:    "deny-all",
		Template: NetworkPolicySpec{
			PodSelector: LabelSelector{},
			PolicyTypes: []PolicyType{PolicyTypeIngress, PolicyTypeEgress},
			Ingress:     []NetworkPolicyIngressRule{},
			Egress:      []NetworkPolicyEgressRule{},
		},
	}

	pm.templates["allow-same-namespace"] = &PolicyTemplate{
		Name:        "allow-same-namespace",
		Description: "Allow traffic only from the same namespace",
		Category:    "namespace-isolation",
		Template: NetworkPolicySpec{
			PodSelector: LabelSelector{},
			PolicyTypes: []PolicyType{PolicyTypeIngress},
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
	}

	pm.templates["allow-dns"] = &PolicyTemplate{
		Name:        "allow-dns",
		Description: "Allow egress to DNS (kube-dns/coredns)",
		Category:    "infrastructure",
		Template: NetworkPolicySpec{
			PodSelector: LabelSelector{},
			PolicyTypes: []PolicyType{PolicyTypeEgress},
			Egress: []NetworkPolicyEgressRule{
				{
					To: []NetworkPolicyPeer{
						{
							NamespaceSelector: &LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "kube-system",
								},
							},
							PodSelector: &LabelSelector{
								MatchLabels: map[string]string{
									"k8s-app": "kube-dns",
								},
							},
						},
					},
					Ports: []NetworkPolicyPort{
						{Protocol: ProtocolUDP, Port: 53},
						{Protocol: ProtocolTCP, Port: 53},
					},
				},
			},
		},
	}

	pm.templates["allow-monitoring"] = &PolicyTemplate{
		Name:        "allow-monitoring",
		Description: "Allow ingress from monitoring namespace (Prometheus)",
		Category:    "infrastructure",
		Template: NetworkPolicySpec{
			PodSelector: LabelSelector{},
			PolicyTypes: []PolicyType{PolicyTypeIngress},
			Ingress: []NetworkPolicyIngressRule{
				{
					From: []NetworkPolicyPeer{
						{
							NamespaceSelector: &LabelSelector{
								MatchLabels: map[string]string{
									"name": "monitoring",
								},
							},
						},
					},
				},
			},
		},
	}

	pm.templates["web-tier"] = &PolicyTemplate{
		Name:        "web-tier",
		Description: "Allow HTTP/HTTPS ingress from anywhere, egress to app tier only",
		Category:    "tiered-architecture",
		Template: NetworkPolicySpec{
			PodSelector: LabelSelector{
				MatchLabels: map[string]string{
					"tier": "web",
				},
			},
			PolicyTypes: []PolicyType{PolicyTypeIngress, PolicyTypeEgress},
			Ingress: []NetworkPolicyIngressRule{
				{
					Ports: []NetworkPolicyPort{
						{Protocol: ProtocolTCP, Port: 80},
						{Protocol: ProtocolTCP, Port: 443},
					},
				},
			},
			Egress: []NetworkPolicyEgressRule{
				{
					To: []NetworkPolicyPeer{
						{
							PodSelector: &LabelSelector{
								MatchLabels: map[string]string{
									"tier": "app",
								},
							},
						},
					},
				},
			},
		},
	}

	pm.templates["database-tier"] = &PolicyTemplate{
		Name:        "database-tier",
		Description: "Allow connections only from app tier to database ports",
		Category:    "tiered-architecture",
		Template: NetworkPolicySpec{
			PodSelector: LabelSelector{
				MatchLabels: map[string]string{
					"tier": "database",
				},
			},
			PolicyTypes: []PolicyType{PolicyTypeIngress},
			Ingress: []NetworkPolicyIngressRule{
				{
					From: []NetworkPolicyPeer{
						{
							PodSelector: &LabelSelector{
								MatchLabels: map[string]string{
									"tier": "app",
								},
							},
						},
					},
					Ports: []NetworkPolicyPort{
						{Protocol: ProtocolTCP, Port: 5432},  // PostgreSQL
						{Protocol: ProtocolTCP, Port: 3306},  // MySQL
						{Protocol: ProtocolTCP, Port: 27017}, // MongoDB
					},
				},
			},
		},
	}
}

// RegisterTemplate registers a custom policy template.
func (pm *PolicyManager) RegisterTemplate(template *PolicyTemplate) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.templates[template.Name] = template
}

// GetTemplate retrieves a policy template by name.
func (pm *PolicyManager) GetTemplate(name string) (*PolicyTemplate, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	t, ok := pm.templates[name]
	return t, ok
}

// ListTemplates returns all registered templates.
func (pm *PolicyManager) ListTemplates() []*PolicyTemplate {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	templates := make([]*PolicyTemplate, 0, len(pm.templates))
	for _, t := range pm.templates {
		templates = append(templates, t)
	}
	return templates
}

// AddListener adds a policy event listener.
func (pm *PolicyManager) AddListener(listener PolicyListener) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.listeners = append(pm.listeners, listener)
}

// emit sends an event to all listeners.
func (pm *PolicyManager) emit(event *PolicyEvent) {
	pm.mu.RLock()
	listeners := make([]PolicyListener, len(pm.listeners))
	copy(listeners, pm.listeners)
	pm.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// CreateFromTemplate creates a policy from a template.
func (pm *PolicyManager) CreateFromTemplate(ctx context.Context, templateName, namespace, policyName string, overrides *LabelSelector) (*NetworkPolicy, error) {
	template, ok := pm.GetTemplate(templateName)
	if !ok {
		return nil, fmt.Errorf("template not found: %s", templateName)
	}

	// Clone the template spec
	specData, _ := json.Marshal(template.Template)
	var spec NetworkPolicySpec
	_ = json.Unmarshal(specData, &spec)

	// Apply pod selector override if provided
	if overrides != nil {
		spec.PodSelector = *overrides
	}

	policy := &NetworkPolicy{
		Name:      policyName,
		Namespace: namespace,
		Labels: map[string]string{
			"keystone.io/managed":  "true",
			"keystone.io/template": templateName,
		},
		Spec:      spec,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := pm.store.Create(ctx, policy); err != nil {
		return nil, err
	}

	pm.emit(&PolicyEvent{
		Type:      "created",
		Policy:    policy,
		Namespace: namespace,
		Timestamp: time.Now(),
	})

	return policy, nil
}

// Create creates a new network policy.
func (pm *PolicyManager) Create(ctx context.Context, policy *NetworkPolicy) error {
	policy.CreatedAt = time.Now()
	policy.UpdatedAt = time.Now()

	if err := pm.store.Create(ctx, policy); err != nil {
		return err
	}

	pm.emit(&PolicyEvent{
		Type:      "created",
		Policy:    policy,
		Namespace: policy.Namespace,
		Timestamp: time.Now(),
	})

	return nil
}

// Update updates an existing network policy.
func (pm *PolicyManager) Update(ctx context.Context, policy *NetworkPolicy) error {
	policy.UpdatedAt = time.Now()

	if err := pm.store.Update(ctx, policy); err != nil {
		return err
	}

	pm.emit(&PolicyEvent{
		Type:      "updated",
		Policy:    policy,
		Namespace: policy.Namespace,
		Timestamp: time.Now(),
	})

	return nil
}

// Delete deletes a network policy.
func (pm *PolicyManager) Delete(ctx context.Context, namespace, name string) error {
	policy, err := pm.store.Get(ctx, namespace, name)
	if err != nil {
		return err
	}

	if err := pm.store.Delete(ctx, namespace, name); err != nil {
		return err
	}

	pm.emit(&PolicyEvent{
		Type:      "deleted",
		Policy:    policy,
		Namespace: namespace,
		Timestamp: time.Now(),
	})

	return nil
}

// Get retrieves a network policy.
func (pm *PolicyManager) Get(ctx context.Context, namespace, name string) (*NetworkPolicy, error) {
	return pm.store.Get(ctx, namespace, name)
}

// List lists network policies in a namespace.
func (pm *PolicyManager) List(ctx context.Context, namespace string) ([]*NetworkPolicy, error) {
	return pm.store.List(ctx, namespace)
}

// Validate validates a network policy.
func (pm *PolicyManager) Validate(policy *NetworkPolicy) *PolicyValidationResult {
	result := &PolicyValidationResult{Valid: true}

	// Check name
	if policy.Name == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "name is required")
	}

	// Check namespace
	if policy.Namespace == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "namespace is required")
	}

	// Validate policy types
	for _, pt := range policy.Spec.PolicyTypes {
		if pt != PolicyTypeIngress && pt != PolicyTypeEgress {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("invalid policy type: %s", pt))
		}
	}

	// Validate ingress rules
	for i, rule := range policy.Spec.Ingress {
		for j, port := range rule.Ports {
			if port.Port < 0 || port.Port > 65535 {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("ingress[%d].ports[%d]: invalid port number: %d", i, j, port.Port))
			}
			if port.EndPort != 0 && port.EndPort < port.Port {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("ingress[%d].ports[%d]: endPort must be >= port", i, j))
			}
		}
	}

	// Validate egress rules
	for i, rule := range policy.Spec.Egress {
		for j, port := range rule.Ports {
			if port.Port < 0 || port.Port > 65535 {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("egress[%d].ports[%d]: invalid port number: %d", i, j, port.Port))
			}
		}
	}

	// Add warnings
	if len(policy.Spec.PolicyTypes) == 0 {
		result.Warnings = append(result.Warnings, "no policy types specified, defaulting to Ingress")
	}

	hasIngress := false
	hasEgress := false
	for _, pt := range policy.Spec.PolicyTypes {
		if pt == PolicyTypeIngress {
			hasIngress = true
		}
		if pt == PolicyTypeEgress {
			hasEgress = true
		}
	}

	if hasIngress && len(policy.Spec.Ingress) == 0 {
		result.Warnings = append(result.Warnings, "Ingress type specified but no ingress rules defined - this will deny all ingress traffic")
	}

	if hasEgress && len(policy.Spec.Egress) == 0 {
		result.Warnings = append(result.Warnings, "Egress type specified but no egress rules defined - this will deny all egress traffic")
	}

	return result
}

// Diff computes the difference between current and desired policies.
func (pm *PolicyManager) Diff(ctx context.Context, namespace string, desired []*NetworkPolicy) (*PolicyDiff, error) {
	current, err := pm.store.List(ctx, namespace)
	if err != nil {
		return nil, err
	}

	diff := &PolicyDiff{}

	currentMap := make(map[string]*NetworkPolicy)
	for _, p := range current {
		currentMap[p.Name] = p
	}

	desiredMap := make(map[string]*NetworkPolicy)
	for _, p := range desired {
		desiredMap[p.Name] = p
	}

	// Find added and changed
	for name, desiredPolicy := range desiredMap {
		if currentPolicy, exists := currentMap[name]; exists {
			if currentPolicy.Hash() != desiredPolicy.Hash() {
				diff.Changed = append(diff.Changed, PolicyChange{
					Name:      name,
					Namespace: namespace,
					Old:       currentPolicy,
					New:       desiredPolicy,
				})
			}
		} else {
			diff.Added = append(diff.Added, desiredPolicy)
		}
	}

	// Find removed
	for name, currentPolicy := range currentMap {
		if _, exists := desiredMap[name]; !exists {
			diff.Removed = append(diff.Removed, currentPolicy)
		}
	}

	return diff, nil
}

// ApplyDiff applies a policy diff.
func (pm *PolicyManager) ApplyDiff(ctx context.Context, diff *PolicyDiff) error {
	// Create new policies
	for _, policy := range diff.Added {
		if err := pm.Create(ctx, policy); err != nil {
			return fmt.Errorf("failed to create policy %s: %w", policy.Name, err)
		}
	}

	// Update changed policies
	for _, change := range diff.Changed {
		if err := pm.Update(ctx, change.New); err != nil {
			return fmt.Errorf("failed to update policy %s: %w", change.Name, err)
		}
	}

	// Delete removed policies
	for _, policy := range diff.Removed {
		if err := pm.Delete(ctx, policy.Namespace, policy.Name); err != nil {
			return fmt.Errorf("failed to delete policy %s: %w", policy.Name, err)
		}
	}

	return nil
}

// InMemoryPolicyStore is an in-memory implementation of PolicyStore.
type InMemoryPolicyStore struct {
	policies map[string]map[string]*NetworkPolicy // namespace -> name -> policy
	mu       sync.RWMutex
}

// NewInMemoryPolicyStore creates a new in-memory policy store.
func NewInMemoryPolicyStore() *InMemoryPolicyStore {
	return &InMemoryPolicyStore{
		policies: make(map[string]map[string]*NetworkPolicy),
	}
}

// Get retrieves a policy by namespace and name.
func (s *InMemoryPolicyStore) Get(_ context.Context, namespace, name string) (*NetworkPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nsPolicies, ok := s.policies[namespace]
	if !ok {
		return nil, fmt.Errorf("policy not found: %s/%s", namespace, name)
	}

	policy, ok := nsPolicies[name]
	if !ok {
		return nil, fmt.Errorf("policy not found: %s/%s", namespace, name)
	}

	// Return a copy
	data, _ := json.Marshal(policy)
	var copied NetworkPolicy
	_ = json.Unmarshal(data, &copied)
	return &copied, nil
}

// List lists policies in a namespace.
func (s *InMemoryPolicyStore) List(_ context.Context, namespace string) ([]*NetworkPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if namespace == "" {
		// Return all policies from all namespaces
		// Count total policies first
		total := 0
		for _, nsPolicies := range s.policies {
			total += len(nsPolicies)
		}
		result := make([]*NetworkPolicy, 0, total)
		for _, nsPolicies := range s.policies {
			for _, policy := range nsPolicies {
				data, _ := json.Marshal(policy)
				var copied NetworkPolicy
				_ = json.Unmarshal(data, &copied)
				result = append(result, &copied)
			}
		}
		return result, nil
	}

	nsPolicies, ok := s.policies[namespace]
	if !ok {
		return []*NetworkPolicy{}, nil
	}

	result := make([]*NetworkPolicy, 0, len(nsPolicies))
	for _, policy := range nsPolicies {
		data, _ := json.Marshal(policy)
		var copied NetworkPolicy
		_ = json.Unmarshal(data, &copied)
		result = append(result, &copied)
	}

	return result, nil
}

// Create creates a new policy.
func (s *InMemoryPolicyStore) Create(_ context.Context, policy *NetworkPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.policies[policy.Namespace] == nil {
		s.policies[policy.Namespace] = make(map[string]*NetworkPolicy)
	}

	if _, exists := s.policies[policy.Namespace][policy.Name]; exists {
		return fmt.Errorf("policy already exists: %s/%s", policy.Namespace, policy.Name)
	}

	// Store a copy
	data, _ := json.Marshal(policy)
	var copied NetworkPolicy
	_ = json.Unmarshal(data, &copied)
	s.policies[policy.Namespace][policy.Name] = &copied

	return nil
}

// Update updates an existing policy.
func (s *InMemoryPolicyStore) Update(_ context.Context, policy *NetworkPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.policies[policy.Namespace] == nil {
		return fmt.Errorf("policy not found: %s/%s", policy.Namespace, policy.Name)
	}

	if _, exists := s.policies[policy.Namespace][policy.Name]; !exists {
		return fmt.Errorf("policy not found: %s/%s", policy.Namespace, policy.Name)
	}

	// Store a copy
	data, _ := json.Marshal(policy)
	var copied NetworkPolicy
	_ = json.Unmarshal(data, &copied)
	s.policies[policy.Namespace][policy.Name] = &copied

	return nil
}

// Delete deletes a policy.
func (s *InMemoryPolicyStore) Delete(_ context.Context, namespace, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.policies[namespace] == nil {
		return fmt.Errorf("policy not found: %s/%s", namespace, name)
	}

	if _, exists := s.policies[namespace][name]; !exists {
		return fmt.Errorf("policy not found: %s/%s", namespace, name)
	}

	delete(s.policies[namespace], name)
	return nil
}

// PolicyGenerator generates policies based on application requirements.
type PolicyGenerator struct {
	manager *PolicyManager
}

// NewPolicyGenerator creates a new policy generator.
func NewPolicyGenerator(manager *PolicyManager) *PolicyGenerator {
	return &PolicyGenerator{manager: manager}
}

// ApplicationSpec describes an application's networking requirements.
type ApplicationSpec struct {
	Name                string            `json:"name"`
	Namespace           string            `json:"namespace"`
	Labels              map[string]string `json:"labels"`
	Tier                string            `json:"tier"` // web, app, database
	IngressPorts        []int32           `json:"ingressPorts,omitempty"`
	EgressTargets       []EgressTarget    `json:"egressTargets,omitempty"`
	AllowFromNamespaces []string          `json:"allowFromNamespaces,omitempty"`
	AllowFromLabels     map[string]string `json:"allowFromLabels,omitempty"`
	DenyAll             bool              `json:"denyAll,omitempty"`
}

// EgressTarget describes an egress target.
type EgressTarget struct {
	Namespace string            `json:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Ports     []int32           `json:"ports,omitempty"`
	CIDR      string            `json:"cidr,omitempty"`
}

// GenerateForApplication generates network policies for an application.
func (g *PolicyGenerator) GenerateForApplication(spec *ApplicationSpec) ([]*NetworkPolicy, error) {
	var policies []*NetworkPolicy

	// Generate ingress policy
	if len(spec.IngressPorts) > 0 || spec.DenyAll {
		ingressPolicy := g.generateIngressPolicy(spec)
		policies = append(policies, ingressPolicy)
	}

	// Generate egress policy if targets specified
	if len(spec.EgressTargets) > 0 || spec.DenyAll {
		egressPolicy := g.generateEgressPolicy(spec)
		policies = append(policies, egressPolicy)
	}

	return policies, nil
}

func (g *PolicyGenerator) generateIngressPolicy(spec *ApplicationSpec) *NetworkPolicy {
	policy := &NetworkPolicy{
		Name:      fmt.Sprintf("%s-ingress", spec.Name),
		Namespace: spec.Namespace,
		Labels: map[string]string{
			"keystone.io/managed":     "true",
			"keystone.io/application": spec.Name,
		},
		Spec: NetworkPolicySpec{
			PodSelector: LabelSelector{
				MatchLabels: spec.Labels,
			},
			PolicyTypes: []PolicyType{PolicyTypeIngress},
		},
	}

	if spec.DenyAll {
		return policy
	}

	var ingressRules []NetworkPolicyIngressRule

	// Convert ports
	var ports []NetworkPolicyPort
	for _, p := range spec.IngressPorts {
		ports = append(ports, NetworkPolicyPort{
			Protocol: ProtocolTCP,
			Port:     p,
		})
	}

	// Build from rules
	var from []NetworkPolicyPeer

	if len(spec.AllowFromNamespaces) > 0 {
		for _, ns := range spec.AllowFromNamespaces {
			from = append(from, NetworkPolicyPeer{
				NamespaceSelector: &LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": ns,
					},
				},
			})
		}
	}

	if len(spec.AllowFromLabels) > 0 {
		from = append(from, NetworkPolicyPeer{
			PodSelector: &LabelSelector{
				MatchLabels: spec.AllowFromLabels,
			},
		})
	}

	if len(from) == 0 && len(ports) > 0 {
		// Allow from anywhere on specified ports
		ingressRules = append(ingressRules, NetworkPolicyIngressRule{
			Ports: ports,
		})
	} else if len(from) > 0 {
		ingressRules = append(ingressRules, NetworkPolicyIngressRule{
			From:  from,
			Ports: ports,
		})
	}

	policy.Spec.Ingress = ingressRules
	return policy
}

func (g *PolicyGenerator) generateEgressPolicy(spec *ApplicationSpec) *NetworkPolicy {
	policy := &NetworkPolicy{
		Name:      fmt.Sprintf("%s-egress", spec.Name),
		Namespace: spec.Namespace,
		Labels: map[string]string{
			"keystone.io/managed":     "true",
			"keystone.io/application": spec.Name,
		},
		Spec: NetworkPolicySpec{
			PodSelector: LabelSelector{
				MatchLabels: spec.Labels,
			},
			PolicyTypes: []PolicyType{PolicyTypeEgress},
		},
	}

	if spec.DenyAll {
		return policy
	}

	var egressRules []NetworkPolicyEgressRule

	for _, target := range spec.EgressTargets {
		rule := NetworkPolicyEgressRule{}

		// Build ports
		for _, p := range target.Ports {
			rule.Ports = append(rule.Ports, NetworkPolicyPort{
				Protocol: ProtocolTCP,
				Port:     p,
			})
		}

		// Build to peers
		if target.CIDR != "" {
			rule.To = append(rule.To, NetworkPolicyPeer{
				IPBlock: &IPBlock{CIDR: target.CIDR},
			})
		} else if target.Namespace != "" || len(target.Labels) > 0 {
			peer := NetworkPolicyPeer{}
			if target.Namespace != "" {
				peer.NamespaceSelector = &LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": target.Namespace,
					},
				}
			}
			if len(target.Labels) > 0 {
				peer.PodSelector = &LabelSelector{
					MatchLabels: target.Labels,
				}
			}
			rule.To = append(rule.To, peer)
		}

		egressRules = append(egressRules, rule)
	}

	policy.Spec.Egress = egressRules
	return policy
}

// PolicyAuditor audits network policies for security issues.
type PolicyAuditor struct {
	rules []AuditRule
}

// AuditRule defines a policy audit rule.
type AuditRule struct {
	Name        string                           `json:"name"`
	Description string                           `json:"description"`
	Severity    string                           `json:"severity"` // critical, high, medium, low
	Check       func(policy *NetworkPolicy) bool `json:"-"`
}

// AuditFinding represents a policy audit finding.
type AuditFinding struct {
	PolicyName  string `json:"policyName"`
	Namespace   string `json:"namespace"`
	RuleName    string `json:"ruleName"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// AuditReport contains the results of a policy audit.
type AuditReport struct {
	Timestamp     time.Time      `json:"timestamp"`
	TotalPolicies int            `json:"totalPolicies"`
	Findings      []AuditFinding `json:"findings"`
	Summary       map[string]int `json:"summary"` // severity -> count
}

// NewPolicyAuditor creates a new policy auditor with default rules.
func NewPolicyAuditor() *PolicyAuditor {
	auditor := &PolicyAuditor{}
	auditor.registerDefaultRules()
	return auditor
}

func (a *PolicyAuditor) registerDefaultRules() {
	a.rules = []AuditRule{
		{
			Name:        "allow-all-ingress",
			Description: "Policy allows ingress from any source",
			Severity:    "high",
			Check: func(policy *NetworkPolicy) bool {
				for _, rule := range policy.Spec.Ingress {
					if len(rule.From) == 0 && len(rule.Ports) == 0 {
						return true
					}
				}
				return false
			},
		},
		{
			Name:        "allow-all-egress",
			Description: "Policy allows egress to any destination",
			Severity:    "medium",
			Check: func(policy *NetworkPolicy) bool {
				for _, rule := range policy.Spec.Egress {
					if len(rule.To) == 0 && len(rule.Ports) == 0 {
						return true
					}
				}
				return false
			},
		},
		{
			Name:        "wide-cidr",
			Description: "Policy uses overly broad CIDR range",
			Severity:    "medium",
			Check: func(policy *NetworkPolicy) bool {
				wideCIDRs := map[string]bool{
					"0.0.0.0/0":  true,
					"::/0":       true,
					"10.0.0.0/8": true,
				}
				for _, rule := range policy.Spec.Ingress {
					for _, from := range rule.From {
						if from.IPBlock != nil && wideCIDRs[from.IPBlock.CIDR] {
							return true
						}
					}
				}
				for _, rule := range policy.Spec.Egress {
					for _, to := range rule.To {
						if to.IPBlock != nil && wideCIDRs[to.IPBlock.CIDR] {
							return true
						}
					}
				}
				return false
			},
		},
		{
			Name:        "empty-pod-selector",
			Description: "Policy applies to all pods in namespace (empty selector)",
			Severity:    "low",
			Check: func(policy *NetworkPolicy) bool {
				return len(policy.Spec.PodSelector.MatchLabels) == 0 &&
					len(policy.Spec.PodSelector.MatchExpressions) == 0
			},
		},
	}
}

// AddRule adds a custom audit rule.
func (a *PolicyAuditor) AddRule(rule AuditRule) {
	a.rules = append(a.rules, rule)
}

// Audit audits a set of policies.
func (a *PolicyAuditor) Audit(policies []*NetworkPolicy) *AuditReport {
	report := &AuditReport{
		Timestamp:     time.Now(),
		TotalPolicies: len(policies),
		Summary:       make(map[string]int),
	}

	for _, policy := range policies {
		for _, rule := range a.rules {
			if rule.Check(policy) {
				finding := AuditFinding{
					PolicyName:  policy.Name,
					Namespace:   policy.Namespace,
					RuleName:    rule.Name,
					Description: rule.Description,
					Severity:    rule.Severity,
				}
				report.Findings = append(report.Findings, finding)
				report.Summary[rule.Severity]++
			}
		}
	}

	return report
}

// SortFindingsBySeverity sorts findings by severity (critical first).
func SortFindingsBySeverity(findings []AuditFinding) {
	severityOrder := map[string]int{
		"critical": 0,
		"high":     1,
		"medium":   2,
		"low":      3,
	}

	sort.Slice(findings, func(i, j int) bool {
		return severityOrder[findings[i].Severity] < severityOrder[findings[j].Severity]
	})
}
