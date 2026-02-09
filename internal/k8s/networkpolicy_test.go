package k8s

import (
	"context"
	"testing"
	"time"
)

func TestNetworkPolicy_Hash(t *testing.T) {
	policy1 := &NetworkPolicy{
		Name:      "test",
		Namespace: "default",
		Spec: NetworkPolicySpec{
			PodSelector: LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
		},
	}

	policy2 := &NetworkPolicy{
		Name:      "test",
		Namespace: "default",
		Spec: NetworkPolicySpec{
			PodSelector: LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
		},
	}

	policy3 := &NetworkPolicy{
		Name:      "test",
		Namespace: "default",
		Spec: NetworkPolicySpec{
			PodSelector: LabelSelector{
				MatchLabels: map[string]string{"app": "api"},
			},
		},
	}

	if policy1.Hash() != policy2.Hash() {
		t.Error("Identical policies should have same hash")
	}

	if policy1.Hash() == policy3.Hash() {
		t.Error("Different policies should have different hash")
	}
}

func TestNewPolicyManager(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	// Check built-in templates are registered
	templates := manager.ListTemplates()
	if len(templates) < 5 {
		t.Errorf("Expected at least 5 built-in templates, got %d", len(templates))
	}

	// Check specific template exists
	template, ok := manager.GetTemplate("deny-all")
	if !ok {
		t.Error("Expected deny-all template to exist")
	}
	if template.Category != "deny-all" {
		t.Errorf("Category = %s, want deny-all", template.Category)
	}
}

func TestPolicyManager_CreateFromTemplate(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)

	ctx := context.Background()

	policy, err := manager.CreateFromTemplate(ctx, "deny-all-ingress", "production", "deny-ingress", nil)
	if err != nil {
		t.Fatalf("CreateFromTemplate failed: %v", err)
	}

	if policy.Name != "deny-ingress" {
		t.Errorf("Name = %s, want deny-ingress", policy.Name)
	}
	if policy.Namespace != "production" {
		t.Errorf("Namespace = %s, want production", policy.Namespace)
	}
	if policy.Labels["keystone.io/template"] != "deny-all-ingress" {
		t.Error("Missing template label")
	}

	// Verify it was saved
	retrieved, err := manager.Get(ctx, "production", "deny-ingress")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Name != policy.Name {
		t.Error("Retrieved policy doesn't match")
	}
}

func TestPolicyManager_CreateFromTemplate_WithOverride(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)

	ctx := context.Background()
	override := &LabelSelector{
		MatchLabels: map[string]string{"app": "my-app"},
	}

	policy, err := manager.CreateFromTemplate(ctx, "deny-all-ingress", "default", "my-policy", override)
	if err != nil {
		t.Fatalf("CreateFromTemplate failed: %v", err)
	}

	if policy.Spec.PodSelector.MatchLabels["app"] != "my-app" {
		t.Error("Override was not applied")
	}
}

func TestPolicyManager_CreateFromTemplate_NotFound(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)

	ctx := context.Background()

	_, err := manager.CreateFromTemplate(ctx, "nonexistent", "default", "test", nil)
	if err == nil {
		t.Error("Expected error for nonexistent template")
	}
}

