package state

import (
	"context"
	"fmt"
	"strings"
)

// OpenConfig YANG namespace constants.
const (
	ocInterfacesNS     = "http://openconfig.net/yang/interfaces"
	ocInterfacesIPNS   = "http://openconfig.net/yang/interfaces/ip"
	ocVLANNS           = "http://openconfig.net/yang/vlan"
	ocNetworkInstance   = "http://openconfig.net/yang/network-instance"
	ocACLNS            = "http://openconfig.net/yang/acl"
)

// netconfGetConfig performs a NETCONF get-config with a subtree filter.
func netconfGetConfig(ctx context.Context, mctx ModuleContext, source, filterXML string) (string, error) {
	cmd := fmt.Sprintf("get-config %s %s", source, filterXML)
	result, err := mctx.ExecuteCommand(ctx, cmd)
	if err != nil {
		return "", err
	}
	return string(result.Stdout), nil
}

// netconfEditConfig performs lock → edit-config → validate → commit → unlock on the
// candidate datastore. In DryRun mode it discards changes instead of committing.
func netconfEditConfig(ctx context.Context, mctx ModuleContext, configXML string) error {
	if _, err := mctx.ExecuteCommand(ctx, "lock candidate"); err != nil {
		return fmt.Errorf("lock candidate: %w", err)
	}

	cmd := fmt.Sprintf("edit-config candidate %s", configXML)
	if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
		_, _ = mctx.ExecuteCommand(ctx, "discard-changes") //nolint:errcheck // best-effort cleanup
		_, _ = mctx.ExecuteCommand(ctx, "unlock candidate") //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("edit-config: %w", err)
	}

	if mctx.DryRun {
		_, _ = mctx.ExecuteCommand(ctx, "discard-changes") //nolint:errcheck // best-effort cleanup
		_, _ = mctx.ExecuteCommand(ctx, "unlock candidate") //nolint:errcheck // best-effort cleanup
		return nil
	}

	if _, err := mctx.ExecuteCommand(ctx, "validate candidate"); err != nil {
		_, _ = mctx.ExecuteCommand(ctx, "discard-changes") //nolint:errcheck // best-effort cleanup
		_, _ = mctx.ExecuteCommand(ctx, "unlock candidate") //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("validate: %w", err)
	}

	if _, err := mctx.ExecuteCommand(ctx, "commit"); err != nil {
		_, _ = mctx.ExecuteCommand(ctx, "discard-changes") //nolint:errcheck // best-effort cleanup
		_, _ = mctx.ExecuteCommand(ctx, "unlock candidate") //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("commit: %w", err)
	}

	_, _ = mctx.ExecuteCommand(ctx, "unlock candidate") //nolint:errcheck // best-effort cleanup
	return nil
}

// =============================================================================
// netconf_interface — Manage network interfaces via NETCONF (OpenConfig)
// =============================================================================

// NetconfInterfaceModule manages network interfaces via NETCONF.
type NetconfInterfaceModule struct {
	BaseProxyModule
}

// NewNetconfInterfaceModule creates a new NETCONF interface module.
func NewNetconfInterfaceModule() *NetconfInterfaceModule {
	return &NetconfInterfaceModule{BaseProxyModule{name: "netconf_interface"}}
}

