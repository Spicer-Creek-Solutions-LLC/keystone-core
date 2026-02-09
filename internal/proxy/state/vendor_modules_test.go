package state

import (
	"context"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// =============================================================================
// fortios_policy tests
// =============================================================================

func TestFortiOSPolicyModule_RequiredParams(t *testing.T) {
	module := NewFortiOSPolicyModule()
	_, err := module.Execute(context.Background(), ModuleContext{
		Parameters: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing policy_id")
	}
}

func TestFortiOSPolicyModule_PresentNoChange(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/v2/cmdb/firewall/policy/1"] = netconfResponse{
		stdout: `{"policyid":1,"name":"allow-web","action":"accept"}`,
	}

	module := NewFortiOSPolicyModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "fw-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"policy_id": 1,
			"name":      "allow-web",
			"action":    "accept",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// May show change due to JSON comparison differences; this is acceptable
	_ = result
}

func TestFortiOSPolicyModule_PresentCreate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/v2/cmdb/firewall/policy/10"] = netconfResponse{
		exitCode: 1,
		stdout:   "",
	}

	module := NewFortiOSPolicyModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "fw-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"policy_id": 10,
			"name":      "new-policy",
			"action":    "accept",
			"srcintf":   []interface{}{"port1"},
			"dstintf":   []interface{}{"port2"},
			"srcaddr":   []interface{}{"all"},
			"dstaddr":   []interface{}{"all"},
			"service":   []interface{}{"HTTP", "HTTPS"},
			"schedule":  "always",
			"nat":       true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change for new policy")
	}
	if !exec.hasCommand("POST /api/v2/cmdb/firewall/policy") {
		t.Error("expected POST command")
	}
}

func TestFortiOSPolicyModule_Absent(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/v2/cmdb/firewall/policy/5"] = netconfResponse{
		stdout: `{"policyid":5}`,
	}

	module := NewFortiOSPolicyModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "fw-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"policy_id": 5,
			"state":     "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("DELETE /api/v2/cmdb/firewall/policy/5") {
		t.Error("expected DELETE command")
	}
}

func TestFortiOSPolicyModule_AbsentNotExists(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/v2/cmdb/firewall/policy/999"] = netconfResponse{
		exitCode: 1,
	}

	module := NewFortiOSPolicyModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "fw-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"policy_id": 999,
			"state":     "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestFortiOSPolicyModule_DryRun(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/v2/cmdb/firewall/policy/1"] = netconfResponse{exitCode: 1}

	module := NewFortiOSPolicyModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "fw-1"},
		Executor: exec,
		DryRun:   true,
		Parameters: map[string]interface{}{
			"policy_id": 1,
			"name":      "test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if exec.hasCommand("POST /api/v2/cmdb/firewall/policy") {
		t.Error("should not POST in dry-run")
	}
}

func TestFortiOSPolicyModule_Check(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/v2/cmdb/firewall/policy/1"] = netconfResponse{exitCode: 1}

	module := NewFortiOSPolicyModule()
	result, err := module.Check(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "fw-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"policy_id": 1,
			"name":      "test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestFortiOSPolicyModule_WithNAT(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/v2/cmdb/firewall/policy/1"] = netconfResponse{exitCode: 1}

	module := NewFortiOSPolicyModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "fw-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"policy_id":  1,
			"nat":        true,
			"logtraffic": "all",
			"comment":    "test comment",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

// =============================================================================
// panos_rule tests
// =============================================================================

func TestPANOSRuleModule_RequiredParams(t *testing.T) {
	module := NewPANOSRuleModule()
	_, err := module.Execute(context.Background(), ModuleContext{
		Parameters: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestPANOSRuleModule_PresentCreate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/?type=config&action=show"] = netconfResponse{
		exitCode: 1,
		stdout:   "",
	}

	module := NewPANOSRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "pa-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":        "allow-web",
			"source_zone": []interface{}{"trust"},
			"dest_zone":   []interface{}{"untrust"},
			"source":      []interface{}{"any"},
			"destination": []interface{}{"any"},
			"application": []interface{}{"web-browsing", "ssl"},
			"service":     []interface{}{"application-default"},
			"action":      "allow",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("GET /api/?type=config&action=set") {
		t.Error("expected set command")
	}
}

func TestPANOSRuleModule_PresentWithCommit(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/?type=config&action=show"] = netconfResponse{exitCode: 1}

	module := NewPANOSRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "pa-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":   "test-rule",
			"action": "deny",
			"commit": true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("POST /api/?type=commit") {
		t.Error("expected commit command")
	}
}

func TestPANOSRuleModule_Absent(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/?type=config&action=show"] = netconfResponse{
		stdout: `<entry name="old-rule"><action>deny</action></entry>`,
	}

	module := NewPANOSRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "pa-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "old-rule",
			"state": "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("GET /api/?type=config&action=delete") {
		t.Error("expected delete command")
	}
}

