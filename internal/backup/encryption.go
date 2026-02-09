package backup

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// AgeEncryptor implements encryption using the age tool
type AgeEncryptor struct {
	recipientKey string // age public key for encryption
	identityPath string // age identity file for decryption
	logger       Logger
}

// NewAgeEncryptor creates a new age encryptor
func NewAgeEncryptor(recipientKey, identityPath string, logger Logger) *AgeEncryptor {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &AgeEncryptor{
		recipientKey: recipientKey,
		identityPath: identityPath,
		logger:       logger,
	}
}

// Type returns the encryption type
func (e *AgeEncryptor) Type() EncryptionType {
	return EncryptionTypeAge
}

// Encrypt encrypts data using age
func (e *AgeEncryptor) Encrypt(ctx context.Context, plaintext io.Reader, ciphertext io.Writer) error {
	args := []string{"-r", e.recipientKey, "-a"}
	//nolint:gosec // G204: age CLI execution is intentional for backup encryption
	cmd := exec.CommandContext(ctx, "age", args...)
	cmd.Stdin = plaintext
	cmd.Stdout = ciphertext
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("age encryption failed: %w", err)
	}

	e.logger.Debug("encrypted data with age")
	return nil
}

// Decrypt decrypts data using age
func (e *AgeEncryptor) Decrypt(ctx context.Context, ciphertext io.Reader, plaintext io.Writer) error {
	args := []string{"-d", "-i", e.identityPath}
	//nolint:gosec // G204: age CLI execution is intentional for backup decryption
	cmd := exec.CommandContext(ctx, "age", args...)
	cmd.Stdin = ciphertext
	cmd.Stdout = plaintext
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("age decryption failed: %w", err)
	}

	e.logger.Debug("decrypted data with age")
	return nil
}

// EncryptFile encrypts a file using age
func (e *AgeEncryptor) EncryptFile(ctx context.Context, inputPath, outputPath string) error {
	args := []string{"-r", e.recipientKey, "-o", outputPath, inputPath}
	//nolint:gosec // G204: age CLI execution is intentional for backup encryption
	cmd := exec.CommandContext(ctx, "age", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("age file encryption failed: %w", err)
	}

	e.logger.Debug("encrypted file with age", "input", inputPath, "output", outputPath)
	return nil
}

// DecryptFile decrypts a file using age
func (e *AgeEncryptor) DecryptFile(ctx context.Context, inputPath, outputPath string) error {
	args := []string{"-d", "-i", e.identityPath, "-o", outputPath, inputPath}
	//nolint:gosec // G204: age CLI execution is intentional for backup decryption
	cmd := exec.CommandContext(ctx, "age", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("age file decryption failed: %w", err)
	}

	e.logger.Debug("decrypted file with age", "input", inputPath, "output", outputPath)
	return nil
}

// AWSKMSEncryptor implements encryption using AWS KMS
type AWSKMSEncryptor struct {
	keyID    string
	region   string
	profile  string
	endpoint string // optional, for LocalStack testing
	logger   Logger
}

// AWSKMSConfig holds AWS KMS configuration
type AWSKMSConfig struct {
	KeyID    string
	Region   string
	Profile  string
	Endpoint string
}

// NewAWSKMSEncryptor creates a new AWS KMS encryptor
func NewAWSKMSEncryptor(config AWSKMSConfig, logger Logger) *AWSKMSEncryptor {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &AWSKMSEncryptor{
		keyID:    config.KeyID,
		region:   config.Region,
		profile:  config.Profile,
		endpoint: config.Endpoint,
		logger:   logger,
	}
}

// Type returns the encryption type
func (e *AWSKMSEncryptor) Type() EncryptionType {
	return EncryptionTypeAWSKMS
}