// Execute runs the NETCONF interface module.
func (m *NetconfInterfaceModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	name, _ := m.GetString(mctx.Parameters, "name")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Build subtree filter for current interface config
	filter := `<interfaces xmlns="` + ocInterfacesNS + `"><interface><name>` + name + `</name></interface></interfaces>`
	current, err := netconfGetConfig(ctx, mctx, "running", filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get current config: %w", err)
	}

	exists := strings.Contains(current, "<name>"+name+"</name>")

	switch state {
	case "absent":
		if !exists {
			result.Comment = fmt.Sprintf("Interface %s already absent", name)
			return result, nil
		}
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Interface %s would be removed", name)
			return result, nil
		}
		deleteXML := `<config><interfaces xmlns="` + ocInterfacesNS + `"><interface operation="delete"><name>` + name + `</name></interface></interfaces></config>`
		if err := netconfEditConfig(ctx, mctx, deleteXML); err != nil {
			return nil, fmt.Errorf("failed to delete interface: %w", err)
		}
		result.Comment = fmt.Sprintf("Interface %s removed", name)
		return result, nil

	case "up", "down":
		enabled := state == "up"
		mctx.Parameters["enabled"] = enabled
	}

	// state == "present" (or "up"/"down" mapped to present with enabled set)
	description, _ := m.GetString(mctx.Parameters, "description")
	enabled, hasEnabled := m.GetBool(mctx.Parameters, "enabled")
	mtu, hasMTU := m.GetInt(mctx.Parameters, "mtu")
	ipAddr, hasIP := m.GetString(mctx.Parameters, "ip_address")
	ipPrefix, hasPrefix := m.GetInt(mctx.Parameters, "ip_prefix_length")

	// Check if changes are needed
	needsChange := !exists
	if exists {
		if description != "" && !strings.Contains(current, "<description>"+description+"</description>") {
			needsChange = true
		}
		if hasEnabled {
			enabledStr := "true"
			if !enabled {
				enabledStr = "false"
			}
			if !strings.Contains(current, "<enabled>"+enabledStr+"</enabled>") {
				needsChange = true
			}
		}
		if hasMTU && !strings.Contains(current, fmt.Sprintf("<mtu>%d</mtu>", mtu)) {
			needsChange = true
		}
		if hasIP && !strings.Contains(current, "<ip>"+ipAddr+"</ip>") {
			needsChange = true
		}
	}

	if !needsChange {
		result.Comment = fmt.Sprintf("Interface %s already configured", name)
		return result, nil
	}

	result.Changed = true

	// Build config XML
	var ifConfig strings.Builder
	ifConfig.WriteString(fmt.Sprintf("<name>%s</name>", name))
	if description != "" {
		ifConfig.WriteString(fmt.Sprintf("<description>%s</description>", description))
	}
	if hasEnabled {
		ifConfig.WriteString(fmt.Sprintf("<enabled>%t</enabled>", enabled))
	}
	if hasMTU {
		ifConfig.WriteString(fmt.Sprintf("<mtu>%d</mtu>", mtu))
	}

	var ipXML string
	if hasIP && hasPrefix {
		ipXML = `<subinterfaces><subinterface><index>0</index><ipv4 xmlns="` + ocInterfacesIPNS + `"><addresses><address><ip>` + ipAddr + `</ip><config><ip>` + ipAddr + `</ip><prefix-length>` + fmt.Sprintf("%d", ipPrefix) + `</prefix-length></config></address></addresses></ipv4></subinterface></subinterfaces>`
	}

	configXML := `<config><interfaces xmlns="` + ocInterfacesNS + `"><interface><name>` + name + `</name><config>` + ifConfig.String() + `</config>` + ipXML + `</interface></interfaces></config>`

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("Interface %s would be configured", name)
		result.Details["config_xml"] = configXML
		return result, nil
	}

	if err := netconfEditConfig(ctx, mctx, configXML); err != nil {
		return nil, fmt.Errorf("failed to configure interface: %w", err)
	}

	result.Comment = fmt.Sprintf("Interface %s configured", name)
	result.Details["config_xml"] = configXML
	return result, nil
}

