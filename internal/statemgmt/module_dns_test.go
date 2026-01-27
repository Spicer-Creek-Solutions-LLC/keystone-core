package statemgmt

import (
	"context"
	"fmt"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// testDNSProvider implements dns.Provider for testing
type testDNSProvider struct {
	records      []dns.Record
	createErr    error
	updateErr    error
	deleteErr    error
	getErr       error
	createCalled int
	updateCalled int
	deleteCalled int
}

func (p *testDNSProvider) GetRecords(ctx context.Context, zone string) ([]dns.Record, error) {
	if p.getErr != nil {
		return nil, p.getErr
	}
	return p.records, nil
}

func (p *testDNSProvider) CreateRecord(ctx context.Context, zone string, record dns.Record) (*dns.Record, error) {
	p.createCalled++
	if p.createErr != nil {
		return nil, p.createErr
	}
	record.ID = fmt.Sprintf("created-%d", p.createCalled)
	p.records = append(p.records, record)
	return &record, nil
}

func (p *testDNSProvider) UpdateRecord(ctx context.Context, zone string, record dns.Record) (*dns.Record, error) {
	p.updateCalled++
	if p.updateErr != nil {
		return nil, p.updateErr
	}
	for i, r := range p.records {
		if r.ID == record.ID {
			p.records[i] = record
			return &record, nil
		}
	}
	return nil, fmt.Errorf("record not found: %s", record.ID)
}

func (p *testDNSProvider) DeleteRecord(ctx context.Context, zone string, record dns.Record) error {
	p.deleteCalled++
	if p.deleteErr != nil {
		return p.deleteErr
	}
	for i, r := range p.records {
		if r.ID == record.ID {
			p.records = append(p.records[:i], p.records[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("record not found: %s", record.ID)
}

func (p *testDNSProvider) Capabilities() dns.ProviderCapabilities {
	return dns.ProviderCapabilities{
		SupportedRecordTypes: dns.AllRecordTypes(),
	}
}

// setupTestModule creates a DNS module with a test provider
func setupTestModule(provider *testDNSProvider) *DNSModule {
	registry := dns.NewRegistry()
	registry.Register("test", func(creds dns.ResolvedCredentials) (dns.Provider, error) {
		return provider, nil
	}, dns.ProviderCapabilities{
		SupportedRecordTypes: dns.AllRecordTypes(),
	})
	return NewDNSModuleWithRegistry(registry)
}

func TestDNSModule_Name(t *testing.T) {
	module := NewDNSModule()
	if module.Name() != "dns_records" {
		t.Errorf("Name() = %v, want dns_records", module.Name())
	}
}

func TestDNSModule_ValidStates(t *testing.T) {
	module := NewDNSModule()
	states := module.ValidStates()

	expected := []string{"present", "synced", "absent"}
	if len(states) != len(expected) {
		t.Errorf("ValidStates() returned %d states, want %d", len(states), len(expected))
	}

	for i, state := range states {
		if state != expected[i] {
			t.Errorf("ValidStates()[%d] = %v, want %v", i, state, expected[i])
		}
	}
}

func TestDNSModule_Check_Present(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		records: []dns.Record{
			{Type: dns.RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
		},
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
			"records": []interface{}{
				map[string]interface{}{
					"type":  "A",
					"name":  "www",
					"value": "192.0.2.1",
					"ttl":   300,
				},
			},
		},
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !result.Matches {
		t.Error("Check() should match when records are present")
	}
	if result.Metadata["zone"] != "example.com" {
		t.Errorf("Metadata[zone] = %v, want example.com", result.Metadata["zone"])
	}
}

func TestDNSModule_Check_Drift(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		records: []dns.Record{
			{Type: dns.RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
		},
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
			"records": []interface{}{
				map[string]interface{}{
					"type":  "A",
					"name":  "www",
					"value": "192.0.2.1",
					"ttl":   600, // TTL differs
				},
			},
		},
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if result.Matches {
		t.Error("Check() should not match when TTL differs")
	}
	if result.CurrentState != "drift" {
		t.Errorf("CurrentState = %v, want drift", result.CurrentState)
	}
}

func TestDNSModule_Check_Absent(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		records: []dns.Record{},
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "absent",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
		},
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !result.Matches {
		t.Error("Check() should match when no records exist for absent state")
	}
	if result.CurrentState != "absent" {
		t.Errorf("CurrentState = %v, want absent", result.CurrentState)
	}
}

func TestDNSModule_Apply_Create(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		records: []dns.Record{},
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
			"records": []interface{}{
				map[string]interface{}{
					"type":  "A",
					"name":  "www",
					"value": "192.0.2.1",
					"ttl":   300,
				},
			},
		},
	}

	result, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Apply() Success = false, want true. Error: %v", result.Error)
	}
	if !result.Changed {
		t.Error("Apply() Changed = false, want true")
	}
	if provider.createCalled != 1 {
		t.Errorf("CreateCalled = %d, want 1", provider.createCalled)
	}
	if result.Changes["created"] != 1 {
		t.Errorf("Changes[created] = %v, want 1", result.Changes["created"])
	}
}

func TestDNSModule_Apply_Update(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		records: []dns.Record{
			{Type: dns.RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
		},
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
			"records": []interface{}{
				map[string]interface{}{
					"type":  "A",
					"name":  "www",
					"value": "192.0.2.1",
					"ttl":   600, // TTL change
				},
			},
		},
	}

	result, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Apply() Success = false, want true. Error: %v", result.Error)
	}
	if !result.Changed {
		t.Error("Apply() Changed = false, want true")
	}
	if provider.updateCalled != 1 {
		t.Errorf("UpdateCalled = %d, want 1", provider.updateCalled)
	}
}

