// Package kms provides PKCS#11 interface for hardware HSM integration.
// This implementation uses a pure-Go approach compatible with Epic 13 (CGO Removal)
// by abstracting HSM operations through subprocess calls or network protocols.
package kms

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// PKCS#11 mechanism types (subset of common mechanisms).
type PKCS11Mechanism uint32

const (
	CKM_RSA_PKCS           PKCS11Mechanism = 0x00000001
	CKM_RSA_PKCS_KEY_PAIR_GEN PKCS11Mechanism = 0x00000000
	CKM_RSA_PKCS_OAEP      PKCS11Mechanism = 0x00000009
	CKM_AES_KEY_GEN        PKCS11Mechanism = 0x00001080
	CKM_AES_CBC            PKCS11Mechanism = 0x00001082
	CKM_AES_CBC_PAD        PKCS11Mechanism = 0x00001085
	CKM_AES_GCM            PKCS11Mechanism = 0x00001087
	CKM_AES_KEY_WRAP       PKCS11Mechanism = 0x00002109
	CKM_AES_KEY_WRAP_PAD   PKCS11Mechanism = 0x0000210A
	CKM_SHA256             PKCS11Mechanism = 0x00000250
	CKM_SHA384             PKCS11Mechanism = 0x00000260
	CKM_SHA512             PKCS11Mechanism = 0x00000270
	CKM_SHA256_RSA_PKCS    PKCS11Mechanism = 0x00000040
	CKM_SHA384_RSA_PKCS    PKCS11Mechanism = 0x00000041
	CKM_SHA512_RSA_PKCS    PKCS11Mechanism = 0x00000042
	CKM_ECDSA              PKCS11Mechanism = 0x00001041
	CKM_ECDSA_SHA256       PKCS11Mechanism = 0x00001043
	CKM_ECDSA_SHA384       PKCS11Mechanism = 0x00001044
)

// String returns the mechanism name.
func (m PKCS11Mechanism) String() string {
	names := map[PKCS11Mechanism]string{
		CKM_RSA_PKCS:              "CKM_RSA_PKCS",
		CKM_RSA_PKCS_KEY_PAIR_GEN: "CKM_RSA_PKCS_KEY_PAIR_GEN",
		CKM_RSA_PKCS_OAEP:         "CKM_RSA_PKCS_OAEP",
		CKM_AES_KEY_GEN:           "CKM_AES_KEY_GEN",
		CKM_AES_CBC:               "CKM_AES_CBC",
		CKM_AES_CBC_PAD:           "CKM_AES_CBC_PAD",
		CKM_AES_GCM:               "CKM_AES_GCM",
		CKM_AES_KEY_WRAP:          "CKM_AES_KEY_WRAP",
		CKM_AES_KEY_WRAP_PAD:      "CKM_AES_KEY_WRAP_PAD",
		CKM_SHA256:                "CKM_SHA256",
		CKM_SHA384:                "CKM_SHA384",
		CKM_SHA512:                "CKM_SHA512",
		CKM_SHA256_RSA_PKCS:       "CKM_SHA256_RSA_PKCS",
		CKM_SHA384_RSA_PKCS:       "CKM_SHA384_RSA_PKCS",
		CKM_SHA512_RSA_PKCS:       "CKM_SHA512_RSA_PKCS",
		CKM_ECDSA:                 "CKM_ECDSA",
		CKM_ECDSA_SHA256:          "CKM_ECDSA_SHA256",
		CKM_ECDSA_SHA384:          "CKM_ECDSA_SHA384",
	}
	if name, ok := names[m]; ok {
		return name
	}
	return fmt.Sprintf("CKM_UNKNOWN_0x%08X", uint32(m))
}

// PKCS#11 object classes.
type PKCS11ObjectClass uint32

const (
	CKO_DATA             PKCS11ObjectClass = 0x00000000
	CKO_CERTIFICATE      PKCS11ObjectClass = 0x00000001
	CKO_PUBLIC_KEY       PKCS11ObjectClass = 0x00000002
	CKO_PRIVATE_KEY      PKCS11ObjectClass = 0x00000003
	CKO_SECRET_KEY       PKCS11ObjectClass = 0x00000004
	CKO_HW_FEATURE       PKCS11ObjectClass = 0x00000005
	CKO_DOMAIN_PARAMETERS PKCS11ObjectClass = 0x00000006
)

// PKCS#11 key types.
type PKCS11KeyType uint32

const (
	CKK_RSA            PKCS11KeyType = 0x00000000
	CKK_DSA            PKCS11KeyType = 0x00000001
	CKK_DH             PKCS11KeyType = 0x00000002
	CKK_EC             PKCS11KeyType = 0x00000003
	CKK_GENERIC_SECRET PKCS11KeyType = 0x00000010
	CKK_AES            PKCS11KeyType = 0x0000001F
)

// PKCS#11 user types.
type PKCS11UserType uint32

