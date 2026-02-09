package dns

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
)

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	factory := func(creds ResolvedCredentials) (Provider, error) {
		return NewMockProvider(), nil
	}

	caps := ProviderCapabilities{
		SupportedRecordTypes: []RecordType{RecordTypeA, RecordTypeAAAA},
	}

	// Register should succeed
	err := registry.Register("test", factory, caps)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Register same name again should fail
	err = registry.Register("test", factory, caps)
	if err == nil {
		t.Error("Register() should fail for duplicate provider")
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()

	factory := func(creds ResolvedCredentials) (Provider, error) {
		return NewMockProvider(), nil
	}

	caps := ProviderCapabilities{}
	registry.Register("test", factory, caps)

	// Get registered provider
	got, exists := registry.Get("test")
	if !exists {
		t.Error("Get() should find registered provider")
	}
	if got == nil {
		t.Error("Get() should return factory")
	}

	// Get non-existent provider
	_, exists = registry.Get("nonexistent")
	if exists {
		t.Error("Get() should not find non-existent provider")
	}
}

func TestRegistry_GetCapabilities(t *testing.T) {
	registry := NewRegistry()

	factory := func(creds ResolvedCredentials) (Provider, error) {
		return NewMockProvider(), nil
	}

	caps := ProviderCapabilities{
		SupportedRecordTypes: []RecordType{RecordTypeA, RecordTypeAAAA},
		SupportsProxied:      true,
		MinTTL:               60,
		MaxTTL:               86400,
	}

	registry.Register("test", factory, caps)

	got, exists := registry.GetCapabilities("test")
	if !exists {
		t.Error("GetCapabilities() should find registered provider")
	}
	if !got.SupportsProxied {
		t.Error("GetCapabilities() should return correct capabilities")
	}
	if got.MinTTL != 60 {
		t.Error("GetCapabilities() should return correct MinTTL")
	}

	_, exists = registry.GetCapabilities("nonexistent")
	if exists {
		t.Error("GetCapabilities() should not find non-existent provider")
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()

	factory := func(creds ResolvedCredentials) (Provider, error) {
		return NewMockProvider(), nil
	}

	caps := ProviderCapabilities{}

	registry.Register("cloudflare", factory, caps)
	registry.Register("route53", factory, caps)
	registry.Register("gcp", factory, caps)

	names := registry.List()
	if len(names) != 3 {
		t.Errorf("List() returned %d names, want 3", len(names))
	}

	// Sort for deterministic comparison
	sort.Strings(names)
	expected := []string{"cloudflare", "gcp", "route53"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("List()[%d] = %v, want %v", i, name, expected[i])
		}
	}
}

func TestRegistry_CreateProvider(t *testing.T) {
	registry := NewRegistry()

	factory := func(creds ResolvedCredentials) (Provider, error) {
		if creds.APIToken == "" {
			return nil, fmt.Errorf("API token required")
		}
		return NewMockProvider(), nil
	}

	caps := ProviderCapabilities{}
	registry.Register("test", factory, caps)

	// Create with valid credentials
	creds := ResolvedCredentials{APIToken: "token123"}
	provider, err := registry.CreateProvider("test", creds)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	if provider == nil {
		t.Error("CreateProvider() should return provider")
	}

	// Create with invalid credentials
	_, err = registry.CreateProvider("test", ResolvedCredentials{})
	if err == nil {
		t.Error("CreateProvider() should fail with missing credentials")
	}

	// Create with unknown provider
	_, err = registry.CreateProvider("unknown", creds)
	if err == nil {
		t.Error("CreateProvider() should fail for unknown provider")
	}
}

