package providers

import (
	"context"
	"testing"
	"time"

	"github.com/libdns/libdns"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// mockLibdnsProvider is a mock implementation of LibdnsProvider for testing.
type mockLibdnsProvider struct {
	records []libdns.Record
	getErr  error
}

func (m *mockLibdnsProvider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.records, nil
}

func (m *mockLibdnsProvider) AppendRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	for i := range recs {
		m.records = append(m.records, recs[i])
	}
	return recs, nil
}

func (m *mockLibdnsProvider) SetRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	// Simple implementation: replace matching records
	for _, newRec := range recs {
		rr := newRec.RR()
		found := false
		for i, existingRec := range m.records {
			existing := existingRec.RR()
			if existing.Name == rr.Name && existing.Type == rr.Type {
				m.records[i] = newRec
				found = true
				break
			}
		}
		if !found {
			m.records = append(m.records, newRec)
		}
	}
	return recs, nil
}

func (m *mockLibdnsProvider) DeleteRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	deleted := make([]libdns.Record, 0)
	for _, delRec := range recs {
		rr := delRec.RR()
		for i, existingRec := range m.records {
			existing := existingRec.RR()
			if existing.Name == rr.Name && existing.Type == rr.Type {
				deleted = append(deleted, m.records[i])
				m.records = append(m.records[:i], m.records[i+1:]...)
				break
			}
		}
	}
	return deleted, nil
}

