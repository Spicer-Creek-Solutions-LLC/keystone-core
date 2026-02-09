package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// AWS EC2 metadata service endpoints
	awsMetadataBaseURL = "http://169.254.169.254/latest/meta-data"
	//nolint:gosec // G101: false positive - this is a URL endpoint, not a hardcoded secret
	awsMetadataToken = "http://169.254.169.254/latest/api/token"
	awsDynamicURL      = "http://169.254.169.254/latest/dynamic/instance-identity/document"

	// ECS metadata endpoints
	ecsMetadataURIEnv = "ECS_CONTAINER_METADATA_URI_V4"
	ecsMetadataURI    = "ECS_CONTAINER_METADATA_URI"

	// Lambda environment variables
	lambdaTaskRootEnv     = "LAMBDA_TASK_ROOT"
	lambdaRuntimeAPIEnv   = "AWS_LAMBDA_RUNTIME_API"
	lambdaFunctionNameEnv = "AWS_LAMBDA_FUNCTION_NAME"
)

// AWSDetector detects AWS environments
type AWSDetector struct {
	config     *Config
	httpClient *http.Client
}

// NewAWSDetector creates a new AWS detector
func NewAWSDetector(config *Config) *AWSDetector {
	if config == nil {
		config = DefaultConfig()
	}

	return &AWSDetector{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Detect attempts to detect AWS environment and collect metadata
func (d *AWSDetector) Detect() (*Metadata, error) {
	// Check environment type
	envType, err := d.detectEnvironmentType()
	if err != nil {
		return nil, err
	}

	metadata := &Metadata{
		Provider:        ProviderAWS,
		EnvironmentType: envType,
		DetectedAt:      time.Now(),
		Tags:            make(map[string]string),
	}

	// Collect metadata based on environment type
	switch envType {
	case EnvTypeServerless:
		if err := d.collectLambdaMetadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect Lambda metadata: %w", err)
		}
	case EnvTypeContainer:
		if err := d.collectECSMetadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect ECS metadata: %w", err)
		}
	case EnvTypeVM:
		if err := d.collectEC2Metadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect EC2 metadata: %w", err)
		}
	default:
	}

	return metadata, nil
}

// IsCloudEnvironment checks if running in AWS
func (d *AWSDetector) IsCloudEnvironment() bool {
	_, err := d.detectEnvironmentType()
	return err == nil
}

// GetProvider returns AWS as the provider
func (d *AWSDetector) GetProvider() Provider {
	return ProviderAWS
}

// detectEnvironmentType determines the AWS environment type
func (d *AWSDetector) detectEnvironmentType() (EnvironmentType, error) {
	// Check for Lambda first (most specific)
	if d.isLambda() {
		return EnvTypeServerless, nil
	}

	// Check for ECS
	if d.isECS() {
		return EnvTypeContainer, nil
	}

	// Check for EC2
	if d.isEC2() {
		return EnvTypeVM, nil
	}

	return EnvTypeUnknown, fmt.Errorf("not running in AWS")
}

// isLambda checks if running in AWS Lambda
func (d *AWSDetector) isLambda() bool {
	return os.Getenv(lambdaTaskRootEnv) != "" &&
		os.Getenv(lambdaRuntimeAPIEnv) != ""
}

// isECS checks if running in AWS ECS
func (d *AWSDetector) isECS() bool {
	return os.Getenv(ecsMetadataURIEnv) != "" ||
		os.Getenv(ecsMetadataURI) != ""
}

