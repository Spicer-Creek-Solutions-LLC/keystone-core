package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/identity"
)

// AzureConfig configures the Azure identity provider.
type AzureConfig struct {
	// TrustDomain is the SPIFFE trust domain for Azure identities.
	TrustDomain string

	// SubscriptionID is the Azure subscription ID (auto-detected if empty).
	SubscriptionID string

	// ResourceGroup is the Azure resource group (auto-detected if empty).
	ResourceGroup string

	// TenantID is the Azure AD tenant ID (auto-detected if empty).
	TenantID string

	// ClientID is the managed identity client ID.
	// If empty, uses the system-assigned managed identity.
	ClientID string

	// IMDSEndpoint is the Azure Instance Metadata Service endpoint.
	// Default: http://169.254.169.254
	IMDSEndpoint string

	// IMDSTimeout is the timeout for IMDS requests.
	// Default: 5 seconds
	IMDSTimeout time.Duration

	// Resource is the Azure resource for which to obtain a token.
	// Default: https://management.azure.com/
	Resource string

	// RefreshInterval is how often to refresh credentials.
	// Default: 5 minutes
	RefreshInterval time.Duration

	// HealthCheckInterval is how often to check provider health.
	// Default: 30 seconds
	HealthCheckInterval time.Duration
}

// DefaultAzureConfig returns an AzureConfig with default values.
func DefaultAzureConfig(trustDomain string) *AzureConfig {
	return &AzureConfig{
		TrustDomain:         trustDomain,
		IMDSEndpoint:        "http://169.254.169.254",
		IMDSTimeout:         5 * time.Second,
		Resource:            "https://management.azure.com/",
		RefreshInterval:     5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
	}
}

// AzureProvider implements the IdentityProvider interface for Azure.
type AzureProvider struct {
	config *AzureConfig
	client *http.Client

	mu              sync.RWMutex
	started         bool
	status          identity.ProviderStatus
	statusMessage   string
	trustBundle     *identity.TrustBundle
	subscriptionID  string
	resourceGroup   string
	tenantID        string
	vmID            string
	vmName          string
	location        string
	vmScaleSetName  string

	healthCheckCancel context.CancelFunc
	lastHealthCheck   time.Time
}

// AzureInstanceMetadata contains Azure instance metadata.
type AzureInstanceMetadata struct {
	Compute AzureComputeMetadata `json:"compute"`
	Network AzureNetworkMetadata `json:"network"`
}

// AzureComputeMetadata contains compute-specific metadata.
type AzureComputeMetadata struct {
	AzEnvironment         string `json:"azEnvironment"`
	Location              string `json:"location"`
	Name                  string `json:"name"`
	OSType                string `json:"osType"`
	VMID                  string `json:"vmId"`
	VMSize                string `json:"vmSize"`
	SubscriptionID        string `json:"subscriptionId"`
	ResourceGroupName     string `json:"resourceGroupName"`
	ResourceID            string `json:"resourceId"`
	VMScaleSetName        string `json:"vmScaleSetName"`
	Zone                  string `json:"zone"`
	Tags                  string `json:"tags"`
	Version               string `json:"version"`
	Publisher             string `json:"publisher"`
	Offer                 string `json:"offer"`
	SKU                   string `json:"sku"`
	PlatformFaultDomain   string `json:"platformFaultDomain"`
	PlatformUpdateDomain  string `json:"platformUpdateDomain"`
}

// AzureNetworkMetadata contains network-specific metadata.
type AzureNetworkMetadata struct {
	Interface []AzureNetworkInterface `json:"interface"`
}

// AzureNetworkInterface contains network interface information.
type AzureNetworkInterface struct {
	IPv4       AzureIPConfig `json:"ipv4"`
	IPv6       AzureIPConfig `json:"ipv6"`
	MACAddress string        `json:"macAddress"`
}

// AzureIPConfig contains IP configuration.
type AzureIPConfig struct {
	IPAddress []AzureIPAddress `json:"ipAddress"`
	Subnet    []AzureSubnet    `json:"subnet"`
}

// AzureIPAddress contains IP address information.
type AzureIPAddress struct {
	PrivateIP string `json:"privateIpAddress"`
	PublicIP  string `json:"publicIpAddress"`
}

// AzureSubnet contains subnet information.
type AzureSubnet struct {
	Address string `json:"address"`
	Prefix  string `json:"prefix"`
}

