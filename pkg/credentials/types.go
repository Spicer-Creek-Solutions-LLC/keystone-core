// Package credentials provides secure credential management for proxy agents.
// It supports multiple credential types (SSH, SNMP, WinRM, REST) with
// encrypted storage and NATS-proxied delivery.
package credentials

import (
	"encoding/json"
	"errors"
	"time"
)

// CredentialType identifies the type of credential.
type CredentialType string

const (
	CredentialTypeSSHPassword CredentialType = "ssh_password"
	CredentialTypeSSHKey      CredentialType = "ssh_key"
	CredentialTypeSNMPv2c     CredentialType = "snmpv2c"
	CredentialTypeSNMPv3      CredentialType = "snmpv3"
	CredentialTypeWinRM       CredentialType = "winrm"
	CredentialTypeRESTBasic   CredentialType = "rest_basic"
	CredentialTypeRESTBearer  CredentialType = "rest_bearer"
	CredentialTypeRESTAPIKey  CredentialType = "rest_apikey"
	CredentialTypeRESTOAuth2  CredentialType = "rest_oauth2"
)

// String returns the string representation of the credential type.
func (c CredentialType) String() string {
	return string(c)
}

// Valid returns true if the credential type is valid.
func (c CredentialType) Valid() bool {
	switch c {
	case CredentialTypeSSHPassword, CredentialTypeSSHKey,
		CredentialTypeSNMPv2c, CredentialTypeSNMPv3,
		CredentialTypeWinRM,
		CredentialTypeRESTBasic, CredentialTypeRESTBearer,
		CredentialTypeRESTAPIKey, CredentialTypeRESTOAuth2:
		return true
	default:
		return false
	}
}

// Credential is the base interface for all credential types.
type Credential interface {
	// Type returns the credential type.
	Type() CredentialType
	// ID returns the credential identifier.
	ID() string
	// ExpiresAt returns when the credential expires (zero = no expiry).
	ExpiresAt() time.Time
	// IsExpired returns true if the credential has expired.
	IsExpired() bool
	// Validate validates the credential fields.
	Validate() error
	// Redact returns a copy with sensitive fields redacted.
	Redact() Credential
}