const (
	CKU_SO              PKCS11UserType = 0
	CKU_USER            PKCS11UserType = 1
	CKU_CONTEXT_SPECIFIC PKCS11UserType = 2
)

// PKCS#11 session flags.
type PKCS11SessionFlags uint32

const (
	CKF_RW_SESSION    PKCS11SessionFlags = 0x00000002
	CKF_SERIAL_SESSION PKCS11SessionFlags = 0x00000004
)

// PKCS#11 return values.
type PKCS11ReturnValue uint32

const (
	CKR_OK                            PKCS11ReturnValue = 0x00000000
	CKR_CANCEL                        PKCS11ReturnValue = 0x00000001
	CKR_HOST_MEMORY                   PKCS11ReturnValue = 0x00000002
	CKR_SLOT_ID_INVALID               PKCS11ReturnValue = 0x00000003
	CKR_GENERAL_ERROR                 PKCS11ReturnValue = 0x00000005
	CKR_FUNCTION_FAILED               PKCS11ReturnValue = 0x00000006
	CKR_DEVICE_ERROR                  PKCS11ReturnValue = 0x00000030
	CKR_DEVICE_MEMORY                 PKCS11ReturnValue = 0x00000031
	CKR_DEVICE_REMOVED                PKCS11ReturnValue = 0x00000032
	CKR_KEY_HANDLE_INVALID            PKCS11ReturnValue = 0x00000060
	CKR_KEY_SIZE_RANGE                PKCS11ReturnValue = 0x00000062
	CKR_KEY_TYPE_INCONSISTENT         PKCS11ReturnValue = 0x00000063
	CKR_KEY_NOT_WRAPPABLE             PKCS11ReturnValue = 0x00000069
	CKR_KEY_UNEXTRACTABLE             PKCS11ReturnValue = 0x0000006A
	CKR_MECHANISM_INVALID             PKCS11ReturnValue = 0x00000070
	CKR_MECHANISM_PARAM_INVALID       PKCS11ReturnValue = 0x00000071
	CKR_OPERATION_ACTIVE              PKCS11ReturnValue = 0x00000090
	CKR_OPERATION_NOT_INITIALIZED     PKCS11ReturnValue = 0x00000091
	CKR_PIN_INCORRECT                 PKCS11ReturnValue = 0x000000A0
	CKR_PIN_LOCKED                    PKCS11ReturnValue = 0x000000A4
	CKR_SESSION_CLOSED                PKCS11ReturnValue = 0x000000B0
	CKR_SESSION_COUNT                 PKCS11ReturnValue = 0x000000B1
	CKR_SESSION_HANDLE_INVALID        PKCS11ReturnValue = 0x000000B3
	CKR_SESSION_READ_ONLY             PKCS11ReturnValue = 0x000000B5
	CKR_TOKEN_NOT_PRESENT             PKCS11ReturnValue = 0x000000E0
	CKR_TOKEN_NOT_RECOGNIZED          PKCS11ReturnValue = 0x000000E1
	CKR_USER_ALREADY_LOGGED_IN        PKCS11ReturnValue = 0x00000100
	CKR_USER_NOT_LOGGED_IN            PKCS11ReturnValue = 0x00000101
	CKR_USER_PIN_NOT_INITIALIZED      PKCS11ReturnValue = 0x00000102
	CKR_WRAPPED_KEY_INVALID           PKCS11ReturnValue = 0x00000110
	CKR_WRAPPED_KEY_LEN_RANGE         PKCS11ReturnValue = 0x00000112
	CKR_WRAPPING_KEY_HANDLE_INVALID   PKCS11ReturnValue = 0x00000113
	CKR_WRAPPING_KEY_SIZE_RANGE       PKCS11ReturnValue = 0x00000114
	CKR_WRAPPING_KEY_TYPE_INCONSISTENT PKCS11ReturnValue = 0x00000115
	CKR_RANDOM_SEED_NOT_SUPPORTED     PKCS11ReturnValue = 0x00000120
	CKR_BUFFER_TOO_SMALL              PKCS11ReturnValue = 0x00000150
	CKR_CRYPTOKI_NOT_INITIALIZED      PKCS11ReturnValue = 0x00000190
	CKR_CRYPTOKI_ALREADY_INITIALIZED  PKCS11ReturnValue = 0x00000191
)