// AzureAccessToken represents an Azure access token response.
type AzureAccessToken struct {
	AccessToken  string `json:"access_token"`
	ClientID     string `json:"client_id"`
	ExpiresIn    string `json:"expires_in"`
	ExpiresOn    string `json:"expires_on"`
	ExtExpiresIn string `json:"ext_expires_in"`
	NotBefore    string `json:"not_before"`
	Resource     string `json:"resource"`
	TokenType    string `json:"token_type"`
}

// NewAzureProvider creates a new Azure identity provider.
func NewAzureProvider(config *AzureConfig) (*AzureProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.TrustDomain == "" {
		return nil, fmt.Errorf("trust domain is required")
	}

	return &AzureProvider{
		config: config,
		client: &http.Client{
			Timeout: config.IMDSTimeout,
		},
		status: identity.ProviderStatusUnknown,
	}, nil
}

// Type returns the provider type.
func (p *AzureProvider) Type() identity.ProviderType {
	return identity.ProviderTypeAzure
}

// Start starts the provider.
func (p *AzureProvider) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = true
	p.mu.Unlock()

	// Detect Azure environment
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
func (p *AzureProvider) Stop(ctx context.Context) error {
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
func (p *AzureProvider) Health(ctx context.Context) identity.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// Info returns detailed provider information.
func (p *AzureProvider) Info(ctx context.Context) identity.ProviderInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metadata := map[string]string{
		"subscription_id": p.subscriptionID,
	}
	if p.resourceGroup != "" {
		metadata["resource_group"] = p.resourceGroup
	}
	if p.tenantID != "" {
		metadata["tenant_id"] = p.tenantID
	}
	if p.vmID != "" {
		metadata["vm_id"] = p.vmID
	}
	if p.vmName != "" {
		metadata["vm_name"] = p.vmName
	}
	if p.location != "" {
		metadata["location"] = p.location
	}

	return identity.ProviderInfo{
		Type:            identity.ProviderTypeAzure,
		Status:          p.status,
		TrustDomain:     p.config.TrustDomain,
		Capabilities:    []string{"managed_identity", "vm_identity", "access_token"},
		LastHealthCheck: p.lastHealthCheck,
		ErrorMessage:    p.statusMessage,
		Metadata:        metadata,
	}
}

// TrustDomain returns the trust domain.
func (p *AzureProvider) TrustDomain() string {
	return p.config.TrustDomain
}

// GetTrustBundle returns the current trust bundle.
func (p *AzureProvider) GetTrustBundle(ctx context.Context) (*identity.TrustBundle, error) {
	p.mu.RLock()
	bundle := p.trustBundle
	p.mu.RUnlock()

	if bundle != nil {
		return bundle, nil
	}

	// Azure doesn't provide a trust bundle like SPIFFE
	return &identity.TrustBundle{
		TrustDomain: p.config.TrustDomain,
		UpdatedAt:   time.Now(),
	}, nil
}

// WatchTrustBundle watches for trust bundle updates.
func (p *AzureProvider) WatchTrustBundle(ctx context.Context) (<-chan *identity.TrustBundle, error) {
	ch := make(chan *identity.TrustBundle, 1)
	// Azure doesn't have dynamic trust bundle updates
	return ch, nil
}

// GetInstanceMetadata returns the Azure instance metadata.
func (p *AzureProvider) GetInstanceMetadata(ctx context.Context) (*AzureInstanceMetadata, error) {
	url := p.config.IMDSEndpoint + "/metadata/instance?api-version=2021-02-01"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Metadata", "true")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IMDS returned status %d", resp.StatusCode)
	}

	var meta AzureInstanceMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("failed to decode instance metadata: %w", err)
	}

	return &meta, nil
}

// GetAccessToken returns an Azure access token for the managed identity.
func (p *AzureProvider) GetAccessToken(ctx context.Context, resource string) (*AzureAccessToken, error) {
	if resource == "" {
		resource = p.config.Resource
	}

	url := fmt.Sprintf("%s/metadata/identity/oauth2/token?api-version=2018-02-01&resource=%s",
		p.config.IMDSEndpoint, resource)

	// Add client_id if using user-assigned managed identity
	if p.config.ClientID != "" {
		url += "&client_id=" + p.config.ClientID
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Metadata", "true")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IMDS returned status %d: %s", resp.StatusCode, string(body))
	}

	var token AzureAccessToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("failed to decode access token: %w", err)
	}

	return &token, nil
}

