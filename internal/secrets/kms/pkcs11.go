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

// PKCS11Mechanism represents PKCS#11 mechanism types (subset of common mechanisms).
type PKCS11Mechanism uint32

// CkmRSAPKCS constants define the PKCS#11 mechanisms.
const (
	CkmRSAPKCS           PKCS11Mechanism = 0x00000001
	CkmRSAPKCSKeyPairGen PKCS11Mechanism = 0x00000000
	CkmRSAPKCSOAEP       PKCS11Mechanism = 0x00000009
	CkmAESKeyGen         PKCS11Mechanism = 0x00001080
	CkmAESCBC            PKCS11Mechanism = 0x00001082
	CkmAESCBCPad         PKCS11Mechanism = 0x00001085
	CkmAESGCM            PKCS11Mechanism = 0x00001087
	CkmAESKeyWrap        PKCS11Mechanism = 0x00002109
	CkmAESKeyWrapPad     PKCS11Mechanism = 0x0000210A
	CkmSHA256            PKCS11Mechanism = 0x00000250
	CkmSHA384            PKCS11Mechanism = 0x00000260
	CkmSHA512            PKCS11Mechanism = 0x00000270
	CkmSHA256RSAPKCS     PKCS11Mechanism = 0x00000040
	CkmSHA384RSAPKCS     PKCS11Mechanism = 0x00000041
	CkmSHA512RSAPKCS     PKCS11Mechanism = 0x00000042
	CkmECDSA             PKCS11Mechanism = 0x00001041
	CkmECDSASHA256       PKCS11Mechanism = 0x00001043
	CkmECDSASHA384       PKCS11Mechanism = 0x00001044
)

// String returns the mechanism name.
func (m PKCS11Mechanism) String() string {
	names := map[PKCS11Mechanism]string{
		CkmRSAPKCS:           "CKM_RSA_PKCS",
		CkmRSAPKCSKeyPairGen: "CKM_RSA_PKCS_KEY_PAIR_GEN",
		CkmRSAPKCSOAEP:       "CKM_RSA_PKCS_OAEP",
		CkmAESKeyGen:         "CKM_AES_KEY_GEN",
		CkmAESCBC:            "CKM_AES_CBC",
		CkmAESCBCPad:         "CKM_AES_CBC_PAD",
		CkmAESGCM:            "CKM_AES_GCM",
		CkmAESKeyWrap:        "CKM_AES_KEY_WRAP",
		CkmAESKeyWrapPad:     "CKM_AES_KEY_WRAP_PAD",
		CkmSHA256:            "CKM_SHA256",
		CkmSHA384:            "CKM_SHA384",
		CkmSHA512:            "CKM_SHA512",
		CkmSHA256RSAPKCS:     "CKM_SHA256_RSA_PKCS",
		CkmSHA384RSAPKCS:     "CKM_SHA384_RSA_PKCS",
		CkmSHA512RSAPKCS:     "CKM_SHA512_RSA_PKCS",
		CkmECDSA:             "CKM_ECDSA",
		CkmECDSASHA256:       "CKM_ECDSA_SHA256",
		CkmECDSASHA384:       "CKM_ECDSA_SHA384",
	}
	if name, ok := names[m]; ok {
		return name
	}
	return fmt.Sprintf("CKM_UNKNOWN_0x%08X", uint32(m))
}

// PKCS11ObjectClass represents PKCS#11 object classes.
type PKCS11ObjectClass uint32

// CkoData constants define the PKCS#11 object classes.
const (
	CkoData             PKCS11ObjectClass = 0x00000000
	CkoCertificate      PKCS11ObjectClass = 0x00000001
	CkoPublicKey        PKCS11ObjectClass = 0x00000002
	CkoPrivateKey       PKCS11ObjectClass = 0x00000003
	CkoSecretKey        PKCS11ObjectClass = 0x00000004
	CkoHWFeature        PKCS11ObjectClass = 0x00000005
	CkoDomainParameters PKCS11ObjectClass = 0x00000006
)

// PKCS11KeyType represents PKCS#11 key types.
type PKCS11KeyType uint32

// CkkRSA constants define the PKCS#11 key types.
const (
	CkkRSA           PKCS11KeyType = 0x00000000
	CkkDSA           PKCS11KeyType = 0x00000001
	CkkDH            PKCS11KeyType = 0x00000002
	CkkEC            PKCS11KeyType = 0x00000003
	CkkGenericSecret PKCS11KeyType = 0x00000010
	CkkAES           PKCS11KeyType = 0x0000001F
)

