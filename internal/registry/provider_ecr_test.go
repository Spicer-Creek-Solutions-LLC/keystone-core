package registry

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewECRCredentialProvider(t *testing.T) {
	provider := NewECRCredentialProvider("us-west-2")

	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
	if provider.region != "us-west-2" {
		t.Errorf("Region = %q, want %q", provider.region, "us-west-2")
	}
}

func TestECRCredentialProvider_RegistryType(t *testing.T) {
	provider := NewECRCredentialProvider("")

	if provider.RegistryType() != RegistryTypeECR {
		t.Errorf("RegistryType() = %v, want %v", provider.RegistryType(), RegistryTypeECR)
	}
}

func TestECRCredentialProvider_MatchesRegistry(t *testing.T) {
	provider := NewECRCredentialProvider("")

	tests := []struct {
		registry string
		want     bool
	}{
		{"123456789.dkr.ecr.us-west-2.amazonaws.com", true},
		{"123456789.dkr.ecr.eu-west-1.amazonaws.com", true},
		{"gcr.io", false},
		{"docker.io", false},
		{"myregistry.azurecr.io", false},
	}

	for _, tt := range tests {
		t.Run(tt.registry, func(t *testing.T) {
			if got := provider.MatchesRegistry(tt.registry); got != tt.want {
				t.Errorf("MatchesRegistry(%q) = %v, want %v", tt.registry, got, tt.want)
			}
		})
	}
}

func TestExtractECRRegion(t *testing.T) {
	tests := []struct {
		registry string
		want     string
	}{
		{"123456789.dkr.ecr.us-west-2.amazonaws.com", "us-west-2"},
		{"123456789.dkr.ecr.eu-west-1.amazonaws.com", "eu-west-1"},
		{"123456789.dkr.ecr.ap-southeast-1.amazonaws.com", "ap-southeast-1"},
		{"gcr.io", ""},
		{"docker.io", ""},
	}

	for _, tt := range tests {
		t.Run(tt.registry, func(t *testing.T) {
			if got := extractECRRegion(tt.registry); got != tt.want {
				t.Errorf("extractECRRegion(%q) = %q, want %q", tt.registry, got, tt.want)
			}
		})
	}
}

func TestECRCredentialProvider_IsAvailable(t *testing.T) {
	// This will always return false in a non-AWS environment
	provider := NewECRCredentialProvider("")
	// We just verify it doesn't panic
	_ = provider.IsAvailable()
}

func TestSha256Hex(t *testing.T) {
	// Test known hash
	data := []byte("{}")
	hash := sha256Hex(data)
	// SHA256 of "{}" is known
	expected := "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
	if hash != expected {
		t.Errorf("sha256Hex({}) = %s, want %s", hash, expected)
	}
}

func TestHmacSHA256(t *testing.T) {
	key := []byte("key")
	data := []byte("data")
	result := hmacSHA256(key, data)
	if len(result) != 32 {
		t.Errorf("hmacSHA256 result length = %d, want 32", len(result))
	}
}

func TestGetSignatureKey(t *testing.T) {
	key := getSignatureKey("secretKey", "20210101", "us-west-2", "ecr")
	if len(key) != 32 {
		t.Errorf("getSignatureKey result length = %d, want 32", len(key))
	}
}

func TestECRCredentialProvider_GetCredential_MockECR(t *testing.T) {
	// Create mock ECR server
	ecrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for correct headers
		if r.Header.Get("X-Amz-Target") != "AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken" {
			t.Errorf("Unexpected X-Amz-Target header: %s", r.Header.Get("X-Amz-Target"))
		}

		// Return mock response
		authToken := base64.StdEncoding.EncodeToString([]byte("AWS:testtoken123"))
		response := map[string]interface{}{
			"authorizationData": []map[string]interface{}{
				{
					"authorizationToken": authToken,
					"expiresAt":          time.Now().Add(12 * time.Hour).Unix(),
					"proxyEndpoint":      "https://123456789.dkr.ecr.us-west-2.amazonaws.com",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer ecrServer.Close()

	// Create mock IMDS server
	imdsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/api/token":
			w.Write([]byte("mock-token"))
		case "/latest/meta-data/iam/security-credentials/":
			w.Write([]byte("test-role"))
		case "/latest/meta-data/iam/security-credentials/test-role":
			creds := map[string]string{
				"AccessKeyId":     "AKIAIOSFODNN7EXAMPLE",
				"SecretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"Token":           "sessiontoken",
				"Expiration":      time.Now().Add(6 * time.Hour).Format(time.RFC3339),
			}
			json.NewEncoder(w).Encode(creds)
		default:
			http.NotFound(w, r)
		}
	}))
	defer imdsServer.Close()

	// Note: In a real test, we'd need to mock the HTTP clients
	// For now, this test documents the expected behavior
	t.Log("Mock servers created successfully")
}
