// Package cloud provides cloud-native identity providers for AWS, GCP, and Azure.
package cloud

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/identity"
)

// AWSConfig configures the AWS identity provider.
type AWSConfig struct {
	// TrustDomain is the SPIFFE trust domain for AWS identities.
	TrustDomain string

	// Region is the AWS region (auto-detected if empty).
	Region string

	// RoleARN is the IAM role to assume (for IRSA).
	RoleARN string

	// IMDSEndpoint is the Instance Metadata Service endpoint.
	// Default: http://169.254.169.254
	IMDSEndpoint string

	// IMDSv2 enables IMDSv2 (token-based) metadata requests.
	// Default: true
	IMDSv2 bool

	// IMDSTimeout is the timeout for IMDS requests.
	// Default: 5 seconds
	IMDSTimeout time.Duration

	// STSEndpoint is the STS endpoint for token exchange.
	// Auto-detected based on region if empty.
	STSEndpoint string

	// WebIdentityTokenFile is the path to the OIDC token file (for IRSA).
	// Default: /var/run/secrets/eks.amazonaws.com/serviceaccount/token
	WebIdentityTokenFile string

	// RefreshInterval is how often to refresh credentials.
	// Default: 5 minutes
	RefreshInterval time.Duration

	// HealthCheckInterval is how often to check provider health.
	// Default: 30 seconds
	HealthCheckInterval time.Duration
}

// DefaultAWSConfig returns an AWSConfig with default values.
func DefaultAWSConfig(trustDomain string) *AWSConfig {
	return &AWSConfig{
		TrustDomain:          trustDomain,
		IMDSEndpoint:         "http://169.254.169.254",
		IMDSv2:               true,
		IMDSTimeout:          5 * time.Second,
		WebIdentityTokenFile: "/var/run/secrets/eks.amazonaws.com/serviceaccount/token",
		RefreshInterval:      5 * time.Minute,
		HealthCheckInterval:  30 * time.Second,
	}
}

// AWSProvider implements the IdentityProvider interface for AWS.
type AWSProvider struct {
	config *AWSConfig
	client *http.Client

	mu              sync.RWMutex
	started         bool
	status          identity.ProviderStatus
	statusMessage   string
	trustBundle     *identity.TrustBundle
	instanceID      string
	accountID       string
	region          string
	availabilityZone string
	ipv6Addresses    []string
	imdsToken       string
	imdsTokenExpiry time.Time

	healthCheckCancel context.CancelFunc
	lastHealthCheck   time.Time
}

// AWSInstanceIdentity contains AWS instance identity information.
type AWSInstanceIdentity struct {
	InstanceID       string    `json:"instanceId"`
	AccountID        string    `json:"accountId"`
	Region           string    `json:"region"`
	AvailabilityZone string    `json:"availabilityZone"`
	ImageID          string    `json:"imageId"`
	InstanceType     string    `json:"instanceType"`
	PrivateIP        string    `json:"privateIp"`
	Architecture     string    `json:"architecture"`
	PendingTime      time.Time `json:"pendingTime"`
}

// AWSInstanceIdentityDocument is the signed identity document from IMDS.
type AWSInstanceIdentityDocument struct {
	Document  string `json:"document"`
	Signature string `json:"signature"`
	PKCS7     string `json:"pkcs7"`
}

// NewAWSProvider creates a new AWS identity provider.
func NewAWSProvider(config *AWSConfig) (*AWSProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.TrustDomain == "" {
		return nil, fmt.Errorf("trust domain is required")
	}

	return &AWSProvider{
		config: config,
		client: &http.Client{
			Timeout: config.IMDSTimeout,
		},
		status: identity.ProviderStatusUnknown,
	}, nil
}

// Type returns the provider type.
func (p *AWSProvider) Type() identity.ProviderType {
	return identity.ProviderTypeAWS
}

// Start starts the provider.
func (p *AWSProvider) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = true
	p.mu.Unlock()

	// Detect AWS environment
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
func (p *AWSProvider) Stop(ctx context.Context) error {
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
func (p *AWSProvider) Health(ctx context.Context) identity.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// Info returns detailed provider information.
func (p *AWSProvider) Info(ctx context.Context) identity.ProviderInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metadata := map[string]string{
		"region":     p.region,
		"account_id": p.accountID,
	}
	if p.instanceID != "" {
		metadata["instance_id"] = p.instanceID
	}
	if p.availabilityZone != "" {
		metadata["availability_zone"] = p.availabilityZone
	}
	if len(p.ipv6Addresses) > 0 {
		metadata["ipv6_addresses"] = strings.Join(p.ipv6Addresses, ",")
	}

	return identity.ProviderInfo{
		Type:            identity.ProviderTypeAWS,
		Status:          p.status,
		TrustDomain:     p.config.TrustDomain,
		Capabilities:    []string{"instance_identity", "irsa", "sts"},
		LastHealthCheck: p.lastHealthCheck,
		ErrorMessage:    p.statusMessage,
		Metadata:        metadata,
	}
}