// PKCS11UserType represents PKCS#11 user types.
type PKCS11UserType uint32

// CkuSO constants define the PKCS#11 user types.
const (
	CkuSO              PKCS11UserType = 0
	CkuUser            PKCS11UserType = 1
	CkuContextSpecific PKCS11UserType = 2
)

// PKCS11SessionFlags represents PKCS#11 session flags.
type PKCS11SessionFlags uint32

// CkfRWSession constants define the PKCS#11 session flags.
const (
	CkfRWSession     PKCS11SessionFlags = 0x00000002
	CkfSerialSession PKCS11SessionFlags = 0x00000004
)

// PKCS11ReturnError represents PKCS#11 return values.
type PKCS11ReturnError uint32

// ErrCkrOK constants define the PKCS#11 return codes.
const (
	ErrCkrOK                           PKCS11ReturnError = 0x00000000
	ErrCkrCancel                       PKCS11ReturnError = 0x00000001
	ErrCkrHostMemory                   PKCS11ReturnError = 0x00000002
	ErrCkrSlotIDInvalid                PKCS11ReturnError = 0x00000003
	ErrCkrGeneralError                 PKCS11ReturnError = 0x00000005
	ErrCkrFunctionFailed               PKCS11ReturnError = 0x00000006
	ErrCkrDeviceError                  PKCS11ReturnError = 0x00000030
	ErrCkrDeviceMemory                 PKCS11ReturnError = 0x00000031
	ErrCkrDeviceRemoved                PKCS11ReturnError = 0x00000032
	ErrCkrKeyHandleInvalid             PKCS11ReturnError = 0x00000060
	ErrCkrKeySizeRange                 PKCS11ReturnError = 0x00000062
	ErrCkrKeyTypeInconsistent          PKCS11ReturnError = 0x00000063
	ErrCkrKeyNotWrappable              PKCS11ReturnError = 0x00000069
	ErrCkrKeyUnextractable             PKCS11ReturnError = 0x0000006A
	ErrCkrMechanismInvalid             PKCS11ReturnError = 0x00000070
	ErrCkrMechanismParamInvalid        PKCS11ReturnError = 0x00000071
	ErrCkrOperationActive              PKCS11ReturnError = 0x00000090
	ErrCkrOperationNotInitialized      PKCS11ReturnError = 0x00000091
	ErrCkrPINIncorrect                 PKCS11ReturnError = 0x000000A0
	ErrCkrPINLocked                    PKCS11ReturnError = 0x000000A4
	ErrCkrSessionClosed                PKCS11ReturnError = 0x000000B0
	ErrCkrSessionCount                 PKCS11ReturnError = 0x000000B1
	ErrCkrSessionHandleInvalid         PKCS11ReturnError = 0x000000B3
	ErrCkrSessionReadOnly              PKCS11ReturnError = 0x000000B5
	ErrCkrTokenNotPresent              PKCS11ReturnError = 0x000000E0
	ErrCkrTokenNotRecognized           PKCS11ReturnError = 0x000000E1
	ErrCkrUserAlreadyLoggedIn          PKCS11ReturnError = 0x00000100
	ErrCkrUserNotLoggedIn              PKCS11ReturnError = 0x00000101
	ErrCkrUserPINNotInitialized        PKCS11ReturnError = 0x00000102
	ErrCkrWrappedKeyInvalid            PKCS11ReturnError = 0x00000110
	ErrCkrWrappedKeyLenRange           PKCS11ReturnError = 0x00000112
	ErrCkrWrappingKeyHandleInvalid     PKCS11ReturnError = 0x00000113
	ErrCkrWrappingKeySizeRange         PKCS11ReturnError = 0x00000114
	ErrCkrWrappingKeyTypeInconsistent  PKCS11ReturnError = 0x00000115
	ErrCkrRandomSeedNotSupported       PKCS11ReturnError = 0x00000120
	ErrCkrBufferTooSmall               PKCS11ReturnError = 0x00000150
	ErrCkrCryptokiNotInitialized       PKCS11ReturnError = 0x00000190
	ErrCkrCryptokiAlreadyInitialized   PKCS11ReturnError = 0x00000191
)