// Encrypt encrypts data using AWS KMS envelope encryption
func (e *AWSKMSEncryptor) Encrypt(ctx context.Context, plaintext io.Reader, ciphertext io.Writer) error {
	// Generate data key using KMS
	dataKey, encryptedKey, err := e.generateDataKey(ctx)
	if err != nil {
		return fmt.Errorf("failed to generate data key: %w", err)
	}

	// Write encrypted key length and key first
	//nolint:gosec // G115: encryptedKey length is bounded by KMS key size (typically <1KB)
	keyLen := uint32(len(encryptedKey))
	if _, err := ciphertext.Write([]byte{byte(keyLen >> 24), byte(keyLen >> 16), byte(keyLen >> 8), byte(keyLen)}); err != nil {
		return err
	}
	if _, err := ciphertext.Write(encryptedKey); err != nil {
		return err
	}

	// Encrypt data with AES-GCM using the data key
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Write nonce
	if _, err := ciphertext.Write(nonce); err != nil {
		return err
	}

	// Read all plaintext (envelope encryption requires full data)
	plaintextData, err := io.ReadAll(plaintext)
	if err != nil {
		return fmt.Errorf("failed to read plaintext: %w", err)
	}

	// Encrypt
	encrypted := gcm.Seal(nil, nonce, plaintextData, nil)
	if _, err := ciphertext.Write(encrypted); err != nil {
		return err
	}

	e.logger.Debug("encrypted data with AWS KMS envelope encryption")
	return nil
}

// Decrypt decrypts data using AWS KMS envelope encryption
func (e *AWSKMSEncryptor) Decrypt(ctx context.Context, ciphertext io.Reader, plaintext io.Writer) error {
	// Read encrypted key length
	keyLenBytes := make([]byte, 4)
	if _, err := io.ReadFull(ciphertext, keyLenBytes); err != nil {
		return fmt.Errorf("failed to read key length: %w", err)
	}
	keyLen := uint32(keyLenBytes[0])<<24 | uint32(keyLenBytes[1])<<16 | uint32(keyLenBytes[2])<<8 | uint32(keyLenBytes[3])

	// Read encrypted key
	encryptedKey := make([]byte, keyLen)
	if _, err := io.ReadFull(ciphertext, encryptedKey); err != nil {
		return fmt.Errorf("failed to read encrypted key: %w", err)
	}

	// Decrypt data key using KMS
	dataKey, err := e.decryptDataKey(ctx, encryptedKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt data key: %w", err)
	}

	// Create cipher
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Read nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(ciphertext, nonce); err != nil {
		return fmt.Errorf("failed to read nonce: %w", err)
	}

	// Read encrypted data
	encryptedData, err := io.ReadAll(ciphertext)
	if err != nil {
		return fmt.Errorf("failed to read encrypted data: %w", err)
	}

	// Decrypt
	decrypted, err := gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt data: %w", err)
	}

	if _, err := plaintext.Write(decrypted); err != nil {
		return err
	}

	e.logger.Debug("decrypted data with AWS KMS envelope encryption")
	return nil
}

// EncryptFile encrypts a file
func (e *AWSKMSEncryptor) EncryptFile(ctx context.Context, inputPath, outputPath string) error {
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	return e.Encrypt(ctx, input, output)
}

// DecryptFile decrypts a file
func (e *AWSKMSEncryptor) DecryptFile(ctx context.Context, inputPath, outputPath string) error {
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	return e.Decrypt(ctx, input, output)
}

// generateDataKey generates a data key using AWS KMS
func (e *AWSKMSEncryptor) generateDataKey(ctx context.Context) (plaintext, ciphertext []byte, err error) {
	args := []string{
		"kms", "generate-data-key",
		"--key-id", e.keyID,
		"--key-spec", "AES_256",
		"--output", "json",
	}
	if e.region != "" {
		args = append(args, "--region", e.region)
	}
	if e.profile != "" {
		args = append(args, "--profile", e.profile)
	}
	if e.endpoint != "" {
		args = append(args, "--endpoint-url", e.endpoint)
	}

	//nolint:gosec // G204: AWS CLI execution is intentional for KMS key generation
	cmd := exec.CommandContext(ctx, "aws", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("aws kms generate-data-key failed: %w", err)
	}

	var result struct {
		Plaintext      string `json:"Plaintext"`
		CiphertextBlob string `json:"CiphertextBlob"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, nil, fmt.Errorf("failed to parse KMS response: %w", err)
	}

	dataKey, err := base64.StdEncoding.DecodeString(result.Plaintext)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode data key: %w", err)
	}

	encryptedKey, err := base64.StdEncoding.DecodeString(result.CiphertextBlob)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode encrypted key: %w", err)
	}

	return dataKey, encryptedKey, nil
}

// decryptDataKey decrypts a data key using AWS KMS
func (e *AWSKMSEncryptor) decryptDataKey(ctx context.Context, encryptedKey []byte) ([]byte, error) {
	// Write encrypted key to temp file
	tmpFile, err := os.CreateTemp("", "kms_key_*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write(encryptedKey); err != nil {
		tmpFile.Close()
		return nil, err
	}
	tmpFile.Close()

	args := []string{
		"kms", "decrypt",
		"--ciphertext-blob", "fileb://" + tmpFile.Name(),
		"--output", "json",
	}
	if e.region != "" {
		args = append(args, "--region", e.region)
	}
	if e.profile != "" {
		args = append(args, "--profile", e.profile)
	}
	if e.endpoint != "" {
		args = append(args, "--endpoint-url", e.endpoint)
	}

	//nolint:gosec // G204: AWS CLI execution is intentional for KMS decryption
	cmd := exec.CommandContext(ctx, "aws", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("aws kms decrypt failed: %w", err)
	}

	var result struct {
		Plaintext string `json:"Plaintext"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse KMS response: %w", err)
	}

	dataKey, err := base64.StdEncoding.DecodeString(result.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode data key: %w", err)
	}

	return dataKey, nil
}

