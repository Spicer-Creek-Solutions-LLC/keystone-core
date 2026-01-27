package dns

import (
	"fmt"
	"testing"
)

func TestRecordType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		rt       RecordType
		expected bool
	}{
		{"A record", RecordTypeA, true},
		{"AAAA record", RecordTypeAAAA, true},
		{"CNAME record", RecordTypeCNAME, true},
		{"TXT record", RecordTypeTXT, true},
		{"MX record", RecordTypeMX, true},
		{"SRV record", RecordTypeSRV, true},
		{"CAA record", RecordTypeCAA, true},
		{"NS record", RecordTypeNS, true},
		{"ALIAS record", RecordTypeALIAS, true},
		{"PTR record", RecordTypePTR, true},
		{"invalid type", RecordType("INVALID"), false},
		{"empty type", RecordType(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rt.IsValid(); got != tt.expected {
				t.Errorf("RecordType.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAllRecordTypes(t *testing.T) {
	types := AllRecordTypes()
	if len(types) != 10 {
		t.Errorf("AllRecordTypes() returned %d types, want 10", len(types))
	}

	// Verify all returned types are valid
	for _, rt := range types {
		if !rt.IsValid() {
			t.Errorf("AllRecordTypes() returned invalid type: %s", rt)
		}
	}
}

func TestRecord_Key(t *testing.T) {
	tests := []struct {
		name     string
		record   Record
		expected string
	}{
		{
			name:     "A record",
			record:   Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1"},
			expected: "A:www:192.0.2.1",
		},
		{
			name:     "CNAME record",
			record:   Record{Type: RecordTypeCNAME, Name: "api", Value: "api.example.com."},
			expected: "CNAME:api:api.example.com.",
		},
		{
			name:     "MX record",
			record:   Record{Type: RecordTypeMX, Name: "@", Value: "mail.example.com.", Priority: 10},
			expected: "MX:@:mail.example.com.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.record.Key(); got != tt.expected {
				t.Errorf("Record.Key() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRecord_Validate(t *testing.T) {
	tests := []struct {
		name    string
		record  Record
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid A record",
			record:  Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
			wantErr: false,
		},
		{
			name:    "valid AAAA record",
			record:  Record{Type: RecordTypeAAAA, Name: "www", Value: "2001:db8::1", TTL: 300},
			wantErr: false,
		},
		{
			name:    "valid CNAME record",
			record:  Record{Type: RecordTypeCNAME, Name: "api", Value: "api.example.com", TTL: 300},
			wantErr: false,
		},
		{
			name:    "valid TXT record",
			record:  Record{Type: RecordTypeTXT, Name: "_dmarc", Value: "v=DMARC1; p=none", TTL: 300},
			wantErr: false,
		},
		{
			name:    "valid MX record",
			record:  Record{Type: RecordTypeMX, Name: "@", Value: "mail.example.com", TTL: 300, Priority: 10},
			wantErr: false,
		},
		{
			name:    "valid SRV record",
			record:  Record{Type: RecordTypeSRV, Name: "_http._tcp", Value: "server.example.com", TTL: 300, Priority: 10, Weight: 5, Port: 80},
			wantErr: false,
		},
		{
			name:    "invalid record type",
			record:  Record{Type: RecordType("INVALID"), Name: "www", Value: "192.0.2.1"},
			wantErr: true,
			errMsg:  "invalid record type",
		},
		{
			name:    "missing name",
			record:  Record{Type: RecordTypeA, Name: "", Value: "192.0.2.1"},
			wantErr: true,
			errMsg:  "record name is required",
		},
		{
			name:    "missing value",
			record:  Record{Type: RecordTypeA, Name: "www", Value: ""},
			wantErr: true,
			errMsg:  "record value is required",
		},
		{
			name:    "negative TTL",
			record:  Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: -1},
			wantErr: true,
			errMsg:  "TTL must be non-negative",
		},
		{
			name:    "invalid IPv4 for A record",
			record:  Record{Type: RecordTypeA, Name: "www", Value: "not-an-ip", TTL: 300},
			wantErr: true,
			errMsg:  "invalid IPv4 address",
		},
		{
			name:    "IPv6 for A record",
			record:  Record{Type: RecordTypeA, Name: "www", Value: "2001:db8::1", TTL: 300},
			wantErr: true,
			errMsg:  "invalid IPv4 address",
		},
		{
			name:    "invalid IPv6 for AAAA record",
			record:  Record{Type: RecordTypeAAAA, Name: "www", Value: "not-an-ip", TTL: 300},
			wantErr: true,
			errMsg:  "invalid IPv6 address",
		},
		{
			name:    "IPv4 for AAAA record",
			record:  Record{Type: RecordTypeAAAA, Name: "www", Value: "192.0.2.1", TTL: 300},
			wantErr: true,
			errMsg:  "invalid IPv6 address",
		},
		{
			name:    "MX priority too high",
			record:  Record{Type: RecordTypeMX, Name: "@", Value: "mail.example.com", Priority: 70000},
			wantErr: true,
			errMsg:  "MX priority must be between 0 and 65535",
		},
		{
			name:    "SRV priority negative",
			record:  Record{Type: RecordTypeSRV, Name: "_http._tcp", Value: "server.example.com", Priority: -1},
			wantErr: true,
			errMsg:  "SRV priority must be between 0 and 65535",
		},
		{
			name:    "SRV weight too high",
			record:  Record{Type: RecordTypeSRV, Name: "_http._tcp", Value: "server.example.com", Weight: 70000},
			wantErr: true,
			errMsg:  "SRV weight must be between 0 and 65535",
		},
		{
			name:    "SRV port too high",
			record:  Record{Type: RecordTypeSRV, Name: "_http._tcp", Value: "server.example.com", Port: 70000},
			wantErr: true,
			errMsg:  "SRV port must be between 0 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.record.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Record.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !contains(err.Error(), tt.errMsg) {
					t.Errorf("Record.Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestRecord_Normalize(t *testing.T) {
	tests := []struct {
		name     string
		record   Record
		zone     string
		expected Record
	}{
		{
			name:   "normalize apex record @",
			record: Record{Type: RecordTypeA, Name: "@", Value: "192.0.2.1"},
			zone:   "example.com",
			expected: Record{
				Type: RecordTypeA, Name: "@", Value: "192.0.2.1", TTL: 300,
			},
		},
		{
			name:   "normalize apex record empty",
			record: Record{Type: RecordTypeA, Name: "", Value: "192.0.2.1"},
			zone:   "example.com",
			expected: Record{
				Type: RecordTypeA, Name: "@", Value: "192.0.2.1", TTL: 300,
			},
		},
		{
			name:   "normalize apex record zone name",
			record: Record{Type: RecordTypeA, Name: "example.com", Value: "192.0.2.1"},
			zone:   "example.com",
			expected: Record{
				Type: RecordTypeA, Name: "@", Value: "192.0.2.1", TTL: 300,
			},
		},
		{
			name:   "normalize subdomain with zone suffix",
			record: Record{Type: RecordTypeA, Name: "www.example.com", Value: "192.0.2.1"},
			zone:   "example.com",
			expected: Record{
				Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300,
			},
		},
		{
			name:   "normalize subdomain without zone suffix",
			record: Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1"},
			zone:   "example.com",
			expected: Record{
				Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300,
			},
		},
		{
			name:   "normalize CNAME value to FQDN",
			record: Record{Type: RecordTypeCNAME, Name: "api", Value: "api.internal.example.com"},
			zone:   "example.com",
			expected: Record{
				Type: RecordTypeCNAME, Name: "api", Value: "api.internal.example.com.", TTL: 300,
			},
		},
		{
			name:   "normalize MX value to FQDN",
			record: Record{Type: RecordTypeMX, Name: "@", Value: "mail.example.com", Priority: 10},
			zone:   "example.com",
			expected: Record{
				Type: RecordTypeMX, Name: "@", Value: "mail.example.com.", TTL: 300, Priority: 10,
			},
		},
		{
			name:   "preserve existing TTL",
			record: Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 600},
			zone:   "example.com",
			expected: Record{
				Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 600,
			},
		},
		{
			name:   "normalize to lowercase",
			record: Record{Type: RecordTypeA, Name: "WWW.EXAMPLE.COM", Value: "192.0.2.1"},
			zone:   "example.com",
			expected: Record{
				Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300,
			},
		},
		{
			name:   "strip trailing dot from name",
			record: Record{Type: RecordTypeA, Name: "www.example.com.", Value: "192.0.2.1"},
			zone:   "example.com",
			expected: Record{
				Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.record.Normalize(tt.zone)
			if got.Type != tt.expected.Type {
				t.Errorf("Normalize().Type = %v, want %v", got.Type, tt.expected.Type)
			}
			if got.Name != tt.expected.Name {
				t.Errorf("Normalize().Name = %v, want %v", got.Name, tt.expected.Name)
			}
			if got.Value != tt.expected.Value {
				t.Errorf("Normalize().Value = %v, want %v", got.Value, tt.expected.Value)
			}
			if got.TTL != tt.expected.TTL {
				t.Errorf("Normalize().TTL = %v, want %v", got.TTL, tt.expected.TTL)
			}
			if got.Priority != tt.expected.Priority {
				t.Errorf("Normalize().Priority = %v, want %v", got.Priority, tt.expected.Priority)
			}
		})
	}
}

func TestRecord_Equal(t *testing.T) {
	tests := []struct {
		name     string
		r1       *Record
		r2       *Record
		expected bool
	}{
		{
			name:     "equal records",
			r1:       &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
			r2:       &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
			expected: true,
		},
		{
			name:     "equal records with ID difference",
			r1:       &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "id1"},
			r2:       &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "id2"},
			expected: true,
		},
		{
			name:     "different type",
			r1:       &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
			r2:       &Record{Type: RecordTypeAAAA, Name: "www", Value: "192.0.2.1", TTL: 300},
			expected: false,
		},
		{
			name:     "different name",
			r1:       &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
			r2:       &Record{Type: RecordTypeA, Name: "api", Value: "192.0.2.1", TTL: 300},
			expected: false,
		},
		{
			name:     "different value",
			r1:       &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
			r2:       &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.2", TTL: 300},
			expected: false,
		},
		{
			name:     "different TTL",
			r1:       &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
			r2:       &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 600},
			expected: false,
		},
		{
			name:     "different priority",
			r1:       &Record{Type: RecordTypeMX, Name: "@", Value: "mail.example.com", TTL: 300, Priority: 10},
			r2:       &Record{Type: RecordTypeMX, Name: "@", Value: "mail.example.com", TTL: 300, Priority: 20},
			expected: false,
		},
		{
			name:     "different weight",
			r1:       &Record{Type: RecordTypeSRV, Name: "_http._tcp", Value: "server.example.com", TTL: 300, Weight: 5},
			r2:       &Record{Type: RecordTypeSRV, Name: "_http._tcp", Value: "server.example.com", TTL: 300, Weight: 10},
			expected: false,
		},
		{
			name:     "different port",
			r1:       &Record{Type: RecordTypeSRV, Name: "_http._tcp", Value: "server.example.com", TTL: 300, Port: 80},
			r2:       &Record{Type: RecordTypeSRV, Name: "_http._tcp", Value: "server.example.com", TTL: 300, Port: 443},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r1.Equal(tt.r2); got != tt.expected {
				t.Errorf("Record.Equal() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestZone_Validate(t *testing.T) {
	tests := []struct {
		name    string
		zone    Zone
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid zone",
			zone: Zone{
				Name:     "example.com",
				Provider: "cloudflare",
				Records: []Record{
					{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
				},
			},
			wantErr: false,
		},
		{
			name: "valid zone with no records",
			zone: Zone{
				Name:     "example.com",
				Provider: "route53",
				Records:  []Record{},
			},
			wantErr: false,
		},
		{
			name: "missing zone name",
			zone: Zone{
				Name:     "",
				Provider: "cloudflare",
			},
			wantErr: true,
			errMsg:  "zone name is required",
		},
		{
			name: "invalid zone name",
			zone: Zone{
				Name:     "invalid",
				Provider: "cloudflare",
			},
			wantErr: true,
			errMsg:  "invalid zone name",
		},
		{
			name: "missing provider",
			zone: Zone{
				Name:     "example.com",
				Provider: "",
			},
			wantErr: true,
			errMsg:  "provider is required",
		},
		{
			name: "invalid record in zone",
			zone: Zone{
				Name:     "example.com",
				Provider: "cloudflare",
				Records: []Record{
					{Type: RecordTypeA, Name: "", Value: "192.0.2.1"},
				},
			},
			wantErr: true,
			errMsg:  "record[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.zone.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Zone.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !contains(err.Error(), tt.errMsg) {
					t.Errorf("Zone.Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestZone_NormalizeRecords(t *testing.T) {
	zone := Zone{
		Name:     "example.com",
		Provider: "cloudflare",
		Records: []Record{
			{Type: RecordTypeA, Name: "www.example.com", Value: "192.0.2.1"},
			{Type: RecordTypeCNAME, Name: "api", Value: "api.internal.example.com"},
		},
	}

	normalized := zone.NormalizeRecords()

	if len(normalized) != 2 {
		t.Fatalf("NormalizeRecords() returned %d records, want 2", len(normalized))
	}

	if normalized[0].Name != "www" {
		t.Errorf("normalized[0].Name = %v, want www", normalized[0].Name)
	}

	if normalized[1].Value != "api.internal.example.com." {
		t.Errorf("normalized[1].Value = %v, want api.internal.example.com.", normalized[1].Value)
	}
}

func TestIsValidZone(t *testing.T) {
	tests := []struct {
		name     string
		zone     string
		expected bool
	}{
		{"valid domain", "example.com", true},
		{"valid subdomain", "sub.example.com", true},
		{"valid with trailing dot", "example.com.", true},
		{"valid long tld", "example.technology", true},
		{"invalid - no tld", "example", false},
		{"invalid - empty", "", false},
		{"invalid - just tld", "com", false},
		{"invalid - starts with dash", "-example.com", false},
		{"invalid - ends with dash", "example-.com", false},
		{"valid - with numbers", "example123.com", true},
		{"valid - with dash", "my-example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidZone(tt.zone); got != tt.expected {
				t.Errorf("IsValidZone(%q) = %v, want %v", tt.zone, got, tt.expected)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		zone     string
		expected string
	}{
		{"apex @", "@", "example.com", "@"},
		{"apex empty", "", "example.com", "@"},
		{"apex zone name", "example.com", "example.com", "@"},
		{"subdomain", "www", "example.com", "www"},
		{"subdomain with zone suffix", "www.example.com", "example.com", "www"},
		{"trailing dot", "www.example.com.", "example.com", "www"},
		{"uppercase", "WWW", "example.com", "www"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeName(tt.input, tt.zone); got != tt.expected {
				t.Errorf("NormalizeName(%q, %q) = %v, want %v", tt.input, tt.zone, got, tt.expected)
			}
		})
	}
}

func TestNormalizeFQDN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"without trailing dot", "example.com", "example.com."},
		{"with trailing dot", "example.com.", "example.com."},
		{"uppercase", "EXAMPLE.COM", "example.com."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeFQDN(tt.input); got != tt.expected {
				t.Errorf("NormalizeFQDN(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCredentials_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		creds    Credentials
		expected bool
	}{
		{
			name:     "empty credentials",
			creds:    Credentials{},
			expected: true,
		},
		{
			name:     "with secret ref",
			creds:    Credentials{SecretRef: "secret://dns/cloudflare"},
			expected: false,
		},
		{
			name:     "with API key",
			creds:    Credentials{APIKey: "key123"},
			expected: false,
		},
		{
			name:     "with API token",
			creds:    Credentials{APIToken: "token123"},
			expected: false,
		},
		{
			name:     "with extra",
			creds:    Credentials{Extra: map[string]string{"account_id": "123"}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.creds.IsEmpty(); got != tt.expected {
				t.Errorf("Credentials.IsEmpty() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestProviderCapabilities_SupportsRecordType(t *testing.T) {
	caps := ProviderCapabilities{
		SupportedRecordTypes: []RecordType{RecordTypeA, RecordTypeAAAA, RecordTypeCNAME},
	}

	tests := []struct {
		name     string
		rt       RecordType
		expected bool
	}{
		{"supported A", RecordTypeA, true},
		{"supported AAAA", RecordTypeAAAA, true},
		{"supported CNAME", RecordTypeCNAME, true},
		{"unsupported MX", RecordTypeMX, false},
		{"unsupported SRV", RecordTypeSRV, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caps.SupportsRecordType(tt.rt); got != tt.expected {
				t.Errorf("ProviderCapabilities.SupportsRecordType(%v) = %v, want %v", tt.rt, got, tt.expected)
			}
		})
	}
}

func TestSyncResult(t *testing.T) {
	result := SyncResult{
		Zone:      "example.com",
		Created:   2,
		Updated:   1,
		Deleted:   1,
		Unchanged: 5,
	}

	if result.TotalChanges() != 4 {
		t.Errorf("TotalChanges() = %v, want 4", result.TotalChanges())
	}

	if result.HasErrors() {
		t.Error("HasErrors() = true, want false")
	}

	result.Errors = append(result.Errors, fmt.Errorf("test error"))
	if !result.HasErrors() {
		t.Error("HasErrors() = false after adding error, want true")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
