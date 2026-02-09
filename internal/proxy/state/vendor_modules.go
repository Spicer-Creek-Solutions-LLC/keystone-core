package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// =============================================================================
// fortios_policy — Manage FortiGate firewall policies via REST API
// =============================================================================

// FortiOSPolicyModule manages FortiGate firewall policies.
type FortiOSPolicyModule struct {
	BaseProxyModule
}

// NewFortiOSPolicyModule creates a new FortiOS policy module.
func NewFortiOSPolicyModule() *FortiOSPolicyModule {
	return &FortiOSPolicyModule{BaseProxyModule{name: "fortios_policy"}}
}

// Execute runs the FortiOS policy module.
func (m *FortiOSPolicyModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	policyID, hasID := m.GetInt(mctx.Parameters, "policy_id")
	if !hasID {
		return nil, fmt.Errorf("policy_id parameter is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Check if policy exists
	getPath := fmt.Sprintf("GET /api/v2/cmdb/firewall/policy/%d", policyID)
	getResult, _ := mctx.ExecuteCommand(ctx, getPath)
	exists := getResult.ExitCode == 0 && len(getResult.Stdout) > 0

	var currentPolicy map[string]interface{}
	if exists {
		_ = json.Unmarshal(getResult.Stdout, &currentPolicy)
	}

	if state == "absent" {
		if !exists {
			result.Comment = fmt.Sprintf("Policy %d already absent", policyID)
			return result, nil
		}
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Policy %d would be removed", policyID)
			return result, nil
		}
		deletePath := fmt.Sprintf("DELETE /api/v2/cmdb/firewall/policy/%d", policyID)
		if _, err := mctx.ExecuteCommand(ctx, deletePath); err != nil {
			return nil, fmt.Errorf("failed to delete policy: %w", err)
		}
		result.Comment = fmt.Sprintf("Policy %d removed", policyID)
		return result, nil
	}

	// Build policy body
	policy := map[string]interface{}{
		"policyid": policyID,
	}

	if name, ok := m.GetString(mctx.Parameters, "name"); ok {
		policy["name"] = name
	}
	if action, ok := m.GetString(mctx.Parameters, "action"); ok {
		policy["action"] = action
	}
	if schedule, ok := m.GetString(mctx.Parameters, "schedule"); ok {
		policy["schedule"] = schedule
	}
	if logtraffic, ok := m.GetString(mctx.Parameters, "logtraffic"); ok {
		policy["logtraffic"] = logtraffic
	}
	if comment, ok := m.GetString(mctx.Parameters, "comment"); ok {
		policy["comments"] = comment
	}
	if nat, ok := m.GetBool(mctx.Parameters, "nat"); ok {
		if nat {
			policy["nat"] = "enable"
		} else {
			policy["nat"] = "disable"
		}
	}

	// List fields use FortiOS array-of-objects format
	for _, field := range []struct{ param, api string }{
		{"srcintf", "srcintf"},
		{"dstintf", "dstintf"},
		{"srcaddr", "srcaddr"},
		{"dstaddr", "dstaddr"},
		{"service", "service"},
	} {
		if vals, ok := m.GetStringSlice(mctx.Parameters, field.param); ok {
			members := make([]map[string]string, len(vals))
			for i, v := range vals {
				members[i] = map[string]string{"name": v}
			}
			policy[field.api] = members
		}
	}

	// Check if changes needed
	needsChange := !exists
	if exists {
		policyJSON, _ := json.Marshal(policy)
		currentJSON, _ := json.Marshal(currentPolicy)
		if !bytes.Equal(policyJSON, currentJSON) {
			needsChange = true
		}
	}

	if !needsChange {
		result.Comment = fmt.Sprintf("Policy %d already configured", policyID)
		return result, nil
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("Policy %d would be configured", policyID)
		result.Details["policy"] = policy
		return result, nil
	}

	policyJSON, _ := json.Marshal(policy)

	if exists {
		putPath := fmt.Sprintf("PUT /api/v2/cmdb/firewall/policy/%d %s", policyID, string(policyJSON))
		if _, err := mctx.ExecuteCommand(ctx, putPath); err != nil {
			return nil, fmt.Errorf("failed to update policy: %w", err)
		}
		result.Comment = fmt.Sprintf("Policy %d updated", policyID)
	} else {
		postPath := fmt.Sprintf("POST /api/v2/cmdb/firewall/policy %s", string(policyJSON))
		if _, err := mctx.ExecuteCommand(ctx, postPath); err != nil {
			return nil, fmt.Errorf("failed to create policy: %w", err)
		}
		result.Comment = fmt.Sprintf("Policy %d created", policyID)
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *FortiOSPolicyModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// panos_rule — Manage Palo Alto PAN-OS security/NAT rules via XML API
// =============================================================================

// PANOSRuleModule manages PAN-OS security and NAT rules.
type PANOSRuleModule struct {
	BaseProxyModule
}

// NewPANOSRuleModule creates a new PAN-OS rule module.
func NewPANOSRuleModule() *PANOSRuleModule {
	return &PANOSRuleModule{BaseProxyModule{name: "panos_rule"}}
}

func (m *PANOSRuleModule) buildXPath(ruleType, name string) string {
	rulebase := "security"
	if ruleType == "nat" {
		rulebase = "nat"
	}
	return fmt.Sprintf("/config/devices/entry[@name='localhost.localdomain']/vsys/entry[@name='vsys1']/rulebase/%s/rules/entry[@name='%s']", rulebase, name)
}

// Execute runs the PAN-OS rule module.
func (m *PANOSRuleModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	name, _ := m.GetString(mctx.Parameters, "name")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	ruleType, _ := m.GetString(mctx.Parameters, "rule_type")
	if ruleType == "" {
		ruleType = "security"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	xpath := m.buildXPath(ruleType, name)

	// Check if rule exists
	getCmd := fmt.Sprintf("GET /api/?type=config&action=show&xpath=%s", xpath)
	getResult, _ := mctx.ExecuteCommand(ctx, getCmd)
	exists := getResult.ExitCode == 0 && strings.Contains(string(getResult.Stdout), name)

	if state == "absent" {
		if !exists {
			result.Comment = fmt.Sprintf("Rule %s already absent", name)
			return result, nil
		}
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Rule %s would be removed", name)
			return result, nil
		}
		deleteCmd := fmt.Sprintf("GET /api/?type=config&action=delete&xpath=%s", xpath)
		if _, err := mctx.ExecuteCommand(ctx, deleteCmd); err != nil {
			return nil, fmt.Errorf("failed to delete rule: %w", err)
		}
		if doCommit, _ := m.GetBool(mctx.Parameters, "commit"); doCommit {
			if _, err := mctx.ExecuteCommand(ctx, "POST /api/?type=commit&cmd=<commit></commit>"); err != nil {
				return nil, fmt.Errorf("failed to commit: %w", err)
			}
		}
		result.Comment = fmt.Sprintf("Rule %s removed", name)
		return result, nil
	}

	// Build rule XML element
	var ruleXML strings.Builder
	ruleXML.WriteString(`<entry name="` + name + `">`)

	for _, field := range []struct{ param, elem string }{
		{"source_zone", "from"},
		{"dest_zone", "to"},
		{"source", "source"},
		{"destination", "destination"},
		{"application", "application"},
		{"service", "service"},
	} {
		if vals, ok := m.GetStringSlice(mctx.Parameters, field.param); ok {
			ruleXML.WriteString(fmt.Sprintf("<%s>", field.elem))
			for _, v := range vals {
				ruleXML.WriteString(fmt.Sprintf("<member>%s</member>", v))
			}
			ruleXML.WriteString(fmt.Sprintf("</%s>", field.elem))
		}
	}

	if action, ok := m.GetString(mctx.Parameters, "action"); ok {
		ruleXML.WriteString(fmt.Sprintf("<action>%s</action>", action))
	}
	if logSetting, ok := m.GetString(mctx.Parameters, "log_setting"); ok {
		ruleXML.WriteString(fmt.Sprintf("<log-setting>%s</log-setting>", logSetting))
	}

	ruleXML.WriteString("</entry>")

	result.Changed = true

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("Rule %s would be configured", name)
		result.Details["rule_xml"] = ruleXML.String()
		return result, nil
	}

	setCmd := fmt.Sprintf("GET /api/?type=config&action=set&xpath=%s&element=%s", xpath, ruleXML.String())
	if _, err := mctx.ExecuteCommand(ctx, setCmd); err != nil {
		return nil, fmt.Errorf("failed to set rule: %w", err)
	}

	if doCommit, _ := m.GetBool(mctx.Parameters, "commit"); doCommit {
		if _, err := mctx.ExecuteCommand(ctx, "POST /api/?type=commit&cmd=<commit></commit>"); err != nil {
			return nil, fmt.Errorf("failed to commit: %w", err)
		}
	}

	if exists {
		result.Comment = fmt.Sprintf("Rule %s updated", name)
	} else {
		result.Comment = fmt.Sprintf("Rule %s created", name)
	}
	return result, nil
}

// Check performs a dry-run check.
func (m *PANOSRuleModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// bigip_pool — Manage F5 BIG-IP pools via iControl REST
// =============================================================================

// BigIPPoolModule manages F5 BIG-IP LTM pools.
type BigIPPoolModule struct {
	BaseProxyModule
}

// NewBigIPPoolModule creates a new BIG-IP pool module.
func NewBigIPPoolModule() *BigIPPoolModule {
	return &BigIPPoolModule{BaseProxyModule{name: "bigip_pool"}}
}

func bigipPoolPath(name string) string {
	escaped := strings.ReplaceAll(name, "/", "~")
	return fmt.Sprintf("/mgmt/tm/ltm/pool/%s", escaped)
}

// Execute runs the BIG-IP pool module.
func (m *BigIPPoolModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
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

	poolPath := bigipPoolPath(name)
	getCmd := fmt.Sprintf("GET %s", poolPath)
	getResult, _ := mctx.ExecuteCommand(ctx, getCmd)
	exists := getResult.ExitCode == 0 && len(getResult.Stdout) > 0

	var currentPool map[string]interface{}
	if exists {
		_ = json.Unmarshal(getResult.Stdout, &currentPool)
	}

	if state == "absent" {
		if !exists {
			result.Comment = fmt.Sprintf("Pool %s already absent", name)
			return result, nil
		}
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Pool %s would be removed", name)
			return result, nil
		}
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("DELETE %s", poolPath)); err != nil {
			return nil, fmt.Errorf("failed to delete pool: %w", err)
		}
		result.Comment = fmt.Sprintf("Pool %s removed", name)
		return result, nil
	}

	// Build pool config
	pool := map[string]interface{}{
		"name": name,
	}
	if lbMethod, ok := m.GetString(mctx.Parameters, "lb_method"); ok {
		pool["loadBalancingMode"] = lbMethod
	}
	if description, ok := m.GetString(mctx.Parameters, "description"); ok {
		pool["description"] = description
	}
	if monitors, ok := m.GetStringSlice(mctx.Parameters, "monitors"); ok {
		pool["monitor"] = strings.Join(monitors, " and ")
	}

	needsChange := !exists
	if exists {
		if lb, ok := pool["loadBalancingMode"].(string); ok {
			if cur, ok := currentPool["loadBalancingMode"].(string); !ok || cur != lb {
				needsChange = true
			}
		}
		if desc, ok := pool["description"].(string); ok {
			if cur, ok := currentPool["description"].(string); !ok || cur != desc {
				needsChange = true
			}
		}
	}

	if !needsChange {
		result.Comment = fmt.Sprintf("Pool %s already configured", name)
		return result, nil
	}

	result.Changed = true
	poolJSON, _ := json.Marshal(pool)

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("Pool %s would be configured", name)
		result.Details["pool"] = pool
		return result, nil
	}

	if exists {
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("PUT %s %s", poolPath, string(poolJSON))); err != nil {
			return nil, fmt.Errorf("failed to update pool: %w", err)
		}
		result.Comment = fmt.Sprintf("Pool %s updated", name)
	} else {
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("POST /mgmt/tm/ltm/pool %s", string(poolJSON))); err != nil {
			return nil, fmt.Errorf("failed to create pool: %w", err)
		}
		result.Comment = fmt.Sprintf("Pool %s created", name)
	}

	// Add members if specified
	if members, ok := mctx.Parameters["members"].([]interface{}); ok && !exists {
		for _, member := range members {
			memberMap, ok := member.(map[string]interface{})
			if !ok {
				continue
			}
			addr, _ := memberMap["address"].(string)
			port, _ := memberMap["port"].(float64)
			if addr == "" || port == 0 {
				continue
			}
			memberBody := map[string]interface{}{
				"name": fmt.Sprintf("%s:%d", addr, int(port)),
			}
			memberJSON, _ := json.Marshal(memberBody)
			membersPath := fmt.Sprintf("POST %s/members %s", poolPath, string(memberJSON))
			_, _ = mctx.ExecuteCommand(ctx, membersPath) //nolint:errcheck // best-effort member addition
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *BigIPPoolModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// bigip_virtual — Manage F5 BIG-IP virtual servers via iControl REST
// =============================================================================

// BigIPVirtualModule manages F5 BIG-IP LTM virtual servers.
type BigIPVirtualModule struct {
	BaseProxyModule
}

// NewBigIPVirtualModule creates a new BIG-IP virtual server module.
func NewBigIPVirtualModule() *BigIPVirtualModule {
	return &BigIPVirtualModule{BaseProxyModule{name: "bigip_virtual"}}
}

func bigipVirtualPath(name string) string {
	escaped := strings.ReplaceAll(name, "/", "~")
	return fmt.Sprintf("/mgmt/tm/ltm/virtual/%s", escaped)
}

// Execute runs the BIG-IP virtual server module.
func (m *BigIPVirtualModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
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

	vsPath := bigipVirtualPath(name)
	getCmd := fmt.Sprintf("GET %s", vsPath)
	getResult, _ := mctx.ExecuteCommand(ctx, getCmd)
	exists := getResult.ExitCode == 0 && len(getResult.Stdout) > 0

	var currentVS map[string]interface{}
	if exists {
		_ = json.Unmarshal(getResult.Stdout, &currentVS)
	}

	if state == "absent" {
		if !exists {
			result.Comment = fmt.Sprintf("Virtual server %s already absent", name)
			return result, nil
		}
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Virtual server %s would be removed", name)
			return result, nil
		}
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("DELETE %s", vsPath)); err != nil {
			return nil, fmt.Errorf("failed to delete virtual server: %w", err)
		}
		result.Comment = fmt.Sprintf("Virtual server %s removed", name)
		return result, nil
	}

	// Handle enabled/disabled states
	if state == "enabled" || state == "disabled" {
		if !exists {
			return nil, fmt.Errorf("virtual server %s does not exist", name)
		}
		enabledVal := state == "enabled"
		body := map[string]interface{}{"enabled": enabledVal, "disabled": !enabledVal}
		bodyJSON, _ := json.Marshal(body)
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Virtual server %s would be %s", name, state)
			return result, nil
		}
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("PUT %s %s", vsPath, string(bodyJSON))); err != nil {
			return nil, fmt.Errorf("failed to update virtual server: %w", err)
		}
		result.Comment = fmt.Sprintf("Virtual server %s %s", name, state)
		return result, nil
	}

	// Build virtual server config
	vs := map[string]interface{}{
		"name": name,
	}
	if dest, ok := m.GetString(mctx.Parameters, "destination"); ok {
		port, _ := m.GetInt(mctx.Parameters, "port")
		if port > 0 {
			vs["destination"] = fmt.Sprintf("%s:%d", dest, port)
		} else {
			vs["destination"] = dest
		}
	}
	if pool, ok := m.GetString(mctx.Parameters, "pool"); ok {
		vs["pool"] = pool
	}
	if snat, ok := m.GetString(mctx.Parameters, "snat"); ok {
		vs["sourceAddressTranslation"] = map[string]string{"type": snat}
	}
	if description, ok := m.GetString(mctx.Parameters, "description"); ok {
		vs["description"] = description
	}
	if profiles, ok := m.GetStringSlice(mctx.Parameters, "profiles"); ok {
		profileItems := make([]map[string]string, len(profiles))
		for i, p := range profiles {
			profileItems[i] = map[string]string{"name": p}
		}
		vs["profiles"] = profileItems
	}

	needsChange := !exists
	if exists {
		if pool, ok := vs["pool"].(string); ok {
			if cur, ok := currentVS["pool"].(string); !ok || cur != pool {
				needsChange = true
			}
		}
		if desc, ok := vs["description"].(string); ok {
			if cur, ok := currentVS["description"].(string); !ok || cur != desc {
				needsChange = true
			}
		}
	}

	if !needsChange {
		result.Comment = fmt.Sprintf("Virtual server %s already configured", name)
		return result, nil
	}

	result.Changed = true
	vsJSON, _ := json.Marshal(vs)

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("Virtual server %s would be configured", name)
		result.Details["virtual"] = vs
		return result, nil
	}

	if exists {
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("PUT %s %s", vsPath, string(vsJSON))); err != nil {
			return nil, fmt.Errorf("failed to update virtual server: %w", err)
		}
		result.Comment = fmt.Sprintf("Virtual server %s updated", name)
	} else {
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("POST /mgmt/tm/ltm/virtual %s", string(vsJSON))); err != nil {
			return nil, fmt.Errorf("failed to create virtual server: %w", err)
		}
		result.Comment = fmt.Sprintf("Virtual server %s created", name)
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *BigIPVirtualModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// checkpoint_rule — Manage Check Point firewall rules via Web Services API
// =============================================================================