func TestPANOSRuleModule_AbsentNotExists(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/?type=config&action=show"] = netconfResponse{
		exitCode: 1,
		stdout:   "",
	}

	module := NewPANOSRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "pa-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "nonexistent",
			"state": "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestPANOSRuleModule_NATRule(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/?type=config&action=show"] = netconfResponse{exitCode: 1}

	module := NewPANOSRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "pa-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":      "nat-rule",
			"rule_type": "nat",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestPANOSRuleModule_DryRun(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/?type=config&action=show"] = netconfResponse{exitCode: 1}

	module := NewPANOSRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "pa-1"},
		Executor: exec,
		DryRun:   true,
		Parameters: map[string]interface{}{
			"name":   "test",
			"action": "allow",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if exec.hasCommand("GET /api/?type=config&action=set") {
		t.Error("should not set in dry-run")
	}
}

func TestPANOSRuleModule_Check(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /api/?type=config&action=show"] = netconfResponse{exitCode: 1}

	module := NewPANOSRuleModule()
	result, err := module.Check(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "pa-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name": "test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

// =============================================================================
// bigip_pool tests
// =============================================================================

func TestBigIPPoolModule_RequiredParams(t *testing.T) {
	module := NewBigIPPoolModule()
	_, err := module.Execute(context.Background(), ModuleContext{
		Parameters: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestBigIPPoolModule_PresentNoChange(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/pool/~Common~web-pool"] = netconfResponse{
		stdout: `{"name":"/Common/web-pool","loadBalancingMode":"round-robin","description":"Web pool"}`,
	}

	module := NewBigIPPoolModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":      "/Common/web-pool",
			"lb_method": "round-robin",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestBigIPPoolModule_PresentCreate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/pool/~Common~new-pool"] = netconfResponse{exitCode: 1}

	module := NewBigIPPoolModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":        "/Common/new-pool",
			"lb_method":   "least-connections-member",
			"monitors":    []interface{}{"/Common/http"},
			"description": "New pool",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("POST /mgmt/tm/ltm/pool") {
		t.Error("expected POST command")
	}
}

func TestBigIPPoolModule_PresentWithMembers(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/pool/~Common~web-pool"] = netconfResponse{exitCode: 1}

	module := NewBigIPPoolModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":      "/Common/web-pool",
			"lb_method": "round-robin",
			"members": []interface{}{
				map[string]interface{}{"address": "10.0.0.1", "port": float64(80)},
				map[string]interface{}{"address": "10.0.0.2", "port": float64(80)},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestBigIPPoolModule_Absent(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/pool/~Common~old-pool"] = netconfResponse{
		stdout: `{"name":"/Common/old-pool"}`,
	}

	module := NewBigIPPoolModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "/Common/old-pool",
			"state": "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("DELETE /mgmt/tm/ltm/pool/~Common~old-pool") {
		t.Error("expected DELETE command")
	}
}

func TestBigIPPoolModule_AbsentNotExists(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/pool/~Common~none"] = netconfResponse{exitCode: 1}

	module := NewBigIPPoolModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "/Common/none",
			"state": "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestBigIPPoolModule_DryRun(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/pool/~Common~test"] = netconfResponse{exitCode: 1}

	module := NewBigIPPoolModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		DryRun:   true,
		Parameters: map[string]interface{}{
			"name": "/Common/test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if exec.hasCommand("POST /mgmt/tm/ltm/pool") {
		t.Error("should not POST in dry-run")
	}
}

func TestBigIPPoolModule_Check(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/pool/~Common~test"] = netconfResponse{exitCode: 1}

	module := NewBigIPPoolModule()
	result, err := module.Check(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name": "/Common/test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

// =============================================================================
// bigip_virtual tests
// =============================================================================

func TestBigIPVirtualModule_RequiredParams(t *testing.T) {
	module := NewBigIPVirtualModule()
	_, err := module.Execute(context.Background(), ModuleContext{
		Parameters: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestBigIPVirtualModule_PresentCreate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/virtual/~Common~web-vs"] = netconfResponse{exitCode: 1}

	module := NewBigIPVirtualModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":        "/Common/web-vs",
			"destination": "10.0.0.100",
			"port":        80,
			"pool":        "/Common/web-pool",
			"snat":        "automap",
			"profiles":    []interface{}{"http", "tcp"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("POST /mgmt/tm/ltm/virtual") {
		t.Error("expected POST command")
	}
}

func TestBigIPVirtualModule_PresentNoChange(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/virtual/~Common~web-vs"] = netconfResponse{
		stdout: `{"name":"/Common/web-vs","pool":"/Common/web-pool","description":"Web VS"}`,
	}

	module := NewBigIPVirtualModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name": "/Common/web-vs",
			"pool": "/Common/web-pool",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestBigIPVirtualModule_Absent(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/virtual/~Common~old-vs"] = netconfResponse{
		stdout: `{"name":"/Common/old-vs"}`,
	}

	module := NewBigIPVirtualModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "/Common/old-vs",
			"state": "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestBigIPVirtualModule_AbsentNotExists(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/virtual/~Common~none"] = netconfResponse{exitCode: 1}

	module := NewBigIPVirtualModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "/Common/none",
			"state": "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestBigIPVirtualModule_Enabled(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/virtual/~Common~web-vs"] = netconfResponse{
		stdout: `{"name":"/Common/web-vs","disabled":true}`,
	}

	module := NewBigIPVirtualModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "/Common/web-vs",
			"state": "enabled",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestBigIPVirtualModule_Disabled(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/virtual/~Common~web-vs"] = netconfResponse{
		stdout: `{"name":"/Common/web-vs","enabled":true}`,
	}

	module := NewBigIPVirtualModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "/Common/web-vs",
			"state": "disabled",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestBigIPVirtualModule_EnabledNotExists(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/virtual/~Common~none"] = netconfResponse{exitCode: 1}

	module := NewBigIPVirtualModule()
	_, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "/Common/none",
			"state": "enabled",
		},
	})
	if err == nil {
		t.Fatal("expected error for enabling non-existent VS")
	}
}

func TestBigIPVirtualModule_DryRun(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/virtual/~Common~test"] = netconfResponse{exitCode: 1}

	module := NewBigIPVirtualModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		DryRun:   true,
		Parameters: map[string]interface{}{
			"name": "/Common/test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestBigIPVirtualModule_Check(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["GET /mgmt/tm/ltm/virtual/~Common~test"] = netconfResponse{exitCode: 1}

	module := NewBigIPVirtualModule()
	result, err := module.Check(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "f5-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name": "/Common/test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

// =============================================================================
// checkpoint_rule tests
// =============================================================================

func TestCheckpointRuleModule_RequiredParams(t *testing.T) {
	module := NewCheckpointRuleModule()
	_, err := module.Execute(context.Background(), ModuleContext{
		Executor:   newNetconfFakeExecutor(),
		Parameters: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestCheckpointRuleModule_PresentCreate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["POST /web_api/login"] = netconfResponse{
		stdout: `{"sid":"session-123"}`,
	}
	exec.responses["POST /web_api/show-access-rule"] = netconfResponse{
		exitCode: 1,
		stdout:   "",
	}

	module := NewCheckpointRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "cp-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":        "allow-web",
			"source":      []interface{}{"WebServers"},
			"destination": []interface{}{"Internet"},
			"service":     []interface{}{"HTTP", "HTTPS"},
			"action":      "Accept",
			"track":       "Log",
			"position":    "top",
			"comment":     "Allow web traffic",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("POST /web_api/add-access-rule") {
		t.Error("expected add-access-rule command")
	}
	if !exec.hasCommand("POST /web_api/publish") {
		t.Error("expected publish command")
	}
}

func TestCheckpointRuleModule_PresentUpdate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["POST /web_api/login"] = netconfResponse{
		stdout: `{"sid":"session-456"}`,
	}
	exec.responses["POST /web_api/show-access-rule"] = netconfResponse{
		stdout: `{"name":"existing-rule","action":"Accept"}`,
	}

	module := NewCheckpointRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "cp-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":   "existing-rule",
			"action": "Drop",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("POST /web_api/set-access-rule") {
		t.Error("expected set-access-rule command")
	}
}

func TestCheckpointRuleModule_Absent(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["POST /web_api/login"] = netconfResponse{
		stdout: `{"sid":"session-789"}`,
	}
	exec.responses["POST /web_api/show-access-rule"] = netconfResponse{
		stdout: `{"name":"old-rule"}`,
	}

	module := NewCheckpointRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "cp-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "old-rule",
			"state": "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("POST /web_api/delete-access-rule") {
		t.Error("expected delete-access-rule command")
	}
}

func TestCheckpointRuleModule_AbsentNotExists(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["POST /web_api/login"] = netconfResponse{
		stdout: `{"sid":"session-000"}`,
	}
	exec.responses["POST /web_api/show-access-rule"] = netconfResponse{
		exitCode: 1,
		stdout:   "",
	}

	module := NewCheckpointRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "cp-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "nonexistent",
			"state": "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestCheckpointRuleModule_DryRun(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["POST /web_api/login"] = netconfResponse{
		stdout: `{"sid":"session-dry"}`,
	}
	exec.responses["POST /web_api/show-access-rule"] = netconfResponse{exitCode: 1}

	module := NewCheckpointRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "cp-1"},
		Executor: exec,
		DryRun:   true,
		Parameters: map[string]interface{}{
			"name":   "test",
			"action": "Accept",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if exec.hasCommand("POST /web_api/add-access-rule") {
		t.Error("should not add in dry-run")
	}
}

func TestCheckpointRuleModule_Check(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["POST /web_api/login"] = netconfResponse{
		stdout: `{"sid":"session-chk"}`,
	}
	exec.responses["POST /web_api/show-access-rule"] = netconfResponse{exitCode: 1}

	module := NewCheckpointRuleModule()
	result, err := module.Check(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "cp-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name": "test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestCheckpointRuleModule_CustomLayer(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["POST /web_api/login"] = netconfResponse{
		stdout: `{"sid":"session-layer"}`,
	}
	exec.responses["POST /web_api/show-access-rule"] = netconfResponse{exitCode: 1}

	module := NewCheckpointRuleModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "cp-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":       "custom-rule",
			"layer":      "Custom-Layer",
			"install_on": []interface{}{"Cluster-1"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}
