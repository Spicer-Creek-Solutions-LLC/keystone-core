// Package kms provides AWS CloudHSM integration.
// This implementation uses the CloudHSM CLI tools (cloudhsm_mgmt_util, key_mgmt_util)
// or the PKCS#11 interface for communication with AWS CloudHSM clusters.
package kms

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CloudHSMConfig contains configuration for AWS CloudHSM.
type CloudHSMConfig struct {
	ProviderConfig

	// ClusterID is the CloudHSM cluster identifier.
	ClusterID string `json:"cluster_id"`

	// HSMIPAddresses are the IP addresses of HSMs in the cluster.
	HSMIPAddresses []string `json:"hsm_ip_addresses"`

	// CryptoUserName is the crypto user name.
	CryptoUserName string `json:"crypto_user_name"`

	// CryptoUserPassword is the crypto user password.
	CryptoUserPassword string `json:"-"`

	// CustomerCA is the path to the customer CA certificate.
	CustomerCA string `json:"customer_ca,omitempty"`

	// ClientCert is the path to the client certificate.
	ClientCert string `json:"client_cert,omitempty"`

	// ClientKey is the path to the client private key.
	ClientKey string `json:"-"`

	// Region is the AWS region.
	Region string `json:"region,omitempty"`

	// KeyMgmtUtilPath is the path to key_mgmt_util.
	KeyMgmtUtilPath string `json:"key_mgmt_util_path,omitempty"`

	// CloudHSMCLIPath is the path to the CloudHSM CLI.
	CloudHSMCLIPath string `json:"cloudhsm_cli_path,omitempty"`

	// PKCS11LibPath is the path to the PKCS#11 library.
	PKCS11LibPath string `json:"pkcs11_lib_path,omitempty"`

	// MaxConnections is the maximum number of concurrent connections.
	MaxConnections int `json:"max_connections,omitempty"`

	// ConnectionTimeout is the connection timeout.
	ConnectionTimeout time.Duration `json:"connection_timeout,omitempty"`

	// UseCLI uses the CloudHSM CLI instead of key_mgmt_util.
	UseCLI bool `json:"use_cli,omitempty"`
}

// DefaultCloudHSMConfig returns default AWS CloudHSM configuration.
func DefaultCloudHSMConfig() *CloudHSMConfig {
	return &CloudHSMConfig{
		ProviderConfig: ProviderConfig{
			Name:       "cloudhsm",
			Type:       ProviderTypeCloudHSM,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
		Region:             "us-east-1",
		KeyMgmtUtilPath:    "/opt/cloudhsm/bin/key_mgmt_util",
		CloudHSMCLIPath:    "/opt/cloudhsm/bin/cloudhsm-cli",
		PKCS11LibPath:      "/opt/cloudhsm/lib/libcloudhsm_pkcs11.so",
		MaxConnections:     5,
		ConnectionTimeout:  10 * time.Second,
		UseCLI:             true,
	}
}

// CloudHSMProvider implements the Provider interface for AWS CloudHSM.
type CloudHSMProvider struct {
	config      *CloudHSMConfig
	pkcs11      *PKCS11Provider

	mu          sync.RWMutex
	initialized bool
	closed      bool
	loggedIn    bool
}

// NewCloudHSMProvider creates a new AWS CloudHSM provider.
func NewCloudHSMProvider(ctx context.Context, config *CloudHSMConfig) (*CloudHSMProvider, error) {
	if config == nil {
		config = DefaultCloudHSMConfig()
	}
	if config.ClusterID == "" && len(config.HSMIPAddresses) == 0 {
		return nil, errors.New("CloudHSM cluster ID or HSM IP addresses required")
	}
	if config.CryptoUserName == "" {
		return nil, errors.New("crypto user name is required")
	}

	p := &CloudHSMProvider{
		config: config,
	}

	iface := &cloudHSMInterface{
		config: config,
	}
	pkcs11Config := &PKCS11Config{
		ProviderConfig: config.ProviderConfig,
		PIN:            config.CryptoUserPassword,
		MaxSessions:    config.MaxConnections,
		Backend:        HSMBackendCloudHSM,
		BackendConfig: map[string]string{
			"cluster_id": config.ClusterID,
			"cu_user":    config.CryptoUserName,
		},
	}

	pkcs11Provider, err := NewPKCS11Provider(ctx, pkcs11Config, iface)
	if err != nil {
		return nil, fmt.Errorf("failed to create PKCS#11 provider: %w", err)
	}
	p.pkcs11 = pkcs11Provider
	p.initialized = true

	return p, nil
}

// Type returns the provider type.
func (p *CloudHSMProvider) Type() ProviderType {
	return ProviderTypeCloudHSM
}

// Name returns the provider instance name.
func (p *CloudHSMProvider) Name() string {
	return p.config.Name
}

// Healthy checks if the provider is healthy.
func (p *CloudHSMProvider) Healthy(ctx context.Context) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || !p.initialized {
		return false
	}

	return p.pkcs11.Healthy(ctx)
}

