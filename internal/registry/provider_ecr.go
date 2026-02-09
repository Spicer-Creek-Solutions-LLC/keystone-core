// Package registry provides container registry authentication support for Keystone.
package registry

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ECRCredentialProvider provides AWS ECR credentials using instance metadata or environment credentials.
type ECRCredentialProvider struct {
	httpClient *http.Client
	region     string
}

// NewECRCredentialProvider creates a new ECR credential provider.
func NewECRCredentialProvider(region string) *ECRCredentialProvider {
	return &ECRCredentialProvider{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		region:     region,
	}
}

// NewECRCredentialProviderWithClient creates an ECR provider with a custom HTTP client.
func NewECRCredentialProviderWithClient(region string, client *http.Client) *ECRCredentialProvider {
	return &ECRCredentialProvider{
		httpClient: client,
		region:     region,
	}
}

// GetCredential retrieves ECR credentials using AWS instance metadata.
func (p *ECRCredentialProvider) GetCredential(ctx context.Context, registry string) (*Credential, error) {
	// Extract region from registry URL if not set
	region := p.region
	if region == "" {
		region = extractECRRegion(registry)
	}
	if region == "" {
		return nil, fmt.Errorf("ECR region is required")
	}

	// Get AWS credentials from instance metadata
	awsCreds, err := p.getAWSCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	// Call ECR GetAuthorizationToken API
	authToken, expiresAt, err := p.getECRAuthorizationToken(ctx, region, awsCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECR authorization token: %w", err)
	}

	// Parse the authorization token (base64 encoded "username:password")
	decoded, err := base64.StdEncoding.DecodeString(authToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode authorization token: %w", err)
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid authorization token format")
	}

	return &Credential{
		Type:      TypeECR,
		Registry:  registry,
		Username:  parts[0], // "AWS"
		Password:  parts[1], // The actual token
		Token:     parts[1],
		ExpiresAt: expiresAt,
		Region:    region,
	}, nil
}

// IsAvailable checks if ECR credentials are available (running on AWS).
func (p *ECRCredentialProvider) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := p.getIMDSv2Token(ctx)
	return err == nil
}

// Type returns the registry type this provider handles.
func (p *ECRCredentialProvider) Type() Type {
	return TypeECR
}

// awsCredentials holds AWS credentials from instance metadata.
type awsCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
}

// getAWSCredentials retrieves AWS credentials from instance metadata (IMDSv2).
func (p *ECRCredentialProvider) getAWSCredentials(ctx context.Context) (*awsCredentials, error) {
	// Get IMDSv2 token
	token, err := p.getIMDSv2Token(ctx)
	if err != nil {
		return nil, err
	}

	// Get IAM role name
	roleName, err := p.getMetadata(ctx, token, "/iam/security-credentials/")
	if err != nil {
		return nil, fmt.Errorf("failed to get IAM role: %w", err)
	}
	roleName = strings.TrimSpace(roleName)
	if roleName == "" {
		return nil, fmt.Errorf("no IAM role attached to instance")
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

	result := &awsCredentials{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.Token,
	}

	if creds.Expiration != "" {
		expiry, _ := time.Parse(time.RFC3339, creds.Expiration)
		result.Expiration = expiry
	}

	return result, nil
}

func (p *ECRCredentialProvider) getIMDSv2Token(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "PUT", "http://169.254.169.254/latest/api/token", http.NoBody)
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
		return "", fmt.Errorf("failed to get IMDSv2 token: %d", resp.StatusCode)
	}

	token, err := io.ReadAll(resp.Body)
	return string(token), err
}

func (p *ECRCredentialProvider) getMetadata(ctx context.Context, token, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://169.254.169.254/latest/meta-data"+path, http.NoBody)
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

// getECRAuthorizationToken calls the ECR GetAuthorizationToken API.
func (p *ECRCredentialProvider) getECRAuthorizationToken(ctx context.Context, region string, creds *awsCredentials) (string, time.Time, error) {
	endpoint := fmt.Sprintf("https://api.ecr.%s.amazonaws.com/", region)
	body := []byte("{}")

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(body)))
	if err != nil {
		return "", time.Time{}, err
	}

	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken")

	// Sign the request with AWS Signature V4
	if err := p.signRequest(req, body, region, "ecr", creds); err != nil {
		return "", time.Time{}, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("ECR API error: %d - %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		AuthorizationData []struct {
			AuthorizationToken string `json:"authorizationToken"`
			ExpiresAt          int64  `json:"expiresAt"`
			ProxyEndpoint      string `json:"proxyEndpoint"`
		} `json:"authorizationData"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse ECR response: %w", err)
	}

	if len(result.AuthorizationData) == 0 {
		return "", time.Time{}, fmt.Errorf("no authorization data returned")
	}

	authData := result.AuthorizationData[0]
	expiresAt := time.Unix(authData.ExpiresAt, 0)

	return authData.AuthorizationToken, expiresAt, nil
}

// signRequest signs an HTTP request using AWS Signature V4.
func (p *ECRCredentialProvider) signRequest(req *http.Request, body []byte, region, service string, creds *awsCredentials) error {
	t := time.Now().UTC()
	dateStamp := t.Format("20060102")
	amzDate := t.Format("20060102T150405Z")

	// Add required headers
	req.Header.Set("X-Amz-Date", amzDate)
	if creds.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.SessionToken)
	}

	// Create canonical request
	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	canonicalQueryString := ""
	if req.URL.RawQuery != "" {
		params := req.URL.Query()
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(params))
		for _, k := range keys {
			for _, v := range params[k] {
				pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
			}
		}
		canonicalQueryString = strings.Join(pairs, "&")
	}

	// Build signed headers
	signedHeaders := []string{"content-type", "host", "x-amz-date", "x-amz-target"}
	if creds.SessionToken != "" {
		signedHeaders = append(signedHeaders, "x-amz-security-token")
	}
	sort.Strings(signedHeaders)

	host := req.URL.Host
	if host == "" {
		host = req.Host
	}

	var canonicalHeaders strings.Builder
	headerValues := map[string]string{
		"content-type":         req.Header.Get("Content-Type"),
		"host":                 host,
		"x-amz-date":           amzDate,
		"x-amz-target":         req.Header.Get("X-Amz-Target"),
		"x-amz-security-token": creds.SessionToken,
	}
	for _, h := range signedHeaders {
		canonicalHeaders.WriteString(h)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(headerValues[h]))
		canonicalHeaders.WriteString("\n")
	}

	payloadHash := sha256Hex(body)

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders.String(),
		strings.Join(signedHeaders, ";"),
		payloadHash,
	}, "\n")

	// Create string to sign
	algorithm := "AWS4-HMAC-SHA256"
	credentialScope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		algorithm,
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// Calculate signature
	signingKey := getSignatureKey(creds.SecretAccessKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// Build authorization header
	authHeader := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		creds.AccessKeyID,
		credentialScope,
		strings.Join(signedHeaders, ";"),
		signature,
	)

	req.Header.Set("Authorization", authHeader)

	return nil
}

func sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func getSignatureKey(secretKey, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

// extractECRRegion extracts the region from an ECR registry URL.
var ecrRegionRegex = regexp.MustCompile(`\.dkr\.ecr\.([a-z0-9-]+)\.amazonaws\.com`)

func extractECRRegion(registry string) string {
	matches := ecrRegionRegex.FindStringSubmatch(registry)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// MatchesRegistry checks if this provider can handle the given registry.
func (p *ECRCredentialProvider) MatchesRegistry(registry string) bool {
	return strings.Contains(registry, ".dkr.ecr.") && strings.Contains(registry, ".amazonaws.com")
}