// Error returns the error message for the return value.
func (r PKCS11ReturnError) Error() string {
	messages := map[PKCS11ReturnError]string{
		ErrCkrOK:                          "operation successful",
		ErrCkrCancel:                      "operation cancelled",
		ErrCkrHostMemory:                  "host memory allocation failed",
		ErrCkrSlotIDInvalid:               "invalid slot ID",
		ErrCkrGeneralError:                "general error",
		ErrCkrFunctionFailed:              "function failed",
		ErrCkrDeviceError:                 "device error",
		ErrCkrDeviceMemory:                "device memory error",
		ErrCkrDeviceRemoved:               "device removed",
		ErrCkrKeyHandleInvalid:            "invalid key handle",
		ErrCkrKeySizeRange:                "key size out of range",
		ErrCkrKeyTypeInconsistent:         "inconsistent key type",
		ErrCkrKeyNotWrappable:             "key not wrappable",
		ErrCkrKeyUnextractable:            "key unextractable",
		ErrCkrMechanismInvalid:            "invalid mechanism",
		ErrCkrMechanismParamInvalid:       "invalid mechanism parameters",
		ErrCkrOperationActive:             "operation already active",
		ErrCkrOperationNotInitialized:     "operation not initialized",
		ErrCkrPINIncorrect:                "incorrect PIN",
		ErrCkrPINLocked:                   "PIN locked",
		ErrCkrSessionClosed:               "session closed",
		ErrCkrSessionCount:                "session count exceeded",
		ErrCkrSessionHandleInvalid:        "invalid session handle",
		ErrCkrSessionReadOnly:             "session is read-only",
		ErrCkrTokenNotPresent:             "token not present",
		ErrCkrTokenNotRecognized:          "token not recognized",
		ErrCkrUserAlreadyLoggedIn:         "user already logged in",
		ErrCkrUserNotLoggedIn:             "user not logged in",
		ErrCkrUserPINNotInitialized:       "user PIN not initialized",
		ErrCkrWrappedKeyInvalid:           "invalid wrapped key",
		ErrCkrWrappedKeyLenRange:          "wrapped key length out of range",
		ErrCkrWrappingKeyHandleInvalid:    "invalid wrapping key handle",
		ErrCkrWrappingKeySizeRange:        "wrapping key size out of range",
		ErrCkrWrappingKeyTypeInconsistent: "inconsistent wrapping key type",
		ErrCkrRandomSeedNotSupported:      "random seed not supported",
		ErrCkrBufferTooSmall:              "buffer too small",
		ErrCkrCryptokiNotInitialized:      "cryptoki not initialized",
		ErrCkrCryptokiAlreadyInitialized:  "cryptoki already initialized",
	}
	if msg, ok := messages[r]; ok {
		return fmt.Sprintf("PKCS#11 error: %s (0x%08X)", msg, uint32(r))
	}
	return fmt.Sprintf("PKCS#11 error: unknown error (0x%08X)", uint32(r))
}

// PKCS11Error wraps a PKCS#11 return value as a Go error.
type PKCS11Error struct {
	Code PKCS11ReturnError
}

func (e *PKCS11Error) Error() string {
	return e.Code.Error()
}