// GetIdentityToken returns an Azure identity token for the given audience.
// This uses the AAD identity endpoint.
func (p *AzureProvider) GetIdentityToken(ctx context.Context, audience string) (string, error) {
	token, err := p.GetAccessToken(ctx, audience)
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

// GetSPIFFEID returns the SPIFFE ID for this Azure instance.
func (p *AzureProvider) GetSPIFFEID() (identity.SPIFFEID, error) {
	p.mu.RLock()
	subscriptionID := p.subscriptionID
	resourceGroup := p.resourceGroup
	vmID := p.vmID
	p.mu.RUnlock()

	if subscriptionID == "" {
		return identity.SPIFFEID{}, fmt.Errorf("instance not detected")
	}

	// SPIFFE ID format: spiffe://trust-domain/azure/subscription-id/resource-group/vm-id
	path := fmt.Sprintf("/azure/%s/%s/%s", subscriptionID, resourceGroup, vmID)
	return identity.SPIFFEID{
		TrustDomain: p.config.TrustDomain,
		Path:        path,
	}, nil
}

// IsManagedIdentityAvailable returns true if Azure Managed Identity is available.
func (p *AzureProvider) IsManagedIdentityAvailable() bool {
	// Check for Azure environment variables
	if os.Getenv("IDENTITY_ENDPOINT") != "" {
		return true
	}

	// Try to get a token (quick check)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := p.GetAccessToken(ctx, p.config.Resource)
	return err == nil
}

// IsVMScaleSet returns true if running in a VM Scale Set.
func (p *AzureProvider) IsVMScaleSet() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.vmScaleSetName != ""
}

// CreateAttestationEvidence creates attestation evidence for Azure.
func (p *AzureProvider) CreateAttestationEvidence(ctx context.Context) (*identity.AttestationEvidence, error) {
	// Get access token as attestation evidence
	token, err := p.GetAccessToken(ctx, p.config.Resource)
	if err != nil {
		return nil, err
	}

	evidenceData, err := json.Marshal(token)
	if err != nil {
		return nil, err
	}

	p.mu.RLock()
	metadata := map[string]string{
		"subscription_id": p.subscriptionID,
	}
	if p.resourceGroup != "" {
		metadata["resource_group"] = p.resourceGroup
	}
	if p.vmID != "" {
		metadata["vm_id"] = p.vmID
	}
	p.mu.RUnlock()

	return &identity.AttestationEvidence{
		Type:     identity.AttestationTypeAzureIMDS,
		Data:     evidenceData,
		Metadata: metadata,
	}, nil
}

// Private methods

func (p *AzureProvider) detectEnvironment(ctx context.Context) error {
	meta, err := p.GetInstanceMetadata(ctx)
	if err != nil {
		// May not be on Azure, check for managed identity
		if p.IsManagedIdentityAvailable() {
			p.mu.Lock()
			p.subscriptionID = p.config.SubscriptionID
			p.resourceGroup = p.config.ResourceGroup
			p.tenantID = p.config.TenantID
			p.mu.Unlock()
			return nil
		}
		return fmt.Errorf("failed to detect Azure environment: %w", err)
	}

	p.mu.Lock()
	p.subscriptionID = meta.Compute.SubscriptionID
	p.resourceGroup = meta.Compute.ResourceGroupName
	p.vmID = meta.Compute.VMID
	p.vmName = meta.Compute.Name
	p.location = meta.Compute.Location
	p.vmScaleSetName = meta.Compute.VMScaleSetName
	p.mu.Unlock()

	// Get tenant ID from token
	token, err := p.GetAccessToken(ctx, p.config.Resource)
	if err == nil && token.ClientID != "" {
		// We could parse the token to get tenant ID, but it's complex
		// For now, use environment variable or config
		p.mu.Lock()
		p.tenantID = p.config.TenantID
		if p.tenantID == "" {
			p.tenantID = os.Getenv("AZURE_TENANT_ID")
		}
		p.mu.Unlock()
	}

	return nil
}

func (p *AzureProvider) healthCheckLoop(ctx context.Context) {
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

func (p *AzureProvider) performHealthCheck(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, p.config.IMDSTimeout)
	defer cancel()

	_, err := p.GetInstanceMetadata(checkCtx)

	p.mu.Lock()
	p.lastHealthCheck = time.Now()

	if err != nil {
		if p.IsManagedIdentityAvailable() {
			p.status = identity.ProviderStatusDegraded
			p.statusMessage = "IMDS unavailable, using managed identity"
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

// Verify AzureProvider implements IdentityProvider
var _ identity.IdentityProvider = (*AzureProvider)(nil)