// Error returns the error message for the return value.
func (r PKCS11ReturnValue) Error() string {
	messages := map[PKCS11ReturnValue]string{
		CKR_OK:                            "operation successful",
		CKR_CANCEL:                        "operation cancelled",
		CKR_HOST_MEMORY:                   "host memory allocation failed",
		CKR_SLOT_ID_INVALID:               "invalid slot ID",
		CKR_GENERAL_ERROR:                 "general error",
		CKR_FUNCTION_FAILED:               "function failed",
		CKR_DEVICE_ERROR:                  "device error",
		CKR_DEVICE_MEMORY:                 "device memory error",
		CKR_DEVICE_REMOVED:                "device removed",
		CKR_KEY_HANDLE_INVALID:            "invalid key handle",
		CKR_KEY_SIZE_RANGE:                "key size out of range",
		CKR_KEY_TYPE_INCONSISTENT:         "inconsistent key type",
		CKR_KEY_NOT_WRAPPABLE:             "key not wrappable",
		CKR_KEY_UNEXTRACTABLE:             "key unextractable",
		CKR_MECHANISM_INVALID:             "invalid mechanism",
		CKR_MECHANISM_PARAM_INVALID:       "invalid mechanism parameters",
		CKR_OPERATION_ACTIVE:              "operation already active",
		CKR_OPERATION_NOT_INITIALIZED:     "operation not initialized",
		CKR_PIN_INCORRECT:                 "incorrect PIN",
		CKR_PIN_LOCKED:                    "PIN locked",
		CKR_SESSION_CLOSED:                "session closed",
		CKR_SESSION_COUNT:                 "session count exceeded",
		CKR_SESSION_HANDLE_INVALID:        "invalid session handle",
		CKR_SESSION_READ_ONLY:             "session is read-only",
		CKR_TOKEN_NOT_PRESENT:             "token not present",
		CKR_TOKEN_NOT_RECOGNIZED:          "token not recognized",
		CKR_USER_ALREADY_LOGGED_IN:        "user already logged in",
		CKR_USER_NOT_LOGGED_IN:            "user not logged in",
		CKR_USER_PIN_NOT_INITIALIZED:      "user PIN not initialized",
		CKR_WRAPPED_KEY_INVALID:           "invalid wrapped key",
		CKR_WRAPPED_KEY_LEN_RANGE:         "wrapped key length out of range",
		CKR_WRAPPING_KEY_HANDLE_INVALID:   "invalid wrapping key handle",
		CKR_WRAPPING_KEY_SIZE_RANGE:       "wrapping key size out of range",
		CKR_WRAPPING_KEY_TYPE_INCONSISTENT: "inconsistent wrapping key type",
		CKR_RANDOM_SEED_NOT_SUPPORTED:     "random seed not supported",
		CKR_BUFFER_TOO_SMALL:              "buffer too small",
		CKR_CRYPTOKI_NOT_INITIALIZED:      "cryptoki not initialized",
		CKR_CRYPTOKI_ALREADY_INITIALIZED:  "cryptoki already initialized",
	}
	if msg, ok := messages[r]; ok {
		return fmt.Sprintf("PKCS#11 error: %s (0x%08X)", msg, uint32(r))
	}
	return fmt.Sprintf("PKCS#11 error: unknown error (0x%08X)", uint32(r))
}

// PKCS11Error wraps a PKCS#11 return value as a Go error.
type PKCS11Error struct {
	Code PKCS11ReturnValue
}

func (e *PKCS11Error) Error() string {
	return e.Code.Error()
}

// NewPKCS11Error creates a new PKCS#11 error.
func NewPKCS11Error(code PKCS11ReturnValue) error {
	if code == CKR_OK {
		return nil
	}
	return &PKCS11Error{Code: code}
}

// SlotInfo contains information about a PKCS#11 slot.
type SlotInfo struct {
	SlotID          uint32 `json:"slot_id"`
	SlotDescription string `json:"slot_description"`
	ManufacturerID  string `json:"manufacturer_id"`
	Flags           uint32 `json:"flags"`
	HardwareVersion string `json:"hardware_version"`
	FirmwareVersion string `json:"firmware_version"`
	TokenPresent    bool   `json:"token_present"`
}

// TokenInfo contains information about a PKCS#11 token.
type TokenInfo struct {
	Label              string    `json:"label"`
	ManufacturerID     string    `json:"manufacturer_id"`
	Model              string    `json:"model"`
	SerialNumber       string    `json:"serial_number"`
	Flags              uint32    `json:"flags"`
	MaxSessionCount    uint32    `json:"max_session_count"`
	SessionCount       uint32    `json:"session_count"`
	MaxRwSessionCount  uint32    `json:"max_rw_session_count"`
	RwSessionCount     uint32    `json:"rw_session_count"`
	MaxPinLen          uint32    `json:"max_pin_len"`
	MinPinLen          uint32    `json:"min_pin_len"`
	TotalPublicMemory  uint64    `json:"total_public_memory"`
	FreePublicMemory   uint64    `json:"free_public_memory"`
	TotalPrivateMemory uint64    `json:"total_private_memory"`
	FreePrivateMemory  uint64    `json:"free_private_memory"`
	HardwareVersion    string    `json:"hardware_version"`
	FirmwareVersion    string    `json:"firmware_version"`
	UTCTime            time.Time `json:"utc_time,omitempty"`
}

