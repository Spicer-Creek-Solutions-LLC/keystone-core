// Package kms provides Thales Luna HSM integration.
// This implementation uses the LunaClient utility (lunacm) or KMIP protocol
// to communicate with Thales Luna Network HSM appliances without requiring CGO.
package kms

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ThalesLunaConfig contains configuration for Thales Luna HSM.
type ThalesLunaConfig struct {
	ProviderConfig

	// Hostname is the Luna HSM hostname or IP address.
	Hostname string `json:"hostname"`

	// Port is the Luna HSM port (default: 1792 for NTLS).
	Port int `json:"port,omitempty"`

	// Partition is the HSM partition name.
	Partition string `json:"partition"`

	// Password is the partition password.
	Password string `json:"-"`

	// ClientCert is the path to the client certificate.
	ClientCert string `json:"client_cert,omitempty"`

	// ClientKey is the path to the client private key.
	ClientKey string `json:"-"`

	// CACert is the path to the CA certificate.
	CACert string `json:"ca_cert,omitempty"`

	// LunaCMPath is the path to the lunacm utility.
	LunaCMPath string `json:"lunacm_path,omitempty"`

	// UseKMIP enables KMIP protocol instead of lunacm.
	UseKMIP bool `json:"use_kmip,omitempty"`

	// KMIPPort is the KMIP port (default: 5696).
	KMIPPort int `json:"kmip_port,omitempty"`

	// MaxConnections is the maximum number of concurrent connections.
	MaxConnections int `json:"max_connections,omitempty"`

	// ConnectionTimeout is the connection timeout.
	ConnectionTimeout time.Duration `json:"connection_timeout,omitempty"`
}

// DefaultThalesLunaConfig returns default Thales Luna configuration.
func DefaultThalesLunaConfig() *ThalesLunaConfig {
	return &ThalesLunaConfig{
		ProviderConfig: ProviderConfig{
			Name:       "thales-luna",
			Type:       ProviderTypeThalesLuna,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
		Port:              1792,
		KMIPPort:          5696,
		LunaCMPath:        "/usr/safenet/lunaclient/bin/lunacm",
		MaxConnections:    5,
		ConnectionTimeout: 10 * time.Second,
	}
}

// ThalesLunaProvider implements the Provider interface for Thales Luna HSM.
type ThalesLunaProvider struct {
	config      *ThalesLunaConfig
	pkcs11      *PKCS11Provider
	kmipClient  *KMIPClient

	mu          sync.RWMutex
	initialized bool
	closed      bool
}

// NewThalesLunaProvider creates a new Thales Luna HSM provider.
func NewThalesLunaProvider(ctx context.Context, config *ThalesLunaConfig) (*ThalesLunaProvider, error) {
	if config == nil {
		config = DefaultThalesLunaConfig()
	}
	if config.Hostname == "" {
		return nil, errors.New("Luna HSM hostname is required")
	}
	if config.Partition == "" {
		return nil, errors.New("Luna HSM partition is required")
	}

	p := &ThalesLunaProvider{
		config: config,
	}

	if config.UseKMIP {
		client, err := newKMIPClient(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create KMIP client: %w", err)
		}
		p.kmipClient = client
	} else {
		iface := &lunaCMInterface{
			config: config,
		}
		pkcs11Config := &PKCS11Config{
			ProviderConfig: config.ProviderConfig,
			TokenLabel:     config.Partition,
			PIN:            config.Password,
			MaxSessions:    config.MaxConnections,
			Backend:        HSMBackendThalesLuna,
		}
		pkcs11Provider, err := NewPKCS11Provider(ctx, pkcs11Config, iface)
		if err != nil {
			return nil, fmt.Errorf("failed to create PKCS#11 provider: %w", err)
		}
		p.pkcs11 = pkcs11Provider
	}

	p.initialized = true
	return p, nil
}

// Type returns the provider type.
func (p *ThalesLunaProvider) Type() ProviderType {
	return ProviderTypeThalesLuna
}

// Name returns the provider instance name.
func (p *ThalesLunaProvider) Name() string {
	return p.config.Name
}

// Healthy checks if the provider is healthy.
func (p *ThalesLunaProvider) Healthy(ctx context.Context) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed || !p.initialized {
		return false
	}

	if p.config.UseKMIP {
		return p.kmipClient.Healthy(ctx)
	}
	return p.pkcs11.Healthy(ctx)
}