func TestPolicyManager_CRUD(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)

	ctx := context.Background()

	policy := &NetworkPolicy{
		Name:      "test-policy",
		Namespace: "default",
		Spec: NetworkPolicySpec{
			PodSelector: LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
			PolicyTypes: []PolicyType{PolicyTypeIngress},
			Ingress: []NetworkPolicyIngressRule{
				{
					Ports: []NetworkPolicyPort{
						{Protocol: ProtocolTCP, Port: 80},
					},
				},
			},
		},
	}

	// Create
	if err := manager.Create(ctx, policy); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get
	retrieved, err := manager.Get(ctx, "default", "test-policy")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Name != "test-policy" {
		t.Error("Retrieved policy name mismatch")
	}

	// List
	policies, err := manager.List(ctx, "default")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(policies) != 1 {
		t.Errorf("List count = %d, want 1", len(policies))
	}

	// Update
	policy.Spec.Ingress[0].Ports[0].Port = 8080
	if err := manager.Update(ctx, policy); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	retrieved, _ = manager.Get(ctx, "default", "test-policy")
	if retrieved.Spec.Ingress[0].Ports[0].Port != 8080 {
		t.Error("Update was not applied")
	}

	// Delete
	if err := manager.Delete(ctx, "default", "test-policy"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = manager.Get(ctx, "default", "test-policy")
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestPolicyManager_Validate(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)

	tests := []struct {
		name         string
		policy       *NetworkPolicy
		wantValid    bool
		wantErrors   int
		wantWarnings int
	}{
		{
			name: "valid policy",
			policy: &NetworkPolicy{
				Name:      "test",
				Namespace: "default",
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeIngress},
					Ingress: []NetworkPolicyIngressRule{
						{Ports: []NetworkPolicyPort{{Port: 80}}},
					},
				},
			},
			wantValid: true,
		},
		{
			name: "missing name",
			policy: &NetworkPolicy{
				Namespace: "default",
			},
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name: "missing namespace",
			policy: &NetworkPolicy{
				Name: "test",
			},
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name: "invalid port",
			policy: &NetworkPolicy{
				Name:      "test",
				Namespace: "default",
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeIngress},
					Ingress: []NetworkPolicyIngressRule{
						{Ports: []NetworkPolicyPort{{Port: 70000}}},
					},
				},
			},
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name: "invalid port range",
			policy: &NetworkPolicy{
				Name:      "test",
				Namespace: "default",
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeIngress},
					Ingress: []NetworkPolicyIngressRule{
						{Ports: []NetworkPolicyPort{{Port: 100, EndPort: 50}}},
					},
				},
			},
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name: "no policy types warning",
			policy: &NetworkPolicy{
				Name:      "test",
				Namespace: "default",
			},
			wantValid:    true,
			wantWarnings: 1,
		},
		{
			name: "ingress type with no rules warning",
			policy: &NetworkPolicy{
				Name:      "test",
				Namespace: "default",
				Spec: NetworkPolicySpec{
					PolicyTypes: []PolicyType{PolicyTypeIngress},
				},
			},
			wantValid:    true,
			wantWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.Validate(tt.policy)

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if tt.wantErrors > 0 && len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d: %v", len(result.Errors), tt.wantErrors, result.Errors)
			}
			if tt.wantWarnings > 0 && len(result.Warnings) != tt.wantWarnings {
				t.Errorf("Warnings = %d, want %d: %v", len(result.Warnings), tt.wantWarnings, result.Warnings)
			}
		})
	}
}

func TestPolicyManager_Diff(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)

	ctx := context.Background()

	// Create some existing policies
	existing := []*NetworkPolicy{
		{Name: "policy-1", Namespace: "default", Spec: NetworkPolicySpec{PodSelector: LabelSelector{MatchLabels: map[string]string{"a": "1"}}}},
		{Name: "policy-2", Namespace: "default", Spec: NetworkPolicySpec{PodSelector: LabelSelector{MatchLabels: map[string]string{"b": "2"}}}},
		{Name: "policy-3", Namespace: "default", Spec: NetworkPolicySpec{PodSelector: LabelSelector{MatchLabels: map[string]string{"c": "3"}}}},
	}

	for _, p := range existing {
		manager.Create(ctx, p)
	}

	// Desired state
	desired := []*NetworkPolicy{
		{Name: "policy-1", Namespace: "default", Spec: NetworkPolicySpec{PodSelector: LabelSelector{MatchLabels: map[string]string{"a": "1"}}}},       // unchanged
		{Name: "policy-2", Namespace: "default", Spec: NetworkPolicySpec{PodSelector: LabelSelector{MatchLabels: map[string]string{"b": "changed"}}}}, // changed
		{Name: "policy-4", Namespace: "default", Spec: NetworkPolicySpec{PodSelector: LabelSelector{MatchLabels: map[string]string{"d": "4"}}}},       // new
		// policy-3 is removed
	}

	diff, err := manager.Diff(ctx, "default", desired)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	if len(diff.Added) != 1 {
		t.Errorf("Added = %d, want 1", len(diff.Added))
	}
	if len(diff.Removed) != 1 {
		t.Errorf("Removed = %d, want 1", len(diff.Removed))
	}
	if len(diff.Changed) != 1 {
		t.Errorf("Changed = %d, want 1", len(diff.Changed))
	}
}

