package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/identity"
)

// GCPConfig configures the GCP identity provider.
type GCPConfig struct {
	// TrustDomain is the SPIFFE trust domain for GCP identities.
	TrustDomain string

	// ProjectID is the GCP project ID (auto-detected if empty).
	ProjectID string

	// ServiceAccountEmail is the service account email.
	ServiceAccountEmail string

	// MetadataEndpoint is the GCP metadata server endpoint.
	// Default: http://metadata.google.internal
	MetadataEndpoint string

	// MetadataTimeout is the timeout for metadata requests.
	// Default: 5 seconds
	MetadataTimeout time.Duration

	// Audience is the audience for identity tokens.
	// Required for generating identity tokens.
	Audience string

	// RefreshInterval is how often to refresh credentials.
	// Default: 5 minutes
	RefreshInterval time.Duration

	// HealthCheckInterval is how often to check provider health.
	// Default: 30 seconds
	HealthCheckInterval time.Duration
}

// DefaultGCPConfig returns a GCPConfig with default values.
func DefaultGCPConfig(trustDomain string) *GCPConfig {
	return &GCPConfig{
		TrustDomain:         trustDomain,
		MetadataEndpoint:    "http://metadata.google.internal",
		MetadataTimeout:     5 * time.Second,
		RefreshInterval:     5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
	}
}

// GCPProvider implements the IdentityProvider interface for GCP.
type GCPProvider struct {
	config *GCPConfig
	client *http.Client

	mu                   sync.RWMutex
	started              bool
	status               identity.ProviderStatus
	statusMessage        string
	trustBundle          *identity.TrustBundle
	projectID            string
	projectNumber        string
	zone                 string
	instanceID           string
	instanceName         string
	serviceAccountEmail  string
	serviceAccountScopes []string

	healthCheckCancel context.CancelFunc
	lastHealthCheck   time.Time
}

// GCPInstanceMetadata contains GCP instance metadata.
type GCPInstanceMetadata struct {
	ProjectID           string   `json:"project-id"`
	ProjectNumber       string   `json:"numeric-project-id"`
	Zone                string   `json:"zone"`
	InstanceID          string   `json:"id"`
	InstanceName        string   `json:"name"`
	ServiceAccountEmail string   `json:"service-account-email"`
	Scopes              []string `json:"scopes"`
}

// NewGCPProvider creates a new GCP identity provider.
func NewGCPProvider(config *GCPConfig) (*GCPProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.TrustDomain == "" {
		return nil, fmt.Errorf("trust domain is required")
	}

	return &GCPProvider{
		config: config,
		client: &http.Client{
			Timeout: config.MetadataTimeout,
		},
		status: identity.ProviderStatusUnknown,
	}, nil
}

// Type returns the provider type.
func (p *GCPProvider) Type() identity.ProviderType {
	return identity.ProviderTypeGCP
}

// Start starts the provider.
func (p *GCPProvider) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = true
	p.mu.Unlock()

	// Detect GCP environment
	if err := p.detectEnvironment(ctx); err != nil {
		p.mu.Lock()
		p.status = identity.ProviderStatusUnhealthy
		p.statusMessage = err.Error()
		p.mu.Unlock()
		return err
	}

	p.mu.Lock()
	p.status = identity.ProviderStatusHealthy
	p.statusMessage = ""
	p.mu.Unlock()

	// Start health check loop
	healthCtx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	p.healthCheckCancel = cancel
	p.mu.Unlock()
	go p.healthCheckLoop(healthCtx)

	return nil
}

// Stop stops the provider.
func (p *GCPProvider) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return nil
	}
	p.started = false

	if p.healthCheckCancel != nil {
		p.healthCheckCancel()
	}

	return nil
}