// GCPKMSEncryptor implements encryption using Google Cloud KMS
type GCPKMSEncryptor struct {
	keyName string // projects/{project}/locations/{location}/keyRings/{keyring}/cryptoKeys/{key}
	logger  Logger
}

// GCPKMSConfig holds GCP KMS configuration
type GCPKMSConfig struct {
	Project  string
	Location string
	KeyRing  string
	Key      string
}

// NewGCPKMSEncryptor creates a new GCP KMS encryptor
func NewGCPKMSEncryptor(config GCPKMSConfig, logger Logger) *GCPKMSEncryptor {
	if logger == nil {
		logger = &noopLogger{}
	}
	keyName := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
		config.Project, config.Location, config.KeyRing, config.Key)
	return &GCPKMSEncryptor{
		keyName: keyName,
		logger:  logger,
	}
}

// Type returns the encryption type
func (e *GCPKMSEncryptor) Type() EncryptionType {
	return EncryptionTypeGCPKMS
}

// Encrypt encrypts data using GCP KMS
func (e *GCPKMSEncryptor) Encrypt(ctx context.Context, plaintext io.Reader, ciphertext io.Writer) error {
	// Read plaintext
	plaintextData, err := io.ReadAll(plaintext)
	if err != nil {
		return fmt.Errorf("failed to read plaintext: %w", err)
	}

	// Use gcloud to encrypt (for simplicity, avoiding SDK dependency)
	tmpPlain, err := os.CreateTemp("", "gcp_plain_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpPlain.Name())
	if _, err := tmpPlain.Write(plaintextData); err != nil {
		tmpPlain.Close()
		return err
	}
	tmpPlain.Close()

	tmpCipher, err := os.CreateTemp("", "gcp_cipher_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpCipher.Name())
	tmpCipher.Close()

	args := []string{
		"kms", "encrypt",
		"--key", e.keyName,
		"--plaintext-file", tmpPlain.Name(),
		"--ciphertext-file", tmpCipher.Name(),
	}

	//nolint:gosec // G204: gcloud CLI execution is intentional for KMS encryption
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gcloud kms encrypt failed: %w", err)
	}

	// Read encrypted data
	encryptedData, err := os.ReadFile(tmpCipher.Name())
	if err != nil {
		return err
	}

	if _, err := ciphertext.Write(encryptedData); err != nil {
		return err
	}

	e.logger.Debug("encrypted data with GCP KMS")
	return nil
}

// Decrypt decrypts data using GCP KMS
func (e *GCPKMSEncryptor) Decrypt(ctx context.Context, ciphertext io.Reader, plaintext io.Writer) error {
	// Read ciphertext
	ciphertextData, err := io.ReadAll(ciphertext)
	if err != nil {
		return fmt.Errorf("failed to read ciphertext: %w", err)
	}

	tmpCipher, err := os.CreateTemp("", "gcp_cipher_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpCipher.Name())
	if _, err := tmpCipher.Write(ciphertextData); err != nil {
		tmpCipher.Close()
		return err
	}
	tmpCipher.Close()

	tmpPlain, err := os.CreateTemp("", "gcp_plain_*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpPlain.Name())
	tmpPlain.Close()

	args := []string{
		"kms", "decrypt",
		"--key", e.keyName,
		"--ciphertext-file", tmpCipher.Name(),
		"--plaintext-file", tmpPlain.Name(),
	}

	//nolint:gosec // G204: gcloud CLI execution is intentional for KMS decryption
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gcloud kms decrypt failed: %w", err)
	}

	// Read decrypted data
	plaintextData, err := os.ReadFile(tmpPlain.Name())
	if err != nil {
		return err
	}

	if _, err := plaintext.Write(plaintextData); err != nil {
		return err
	}

	e.logger.Debug("decrypted data with GCP KMS")
	return nil
}

// EncryptFile encrypts a file
func (e *GCPKMSEncryptor) EncryptFile(ctx context.Context, inputPath, outputPath string) error {
	args := []string{
		"kms", "encrypt",
		"--key", e.keyName,
		"--plaintext-file", inputPath,
		"--ciphertext-file", outputPath,
	}

	//nolint:gosec // G204: gcloud CLI execution is intentional for KMS file encryption
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gcloud kms encrypt failed: %w", err)
	}

	e.logger.Debug("encrypted file with GCP KMS", "input", inputPath, "output", outputPath)
	return nil
}

// DecryptFile decrypts a file
func (e *GCPKMSEncryptor) DecryptFile(ctx context.Context, inputPath, outputPath string) error {
	args := []string{
		"kms", "decrypt",
		"--key", e.keyName,
		"--ciphertext-file", inputPath,
		"--plaintext-file", outputPath,
	}

	//nolint:gosec // G204: gcloud CLI execution is intentional for KMS file decryption
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gcloud kms decrypt failed: %w", err)
	}

	e.logger.Debug("decrypted file with GCP KMS", "input", inputPath, "output", outputPath)
	return nil
}