func TestPolicyManager_ApplyDiff(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)

	ctx := context.Background()

	// Create initial policy
	manager.Create(ctx, &NetworkPolicy{
		Name:      "to-update",
		Namespace: "default",
		Spec:      NetworkPolicySpec{PodSelector: LabelSelector{MatchLabels: map[string]string{"old": "value"}}},
	})
	manager.Create(ctx, &NetworkPolicy{
		Name:      "to-delete",
		Namespace: "default",
	})

	diff := &PolicyDiff{
		Added: []*NetworkPolicy{
			{Name: "new-policy", Namespace: "default"},
		},
		Changed: []PolicyChange{
			{
				Name:      "to-update",
				Namespace: "default",
				New:       &NetworkPolicy{Name: "to-update", Namespace: "default", Spec: NetworkPolicySpec{PodSelector: LabelSelector{MatchLabels: map[string]string{"new": "value"}}}},
			},
		},
		Removed: []*NetworkPolicy{
			{Name: "to-delete", Namespace: "default"},
		},
	}

	if err := manager.ApplyDiff(ctx, diff); err != nil {
		t.Fatalf("ApplyDiff failed: %v", err)
	}

	// Verify new policy was created
	_, err := manager.Get(ctx, "default", "new-policy")
	if err != nil {
		t.Error("Expected new-policy to be created")
	}

	// Verify policy was updated
	updated, _ := manager.Get(ctx, "default", "to-update")
	if updated.Spec.PodSelector.MatchLabels["new"] != "value" {
		t.Error("Expected to-update to be updated")
	}

	// Verify policy was deleted
	_, err = manager.Get(ctx, "default", "to-delete")
	if err == nil {
		t.Error("Expected to-delete to be deleted")
	}
}

func TestPolicyManager_Listener(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)

	var events []*PolicyEvent
	manager.AddListener(func(e *PolicyEvent) {
		events = append(events, e)
	})

	ctx := context.Background()

	// Create
	manager.Create(ctx, &NetworkPolicy{Name: "test", Namespace: "default"})
	if len(events) != 1 || events[0].Type != "created" {
		t.Error("Expected created event")
	}

	// Update
	manager.Update(ctx, &NetworkPolicy{Name: "test", Namespace: "default"})
	if len(events) != 2 || events[1].Type != "updated" {
		t.Error("Expected updated event")
	}

	// Delete
	manager.Delete(ctx, "default", "test")
	if len(events) != 3 || events[2].Type != "deleted" {
		t.Error("Expected deleted event")
	}
}

func TestInMemoryPolicyStore(t *testing.T) {
	store := NewInMemoryPolicyStore()
	ctx := context.Background()

	// Test Create
	policy := &NetworkPolicy{Name: "test", Namespace: "default"}
	if err := store.Create(ctx, policy); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Test duplicate create
	if err := store.Create(ctx, policy); err == nil {
		t.Error("Expected error on duplicate create")
	}

	// Test Get
	retrieved, err := store.Get(ctx, "default", "test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Name != "test" {
		t.Error("Get returned wrong policy")
	}

	// Test Get not found
	_, err = store.Get(ctx, "default", "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent policy")
	}

	// Test List all
	store.Create(ctx, &NetworkPolicy{Name: "test2", Namespace: "other"})
	all, _ := store.List(ctx, "")
	if len(all) != 2 {
		t.Errorf("List all = %d, want 2", len(all))
	}

	// Test List by namespace
	defaults, _ := store.List(ctx, "default")
	if len(defaults) != 1 {
		t.Errorf("List default = %d, want 1", len(defaults))
	}

	// Test Update
	policy.Spec = NetworkPolicySpec{PolicyTypes: []PolicyType{PolicyTypeIngress}}
	if err := store.Update(ctx, policy); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Test Update not found
	if err := store.Update(ctx, &NetworkPolicy{Name: "nonexistent", Namespace: "default"}); err == nil {
		t.Error("Expected error for update of nonexistent policy")
	}

	// Test Delete
	if err := store.Delete(ctx, "default", "test"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Test Delete not found
	if err := store.Delete(ctx, "default", "test"); err == nil {
		t.Error("Expected error for delete of nonexistent policy")
	}
}

func TestPolicyGenerator(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)
	generator := NewPolicyGenerator(manager)

	spec := &ApplicationSpec{
		Name:      "web-app",
		Namespace: "production",
		Labels: map[string]string{
			"app": "web-app",
		},
		Tier:         "web",
		IngressPorts: []int32{80, 443},
		EgressTargets: []EgressTarget{
			{
				Namespace: "production",
				Labels:    map[string]string{"tier": "app"},
				Ports:     []int32{8080},
			},
		},
		AllowFromNamespaces: []string{"ingress"},
	}

	policies, err := generator.GenerateForApplication(spec)
	if err != nil {
		t.Fatalf("GenerateForApplication failed: %v", err)
	}

	if len(policies) != 2 {
		t.Errorf("Generated %d policies, want 2", len(policies))
	}

	// Check ingress policy
	var ingress, egress *NetworkPolicy
	for _, p := range policies {
		if p.Name == "web-app-ingress" {
			ingress = p
		}
		if p.Name == "web-app-egress" {
			egress = p
		}
	}

	if ingress == nil {
		t.Fatal("Expected ingress policy")
	}
	if len(ingress.Spec.Ingress) != 1 {
		t.Errorf("Ingress rules = %d, want 1", len(ingress.Spec.Ingress))
	}
	if len(ingress.Spec.Ingress[0].Ports) != 2 {
		t.Errorf("Ingress ports = %d, want 2", len(ingress.Spec.Ingress[0].Ports))
	}

	if egress == nil {
		t.Fatal("Expected egress policy")
	}
	if len(egress.Spec.Egress) != 1 {
		t.Errorf("Egress rules = %d, want 1", len(egress.Spec.Egress))
	}
}