// isEC2 checks if running on AWS EC2
func (d *AWSDetector) isEC2() bool {
	// Try to get IMDSv2 token
	token, err := d.getIMDSv2Token()
	if err != nil {
		return false
	}

	// Try to access metadata service
	// Use context.Background() since this is a simple detection call with no parent context
	req, err := http.NewRequestWithContext(context.Background(), "GET", awsMetadataBaseURL+"/ami-id", http.NoBody) // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.http-customized-request.http-customized-request -- AWS IMDS uses link-local HTTP
	if err != nil {
		return false
	}

	if token != "" {
		req.Header.Set("X-aws-ec2-metadata-token", token)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// getIMDSv2Token gets an IMDSv2 token for EC2 metadata access
func (d *AWSDetector) getIMDSv2Token() (string, error) {
	// Use context.Background() since this is called from detection methods with no parent context
	req, err := http.NewRequestWithContext(context.Background(), "PUT", awsMetadataToken, http.NoBody) // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.http-customized-request.http-customized-request -- AWS IMDS uses link-local HTTP
	if err != nil {
		return "", err
	}

	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get IMDSv2 token: %d", resp.StatusCode)
	}

	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(token), nil
}

// collectEC2Metadata collects EC2 instance metadata
func (d *AWSDetector) collectEC2Metadata(metadata *Metadata) error {
	token, _ := d.getIMDSv2Token()

	// Get instance identity document (has most info in one call)
	doc, err := d.getInstanceIdentityDocument(token)
	if err == nil {
		metadata.Region = doc.Region
		metadata.AvailabilityZone = doc.AvailabilityZone
		metadata.InstanceID = doc.InstanceID
		metadata.InstanceType = doc.InstanceType
		metadata.AccountID = doc.AccountID
		metadata.PrivateIP = doc.PrivateIP
	}

	// Get additional metadata
	if publicIP, err := d.getMetadata(token, "/public-ipv4"); err == nil {
		metadata.PublicIP = publicIP
	}

	if vpcID, err := d.getMetadata(token, "/network/interfaces/macs/"+getMACAddress(token, d)+"/vpc-id"); err == nil {
		metadata.VPCID = vpcID
	}

	if subnetID, err := d.getMetadata(token, "/network/interfaces/macs/"+getMACAddress(token, d)+"/subnet-id"); err == nil {
		metadata.SubnetID = subnetID
	}

	// Get tags (if available)
	if tags, err := d.getInstanceTags(token); err == nil {
		metadata.Tags = tags
	}

	return nil
}

// collectECSMetadata collects ECS task metadata
func (d *AWSDetector) collectECSMetadata(metadata *Metadata) error {
	metadataURI := os.Getenv(ecsMetadataURIEnv)
	if metadataURI == "" {
		metadataURI = os.Getenv(ecsMetadataURI)
	}

	if metadataURI == "" {
		return fmt.Errorf("ECS metadata URI not found")
	}

	// Get task metadata
	taskMetadata, err := d.getECSTaskMetadata(metadataURI)
	if err != nil {
		return fmt.Errorf("failed to get ECS task metadata: %w", err)
	}

	metadata.Region = extractRegionFromARN(taskMetadata.TaskARN)
	metadata.AccountID = extractAccountFromARN(taskMetadata.TaskARN)

	metadata.Container = &ContainerMetadata{
		TaskARN:        taskMetadata.TaskARN,
		TaskDefinition: taskMetadata.Family + ":" + taskMetadata.Revision,
		ClusterARN:     taskMetadata.Cluster,
	}

	// Get container metadata
	containerMetadata, err := d.getECSContainerMetadata(metadataURI)
	if err == nil {
		metadata.Container.ContainerID = containerMetadata.DockerID
		metadata.Container.ContainerName = containerMetadata.Name
		metadata.Container.ImageName = containerMetadata.Image
		metadata.Container.ImageDigest = containerMetadata.ImageID
	}

	return nil
}

// collectLambdaMetadata collects Lambda function metadata
func (d *AWSDetector) collectLambdaMetadata(metadata *Metadata) error {
	functionName := os.Getenv(lambdaFunctionNameEnv)
	if functionName == "" {
		return fmt.Errorf("lambda function name not found")
	}

	metadata.Serverless = &ServerlessMetadata{
		FunctionName:    functionName,
		FunctionVersion: os.Getenv("AWS_LAMBDA_FUNCTION_VERSION"),
		FunctionARN:     os.Getenv("AWS_LAMBDA_FUNCTION_ARN"),
		Handler:         os.Getenv("_HANDLER"),
		Runtime:         os.Getenv("AWS_EXECUTION_ENV"),
		RequestID:       os.Getenv("AWS_REQUEST_ID"),
	}

	// Parse memory size
	if memStr := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE"); memStr != "" {
		var mem int
		fmt.Sscanf(memStr, "%d", &mem)
		metadata.Serverless.MemorySize = mem
	}

	// Extract region from function ARN
	if arn := metadata.Serverless.FunctionARN; arn != "" {
		metadata.Region = extractRegionFromARN(arn)
		metadata.AccountID = extractAccountFromARN(arn)
	}

	return nil
}