// AzureKeyVaultEncryptor implements encryption using Azure Key Vault
type AzureKeyVaultEncryptor struct {
	vaultURL string
	keyName  string
	logger   Logger
}

// AzureKeyVaultConfig holds Azure Key Vault configuration
type AzureKeyVaultConfig struct {
	VaultURL string
	KeyName  string
}

// NewAzureKeyVaultEncryptor creates a new Azure Key Vault encryptor
func NewAzureKeyVaultEncryptor(config AzureKeyVaultConfig, logger Logger) *AzureKeyVaultEncryptor {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &AzureKeyVaultEncryptor{
		vaultURL: config.VaultURL,
		keyName:  config.KeyName,
		logger:   logger,
	}
}

// Type returns the encryption type
func (e *AzureKeyVaultEncryptor) Type() EncryptionType {
	return EncryptionTypeAzureKeyVault
}

// Encrypt encrypts data using Azure Key Vault
func (e *AzureKeyVaultEncryptor) Encrypt(ctx context.Context, plaintext io.Reader, ciphertext io.Writer) error {
	// Read plaintext
	plaintextData, err := io.ReadAll(plaintext)
	if err != nil {
		return fmt.Errorf("failed to read plaintext: %w", err)
	}

	// Encode as base64 for az command
	encoded := base64.StdEncoding.EncodeToString(plaintextData)

	args := []string{
		"keyvault", "key", "encrypt",
		"--vault-name", strings.TrimPrefix(strings.Split(e.vaultURL, ".")[0], "https://"),
		"--name", e.keyName,
		"--algorithm", "RSA-OAEP",
		"--value", encoded,
		"--output", "json",
	}

	//nolint:gosec // G204: Azure CLI execution is intentional for Key Vault encryption
	cmd := exec.CommandContext(ctx, "az", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("az keyvault encrypt failed: %w", err)
	}

	var result struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("failed to parse Azure response: %w", err)
	}

	encryptedData, err := base64.URLEncoding.DecodeString(result.Result)
	if err != nil {
		return fmt.Errorf("failed to decode encrypted data: %w", err)
	}

	if _, err := ciphertext.Write(encryptedData); err != nil {
		return err
	}

	e.logger.Debug("encrypted data with Azure Key Vault")
	return nil
}

// Decrypt decrypts data using Azure Key Vault
func (e *AzureKeyVaultEncryptor) Decrypt(ctx context.Context, ciphertext io.Reader, plaintext io.Writer) error {
	// Read ciphertext
	ciphertextData, err := io.ReadAll(ciphertext)
	if err != nil {
		return fmt.Errorf("failed to read ciphertext: %w", err)
	}

	// Encode as base64 URL for az command
	encoded := base64.URLEncoding.EncodeToString(ciphertextData)

	args := []string{
		"keyvault", "key", "decrypt",
		"--vault-name", strings.TrimPrefix(strings.Split(e.vaultURL, ".")[0], "https://"),
		"--name", e.keyName,
		"--algorithm", "RSA-OAEP",
		"--value", encoded,
		"--output", "json",
	}

	//nolint:gosec // G204: Azure CLI execution is intentional for Key Vault decryption
	cmd := exec.CommandContext(ctx, "az", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("az keyvault decrypt failed: %w", err)
	}

	var result struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("failed to parse Azure response: %w", err)
	}

	decryptedData, err := base64.StdEncoding.DecodeString(result.Result)
	if err != nil {
		return fmt.Errorf("failed to decode decrypted data: %w", err)
	}

	if _, err := plaintext.Write(decryptedData); err != nil {
		return err
	}

	e.logger.Debug("decrypted data with Azure Key Vault")
	return nil
}