// BaseCredential contains common fields for all credentials.
type BaseCredential struct {
	// CredentialID is the unique identifier for this credential.
	CredentialID string `json:"id"`
	// CredentialType is the type of credential.
	CredentialType CredentialType `json:"type"`
	// Description is a human-readable description.
	Description string `json:"description,omitempty"`
	// Expires is when the credential expires.
	Expires time.Time `json:"expires,omitempty"`
	// CreatedAt is when the credential was created.
	CreatedAt time.Time `json:"created_at,omitempty"`
	// UpdatedAt is when the credential was last updated.
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	// Metadata contains additional metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ID returns the credential identifier.
func (b *BaseCredential) ID() string {
	return b.CredentialID
}

// Type returns the credential type.
func (b *BaseCredential) Type() CredentialType {
	return b.CredentialType
}

// ExpiresAt returns when the credential expires.
func (b *BaseCredential) ExpiresAt() time.Time {
	return b.Expires
}

// IsExpired returns true if the credential has expired.
func (b *BaseCredential) IsExpired() bool {
	if b.Expires.IsZero() {
		return false
	}
	return time.Now().After(b.Expires)
}

// SSHPasswordCredential contains SSH password authentication credentials.
type SSHPasswordCredential struct {
	BaseCredential
	Username string `json:"username"`
	Password string `json:"password"`
}

// Validate validates the credential.
func (c *SSHPasswordCredential) Validate() error {
	if c.CredentialID == "" {
		return errors.New("credential ID is required")
	}
	if c.Username == "" {
		return errors.New("username is required")
	}
	if c.Password == "" {
		return errors.New("password is required")
	}
	return nil
}

// Redact returns a copy with sensitive fields redacted.
func (c *SSHPasswordCredential) Redact() Credential {
	redacted := *c
	redacted.Password = "[REDACTED]"
	return &redacted
}

// SSHKeyCredential contains SSH key authentication credentials.
type SSHKeyCredential struct {
	BaseCredential
	Username   string `json:"username"`
	PrivateKey []byte `json:"private_key"`
	Passphrase string `json:"passphrase,omitempty"`
	PublicKey  []byte `json:"public_key,omitempty"`
}

// Validate validates the credential.
func (c *SSHKeyCredential) Validate() error {
	if c.CredentialID == "" {
		return errors.New("credential ID is required")
	}
	if c.Username == "" {
		return errors.New("username is required")
	}
	if len(c.PrivateKey) == 0 {
		return errors.New("private key is required")
	}
	return nil
}

// Redact returns a copy with sensitive fields redacted.
func (c *SSHKeyCredential) Redact() Credential {
	redacted := *c
	redacted.PrivateKey = []byte("[REDACTED]")
	redacted.Passphrase = "[REDACTED]"
	return &redacted
}

// SNMPv2cCredential contains SNMP v2c credentials.
type SNMPv2cCredential struct {
	BaseCredential
	Community string `json:"community"`
}

// Validate validates the credential.
func (c *SNMPv2cCredential) Validate() error {
	if c.CredentialID == "" {
		return errors.New("credential ID is required")
	}
	if c.Community == "" {
		return errors.New("community string is required")
	}
	return nil
}

// Redact returns a copy with sensitive fields redacted.
func (c *SNMPv2cCredential) Redact() Credential {
	redacted := *c
	redacted.Community = "[REDACTED]"
	return &redacted
}

// SNMPv3AuthProtocol defines SNMP v3 authentication protocols.
type SNMPv3AuthProtocol string

const (
	SNMPv3AuthNone   SNMPv3AuthProtocol = "none"
	SNMPv3AuthMD5    SNMPv3AuthProtocol = "md5"
	SNMPv3AuthSHA    SNMPv3AuthProtocol = "sha"
	SNMPv3AuthSHA224 SNMPv3AuthProtocol = "sha224"
	SNMPv3AuthSHA256 SNMPv3AuthProtocol = "sha256"
	SNMPv3AuthSHA384 SNMPv3AuthProtocol = "sha384"
	SNMPv3AuthSHA512 SNMPv3AuthProtocol = "sha512"
)

// SNMPv3PrivProtocol defines SNMP v3 privacy protocols.
type SNMPv3PrivProtocol string

const (
	SNMPv3PrivNone   SNMPv3PrivProtocol = "none"
	SNMPv3PrivDES    SNMPv3PrivProtocol = "des"
	SNMPv3PrivAES    SNMPv3PrivProtocol = "aes"
	SNMPv3PrivAES192 SNMPv3PrivProtocol = "aes192"
	SNMPv3PrivAES256 SNMPv3PrivProtocol = "aes256"
)

// SNMPv3SecurityLevel defines SNMP v3 security levels.
type SNMPv3SecurityLevel string

const (
	SNMPv3SecurityNoAuthNoPriv SNMPv3SecurityLevel = "noAuthNoPriv"
	SNMPv3SecurityAuthNoPriv   SNMPv3SecurityLevel = "authNoPriv"
	SNMPv3SecurityAuthPriv     SNMPv3SecurityLevel = "authPriv"
)

// SNMPv3Credential contains SNMP v3 credentials.
type SNMPv3Credential struct {
	BaseCredential
	Username         string              `json:"username"`
	SecurityLevel    SNMPv3SecurityLevel `json:"security_level"`
	AuthProtocol     SNMPv3AuthProtocol  `json:"auth_protocol,omitempty"`
	AuthPassword     string              `json:"auth_password,omitempty"`
	PrivacyProtocol  SNMPv3PrivProtocol  `json:"privacy_protocol,omitempty"`
	PrivacyPassword  string              `json:"privacy_password,omitempty"`
	ContextName      string              `json:"context_name,omitempty"`
	ContextEngineID  string              `json:"context_engine_id,omitempty"`
	SecurityEngineID string              `json:"security_engine_id,omitempty"`
}

// Validate validates the credential.
func (c *SNMPv3Credential) Validate() error {
	if c.CredentialID == "" {
		return errors.New("credential ID is required")
	}
	if c.Username == "" {
		return errors.New("username is required")
	}
	if c.SecurityLevel == "" {
		return errors.New("security level is required")
	}
	if c.SecurityLevel == SNMPv3SecurityAuthNoPriv || c.SecurityLevel == SNMPv3SecurityAuthPriv {
		if c.AuthProtocol == "" || c.AuthProtocol == SNMPv3AuthNone {
			return errors.New("auth protocol is required for authNoPriv/authPriv")
		}
		if c.AuthPassword == "" {
			return errors.New("auth password is required for authNoPriv/authPriv")
		}
	}
	if c.SecurityLevel == SNMPv3SecurityAuthPriv {
		if c.PrivacyProtocol == "" || c.PrivacyProtocol == SNMPv3PrivNone {
			return errors.New("privacy protocol is required for authPriv")
		}
		if c.PrivacyPassword == "" {
			return errors.New("privacy password is required for authPriv")
		}
	}
	return nil
}

// Redact returns a copy with sensitive fields redacted.
func (c *SNMPv3Credential) Redact() Credential {
	redacted := *c
	redacted.AuthPassword = "[REDACTED]"
	redacted.PrivacyPassword = "[REDACTED]"
	return &redacted
}

// WinRMCredential contains WinRM credentials.
type WinRMCredential struct {
	BaseCredential
	Username    string `json:"username"`
	Password    string `json:"password"`
	Domain      string `json:"domain,omitempty"`
	UseHTTPS    bool   `json:"use_https"`
	UseNTLM     bool   `json:"use_ntlm"`
	UseKerberos bool   `json:"use_kerberos"`
	CACert      []byte `json:"ca_cert,omitempty"`
	ClientCert  []byte `json:"client_cert,omitempty"`
	ClientKey   []byte `json:"client_key,omitempty"`
}

// Validate validates the credential.
func (c *WinRMCredential) Validate() error {
	if c.CredentialID == "" {
		return errors.New("credential ID is required")
	}
	if c.Username == "" {
		return errors.New("username is required")
	}
	if c.Password == "" {
		return errors.New("password is required")
	}
	return nil
}

// Redact returns a copy with sensitive fields redacted.
func (c *WinRMCredential) Redact() Credential {
	redacted := *c
	redacted.Password = "[REDACTED]"
	redacted.ClientKey = []byte("[REDACTED]")
	return &redacted
}

// RESTBasicCredential contains REST API basic auth credentials.
type RESTBasicCredential struct {
	BaseCredential
	Username string `json:"username"`
	Password string `json:"password"`
}

// Validate validates the credential.
func (c *RESTBasicCredential) Validate() error {
	if c.CredentialID == "" {
		return errors.New("credential ID is required")
	}
	if c.Username == "" {
		return errors.New("username is required")
	}
	if c.Password == "" {
		return errors.New("password is required")
	}
	return nil
}

// Redact returns a copy with sensitive fields redacted.
func (c *RESTBasicCredential) Redact() Credential {
	redacted := *c
	redacted.Password = "[REDACTED]"
	return &redacted
}

// RESTBearerCredential contains REST API bearer token credentials.
type RESTBearerCredential struct {
	BaseCredential
	Token string `json:"token"`
}

// Validate validates the credential.
func (c *RESTBearerCredential) Validate() error {
	if c.CredentialID == "" {
		return errors.New("credential ID is required")
	}
	if c.Token == "" {
		return errors.New("token is required")
	}
	return nil
}

// Redact returns a copy with sensitive fields redacted.
func (c *RESTBearerCredential) Redact() Credential {
	redacted := *c
	redacted.Token = "[REDACTED]"
	return &redacted
}

// RESTAPIKeyCredential contains REST API key credentials.
type RESTAPIKeyCredential struct {
	BaseCredential
	APIKey     string `json:"api_key"`
	HeaderName string `json:"header_name,omitempty"` // Default: X-API-Key
	QueryParam string `json:"query_param,omitempty"` // Alternative: use query parameter
}

// Validate validates the credential.
func (c *RESTAPIKeyCredential) Validate() error {
	if c.CredentialID == "" {
		return errors.New("credential ID is required")
	}
	if c.APIKey == "" {
		return errors.New("API key is required")
	}
	return nil
}

// Redact returns a copy with sensitive fields redacted.
func (c *RESTAPIKeyCredential) Redact() Credential {
	redacted := *c
	redacted.APIKey = "[REDACTED]"
	return &redacted
}

// RESTOAuth2Credential contains REST API OAuth2 credentials.
type RESTOAuth2Credential struct {
	BaseCredential
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	TokenURL     string   `json:"token_url"`
	Scopes       []string `json:"scopes,omitempty"`
	// Cached token (populated by credential provider)
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenExpiry  time.Time `json:"token_expiry,omitempty"`
}

// Validate validates the credential.
func (c *RESTOAuth2Credential) Validate() error {
	if c.CredentialID == "" {
		return errors.New("credential ID is required")
	}
	if c.ClientID == "" {
		return errors.New("client ID is required")
	}
	if c.ClientSecret == "" {
		return errors.New("client secret is required")
	}
	if c.TokenURL == "" {
		return errors.New("token URL is required")
	}
	return nil
}

// Redact returns a copy with sensitive fields redacted.
func (c *RESTOAuth2Credential) Redact() Credential {
	redacted := *c
	redacted.ClientSecret = "[REDACTED]"
	redacted.AccessToken = "[REDACTED]"
	redacted.RefreshToken = "[REDACTED]"
	return &redacted
}

// NeedsRefresh returns true if the OAuth2 token needs to be refreshed.
func (c *RESTOAuth2Credential) NeedsRefresh() bool {
	if c.AccessToken == "" {
		return true
	}
	if c.TokenExpiry.IsZero() {
		return false
	}
	// Refresh if expiring within 5 minutes
	return time.Now().Add(5 * time.Minute).After(c.TokenExpiry)
}

// CredentialRequest is a request to fetch credentials.
type CredentialRequest struct {
	// CredentialRef is the reference to the credential.
	CredentialRef string `json:"credential_ref"`
	// DeviceID is the device requesting the credential.
	DeviceID string `json:"device_id"`
	// ProxyAgentID is the proxy agent requesting the credential.
	ProxyAgentID string `json:"proxy_agent_id"`
	// RequestID is a unique identifier for this request.
	RequestID string `json:"request_id"`
	// RequestTime is when the request was made.
	RequestTime time.Time `json:"request_time"`
}

// CredentialResponse is a response containing credentials.
type CredentialResponse struct {
	// RequestID matches the request.
	RequestID string `json:"request_id"`
	// Credential is the encrypted credential data.
	Credential []byte `json:"credential"`
	// CredentialType is the type of credential.
	CredentialType CredentialType `json:"credential_type"`
	// ExpiresAt is when the credential expires.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// Error is any error that occurred.
	Error string `json:"error,omitempty"`
	// ResponseTime is when the response was generated.
	ResponseTime time.Time `json:"response_time"`
}

// Common errors.
var (
	ErrCredentialNotFound  = errors.New("credential not found")
	ErrCredentialExpired   = errors.New("credential expired")
	ErrInvalidCredential   = errors.New("invalid credential")
	ErrDecryptionFailed    = errors.New("decryption failed")
	ErrEncryptionFailed    = errors.New("encryption failed")
	ErrKeyExchangeFailed   = errors.New("key exchange failed")
	ErrInvalidCredentialRef = errors.New("invalid credential reference")
)

// ParseCredential parses a credential from JSON based on type.
func ParseCredential(credType CredentialType, data []byte) (Credential, error) {
	var cred Credential
	switch credType {
	case CredentialTypeSSHPassword:
		cred = &SSHPasswordCredential{}
	case CredentialTypeSSHKey:
		cred = &SSHKeyCredential{}
	case CredentialTypeSNMPv2c:
		cred = &SNMPv2cCredential{}
	case CredentialTypeSNMPv3:
		cred = &SNMPv3Credential{}
	case CredentialTypeWinRM:
		cred = &WinRMCredential{}
	case CredentialTypeRESTBasic:
		cred = &RESTBasicCredential{}
	case CredentialTypeRESTBearer:
		cred = &RESTBearerCredential{}
	case CredentialTypeRESTAPIKey:
		cred = &RESTAPIKeyCredential{}
	case CredentialTypeRESTOAuth2:
		cred = &RESTOAuth2Credential{}
	default:
		return nil, ErrInvalidCredential
	}

	if err := json.Unmarshal(data, cred); err != nil {
		return nil, err
	}

	return cred, nil
}