// MechanismInfo contains information about a PKCS#11 mechanism.
type MechanismInfo struct {
	Mechanism PKCS11Mechanism `json:"mechanism"`
	MinKeySize uint32          `json:"min_key_size"`
	MaxKeySize uint32          `json:"max_key_size"`
	Flags      uint32          `json:"flags"`
}

// ObjectHandle represents a PKCS#11 object handle.
type ObjectHandle uint32

// SessionHandle represents a PKCS#11 session handle.
type SessionHandle uint32

// Session represents an active PKCS#11 session.
type Session struct {
	Handle      SessionHandle `json:"handle"`
	SlotID      uint32        `json:"slot_id"`
	State       uint32        `json:"state"`
	Flags       uint32        `json:"flags"`
	DeviceError uint32        `json:"device_error"`
	CreatedAt   time.Time     `json:"created_at"`
	LastUsed    time.Time     `json:"last_used"`
}

// IsReadWrite returns true if the session is read-write.
func (s *Session) IsReadWrite() bool {
	return s.Flags&uint32(CKF_RW_SESSION) != 0
}

// KeyObject represents a key stored in the HSM.
type KeyObject struct {
	Handle     ObjectHandle      `json:"handle"`
	Class      PKCS11ObjectClass `json:"class"`
	KeyType    PKCS11KeyType     `json:"key_type"`
	Label      string            `json:"label"`
	ID         []byte            `json:"id"`
	Modulus    []byte            `json:"modulus,omitempty"`
	ModulusBits uint32           `json:"modulus_bits,omitempty"`
	Extractable bool             `json:"extractable"`
	Sensitive   bool             `json:"sensitive"`
	Token       bool             `json:"token"`
	Private     bool             `json:"private"`
	Encrypt     bool             `json:"encrypt"`
	Decrypt     bool             `json:"decrypt"`
	Sign        bool             `json:"sign"`
	Verify      bool             `json:"verify"`
	Wrap        bool             `json:"wrap"`
	Unwrap      bool             `json:"unwrap"`
	Derive      bool             `json:"derive"`
}

// PKCS11Config contains configuration for PKCS#11 HSM provider.
type PKCS11Config struct {
	ProviderConfig

	// SlotID is the HSM slot to use (0-based).
	SlotID uint32 `json:"slot_id"`

	// TokenLabel is the token label (alternative to SlotID).
	TokenLabel string `json:"token_label,omitempty"`

	// PIN is the user PIN for authentication.
	PIN string `json:"-"`

	// SOPIN is the security officer PIN (for administrative operations).
	SOPIN string `json:"-"`

	// MaxSessions is the maximum number of concurrent sessions.
	MaxSessions int `json:"max_sessions,omitempty"`

	// SessionIdleTimeout is the idle timeout for sessions.
	SessionIdleTimeout time.Duration `json:"session_idle_timeout,omitempty"`

	// Backend specifies the HSM backend implementation.
	Backend HSMBackend `json:"backend,omitempty"`

	// BackendConfig contains backend-specific configuration.
	BackendConfig map[string]string `json:"backend_config,omitempty"`
}

// HSMBackend specifies the HSM backend type.
type HSMBackend string

const (
	// HSMBackendSoftHSM uses SoftHSM2 for testing.
	HSMBackendSoftHSM HSMBackend = "softhsm"
	// HSMBackendThalesLuna uses Thales Luna HSM.
	HSMBackendThalesLuna HSMBackend = "thales_luna"
	// HSMBackendCloudHSM uses AWS CloudHSM.
	HSMBackendCloudHSM HSMBackend = "cloudhsm"
	// HSMBackendYubiHSM uses YubiHSM.
	HSMBackendYubiHSM HSMBackend = "yubihsm"
)