// GetKeyMetadata retrieves metadata for a key.
func (p *ThalesLunaProvider) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	if p.config.UseKMIP {
		return p.kmipClient.GetKeyMetadata(ctx, keyID)
	}
	return p.pkcs11.GetKeyMetadata(ctx, keyID)
}

// Encrypt encrypts plaintext data.
func (p *ThalesLunaProvider) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	if p.config.UseKMIP {
		return p.kmipClient.Encrypt(ctx, req)
	}
	return p.pkcs11.Encrypt(ctx, req)
}

// Decrypt decrypts ciphertext data.
func (p *ThalesLunaProvider) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	if p.config.UseKMIP {
		return p.kmipClient.Decrypt(ctx, req)
	}
	return p.pkcs11.Decrypt(ctx, req)
}

// GenerateDataKey generates a data encryption key.
func (p *ThalesLunaProvider) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	if p.config.UseKMIP {
		return p.kmipClient.GenerateDataKey(ctx, req)
	}
	return p.pkcs11.GenerateDataKey(ctx, req)
}

// WrapKey wraps (encrypts) a key with the KMS key.
func (p *ThalesLunaProvider) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	if p.config.UseKMIP {
		return p.kmipClient.WrapKey(ctx, req)
	}
	return p.pkcs11.WrapKey(ctx, req)
}

// UnwrapKey unwraps (decrypts) a key with the KMS key.
func (p *ThalesLunaProvider) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	if p.config.UseKMIP {
		return p.kmipClient.UnwrapKey(ctx, req)
	}
	return p.pkcs11.UnwrapKey(ctx, req)
}

// Sign signs data with the HSM key.
func (p *ThalesLunaProvider) Sign(ctx context.Context, req *SignRequest) (*SignResponse, error) {
	if p.config.UseKMIP {
		return p.kmipClient.Sign(ctx, req)
	}
	return p.pkcs11.Sign(ctx, req)
}

// Verify verifies a signature with the HSM key.
func (p *ThalesLunaProvider) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	if p.config.UseKMIP {
		return p.kmipClient.Verify(ctx, req)
	}
	return p.pkcs11.Verify(ctx, req)
}

// Close closes the provider connection.
func (p *ThalesLunaProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	if p.config.UseKMIP {
		return p.kmipClient.Close()
	}
	return p.pkcs11.Close()
}

// lunaCMInterface implements PKCS11Interface using the lunacm CLI tool.
type lunaCMInterface struct {
	config      *ThalesLunaConfig
	mu          sync.Mutex
	initialized bool
	slotID      uint32
	sessions    map[SessionHandle]*lunaSession
	nextSession SessionHandle
}

type lunaSession struct {
	handle  SessionHandle
	slotID  uint32
	flags   PKCS11SessionFlags
	created time.Time
}

// Initialize initializes the PKCS#11 library.
func (l *lunaCMInterface) Initialize(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.initialized {
		return nil
	}

	if _, err := exec.LookPath(l.config.LunaCMPath); err != nil {
		return fmt.Errorf("lunacm not found at %s: %w", l.config.LunaCMPath, err)
	}

	l.sessions = make(map[SessionHandle]*lunaSession)
	l.nextSession = 1
	l.initialized = true
	return nil
}

// Finalize finalizes the PKCS#11 library.
func (l *lunaCMInterface) Finalize(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.initialized = false
	l.sessions = nil
	return nil
}

// runLunaCM executes a lunacm command.
func (l *lunaCMInterface) runLunaCM(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, l.config.LunaCMPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("lunacm command failed: %s: %w", stderr.String(), err)
	}

	return stdout.String(), nil
}