func TestPolicyGenerator_DenyAll(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)
	generator := NewPolicyGenerator(manager)

	spec := &ApplicationSpec{
		Name:      "isolated-app",
		Namespace: "secure",
		Labels:    map[string]string{"app": "isolated"},
		DenyAll:   true,
	}

	policies, err := generator.GenerateForApplication(spec)
	if err != nil {
		t.Fatalf("GenerateForApplication failed: %v", err)
	}

	if len(policies) != 2 {
		t.Errorf("Generated %d policies, want 2", len(policies))
	}

	// Both should have empty rules (deny all)
	for _, p := range policies {
		if len(p.Spec.Ingress) != 0 || len(p.Spec.Egress) != 0 {
			t.Error("Deny-all policy should have no rules")
		}
	}
}

func TestPolicyAuditor(t *testing.T) {
	auditor := NewPolicyAuditor()

	policies := []*NetworkPolicy{
		{
			Name:      "allow-all",
			Namespace: "default",
			Spec: NetworkPolicySpec{
				Ingress: []NetworkPolicyIngressRule{
					{}, // Empty - allows all
				},
			},
		},
		{
			Name:      "wide-cidr",
			Namespace: "default",
			Spec: NetworkPolicySpec{
				Ingress: []NetworkPolicyIngressRule{
					{
						From: []NetworkPolicyPeer{
							{IPBlock: &IPBlock{CIDR: "0.0.0.0/0"}},
						},
					},
				},
			},
		},
		{
			Name:      "good-policy",
			Namespace: "default",
			Spec: NetworkPolicySpec{
				PodSelector: LabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
				Ingress: []NetworkPolicyIngressRule{
					{
						From: []NetworkPolicyPeer{
							{PodSelector: &LabelSelector{MatchLabels: map[string]string{"app": "frontend"}}},
						},
						Ports: []NetworkPolicyPort{
							{Protocol: ProtocolTCP, Port: 80},
						},
					},
				},
			},
		},
	}

	report := auditor.Audit(policies)

	if report.TotalPolicies != 3 {
		t.Errorf("TotalPolicies = %d, want 3", report.TotalPolicies)
	}

	if len(report.Findings) < 2 {
		t.Errorf("Findings = %d, want at least 2", len(report.Findings))
	}

	// Check that findings are categorized by severity
	if report.Summary["high"] < 1 {
		t.Error("Expected at least 1 high severity finding")
	}
}

func TestPolicyAuditor_CustomRule(t *testing.T) {
	auditor := NewPolicyAuditor()

	// Add custom rule
	auditor.AddRule(AuditRule{
		Name:        "no-production-allow-all",
		Description: "Production namespaces should not allow all traffic",
		Severity:    "critical",
		Check: func(policy *NetworkPolicy) bool {
			if policy.Namespace != "production" {
				return false
			}
			for _, rule := range policy.Spec.Ingress {
				if len(rule.From) == 0 {
					return true
				}
			}
			return false
		},
	})

	policies := []*NetworkPolicy{
		{
			Name:      "prod-allow-all",
			Namespace: "production",
			Spec: NetworkPolicySpec{
				Ingress: []NetworkPolicyIngressRule{{}},
			},
		},
		{
			Name:      "dev-allow-all",
			Namespace: "development",
			Spec: NetworkPolicySpec{
				Ingress: []NetworkPolicyIngressRule{{}},
			},
		},
	}

	report := auditor.Audit(policies)

	criticalFindings := 0
	for _, f := range report.Findings {
		if f.Severity == "critical" && f.RuleName == "no-production-allow-all" {
			criticalFindings++
		}
	}

	if criticalFindings != 1 {
		t.Errorf("Critical findings = %d, want 1", criticalFindings)
	}
}