// Check performs a dry-run check.
func (m *NetconfInterfaceModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// netconf_vlan — Manage VLANs via NETCONF (OpenConfig)
// =============================================================================

// NetconfVLANModule manages VLANs via NETCONF.
type NetconfVLANModule struct {
	BaseProxyModule
}

// NewNetconfVLANModule creates a new NETCONF VLAN module.
func NewNetconfVLANModule() *NetconfVLANModule {
	return &NetconfVLANModule{BaseProxyModule{name: "netconf_vlan"}}
}

// Execute runs the NETCONF VLAN module.
func (m *NetconfVLANModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	vlanID, hasID := m.GetInt(mctx.Parameters, "vlan_id")
	if !hasID {
		return nil, fmt.Errorf("vlan_id parameter is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	filter := fmt.Sprintf(`<vlans xmlns="`+ocVLANNS+`"><vlan><vlan-id>%d</vlan-id></vlan></vlans>`, vlanID)
	current, err := netconfGetConfig(ctx, mctx, "running", filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get current config: %w", err)
	}

	exists := strings.Contains(current, fmt.Sprintf("<vlan-id>%d</vlan-id>", vlanID))

	if state == "absent" {
		if !exists {
			result.Comment = fmt.Sprintf("VLAN %d already absent", vlanID)
			return result, nil
		}
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("VLAN %d would be removed", vlanID)
			return result, nil
		}
		deleteXML := fmt.Sprintf(`<config><vlans xmlns="`+ocVLANNS+`"><vlan operation="delete"><vlan-id>%d</vlan-id></vlan></vlans></config>`, vlanID)
		if err := netconfEditConfig(ctx, mctx, deleteXML); err != nil {
			return nil, fmt.Errorf("failed to delete VLAN: %w", err)
		}
		result.Comment = fmt.Sprintf("VLAN %d removed", vlanID)
		return result, nil
	}

	// state == "present"
	vlanName, _ := m.GetString(mctx.Parameters, "name")
	vlanState, _ := m.GetString(mctx.Parameters, "vlan_state")

	needsChange := !exists
	if exists {
		if vlanName != "" && !strings.Contains(current, "<name>"+vlanName+"</name>") {
			needsChange = true
		}
		if vlanState != "" {
			statusVal := "ACTIVE"
			if vlanState == "suspend" {
				statusVal = "SUSPENDED"
			}
			if !strings.Contains(current, "<status>"+statusVal+"</status>") {
				needsChange = true
			}
		}
	}

	if !needsChange {
		result.Comment = fmt.Sprintf("VLAN %d already configured", vlanID)
		return result, nil
	}

	result.Changed = true

	var vlanConfig strings.Builder
	vlanConfig.WriteString(fmt.Sprintf("<vlan-id>%d</vlan-id>", vlanID))
	if vlanName != "" {
		vlanConfig.WriteString(fmt.Sprintf("<name>%s</name>", vlanName))
	}
	if vlanState != "" {
		statusVal := "ACTIVE"
		if vlanState == "suspend" {
			statusVal = "SUSPENDED"
		}
		vlanConfig.WriteString(fmt.Sprintf("<status>%s</status>", statusVal))
	}

	configXML := fmt.Sprintf(`<config><vlans xmlns="`+ocVLANNS+`"><vlan><vlan-id>%d</vlan-id><config>%s</config></vlan></vlans></config>`,
		vlanID, vlanConfig.String())

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("VLAN %d would be configured", vlanID)
		result.Details["config_xml"] = configXML
		return result, nil
	}

	if err := netconfEditConfig(ctx, mctx, configXML); err != nil {
		return nil, fmt.Errorf("failed to configure VLAN: %w", err)
	}

	result.Comment = fmt.Sprintf("VLAN %d configured", vlanID)
	result.Details["config_xml"] = configXML
	return result, nil
}

// Check performs a dry-run check.
func (m *NetconfVLANModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// netconf_routing — Manage static routes via NETCONF (OpenConfig)
// =============================================================================

// NetconfRoutingModule manages static routes via NETCONF.
type NetconfRoutingModule struct {
	BaseProxyModule
}

// NewNetconfRoutingModule creates a new NETCONF routing module.
func NewNetconfRoutingModule() *NetconfRoutingModule {
	return &NetconfRoutingModule{BaseProxyModule{name: "netconf_routing"}}
}

// Execute runs the NETCONF routing module.
func (m *NetconfRoutingModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	prefix, _ := m.GetString(mctx.Parameters, "prefix")
	if prefix == "" {
		return nil, fmt.Errorf("prefix parameter is required")
	}

	nextHop, _ := m.GetString(mctx.Parameters, "next_hop")
	if nextHop == "" {
		return nil, fmt.Errorf("next_hop parameter is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	vrf, _ := m.GetString(mctx.Parameters, "vrf")
	if vrf == "" {
		vrf = "default"
	}

	metric, hasMetric := m.GetInt(mctx.Parameters, "metric")

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	filter := `<network-instances xmlns="` + ocNetworkInstance + `"><network-instance><name>` + vrf + `</name><protocols><protocol><identifier>STATIC</identifier><name>DEFAULT</name><static-routes><static><prefix>` + prefix + `</prefix></static></static-routes></protocol></protocols></network-instance></network-instances>`

	current, err := netconfGetConfig(ctx, mctx, "running", filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get current config: %w", err)
	}

	exists := strings.Contains(current, "<prefix>"+prefix+"</prefix>")

	if state == "absent" {
		if !exists {
			result.Comment = fmt.Sprintf("Route %s already absent", prefix)
			return result, nil
		}
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Route %s would be removed", prefix)
			return result, nil
		}
		deleteXML := `<config><network-instances xmlns="` + ocNetworkInstance + `"><network-instance><name>` + vrf + `</name><protocols><protocol><identifier>STATIC</identifier><name>DEFAULT</name><static-routes><static operation="delete"><prefix>` + prefix + `</prefix></static></static-routes></protocol></protocols></network-instance></network-instances></config>`
		if err := netconfEditConfig(ctx, mctx, deleteXML); err != nil {
			return nil, fmt.Errorf("failed to delete route: %w", err)
		}
		result.Comment = fmt.Sprintf("Route %s removed", prefix)
		return result, nil
	}

	// state == "present"
	needsChange := !exists
	if exists {
		if !strings.Contains(current, "<next-hop>"+nextHop+"</next-hop>") {
			needsChange = true
		}
		if hasMetric && !strings.Contains(current, fmt.Sprintf("<metric>%d</metric>", metric)) {
			needsChange = true
		}
	}

	if !needsChange {
		result.Comment = fmt.Sprintf("Route %s already configured", prefix)
		return result, nil
	}

	result.Changed = true

	var nhConfig strings.Builder
	nhConfig.WriteString(fmt.Sprintf("<next-hop>%s</next-hop>", nextHop))
	if hasMetric {
		nhConfig.WriteString(fmt.Sprintf("<metric>%d</metric>", metric))
	}

	configXML := `<config><network-instances xmlns="` + ocNetworkInstance + `"><network-instance><name>` + vrf + `</name><protocols><protocol><identifier>STATIC</identifier><name>DEFAULT</name><static-routes><static><prefix>` + prefix + `</prefix><config><prefix>` + prefix + `</prefix></config><next-hops><next-hop><index>0</index><config><index>0</index>` + nhConfig.String() + `</config></next-hop></next-hops></static></static-routes></protocol></protocols></network-instance></network-instances></config>`

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("Route %s would be configured", prefix)
		result.Details["config_xml"] = configXML
		return result, nil
	}

	if err := netconfEditConfig(ctx, mctx, configXML); err != nil {
		return nil, fmt.Errorf("failed to configure route: %w", err)
	}

	result.Comment = fmt.Sprintf("Route %s configured", prefix)
	result.Details["config_xml"] = configXML
	return result, nil
}

// Check performs a dry-run check.
func (m *NetconfRoutingModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// netconf_acl — Manage access control lists via NETCONF (OpenConfig)
// =============================================================================

// NetconfACLModule manages access control lists via NETCONF.
type NetconfACLModule struct {
	BaseProxyModule
}

// NewNetconfACLModule creates a new NETCONF ACL module.
func NewNetconfACLModule() *NetconfACLModule {
	return &NetconfACLModule{BaseProxyModule{name: "netconf_acl"}}
}

// Execute runs the NETCONF ACL module.
func (m *NetconfACLModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	name, _ := m.GetString(mctx.Parameters, "name")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	aclType, _ := m.GetString(mctx.Parameters, "type")
	if aclType == "" {
		aclType = "ipv4"
	}
	aclTypeUpper := "ACL_IPV4"
	if aclType == "ipv6" {
		aclTypeUpper = "ACL_IPV6"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	filter := `<acl xmlns="` + ocACLNS + `"><acl-sets><acl-set><name>` + name + `</name><type>` + aclTypeUpper + `</type></acl-set></acl-sets></acl>`

	current, err := netconfGetConfig(ctx, mctx, "running", filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get current config: %w", err)
	}

	exists := strings.Contains(current, "<name>"+name+"</name>")

	if state == "absent" {
		if !exists {
			result.Comment = fmt.Sprintf("ACL %s already absent", name)
			return result, nil
		}
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("ACL %s would be removed", name)
			return result, nil
		}
		deleteXML := `<config><acl xmlns="` + ocACLNS + `"><acl-sets><acl-set operation="delete"><name>` + name + `</name><type>` + aclTypeUpper + `</type></acl-set></acl-sets></acl></config>`
		if err := netconfEditConfig(ctx, mctx, deleteXML); err != nil {
			return nil, fmt.Errorf("failed to delete ACL: %w", err)
		}
		result.Comment = fmt.Sprintf("ACL %s removed", name)
		return result, nil
	}

	// state == "present"
	entries, _ := mctx.Parameters["entries"].([]interface{})

	needsChange := !exists
	if exists && len(entries) > 0 {
		for _, entry := range entries {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			seq, _ := entryMap["sequence"].(float64)
			if seq > 0 && !strings.Contains(current, fmt.Sprintf("<sequence-id>%d</sequence-id>", int(seq))) {
				needsChange = true
				break
			}
		}
	}

	if !needsChange {
		result.Comment = fmt.Sprintf("ACL %s already configured", name)
		return result, nil
	}

	result.Changed = true

	var entriesXML strings.Builder
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		seq := 10
		switch s := entryMap["sequence"].(type) {
		case float64:
			seq = int(s)
		case int:
			seq = s
		}

		action := "ACCEPT"
		if a, ok := entryMap["action"].(string); ok && a == "deny" {
			action = "DROP"
		}

		source, _ := entryMap["source"].(string)
		if source == "" {
			source = "0.0.0.0/0"
		}
		destination, _ := entryMap["destination"].(string)
		if destination == "" {
			destination = "0.0.0.0/0"
		}
		protocol, _ := entryMap["protocol"].(string)

		var ipFilterXML string
		if aclType == "ipv4" {
			var protoXML string
			if protocol != "" {
				protoXML = fmt.Sprintf("<protocol>%s</protocol>", protocol)
			}
			ipFilterXML = fmt.Sprintf(`<ipv4><config><source-address>%s</source-address><destination-address>%s</destination-address>%s</config></ipv4>`,
				source, destination, protoXML)
		} else {
			var protoXML string
			if protocol != "" {
				protoXML = fmt.Sprintf("<protocol>%s</protocol>", protocol)
			}
			ipFilterXML = fmt.Sprintf(`<ipv6><config><source-address>%s</source-address><destination-address>%s</destination-address>%s</config></ipv6>`,
				source, destination, protoXML)
		}

		entriesXML.WriteString(fmt.Sprintf(`<acl-entry><sequence-id>%d</sequence-id><config><sequence-id>%d</sequence-id></config>%s<actions><config><forwarding-action>%s</forwarding-action></config></actions></acl-entry>`,
			seq, seq, ipFilterXML, action))
	}

	configXML := `<config><acl xmlns="` + ocACLNS + `"><acl-sets><acl-set><name>` + name + `</name><type>` + aclTypeUpper + `</type><config><name>` + name + `</name><type>` + aclTypeUpper + `</type></config><acl-entries>` + entriesXML.String() + `</acl-entries></acl-set></acl-sets></acl></config>`

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("ACL %s would be configured", name)
		result.Details["config_xml"] = configXML
		return result, nil
	}

	if err := netconfEditConfig(ctx, mctx, configXML); err != nil {
		return nil, fmt.Errorf("failed to configure ACL: %w", err)
	}

	result.Comment = fmt.Sprintf("ACL %s configured", name)
	result.Details["config_xml"] = configXML
	return result, nil
}

// Check performs a dry-run check.
func (m *NetconfACLModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}