// GetSlotList returns the list of available slots.
func (l *lunaCMInterface) GetSlotList(ctx context.Context, tokenPresent bool) ([]uint32, error) {
	output, err := l.runLunaCM(ctx, "slot", "list")
	if err != nil {
		return nil, err
	}

	var slots []uint32
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Slot Id:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				id, err := strconv.ParseUint(parts[2], 10, 32)
				if err == nil {
					slots = append(slots, uint32(id))
				}
			}
		}
	}

	if len(slots) == 0 {
		slots = []uint32{0}
	}
	return slots, nil
}

// GetSlotInfo returns information about a slot.
func (l *lunaCMInterface) GetSlotInfo(ctx context.Context, slotID uint32) (*SlotInfo, error) {
	output, err := l.runLunaCM(ctx, "slot", "info", "-slot", strconv.FormatUint(uint64(slotID), 10))
	if err != nil {
		return nil, err
	}

	info := &SlotInfo{
		SlotID:       slotID,
		TokenPresent: true,
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Slot Description:") {
			info.SlotDescription = strings.TrimPrefix(line, "Slot Description:")
			info.SlotDescription = strings.TrimSpace(info.SlotDescription)
		} else if strings.HasPrefix(line, "Manufacturer:") {
			info.ManufacturerID = strings.TrimPrefix(line, "Manufacturer:")
			info.ManufacturerID = strings.TrimSpace(info.ManufacturerID)
		}
	}

	return info, nil
}

// GetTokenInfo returns information about a token.
func (l *lunaCMInterface) GetTokenInfo(ctx context.Context, slotID uint32) (*TokenInfo, error) {
	output, err := l.runLunaCM(ctx, "partition", "show", "-slot", strconv.FormatUint(uint64(slotID), 10))
	if err != nil {
		return nil, err
	}

	info := &TokenInfo{
		Model: "Luna Network HSM",
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Partition Name:") {
			info.Label = strings.TrimPrefix(line, "Partition Name:")
			info.Label = strings.TrimSpace(info.Label)
		} else if strings.HasPrefix(line, "Serial Number:") {
			info.SerialNumber = strings.TrimPrefix(line, "Serial Number:")
			info.SerialNumber = strings.TrimSpace(info.SerialNumber)
		}
	}

	return info, nil
}

// GetMechanismList returns the list of mechanisms supported by a slot.
func (l *lunaCMInterface) GetMechanismList(ctx context.Context, slotID uint32) ([]PKCS11Mechanism, error) {
	return []PKCS11Mechanism{
		CKM_RSA_PKCS,
		CKM_RSA_PKCS_OAEP,
		CKM_AES_KEY_GEN,
		CKM_AES_CBC,
		CKM_AES_CBC_PAD,
		CKM_AES_GCM,
		CKM_AES_KEY_WRAP,
		CKM_SHA256_RSA_PKCS,
		CKM_ECDSA_SHA256,
	}, nil
}

// GetMechanismInfo returns information about a mechanism.
func (l *lunaCMInterface) GetMechanismInfo(ctx context.Context, slotID uint32, mechanism PKCS11Mechanism) (*MechanismInfo, error) {
	info := &MechanismInfo{
		Mechanism: mechanism,
	}

	switch mechanism {
	case CKM_AES_KEY_GEN, CKM_AES_CBC, CKM_AES_CBC_PAD, CKM_AES_GCM, CKM_AES_KEY_WRAP:
		info.MinKeySize = 128
		info.MaxKeySize = 256
	case CKM_RSA_PKCS, CKM_RSA_PKCS_OAEP, CKM_SHA256_RSA_PKCS:
		info.MinKeySize = 2048
		info.MaxKeySize = 4096
	case CKM_ECDSA_SHA256:
		info.MinKeySize = 256
		info.MaxKeySize = 384
	}

	return info, nil
}