// DefaultPKCS11Config returns default PKCS#11 configuration.
func DefaultPKCS11Config() *PKCS11Config {
	return &PKCS11Config{
		ProviderConfig: ProviderConfig{
			Name:       "pkcs11",
			Type:       ProviderTypePKCS11,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
		MaxSessions:        10,
		SessionIdleTimeout: 5 * time.Minute,
		Backend:            HSMBackendSoftHSM,
	}
}

// PKCS11Interface defines the operations for PKCS#11 HSM access.
// Implementations can use subprocess calls, network protocols, or native libraries.
type PKCS11Interface interface {
	// Initialize initializes the PKCS#11 library.
	Initialize(ctx context.Context) error

	// Finalize finalizes the PKCS#11 library.
	Finalize(ctx context.Context) error

	// GetSlotList returns the list of available slots.
	GetSlotList(ctx context.Context, tokenPresent bool) ([]uint32, error)

	// GetSlotInfo returns information about a slot.
	GetSlotInfo(ctx context.Context, slotID uint32) (*SlotInfo, error)

	// GetTokenInfo returns information about a token.
	GetTokenInfo(ctx context.Context, slotID uint32) (*TokenInfo, error)

	// GetMechanismList returns the list of mechanisms supported by a slot.
	GetMechanismList(ctx context.Context, slotID uint32) ([]PKCS11Mechanism, error)

	// GetMechanismInfo returns information about a mechanism.
	GetMechanismInfo(ctx context.Context, slotID uint32, mechanism PKCS11Mechanism) (*MechanismInfo, error)

	// OpenSession opens a session to a token.
	OpenSession(ctx context.Context, slotID uint32, flags PKCS11SessionFlags) (*Session, error)

	// CloseSession closes a session.
	CloseSession(ctx context.Context, session SessionHandle) error

	// CloseAllSessions closes all sessions for a slot.
	CloseAllSessions(ctx context.Context, slotID uint32) error

	// Login authenticates to a token.
	Login(ctx context.Context, session SessionHandle, userType PKCS11UserType, pin string) error

	// Logout logs out from a token.
	Logout(ctx context.Context, session SessionHandle) error

	// GenerateKey generates a symmetric key.
	GenerateKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, template map[string]interface{}) (ObjectHandle, error)

	// GenerateKeyPair generates an asymmetric key pair.
	GenerateKeyPair(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, publicTemplate, privateTemplate map[string]interface{}) (publicKey, privateKey ObjectHandle, err error)

	// FindObjectsInit initializes a search for objects.
	FindObjectsInit(ctx context.Context, session SessionHandle, template map[string]interface{}) error

	// FindObjects continues a search for objects.
	FindObjects(ctx context.Context, session SessionHandle, maxObjects uint32) ([]ObjectHandle, error)

	// FindObjectsFinal finalizes a search for objects.
	FindObjectsFinal(ctx context.Context, session SessionHandle) error

	// GetAttributeValue gets attribute values for an object.
	GetAttributeValue(ctx context.Context, session SessionHandle, object ObjectHandle, attributes []string) (map[string]interface{}, error)

	// Encrypt encrypts data.
	Encrypt(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error)

	// Decrypt decrypts data.
	Decrypt(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error)

	// Sign signs data.
	Sign(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error)

	// Verify verifies a signature.
	Verify(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data, signature []byte) (bool, error)

	// WrapKey wraps a key.
	WrapKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, wrappingKey, keyToWrap ObjectHandle) ([]byte, error)

	// UnwrapKey unwraps a key.
	UnwrapKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, unwrappingKey ObjectHandle, wrappedKey []byte, template map[string]interface{}) (ObjectHandle, error)

	// GenerateRandom generates random bytes.
	GenerateRandom(ctx context.Context, session SessionHandle, length int) ([]byte, error)

	// DestroyObject destroys an object.
	DestroyObject(ctx context.Context, session SessionHandle, object ObjectHandle) error
}

// PKCS11Provider implements the Provider interface for PKCS#11 HSMs.
type PKCS11Provider struct {
	config     *PKCS11Config
	iface      PKCS11Interface
	slotInfo   *SlotInfo
	tokenInfo  *TokenInfo
	mechanisms map[PKCS11Mechanism]*MechanismInfo

	mu           sync.RWMutex
	sessions     []*Session
	sessionPool  chan *Session
	loggedIn     bool
	initialized  bool
	closed       bool

	keyCache     map[string]ObjectHandle
	keyCacheMu   sync.RWMutex
}

// NewPKCS11Provider creates a new PKCS#11 provider.
func NewPKCS11Provider(ctx context.Context, config *PKCS11Config, iface PKCS11Interface) (*PKCS11Provider, error) {
	if config == nil {
		config = DefaultPKCS11Config()
	}
	if iface == nil {
		return nil, errors.New("PKCS#11 interface is required")
	}

	p := &PKCS11Provider{
		config:      config,
		iface:       iface,
		mechanisms:  make(map[PKCS11Mechanism]*MechanismInfo),
		sessions:    make([]*Session, 0, config.MaxSessions),
		sessionPool: make(chan *Session, config.MaxSessions),
		keyCache:    make(map[string]ObjectHandle),
	}

	if err := p.initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize PKCS#11: %w", err)
	}

	return p, nil
}