// Health returns the current health status.
func (p *GCPProvider) Health(ctx context.Context) identity.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// Info returns detailed provider information.
func (p *GCPProvider) Info(ctx context.Context) identity.ProviderInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metadata := map[string]string{
		"project_id": p.projectID,
	}
	if p.zone != "" {
		metadata["zone"] = p.zone
	}
	if p.instanceID != "" {
		metadata["instance_id"] = p.instanceID
	}
	if p.instanceName != "" {
		metadata["instance_name"] = p.instanceName
	}
	if p.serviceAccountEmail != "" {
		metadata["service_account"] = p.serviceAccountEmail
	}

	return identity.ProviderInfo{
		Type:            identity.ProviderTypeGCP,
		Status:          p.status,
		TrustDomain:     p.config.TrustDomain,
		Capabilities:    []string{"instance_identity", "workload_identity", "identity_token"},
		LastHealthCheck: p.lastHealthCheck,
		ErrorMessage:    p.statusMessage,
		Metadata:        metadata,
	}
}

// TrustDomain returns the trust domain.
func (p *GCPProvider) TrustDomain() string {
	return p.config.TrustDomain
}

// GetTrustBundle returns the current trust bundle.
func (p *GCPProvider) GetTrustBundle(ctx context.Context) (*identity.TrustBundle, error) {
	p.mu.RLock()
	bundle := p.trustBundle
	p.mu.RUnlock()

	if bundle != nil {
		return bundle, nil
	}

	// GCP doesn't provide a trust bundle like SPIFFE
	return &identity.TrustBundle{
		TrustDomain: p.config.TrustDomain,
		UpdatedAt:   time.Now(),
	}, nil
}

// WatchTrustBundle watches for trust bundle updates.
func (p *GCPProvider) WatchTrustBundle(ctx context.Context) (<-chan *identity.TrustBundle, error) {
	ch := make(chan *identity.TrustBundle, 1)
	// GCP doesn't have dynamic trust bundle updates
	return ch, nil
}

// GetInstanceMetadata returns the GCP instance metadata.
func (p *GCPProvider) GetInstanceMetadata(ctx context.Context) (*GCPInstanceMetadata, error) {
	meta := &GCPInstanceMetadata{}

	// Get project ID
	projectID, err := p.getMetadataValue(ctx, "/computeMetadata/v1/project/project-id")
	if err != nil {
		return nil, fmt.Errorf("failed to get project ID: %w", err)
	}
	meta.ProjectID = projectID

	// Get project number
	projectNum, err := p.getMetadataValue(ctx, "/computeMetadata/v1/project/numeric-project-id")
	if err == nil {
		meta.ProjectNumber = projectNum
	}

	// Get zone
	zone, err := p.getMetadataValue(ctx, "/computeMetadata/v1/instance/zone")
	if err == nil {
		// Zone is returned as projects/PROJECT_NUM/zones/ZONE, extract just the zone
		parts := strings.Split(zone, "/")
		if len(parts) > 0 {
			meta.Zone = parts[len(parts)-1]
		}
	}

	// Get instance ID
	instanceID, err := p.getMetadataValue(ctx, "/computeMetadata/v1/instance/id")
	if err == nil {
		meta.InstanceID = instanceID
	}

	// Get instance name
	instanceName, err := p.getMetadataValue(ctx, "/computeMetadata/v1/instance/name")
	if err == nil {
		meta.InstanceName = instanceName
	}

	// Get service account email
	saEmail, err := p.getMetadataValue(ctx, "/computeMetadata/v1/instance/service-accounts/default/email")
	if err == nil {
		meta.ServiceAccountEmail = saEmail
	}

	return meta, nil
}

// GetIdentityToken returns a GCP identity token for the given audience.
func (p *GCPProvider) GetIdentityToken(ctx context.Context, audience string) (string, error) {
	if audience == "" {
		audience = p.config.Audience
	}
	if audience == "" {
		return "", fmt.Errorf("audience is required")
	}

	path := fmt.Sprintf("/computeMetadata/v1/instance/service-accounts/default/identity?audience=%s", audience)
	token, err := p.getMetadataValue(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to get identity token: %w", err)
	}

	return token, nil
}

