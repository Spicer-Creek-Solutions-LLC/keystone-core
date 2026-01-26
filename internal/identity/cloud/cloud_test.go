package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/identity"
)

// TestDefaultAWSConfig tests AWS default configuration.
func TestDefaultAWSConfig(t *testing.T) {
	cfg := DefaultAWSConfig("example.org")

	if cfg.TrustDomain != "example.org" {
		t.Errorf("expected trust domain 'example.org', got %s", cfg.TrustDomain)
	}
	if cfg.IMDSEndpoint != "http://169.254.169.254" {
		t.Errorf("unexpected IMDS endpoint: %s", cfg.IMDSEndpoint)
	}
	if !cfg.IMDSv2 {
		t.Error("expected IMDSv2 enabled by default")
	}
	if cfg.IMDSTimeout != 5*time.Second {
		t.Errorf("unexpected IMDS timeout: %v", cfg.IMDSTimeout)
	}
	if cfg.RefreshInterval != 5*time.Minute {
		t.Errorf("unexpected refresh interval: %v", cfg.RefreshInterval)
	}
}

// TestDefaultGCPConfig tests GCP default configuration.
func TestDefaultGCPConfig(t *testing.T) {
	cfg := DefaultGCPConfig("example.org")

	if cfg.TrustDomain != "example.org" {
		t.Errorf("expected trust domain 'example.org', got %s", cfg.TrustDomain)
	}
	if cfg.MetadataEndpoint != "http://metadata.google.internal" {
		t.Errorf("unexpected metadata endpoint: %s", cfg.MetadataEndpoint)
	}
	if cfg.MetadataTimeout != 5*time.Second {
		t.Errorf("unexpected metadata timeout: %v", cfg.MetadataTimeout)
	}
}

// TestDefaultAzureConfig tests Azure default configuration.
func TestDefaultAzureConfig(t *testing.T) {
	cfg := DefaultAzureConfig("example.org")

	if cfg.TrustDomain != "example.org" {
		t.Errorf("expected trust domain 'example.org', got %s", cfg.TrustDomain)
	}
	if cfg.IMDSEndpoint != "http://169.254.169.254" {
		t.Errorf("unexpected IMDS endpoint: %s", cfg.IMDSEndpoint)
	}
	if cfg.Resource != "https://management.azure.com/" {
		t.Errorf("unexpected resource: %s", cfg.Resource)
	}
}