func TestDNSModule_Apply_Synced_Delete(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		records: []dns.Record{
			{Type: dns.RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
			{Type: dns.RecordTypeA, Name: "old", Value: "192.0.2.2", TTL: 300, ID: "rec2"},
		},
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "synced",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
			"records": []interface{}{
				map[string]interface{}{
					"type":  "A",
					"name":  "www",
					"value": "192.0.2.1",
					"ttl":   300,
				},
				// "old" record is not in desired state
			},
		},
	}

	result, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Apply() Success = false, want true. Error: %v", result.Error)
	}
	if provider.deleteCalled != 1 {
		t.Errorf("DeleteCalled = %d, want 1", provider.deleteCalled)
	}
	if result.Changes["deleted"] != 1 {
		t.Errorf("Changes[deleted] = %v, want 1", result.Changes["deleted"])
	}
}

func TestDNSModule_Apply_Present_NoDelete(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		records: []dns.Record{
			{Type: dns.RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
			{Type: dns.RecordTypeA, Name: "other", Value: "192.0.2.2", TTL: 300, ID: "rec2"},
		},
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present", // present should not delete extra records
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
			"records": []interface{}{
				map[string]interface{}{
					"type":  "A",
					"name":  "www",
					"value": "192.0.2.1",
					"ttl":   300,
				},
			},
		},
	}

	result, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Apply() Success = false, want true. Error: %v", result.Error)
	}
	if provider.deleteCalled != 0 {
		t.Errorf("DeleteCalled = %d, want 0 (present state should not delete)", provider.deleteCalled)
	}
}

func TestDNSModule_Apply_AlreadyMatches(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		records: []dns.Record{
			{Type: dns.RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
		},
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
			"records": []interface{}{
				map[string]interface{}{
					"type":  "A",
					"name":  "www",
					"value": "192.0.2.1",
					"ttl":   300,
				},
			},
		},
	}

	result, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Apply() Success = false, want true")
	}
	if result.Changed {
		t.Error("Apply() Changed = true, want false (already in desired state)")
	}
	if result.Comment != "DNS records already in desired state" {
		t.Errorf("Comment = %v, want 'DNS records already in desired state'", result.Comment)
	}
}

func TestDNSModule_Test(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		records: []dns.Record{
			{Type: dns.RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
		},
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
			"records": []interface{}{
				map[string]interface{}{
					"type":  "A",
					"name":  "www",
					"value": "192.0.2.1",
					"ttl":   300,
				},
			},
		},
	}

	matches, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}

	if !matches {
		t.Error("Test() = false, want true")
	}
}

func TestDNSModule_parseConfig_MissingZone(t *testing.T) {
	module := NewDNSModule()

	decl := &StateDeclaration{
		ID:    "",
		State: "present",
		Parameters: map[string]interface{}{
			"provider": "test",
		},
	}

	_, err := module.parseConfig(decl)
	if err == nil {
		t.Error("parseConfig() should fail with missing zone")
	}
}

func TestDNSModule_parseConfig_InvalidZone(t *testing.T) {
	module := NewDNSModule()

	decl := &StateDeclaration{
		ID:    "invalid",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":     "invalid",
			"provider": "test",
		},
	}

	_, err := module.parseConfig(decl)
	if err == nil {
		t.Error("parseConfig() should fail with invalid zone")
	}
}

func TestDNSModule_parseConfig_MissingProvider(t *testing.T) {
	module := NewDNSModule()

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone": "example.com",
		},
	}

	_, err := module.parseConfig(decl)
	if err == nil {
		t.Error("parseConfig() should fail with missing provider")
	}
}

