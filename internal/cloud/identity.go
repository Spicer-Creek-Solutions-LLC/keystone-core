package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// IdentityInfo contains cloud identity/credential information
type IdentityInfo struct {
	// Provider is the cloud provider
	Provider Provider

	// IdentityType describes the type of identity (IAM role, managed identity, service account)
	IdentityType string

	// ARN/ID of the identity (AWS ARN, Azure Object ID, GCP email)
	IdentityID string

	// Name/email of the identity
	IdentityName string

	// Account/subscription/project this identity belongs to
	AccountID string

	// Scopes/permissions (if available)
	Scopes []string

	// AccessToken (if retrieved)
	AccessToken string

	// TokenExpiry when the token expires
	TokenExpiry time.Time

	// Metadata additional identity metadata
	Metadata map[string]string
}

// IdentityProvider retrieves cloud identity information
type IdentityProvider interface {
	// GetIdentity retrieves the current identity information
	GetIdentity(ctx context.Context) (*IdentityInfo, error)

	// GetAccessToken retrieves an access token for the given scope/audience
	GetAccessToken(ctx context.Context, scope string) (string, time.Time, error)
}

// AWSIdentityProvider retrieves AWS identity information
type AWSIdentityProvider struct {
	httpClient *http.Client
}

// NewAWSIdentityProvider creates a new AWS identity provider
func NewAWSIdentityProvider(timeout time.Duration) *AWSIdentityProvider {
	return &AWSIdentityProvider{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// GetIdentity retrieves AWS identity information from instance metadata
func (p *AWSIdentityProvider) GetIdentity(ctx context.Context) (*IdentityInfo, error) {
	// Get IMDSv2 token
	token, err := p.getIMDSv2Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get IMDSv2 token: %w", err)
	}

	// Get IAM role info
	roleName, err := p.getMetadata(ctx, token, "/iam/security-credentials/")
	if err != nil {
		return nil, fmt.Errorf("failed to get IAM role: %w", err)
	}

	roleName = strings.TrimSpace(roleName)
	if roleName == "" {
		return nil, fmt.Errorf("no IAM role attached")
	}

	// Get credentials for the role
	credJSON, err := p.getMetadata(ctx, token, "/iam/security-credentials/"+roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	var creds struct {
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		Token           string `json:"Token"`
		Expiration      string `json:"Expiration"`
	}
	if err := json.Unmarshal([]byte(credJSON), &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	// Get instance profile ARN
	iamInfo, _ := p.getMetadata(ctx, token, "/iam/info")
	var profileARN string
	if iamInfo != "" {
		var info struct {
			InstanceProfileARN string `json:"InstanceProfileArn"`
		}
		if err := json.Unmarshal([]byte(iamInfo), &info); err == nil {
			profileARN = info.InstanceProfileARN
		}
	}

	identity := &IdentityInfo{
		Provider:     ProviderAWS,
		IdentityType: "iam-role",
		IdentityID:   profileARN,
		IdentityName: roleName,
		AccessToken:  creds.Token,
		Metadata: map[string]string{
			"access_key_id": creds.AccessKeyID,
		},
	}

	if creds.Expiration != "" {
		expiry, _ := time.Parse(time.RFC3339, creds.Expiration)
		identity.TokenExpiry = expiry
	}

	return identity, nil
}

// GetAccessToken retrieves an AWS session token
func (p *AWSIdentityProvider) GetAccessToken(ctx context.Context, scope string) (string, time.Time, error) {
	identity, err := p.GetIdentity(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	return identity.AccessToken, identity.TokenExpiry, nil
}

func (p *AWSIdentityProvider) getIMDSv2Token(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "PUT", awsMetadataToken, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get token: %d", resp.StatusCode)
	}

	token, err := io.ReadAll(resp.Body)
	return string(token), err
}

func (p *AWSIdentityProvider) getMetadata(ctx context.Context, token, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", awsMetadataBaseURL+path, http.NoBody)
	if err != nil {
		return "", err
	}
	if token != "" {
		req.Header.Set("X-aws-ec2-metadata-token", token)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata request failed: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	return string(data), err
}

// GCPIdentityProvider retrieves GCP identity information
type GCPIdentityProvider struct {
	httpClient *http.Client
}

// NewGCPIdentityProvider creates a new GCP identity provider
func NewGCPIdentityProvider(timeout time.Duration) *GCPIdentityProvider {
	return &GCPIdentityProvider{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// GetIdentity retrieves GCP service account information
func (p *GCPIdentityProvider) GetIdentity(ctx context.Context) (*IdentityInfo, error) {
	// Get service account email
	email, err := p.getMetadata(ctx, "/instance/service-accounts/default/email")
	if err != nil {
		return nil, fmt.Errorf("failed to get service account email: %w", err)
	}

	// Get scopes
	scopesStr, err := p.getMetadata(ctx, "/instance/service-accounts/default/scopes")
	var scopes []string
	if err == nil {
		scopes = strings.Split(strings.TrimSpace(scopesStr), "\n")
	}

	// Get project ID
	projectID, _ := p.getMetadata(ctx, "/project/project-id")

	identity := &IdentityInfo{
		Provider:     ProviderGCP,
		IdentityType: "service-account",
		IdentityID:   email,
		IdentityName: email,
		AccountID:    projectID,
		Scopes:       scopes,
		Metadata:     make(map[string]string),
	}

	// Get numeric project ID
	if numericID, err := p.getMetadata(ctx, "/project/numeric-project-id"); err == nil {
		identity.Metadata["numeric_project_id"] = numericID
	}

	return identity, nil
}

// GetAccessToken retrieves a GCP access token for the given audience
func (p *GCPIdentityProvider) GetAccessToken(ctx context.Context, audience string) (string, time.Time, error) {
	var url string
	if audience == "" {
		// Standard access token
		url = gcpMetadataBaseURL + "/instance/service-accounts/default/token"
	} else {
		// Identity token for specific audience
		url = gcpMetadataBaseURL + "/instance/service-accounts/default/identity?audience=" + audience
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("token request failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, err
	}

	if audience == "" {
		// Parse access token response
		var tokenResp struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
			TokenType   string `json:"token_type"`
		}
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return "", time.Time{}, err
		}
		expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		return tokenResp.AccessToken, expiry, nil
	}

	// Identity token is returned directly
	return string(body), time.Time{}, nil
}

func (p *GCPIdentityProvider) getMetadata(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", gcpMetadataBaseURL+path, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata request failed: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(data)), err
}

// AzureIdentityProvider retrieves Azure managed identity information
type AzureIdentityProvider struct {
	httpClient *http.Client
}

// NewAzureIdentityProvider creates a new Azure identity provider
func NewAzureIdentityProvider(timeout time.Duration) *AzureIdentityProvider {
	return &AzureIdentityProvider{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// GetIdentity retrieves Azure managed identity information
func (p *AzureIdentityProvider) GetIdentity(ctx context.Context) (*IdentityInfo, error) {
	// Get managed identity token to extract identity info
	token, expiry, err := p.GetAccessToken(ctx, "https://management.azure.com/")
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// Get VM metadata for subscription info
	metadata, err := p.getInstanceMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance metadata: %w", err)
	}

	identity := &IdentityInfo{
		Provider:     ProviderAzure,
		IdentityType: "managed-identity",
		AccountID:    metadata.SubscriptionID,
		AccessToken:  token,
		TokenExpiry:  expiry,
		Metadata: map[string]string{
			"resource_group": metadata.ResourceGroupName,
			"vm_name":        metadata.VMName,
		},
	}

	// Try to get identity object ID from token claims
	// (In production, would decode the JWT to get object_id)
	identity.IdentityID = metadata.VMID

	return identity, nil
}

// GetAccessToken retrieves an Azure managed identity access token
func (p *AzureIdentityProvider) GetAccessToken(ctx context.Context, resource string) (string, time.Time, error) {
	url := fmt.Sprintf("%s/identity/oauth2/token?api-version=2018-02-01&resource=%s",
		azureMetadataBaseURL, resource)

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Metadata", "true")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("token request failed: %d - %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
		ExpiresOn   string `json:"expires_on"`
		Resource    string `json:"resource"`
		TokenType   string `json:"token_type"`
		ClientID    string `json:"client_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", time.Time{}, err
	}

	var expiry time.Time
	if tokenResp.ExpiresOn != "" {
		var expiresOnSec int64
		fmt.Sscanf(tokenResp.ExpiresOn, "%d", &expiresOnSec)
		expiry = time.Unix(expiresOnSec, 0)
	}

	return tokenResp.AccessToken, expiry, nil
}

type azureVMMetadataSimple struct {
	SubscriptionID    string `json:"subscriptionId"`
	ResourceGroupName string `json:"resourceGroupName"`
	VMName            string `json:"name"`
	VMID              string `json:"vmId"`
}

func (p *AzureIdentityProvider) getInstanceMetadata(ctx context.Context) (*azureVMMetadataSimple, error) {
	url := azureMetadataBaseURL + "/instance/compute?api-version=" + azureMetadataVersion

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Metadata", "true")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata request failed: %d", resp.StatusCode)
	}

	var metadata azureVMMetadataSimple
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// GetIdentityProvider returns the appropriate identity provider for the detected cloud
func GetIdentityProvider(provider Provider, timeout time.Duration) IdentityProvider {
	switch provider {
	case ProviderAWS:
		return NewAWSIdentityProvider(timeout)
	case ProviderGCP:
		return NewGCPIdentityProvider(timeout)
	case ProviderAzure:
		return NewAzureIdentityProvider(timeout)
	default:
		return nil
	}
}

// AttestationInfo contains attestation data for identity verification
type AttestationInfo struct {
	// Provider is the cloud provider
	Provider Provider

	// Document is the attestation document/token
	Document string

	// DocumentType describes the document type (signed-identity-document, instance-identity-document, etc.)
	DocumentType string

	// Signature for the document (if separate)
	Signature string

	// PKCS7 signed document (if applicable)
	PKCS7 string

	// Nonce used in the attestation request
	Nonce string

	// Timestamp when attestation was retrieved
	Timestamp time.Time
}

// AttestationProvider retrieves cloud attestation documents
type AttestationProvider interface {
	// GetAttestation retrieves an attestation document
	GetAttestation(ctx context.Context, nonce string) (*AttestationInfo, error)
}

// AWSAttestationProvider retrieves AWS attestation documents
type AWSAttestationProvider struct {
	httpClient *http.Client
}

// NewAWSAttestationProvider creates a new AWS attestation provider
func NewAWSAttestationProvider(timeout time.Duration) *AWSAttestationProvider {
	return &AWSAttestationProvider{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// GetAttestation retrieves AWS instance identity document
func (p *AWSAttestationProvider) GetAttestation(ctx context.Context, nonce string) (*AttestationInfo, error) {
	// Get IMDSv2 token
	tokenReq, err := http.NewRequestWithContext(ctx, "PUT", awsMetadataToken, http.NoBody)
	if err != nil {
		return nil, err
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")

	tokenResp, err := p.httpClient.Do(tokenReq)
	if err != nil {
		return nil, err
	}
	defer tokenResp.Body.Close()

	token, _ := io.ReadAll(tokenResp.Body)

	// Get identity document
	docReq, err := http.NewRequestWithContext(ctx, "GET", awsDynamicURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	docReq.Header.Set("X-aws-ec2-metadata-token", string(token))

	docResp, err := p.httpClient.Do(docReq)
	if err != nil {
		return nil, err
	}
	defer docResp.Body.Close()

	doc, err := io.ReadAll(docResp.Body)
	if err != nil {
		return nil, err
	}

	// Get PKCS7 signature
	pkcs7Req, err := http.NewRequestWithContext(ctx, "GET",
		"http://169.254.169.254/latest/dynamic/instance-identity/pkcs7", http.NoBody)
	if err != nil {
		return nil, err
	}
	pkcs7Req.Header.Set("X-aws-ec2-metadata-token", string(token))

	pkcs7Resp, err := p.httpClient.Do(pkcs7Req)
	var pkcs7 string
	if err == nil {
		defer pkcs7Resp.Body.Close()
		pkcs7Data, _ := io.ReadAll(pkcs7Resp.Body)
		pkcs7 = string(pkcs7Data)
	}

	// Get RSA2048 signature
	sigReq, err := http.NewRequestWithContext(ctx, "GET",
		"http://169.254.169.254/latest/dynamic/instance-identity/rsa2048", http.NoBody)
	if err != nil {
		return nil, err
	}
	sigReq.Header.Set("X-aws-ec2-metadata-token", string(token))

	sigResp, err := p.httpClient.Do(sigReq)
	var sig string
	if err == nil {
		defer sigResp.Body.Close()
		sigData, _ := io.ReadAll(sigResp.Body)
		sig = string(sigData)
	}

	return &AttestationInfo{
		Provider:     ProviderAWS,
		Document:     string(doc),
		DocumentType: "instance-identity-document",
		Signature:    sig,
		PKCS7:        pkcs7,
		Nonce:        nonce,
		Timestamp:    time.Now(),
	}, nil
}

// GCPAttestationProvider retrieves GCP attestation tokens
type GCPAttestationProvider struct {
	httpClient *http.Client
}

// NewGCPAttestationProvider creates a new GCP attestation provider
func NewGCPAttestationProvider(timeout time.Duration) *GCPAttestationProvider {
	return &GCPAttestationProvider{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// GetAttestation retrieves a GCP identity token for attestation
func (p *GCPAttestationProvider) GetAttestation(ctx context.Context, audience string) (*AttestationInfo, error) {
	if audience == "" {
		audience = "keystone"
	}

	url := gcpMetadataBaseURL + "/instance/service-accounts/default/identity?audience=" + audience + "&format=full"

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("attestation request failed: %d", resp.StatusCode)
	}

	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &AttestationInfo{
		Provider:     ProviderGCP,
		Document:     string(token),
		DocumentType: "identity-token",
		Nonce:        audience,
		Timestamp:    time.Now(),
	}, nil
}

// AzureAttestationProvider retrieves Azure attestation tokens
type AzureAttestationProvider struct {
	httpClient *http.Client
}

// NewAzureAttestationProvider creates a new Azure attestation provider
func NewAzureAttestationProvider(timeout time.Duration) *AzureAttestationProvider {
	return &AzureAttestationProvider{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// GetAttestation retrieves an Azure managed identity token for attestation
func (p *AzureAttestationProvider) GetAttestation(ctx context.Context, resource string) (*AttestationInfo, error) {
	if resource == "" {
		resource = "https://management.azure.com/"
	}

	url := fmt.Sprintf("%s/identity/oauth2/token?api-version=2018-02-01&resource=%s",
		azureMetadataBaseURL, resource)

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Metadata", "true")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("attestation request failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	return &AttestationInfo{
		Provider:     ProviderAzure,
		Document:     tokenResp.AccessToken,
		DocumentType: "managed-identity-token",
		Nonce:        resource,
		Timestamp:    time.Now(),
	}, nil
}

// GetAttestationProvider returns the appropriate attestation provider for the detected cloud
func GetAttestationProvider(provider Provider, timeout time.Duration) AttestationProvider {
	switch provider {
	case ProviderAWS:
		return NewAWSAttestationProvider(timeout)
	case ProviderGCP:
		return NewGCPAttestationProvider(timeout)
	case ProviderAzure:
		return NewAzureAttestationProvider(timeout)
	default:
		return nil
	}
}