// GetKeyMetadata retrieves metadata for a key.
func (p *CloudHSMProvider) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	meta, err := p.pkcs11.GetKeyMetadata(ctx, keyID)
	if err != nil {
		return nil, err
	}
	meta.Provider = ProviderTypeCloudHSM
	return meta, nil
}

// Encrypt encrypts plaintext data.
func (p *CloudHSMProvider) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	return p.pkcs11.Encrypt(ctx, req)
}

// Decrypt decrypts ciphertext data.
func (p *CloudHSMProvider) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	return p.pkcs11.Decrypt(ctx, req)
}

// GenerateDataKey generates a data encryption key.
func (p *CloudHSMProvider) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	dk, err := p.pkcs11.GenerateDataKey(ctx, req)
	if err != nil {
		return nil, err
	}
	dk.Provider = ProviderTypeCloudHSM
	return dk, nil
}

// WrapKey wraps (encrypts) a key with the KMS key.
func (p *CloudHSMProvider) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	return p.pkcs11.WrapKey(ctx, req)
}

// UnwrapKey unwraps (decrypts) a key with the KMS key.
func (p *CloudHSMProvider) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	return p.pkcs11.UnwrapKey(ctx, req)
}

// Sign signs data with the HSM key.
func (p *CloudHSMProvider) Sign(ctx context.Context, req *SignRequest) (*SignResponse, error) {
	return p.pkcs11.Sign(ctx, req)
}

// Verify verifies a signature with the HSM key.
func (p *CloudHSMProvider) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	return p.pkcs11.Verify(ctx, req)
}

// Close closes the provider connection.
func (p *CloudHSMProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	return p.pkcs11.Close()
}

// cloudHSMInterface implements PKCS11Interface using CloudHSM CLI tools.
type cloudHSMInterface struct {
	config      *CloudHSMConfig
	mu          sync.Mutex
	initialized bool
	sessions    map[SessionHandle]*cloudHSMSession
	nextSession SessionHandle
	loggedIn    bool
}

type cloudHSMSession struct {
	handle  SessionHandle
	created time.Time
}

// Initialize initializes the PKCS#11 library.
func (c *cloudHSMInterface) Initialize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	cliPath := c.config.CloudHSMCLIPath
	if !c.config.UseCLI {
		cliPath = c.config.KeyMgmtUtilPath
	}
	if _, err := exec.LookPath(cliPath); err != nil {
		return fmt.Errorf("CloudHSM CLI not found at %s: %w", cliPath, err)
	}

	c.sessions = make(map[SessionHandle]*cloudHSMSession)
	c.nextSession = 1
	c.initialized = true
	return nil
}

// Finalize finalizes the PKCS#11 library.
func (c *cloudHSMInterface) Finalize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loggedIn {
		c.runCloudHSMCLI(ctx, "user", "logout")
		c.loggedIn = false
	}

	c.initialized = false
	c.sessions = nil
	return nil
}

// runCloudHSMCLI executes a CloudHSM CLI command.
func (c *cloudHSMInterface) runCloudHSMCLI(ctx context.Context, args ...string) (string, error) {
	var cmd *exec.Cmd
	if c.config.UseCLI {
		cmd = exec.CommandContext(ctx, c.config.CloudHSMCLIPath, args...)
	} else {
		input := strings.Join(args, " ") + "\n"
		cmd = exec.CommandContext(ctx, c.config.KeyMgmtUtilPath)
		cmd.Stdin = strings.NewReader(input)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("CloudHSM CLI command failed: %s: %w", stderr.String(), err)
	}

	return stdout.String(), nil
}