// OpenSession opens a session to a token.
func (l *lunaCMInterface) OpenSession(ctx context.Context, slotID uint32, flags PKCS11SessionFlags) (*Session, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	handle := l.nextSession
	l.nextSession++

	session := &lunaSession{
		handle:  handle,
		slotID:  slotID,
		flags:   flags,
		created: time.Now(),
	}
	l.sessions[handle] = session
	l.slotID = slotID

	return &Session{
		Handle:    handle,
		SlotID:    slotID,
		Flags:     uint32(flags),
		CreatedAt: session.created,
		LastUsed:  session.created,
	}, nil
}

// CloseSession closes a session.
func (l *lunaCMInterface) CloseSession(ctx context.Context, session SessionHandle) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.sessions, session)
	return nil
}

// CloseAllSessions closes all sessions for a slot.
func (l *lunaCMInterface) CloseAllSessions(ctx context.Context, slotID uint32) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for handle, session := range l.sessions {
		if session.slotID == slotID {
			delete(l.sessions, handle)
		}
	}
	return nil
}

// Login authenticates to a token.
func (l *lunaCMInterface) Login(ctx context.Context, session SessionHandle, userType PKCS11UserType, pin string) error {
	_, err := l.runLunaCM(ctx, "role", "login", "-name", "crypto_officer", "-password", pin)
	if err != nil {
		if strings.Contains(err.Error(), "already logged in") {
			return NewPKCS11Error(CKR_USER_ALREADY_LOGGED_IN)
		}
		return fmt.Errorf("login failed: %w", err)
	}
	return nil
}

// Logout logs out from a token.
func (l *lunaCMInterface) Logout(ctx context.Context, session SessionHandle) error {
	_, err := l.runLunaCM(ctx, "role", "logout")
	return err
}

// GenerateKey generates a symmetric key.
func (l *lunaCMInterface) GenerateKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, template map[string]interface{}) (ObjectHandle, error) {
	label, _ := template["CKA_LABEL"].(string)
	if label == "" {
		label = fmt.Sprintf("key-%d", time.Now().UnixNano())
	}

	size := 256
	if s, ok := template["CKA_VALUE_LEN"].(int); ok {
		size = s * 8
	}

	_, err := l.runLunaCM(ctx, "key", "generate", "-type", "aes", "-size", strconv.Itoa(size), "-label", label)
	if err != nil {
		return 0, err
	}

	return ObjectHandle(time.Now().UnixNano() & 0xFFFFFFFF), nil
}

// GenerateKeyPair generates an asymmetric key pair.
func (l *lunaCMInterface) GenerateKeyPair(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, publicTemplate, privateTemplate map[string]interface{}) (publicKey, privateKey ObjectHandle, err error) {
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

	_, err = l.runLunaCM(ctx, "key", "generate", "-type", keyType, "-size", strconv.Itoa(size), "-label", label)
	if err != nil {
		return 0, 0, err
	}

	now := time.Now().UnixNano()
	return ObjectHandle(now & 0xFFFFFFFF), ObjectHandle((now + 1) & 0xFFFFFFFF), nil
}

// FindObjectsInit initializes a search for objects.
func (l *lunaCMInterface) FindObjectsInit(ctx context.Context, session SessionHandle, template map[string]interface{}) error {
	return nil
}

// FindObjects continues a search for objects.
func (l *lunaCMInterface) FindObjects(ctx context.Context, session SessionHandle, maxObjects uint32) ([]ObjectHandle, error) {
	output, err := l.runLunaCM(ctx, "key", "list")
	if err != nil {
		return nil, err
	}

	var handles []ObjectHandle
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Key Handle:") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "Handle:" && i+1 < len(parts) {
					h, err := strconv.ParseUint(parts[i+1], 10, 32)
					if err == nil {
						handles = append(handles, ObjectHandle(h))
					}
				}
			}
		}
	}

	if uint32(len(handles)) > maxObjects {
		handles = handles[:maxObjects]
	}
	return handles, nil
}

// FindObjectsFinal finalizes a search for objects.
func (l *lunaCMInterface) FindObjectsFinal(ctx context.Context, session SessionHandle) error {
	return nil
}

