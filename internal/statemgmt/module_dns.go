package statemgmt

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// DNSModule implements DNS record management
type DNSModule struct {
	*BaseModule
	providerRegistry *dns.Registry
}

// NewDNSModule creates a new DNS module
func NewDNSModule() *DNSModule {
	return &DNSModule{
		BaseModule:       NewBaseModule("dns_records", []string{"present", "synced", "absent"}),
		providerRegistry: dns.DefaultRegistry,
	}
}

// NewDNSModuleWithRegistry creates a new DNS module with a custom provider registry
func NewDNSModuleWithRegistry(registry *dns.Registry) *DNSModule {
	return &DNSModule{
		BaseModule:       NewBaseModule("dns_records", []string{"present", "synced", "absent"}),
		providerRegistry: registry,
	}
}

// Check checks the current state of DNS records
func (m *DNSModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	// Parse parameters
	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Get provider
	provider, err := m.getProvider(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	// Get current records from provider
	current, err := provider.GetRecords(ctx, config.Zone)
	if err != nil {
		return nil, fmt.Errorf("failed to get current records: %w", err)
	}

	result.Present = len(current) > 0
	result.Metadata["zone"] = config.Zone
	result.Metadata["provider"] = config.Provider
	result.Metadata["current_record_count"] = len(current)
	result.Metadata["desired_record_count"] = len(config.Records)

	// Handle "absent" state
	if decl.State == "absent" {
		// All desired records should not exist
		differ := dns.NewDiffer(config.Zone)
		plan := differ.Diff([]dns.Record{}, current) // Desired is empty

		if !plan.HasChanges() {
			result.Matches = true
			result.CurrentState = "absent"
		} else {
			result.Matches = false
			result.CurrentState = "present"
			result.Diff["records_to_delete"] = plan.Summary().Delete
		}
		return result, nil
	}

	// Handle "present" or "synced" state
	differ := dns.NewDiffer(config.Zone)
	differ.IgnoreTTL = config.IgnoreTTL
	differ.IgnoreProxied = config.IgnoreProxied

	plan := differ.Diff(config.Records, current)
	summary := plan.Summary()

	result.Metadata["plan_summary"] = summary.String()

	if !plan.HasChanges() {
		result.Matches = true
		result.CurrentState = decl.State
	} else {
		result.Matches = false
		result.CurrentState = "drift"

		// Record the drift details
		if summary.Create > 0 {
			result.Diff["records_to_create"] = summary.Create
		}
		if summary.Update > 0 {
			result.Diff["records_to_update"] = summary.Update
		}
		if summary.Delete > 0 && decl.State == "synced" {
			result.Diff["records_to_delete"] = summary.Delete
		}

		// Include detailed changes
		changes := make([]map[string]interface{}, 0)
		for _, change := range plan.Changes {
			if change.Type == dns.ChangeTypeNoop {
				continue
			}
			changeInfo := map[string]interface{}{
				"action": string(change.Type),
				"type":   string(change.Record.Type),
				"name":   change.Record.Name,
				"value":  change.Record.Value,
			}
			if change.Diff != nil {
				changeInfo["diff"] = change.Diff
			}
			changes = append(changes, changeInfo)
		}
		result.Diff["changes"] = changes
	}

	return result, nil
}

// Apply applies the DNS record state
func (m *DNSModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	// Parse parameters
	config, err := m.parseConfig(decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Invalid configuration: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Get provider
	provider, err := m.getProvider(config)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to get provider: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Check current state first
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "DNS records already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	var syncResult *dns.SyncResult
	switch decl.State {
	case "absent":
		syncResult, err = m.applyAbsent(ctx, provider, config)
	case "present":
		syncResult, err = m.applyPresent(ctx, provider, config)
	case "synced":
		syncResult, err = m.applySynced(ctx, provider, config)
	default:
		err = fmt.Errorf("unsupported state: %s", decl.State)
	}

	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to apply state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Record results
	if syncResult != nil {
		result.Changes["created"] = syncResult.Created
		result.Changes["updated"] = syncResult.Updated
		result.Changes["deleted"] = syncResult.Deleted
		result.Changes["unchanged"] = syncResult.Unchanged

		if syncResult.HasErrors() {
			result.Success = false
			result.Comment = fmt.Sprintf("Partial failure: %d errors", len(syncResult.Errors))
			result.Error = syncResult.Errors[0] // Report first error
		} else {
			result.Success = true
			result.Changed = syncResult.TotalChanges() > 0
			result.Comment = fmt.Sprintf("DNS records synchronized: %d created, %d updated, %d deleted",
				syncResult.Created, syncResult.Updated, syncResult.Deleted)
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// applyAbsent removes all specified records
func (m *DNSModule) applyAbsent(ctx context.Context, provider dns.Provider, config *dnsConfig) (*dns.SyncResult, error) {
	syncer := dns.NewSyncer(provider, config.Zone, dns.SyncOptions{
		DeleteExisting: true,
		IgnoreTTL:      config.IgnoreTTL,
		IgnoreProxied:  config.IgnoreProxied,
	})

	// Sync with empty desired state to delete all
	return syncer.Sync(ctx, []dns.Record{})
}

// applyPresent ensures specified records exist (does not delete extras)
func (m *DNSModule) applyPresent(ctx context.Context, provider dns.Provider, config *dnsConfig) (*dns.SyncResult, error) {
	syncer := dns.NewSyncer(provider, config.Zone, dns.SyncOptions{
		DeleteExisting: false, // Don't delete records not in desired state
		IgnoreTTL:      config.IgnoreTTL,
		IgnoreProxied:  config.IgnoreProxied,
	})

	return syncer.Sync(ctx, config.Records)
}

// applySynced ensures exact match (creates, updates, and deletes)
func (m *DNSModule) applySynced(ctx context.Context, provider dns.Provider, config *dnsConfig) (*dns.SyncResult, error) {
	syncer := dns.NewSyncer(provider, config.Zone, dns.SyncOptions{
		DeleteExisting: true, // Delete records not in desired state
		IgnoreTTL:      config.IgnoreTTL,
		IgnoreProxied:  config.IgnoreProxied,
	})

	return syncer.Sync(ctx, config.Records)
}

// Test tests if the DNS state is correct
func (m *DNSModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// dnsConfig holds parsed DNS module configuration
type dnsConfig struct {
	Zone          string
	Provider      string
	Credentials   dns.ResolvedCredentials
	Records       []dns.Record
	IgnoreTTL     bool
	IgnoreProxied bool
}

// parseConfig parses and validates the state declaration
func (m *DNSModule) parseConfig(decl *StateDeclaration) (*dnsConfig, error) {
	config := &dnsConfig{}

	// Zone is required
	config.Zone = getStringParameter(decl, "zone", "")
	if config.Zone == "" {
		// Try using the state ID as the zone
		config.Zone = decl.ID
	}
	if config.Zone == "" {
		return nil, fmt.Errorf("zone is required")
	}

	if !dns.IsValidZone(config.Zone) {
		return nil, fmt.Errorf("invalid zone name: %s", config.Zone)
	}

	// Provider is required
	config.Provider = getStringParameter(decl, "provider", "")
	if config.Provider == "" {
		return nil, fmt.Errorf("provider is required")
	}

	// Parse credentials
	config.Credentials = m.parseCredentials(decl)

	// Parse options
	config.IgnoreTTL = getBoolParameter(decl, "ignore_ttl", false)
	config.IgnoreProxied = getBoolParameter(decl, "ignore_proxied", false)

	// Parse records
	records, err := m.parseRecords(decl)
	if err != nil {
		return nil, fmt.Errorf("invalid records: %w", err)
	}
	config.Records = records

	return config, nil
}

// parseCredentials parses credentials from the state declaration
func (m *DNSModule) parseCredentials(decl *StateDeclaration) dns.ResolvedCredentials {
	creds := dns.ResolvedCredentials{
		Extra: make(map[string]string),
	}

	// Check for credentials object
	if credsParam, ok := decl.Parameters["credentials"]; ok {
		if credsMap, ok := credsParam.(map[string]interface{}); ok {
			if v, ok := credsMap["api_key"].(string); ok {
				creds.APIKey = v
			}
			if v, ok := credsMap["api_token"].(string); ok {
				creds.APIToken = v
			}
			if v, ok := credsMap["account_id"].(string); ok {
				creds.AccountID = v
			}
			if extra, ok := credsMap["extra"].(map[string]interface{}); ok {
				for k, v := range extra {
					if str, ok := v.(string); ok {
						creds.Extra[k] = str
					}
				}
			}
		}
	}

	// Also check top-level parameters for convenience
	if v := getStringParameter(decl, "api_key", ""); v != "" {
		creds.APIKey = v
	}
	if v := getStringParameter(decl, "api_token", ""); v != "" {
		creds.APIToken = v
	}
	if v := getStringParameter(decl, "account_id", ""); v != "" {
		creds.AccountID = v
	}

	return creds
}

// parseRecords parses DNS records from the state declaration
func (m *DNSModule) parseRecords(decl *StateDeclaration) ([]dns.Record, error) {
	recordsParam, ok := decl.Parameters["records"]
	if !ok {
		return []dns.Record{}, nil
	}

	recordsList, ok := recordsParam.([]interface{})
	if !ok {
		return nil, fmt.Errorf("records must be a list")
	}

	records := make([]dns.Record, 0, len(recordsList))
	for i, item := range recordsList {
		recordMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("record[%d] must be an object", i)
		}

		record, err := m.parseRecord(recordMap, i)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, nil
}

// parseRecord parses a single DNS record from a map
func (m *DNSModule) parseRecord(recordMap map[string]interface{}, index int) (dns.Record, error) {
	record := dns.Record{}

	// Type is required
	if typeStr, ok := recordMap["type"].(string); ok {
		record.Type = dns.RecordType(typeStr)
	} else {
		return record, fmt.Errorf("record[%d]: type is required", index)
	}

	// Name is required
	if name, ok := recordMap["name"].(string); ok {
		record.Name = name
	} else {
		return record, fmt.Errorf("record[%d]: name is required", index)
	}

	// Value is required
	if value, ok := recordMap["value"].(string); ok {
		record.Value = value
	} else {
		return record, fmt.Errorf("record[%d]: value is required", index)
	}

	// TTL is optional (default will be set by Normalize)
	if ttl, ok := recordMap["ttl"]; ok {
		switch v := ttl.(type) {
		case int:
			record.TTL = v
		case int64:
			record.TTL = int(v)
		case float64:
			record.TTL = int(v)
		}
	}

	// Priority for MX/SRV
	if priority, ok := recordMap["priority"]; ok {
		switch v := priority.(type) {
		case int:
			record.Priority = v
		case int64:
			record.Priority = int(v)
		case float64:
			record.Priority = int(v)
		}
	}

	// Weight for SRV
	if weight, ok := recordMap["weight"]; ok {
		switch v := weight.(type) {
		case int:
			record.Weight = v
		case int64:
			record.Weight = int(v)
		case float64:
			record.Weight = int(v)
		}
	}

	// Port for SRV
	if port, ok := recordMap["port"]; ok {
		switch v := port.(type) {
		case int:
			record.Port = v
		case int64:
			record.Port = int(v)
		case float64:
			record.Port = int(v)
		}
	}

	// Proxied for Cloudflare
	if proxied, ok := recordMap["proxied"].(bool); ok {
		record.Proxied = &proxied
	}

	// Validate the record
	if err := record.Validate(); err != nil {
		return record, fmt.Errorf("record[%d]: %w", index, err)
	}

	return record, nil
}

// getProvider creates a provider instance from the configuration
func (m *DNSModule) getProvider(config *dnsConfig) (dns.Provider, error) {
	return m.providerRegistry.CreateProvider(config.Provider, config.Credentials)
}

func init() {
	_ = RegisterModule(NewDNSModule()) //nolint:errcheck // module registration in init
}