// CheckpointRuleModule manages Check Point access rules.
type CheckpointRuleModule struct {
	BaseProxyModule
}

// NewCheckpointRuleModule creates a new Check Point rule module.
func NewCheckpointRuleModule() *CheckpointRuleModule {
	return &CheckpointRuleModule{BaseProxyModule{name: "checkpoint_rule"}}
}

// Execute runs the Check Point rule module.
func (m *CheckpointRuleModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	name, _ := m.GetString(mctx.Parameters, "name")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	layer, _ := m.GetString(mctx.Parameters, "layer")
	if layer == "" {
		layer = "Network"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Login to get session
	loginBody, _ := json.Marshal(map[string]string{})
	loginResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("POST /web_api/login %s", string(loginBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to login: %w", err)
	}

	var loginResp map[string]interface{}
	_ = json.Unmarshal(loginResult.Stdout, &loginResp)
	sid, _ := loginResp["sid"].(string)

	// Ensure logout at the end
	defer func() {
		logoutBody, _ := json.Marshal(map[string]string{})
		_, _ = mctx.ExecuteCommand(ctx, fmt.Sprintf("POST /web_api/logout %s", string(logoutBody))) //nolint:errcheck // best-effort logout
	}()

	// Check if rule exists
	showBody, _ := json.Marshal(map[string]interface{}{
		"name":  name,
		"layer": layer,
	})
	showResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("POST /web_api/show-access-rule %s", string(showBody)))
	exists := showResult.ExitCode == 0 && strings.Contains(string(showResult.Stdout), name)

	_ = sid // SID is passed via session context managed by the executor

	if state == "absent" {
		if !exists {
			result.Comment = fmt.Sprintf("Rule %s already absent", name)
			return result, nil
		}
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Rule %s would be removed", name)
			return result, nil
		}
		deleteBody, _ := json.Marshal(map[string]interface{}{
			"name":  name,
			"layer": layer,
		})
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("POST /web_api/delete-access-rule %s", string(deleteBody))); err != nil {
			return nil, fmt.Errorf("failed to delete rule: %w", err)
		}
		// Publish changes
		publishBody, _ := json.Marshal(map[string]string{})
		_, _ = mctx.ExecuteCommand(ctx, fmt.Sprintf("POST /web_api/publish %s", string(publishBody))) //nolint:errcheck // best-effort publish
		result.Comment = fmt.Sprintf("Rule %s removed", name)
		return result, nil
	}

	// Build rule body
	rule := map[string]interface{}{
		"name":  name,
		"layer": layer,
	}

	if position, ok := m.GetString(mctx.Parameters, "position"); ok {
		rule["position"] = position
	}
	if action, ok := m.GetString(mctx.Parameters, "action"); ok {
		rule["action"] = action
	}
	if track, ok := m.GetString(mctx.Parameters, "track"); ok {
		rule["track"] = map[string]string{"type": track}
	}
	if comment, ok := m.GetString(mctx.Parameters, "comment"); ok {
		rule["comments"] = comment
	}

	for _, field := range []string{"source", "destination", "service", "install_on"} {
		if vals, ok := m.GetStringSlice(mctx.Parameters, field); ok {
			apiField := strings.ReplaceAll(field, "_", "-")
			rule[apiField] = vals
		}
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("Rule %s would be configured", name)
		result.Details["rule"] = rule
		return result, nil
	}

	ruleJSON, _ := json.Marshal(rule)

	if exists {
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("POST /web_api/set-access-rule %s", string(ruleJSON))); err != nil {
			return nil, fmt.Errorf("failed to update rule: %w", err)
		}
		result.Comment = fmt.Sprintf("Rule %s updated", name)
	} else {
		if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("POST /web_api/add-access-rule %s", string(ruleJSON))); err != nil {
			return nil, fmt.Errorf("failed to create rule: %w", err)
		}
		result.Comment = fmt.Sprintf("Rule %s created", name)
	}

	// Publish changes
	publishBody, _ := json.Marshal(map[string]string{})
	_, _ = mctx.ExecuteCommand(ctx, fmt.Sprintf("POST /web_api/publish %s", string(publishBody))) //nolint:errcheck // best-effort publish

	return result, nil
}

// Check performs a dry-run check.
func (m *CheckpointRuleModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}