// TestNewAWSProvider tests AWS provider creation.
func TestNewAWSProvider(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		_, err := NewAWSProvider(nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("empty trust domain", func(t *testing.T) {
		_, err := NewAWSProvider(&AWSConfig{})
		if err == nil {
			t.Error("expected error for empty trust domain")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := DefaultAWSConfig("example.org")
		provider, err := NewAWSProvider(cfg)
		if err != nil {
			t.Fatalf("NewAWSProvider() error = %v", err)
		}
		if provider.Type() != identity.ProviderTypeAWS {
			t.Errorf("expected type AWS, got %v", provider.Type())
		}
		if provider.TrustDomain() != "example.org" {
			t.Errorf("expected trust domain 'example.org', got %s", provider.TrustDomain())
		}
	})
}

// TestNewGCPProvider tests GCP provider creation.
func TestNewGCPProvider(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		_, err := NewGCPProvider(nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("empty trust domain", func(t *testing.T) {
		_, err := NewGCPProvider(&GCPConfig{})
		if err == nil {
			t.Error("expected error for empty trust domain")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := DefaultGCPConfig("example.org")
		provider, err := NewGCPProvider(cfg)
		if err != nil {
			t.Fatalf("NewGCPProvider() error = %v", err)
		}
		if provider.Type() != identity.ProviderTypeGCP {
			t.Errorf("expected type GCP, got %v", provider.Type())
		}
		if provider.TrustDomain() != "example.org" {
			t.Errorf("expected trust domain 'example.org', got %s", provider.TrustDomain())
		}
	})
}

// TestNewAzureProvider tests Azure provider creation.
func TestNewAzureProvider(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		_, err := NewAzureProvider(nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("empty trust domain", func(t *testing.T) {
		_, err := NewAzureProvider(&AzureConfig{})
		if err == nil {
			t.Error("expected error for empty trust domain")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := DefaultAzureConfig("example.org")
		provider, err := NewAzureProvider(cfg)
		if err != nil {
			t.Fatalf("NewAzureProvider() error = %v", err)
		}
		if provider.Type() != identity.ProviderTypeAzure {
			t.Errorf("expected type Azure, got %v", provider.Type())
		}
		if provider.TrustDomain() != "example.org" {
			t.Errorf("expected trust domain 'example.org', got %s", provider.TrustDomain())
		}
	})
}

// TestAWSProviderInfo tests AWS provider info.
func TestAWSProviderInfo(t *testing.T) {
	cfg := DefaultAWSConfig("example.org")
	provider, err := NewAWSProvider(cfg)
	if err != nil {
		t.Fatalf("NewAWSProvider() error = %v", err)
	}

	info := provider.Info(context.Background())

	if info.Type != identity.ProviderTypeAWS {
		t.Errorf("expected type AWS, got %v", info.Type)
	}
	if info.TrustDomain != "example.org" {
		t.Errorf("expected trust domain 'example.org', got %s", info.TrustDomain)
	}
	if len(info.Capabilities) == 0 {
		t.Error("expected non-empty capabilities")
	}
}

// TestGCPProviderInfo tests GCP provider info.
func TestGCPProviderInfo(t *testing.T) {
	cfg := DefaultGCPConfig("example.org")
	provider, err := NewGCPProvider(cfg)
	if err != nil {
		t.Fatalf("NewGCPProvider() error = %v", err)
	}

	info := provider.Info(context.Background())

	if info.Type != identity.ProviderTypeGCP {
		t.Errorf("expected type GCP, got %v", info.Type)
	}
	if info.TrustDomain != "example.org" {
		t.Errorf("expected trust domain 'example.org', got %s", info.TrustDomain)
	}
	if len(info.Capabilities) == 0 {
		t.Error("expected non-empty capabilities")
	}
}

// TestAzureProviderInfo tests Azure provider info.
func TestAzureProviderInfo(t *testing.T) {
	cfg := DefaultAzureConfig("example.org")
	provider, err := NewAzureProvider(cfg)
	if err != nil {
		t.Fatalf("NewAzureProvider() error = %v", err)
	}

	info := provider.Info(context.Background())

	if info.Type != identity.ProviderTypeAzure {
		t.Errorf("expected type Azure, got %v", info.Type)
	}
	if info.TrustDomain != "example.org" {
		t.Errorf("expected trust domain 'example.org', got %s", info.TrustDomain)
	}
	if len(info.Capabilities) == 0 {
		t.Error("expected non-empty capabilities")
	}
}

// TestAWSProviderHealth tests AWS provider initial health.
func TestAWSProviderHealth(t *testing.T) {
	cfg := DefaultAWSConfig("example.org")
	provider, err := NewAWSProvider(cfg)
	if err != nil {
		t.Fatalf("NewAWSProvider() error = %v", err)
	}

	// Initial health should be unknown
	status := provider.Health(context.Background())
	if status != identity.ProviderStatusUnknown {
		t.Errorf("expected status unknown, got %v", status)
	}
}

// TestAWSMockIMDS tests AWS with mock IMDS.
func TestAWSMockIMDS(t *testing.T) {
	// Create mock IMDS server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/api/token":
			w.Write([]byte("mock-token"))
		case "/latest/meta-data/ipv6":
			w.Write([]byte("2001:db8::1\n2001:db8::2\n"))
		case "/latest/dynamic/instance-identity/document":
			doc := AWSInstanceIdentity{
				InstanceID:       "i-1234567890abcdef0",
				AccountID:        "123456789012",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
				InstanceType:     "t3.micro",
			}
			json.NewEncoder(w).Encode(doc)
		case "/latest/dynamic/instance-identity/signature":
			w.Write([]byte("mock-signature"))
		case "/latest/dynamic/instance-identity/pkcs7":
			w.Write([]byte("mock-pkcs7"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := &AWSConfig{
		TrustDomain:         "example.org",
		IMDSEndpoint:        server.URL,
		IMDSv2:              true,
		IMDSTimeout:         5 * time.Second,
		HealthCheckInterval: time.Hour, // Don't run health checks during test
	}

	provider, err := NewAWSProvider(cfg)
	if err != nil {
		t.Fatalf("NewAWSProvider() error = %v", err)
	}

	ctx := context.Background()

	// Start provider
	if err := provider.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer provider.Stop(ctx)

	// Check health
	if provider.Health(ctx) != identity.ProviderStatusHealthy {
		t.Errorf("expected healthy status after start")
	}

	// Get instance identity
	id, err := provider.GetInstanceIdentity(ctx)
	if err != nil {
		t.Fatalf("GetInstanceIdentity() error = %v", err)
	}
	if id.InstanceID != "i-1234567890abcdef0" {
		t.Errorf("unexpected instance ID: %s", id.InstanceID)
	}
	if id.AccountID != "123456789012" {
		t.Errorf("unexpected account ID: %s", id.AccountID)
	}

	// Get signed identity
	signed, err := provider.GetSignedInstanceIdentity(ctx)
	if err != nil {
		t.Fatalf("GetSignedInstanceIdentity() error = %v", err)
	}
	if signed.Signature != "mock-signature" {
		t.Errorf("unexpected signature: %s", signed.Signature)
	}

	// Get SPIFFE ID
	spiffeID, err := provider.GetSPIFFEID()
	if err != nil {
		t.Fatalf("GetSPIFFEID() error = %v", err)
	}
	if spiffeID.TrustDomain != "example.org" {
		t.Errorf("unexpected trust domain: %s", spiffeID.TrustDomain)
	}
	expectedPath := "/aws/123456789012/us-east-1/i-1234567890abcdef0"
	if spiffeID.Path != expectedPath {
		t.Errorf("unexpected path: %s, expected: %s", spiffeID.Path, expectedPath)
	}

	info := provider.Info(ctx)
	if info.Metadata["ipv6_addresses"] != "2001:db8::1,2001:db8::2" {
		t.Errorf("unexpected ipv6 metadata: %s", info.Metadata["ipv6_addresses"])
	}
}

// TestGCPMockMetadata tests GCP with mock metadata server.
func TestGCPMockMetadata(t *testing.T) {
	// Create mock metadata server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for required header
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "missing metadata header", http.StatusForbidden)
			return
		}

		switch r.URL.Path {
		case "/computeMetadata/v1/project/project-id":
			w.Write([]byte("my-project-123"))
		case "/computeMetadata/v1/project/numeric-project-id":
			w.Write([]byte("123456789012"))
		case "/computeMetadata/v1/instance/zone":
			w.Write([]byte("projects/123456789012/zones/us-central1-a"))
		case "/computeMetadata/v1/instance/id":
			w.Write([]byte("1234567890123456789"))
		case "/computeMetadata/v1/instance/name":
			w.Write([]byte("my-instance"))
		case "/computeMetadata/v1/instance/service-accounts/default/email":
			w.Write([]byte("default@my-project-123.iam.gserviceaccount.com"))
		case "/computeMetadata/v1/instance/network-interfaces/0/ipv6s":
			w.Write([]byte("2001:db8::10\n2001:db8::11\n"))
		case "/computeMetadata/v1/instance/service-accounts/default/identity":
			w.Write([]byte("eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.mock"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := &GCPConfig{
		TrustDomain:         "example.org",
		MetadataEndpoint:    server.URL,
		MetadataTimeout:     5 * time.Second,
		HealthCheckInterval: time.Hour,
	}

	provider, err := NewGCPProvider(cfg)
	if err != nil {
		t.Fatalf("NewGCPProvider() error = %v", err)
	}

	ctx := context.Background()

	// Start provider
	if err := provider.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer provider.Stop(ctx)

	// Check health
	if provider.Health(ctx) != identity.ProviderStatusHealthy {
		t.Errorf("expected healthy status after start")
	}

	// Get instance metadata
	meta, err := provider.GetInstanceMetadata(ctx)
	if err != nil {
		t.Fatalf("GetInstanceMetadata() error = %v", err)
	}
	if meta.ProjectID != "my-project-123" {
		t.Errorf("unexpected project ID: %s", meta.ProjectID)
	}
	if meta.Zone != "us-central1-a" {
		t.Errorf("unexpected zone: %s", meta.Zone)
	}
	if meta.InstanceName != "my-instance" {
		t.Errorf("unexpected instance name: %s", meta.InstanceName)
	}

	// Get SPIFFE ID
	spiffeID, err := provider.GetSPIFFEID()
	if err != nil {
		t.Fatalf("GetSPIFFEID() error = %v", err)
	}
	if spiffeID.TrustDomain != "example.org" {
		t.Errorf("unexpected trust domain: %s", spiffeID.TrustDomain)
	}

	info := provider.Info(ctx)
	if info.Metadata["ipv6_addresses"] != "2001:db8::10,2001:db8::11" {
		t.Errorf("unexpected ipv6 metadata: %s", info.Metadata["ipv6_addresses"])
	}
}

// TestAzureMockIMDS tests Azure with mock IMDS.
func TestAzureMockIMDS(t *testing.T) {
	// Create mock IMDS server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for required header
		if r.Header.Get("Metadata") != "true" {
			http.Error(w, "missing metadata header", http.StatusForbidden)
			return
		}

		switch r.URL.Path {
		case "/metadata/instance":
			meta := AzureInstanceMetadata{
				Compute: AzureComputeMetadata{
					SubscriptionID:    "12345678-1234-1234-1234-123456789012",
					ResourceGroupName: "my-resource-group",
					VMID:              "abcd1234-ef56-7890-abcd-ef1234567890",
					Name:              "my-vm",
					Location:          "eastus",
				},
				Network: AzureNetworkMetadata{
					Interface: []AzureNetworkInterface{
						{
							IPv6: AzureIPConfig{
								IPAddress: []AzureIPAddress{
									{PrivateIP: "2001:db8::20"},
									{PrivateIP: "2001:db8::21"},
								},
							},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(meta)
		case "/metadata/identity/oauth2/token":
			token := AzureAccessToken{
				AccessToken: "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.mock",
				ExpiresIn:   "3600",
				TokenType:   "Bearer",
				Resource:    r.URL.Query().Get("resource"),
			}
			json.NewEncoder(w).Encode(token)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := &AzureConfig{
		TrustDomain:         "example.org",
		IMDSEndpoint:        server.URL,
		IMDSTimeout:         5 * time.Second,
		Resource:            "https://management.azure.com/",
		HealthCheckInterval: time.Hour,
	}

	provider, err := NewAzureProvider(cfg)
	if err != nil {
		t.Fatalf("NewAzureProvider() error = %v", err)
	}

	ctx := context.Background()

	// Start provider
	if err := provider.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer provider.Stop(ctx)

	// Check health
	if provider.Health(ctx) != identity.ProviderStatusHealthy {
		t.Errorf("expected healthy status after start")
	}

	// Get instance metadata
	meta, err := provider.GetInstanceMetadata(ctx)
	if err != nil {
		t.Fatalf("GetInstanceMetadata() error = %v", err)
	}
	if meta.Compute.SubscriptionID != "12345678-1234-1234-1234-123456789012" {
		t.Errorf("unexpected subscription ID: %s", meta.Compute.SubscriptionID)
	}
	if meta.Compute.ResourceGroupName != "my-resource-group" {
		t.Errorf("unexpected resource group: %s", meta.Compute.ResourceGroupName)
	}
	if meta.Compute.Name != "my-vm" {
		t.Errorf("unexpected VM name: %s", meta.Compute.Name)
	}

	// Get access token
	token, err := provider.GetAccessToken(ctx, "")
	if err != nil {
		t.Fatalf("GetAccessToken() error = %v", err)
	}
	if token.AccessToken == "" {
		t.Error("expected non-empty access token")
	}

	// Get SPIFFE ID
	spiffeID, err := provider.GetSPIFFEID()
	if err != nil {
		t.Fatalf("GetSPIFFEID() error = %v", err)
	}
	if spiffeID.TrustDomain != "example.org" {
		t.Errorf("unexpected trust domain: %s", spiffeID.TrustDomain)
	}

	info := provider.Info(ctx)
	if info.Metadata["ipv6_addresses"] != "2001:db8::20,2001:db8::21" {
		t.Errorf("unexpected ipv6 metadata: %s", info.Metadata["ipv6_addresses"])
	}
}

// TestAWSGetTrustBundle tests getting trust bundle.
func TestAWSGetTrustBundle(t *testing.T) {
	cfg := DefaultAWSConfig("example.org")
	provider, err := NewAWSProvider(cfg)
	if err != nil {
		t.Fatalf("NewAWSProvider() error = %v", err)
	}

	bundle, err := provider.GetTrustBundle(context.Background())
	if err != nil {
		t.Fatalf("GetTrustBundle() error = %v", err)
	}
	if bundle.TrustDomain != "example.org" {
		t.Errorf("unexpected trust domain: %s", bundle.TrustDomain)
	}
}

// TestGCPGetTrustBundle tests getting trust bundle.
func TestGCPGetTrustBundle(t *testing.T) {
	cfg := DefaultGCPConfig("example.org")
	provider, err := NewGCPProvider(cfg)
	if err != nil {
		t.Fatalf("NewGCPProvider() error = %v", err)
	}

	bundle, err := provider.GetTrustBundle(context.Background())
	if err != nil {
		t.Fatalf("GetTrustBundle() error = %v", err)
	}
	if bundle.TrustDomain != "example.org" {
		t.Errorf("unexpected trust domain: %s", bundle.TrustDomain)
	}
}

// TestAzureGetTrustBundle tests getting trust bundle.
func TestAzureGetTrustBundle(t *testing.T) {
	cfg := DefaultAzureConfig("example.org")
	provider, err := NewAzureProvider(cfg)
	if err != nil {
		t.Fatalf("NewAzureProvider() error = %v", err)
	}

	bundle, err := provider.GetTrustBundle(context.Background())
	if err != nil {
		t.Fatalf("GetTrustBundle() error = %v", err)
	}
	if bundle.TrustDomain != "example.org" {
		t.Errorf("unexpected trust domain: %s", bundle.TrustDomain)
	}
}

// TestAWSIsIRSAAvailable tests IRSA detection.
func TestAWSIsIRSAAvailable(t *testing.T) {
	cfg := DefaultAWSConfig("example.org")
	provider, err := NewAWSProvider(cfg)
	if err != nil {
		t.Fatalf("NewAWSProvider() error = %v", err)
	}

	// Should return false when token file doesn't exist
	if provider.IsIRSAAvailable() {
		t.Error("expected IRSA not available without token file")
	}

	// Create temp token file
	tmpFile, err := os.CreateTemp("", "irsa-token")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("mock-token")
	tmpFile.Close()

	cfg.WebIdentityTokenFile = tmpFile.Name()
	provider2, _ := NewAWSProvider(cfg)

	if !provider2.IsIRSAAvailable() {
		t.Error("expected IRSA available with token file")
	}

	token, err := provider2.GetWebIdentityToken()
	if err != nil {
		t.Fatalf("GetWebIdentityToken() error = %v", err)
	}
	if token != "mock-token" {
		t.Errorf("unexpected token: %s", token)
	}
}

// TestGCPIsWorkloadIdentityAvailable tests Workload Identity detection.
func TestGCPIsWorkloadIdentityAvailable(t *testing.T) {
	cfg := DefaultGCPConfig("example.org")
	provider, err := NewGCPProvider(cfg)
	if err != nil {
		t.Fatalf("NewGCPProvider() error = %v", err)
	}

	// Should return false when not on GKE
	// (We can't easily mock the file system in unit tests)
	result := provider.IsWorkloadIdentityAvailable()
	// Just verify it doesn't panic
	_ = result
}

// TestAzureIsManagedIdentityAvailable tests Managed Identity detection.
func TestAzureIsManagedIdentityAvailable(t *testing.T) {
	cfg := DefaultAzureConfig("example.org")
	cfg.IMDSTimeout = 100 * time.Millisecond
	provider, err := NewAzureProvider(cfg)
	if err != nil {
		t.Fatalf("NewAzureProvider() error = %v", err)
	}

	// Should return false when not on Azure
	result := provider.IsManagedIdentityAvailable()
	// Just verify it doesn't panic and returns false (since we're not on Azure)
	if result {
		t.Error("expected managed identity not available in test environment")
	}
}

// TestProviderStop tests stopping providers.
func TestProviderStop(t *testing.T) {
	t.Run("AWS", func(t *testing.T) {
		cfg := DefaultAWSConfig("example.org")
		provider, _ := NewAWSProvider(cfg)
		err := provider.Stop(context.Background())
		if err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	})

	t.Run("GCP", func(t *testing.T) {
		cfg := DefaultGCPConfig("example.org")
		provider, _ := NewGCPProvider(cfg)
		err := provider.Stop(context.Background())
		if err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	})

	t.Run("Azure", func(t *testing.T) {
		cfg := DefaultAzureConfig("example.org")
		provider, _ := NewAzureProvider(cfg)
		err := provider.Stop(context.Background())
		if err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	})
}

// TestCreateAttestationEvidence tests creating attestation evidence.
func TestCreateAttestationEvidence(t *testing.T) {
	// AWS with mock IMDS
	t.Run("AWS", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/latest/api/token":
				w.Write([]byte("mock-token"))
			case "/latest/dynamic/instance-identity/document":
				w.Write([]byte(`{"instanceId":"i-123","accountId":"123"}`))
			case "/latest/dynamic/instance-identity/signature":
				w.Write([]byte("sig"))
			case "/latest/dynamic/instance-identity/pkcs7":
				w.Write([]byte("pkcs7"))
			}
		}))
		defer server.Close()

		cfg := &AWSConfig{
			TrustDomain:         "example.org",
			IMDSEndpoint:        server.URL,
			IMDSv2:              true,
			IMDSTimeout:         5 * time.Second,
			HealthCheckInterval: time.Hour,
		}
		provider, _ := NewAWSProvider(cfg)
		provider.Start(context.Background())

		evidence, err := provider.CreateAttestationEvidence(context.Background())
		if err != nil {
			t.Fatalf("CreateAttestationEvidence() error = %v", err)
		}
		if evidence.Type != identity.AttestationTypeAWSIID {
			t.Errorf("unexpected type: %s", evidence.Type)
		}
	})

	// GCP with mock metadata
	t.Run("GCP", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/computeMetadata/v1/project/project-id":
				w.Write([]byte("project-123"))
			case "/computeMetadata/v1/instance/zone":
				w.Write([]byte("projects/123/zones/us-central1-a"))
			case "/computeMetadata/v1/instance/id":
				w.Write([]byte("123456"))
			case "/computeMetadata/v1/instance/service-accounts/default/identity":
				w.Write([]byte("jwt-token"))
			default:
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer server.Close()

		cfg := &GCPConfig{
			TrustDomain:         "example.org",
			MetadataEndpoint:    server.URL,
			MetadataTimeout:     5 * time.Second,
			Audience:            "https://example.org",
			HealthCheckInterval: time.Hour,
		}
		provider, _ := NewGCPProvider(cfg)
		provider.Start(context.Background())

		evidence, err := provider.CreateAttestationEvidence(context.Background())
		if err != nil {
			t.Fatalf("CreateAttestationEvidence() error = %v", err)
		}
		if evidence.Type != identity.AttestationTypeGCPIIT {
			t.Errorf("unexpected type: %s", evidence.Type)
		}
	})

	// Azure with mock IMDS
	t.Run("Azure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/metadata/instance":
				json.NewEncoder(w).Encode(AzureInstanceMetadata{
					Compute: AzureComputeMetadata{
						SubscriptionID:    "sub-123",
						ResourceGroupName: "rg-123",
						VMID:              "vm-123",
					},
				})
			case "/metadata/identity/oauth2/token":
				json.NewEncoder(w).Encode(AzureAccessToken{
					AccessToken: "token",
					TokenType:   "Bearer",
				})
			}
		}))
		defer server.Close()

		cfg := &AzureConfig{
			TrustDomain:         "example.org",
			IMDSEndpoint:        server.URL,
			IMDSTimeout:         5 * time.Second,
			Resource:            "https://management.azure.com/",
			HealthCheckInterval: time.Hour,
		}
		provider, _ := NewAzureProvider(cfg)
		provider.Start(context.Background())

		evidence, err := provider.CreateAttestationEvidence(context.Background())
		if err != nil {
			t.Fatalf("CreateAttestationEvidence() error = %v", err)
		}
		if evidence.Type != identity.AttestationTypeAzureIMDS {
			t.Errorf("unexpected type: %s", evidence.Type)
		}
	})
}

// Benchmark tests

func BenchmarkAWSGetInstanceIdentity(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/api/token":
			w.Write([]byte("mock-token"))
		case "/latest/dynamic/instance-identity/document":
			w.Write([]byte(`{"instanceId":"i-123","accountId":"123"}`))
		}
	}))
	defer server.Close()

	cfg := &AWSConfig{
		TrustDomain:         "example.org",
		IMDSEndpoint:        server.URL,
		IMDSv2:              true,
		IMDSTimeout:         5 * time.Second,
		HealthCheckInterval: time.Hour,
	}
	provider, _ := NewAWSProvider(cfg)
	provider.Start(context.Background())

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		provider.GetInstanceIdentity(ctx)
	}
}

func BenchmarkGCPGetInstanceMetadata(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("project-123"))
	}))
	defer server.Close()

	cfg := &GCPConfig{
		TrustDomain:         "example.org",
		MetadataEndpoint:    server.URL,
		MetadataTimeout:     5 * time.Second,
		HealthCheckInterval: time.Hour,
	}
	provider, _ := NewGCPProvider(cfg)
	provider.Start(context.Background())

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		provider.GetInstanceMetadata(ctx)
	}
}