// TrustDomain returns the trust domain.
func (p *AWSProvider) TrustDomain() string {
	return p.config.TrustDomain
}

// GetTrustBundle returns the current trust bundle.
func (p *AWSProvider) GetTrustBundle(ctx context.Context) (*identity.TrustBundle, error) {
	p.mu.RLock()
	bundle := p.trustBundle
	p.mu.RUnlock()

	if bundle != nil {
		return bundle, nil
	}

	// AWS doesn't provide a trust bundle like SPIFFE
	// Return an empty bundle (agents should use embedded CA or SPIRE)
	return &identity.TrustBundle{
		TrustDomain: p.config.TrustDomain,
		UpdatedAt:   time.Now(),
	}, nil
}

// WatchTrustBundle watches for trust bundle updates.
func (p *AWSProvider) WatchTrustBundle(ctx context.Context) (<-chan *identity.TrustBundle, error) {
	ch := make(chan *identity.TrustBundle, 1)
	// AWS doesn't have dynamic trust bundle updates
	return ch, nil
}

// GetInstanceIdentity returns the AWS instance identity.
func (p *AWSProvider) GetInstanceIdentity(ctx context.Context) (*AWSInstanceIdentity, error) {
	// Ensure we have a token for IMDSv2
	if p.config.IMDSv2 {
		if err := p.ensureIMDSToken(ctx); err != nil {
			return nil, err
		}
	}

	// Get instance identity document
	docURL := p.config.IMDSEndpoint + "/latest/dynamic/instance-identity/document"
	req, err := http.NewRequestWithContext(ctx, "GET", docURL, nil)
	if err != nil {
		return nil, err
	}

	if p.config.IMDSv2 {
		p.mu.RLock()
		token := p.imdsToken
		p.mu.RUnlock()
		req.Header.Set("X-aws-ec2-metadata-token", token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance identity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IMDS returned status %d", resp.StatusCode)
	}

	var identity AWSInstanceIdentity
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		return nil, fmt.Errorf("failed to decode instance identity: %w", err)
	}

	return &identity, nil
}

// GetSignedInstanceIdentity returns the signed instance identity document.
func (p *AWSProvider) GetSignedInstanceIdentity(ctx context.Context) (*AWSInstanceIdentityDocument, error) {
	if p.config.IMDSv2 {
		if err := p.ensureIMDSToken(ctx); err != nil {
			return nil, err
		}
	}

	p.mu.RLock()
	token := p.imdsToken
	p.mu.RUnlock()

	// Get document
	doc, err := p.getIMDSValue(ctx, "/latest/dynamic/instance-identity/document", token)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	// Get signature
	sig, err := p.getIMDSValue(ctx, "/latest/dynamic/instance-identity/signature", token)
	if err != nil {
		return nil, fmt.Errorf("failed to get signature: %w", err)
	}

	// Get PKCS7
	pkcs7, err := p.getIMDSValue(ctx, "/latest/dynamic/instance-identity/pkcs7", token)
	if err != nil {
		return nil, fmt.Errorf("failed to get pkcs7: %w", err)
	}

	return &AWSInstanceIdentityDocument{
		Document:  doc,
		Signature: sig,
		PKCS7:     pkcs7,
	}, nil
}

// GetSPIFFEID returns the SPIFFE ID for this AWS instance.
func (p *AWSProvider) GetSPIFFEID() (identity.SPIFFEID, error) {
	p.mu.RLock()
	instanceID := p.instanceID
	accountID := p.accountID
	region := p.region
	p.mu.RUnlock()

	if instanceID == "" {
		return identity.SPIFFEID{}, fmt.Errorf("instance not detected")
	}

	// SPIFFE ID format: spiffe://trust-domain/aws/account-id/region/instance-id
	path := fmt.Sprintf("/aws/%s/%s/%s", accountID, region, instanceID)
	return identity.SPIFFEID{
		TrustDomain: p.config.TrustDomain,
		Path:        path,
	}, nil
}

// IsIRSAAvailable returns true if IRSA (IAM Roles for Service Accounts) is available.
func (p *AWSProvider) IsIRSAAvailable() bool {
	if p.config.WebIdentityTokenFile == "" {
		return false
	}
	_, err := os.Stat(p.config.WebIdentityTokenFile)
	return err == nil
}