// NewPKCS11Error creates a new PKCS#11 error.
func NewPKCS11Error(code PKCS11ReturnError) error {
	if code == ErrCkrOK {
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
	Mechanism  PKCS11Mechanism `json:"mechanism"`
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
	return s.Flags&uint32(CkfRWSession) != 0
}

// KeyObject represents a key stored in the HSM.
type KeyObject struct {
	Handle      ObjectHandle      `json:"handle"`
	Class       PKCS11ObjectClass `json:"class"`
	KeyType     PKCS11KeyType     `json:"key_type"`
	Label       string            `json:"label"`
	ID          []byte            `json:"id"`
	Modulus     []byte            `json:"modulus,omitempty"`
	ModulusBits uint32            `json:"modulus_bits,omitempty"`
	Extractable bool              `json:"extractable"`
	Sensitive   bool              `json:"sensitive"`
	Token       bool              `json:"token"`
	Private     bool              `json:"private"`
	Encrypt     bool              `json:"encrypt"`
	Decrypt     bool              `json:"decrypt"`
	Sign        bool              `json:"sign"`
	Verify      bool              `json:"verify"`
	Wrap        bool              `json:"wrap"`
	Unwrap      bool              `json:"unwrap"`
	Derive      bool              `json:"derive"`
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

	mu          sync.RWMutex
	sessions    []*Session
	sessionPool chan *Session
	loggedIn    bool
	initialized bool
	closed      bool

	keyCache   map[string]ObjectHandle
	keyCacheMu sync.RWMutex
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

	session, err := p.iface.OpenSession(ctx, p.config.SlotID, CkfRWSession|CkfSerialSession)
	if err != nil {
		return nil, fmt.Errorf("failed to open session: %w", err)
	}

	if p.config.PIN != "" && !p.loggedIn {
		if err := p.iface.Login(ctx, session.Handle, CkuUser, p.config.PIN); err != nil {
			var pkcs11Err *PKCS11Error
			if errors.As(err, &pkcs11Err) && pkcs11Err.Code == ErrCkrUserAlreadyLoggedIn {
				p.loggedIn = true
			} else {
				_ = p.iface.CloseSession(ctx, session.Handle) //nolint:errcheck // best-effort cleanup
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
	defer func() { _ = p.iface.FindObjectsFinal(ctx, session.Handle) }() //nolint:errcheck // best-effort cleanup

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
			defer func() { _ = p.iface.FindObjectsFinal(ctx, session.Handle) }() //nolint:errcheck // best-effort cleanup
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
		case CkkAES:
			meta.KeyType = KeyTypeSymmetric
			meta.KeySpec = KeySpecAES256
		case CkkRSA:
			meta.KeyType = KeyTypeAsymmetric
			if bits, ok := attrs["CKA_MODULUS_BITS"].(uint32); ok {
				if bits >= 4096 {
					meta.KeySpec = KeySpecRSA4096
				} else {
					meta.KeySpec = KeySpecRSA2048
				}
			}
		case CkkEC:
			meta.KeyType = KeyTypeAsymmetric
			meta.KeySpec = KeySpecECCNISTP256
		default:
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

	mechanism := CkmAESGCM
	if _, ok := p.mechanisms[CkmAESGCM]; !ok {
		if _, ok := p.mechanisms[CkmAESCBCPad]; ok {
			mechanism = CkmAESCBCPad
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

	mechanism := CkmAESGCM
	if _, ok := p.mechanisms[CkmAESGCM]; !ok {
		if _, ok := p.mechanisms[CkmAESCBCPad]; ok {
			mechanism = CkmAESCBCPad
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

	mechanism := CkmAESKeyWrap
	if _, ok := p.mechanisms[CkmAESKeyWrap]; !ok {
		if _, ok := p.mechanisms[CkmAESGCM]; ok {
			mechanism = CkmAESGCM
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

	mechanism := CkmAESKeyWrapPad
	if _, ok := p.mechanisms[CkmAESKeyWrapPad]; !ok {
		mechanism = CkmAESGCM
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

	mechanism := CkmAESKeyWrapPad
	if _, ok := p.mechanisms[CkmAESKeyWrapPad]; !ok {
		mechanism = CkmAESGCM
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
			_ = p.iface.Logout(ctx, session.Handle) //nolint:errcheck // best-effort cleanup
		}
		_ = p.iface.CloseSession(ctx, session.Handle) //nolint:errcheck // best-effort cleanup
	}

	for _, session := range p.sessions {
		if p.loggedIn {
			_ = p.iface.Logout(ctx, session.Handle) //nolint:errcheck // best-effort cleanup
		}
		_ = p.iface.CloseSession(ctx, session.Handle) //nolint:errcheck // best-effort cleanup
	}
	p.sessions = nil
	p.loggedIn = false

	if p.initialized {
		_ = p.iface.Finalize(ctx) //nolint:errcheck // best-effort cleanup
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

	mechanism := CkmSHA256RSAPKCS
	switch req.Algorithm {
	case "RSASSA_PKCS1_V1_5_SHA_256":
		mechanism = CkmSHA256RSAPKCS
	case "RSASSA_PKCS1_V1_5_SHA_384":
		mechanism = CkmSHA384RSAPKCS
	case "RSASSA_PKCS1_V1_5_SHA_512":
		mechanism = CkmSHA512RSAPKCS
	case "ECDSA_SHA_256":
		mechanism = CkmECDSASHA256
	case "ECDSA_SHA_384":
		mechanism = CkmECDSASHA384
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

	mechanism := CkmSHA256RSAPKCS
	switch req.Algorithm {
	case "RSASSA_PKCS1_V1_5_SHA_256":
		mechanism = CkmSHA256RSAPKCS
	case "RSASSA_PKCS1_V1_5_SHA_384":
		mechanism = CkmSHA384RSAPKCS
	case "RSASSA_PKCS1_V1_5_SHA_512":
		mechanism = CkmSHA512RSAPKCS
	case "ECDSA_SHA_256":
		mechanism = CkmECDSASHA256
	case "ECDSA_SHA_384":
		mechanism = CkmECDSASHA384
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