func TestLibdnsAdapter_GetRecords(t *testing.T) {
	mock := &mockLibdnsProvider{
		records: []libdns.Record{
			libdns.RR{Name: "www", Type: "A", TTL: 300 * time.Second, Data: "192.0.2.1"},
			libdns.RR{Name: "mail", Type: "MX", TTL: 3600 * time.Second, Data: "10 mx.example.com."},
		},
	}

	adapter := NewLibdnsAdapter(mock, dns.ProviderCapabilities{})
	records, err := adapter.GetRecords(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("GetRecords() error = %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("GetRecords() returned %d records, want 2", len(records))
	}

	// Check A record
	if records[0].Type != dns.RecordTypeA {
		t.Errorf("records[0].Type = %s, want A", records[0].Type)
	}
	if records[0].Name != "www" {
		t.Errorf("records[0].Name = %s, want www", records[0].Name)
	}
	if records[0].Value != "192.0.2.1" {
		t.Errorf("records[0].Value = %s, want 192.0.2.1", records[0].Value)
	}

	// Check MX record (parsed)
	if records[1].Type != dns.RecordTypeMX {
		t.Errorf("records[1].Type = %s, want MX", records[1].Type)
	}
	if records[1].Priority != 10 {
		t.Errorf("records[1].Priority = %d, want 10", records[1].Priority)
	}
	if records[1].Value != "mx.example.com." {
		t.Errorf("records[1].Value = %s, want mx.example.com.", records[1].Value)
	}
}

func TestLibdnsAdapter_CreateRecord(t *testing.T) {
	mock := &mockLibdnsProvider{
		records: []libdns.Record{},
	}

	adapter := NewLibdnsAdapter(mock, dns.ProviderCapabilities{})

	record := dns.Record{
		Type:  dns.RecordTypeA,
		Name:  "www",
		Value: "192.0.2.1",
		TTL:   300,
	}

	created, err := adapter.CreateRecord(context.Background(), "example.com", record)
	if err != nil {
		t.Fatalf("CreateRecord() error = %v", err)
	}

	if created.Name != "www" {
		t.Errorf("created.Name = %s, want www", created.Name)
	}
	if created.Value != "192.0.2.1" {
		t.Errorf("created.Value = %s, want 192.0.2.1", created.Value)
	}

	// Verify it was added to mock
	if len(mock.records) != 1 {
		t.Errorf("mock.records has %d records, want 1", len(mock.records))
	}
}

func TestLibdnsAdapter_UpdateRecord(t *testing.T) {
	mock := &mockLibdnsProvider{
		records: []libdns.Record{
			libdns.RR{Name: "www", Type: "A", TTL: 300 * time.Second, Data: "192.0.2.1"},
		},
	}

	adapter := NewLibdnsAdapter(mock, dns.ProviderCapabilities{})

	record := dns.Record{
		Type:  dns.RecordTypeA,
		Name:  "www",
		Value: "192.0.2.2",
		TTL:   600,
	}

	updated, err := adapter.UpdateRecord(context.Background(), "example.com", record)
	if err != nil {
		t.Fatalf("UpdateRecord() error = %v", err)
	}

	if updated.Value != "192.0.2.2" {
		t.Errorf("updated.Value = %s, want 192.0.2.2", updated.Value)
	}
	if updated.TTL != 600 {
		t.Errorf("updated.TTL = %d, want 600", updated.TTL)
	}
}

func TestLibdnsAdapter_DeleteRecord(t *testing.T) {
	mock := &mockLibdnsProvider{
		records: []libdns.Record{
			libdns.RR{Name: "www", Type: "A", TTL: 300 * time.Second, Data: "192.0.2.1"},
			libdns.RR{Name: "api", Type: "A", TTL: 300 * time.Second, Data: "192.0.2.2"},
		},
	}

	adapter := NewLibdnsAdapter(mock, dns.ProviderCapabilities{})

	record := dns.Record{
		Type:  dns.RecordTypeA,
		Name:  "www",
		Value: "192.0.2.1",
	}

	err := adapter.DeleteRecord(context.Background(), "example.com", record)
	if err != nil {
		t.Fatalf("DeleteRecord() error = %v", err)
	}

	// Verify it was deleted
	if len(mock.records) != 1 {
		t.Errorf("mock.records has %d records, want 1", len(mock.records))
	}
}

func TestLibdnsAdapter_SRVRecord(t *testing.T) {
	mock := &mockLibdnsProvider{
		records: []libdns.Record{
			libdns.RR{Name: "_sip._tcp", Type: "SRV", TTL: 300 * time.Second, Data: "10 60 5060 sip.example.com."},
		},
	}

	adapter := NewLibdnsAdapter(mock, dns.ProviderCapabilities{})
	records, err := adapter.GetRecords(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("GetRecords() error = %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("GetRecords() returned %d records, want 1", len(records))
	}

	srv := records[0]
	if srv.Type != dns.RecordTypeSRV {
		t.Errorf("Type = %s, want SRV", srv.Type)
	}
	if srv.Priority != 10 {
		t.Errorf("Priority = %d, want 10", srv.Priority)
	}
	if srv.Weight != 60 {
		t.Errorf("Weight = %d, want 60", srv.Weight)
	}
	if srv.Port != 5060 {
		t.Errorf("Port = %d, want 5060", srv.Port)
	}
	if srv.Value != "sip.example.com." {
		t.Errorf("Value = %s, want sip.example.com.", srv.Value)
	}
}

func TestToLibdnsRecord(t *testing.T) {
	tests := []struct {
		name     string
		record   dns.Record
		wantData string
	}{
		{
			name: "A record",
			record: dns.Record{
				Type:  dns.RecordTypeA,
				Name:  "www",
				Value: "192.0.2.1",
				TTL:   300,
			},
			wantData: "192.0.2.1",
		},
		{
			name: "MX record",
			record: dns.Record{
				Type:     dns.RecordTypeMX,
				Name:     "@",
				Value:    "mx.example.com.",
				Priority: 10,
				TTL:      3600,
			},
			wantData: "10 mx.example.com.",
		},
		{
			name: "SRV record",
			record: dns.Record{
				Type:     dns.RecordTypeSRV,
				Name:     "_sip._tcp",
				Value:    "sip.example.com.",
				Priority: 10,
				Weight:   60,
				Port:     5060,
				TTL:      300,
			},
			wantData: "10 60 5060 sip.example.com.",
		},
		{
			name: "TXT record",
			record: dns.Record{
				Type:  dns.RecordTypeTXT,
				Name:  "@",
				Value: "v=spf1 include:_spf.google.com ~all",
				TTL:   3600,
			},
			wantData: "v=spf1 include:_spf.google.com ~all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := toLibdnsRecord(tt.record)
			if rr.Data != tt.wantData {
				t.Errorf("toLibdnsRecord().Data = %s, want %s", rr.Data, tt.wantData)
			}
			if rr.Name != tt.record.Name {
				t.Errorf("toLibdnsRecord().Name = %s, want %s", rr.Name, tt.record.Name)
			}
			if rr.Type != string(tt.record.Type) {
				t.Errorf("toLibdnsRecord().Type = %s, want %s", rr.Type, tt.record.Type)
			}
		})
	}
}

func TestNormalizeZone(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"example.com", "example.com."},
		{"example.com.", "example.com."},
		{"sub.example.com", "sub.example.com."},
		{"sub.example.com.", "sub.example.com."},
	}

	for _, tt := range tests {
		got := normalizeZone(tt.input)
		if got != tt.want {
			t.Errorf("normalizeZone(%s) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestProviderRegistration(t *testing.T) {
	// Verify providers are registered
	providers := dns.ListProviders()

	expectedProviders := []string{"cloudflare", "route53", "gcp", "googleclouddns", "azure", "digitalocean", "hetzner", "dnsmadeeasy", "mock"}
	for _, expected := range expectedProviders {
		found := false
		for _, p := range providers {
			if p == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Provider %s not registered", expected)
		}
	}
}

func TestProviderCapabilities(t *testing.T) {
	// Test Cloudflare capabilities
	caps, exists := dns.DefaultRegistry.GetCapabilities("cloudflare")
	if !exists {
		t.Fatal("Cloudflare provider not registered")
	}
	if !caps.SupportsProxied {
		t.Error("Cloudflare should support proxied records")
	}
	if caps.MinTTL != 60 {
		t.Errorf("Cloudflare MinTTL = %d, want 60", caps.MinTTL)
	}

	// Test Route53 capabilities
	caps, exists = dns.DefaultRegistry.GetCapabilities("route53")
	if !exists {
		t.Fatal("Route53 provider not registered")
	}
	if !caps.SupportsALIAS {
		t.Error("Route53 should support ALIAS records")
	}
	if caps.SupportsProxied {
		t.Error("Route53 should not support proxied records")
	}

	// Test Azure capabilities
	caps, exists = dns.DefaultRegistry.GetCapabilities("azure")
	if !exists {
		t.Fatal("Azure provider not registered")
	}
	if caps.MinTTL != 1 {
		t.Errorf("Azure MinTTL = %d, want 1", caps.MinTTL)
	}
}

func TestCloudflareCredentialValidation(t *testing.T) {
	// Test missing credentials
	_, err := NewCloudflareProvider(dns.ResolvedCredentials{})
	if err == nil {
		t.Error("NewCloudflareProvider() should fail without credentials")
	}

	// Test with API token
	_, err = NewCloudflareProvider(dns.ResolvedCredentials{
		APIToken: "test-token",
	})
	if err != nil {
		t.Errorf("NewCloudflareProvider() with valid credentials error = %v", err)
	}
}

func TestRoute53CredentialValidation(t *testing.T) {
	// Test missing credentials
	_, err := NewRoute53Provider(dns.ResolvedCredentials{})
	if err == nil {
		t.Error("NewRoute53Provider() should fail without credentials")
	}

	// Test with credentials
	_, err = NewRoute53Provider(dns.ResolvedCredentials{
		Extra: map[string]string{
			"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
			"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
	})
	if err != nil {
		t.Errorf("NewRoute53Provider() with valid credentials error = %v", err)
	}
}

func TestAzureCredentialValidation(t *testing.T) {
	// Test missing credentials
	_, err := NewAzureDNSProvider(dns.ResolvedCredentials{})
	if err == nil {
		t.Error("NewAzureDNSProvider() should fail without credentials")
	}

	// Test with partial credentials
	_, err = NewAzureDNSProvider(dns.ResolvedCredentials{
		Extra: map[string]string{
			"subscription_id": "sub-123",
		},
	})
	if err == nil {
		t.Error("NewAzureDNSProvider() should fail with incomplete credentials")
	}

	// Test with complete credentials
	_, err = NewAzureDNSProvider(dns.ResolvedCredentials{
		Extra: map[string]string{
			"subscription_id": "sub-123",
			"resource_group":  "rg-dns",
			"tenant_id":       "tenant-123",
			"client_id":       "client-123",
			"client_secret":   "secret",
		},
	})
	if err != nil {
		t.Errorf("NewAzureDNSProvider() with valid credentials error = %v", err)
	}
}

func TestGoogleCloudDNSCredentialValidation(t *testing.T) {
	// Test missing credentials
	_, err := NewGoogleCloudDNSProvider(dns.ResolvedCredentials{})
	if err == nil {
		t.Error("NewGoogleCloudDNSProvider() should fail without credentials")
	}

	// Test with project_id
	_, err = NewGoogleCloudDNSProvider(dns.ResolvedCredentials{
		Extra: map[string]string{
			"project_id": "my-project",
		},
	})
	if err != nil {
		t.Errorf("NewGoogleCloudDNSProvider() with valid credentials error = %v", err)
	}

	// Test with AccountID
	_, err = NewGoogleCloudDNSProvider(dns.ResolvedCredentials{
		AccountID: "my-project",
	})
	if err != nil {
		t.Errorf("NewGoogleCloudDNSProvider() with AccountID error = %v", err)
	}
}

func TestDigitalOceanCredentialValidation(t *testing.T) {
	// Test missing credentials
	_, err := NewDigitalOceanProvider(dns.ResolvedCredentials{})
	if err == nil {
		t.Error("NewDigitalOceanProvider() should fail without credentials")
	}

	// Test with API token
	_, err = NewDigitalOceanProvider(dns.ResolvedCredentials{
		APIToken: "test-token",
	})
	if err != nil {
		t.Errorf("NewDigitalOceanProvider() with valid credentials error = %v", err)
	}
}

func TestHetznerCredentialValidation(t *testing.T) {
	// Test missing credentials
	_, err := NewHetznerProvider(dns.ResolvedCredentials{})
	if err == nil {
		t.Error("NewHetznerProvider() should fail without credentials")
	}

	// Test with API token
	_, err = NewHetznerProvider(dns.ResolvedCredentials{
		APIToken: "test-token",
	})
	if err != nil {
		t.Errorf("NewHetznerProvider() with valid credentials error = %v", err)
	}
}

func TestDNSMadeEasyCredentialValidation(t *testing.T) {
	// Test missing credentials
	_, err := NewDNSMadeEasyProvider(dns.ResolvedCredentials{})
	if err == nil {
		t.Error("NewDNSMadeEasyProvider() should fail without credentials")
	}

	// Test with only api_key
	_, err = NewDNSMadeEasyProvider(dns.ResolvedCredentials{
		APIKey: "test-api-key",
	})
	if err == nil {
		t.Error("NewDNSMadeEasyProvider() should fail without secret_key")
	}

	// Test with complete credentials
	_, err = NewDNSMadeEasyProvider(dns.ResolvedCredentials{
		APIKey:   "test-api-key",
		APIToken: "test-secret-key",
	})
	if err != nil {
		t.Errorf("NewDNSMadeEasyProvider() with valid credentials error = %v", err)
	}

	// Test with Extra map credentials
	_, err = NewDNSMadeEasyProvider(dns.ResolvedCredentials{
		Extra: map[string]string{
			"api_key":    "test-api-key",
			"secret_key": "test-secret-key",
		},
	})
	if err != nil {
		t.Errorf("NewDNSMadeEasyProvider() with Extra credentials error = %v", err)
	}
}

func TestDNSMadeEasyCapabilities(t *testing.T) {
	caps, exists := dns.DefaultRegistry.GetCapabilities("dnsmadeeasy")
	if !exists {
		t.Fatal("DNSMadeEasy provider not registered")
	}
	if caps.MinTTL != 30 {
		t.Errorf("DNSMadeEasy MinTTL = %d, want 30", caps.MinTTL)
	}
	if caps.MaxTTL != 604800 {
		t.Errorf("DNSMadeEasy MaxTTL = %d, want 604800", caps.MaxTTL)
	}
	if caps.SupportsProxied {
		t.Error("DNSMadeEasy should not support proxied records")
	}
}