// runCloudHSMCLIJSON executes a CloudHSM CLI command and parses JSON output.
func (c *cloudHSMInterface) runCloudHSMCLIJSON(ctx context.Context, args ...string) (map[string]interface{}, error) {
	args = append(args, "--json")
	output, err := c.runCloudHSMCLI(ctx, args...)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON output: %w", err)
	}

	return result, nil
}

// GetSlotList returns the list of available slots.
func (c *cloudHSMInterface) GetSlotList(ctx context.Context, tokenPresent bool) ([]uint32, error) {
	return []uint32{0}, nil
}

// GetSlotInfo returns information about a slot.
func (c *cloudHSMInterface) GetSlotInfo(ctx context.Context, slotID uint32) (*SlotInfo, error) {
	result, err := c.runCloudHSMCLIJSON(ctx, "cluster", "status")
	if err != nil {
		return &SlotInfo{
			SlotID:          slotID,
			SlotDescription: "AWS CloudHSM Slot",
			ManufacturerID:  "AWS",
			TokenPresent:    true,
		}, nil
	}

	info := &SlotInfo{
		SlotID:          slotID,
		SlotDescription: "AWS CloudHSM Slot",
		ManufacturerID:  "AWS",
		TokenPresent:    true,
	}

	if clusterID, ok := result["cluster_id"].(string); ok {
		info.SlotDescription = fmt.Sprintf("AWS CloudHSM Cluster: %s", clusterID)
	}

	return info, nil
}

// GetTokenInfo returns information about a token.
func (c *cloudHSMInterface) GetTokenInfo(ctx context.Context, slotID uint32) (*TokenInfo, error) {
	result, err := c.runCloudHSMCLIJSON(ctx, "cluster", "status")
	if err != nil {
		return &TokenInfo{
			Label: c.config.ClusterID,
			Model: "AWS CloudHSM",
		}, nil
	}

	info := &TokenInfo{
		Label: c.config.ClusterID,
		Model: "AWS CloudHSM",
	}

	if clusterID, ok := result["cluster_id"].(string); ok {
		info.Label = clusterID
	}
	if state, ok := result["state"].(string); ok {
		info.SerialNumber = state
	}

	return info, nil
}

// GetMechanismList returns the list of mechanisms supported by CloudHSM.
func (c *cloudHSMInterface) GetMechanismList(ctx context.Context, slotID uint32) ([]PKCS11Mechanism, error) {
	return []PKCS11Mechanism{
		CKM_RSA_PKCS,
		CKM_RSA_PKCS_OAEP,
		CKM_RSA_PKCS_KEY_PAIR_GEN,
		CKM_AES_KEY_GEN,
		CKM_AES_CBC,
		CKM_AES_CBC_PAD,
		CKM_AES_GCM,
		CKM_AES_KEY_WRAP,
		CKM_AES_KEY_WRAP_PAD,
		CKM_SHA256,
		CKM_SHA384,
		CKM_SHA512,
		CKM_SHA256_RSA_PKCS,
		CKM_SHA384_RSA_PKCS,
		CKM_SHA512_RSA_PKCS,
		CKM_ECDSA,
		CKM_ECDSA_SHA256,
		CKM_ECDSA_SHA384,
	}, nil
}

// GetMechanismInfo returns information about a mechanism.
func (c *cloudHSMInterface) GetMechanismInfo(ctx context.Context, slotID uint32, mechanism PKCS11Mechanism) (*MechanismInfo, error) {
	info := &MechanismInfo{
		Mechanism: mechanism,
	}

	switch mechanism {
	case CKM_AES_KEY_GEN, CKM_AES_CBC, CKM_AES_CBC_PAD, CKM_AES_GCM, CKM_AES_KEY_WRAP, CKM_AES_KEY_WRAP_PAD:
		info.MinKeySize = 128
		info.MaxKeySize = 256
	case CKM_RSA_PKCS, CKM_RSA_PKCS_OAEP, CKM_RSA_PKCS_KEY_PAIR_GEN, CKM_SHA256_RSA_PKCS, CKM_SHA384_RSA_PKCS, CKM_SHA512_RSA_PKCS:
		info.MinKeySize = 2048
		info.MaxKeySize = 4096
	case CKM_ECDSA, CKM_ECDSA_SHA256, CKM_ECDSA_SHA384:
		info.MinKeySize = 256
		info.MaxKeySize = 521
	}

	return info, nil
}