// initialize sets up the PKCS#11 provider.
func (p *PKCS11Provider) initialize(ctx context.Context) error {
	if err := p.iface.Initialize(ctx); err != nil {
		return fmt.Errorf("C_Initialize failed: %w", err)
	}
	p.initialized = true

	slotID := p.config.SlotID
	if p.config.TokenLabel != "" {
		slots, err := p.iface.GetSlotList(ctx, true)
		if err != nil {
			return fmt.Errorf("failed to get slot list: %w", err)
		}
		found := false
		for _, slot := range slots {
			info, err := p.iface.GetTokenInfo(ctx, slot)
			if err != nil {
				continue
			}
			if info.Label == p.config.TokenLabel {
				slotID = slot
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("token with label %q not found", p.config.TokenLabel)
		}
	}

	slotInfo, err := p.iface.GetSlotInfo(ctx, slotID)
	if err != nil {
		return fmt.Errorf("failed to get slot info: %w", err)
	}
	p.slotInfo = slotInfo

	tokenInfo, err := p.iface.GetTokenInfo(ctx, slotID)
	if err != nil {
		return fmt.Errorf("failed to get token info: %w", err)
	}
	p.tokenInfo = tokenInfo

	mechanisms, err := p.iface.GetMechanismList(ctx, slotID)
	if err != nil {
		return fmt.Errorf("failed to get mechanism list: %w", err)
	}
	for _, mech := range mechanisms {
		info, err := p.iface.GetMechanismInfo(ctx, slotID, mech)
		if err != nil {
			continue
		}
		p.mechanisms[mech] = info
	}

	return nil
}

// Type returns the provider type.
func (p *PKCS11Provider) Type() ProviderType {
	return p.config.Type
}

// Name returns the provider instance name.
func (p *PKCS11Provider) Name() string {
	return p.config.Name
}

// Healthy checks if the provider is healthy.
func (p *PKCS11Provider) Healthy(ctx context.Context) bool {
	p.mu.RLock()
	if p.closed || !p.initialized {
		p.mu.RUnlock()
		return false
	}
	p.mu.RUnlock()

	_, err := p.iface.GetTokenInfo(ctx, p.config.SlotID)
	return err == nil
}

// getSession gets or creates a session.
func (p *PKCS11Provider) getSession(ctx context.Context) (*Session, error) {
	select {
	case session := <-p.sessionPool:
		session.LastUsed = time.Now()
		return session, nil
	default:
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.sessions) >= p.config.MaxSessions {
		p.mu.Unlock()
		select {
		case session := <-p.sessionPool:
			session.LastUsed = time.Now()
			p.mu.Lock()
			return session, nil
		case <-ctx.Done():
			p.mu.Lock()
			return nil, ctx.Err()
		}
	}

	session, err := p.iface.OpenSession(ctx, p.config.SlotID, CKF_RW_SESSION|CKF_SERIAL_SESSION)
	if err != nil {
		return nil, fmt.Errorf("failed to open session: %w", err)
	}

	if p.config.PIN != "" && !p.loggedIn {
		if err := p.iface.Login(ctx, session.Handle, CKU_USER, p.config.PIN); err != nil {
			var pkcs11Err *PKCS11Error
			if errors.As(err, &pkcs11Err) && pkcs11Err.Code == CKR_USER_ALREADY_LOGGED_IN {
				p.loggedIn = true
			} else {
				p.iface.CloseSession(ctx, session.Handle)
				return nil, fmt.Errorf("failed to login: %w", err)
			}
		} else {
			p.loggedIn = true
		}
	}

	p.sessions = append(p.sessions, session)
	return session, nil
}

// releaseSession returns a session to the pool.
func (p *PKCS11Provider) releaseSession(session *Session) {
	select {
	case p.sessionPool <- session:
	default:
	}
}

// findKey finds a key by label or ID.
func (p *PKCS11Provider) findKey(ctx context.Context, session *Session, keyID string) (ObjectHandle, error) {
	p.keyCacheMu.RLock()
	if handle, ok := p.keyCache[keyID]; ok {
		p.keyCacheMu.RUnlock()
		return handle, nil
	}
	p.keyCacheMu.RUnlock()

	template := map[string]interface{}{
		"CKA_LABEL": keyID,
	}
	if err := p.iface.FindObjectsInit(ctx, session.Handle, template); err != nil {
		return 0, err
	}
	defer p.iface.FindObjectsFinal(ctx, session.Handle)

	handles, err := p.iface.FindObjects(ctx, session.Handle, 1)
	if err != nil {
		return 0, err
	}
	if len(handles) == 0 {
		decoded, err := hex.DecodeString(keyID)
		if err == nil {
			template = map[string]interface{}{
				"CKA_ID": decoded,
			}
			if err := p.iface.FindObjectsInit(ctx, session.Handle, template); err != nil {
				return 0, err
			}
			defer p.iface.FindObjectsFinal(ctx, session.Handle)
			handles, err = p.iface.FindObjects(ctx, session.Handle, 1)
			if err != nil {
				return 0, err
			}
		}
	}
	if len(handles) == 0 {
		return 0, ErrKeyNotFound
	}

	p.keyCacheMu.Lock()
	p.keyCache[keyID] = handles[0]
	p.keyCacheMu.Unlock()

	return handles[0], nil
}

// GetKeyMetadata retrieves metadata for a key.
func (p *PKCS11Provider) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	session, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	defer p.releaseSession(session)

	handle, err := p.findKey(ctx, session, keyID)
	if err != nil {
		return nil, err
	}

	attrs, err := p.iface.GetAttributeValue(ctx, session.Handle, handle, []string{
		"CKA_CLASS", "CKA_KEY_TYPE", "CKA_LABEL", "CKA_ID",
		"CKA_MODULUS_BITS", "CKA_ENCRYPT", "CKA_DECRYPT",
		"CKA_SIGN", "CKA_VERIFY", "CKA_WRAP", "CKA_UNWRAP",
		"CKA_EXTRACTABLE", "CKA_SENSITIVE",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get key attributes: %w", err)
	}

	meta := &KeyMetadata{
		KeyID:    keyID,
		Provider: p.config.Type,
		Enabled:  true,
		Tags:     make(map[string]string),
	}

	if label, ok := attrs["CKA_LABEL"].(string); ok {
		meta.Alias = label
	}
	if keyType, ok := attrs["CKA_KEY_TYPE"].(PKCS11KeyType); ok {
		switch keyType {
		case CKK_AES:
			meta.KeyType = KeyTypeSymmetric
			meta.KeySpec = KeySpecAES256
		case CKK_RSA:
			meta.KeyType = KeyTypeAsymmetric
			if bits, ok := attrs["CKA_MODULUS_BITS"].(uint32); ok {
				if bits >= 4096 {
					meta.KeySpec = KeySpecRSA4096
				} else {
					meta.KeySpec = KeySpecRSA2048
				}
			}
		case CKK_EC:
			meta.KeyType = KeyTypeAsymmetric
			meta.KeySpec = KeySpecECCNISTP256
		}
	}

	canEncrypt, _ := attrs["CKA_ENCRYPT"].(bool)
	canDecrypt, _ := attrs["CKA_DECRYPT"].(bool)
	canSign, _ := attrs["CKA_SIGN"].(bool)
	canVerify, _ := attrs["CKA_VERIFY"].(bool)

	if canEncrypt || canDecrypt {
		meta.KeyUsage = KeyUsageEncryptDecrypt
	} else if canSign || canVerify {
		meta.KeyUsage = KeyUsageSignVerify
	}

	return meta, nil
}

// Encrypt encrypts plaintext data.
func (p *PKCS11Provider) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	session, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	defer p.releaseSession(session)

	handle, err := p.findKey(ctx, session, req.KeyID)
	if err != nil {
		return nil, err
	}

	mechanism := CKM_AES_GCM
	if _, ok := p.mechanisms[CKM_AES_GCM]; !ok {
		if _, ok := p.mechanisms[CKM_AES_CBC_PAD]; ok {
			mechanism = CKM_AES_CBC_PAD
		}
	}

	ciphertext, err := p.iface.Encrypt(ctx, session.Handle, mechanism, handle, req.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	return &EncryptResponse{
		Ciphertext: ciphertext,
		KeyID:      req.KeyID,
	}, nil
}

// Decrypt decrypts ciphertext data.
func (p *PKCS11Provider) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	session, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	defer p.releaseSession(session)

	handle, err := p.findKey(ctx, session, req.KeyID)
	if err != nil {
		return nil, err
	}

	mechanism := CKM_AES_GCM
	if _, ok := p.mechanisms[CKM_AES_GCM]; !ok {
		if _, ok := p.mechanisms[CKM_AES_CBC_PAD]; ok {
			mechanism = CKM_AES_CBC_PAD
		}
	}

	plaintext, err := p.iface.Decrypt(ctx, session.Handle, mechanism, handle, req.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return &DecryptResponse{
		Plaintext: plaintext,
		KeyID:     req.KeyID,
	}, nil
}

// GenerateDataKey generates a data encryption key.
func (p *PKCS11Provider) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	keyLen := 32
	if req.NumberOfBytes > 0 {
		keyLen = req.NumberOfBytes
	}

	plaintext := make([]byte, keyLen)
	if _, err := rand.Read(plaintext); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	session, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	defer p.releaseSession(session)

	handle, err := p.findKey(ctx, session, req.KeyID)
	if err != nil {
		return nil, err
	}

	mechanism := CKM_AES_KEY_WRAP
	if _, ok := p.mechanisms[CKM_AES_KEY_WRAP]; !ok {
		if _, ok := p.mechanisms[CKM_AES_GCM]; ok {
			mechanism = CKM_AES_GCM
		}
	}

	ciphertext, err := p.iface.Encrypt(ctx, session.Handle, mechanism, handle, plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap data key: %w", err)
	}

	dataKey := &DataKey{
		Plaintext:   plaintext,
		Ciphertext:  ciphertext,
		KeyID:       req.KeyID,
		Provider:    p.config.Type,
		KeySpec:     KeySpecAES256,
		GeneratedAt: time.Now(),
	}

	if req.WithoutPlaintext {
		dataKey.Zero()
		dataKey.Plaintext = nil
	}

	return dataKey, nil
}