// getMetadata gets a metadata value from EC2 metadata service
func (d *AWSDetector) getMetadata(token, path string) (string, error) {
	// Use context.Background() since this is called from detection/collection methods with no parent context
	req, err := http.NewRequestWithContext(context.Background(), "GET", awsMetadataBaseURL+path, http.NoBody) // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.http-customized-request.http-customized-request -- AWS IMDS uses link-local HTTP
	if err != nil {
		return "", err
	}

	if token != "" {
		req.Header.Set("X-aws-ec2-metadata-token", token)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata request failed: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// getInstanceIdentityDocument gets the EC2 instance identity document
func (d *AWSDetector) getInstanceIdentityDocument(token string) (*ec2InstanceIdentityDocument, error) {
	// Use context.Background() since this is called from detection/collection methods with no parent context
	req, err := http.NewRequestWithContext(context.Background(), "GET", awsDynamicURL, http.NoBody) // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.http-customized-request.http-customized-request -- AWS dynamic metadata uses link-local HTTP
	if err != nil {
		return nil, err
	}

	if token != "" {
		req.Header.Set("X-aws-ec2-metadata-token", token)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("identity document request failed: %d", resp.StatusCode)
	}

	var doc ec2InstanceIdentityDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}

	return &doc, nil
}

// getInstanceTags gets EC2 instance tags
func (d *AWSDetector) getInstanceTags(token string) (map[string]string, error) {
	tagKeys, err := d.getMetadata(token, "/tags/instance")
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)
	for _, key := range strings.Split(tagKeys, "\n") {
		if key == "" {
			continue
		}
		value, err := d.getMetadata(token, "/tags/instance/"+key)
		if err == nil {
			tags[key] = value
		}
	}

	return tags, nil
}

// getECSTaskMetadata gets ECS task metadata
func (d *AWSDetector) getECSTaskMetadata(baseURI string) (*ecsTaskMetadata, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURI+"/task", http.NoBody) // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.http-customized-request.http-customized-request -- ECS metadata uses link-local HTTP
	if err != nil {
		return nil, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var metadata ecsTaskMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// getECSContainerMetadata gets ECS container metadata
func (d *AWSDetector) getECSContainerMetadata(baseURI string) (*ecsContainerMetadata, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURI, http.NoBody) // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.http-customized-request.http-customized-request -- ECS metadata uses link-local HTTP
	if err != nil {
		return nil, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var metadata ecsContainerMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// Helper types for AWS metadata

type ec2InstanceIdentityDocument struct {
	PrivateIP        string `json:"privateIp"`
	AvailabilityZone string `json:"availabilityZone"`
	Region           string `json:"region"`
	InstanceID       string `json:"instanceId"`
	InstanceType     string `json:"instanceType"`
	AccountID        string `json:"accountId"`
}

type ecsTaskMetadata struct {
	Cluster       string `json:"Cluster"`
	TaskARN       string `json:"TaskARN"`
	Family        string `json:"Family"`
	Revision      string `json:"Revision"`
	DesiredStatus string `json:"DesiredStatus"`
	KnownStatus   string `json:"KnownStatus"`
}

type ecsContainerMetadata struct {
	DockerID string `json:"DockerId"`
	Name     string `json:"Name"`
	Image    string `json:"Image"`
	ImageID  string `json:"ImageID"`
}

// Helper functions

func getMACAddress(token string, d *AWSDetector) string {
	mac, _ := d.getMetadata(token, "/mac")
	return mac
}

func extractRegionFromARN(arn string) string {
	// ARN format: arn:aws:service:region:account:resource
	parts := strings.Split(arn, ":")
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

func extractAccountFromARN(arn string) string {
	// ARN format: arn:aws:service:region:account:resource
	parts := strings.Split(arn, ":")
	if len(parts) >= 5 {
		return parts[4]
	}
	return ""
}