func TestMockProvider_Operations(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{
		{Type: RecordTypeA, Name: "existing", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
	})

	// GetRecords
	records, err := provider.GetRecords(ctx, "example.com")
	if err != nil {
		t.Fatalf("GetRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Errorf("GetRecords() returned %d records, want 1", len(records))
	}

	// CreateRecord
	newRecord := Record{Type: RecordTypeA, Name: "new", Value: "192.0.2.2", TTL: 300}
	created, err := provider.CreateRecord(ctx, "example.com", newRecord)
	if err != nil {
		t.Fatalf("CreateRecord() error = %v", err)
	}
	if created.ID == "" {
		t.Error("CreateRecord() should set ID")
	}
	if provider.CreateCalled != 1 {
		t.Errorf("CreateCalled = %d, want 1", provider.CreateCalled)
	}

	// Verify records after create
	records, _ = provider.GetRecords(ctx, "example.com")
	if len(records) != 2 {
		t.Errorf("After create, GetRecords() returned %d records, want 2", len(records))
	}

	// UpdateRecord - update the existing record
	existingRecords, _ := provider.GetRecords(ctx, "example.com")
	existing := existingRecords[0]
	existing.TTL = 600
	updated, err := provider.UpdateRecord(ctx, "example.com", existing)
	if err != nil {
		t.Fatalf("UpdateRecord() error = %v", err)
	}
	if updated.TTL != 600 {
		t.Error("UpdateRecord() should update record")
	}
	if provider.UpdateCalled != 1 {
		t.Errorf("UpdateCalled = %d, want 1", provider.UpdateCalled)
	}

	// DeleteRecord
	err = provider.DeleteRecord(ctx, "example.com", existing)
	if err != nil {
		t.Fatalf("DeleteRecord() error = %v", err)
	}
	if provider.DeleteCalled != 1 {
		t.Errorf("DeleteCalled = %d, want 1", provider.DeleteCalled)
	}

	// Verify delete
	records, _ = provider.GetRecords(ctx, "example.com")
	if len(records) != 1 { // Only the new record remains
		t.Errorf("After delete, records = %d, want 1", len(records))
	}
}

func TestMockProvider_Errors(t *testing.T) {
	ctx := context.Background()
	testErr := fmt.Errorf("test error")

	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{{ID: "rec1"}})
	provider.GetErr = testErr
	provider.CreateErr = testErr
	provider.UpdateErr = testErr
	provider.DeleteErr = testErr

	_, err := provider.GetRecords(ctx, "example.com")
	if !errors.Is(err, testErr) {
		t.Error("GetRecords() should return configured error")
	}

	_, err = provider.CreateRecord(ctx, "example.com", Record{})
	if !errors.Is(err, testErr) {
		t.Error("CreateRecord() should return configured error")
	}

	_, err = provider.UpdateRecord(ctx, "example.com", Record{ID: "rec1"})
	if !errors.Is(err, testErr) {
		t.Error("UpdateRecord() should return configured error")
	}

	err = provider.DeleteRecord(ctx, "example.com", Record{ID: "rec1"})
	if !errors.Is(err, testErr) {
		t.Error("DeleteRecord() should return configured error")
	}
}

func TestMockProvider_NotFound(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{{ID: "rec1"}})

	// Update non-existent record
	_, err := provider.UpdateRecord(ctx, "example.com", Record{ID: "nonexistent"})
	if err == nil {
		t.Error("UpdateRecord() should fail for non-existent record")
	}

	// Delete non-existent record
	err = provider.DeleteRecord(ctx, "example.com", Record{ID: "nonexistent"})
	if err == nil {
		t.Error("DeleteRecord() should fail for non-existent record")
	}
}

func TestMockProvider_Reset(t *testing.T) {
	provider := NewMockProvider()
	provider.CreateCalled = 5
	provider.GetErr = fmt.Errorf("error")

	provider.Reset()

	if provider.CreateCalled != 0 {
		t.Error("Reset() should clear call counters")
	}
	if provider.GetErr != nil {
		t.Error("Reset() should clear errors")
	}
}

func TestDefaultRegistry(t *testing.T) {
	// The mock provider should be registered by init()
	names := ListProviders()
	found := false
	for _, name := range names {
		if name == "mock" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Default registry should have 'mock' provider registered")
	}
}

func TestResolvedCredentials(t *testing.T) {
	creds := ResolvedCredentials{
		APIKey:    "key123",
		APIToken:  "token456",
		AccountID: "account789",
		Extra: map[string]string{
			"zone_id": "zone123",
		},
	}

	if creds.APIKey != "key123" {
		t.Error("APIKey not set correctly")
	}
	if creds.APIToken != "token456" {
		t.Error("APIToken not set correctly")
	}
	if creds.AccountID != "account789" {
		t.Error("AccountID not set correctly")
	}
	if creds.Extra["zone_id"] != "zone123" {
		t.Error("Extra not set correctly")
	}
}