// WrapKey wraps (encrypts) a key with the KMS key.
func (p *PKCS11Provider) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	session, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	defer p.releaseSession(session)

	handle, err := p.findKey(ctx, session, req.WrapperKeyID)
	if err != nil {
		return nil, err
	}

	mechanism := CKM_AES_KEY_WRAP_PAD
	if _, ok := p.mechanisms[CKM_AES_KEY_WRAP_PAD]; !ok {
		mechanism = CKM_AES_GCM
	}

	wrapped, err := p.iface.Encrypt(ctx, session.Handle, mechanism, handle, req.KeyToWrap)
	if err != nil {
		return nil, fmt.Errorf("key wrapping failed: %w", err)
	}

	return &WrapKeyResponse{
		WrappedKey:   wrapped,
		WrapperKeyID: req.WrapperKeyID,
	}, nil
}

// UnwrapKey unwraps (decrypts) a key with the KMS key.
func (p *PKCS11Provider) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	session, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	defer p.releaseSession(session)

	handle, err := p.findKey(ctx, session, req.WrapperKeyID)
	if err != nil {
		return nil, err
	}

	mechanism := CKM_AES_KEY_WRAP_PAD
	if _, ok := p.mechanisms[CKM_AES_KEY_WRAP_PAD]; !ok {
		mechanism = CKM_AES_GCM
	}

	plaintext, err := p.iface.Decrypt(ctx, session.Handle, mechanism, handle, req.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("key unwrapping failed: %w", err)
	}

	return &UnwrapKeyResponse{
		PlaintextKey: plaintext,
		WrapperKeyID: req.WrapperKeyID,
	}, nil
}

