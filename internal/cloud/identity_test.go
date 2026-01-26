package cloud

import (
	"testing"
	"time"
)

func TestIdentityInfo_Types(t *testing.T) {
	info := &IdentityInfo{
		Provider:     ProviderAWS,
		IdentityType: "iam-role",
		IdentityID:   "arn:aws:iam::123456789012:instance-profile/my-role",
		IdentityName: "my-role",
		AccountID:    "123456789012",
		Scopes:       []string{"full-access"},
		AccessToken:  "token-value",
		TokenExpiry:  time.Now().Add(time.Hour),
		Metadata: map[string]string{
			"access_key_id": "AKIAIOSFODNN7EXAMPLE",
		},
	}

	if info.Provider != ProviderAWS {
		t.Errorf("Provider = %v, want %v", info.Provider, ProviderAWS)
	}
	if info.IdentityType != "iam-role" {
		t.Errorf("IdentityType = %q, want %q", info.IdentityType, "iam-role")
	}
	if len(info.Scopes) != 1 {
		t.Errorf("Scopes length = %d, want 1", len(info.Scopes))
	}
}

func TestNewAWSIdentityProvider(t *testing.T) {
	provider := NewAWSIdentityProvider(5 * time.Second)
	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
	if provider.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}
	if provider.httpClient.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want %v", provider.httpClient.Timeout, 5*time.Second)
	}
}

func TestNewGCPIdentityProvider(t *testing.T) {
	provider := NewGCPIdentityProvider(5 * time.Second)
	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
	if provider.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}
}

func TestNewAzureIdentityProvider(t *testing.T) {
	provider := NewAzureIdentityProvider(5 * time.Second)
	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
	if provider.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}
}

func TestGetIdentityProvider(t *testing.T) {
	tests := []struct {
		provider Provider
		wantNil  bool
		wantType string
	}{
		{ProviderAWS, false, "*cloud.AWSIdentityProvider"},
		{ProviderGCP, false, "*cloud.GCPIdentityProvider"},
		{ProviderAzure, false, "*cloud.AzureIdentityProvider"},
		{ProviderUnknown, true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.provider.String(), func(t *testing.T) {
			provider := GetIdentityProvider(tt.provider, 5*time.Second)
			if tt.wantNil && provider != nil {
				t.Error("Expected nil provider")
			}
			if !tt.wantNil && provider == nil {
				t.Error("Expected non-nil provider")
			}
		})
	}
}

func TestAttestationInfo_Types(t *testing.T) {
	info := &AttestationInfo{
		Provider:     ProviderAWS,
		Document:     `{"instanceId": "i-1234567890abcdef0"}`,
		DocumentType: "instance-identity-document",
		Signature:    "base64-signature",
		PKCS7:        "base64-pkcs7",
		Nonce:        "test-nonce",
		Timestamp:    time.Now(),
	}

	if info.Provider != ProviderAWS {
		t.Errorf("Provider = %v, want %v", info.Provider, ProviderAWS)
	}
	if info.DocumentType != "instance-identity-document" {
		t.Errorf("DocumentType = %q, want %q", info.DocumentType, "instance-identity-document")
	}
}

func TestNewAWSAttestationProvider(t *testing.T) {
	provider := NewAWSAttestationProvider(5 * time.Second)
	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
}

func TestNewGCPAttestationProvider(t *testing.T) {
	provider := NewGCPAttestationProvider(5 * time.Second)
	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
}

func TestNewAzureAttestationProvider(t *testing.T) {
	provider := NewAzureAttestationProvider(5 * time.Second)
	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
}

func TestGetAttestationProvider(t *testing.T) {
	tests := []struct {
		provider Provider
		wantNil  bool
	}{
		{ProviderAWS, false},
		{ProviderGCP, false},
		{ProviderAzure, false},
		{ProviderUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.provider.String(), func(t *testing.T) {
			provider := GetAttestationProvider(tt.provider, 5*time.Second)
			if tt.wantNil && provider != nil {
				t.Error("Expected nil provider")
			}
			if !tt.wantNil && provider == nil {
				t.Error("Expected non-nil provider")
			}
		})
	}
}

func TestIdentityInfo_GCP(t *testing.T) {
	info := &IdentityInfo{
		Provider:     ProviderGCP,
		IdentityType: "service-account",
		IdentityID:   "my-sa@my-project.iam.gserviceaccount.com",
		IdentityName: "my-sa@my-project.iam.gserviceaccount.com",
		AccountID:    "my-project",
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/compute",
		},
		Metadata: map[string]string{
			"numeric_project_id": "123456789012",
		},
	}

	if info.Provider != ProviderGCP {
		t.Errorf("Provider = %v, want %v", info.Provider, ProviderGCP)
	}
	if len(info.Scopes) != 2 {
		t.Errorf("Scopes length = %d, want 2", len(info.Scopes))
	}
}

func TestIdentityInfo_Azure(t *testing.T) {
	info := &IdentityInfo{
		Provider:     ProviderAzure,
		IdentityType: "managed-identity",
		IdentityID:   "00000000-0000-0000-0000-000000000000",
		AccountID:    "subscription-id",
		AccessToken:  "eyJ0eXAiOiJKV1...",
		TokenExpiry:  time.Now().Add(time.Hour),
		Metadata: map[string]string{
			"resource_group": "my-rg",
			"vm_name":        "my-vm",
		},
	}

	if info.Provider != ProviderAzure {
		t.Errorf("Provider = %v, want %v", info.Provider, ProviderAzure)
	}
	if info.Metadata["resource_group"] != "my-rg" {
		t.Errorf("resource_group = %q, want %q", info.Metadata["resource_group"], "my-rg")
	}
}

func TestAttestationInfo_GCP(t *testing.T) {
	info := &AttestationInfo{
		Provider:     ProviderGCP,
		Document:     "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
		DocumentType: "identity-token",
		Nonce:        "https://my-service.example.com",
		Timestamp:    time.Now(),
	}

	if info.Provider != ProviderGCP {
		t.Errorf("Provider = %v, want %v", info.Provider, ProviderGCP)
	}
	if info.DocumentType != "identity-token" {
		t.Errorf("DocumentType = %q, want %q", info.DocumentType, "identity-token")
	}
}

func TestAttestationInfo_Azure(t *testing.T) {
	info := &AttestationInfo{
		Provider:     ProviderAzure,
		Document:     "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9...",
		DocumentType: "managed-identity-token",
		Nonce:        "https://management.azure.com/",
		Timestamp:    time.Now(),
	}

	if info.Provider != ProviderAzure {
		t.Errorf("Provider = %v, want %v", info.Provider, ProviderAzure)
	}
	if info.DocumentType != "managed-identity-token" {
		t.Errorf("DocumentType = %q, want %q", info.DocumentType, "managed-identity-token")
	}
}

func TestIdentityProvider_Interface(t *testing.T) {
	// Verify all providers implement the interface
	var _ IdentityProvider = (*AWSIdentityProvider)(nil)
	var _ IdentityProvider = (*GCPIdentityProvider)(nil)
	var _ IdentityProvider = (*AzureIdentityProvider)(nil)
}

func TestAttestationProvider_Interface(t *testing.T) {
	// Verify all providers implement the interface
	var _ AttestationProvider = (*AWSAttestationProvider)(nil)
	var _ AttestationProvider = (*GCPAttestationProvider)(nil)
	var _ AttestationProvider = (*AzureAttestationProvider)(nil)
}