// OpenSession opens a session to a token.
func (c *cloudHSMInterface) OpenSession(ctx context.Context, slotID uint32, flags PKCS11SessionFlags) (*Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	handle := c.nextSession
	c.nextSession++

	session := &cloudHSMSession{
		handle:  handle,
		created: time.Now(),
	}
	c.sessions[handle] = session

	return &Session{
		Handle:    handle,
		SlotID:    slotID,
		Flags:     uint32(flags),
		CreatedAt: session.created,
		LastUsed:  session.created,
	}, nil
}

// CloseSession closes a session.
func (c *cloudHSMInterface) CloseSession(ctx context.Context, session SessionHandle) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.sessions, session)
	return nil
}

// CloseAllSessions closes all sessions for a slot.
func (c *cloudHSMInterface) CloseAllSessions(ctx context.Context, slotID uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sessions = make(map[SessionHandle]*cloudHSMSession)
	return nil
}

// Login authenticates to a token.
func (c *cloudHSMInterface) Login(ctx context.Context, session SessionHandle, userType PKCS11UserType, pin string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loggedIn {
		return NewPKCS11Error(CKR_USER_ALREADY_LOGGED_IN)
	}

	_, err := c.runCloudHSMCLI(ctx, "user", "login",
		"--username", c.config.CryptoUserName,
		"--password", pin,
		"--role", "crypto-user")
	if err != nil {
		if strings.Contains(err.Error(), "already logged in") {
			c.loggedIn = true
			return NewPKCS11Error(CKR_USER_ALREADY_LOGGED_IN)
		}
		return fmt.Errorf("login failed: %w", err)
	}

	c.loggedIn = true
	return nil
}

// Logout logs out from a token.
func (c *cloudHSMInterface) Logout(ctx context.Context, session SessionHandle) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.loggedIn {
		return nil
	}

	_, err := c.runCloudHSMCLI(ctx, "user", "logout")
	c.loggedIn = false
	return err
}

// GenerateKey generates a symmetric key.
func (c *cloudHSMInterface) GenerateKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, template map[string]interface{}) (ObjectHandle, error) {
	label, _ := template["CKA_LABEL"].(string)
	if label == "" {
		label = fmt.Sprintf("key-%d", time.Now().UnixNano())
	}

	size := 32
	if s, ok := template["CKA_VALUE_LEN"].(int); ok {
		size = s
	}

	result, err := c.runCloudHSMCLIJSON(ctx, "key", "generate-symmetric", "aes",
		"--label", label,
		"--key-length-bytes", strconv.Itoa(size))
	if err != nil {
		return 0, err
	}

	if keyData, ok := result["data"].(map[string]interface{}); ok {
		if keyInfo, ok := keyData["key"].(map[string]interface{}); ok {
			if handle, ok := keyInfo["key-reference"].(string); ok {
				h, _ := strconv.ParseUint(handle, 10, 32)
				return ObjectHandle(h), nil
			}
		}
	}

	return ObjectHandle(time.Now().UnixNano() & 0xFFFFFFFF), nil
}

// GenerateKeyPair generates an asymmetric key pair.
func (c *cloudHSMInterface) GenerateKeyPair(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, publicTemplate, privateTemplate map[string]interface{}) (publicKey, privateKey ObjectHandle, err error) {
	label, _ := privateTemplate["CKA_LABEL"].(string)
	if label == "" {
		label = fmt.Sprintf("keypair-%d", time.Now().UnixNano())
	}

	keyType := "rsa"
	size := 2048
	if mechanism == CKM_ECDSA || mechanism == CKM_ECDSA_SHA256 {
		keyType = "ec"
		size = 256
	}
	if bits, ok := publicTemplate["CKA_MODULUS_BITS"].(uint32); ok {
		size = int(bits)
	}

	var result map[string]interface{}
	if keyType == "rsa" {
		result, err = c.runCloudHSMCLIJSON(ctx, "key", "generate-asymmetric-pair", "rsa",
			"--label", label,
			"--modulus-size-bits", strconv.Itoa(size))
	} else {
		curve := "secp256r1"
		if size >= 384 {
			curve = "secp384r1"
		}
		result, err = c.runCloudHSMCLIJSON(ctx, "key", "generate-asymmetric-pair", "ec",
			"--label", label,
			"--curve", curve)
	}
	if err != nil {
		return 0, 0, err
	}

	var pubHandle, privHandle ObjectHandle
	if keyData, ok := result["data"].(map[string]interface{}); ok {
		if publicKeyInfo, ok := keyData["public-key"].(map[string]interface{}); ok {
			if handle, ok := publicKeyInfo["key-reference"].(string); ok {
				h, _ := strconv.ParseUint(handle, 10, 32)
				pubHandle = ObjectHandle(h)
			}
		}
		if privateKeyInfo, ok := keyData["private-key"].(map[string]interface{}); ok {
			if handle, ok := privateKeyInfo["key-reference"].(string); ok {
				h, _ := strconv.ParseUint(handle, 10, 32)
				privHandle = ObjectHandle(h)
			}
		}
	}

	if pubHandle == 0 {
		now := time.Now().UnixNano()
		pubHandle = ObjectHandle(now & 0xFFFFFFFF)
		privHandle = ObjectHandle((now + 1) & 0xFFFFFFFF)
	}

	return pubHandle, privHandle, nil
}

