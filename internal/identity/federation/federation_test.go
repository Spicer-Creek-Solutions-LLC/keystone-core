package federation

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/identity"
)

// Test helper functions

func createTestCertificate(t *testing.T, spiffeIDStr string) (*x509.Certificate, []*x509.Certificate) {
	t.Helper()

	// Generate private key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Parse SPIFFE ID for URI
	spiffeURL, err := url.Parse(spiffeIDStr)
	if err != nil {
		t.Fatalf("failed to parse SPIFFE ID: %v", err)
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{spiffeURL},
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	return cert, []*x509.Certificate{cert}
}

func createTestTrustBundle(t *testing.T, trustDomain string) *identity.TrustBundle {
	t.Helper()

	cert, _ := createTestCertificate(t, "spiffe://"+trustDomain+"/ca")
	return &identity.TrustBundle{
		TrustDomain:     trustDomain,
		X509Authorities: []*x509.Certificate{cert},
		UpdatedAt:       time.Now(),
	}
}

// Type tests

func TestState(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StatePending, "pending"},
		{StateActive, "active"},
		{StateSuspended, "suspended"},
		{StateRevoked, "revoked"},
		{StateExpired, "expired"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.state)
			}
		})
	}
}

func TestFederatedDomain_IsExpired(t *testing.T) {
	t.Run("not_expired", func(t *testing.T) {
		domain := &FederatedDomain{
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if domain.IsExpired() {
			t.Error("expected not expired")
		}
	})

	t.Run("expired", func(t *testing.T) {
		domain := &FederatedDomain{
			ExpiresAt: time.Now().Add(-time.Hour),
		}
		if !domain.IsExpired() {
			t.Error("expected expired")
		}
	})

	t.Run("no_expiry", func(t *testing.T) {
		domain := &FederatedDomain{}
		if domain.IsExpired() {
			t.Error("expected not expired with zero expiry")
		}
	})
}

func TestFederatedDomain_IsActive(t *testing.T) {
	t.Run("active_not_expired", func(t *testing.T) {
		domain := &FederatedDomain{
			State:     StateActive,
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if !domain.IsActive() {
			t.Error("expected active")
		}
	})

	t.Run("active_expired", func(t *testing.T) {
		domain := &FederatedDomain{
			State:     StateActive,
			ExpiresAt: time.Now().Add(-time.Hour),
		}
		if domain.IsActive() {
			t.Error("expected not active when expired")
		}
	})

	t.Run("pending", func(t *testing.T) {
		domain := &FederatedDomain{
			State: StatePending,
		}
		if domain.IsActive() {
			t.Error("expected not active when pending")
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("test.local")

	if config.LocalTrustDomain != "test.local" {
		t.Errorf("expected trust domain test.local, got %s", config.LocalTrustDomain)
	}
	if config.DefaultRefreshInterval == 0 {
		t.Error("expected default refresh interval")
	}
	if config.MaxFederatedDomains == 0 {
		t.Error("expected max federated domains")
	}
	if config.DefaultPolicy == nil {
		t.Error("expected default policy")
	}
}

// Manager tests

func TestNewManager(t *testing.T) {
	t.Run("nil_config", func(t *testing.T) {
		_, err := NewManager(nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("missing_trust_domain", func(t *testing.T) {
		_, err := NewManager(&Config{})
		if err == nil {
			t.Error("expected error for missing trust domain")
		}
	})

	t.Run("valid_config", func(t *testing.T) {
		config := DefaultConfig("test.local")
		manager, err := NewManager(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if manager == nil {
			t.Error("expected manager")
		}
	})
}

func TestManager_AddFederatedDomain(t *testing.T) {
	config := DefaultConfig("local.domain")
	config.RequireApproval = false
	manager, _ := NewManager(config)

	ctx := context.Background()

	t.Run("add_valid_domain", func(t *testing.T) {
		domain := &FederatedDomain{
			TrustDomain: "federated.domain",
			Type:        TypeBidirectional,
			TrustBundle: createTestTrustBundle(t, "federated.domain"),
		}

		err := manager.AddFederatedDomain(ctx, domain)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify it was added
		retrieved, err := manager.GetFederatedDomain(ctx, "federated.domain")
		if err != nil {
			t.Fatalf("failed to get domain: %v", err)
		}
		if retrieved.TrustDomain != "federated.domain" {
			t.Errorf("expected federated.domain, got %s", retrieved.TrustDomain)
		}
		if retrieved.State != StateActive {
			t.Errorf("expected active state, got %s", retrieved.State)
		}
	})

	t.Run("add_nil_domain", func(t *testing.T) {
		err := manager.AddFederatedDomain(ctx, nil)
		if err == nil {
			t.Error("expected error for nil domain")
		}
	})

	t.Run("add_empty_trust_domain", func(t *testing.T) {
		err := manager.AddFederatedDomain(ctx, &FederatedDomain{})
		if err == nil {
			t.Error("expected error for empty trust domain")
		}
	})

	t.Run("add_local_trust_domain", func(t *testing.T) {
		domain := &FederatedDomain{
			TrustDomain: "local.domain",
		}
		err := manager.AddFederatedDomain(ctx, domain)
		if err == nil {
			t.Error("expected error for local trust domain")
		}
	})

	t.Run("add_duplicate", func(t *testing.T) {
		domain := &FederatedDomain{
			TrustDomain: "duplicate.domain",
		}
		manager.AddFederatedDomain(ctx, domain)

		err := manager.AddFederatedDomain(ctx, domain)
		if err == nil {
			t.Error("expected error for duplicate domain")
		}
	})
}

func TestManager_RemoveFederatedDomain(t *testing.T) {
	config := DefaultConfig("local.domain")
	config.RequireApproval = false
	manager, _ := NewManager(config)
	ctx := context.Background()

	// Add a domain first
	domain := &FederatedDomain{
		TrustDomain: "to-remove.domain",
	}
	manager.AddFederatedDomain(ctx, domain)

	t.Run("remove_existing", func(t *testing.T) {
		err := manager.RemoveFederatedDomain(ctx, "to-remove.domain")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = manager.GetFederatedDomain(ctx, "to-remove.domain")
		if err == nil {
			t.Error("expected error for removed domain")
		}
	})

	t.Run("remove_nonexistent", func(t *testing.T) {
		err := manager.RemoveFederatedDomain(ctx, "nonexistent.domain")
		if err == nil {
			t.Error("expected error for nonexistent domain")
		}
	})
}

func TestManager_ListFederatedDomains(t *testing.T) {
	config := DefaultConfig("local.domain")
	config.RequireApproval = false
	manager, _ := NewManager(config)
	ctx := context.Background()

	// Add some domains
	for i := 0; i < 3; i++ {
		domain := &FederatedDomain{
			TrustDomain: "domain" + string(rune('a'+i)) + ".test",
		}
		manager.AddFederatedDomain(ctx, domain)
	}

	domains, err := manager.ListFederatedDomains(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(domains) != 3 {
		t.Errorf("expected 3 domains, got %d", len(domains))
	}
}

func TestManager_UpdateFederatedDomain(t *testing.T) {
	config := DefaultConfig("local.domain")
	config.RequireApproval = false
	manager, _ := NewManager(config)
	ctx := context.Background()

	// Add a domain
	domain := &FederatedDomain{
		TrustDomain: "update.domain",
		State:       StateActive,
	}
	manager.AddFederatedDomain(ctx, domain)

	t.Run("update_existing", func(t *testing.T) {
		domain.State = StateSuspended
		err := manager.UpdateFederatedDomain(ctx, domain)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		retrieved, _ := manager.GetFederatedDomain(ctx, "update.domain")
		if retrieved.State != StateSuspended {
			t.Errorf("expected suspended state, got %s", retrieved.State)
		}
	})

	t.Run("update_nonexistent", func(t *testing.T) {
		nonexistent := &FederatedDomain{
			TrustDomain: "nonexistent.domain",
		}
		err := manager.UpdateFederatedDomain(ctx, nonexistent)
		if err == nil {
			t.Error("expected error for nonexistent domain")
		}
	})
}

func TestManager_GetAggregatedTrustBundle(t *testing.T) {
	localBundle := createTestTrustBundle(t, "local.domain")
	config := DefaultConfig("local.domain")
	config.LocalTrustBundle = localBundle
	config.RequireApproval = false
	manager, _ := NewManager(config)
	ctx := context.Background()

	// Add a federated domain with trust bundle
	federatedBundle := createTestTrustBundle(t, "federated.domain")
	domain := &FederatedDomain{
		TrustDomain: "federated.domain",
		State:       StateActive,
		TrustBundle: federatedBundle,
	}
	manager.AddFederatedDomain(ctx, domain)

	aggregated, err := manager.GetAggregatedTrustBundle(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should include both local and federated certs
	if len(aggregated.X509Authorities) != 2 {
		t.Errorf("expected 2 authorities, got %d", len(aggregated.X509Authorities))
	}
}

func TestManager_ValidateSVID(t *testing.T) {
	localBundle := createTestTrustBundle(t, "local.domain")
	config := DefaultConfig("local.domain")
	config.LocalTrustBundle = localBundle
	config.RequireApproval = false
	manager, _ := NewManager(config)
	ctx := context.Background()

	t.Run("validate_local_svid", func(t *testing.T) {
		cert, certs := createTestCertificate(t, "spiffe://local.domain/agent/test")
		svid := &identity.X509SVID{
			SPIFFEID: identity.SPIFFEID{
				TrustDomain: "local.domain",
				Path:        "/agent/test",
			},
			Certificates: certs,
			ExpiresAt:    cert.NotAfter,
		}

		// Update the local bundle to include this cert as CA
		config.LocalTrustBundle.X509Authorities = []*x509.Certificate{cert}

		result, err := manager.ValidateSVID(ctx, svid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid SVID: %s", result.Error)
		}
		if result.IsFederated {
			t.Error("expected not federated")
		}
	})

	t.Run("validate_nil_svid", func(t *testing.T) {
		result, err := manager.ValidateSVID(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid for nil SVID")
		}
	})

	t.Run("validate_unfederated_domain", func(t *testing.T) {
		cert, certs := createTestCertificate(t, "spiffe://unknown.domain/agent/test")
		svid := &identity.X509SVID{
			SPIFFEID: identity.SPIFFEID{
				TrustDomain: "unknown.domain",
				Path:        "/agent/test",
			},
			Certificates: certs,
			ExpiresAt:    cert.NotAfter,
		}

		result, err := manager.ValidateSVID(ctx, svid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid for unfederated domain")
		}
	})

	t.Run("validate_federated_svid", func(t *testing.T) {
		// Create federated domain with matching trust bundle
		cert, certs := createTestCertificate(t, "spiffe://federated.domain/service/api")
		federatedBundle := &identity.TrustBundle{
			TrustDomain:     "federated.domain",
			X509Authorities: []*x509.Certificate{cert},
		}
		domain := &FederatedDomain{
			TrustDomain: "federated.domain",
			State:       StateActive,
			Type:        TypeBidirectional,
			TrustBundle: federatedBundle,
		}
		manager.AddFederatedDomain(ctx, domain)

		svid := &identity.X509SVID{
			SPIFFEID: identity.SPIFFEID{
				TrustDomain: "federated.domain",
				Path:        "/service/api",
			},
			Certificates: certs,
			ExpiresAt:    cert.NotAfter,
		}

		result, err := manager.ValidateSVID(ctx, svid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid SVID: %s", result.Error)
		}
		if !result.IsFederated {
			t.Error("expected federated")
		}
		if result.FederationType != TypeBidirectional {
			t.Errorf("expected bidirectional, got %s", result.FederationType)
		}
	})
}

func TestManager_Lifecycle(t *testing.T) {
	store := NewInMemoryStore()
	config := DefaultConfig("local.domain")
	config.Store = store
	config.RequireApproval = false

	manager, _ := NewManager(config)
	ctx := context.Background()

	// Add domain before start
	domain := &FederatedDomain{
		TrustDomain: "prestart.domain",
		State:       StateActive,
	}
	store.Save(ctx, domain)

	// Start manager
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Verify domain was loaded
	loaded, err := manager.GetFederatedDomain(ctx, "prestart.domain")
	if err != nil {
		t.Fatalf("failed to get domain: %v", err)
	}
	if loaded.TrustDomain != "prestart.domain" {
		t.Errorf("expected prestart.domain, got %s", loaded.TrustDomain)
	}

	// Stop manager
	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("failed to stop manager: %v", err)
	}
}

// Policy tests

func TestApplyPolicy(t *testing.T) {
	config := DefaultConfig("local.domain")
	manager, _ := NewManager(config)

	tests := []struct {
		name     string
		spiffeID identity.SPIFFEID
		policy   *TrustPolicy
		wantErr  bool
	}{
		{
			name: "allowed_path",
			spiffeID: identity.SPIFFEID{
				TrustDomain: "test.domain",
				Path:        "/service/api",
			},
			policy: &TrustPolicy{
				AllowedPaths: []string{"/service/*"},
			},
			wantErr: false,
		},
		{
			name: "denied_path",
			spiffeID: identity.SPIFFEID{
				TrustDomain: "test.domain",
				Path:        "/admin/secret",
			},
			policy: &TrustPolicy{
				DeniedPaths: []string{"/admin/*"},
			},
			wantErr: true,
		},
		{
			name: "denied_takes_precedence",
			spiffeID: identity.SPIFFEID{
				TrustDomain: "test.domain",
				Path:        "/admin/api",
			},
			policy: &TrustPolicy{
				AllowedPaths: []string{"/admin/**"},
				DeniedPaths:  []string{"/admin/*"},
			},
			wantErr: true,
		},
		{
			name: "wildcard_all",
			spiffeID: identity.SPIFFEID{
				TrustDomain: "test.domain",
				Path:        "/anything/here",
			},
			policy: &TrustPolicy{
				AllowedPaths: []string{"/**"},
			},
			wantErr: false,
		},
		{
			name: "allowed_service",
			spiffeID: identity.SPIFFEID{
				TrustDomain: "test.domain",
				Path:        "/service/myapi",
			},
			policy: &TrustPolicy{
				AllowedServices: []string{"myapi"},
			},
			wantErr: false,
		},
		{
			name: "denied_service",
			spiffeID: identity.SPIFFEID{
				TrustDomain: "test.domain",
				Path:        "/service/blocked",
			},
			policy: &TrustPolicy{
				DeniedServices: []string{"blocked"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.applyPolicy(tt.spiffeID, tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("applyPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Store tests

func TestInMemoryStore(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	t.Run("save_and_load", func(t *testing.T) {
		domain := &FederatedDomain{
			TrustDomain: "test.domain",
			State:       StateActive,
		}

		err := store.Save(ctx, domain)
		if err != nil {
			t.Fatalf("failed to save: %v", err)
		}

		loaded, err := store.Load(ctx, "test.domain")
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if loaded.TrustDomain != "test.domain" {
			t.Errorf("expected test.domain, got %s", loaded.TrustDomain)
		}
	})

	t.Run("load_nonexistent", func(t *testing.T) {
		_, err := store.Load(ctx, "nonexistent.domain")
		if err == nil {
			t.Error("expected error for nonexistent domain")
		}
	})

	t.Run("delete", func(t *testing.T) {
		domain := &FederatedDomain{
			TrustDomain: "to-delete.domain",
		}
		store.Save(ctx, domain)

		err := store.Delete(ctx, "to-delete.domain")
		if err != nil {
			t.Fatalf("failed to delete: %v", err)
		}

		_, err = store.Load(ctx, "to-delete.domain")
		if err == nil {
			t.Error("expected error after delete")
		}
	})

	t.Run("list", func(t *testing.T) {
		store.Clear()

		for i := 0; i < 3; i++ {
			domain := &FederatedDomain{
				TrustDomain: "list" + string(rune('a'+i)) + ".domain",
			}
			store.Save(ctx, domain)
		}

		domains, err := store.List(ctx)
		if err != nil {
			t.Fatalf("failed to list: %v", err)
		}
		if len(domains) != 3 {
			t.Errorf("expected 3 domains, got %d", len(domains))
		}
	})

	t.Run("count", func(t *testing.T) {
		store.Clear()
		store.Save(ctx, &FederatedDomain{TrustDomain: "count.domain"})

		if store.Count() != 1 {
			t.Errorf("expected count 1, got %d", store.Count())
		}
	})
}

// Fetcher tests

func TestHTTPBundleFetcher(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		config := DefaultBundleFetcherConfig()
		if config.Timeout == 0 {
			t.Error("expected default timeout")
		}
		if config.MaxBundleSize == 0 {
			t.Error("expected default max size")
		}
	})

	t.Run("fetch_pem_bundle", func(t *testing.T) {
		// Create test certificate
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "test"},
			NotBefore:    time.Now(),
			NotAfter:     time.Now().Add(time.Hour),
		}
		certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certDER,
		})

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-pem-file")
			w.Write(certPEM)
		}))
		defer server.Close()

		fetcher := NewHTTPBundleFetcher(nil)
		bundle, err := fetcher.Fetch(context.Background(), server.URL, "https_web")
		if err != nil {
			t.Fatalf("failed to fetch: %v", err)
		}
		if len(bundle.X509Authorities) != 1 {
			t.Errorf("expected 1 authority, got %d", len(bundle.X509Authorities))
		}
	})

	t.Run("fetch_json_bundle", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bundle := SPIFFEBundle{
				Keys:                 []SPIFFEBundleKey{},
				SpiffeRefreshHint:    300,
				SpiffeSequenceNumber: 1,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(bundle)
		}))
		defer server.Close()

		fetcher := NewHTTPBundleFetcher(nil)
		bundle, err := fetcher.Fetch(context.Background(), server.URL, "spiffe_bundle_endpoint")
		if err != nil {
			t.Fatalf("failed to fetch: %v", err)
		}
		if bundle.RefreshHint != 300*time.Second {
			t.Errorf("expected 5 minute refresh hint, got %v", bundle.RefreshHint)
		}
		if bundle.SequenceNumber != 1 {
			t.Errorf("expected sequence 1, got %d", bundle.SequenceNumber)
		}
	})

	t.Run("fetch_error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		fetcher := NewHTTPBundleFetcher(nil)
		_, err := fetcher.Fetch(context.Background(), server.URL, "https_web")
		if err == nil {
			t.Error("expected error for 500 response")
		}
	})

	t.Run("unknown_profile", func(t *testing.T) {
		fetcher := NewHTTPBundleFetcher(nil)
		_, err := fetcher.Fetch(context.Background(), "http://example.com", "unknown_profile")
		if err == nil {
			t.Error("expected error for unknown profile")
		}
	})
}

// Helper function tests

func TestMatchPath(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    bool
	}{
		{"/service/api", "/service/*", true},
		{"/service/api/v1", "/service/*", false},
		{"/service/api/v1", "/service/**", true},
		{"/anything", "/**", true},
		{"/agent/test", "/agent/test", true},
		{"/agent/other", "/agent/test", false},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.pattern, func(t *testing.T) {
			got := matchPath(tt.path, tt.pattern)
			if got != tt.want {
				t.Errorf("matchPath(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestExtractServiceName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/service/api", "api"},
		{"/ns/default/sa/myservice", "myservice"},
		{"/sa/myservice", "myservice"},
		{"/agent/test", "test"},
		{"/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractServiceName(tt.path)
			if got != tt.want {
				t.Errorf("extractServiceName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// Event callback test

func TestEventCallback(t *testing.T) {
	var events []*Event

	config := DefaultConfig("local.domain")
	config.RequireApproval = false
	config.EventCallback = func(event *Event) {
		events = append(events, event)
	}

	manager, _ := NewManager(config)
	ctx := context.Background()

	// Add domain
	domain := &FederatedDomain{
		TrustDomain: "event.domain",
	}
	manager.AddFederatedDomain(ctx, domain)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAdded {
		t.Errorf("expected added event, got %s", events[0].Type)
	}
	if events[0].TrustDomain != "event.domain" {
		t.Errorf("expected event.domain, got %s", events[0].TrustDomain)
	}

	// Remove domain
	manager.RemoveFederatedDomain(ctx, "event.domain")

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1].Type != EventRemoved {
		t.Errorf("expected removed event, got %s", events[1].Type)
	}
}
