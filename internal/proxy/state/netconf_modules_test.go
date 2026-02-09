package state

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// netconfFakeExecutor is a fake executor that matches commands by prefix for
// NETCONF operations which contain variable XML content.
type netconfFakeExecutor struct {
	responses map[string]netconfResponse
	commands  []string
}

type netconfResponse struct {
	stdout   string
	exitCode int
	err      error
}

func newNetconfFakeExecutor() *netconfFakeExecutor {
	return &netconfFakeExecutor{
		responses: make(map[string]netconfResponse),
	}
}

func (f *netconfFakeExecutor) Execute(ctx context.Context, req *proxy.ProxiedExecuteRequest) (*proxy.ProxiedExecuteResult, error) {
	f.commands = append(f.commands, req.Command)

	// Try exact match first
	if resp, ok := f.responses[req.Command]; ok {
		if resp.err != nil {
			return nil, resp.err
		}
		return &proxy.ProxiedExecuteResult{
			DeviceID:  req.DeviceID,
			ExitCode:  resp.exitCode,
			Stdout:    []byte(resp.stdout),
			StartTime: time.Now(),
			EndTime:   time.Now(),
		}, nil
	}

	// Try prefix match (for commands with XML payloads)
	for prefix, resp := range f.responses {
		if strings.HasPrefix(req.Command, prefix) {
			if resp.err != nil {
				return nil, resp.err
			}
			return &proxy.ProxiedExecuteResult{
				DeviceID:  req.DeviceID,
				ExitCode:  resp.exitCode,
				Stdout:    []byte(resp.stdout),
				StartTime: time.Now(),
				EndTime:   time.Now(),
			}, nil
		}
	}

	// Default: succeed silently
	return &proxy.ProxiedExecuteResult{
		DeviceID:  req.DeviceID,
		Stdout:    []byte(""),
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}, nil
}

func (f *netconfFakeExecutor) ExecuteWithOutput(ctx context.Context, req *proxy.ProxiedExecuteRequest, outputHandler proxy.OutputHandler) (*proxy.ProxiedExecuteResult, error) {
	return f.Execute(ctx, req)
}

func (f *netconfFakeExecutor) Check(ctx context.Context, deviceID string) (*proxy.DeviceHealthResult, error) {
	return &proxy.DeviceHealthResult{DeviceID: deviceID, Status: proxy.DeviceStatusOnline}, nil
}

func (f *netconfFakeExecutor) GetCapabilities(ctx context.Context, deviceID string) (*proxy.DeviceCapabilities, error) {
	return &proxy.DeviceCapabilities{DeviceID: deviceID, CanExecuteCommands: true}, nil
}

func (f *netconfFakeExecutor) Close(ctx context.Context, deviceID string) error {
	return nil
}

func (f *netconfFakeExecutor) hasCommand(prefix string) bool {
	for _, cmd := range f.commands {
		if strings.HasPrefix(cmd, prefix) {
			return true
		}
	}
	return false
}

// =============================================================================
// netconf_interface tests
// =============================================================================