func TestSortFindingsBySeverity(t *testing.T) {
	findings := []AuditFinding{
		{RuleName: "low", Severity: "low"},
		{RuleName: "high", Severity: "high"},
		{RuleName: "critical", Severity: "critical"},
		{RuleName: "medium", Severity: "medium"},
	}

	SortFindingsBySeverity(findings)

	expected := []string{"critical", "high", "medium", "low"}
	for i, f := range findings {
		if f.Severity != expected[i] {
			t.Errorf("Position %d: got %s, want %s", i, f.Severity, expected[i])
		}
	}
}

func TestPolicyTemplates(t *testing.T) {
	store := NewInMemoryPolicyStore()
	manager := NewPolicyManager(store)

	templates := []string{
		"deny-all-ingress",
		"deny-all-egress",
		"deny-all",
		"allow-same-namespace",
		"allow-dns",
		"allow-monitoring",
		"web-tier",
		"database-tier",
	}

	for _, name := range templates {
		t.Run(name, func(t *testing.T) {
			template, ok := manager.GetTemplate(name)
			if !ok {
				t.Errorf("Template %s not found", name)
				return
			}
			if template.Description == "" {
				t.Errorf("Template %s has no description", name)
			}
			if template.Category == "" {
				t.Errorf("Template %s has no category", name)
			}
		})
	}
}

func TestNetworkPolicyPeer(t *testing.T) {
	peer := NetworkPolicyPeer{
		PodSelector: &LabelSelector{
			MatchLabels: map[string]string{"app": "web"},
		},
		NamespaceSelector: &LabelSelector{
			MatchLabels: map[string]string{"env": "production"},
		},
	}

	if peer.PodSelector == nil {
		t.Error("PodSelector should not be nil")
	}
	if peer.NamespaceSelector == nil {
		t.Error("NamespaceSelector should not be nil")
	}
}

func TestIPBlock(t *testing.T) {
	block := IPBlock{
		CIDR:   "10.0.0.0/8",
		Except: []string{"10.0.0.1/32", "10.0.0.2/32"},
	}

	if block.CIDR != "10.0.0.0/8" {
		t.Errorf("CIDR = %s, want 10.0.0.0/8", block.CIDR)
	}
	if len(block.Except) != 2 {
		t.Errorf("Except = %d, want 2", len(block.Except))
	}
}

func TestLabelSelectorRequirement(t *testing.T) {
	selector := LabelSelector{
		MatchExpressions: []LabelSelectorRequirement{
			{
				Key:      "tier",
				Operator: "In",
				Values:   []string{"web", "app"},
			},
			{
				Key:      "deprecated",
				Operator: "DoesNotExist",
			},
		},
	}

	if len(selector.MatchExpressions) != 2 {
		t.Errorf("MatchExpressions = %d, want 2", len(selector.MatchExpressions))
	}
	if selector.MatchExpressions[0].Operator != "In" {
		t.Errorf("Operator = %s, want In", selector.MatchExpressions[0].Operator)
	}
}

func TestPolicyEvent(t *testing.T) {
	event := &PolicyEvent{
		Type:      "created",
		Policy:    &NetworkPolicy{Name: "test"},
		Namespace: "default",
		Timestamp: time.Now(),
	}

	if event.Type != "created" {
		t.Errorf("Type = %s, want created", event.Type)
	}
	if event.Policy.Name != "test" {
		t.Errorf("Policy name = %s, want test", event.Policy.Name)
	}
}

func TestNetworkPolicyPort_Range(t *testing.T) {
	port := NetworkPolicyPort{
		Protocol: ProtocolTCP,
		Port:     1000,
		EndPort:  2000,
	}

	if port.Port != 1000 {
		t.Errorf("Port = %d, want 1000", port.Port)
	}
	if port.EndPort != 2000 {
		t.Errorf("EndPort = %d, want 2000", port.EndPort)
	}
}

func TestProtocols(t *testing.T) {
	protocols := []Protocol{ProtocolTCP, ProtocolUDP, ProtocolSCTP}
	expected := []string{"TCP", "UDP", "SCTP"}

	for i, p := range protocols {
		if string(p) != expected[i] {
			t.Errorf("Protocol = %s, want %s", p, expected[i])
		}
	}
}

func TestPolicyTypes(t *testing.T) {
	types := []PolicyType{PolicyTypeIngress, PolicyTypeEgress}
	expected := []string{"Ingress", "Egress"}

	for i, pt := range types {
		if string(pt) != expected[i] {
			t.Errorf("PolicyType = %s, want %s", pt, expected[i])
		}
	}
}