// FindObjectsInit initializes a search for objects.
func (c *cloudHSMInterface) FindObjectsInit(ctx context.Context, session SessionHandle, template map[string]interface{}) error {
	return nil
}

// FindObjects continues a search for objects.
func (c *cloudHSMInterface) FindObjects(ctx context.Context, session SessionHandle, maxObjects uint32) ([]ObjectHandle, error) {
	result, err := c.runCloudHSMCLIJSON(ctx, "key", "list")
	if err != nil {
		return nil, err
	}

	var handles []ObjectHandle
	if data, ok := result["data"].(map[string]interface{}); ok {
		if keys, ok := data["matched_keys"].([]interface{}); ok {
			for _, key := range keys {
				if keyMap, ok := key.(map[string]interface{}); ok {
					if handle, ok := keyMap["key-reference"].(string); ok {
						h, err := strconv.ParseUint(handle, 10, 32)
						if err == nil {
							handles = append(handles, ObjectHandle(h))
						}
					}
				}
				if uint32(len(handles)) >= maxObjects {
					break
				}
			}
		}
	}

	return handles, nil
}

// FindObjectsFinal finalizes a search for objects.
func (c *cloudHSMInterface) FindObjectsFinal(ctx context.Context, session SessionHandle) error {
	return nil
}

// GetAttributeValue gets attribute values for an object.
func (c *cloudHSMInterface) GetAttributeValue(ctx context.Context, session SessionHandle, object ObjectHandle, attributes []string) (map[string]interface{}, error) {
	result, err := c.runCloudHSMCLIJSON(ctx, "key", "list",
		"--filter", fmt.Sprintf("key-reference=%d", object))
	if err != nil {
		return nil, err
	}

	attrs := make(map[string]interface{})
	if data, ok := result["data"].(map[string]interface{}); ok {
		if keys, ok := data["matched_keys"].([]interface{}); ok && len(keys) > 0 {
			if keyMap, ok := keys[0].(map[string]interface{}); ok {
				if label, ok := keyMap["label"].(string); ok {
					attrs["CKA_LABEL"] = label
				}
				if keyType, ok := keyMap["key-type"].(string); ok {
					switch strings.ToLower(keyType) {
					case "aes":
						attrs["CKA_KEY_TYPE"] = CKK_AES
					case "rsa":
						attrs["CKA_KEY_TYPE"] = CKK_RSA
					case "ec":
						attrs["CKA_KEY_TYPE"] = CKK_EC
					}
				}
				if keyAttrs, ok := keyMap["attributes"].(map[string]interface{}); ok {
					if keyLength, ok := keyAttrs["key-length-bytes"].(float64); ok {
						attrs["CKA_MODULUS_BITS"] = uint32(keyLength * 8)
					}
					if encrypt, ok := keyAttrs["encrypt"].(bool); ok {
						attrs["CKA_ENCRYPT"] = encrypt
					}
					if decrypt, ok := keyAttrs["decrypt"].(bool); ok {
						attrs["CKA_DECRYPT"] = decrypt
					}
					if sign, ok := keyAttrs["sign"].(bool); ok {
						attrs["CKA_SIGN"] = sign
					}
					if verify, ok := keyAttrs["verify"].(bool); ok {
						attrs["CKA_VERIFY"] = verify
					}
					if wrap, ok := keyAttrs["wrap"].(bool); ok {
						attrs["CKA_WRAP"] = wrap
					}
					if unwrap, ok := keyAttrs["unwrap"].(bool); ok {
						attrs["CKA_UNWRAP"] = unwrap
					}
					if extractable, ok := keyAttrs["extractable"].(bool); ok {
						attrs["CKA_EXTRACTABLE"] = extractable
					}
				}
			}
		}
	}

	return attrs, nil
}