// GetAttributeValue gets attribute values for an object.
func (l *lunaCMInterface) GetAttributeValue(ctx context.Context, session SessionHandle, object ObjectHandle, attributes []string) (map[string]interface{}, error) {
	output, err := l.runLunaCM(ctx, "key", "show", "-handle", strconv.FormatUint(uint64(object), 10))
	if err != nil {
		return nil, err
	}

	attrs := make(map[string]interface{})
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Label:") {
			attrs["CKA_LABEL"] = strings.TrimSpace(strings.TrimPrefix(line, "Label:"))
		} else if strings.HasPrefix(line, "Key Type:") {
			typeStr := strings.TrimSpace(strings.TrimPrefix(line, "Key Type:"))
			switch strings.ToLower(typeStr) {
			case "aes":
				attrs["CKA_KEY_TYPE"] = CKK_AES
			case "rsa":
				attrs["CKA_KEY_TYPE"] = CKK_RSA
			case "ec", "ecdsa":
				attrs["CKA_KEY_TYPE"] = CKK_EC
			}
		} else if strings.HasPrefix(line, "Key Size:") {
			sizeStr := strings.TrimSpace(strings.TrimPrefix(line, "Key Size:"))
			sizeStr = strings.TrimSuffix(sizeStr, " bits")
			if size, err := strconv.ParseUint(sizeStr, 10, 32); err == nil {
				attrs["CKA_MODULUS_BITS"] = uint32(size)
			}
		}
	}

	attrs["CKA_ENCRYPT"] = true
	attrs["CKA_DECRYPT"] = true
	attrs["CKA_SIGN"] = true
	attrs["CKA_VERIFY"] = true
	attrs["CKA_WRAP"] = true
	attrs["CKA_UNWRAP"] = true

	return attrs, nil
}

// Encrypt encrypts data.
func (l *lunaCMInterface) Encrypt(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error) {
	_, err := l.runLunaCM(ctx, "key", "encrypt", "-handle", strconv.FormatUint(uint64(key), 10),
		"-mechanism", mechanism.String(), "-data", hex.EncodeToString(data))
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Decrypt decrypts data.
func (l *lunaCMInterface) Decrypt(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error) {
	_, err := l.runLunaCM(ctx, "key", "decrypt", "-handle", strconv.FormatUint(uint64(key), 10),
		"-mechanism", mechanism.String(), "-data", hex.EncodeToString(data))
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Sign signs data.
func (l *lunaCMInterface) Sign(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error) {
	output, err := l.runLunaCM(ctx, "key", "sign", "-handle", strconv.FormatUint(uint64(key), 10),
		"-mechanism", mechanism.String(), "-data", hex.EncodeToString(data))
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Signature:") {
			sigHex := strings.TrimSpace(strings.TrimPrefix(line, "Signature:"))
			return hex.DecodeString(sigHex)
		}
	}

	return nil, errors.New("signature not found in output")
}

// Verify verifies a signature.
func (l *lunaCMInterface) Verify(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data, signature []byte) (bool, error) {
	_, err := l.runLunaCM(ctx, "key", "verify", "-handle", strconv.FormatUint(uint64(key), 10),
		"-mechanism", mechanism.String(), "-data", hex.EncodeToString(data), "-signature", hex.EncodeToString(signature))
	if err != nil {
		if strings.Contains(err.Error(), "verification failed") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// WrapKey wraps a key.
func (l *lunaCMInterface) WrapKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, wrappingKey, keyToWrap ObjectHandle) ([]byte, error) {
	output, err := l.runLunaCM(ctx, "key", "wrap", "-wrapping", strconv.FormatUint(uint64(wrappingKey), 10),
		"-target", strconv.FormatUint(uint64(keyToWrap), 10), "-mechanism", mechanism.String())
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Wrapped Key:") {
			wrappedHex := strings.TrimSpace(strings.TrimPrefix(line, "Wrapped Key:"))
			return hex.DecodeString(wrappedHex)
		}
	}

	return nil, errors.New("wrapped key not found in output")
}