func TestNetconfInterfaceModule_RequiredParams(t *testing.T) {
	module := NewNetconfInterfaceModule()
	_, err := module.Execute(context.Background(), ModuleContext{
		Parameters: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestNetconfInterfaceModule_PresentNoChange(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<interfaces xmlns="http://openconfig.net/yang/interfaces"><interface><name>eth0</name><config><name>eth0</name><description>uplink</description><enabled>true</enabled><mtu>1500</mtu></config></interface></interfaces>`,
	}

	module := NewNetconfInterfaceModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":        "eth0",
			"description": "uplink",
			"enabled":     true,
			"mtu":         1500,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestNetconfInterfaceModule_PresentCreate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfInterfaceModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":        "eth0",
			"description": "uplink",
			"mtu":         9000,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("edit-config candidate") {
		t.Error("expected edit-config command")
	}
	if !exec.hasCommand("commit") {
		t.Error("expected commit command")
	}
}

func TestNetconfInterfaceModule_PresentUpdate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<interface><name>eth0</name><config><name>eth0</name><description>old</description><mtu>1500</mtu></config></interface>`,
	}

	module := NewNetconfInterfaceModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":        "eth0",
			"description": "new-desc",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestNetconfInterfaceModule_Absent(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<interface><name>eth0</name></interface>`,
	}

	module := NewNetconfInterfaceModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "eth0",
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

func TestNetconfInterfaceModule_AbsentNotExists(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfInterfaceModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "eth0",
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

func TestNetconfInterfaceModule_DryRun(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfInterfaceModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		DryRun:   true,
		Parameters: map[string]interface{}{
			"name":        "eth0",
			"description": "test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change in dry-run")
	}
	if exec.hasCommand("commit") {
		t.Error("should not commit in dry-run")
	}
}

func TestNetconfInterfaceModule_StateUp(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<interface><name>eth0</name><config><name>eth0</name><enabled>false</enabled></config></interface>`,
	}

	module := NewNetconfInterfaceModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "eth0",
			"state": "up",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change for up state")
	}
}

func TestNetconfInterfaceModule_StateDown(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<interface><name>eth0</name><config><name>eth0</name><enabled>true</enabled></config></interface>`,
	}

	module := NewNetconfInterfaceModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "eth0",
			"state": "down",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change for down state")
	}
}

func TestNetconfInterfaceModule_WithIP(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfInterfaceModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":             "eth0",
			"ip_address":       "10.0.0.1",
			"ip_prefix_length": 24,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestNetconfInterfaceModule_Check(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfInterfaceModule()
	result, err := module.Check(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":        "eth0",
			"description": "test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if exec.hasCommand("commit") {
		t.Error("check should not commit")
	}
}

func TestNetconfInterfaceModule_EditConfigError(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}
	exec.responses["edit-config candidate"] = netconfResponse{err: fmt.Errorf("edit failed")}

	module := NewNetconfInterfaceModule()
	_, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":        "eth0",
			"description": "test",
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !exec.hasCommand("discard-changes") {
		t.Error("expected discard-changes on error")
	}
}

// =============================================================================
// netconf_vlan tests
// =============================================================================

func TestNetconfVLANModule_RequiredParams(t *testing.T) {
	module := NewNetconfVLANModule()
	_, err := module.Execute(context.Background(), ModuleContext{
		Parameters: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing vlan_id")
	}
}

func TestNetconfVLANModule_PresentNoChange(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<vlans><vlan><vlan-id>100</vlan-id><config><vlan-id>100</vlan-id><name>prod</name><status>ACTIVE</status></config></vlan></vlans>`,
	}

	module := NewNetconfVLANModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"vlan_id":    100,
			"name":       "prod",
			"vlan_state": "active",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestNetconfVLANModule_PresentCreate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfVLANModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"vlan_id":    200,
			"name":       "test",
			"vlan_state": "active",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestNetconfVLANModule_PresentUpdate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<vlan><vlan-id>100</vlan-id><config><name>old-name</name><status>ACTIVE</status></config></vlan>`,
	}

	module := NewNetconfVLANModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"vlan_id": 100,
			"name":    "new-name",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestNetconfVLANModule_Absent(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<vlan><vlan-id>100</vlan-id></vlan>`,
	}

	module := NewNetconfVLANModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"vlan_id": 100,
			"state":   "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestNetconfVLANModule_AbsentNotExists(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfVLANModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"vlan_id": 999,
			"state":   "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestNetconfVLANModule_DryRun(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfVLANModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		DryRun:   true,
		Parameters: map[string]interface{}{
			"vlan_id": 100,
			"name":    "test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if exec.hasCommand("commit") {
		t.Error("should not commit in dry-run")
	}
}

func TestNetconfVLANModule_Suspend(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<vlan><vlan-id>100</vlan-id><config><status>ACTIVE</status></config></vlan>`,
	}

	module := NewNetconfVLANModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"vlan_id":    100,
			"vlan_state": "suspend",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change for suspend")
	}
}

func TestNetconfVLANModule_Check(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfVLANModule()
	result, err := module.Check(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"vlan_id": 100,
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
// netconf_routing tests
// =============================================================================

func TestNetconfRoutingModule_RequiredParams(t *testing.T) {
	module := NewNetconfRoutingModule()

	// Missing prefix
	_, err := module.Execute(context.Background(), ModuleContext{
		Parameters: map[string]interface{}{
			"next_hop": "10.0.0.1",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing prefix")
	}

	// Missing next_hop
	_, err = module.Execute(context.Background(), ModuleContext{
		Parameters: map[string]interface{}{
			"prefix": "10.0.0.0/8",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing next_hop")
	}
}

func TestNetconfRoutingModule_PresentNoChange(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<static><prefix>10.0.0.0/8</prefix><next-hops><next-hop><config><next-hop>192.168.1.1</next-hop></config></next-hop></next-hops></static>`,
	}

	module := NewNetconfRoutingModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"prefix":   "10.0.0.0/8",
			"next_hop": "192.168.1.1",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestNetconfRoutingModule_PresentCreate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfRoutingModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"prefix":   "172.16.0.0/12",
			"next_hop": "10.0.0.1",
			"metric":   100,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("edit-config candidate") {
		t.Error("expected edit-config command")
	}
}