// GetAccessToken returns a GCP access token.
func (p *GCPProvider) GetAccessToken(ctx context.Context) (string, time.Time, error) {
	path := "/computeMetadata/v1/instance/service-accounts/default/token"
	resp, err := p.getMetadataValue(ctx, path)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get access token: %w", err)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}

	if err := json.Unmarshal([]byte(resp), &tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse token response: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return tokenResp.AccessToken, expiresAt, nil
}

// GetSPIFFEID returns the SPIFFE ID for this GCP instance.
func (p *GCPProvider) GetSPIFFEID() (identity.SPIFFEID, error) {
	p.mu.RLock()
	projectID := p.projectID
	zone := p.zone
	instanceID := p.instanceID
	p.mu.RUnlock()

	if projectID == "" {
		return identity.SPIFFEID{}, fmt.Errorf("instance not detected")
	}

	// SPIFFE ID format: spiffe://trust-domain/gcp/project-id/zone/instance-id
	path := fmt.Sprintf("/gcp/%s/%s/%s", projectID, zone, instanceID)
	return identity.SPIFFEID{
		TrustDomain: p.config.TrustDomain,
		Path:        path,
	}, nil
}

// IsWorkloadIdentityAvailable returns true if Workload Identity is available.
func (p *GCPProvider) IsWorkloadIdentityAvailable() bool {
	// Check for the presence of the workload identity configuration
	tokenPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if tokenPath != "" {
		if _, err := os.Stat(tokenPath); err == nil {
			return true
		}
	}

	// Check for GKE workload identity
	gkeIdentityPath := "/var/run/secrets/tokens/gcp-ksa"
	if _, err := os.Stat(gkeIdentityPath); err == nil {
		return true
	}

	return false
}

// CreateAttestationEvidence creates attestation evidence for GCP.
func (p *GCPProvider) CreateAttestationEvidence(ctx context.Context) (*identity.AttestationEvidence, error) {
	// Get identity token as attestation evidence
	audience := p.config.Audience
	if audience == "" {
		audience = fmt.Sprintf("https://%s", p.config.TrustDomain)
	}

	token, err := p.GetIdentityToken(ctx, audience)
	if err != nil {
		return nil, err
	}

	p.mu.RLock()
	metadata := map[string]string{
		"project_id": p.projectID,
	}
	if p.zone != "" {
		metadata["zone"] = p.zone
	}
	if p.instanceID != "" {
		metadata["instance_id"] = p.instanceID
	}
	p.mu.RUnlock()

	return &identity.AttestationEvidence{
		Type:     identity.AttestationTypeGCPIIT,
		Data:     []byte(token),
		Metadata: metadata,
	}, nil
}

// Private methods

func (p *GCPProvider) detectEnvironment(ctx context.Context) error {
	meta, err := p.GetInstanceMetadata(ctx)
	if err != nil {
		// May not be on GCP, check for workload identity
		if p.IsWorkloadIdentityAvailable() {
			p.mu.Lock()
			p.projectID = p.config.ProjectID
			if p.projectID == "" {
				p.projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
			}
			p.mu.Unlock()
			return nil
		}
		return fmt.Errorf("failed to detect GCP environment: %w", err)
	}

	p.mu.Lock()
	p.projectID = meta.ProjectID
	p.projectNumber = meta.ProjectNumber
	p.zone = meta.Zone
	p.instanceID = meta.InstanceID
	p.instanceName = meta.InstanceName
	p.serviceAccountEmail = meta.ServiceAccountEmail
	p.mu.Unlock()

	return nil
}

func (p *GCPProvider) getMetadataValue(ctx context.Context, path string) (string, error) {
	url := p.config.MetadataEndpoint + path
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (p *GCPProvider) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.performHealthCheck(ctx)
		}
	}
}

func (p *GCPProvider) performHealthCheck(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, p.config.MetadataTimeout)
	defer cancel()

	_, err := p.GetInstanceMetadata(checkCtx)

	p.mu.Lock()
	p.lastHealthCheck = time.Now()

	if err != nil {
		if p.IsWorkloadIdentityAvailable() {
			p.status = identity.ProviderStatusDegraded
			p.statusMessage = "metadata unavailable, using workload identity"
		} else {
			p.status = identity.ProviderStatusUnhealthy
			p.statusMessage = err.Error()
		}
	} else {
		p.status = identity.ProviderStatusHealthy
		p.statusMessage = ""
	}
	p.mu.Unlock()
}

// Verify GCPProvider implements IdentityProvider
var _ identity.IdentityProvider = (*GCPProvider)(nil)