// Close closes the provider connection.
func (p *PKCS11Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	ctx := context.Background()
	close(p.sessionPool)
	for session := range p.sessionPool {
		if p.loggedIn {
			p.iface.Logout(ctx, session.Handle)
		}
		p.iface.CloseSession(ctx, session.Handle)
	}

	for _, session := range p.sessions {
		if p.loggedIn {
			p.iface.Logout(ctx, session.Handle)
		}
		p.iface.CloseSession(ctx, session.Handle)
	}
	p.sessions = nil
	p.loggedIn = false

	if p.initialized {
		p.iface.Finalize(ctx)
		p.initialized = false
	}

	return nil
}

// Sign signs data with the HSM key.
func (p *PKCS11Provider) Sign(ctx context.Context, req *SignRequest) (*SignResponse, error) {
	session, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	defer p.releaseSession(session)

	handle, err := p.findKey(ctx, session, req.KeyID)
	if err != nil {
		return nil, err
	}

	mechanism := CKM_SHA256_RSA_PKCS
	switch req.Algorithm {
	case "RSASSA_PKCS1_V1_5_SHA_256":
		mechanism = CKM_SHA256_RSA_PKCS
	case "RSASSA_PKCS1_V1_5_SHA_384":
		mechanism = CKM_SHA384_RSA_PKCS
	case "RSASSA_PKCS1_V1_5_SHA_512":
		mechanism = CKM_SHA512_RSA_PKCS
	case "ECDSA_SHA_256":
		mechanism = CKM_ECDSA_SHA256
	case "ECDSA_SHA_384":
		mechanism = CKM_ECDSA_SHA384
	}

	signature, err := p.iface.Sign(ctx, session.Handle, mechanism, handle, req.Message)
	if err != nil {
		return nil, fmt.Errorf("signing failed: %w", err)
	}

	return &SignResponse{
		Signature: signature,
		KeyID:     req.KeyID,
		Algorithm: req.Algorithm,
	}, nil
}

// Verify verifies a signature with the HSM key.
func (p *PKCS11Provider) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	session, err := p.getSession(ctx)
	if err != nil {
		return nil, err
	}
	defer p.releaseSession(session)

	handle, err := p.findKey(ctx, session, req.KeyID)
	if err != nil {
		return nil, err
	}

	mechanism := CKM_SHA256_RSA_PKCS
	switch req.Algorithm {
	case "RSASSA_PKCS1_V1_5_SHA_256":
		mechanism = CKM_SHA256_RSA_PKCS
	case "RSASSA_PKCS1_V1_5_SHA_384":
		mechanism = CKM_SHA384_RSA_PKCS
	case "RSASSA_PKCS1_V1_5_SHA_512":
		mechanism = CKM_SHA512_RSA_PKCS
	case "ECDSA_SHA_256":
		mechanism = CKM_ECDSA_SHA256
	case "ECDSA_SHA_384":
		mechanism = CKM_ECDSA_SHA384
	}

	valid, err := p.iface.Verify(ctx, session.Handle, mechanism, handle, req.Message, req.Signature)
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	return &VerifyResponse{
		Valid: valid,
		KeyID: req.KeyID,
	}, nil
}

// Ensure PKCS11Provider implements the interfaces.
var (
	_ Provider        = (*PKCS11Provider)(nil)
	_ SigningProvider = (*PKCS11Provider)(nil)
)