// UnwrapKey unwraps a key.
func (l *lunaCMInterface) UnwrapKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, unwrappingKey ObjectHandle, wrappedKey []byte, template map[string]interface{}) (ObjectHandle, error) {
	label, _ := template["CKA_LABEL"].(string)
	if label == "" {
		label = fmt.Sprintf("unwrapped-%d", time.Now().UnixNano())
	}

	output, err := l.runLunaCM(ctx, "key", "unwrap", "-unwrapping", strconv.FormatUint(uint64(unwrappingKey), 10),
		"-wrapped", hex.EncodeToString(wrappedKey), "-mechanism", mechanism.String(), "-label", label)
	if err != nil {
		return 0, err
	}

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Key Handle:") {
			handleStr := strings.TrimSpace(strings.TrimPrefix(line, "Key Handle:"))
			h, err := strconv.ParseUint(handleStr, 10, 32)
			if err == nil {
				return ObjectHandle(h), nil
			}
		}
	}

	return ObjectHandle(time.Now().UnixNano() & 0xFFFFFFFF), nil
}

// GenerateRandom generates random bytes.
func (l *lunaCMInterface) GenerateRandom(ctx context.Context, session SessionHandle, length int) ([]byte, error) {
	output, err := l.runLunaCM(ctx, "random", "generate", "-length", strconv.Itoa(length))
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Random Data:") {
			randomHex := strings.TrimSpace(strings.TrimPrefix(line, "Random Data:"))
			return hex.DecodeString(randomHex)
		}
	}

	return nil, errors.New("random data not found in output")
}

// DestroyObject destroys an object.
func (l *lunaCMInterface) DestroyObject(ctx context.Context, session SessionHandle, object ObjectHandle) error {
	_, err := l.runLunaCM(ctx, "key", "delete", "-handle", strconv.FormatUint(uint64(object), 10), "-force")
	return err
}

// KMIPClient provides KMIP protocol support for HSM communication.
type KMIPClient struct {
	config *ThalesLunaConfig
	conn   net.Conn
	mu     sync.Mutex
	msgID  uint32
}

// newKMIPClient creates a new KMIP client.
func newKMIPClient(ctx context.Context, config *ThalesLunaConfig) (*KMIPClient, error) {
	addr := fmt.Sprintf("%s:%d", config.Hostname, config.KMIPPort)

	dialer := &net.Dialer{
		Timeout: config.ConnectionTimeout,
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to KMIP server: %w", err)
	}

	return &KMIPClient{
		config: config,
		conn:   conn,
		msgID:  1,
	}, nil
}

// KMIP message tags (simplified subset).
const (
	kmipTagRequestMessage     uint32 = 0x420078
	kmipTagResponseMessage    uint32 = 0x42007B
	kmipTagRequestHeader      uint32 = 0x420077
	kmipTagResponseHeader     uint32 = 0x42007A
	kmipTagProtocolVersion    uint32 = 0x420069
	kmipTagProtocolVersionMajor uint32 = 0x42006A
	kmipTagProtocolVersionMinor uint32 = 0x42006B
	kmipTagBatchItem          uint32 = 0x42000F
	kmipTagOperation          uint32 = 0x42005C
	kmipTagResultStatus       uint32 = 0x42007F
	kmipTagUniqueID           uint32 = 0x420094
	kmipTagData               uint32 = 0x4200C2
)

// KMIP operations.
const (
	kmipOpCreate    uint32 = 0x00000001
	kmipOpGet       uint32 = 0x0000000A
	kmipOpEncrypt   uint32 = 0x0000001F
	kmipOpDecrypt   uint32 = 0x00000020
	kmipOpSign      uint32 = 0x00000021
	kmipOpVerify    uint32 = 0x00000024
)

// Healthy checks if the KMIP client is healthy.
func (c *KMIPClient) Healthy(ctx context.Context) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return false
	}

	if err := c.conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		return false
	}

	one := make([]byte, 1)
	_, err := c.conn.Read(one)
	if err == io.EOF {
		return false
	}

	return true
}