// EncryptFile encrypts a file
func (e *AzureKeyVaultEncryptor) EncryptFile(ctx context.Context, inputPath, outputPath string) error {
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	return e.Encrypt(ctx, input, output)
}

// DecryptFile decrypts a file
func (e *AzureKeyVaultEncryptor) DecryptFile(ctx context.Context, inputPath, outputPath string) error {
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	return e.Decrypt(ctx, input, output)
}

// VaultTransitEncryptor implements encryption using HashiCorp Vault Transit
type VaultTransitEncryptor struct {
	address   string
	token     string
	keyName   string
	mountPath string
	namespace string
	logger    Logger
	client    *http.Client
}

// VaultTransitConfig holds Vault Transit configuration
type VaultTransitConfig struct {
	Address   string
	Token     string
	KeyName   string
	MountPath string // default: "transit"
	Namespace string // optional, for Vault Enterprise
}

// NewVaultTransitEncryptor creates a new Vault Transit encryptor
func NewVaultTransitEncryptor(config VaultTransitConfig, logger Logger) *VaultTransitEncryptor {
	if logger == nil {
		logger = &noopLogger{}
	}
	if config.MountPath == "" {
		config.MountPath = "transit"
	}
	return &VaultTransitEncryptor{
		address:   config.Address,
		token:     config.Token,
		keyName:   config.KeyName,
		mountPath: config.MountPath,
		namespace: config.Namespace,
		logger:    logger,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Type returns the encryption type
func (e *VaultTransitEncryptor) Type() EncryptionType {
	return EncryptionTypeVaultTransit
}

// Encrypt encrypts data using Vault Transit
func (e *VaultTransitEncryptor) Encrypt(ctx context.Context, plaintext io.Reader, ciphertext io.Writer) error {
	// Read plaintext
	plaintextData, err := io.ReadAll(plaintext)
	if err != nil {
		return fmt.Errorf("failed to read plaintext: %w", err)
	}

	// Encode as base64 for Vault
	encoded := base64.StdEncoding.EncodeToString(plaintextData)

	// Build request
	url := fmt.Sprintf("%s/v1/%s/encrypt/%s", e.address, e.mountPath, e.keyName)
	body := map[string]string{"plaintext": encoded}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", e.token)
	req.Header.Set("Content-Type", "application/json")
	if e.namespace != "" {
		req.Header.Set("X-Vault-Namespace", e.namespace)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault encrypt failed: %s", string(respBody))
	}

	var result struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse vault response: %w", err)
	}

	// Write ciphertext (Vault format: vault:v1:base64...)
	if _, err := ciphertext.Write([]byte(result.Data.Ciphertext)); err != nil {
		return err
	}

	e.logger.Debug("encrypted data with Vault Transit")
	return nil
}

// Decrypt decrypts data using Vault Transit
func (e *VaultTransitEncryptor) Decrypt(ctx context.Context, ciphertext io.Reader, plaintext io.Writer) error {
	// Read ciphertext
	ciphertextData, err := io.ReadAll(ciphertext)
	if err != nil {
		return fmt.Errorf("failed to read ciphertext: %w", err)
	}

	// Build request
	url := fmt.Sprintf("%s/v1/%s/decrypt/%s", e.address, e.mountPath, e.keyName)
	body := map[string]string{"ciphertext": string(ciphertextData)}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", e.token)
	req.Header.Set("Content-Type", "application/json")
	if e.namespace != "" {
		req.Header.Set("X-Vault-Namespace", e.namespace)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault decrypt failed: %s", string(respBody))
	}

	var result struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse vault response: %w", err)
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(result.Data.Plaintext)
	if err != nil {
		return fmt.Errorf("failed to decode plaintext: %w", err)
	}

	if _, err := plaintext.Write(decoded); err != nil {
		return err
	}

	e.logger.Debug("decrypted data with Vault Transit")
	return nil
}

// EncryptFile encrypts a file
func (e *VaultTransitEncryptor) EncryptFile(ctx context.Context, inputPath, outputPath string) error {
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	return e.Encrypt(ctx, input, output)
}

// DecryptFile decrypts a file
func (e *VaultTransitEncryptor) DecryptFile(ctx context.Context, inputPath, outputPath string) error {
	input, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	return e.Decrypt(ctx, input, output)
}

// NoopEncryptor is a no-operation encryptor for unencrypted backups
type NoopEncryptor struct{}

// NewNoopEncryptor creates a new no-op encryptor
func NewNoopEncryptor() *NoopEncryptor {
	return &NoopEncryptor{}
}

// Type returns the encryption type
func (e *NoopEncryptor) Type() EncryptionType {
	return EncryptionTypeNone
}

// Encrypt copies data without encryption
func (e *NoopEncryptor) Encrypt(ctx context.Context, plaintext io.Reader, ciphertext io.Writer) error {
	_, err := io.Copy(ciphertext, plaintext)
	return err
}

// Decrypt copies data without decryption
func (e *NoopEncryptor) Decrypt(ctx context.Context, ciphertext io.Reader, plaintext io.Writer) error {
	_, err := io.Copy(plaintext, ciphertext)
	return err
}

// EncryptFile copies a file without encryption
func (e *NoopEncryptor) EncryptFile(ctx context.Context, inputPath, outputPath string) error {
	return copyFile(inputPath, outputPath)
}

// DecryptFile copies a file without decryption
func (e *NoopEncryptor) DecryptFile(ctx context.Context, inputPath, outputPath string) error {
	return copyFile(inputPath, outputPath)
}

// NewEncryptor creates an encryptor based on configuration
func NewEncryptor(config *EncryptionConfig, logger Logger) (Encryptor, error) {
	if config == nil || config.Type == EncryptionTypeNone {
		return NewNoopEncryptor(), nil
	}

	switch config.Type {
	case EncryptionTypeAge:
		if config.AgeRecipient == "" || config.AgeIdentityPath == "" {
			return nil, fmt.Errorf("age encryption requires recipient and identity path")
		}
		return NewAgeEncryptor(config.AgeRecipient, config.AgeIdentityPath, logger), nil

	case EncryptionTypeAWSKMS:
		awsConfig := AWSKMSConfig{
			KeyID:   config.AWSKeyID,
			Region:  config.AWSRegion,
			Profile: config.AWSProfile,
		}
		if awsConfig.KeyID == "" {
			return nil, fmt.Errorf("AWS KMS encryption requires key ID")
		}
		return NewAWSKMSEncryptor(awsConfig, logger), nil

	case EncryptionTypeGCPKMS:
		gcpConfig := GCPKMSConfig{
			Project:  config.GCPProject,
			Location: config.GCPLocation,
			KeyRing:  config.GCPKeyRing,
			Key:      config.GCPKey,
		}
		if gcpConfig.Project == "" || gcpConfig.KeyRing == "" || gcpConfig.Key == "" {
			return nil, fmt.Errorf("GCP KMS encryption requires project, keyring, and key")
		}
		return NewGCPKMSEncryptor(gcpConfig, logger), nil

	case EncryptionTypeAzureKeyVault:
		azureConfig := AzureKeyVaultConfig{
			VaultURL: config.AzureVaultURL,
			KeyName:  config.AzureKeyName,
		}
		if azureConfig.VaultURL == "" || azureConfig.KeyName == "" {
			return nil, fmt.Errorf("azure Key Vault encryption requires vault URL and key name")
		}
		return NewAzureKeyVaultEncryptor(azureConfig, logger), nil

	case EncryptionTypeVaultTransit:
		vaultConfig := VaultTransitConfig{
			Address:   config.VaultAddress,
			Token:     config.VaultToken,
			KeyName:   config.VaultKeyName,
			MountPath: config.VaultMountPath,
			Namespace: config.VaultNamespace,
		}
		if vaultConfig.Address == "" || vaultConfig.Token == "" || vaultConfig.KeyName == "" {
			return nil, fmt.Errorf("vault transit encryption requires address, token, and key name")
		}
		return NewVaultTransitEncryptor(vaultConfig, logger), nil

	default:
		return nil, fmt.Errorf("unsupported encryption type: %s", config.Type)
	}
}