// Encrypt encrypts data.
func (c *cloudHSMInterface) Encrypt(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error) {
	algo := "aes-gcm"
	switch mechanism {
	case CKM_AES_CBC, CKM_AES_CBC_PAD:
		algo = "aes-cbc"
	case CKM_RSA_PKCS:
		algo = "rsa-pkcs"
	case CKM_RSA_PKCS_OAEP:
		algo = "rsa-oaep"
	}

	result, err := c.runCloudHSMCLIJSON(ctx, "key", "encrypt",
		"--key-filter", fmt.Sprintf("key-reference=%d", key),
		"--algorithm", algo,
		"--data", hex.EncodeToString(data))
	if err != nil {
		return nil, err
	}

	if resultData, ok := result["data"].(map[string]interface{}); ok {
		if ciphertext, ok := resultData["ciphertext"].(string); ok {
			return hex.DecodeString(ciphertext)
		}
	}

	return nil, errors.New("ciphertext not found in response")
}

// Decrypt decrypts data.
func (c *cloudHSMInterface) Decrypt(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error) {
	algo := "aes-gcm"
	switch mechanism {
	case CKM_AES_CBC, CKM_AES_CBC_PAD:
		algo = "aes-cbc"
	case CKM_RSA_PKCS:
		algo = "rsa-pkcs"
	case CKM_RSA_PKCS_OAEP:
		algo = "rsa-oaep"
	}

	result, err := c.runCloudHSMCLIJSON(ctx, "key", "decrypt",
		"--key-filter", fmt.Sprintf("key-reference=%d", key),
		"--algorithm", algo,
		"--data", hex.EncodeToString(data))
	if err != nil {
		return nil, err
	}

	if resultData, ok := result["data"].(map[string]interface{}); ok {
		if plaintext, ok := resultData["plaintext"].(string); ok {
			return hex.DecodeString(plaintext)
		}
	}

	return nil, errors.New("plaintext not found in response")
}

// Sign signs data.
func (c *cloudHSMInterface) Sign(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error) {
	algo := "rsassa-pkcs1-v1_5-sha256"
	switch mechanism {
	case CKM_SHA384_RSA_PKCS:
		algo = "rsassa-pkcs1-v1_5-sha384"
	case CKM_SHA512_RSA_PKCS:
		algo = "rsassa-pkcs1-v1_5-sha512"
	case CKM_ECDSA_SHA256:
		algo = "ecdsa-sha256"
	case CKM_ECDSA_SHA384:
		algo = "ecdsa-sha384"
	}

	result, err := c.runCloudHSMCLIJSON(ctx, "key", "sign",
		"--key-filter", fmt.Sprintf("key-reference=%d", key),
		"--signing-algorithm", algo,
		"--data", hex.EncodeToString(data))
	if err != nil {
		return nil, err
	}

	if resultData, ok := result["data"].(map[string]interface{}); ok {
		if signature, ok := resultData["signature"].(string); ok {
			return hex.DecodeString(signature)
		}
	}

	return nil, errors.New("signature not found in response")
}

// Verify verifies a signature.
func (c *cloudHSMInterface) Verify(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data, signature []byte) (bool, error) {
	algo := "rsassa-pkcs1-v1_5-sha256"
	switch mechanism {
	case CKM_SHA384_RSA_PKCS:
		algo = "rsassa-pkcs1-v1_5-sha384"
	case CKM_SHA512_RSA_PKCS:
		algo = "rsassa-pkcs1-v1_5-sha512"
	case CKM_ECDSA_SHA256:
		algo = "ecdsa-sha256"
	case CKM_ECDSA_SHA384:
		algo = "ecdsa-sha384"
	}

	result, err := c.runCloudHSMCLIJSON(ctx, "key", "verify",
		"--key-filter", fmt.Sprintf("key-reference=%d", key),
		"--signing-algorithm", algo,
		"--data", hex.EncodeToString(data),
		"--signature", hex.EncodeToString(signature))
	if err != nil {
		if strings.Contains(err.Error(), "verification failed") {
			return false, nil
		}
		return false, err
	}

	if resultData, ok := result["data"].(map[string]interface{}); ok {
		if valid, ok := resultData["valid"].(bool); ok {
			return valid, nil
		}
	}

	return true, nil
}