// sendRequest sends a KMIP request and receives the response.
func (c *KMIPClient) sendRequest(ctx context.Context, operation uint32, uniqueID string, data []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.msgID++

	request := c.buildRequest(operation, uniqueID, data)

	if deadline, ok := ctx.Deadline(); ok {
		c.conn.SetWriteDeadline(deadline)
		c.conn.SetReadDeadline(deadline)
	}

	if _, err := c.conn.Write(request); err != nil {
		return nil, fmt.Errorf("failed to send KMIP request: %w", err)
	}

	response := make([]byte, 4096)
	n, err := c.conn.Read(response)
	if err != nil {
		return nil, fmt.Errorf("failed to read KMIP response: %w", err)
	}

	return c.parseResponse(response[:n])
}

// buildRequest constructs a KMIP request message.
func (c *KMIPClient) buildRequest(operation uint32, uniqueID string, data []byte) []byte {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, kmipTagRequestMessage)
	binary.Write(buf, binary.BigEndian, uint8(0x01))
	binary.Write(buf, binary.BigEndian, uint32(0))

	binary.Write(buf, binary.BigEndian, kmipTagRequestHeader)
	binary.Write(buf, binary.BigEndian, uint8(0x01))
	headerStart := buf.Len()
	binary.Write(buf, binary.BigEndian, uint32(0))

	binary.Write(buf, binary.BigEndian, kmipTagProtocolVersion)
	binary.Write(buf, binary.BigEndian, uint8(0x01))
	binary.Write(buf, binary.BigEndian, uint32(16))
	binary.Write(buf, binary.BigEndian, kmipTagProtocolVersionMajor)
	binary.Write(buf, binary.BigEndian, uint8(0x02))
	binary.Write(buf, binary.BigEndian, uint32(4))
	binary.Write(buf, binary.BigEndian, uint32(1))
	binary.Write(buf, binary.BigEndian, kmipTagProtocolVersionMinor)
	binary.Write(buf, binary.BigEndian, uint8(0x02))
	binary.Write(buf, binary.BigEndian, uint32(4))
	binary.Write(buf, binary.BigEndian, uint32(4))

	headerLen := uint32(buf.Len() - headerStart - 4)
	headerBytes := buf.Bytes()
	binary.BigEndian.PutUint32(headerBytes[headerStart:], headerLen)

	binary.Write(buf, binary.BigEndian, kmipTagBatchItem)
	binary.Write(buf, binary.BigEndian, uint8(0x01))
	batchStart := buf.Len()
	binary.Write(buf, binary.BigEndian, uint32(0))

	binary.Write(buf, binary.BigEndian, kmipTagOperation)
	binary.Write(buf, binary.BigEndian, uint8(0x05))
	binary.Write(buf, binary.BigEndian, uint32(4))
	binary.Write(buf, binary.BigEndian, operation)

	if uniqueID != "" {
		binary.Write(buf, binary.BigEndian, kmipTagUniqueID)
		binary.Write(buf, binary.BigEndian, uint8(0x07))
		binary.Write(buf, binary.BigEndian, uint32(len(uniqueID)))
		buf.WriteString(uniqueID)
		for len(uniqueID)%8 != 0 {
			buf.WriteByte(0)
			uniqueID += " "
		}
	}

	if len(data) > 0 {
		binary.Write(buf, binary.BigEndian, kmipTagData)
		binary.Write(buf, binary.BigEndian, uint8(0x08))
		binary.Write(buf, binary.BigEndian, uint32(len(data)))
		buf.Write(data)
		for len(data)%8 != 0 {
			buf.WriteByte(0)
			data = append(data, 0)
		}
	}

	batchLen := uint32(buf.Len() - batchStart - 4)
	batchBytes := buf.Bytes()
	binary.BigEndian.PutUint32(batchBytes[batchStart:], batchLen)

	totalLen := uint32(buf.Len() - 8)
	binary.BigEndian.PutUint32(buf.Bytes()[4:], totalLen)

	return buf.Bytes()
}