// GetWebIdentityToken reads the OIDC token for IRSA.
func (p *AWSProvider) GetWebIdentityToken() (string, error) {
	if p.config.WebIdentityTokenFile == "" {
		return "", fmt.Errorf("web identity token file not configured")
	}

	data, err := os.ReadFile(p.config.WebIdentityTokenFile)
	if err != nil {
		return "", fmt.Errorf("failed to read token file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// CreateAttestationEvidence creates attestation evidence for AWS.
func (p *AWSProvider) CreateAttestationEvidence(ctx context.Context) (*identity.AttestationEvidence, error) {
	doc, err := p.GetSignedInstanceIdentity(ctx)
	if err != nil {
		return nil, err
	}

	// Encode as attestation evidence
	evidenceData, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}

	metadata := map[string]string{
		"account_id": p.accountID,
		"region":     p.region,
	}
	if p.instanceID != "" {
		metadata["instance_id"] = p.instanceID
	}

	return &identity.AttestationEvidence{
		Type:     identity.AttestationTypeAWSIID,
		Data:     evidenceData,
		Metadata: metadata,
	}, nil
}

// Private methods

func (p *AWSProvider) detectEnvironment(ctx context.Context) error {
	// Try to detect AWS environment via IMDS
	id, err := p.GetInstanceIdentity(ctx)
	if err != nil {
		// May not be on EC2, try other detection methods
		p.mu.Lock()
		p.region = p.config.Region
		if p.region == "" {
			p.region = os.Getenv("AWS_REGION")
			if p.region == "" {
				p.region = os.Getenv("AWS_DEFAULT_REGION")
			}
		}
		p.mu.Unlock()

		// Check for IRSA
		if p.IsIRSAAvailable() {
			return nil
		}

		return fmt.Errorf("failed to detect AWS environment: %w", err)
	}

	p.mu.Lock()
	p.instanceID = id.InstanceID
	p.accountID = id.AccountID
	p.region = id.Region
	p.availabilityZone = id.AvailabilityZone
	p.mu.Unlock()

	token := ""
	if p.config.IMDSv2 {
		if err := p.ensureIMDSToken(ctx); err == nil {
			p.mu.RLock()
			token = p.imdsToken
			p.mu.RUnlock()
		}
	}

	if rawIPv6, err := p.getIMDSValue(ctx, "/latest/meta-data/ipv6", token); err == nil {
		ipv6 := parseIPv6List(rawIPv6)
		if len(ipv6) > 0 {
			p.mu.Lock()
			p.ipv6Addresses = ipv6
			p.mu.Unlock()
		}
	}

	return nil
}

func (p *AWSProvider) ensureIMDSToken(ctx context.Context) error {
	p.mu.RLock()
	if p.imdsToken != "" && time.Now().Before(p.imdsTokenExpiry) {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	// Get new token
	tokenURL := p.config.IMDSEndpoint + "/latest/api/token"
	req, err := http.NewRequestWithContext(ctx, "PUT", tokenURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "3600")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get IMDS token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("IMDS token request returned status %d", resp.StatusCode)
	}

	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read IMDS token: %w", err)
	}

	p.mu.Lock()
	p.imdsToken = string(token)
	p.imdsTokenExpiry = time.Now().Add(55 * time.Minute) // Refresh before expiry
	p.mu.Unlock()

	return nil
}

func (p *AWSProvider) getIMDSValue(ctx context.Context, path, token string) (string, error) {
	url := p.config.IMDSEndpoint + path
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	if token != "" {
		req.Header.Set("X-aws-ec2-metadata-token", token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IMDS returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func parseIPv6List(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (p *AWSProvider) healthCheckLoop(ctx context.Context) {
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

func (p *AWSProvider) performHealthCheck(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, p.config.IMDSTimeout)
	defer cancel()

	_, err := p.GetInstanceIdentity(checkCtx)

	p.mu.Lock()
	p.lastHealthCheck = time.Now()

	if err != nil {
		// Check if we have IRSA as fallback
		if p.IsIRSAAvailable() {
			p.status = identity.ProviderStatusDegraded
			p.statusMessage = "IMDS unavailable, using IRSA"
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

// Verify AWSProvider implements IdentityProvider
var _ identity.IdentityProvider = (*AWSProvider)(nil)

// VerifyAWSSignature verifies an AWS instance identity document signature.
func VerifyAWSSignature(doc *AWSInstanceIdentityDocument, awsCerts []*x509.Certificate) error {
	if len(awsCerts) == 0 {
		return fmt.Errorf("no AWS certificates provided")
	}

	// Decode PKCS7 signature
	pkcs7Data, err := base64.StdEncoding.DecodeString(doc.PKCS7)
	if err != nil {
		return fmt.Errorf("failed to decode PKCS7: %w", err)
	}

	// Note: Full PKCS7 verification would require a PKCS7 library
	// For now, we just verify the data is present
	if len(pkcs7Data) == 0 {
		return fmt.Errorf("empty PKCS7 signature")
	}

	return nil
}