// WrapKey wraps a key.
func (c *cloudHSMInterface) WrapKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, wrappingKey, keyToWrap ObjectHandle) ([]byte, error) {
	algo := "aes-gcm"
	switch mechanism {
	case CKM_AES_KEY_WRAP, CKM_AES_KEY_WRAP_PAD:
		algo = "aes-key-wrap-no-pad"
	case CKM_RSA_PKCS_OAEP:
		algo = "rsa-oaep"
	}

	result, err := c.runCloudHSMCLIJSON(ctx, "key", "wrap",
		"--wrapping-filter", fmt.Sprintf("key-reference=%d", wrappingKey),
		"--key-filter", fmt.Sprintf("key-reference=%d", keyToWrap),
		"--wrapping-algorithm", algo)
	if err != nil {
		return nil, err
	}

	if resultData, ok := result["data"].(map[string]interface{}); ok {
		if wrapped, ok := resultData["wrapped-key-data"].(string); ok {
			return hex.DecodeString(wrapped)
		}
	}

	return nil, errors.New("wrapped key not found in response")
}

// UnwrapKey unwraps a key.
func (c *cloudHSMInterface) UnwrapKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, unwrappingKey ObjectHandle, wrappedKey []byte, template map[string]interface{}) (ObjectHandle, error) {
	label, _ := template["CKA_LABEL"].(string)
	if label == "" {
		label = fmt.Sprintf("unwrapped-%d", time.Now().UnixNano())
	}

	algo := "aes-gcm"
	switch mechanism {
	case CKM_AES_KEY_WRAP, CKM_AES_KEY_WRAP_PAD:
		algo = "aes-key-wrap-no-pad"
	case CKM_RSA_PKCS_OAEP:
		algo = "rsa-oaep"
	}

	result, err := c.runCloudHSMCLIJSON(ctx, "key", "unwrap",
		"--unwrapping-filter", fmt.Sprintf("key-reference=%d", unwrappingKey),
		"--wrapped-key-data", hex.EncodeToString(wrappedKey),
		"--unwrapping-algorithm", algo,
		"--key-type-class", "aes",
		"--label", label)
	if err != nil {
		return 0, err
	}

	if resultData, ok := result["data"].(map[string]interface{}); ok {
		if keyInfo, ok := resultData["key"].(map[string]interface{}); ok {
			if handle, ok := keyInfo["key-reference"].(string); ok {
				h, _ := strconv.ParseUint(handle, 10, 32)
				return ObjectHandle(h), nil
			}
		}
	}

	return ObjectHandle(time.Now().UnixNano() & 0xFFFFFFFF), nil
}

// GenerateRandom generates random bytes.
func (c *cloudHSMInterface) GenerateRandom(ctx context.Context, session SessionHandle, length int) ([]byte, error) {
	result, err := c.runCloudHSMCLIJSON(ctx, "crypto", "generate-random",
		"--number-of-bytes", strconv.Itoa(length))
	if err != nil {
		return nil, err
	}

	if resultData, ok := result["data"].(map[string]interface{}); ok {
		if randomHex, ok := resultData["random-bytes"].(string); ok {
			return hex.DecodeString(randomHex)
		}
	}

	return nil, errors.New("random bytes not found in response")
}

// DestroyObject destroys an object.
func (c *cloudHSMInterface) DestroyObject(ctx context.Context, session SessionHandle, object ObjectHandle) error {
	_, err := c.runCloudHSMCLI(ctx, "key", "delete",
		"--filter", fmt.Sprintf("key-reference=%d", object))
	return err
}

// Ensure CloudHSMProvider implements the interfaces.
var (
	_ Provider        = (*CloudHSMProvider)(nil)
	_ SigningProvider = (*CloudHSMProvider)(nil)
)