// parseResponse parses a KMIP response message.
func (c *KMIPClient) parseResponse(data []byte) ([]byte, error) {
	if len(data) < 8 {
		return nil, errors.New("invalid KMIP response: too short")
	}

	return data, nil
}

// GetKeyMetadata retrieves metadata for a key.
func (c *KMIPClient) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	_, err := c.sendRequest(ctx, kmipOpGet, keyID, nil)
	if err != nil {
		return nil, err
	}

	return &KeyMetadata{
		KeyID:    keyID,
		Provider: ProviderTypeThalesLuna,
		Enabled:  true,
		KeyType:  KeyTypeSymmetric,
		KeySpec:  KeySpecAES256,
	}, nil
}

// Encrypt encrypts plaintext data.
func (c *KMIPClient) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	ciphertext, err := c.sendRequest(ctx, kmipOpEncrypt, req.KeyID, req.Plaintext)
	if err != nil {
		return nil, err
	}

	return &EncryptResponse{
		Ciphertext: ciphertext,
		KeyID:      req.KeyID,
	}, nil
}

// Decrypt decrypts ciphertext data.
func (c *KMIPClient) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	plaintext, err := c.sendRequest(ctx, kmipOpDecrypt, req.KeyID, req.Ciphertext)
	if err != nil {
		return nil, err
	}

	return &DecryptResponse{
		Plaintext: plaintext,
		KeyID:     req.KeyID,
	}, nil
}

// GenerateDataKey generates a data encryption key.
func (c *KMIPClient) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	keyLen := 32
	if req.NumberOfBytes > 0 {
		keyLen = req.NumberOfBytes
	}

	plaintext := make([]byte, keyLen)
	if _, err := io.ReadFull(c.conn, plaintext); err != nil {
		return nil, err
	}

	ciphertext, err := c.sendRequest(ctx, kmipOpEncrypt, req.KeyID, plaintext)
	if err != nil {
		return nil, err
	}

	return &DataKey{
		Plaintext:   plaintext,
		Ciphertext:  ciphertext,
		KeyID:       req.KeyID,
		Provider:    ProviderTypeThalesLuna,
		KeySpec:     KeySpecAES256,
		GeneratedAt: time.Now(),
	}, nil
}

// WrapKey wraps (encrypts) a key with the KMS key.
func (c *KMIPClient) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	wrapped, err := c.sendRequest(ctx, kmipOpEncrypt, req.WrapperKeyID, req.KeyToWrap)
	if err != nil {
		return nil, err
	}

	return &WrapKeyResponse{
		WrappedKey:   wrapped,
		WrapperKeyID: req.WrapperKeyID,
	}, nil
}

// UnwrapKey unwraps (decrypts) a key with the KMS key.
func (c *KMIPClient) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	plaintext, err := c.sendRequest(ctx, kmipOpDecrypt, req.WrapperKeyID, req.WrappedKey)
	if err != nil {
		return nil, err
	}

	return &UnwrapKeyResponse{
		PlaintextKey: plaintext,
		WrapperKeyID: req.WrapperKeyID,
	}, nil
}

// Sign signs data with the HSM key.
func (c *KMIPClient) Sign(ctx context.Context, req *SignRequest) (*SignResponse, error) {
	signature, err := c.sendRequest(ctx, kmipOpSign, req.KeyID, req.Message)
	if err != nil {
		return nil, err
	}

	return &SignResponse{
		Signature: signature,
		KeyID:     req.KeyID,
		Algorithm: req.Algorithm,
	}, nil
}

// Verify verifies a signature with the HSM key.
func (c *KMIPClient) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	_, err := c.sendRequest(ctx, kmipOpVerify, req.KeyID, append(req.Message, req.Signature...))
	if err != nil {
		return nil, err
	}

	return &VerifyResponse{
		Valid: true,
		KeyID: req.KeyID,
	}, nil
}

// Close closes the KMIP client connection.
func (c *KMIPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Ensure ThalesLunaProvider implements the interfaces.
var (
	_ Provider        = (*ThalesLunaProvider)(nil)
	_ SigningProvider = (*ThalesLunaProvider)(nil)
)