func TestNetconfRoutingModule_PresentUpdateNextHop(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<static><prefix>10.0.0.0/8</prefix><next-hops><next-hop><config><next-hop>192.168.1.1</next-hop></config></next-hop></next-hops></static>`,
	}

	module := NewNetconfRoutingModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"prefix":   "10.0.0.0/8",
			"next_hop": "192.168.1.254",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change for new next hop")
	}
}

func TestNetconfRoutingModule_Absent(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<static><prefix>10.0.0.0/8</prefix></static>`,
	}

	module := NewNetconfRoutingModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"prefix":   "10.0.0.0/8",
			"next_hop": "10.0.0.1",
			"state":    "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestNetconfRoutingModule_AbsentNotExists(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfRoutingModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"prefix":   "10.0.0.0/8",
			"next_hop": "10.0.0.1",
			"state":    "absent",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestNetconfRoutingModule_WithVRF(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfRoutingModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"prefix":   "10.0.0.0/8",
			"next_hop": "10.0.0.1",
			"vrf":      "mgmt",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestNetconfRoutingModule_DryRun(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfRoutingModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		DryRun:   true,
		Parameters: map[string]interface{}{
			"prefix":   "10.0.0.0/8",
			"next_hop": "10.0.0.1",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if exec.hasCommand("commit") {
		t.Error("should not commit in dry-run")
	}
}

func TestNetconfRoutingModule_Check(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfRoutingModule()
	result, err := module.Check(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"prefix":   "10.0.0.0/8",
			"next_hop": "10.0.0.1",
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
// netconf_acl tests
// =============================================================================

func TestNetconfACLModule_RequiredParams(t *testing.T) {
	module := NewNetconfACLModule()
	_, err := module.Execute(context.Background(), ModuleContext{
		Parameters: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestNetconfACLModule_PresentNoChange(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<acl-set><name>BLOCK-RFC1918</name><type>ACL_IPV4</type><acl-entries><acl-entry><sequence-id>10</sequence-id></acl-entry></acl-entries></acl-set>`,
	}

	module := NewNetconfACLModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name": "BLOCK-RFC1918",
			"entries": []interface{}{
				map[string]interface{}{
					"sequence": float64(10),
					"action":   "deny",
					"source":   "10.0.0.0/8",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Error("expected no change")
	}
}

func TestNetconfACLModule_PresentCreate(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfACLModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name": "ALLOW-WEB",
			"entries": []interface{}{
				map[string]interface{}{
					"sequence":    float64(10),
					"action":      "permit",
					"protocol":    "tcp",
					"source":      "192.168.1.0/24",
					"destination": "10.0.0.0/8",
				},
				map[string]interface{}{
					"sequence": float64(20),
					"action":   "deny",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if !exec.hasCommand("edit-config candidate") {
		t.Error("expected edit-config command")
	}
}

func TestNetconfACLModule_PresentIPv6(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfACLModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name": "V6-FILTER",
			"type": "ipv6",
			"entries": []interface{}{
				map[string]interface{}{
					"sequence":    float64(10),
					"action":      "permit",
					"source":      "2001:db8::/32",
					"destination": "::/0",
				},
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

func TestNetconfACLModule_Absent(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{
		stdout: `<acl-set><name>OLD-ACL</name><type>ACL_IPV4</type></acl-set>`,
	}

	module := NewNetconfACLModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "OLD-ACL",
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

func TestNetconfACLModule_AbsentNotExists(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfACLModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name":  "NONEXISTENT",
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

func TestNetconfACLModule_DryRun(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfACLModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		DryRun:   true,
		Parameters: map[string]interface{}{
			"name": "TEST-ACL",
			"entries": []interface{}{
				map[string]interface{}{
					"sequence": float64(10),
					"action":   "permit",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
	if exec.hasCommand("commit") {
		t.Error("should not commit in dry-run")
	}
}

func TestNetconfACLModule_Check(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfACLModule()
	result, err := module.Check(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name": "TEST-ACL",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}
}

func TestNetconfACLModule_NoEntries(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["get-config running"] = netconfResponse{stdout: ""}

	module := NewNetconfACLModule()
	result, err := module.Execute(context.Background(), ModuleContext{
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
		Executor: exec,
		Parameters: map[string]interface{}{
			"name": "EMPTY-ACL",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Error("expected change for new ACL creation")
	}
}

// =============================================================================
// netconfEditConfig helper tests
// =============================================================================

func TestNetconfEditConfig_CommitError(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["commit"] = netconfResponse{err: fmt.Errorf("commit failed")}

	err := netconfEditConfig(context.Background(), ModuleContext{
		Executor: exec,
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
	}, "<config></config>")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "commit") {
		t.Errorf("expected commit error, got: %v", err)
	}
}

func TestNetconfEditConfig_ValidateError(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["validate"] = netconfResponse{err: fmt.Errorf("validation failed")}

	err := netconfEditConfig(context.Background(), ModuleContext{
		Executor: exec,
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
	}, "<config></config>")
	if err == nil {
		t.Fatal("expected error")
	}
	if !exec.hasCommand("discard-changes") {
		t.Error("expected discard-changes on validate error")
	}
}

func TestNetconfEditConfig_LockError(t *testing.T) {
	exec := newNetconfFakeExecutor()
	exec.responses["lock candidate"] = netconfResponse{err: fmt.Errorf("lock failed")}

	err := netconfEditConfig(context.Background(), ModuleContext{
		Executor: exec,
		Device:   &proxy.ProxiedDevice{ID: "dev-1"},
	}, "<config></config>")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "lock") {
		t.Errorf("expected lock error, got: %v", err)
	}
}