func TestDNSModule_parseRecords_InvalidRecord(t *testing.T) {
	module := NewDNSModule()

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
			"records": []interface{}{
				map[string]interface{}{
					// Missing required type
					"name":  "www",
					"value": "192.0.2.1",
				},
			},
		},
	}

	_, err := module.parseConfig(decl)
	if err == nil {
		t.Error("parseConfig() should fail with missing record type")
	}
}

func TestDNSModule_parseCredentials(t *testing.T) {
	module := NewDNSModule()

	tests := []struct {
		name       string
		params     map[string]interface{}
		wantKey    string
		wantToken  string
		wantAcctID string
	}{
		{
			name: "credentials object",
			params: map[string]interface{}{
				"credentials": map[string]interface{}{
					"api_key":    "key123",
					"api_token":  "token456",
					"account_id": "acct789",
				},
			},
			wantKey:    "key123",
			wantToken:  "token456",
			wantAcctID: "acct789",
		},
		{
			name: "top-level parameters",
			params: map[string]interface{}{
				"api_key":    "key123",
				"api_token":  "token456",
				"account_id": "acct789",
			},
			wantKey:    "key123",
			wantToken:  "token456",
			wantAcctID: "acct789",
		},
		{
			name:       "no credentials",
			params:     map[string]interface{}{},
			wantKey:    "",
			wantToken:  "",
			wantAcctID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := &StateDeclaration{Parameters: tt.params}
			creds := module.parseCredentials(decl)

			if creds.APIKey != tt.wantKey {
				t.Errorf("APIKey = %v, want %v", creds.APIKey, tt.wantKey)
			}
			if creds.APIToken != tt.wantToken {
				t.Errorf("APIToken = %v, want %v", creds.APIToken, tt.wantToken)
			}
			if creds.AccountID != tt.wantAcctID {
				t.Errorf("AccountID = %v, want %v", creds.AccountID, tt.wantAcctID)
			}
		})
	}
}

func TestDNSModule_parseRecord_AllFields(t *testing.T) {
	module := NewDNSModule()

	recordMap := map[string]interface{}{
		"type":     "SRV",
		"name":     "_http._tcp",
		"value":    "server.example.com",
		"ttl":      300,
		"priority": 10,
		"weight":   5,
		"port":     80,
		"proxied":  true,
	}

	record, err := module.parseRecord(recordMap, 0)
	if err != nil {
		t.Fatalf("parseRecord() error = %v", err)
	}

	if record.Type != dns.RecordTypeSRV {
		t.Errorf("Type = %v, want SRV", record.Type)
	}
	if record.Name != "_http._tcp" {
		t.Errorf("Name = %v, want _http._tcp", record.Name)
	}
	if record.TTL != 300 {
		t.Errorf("TTL = %v, want 300", record.TTL)
	}
	if record.Priority != 10 {
		t.Errorf("Priority = %v, want 10", record.Priority)
	}
	if record.Weight != 5 {
		t.Errorf("Weight = %v, want 5", record.Weight)
	}
	if record.Port != 80 {
		t.Errorf("Port = %v, want 80", record.Port)
	}
	if record.Proxied == nil || !*record.Proxied {
		t.Error("Proxied = false, want true")
	}
}

func TestDNSModule_IgnoreTTL(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		records: []dns.Record{
			{Type: dns.RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
		},
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":       "example.com",
			"provider":   "test",
			"ignore_ttl": true,
			"records": []interface{}{
				map[string]interface{}{
					"type":  "A",
					"name":  "www",
					"value": "192.0.2.1",
					"ttl":   600, // Different TTL
				},
			},
		},
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !result.Matches {
		t.Error("Check() should match when ignore_ttl=true")
	}
}

func TestDNSModule_ProviderError(t *testing.T) {
	ctx := context.Background()
	provider := &testDNSProvider{
		getErr: fmt.Errorf("API error"),
	}
	module := setupTestModule(provider)

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "test",
			"records":  []interface{}{},
		},
	}

	_, err := module.Check(ctx, decl)
	if err == nil {
		t.Error("Check() should return error from provider")
	}
}

func TestDNSModule_UnknownProvider(t *testing.T) {
	module := NewDNSModule()

	decl := &StateDeclaration{
		ID:    "example.com",
		State: "present",
		Parameters: map[string]interface{}{
			"zone":     "example.com",
			"provider": "unknown",
			"records":  []interface{}{},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply() unexpected error = %v", err)
	}

	if result.Success {
		t.Error("Apply() Success = true, want false for unknown provider")
	}
	if result.Error == nil {
		t.Error("Apply() Error should be set for unknown provider")
	}
}
