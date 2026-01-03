package identity

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// SVIDIssuerConfig contains configuration for the SVID issuer.
type SVIDIssuerConfig struct {
	// CA is the CA manager for signing certificates.
	CA *CAManager

	// TrustDomain is the SPIFFE trust domain.
	TrustDomain string

	// DefaultTTL is the default SVID time-to-live.
	DefaultTTL time.Duration

	// MaxTTL is the maximum allowed SVID TTL.
	MaxTTL time.Duration

	// KeyType is the key type for generated SVIDs.
	// Default: "ecdsa-p256"
	KeyType string
}

// SVIDIssuerService issues and manages SVIDs.
type SVIDIssuerService struct {
	config *SVIDIssuerConfig

	// JWT signing key
	jwtSigningKey crypto.PrivateKey
	jwtKeyID      string

	mu sync.RWMutex
}

// NewSVIDIssuerService creates a new SVID issuer service.
func NewSVIDIssuerService(config *SVIDIssuerConfig) (*SVIDIssuerService, error) {
	if config == nil {
		return nil, fmt.Errorf("config required")
	}

	if config.CA == nil {
		return nil, fmt.Errorf("CA manager required")
	}

	if config.TrustDomain == "" {
		return nil, fmt.Errorf("trust domain required")
	}

	// Set defaults
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 1 * time.Hour
	}
	if config.MaxTTL == 0 {
		config.MaxTTL = 24 * time.Hour
	}
	if config.KeyType == "" {
		config.KeyType = "ecdsa-p256"
	}

	// Generate JWT signing key
	jwtKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT signing key: %w", err)
	}

	// Generate key ID from public key
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&jwtKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	keyHash := sha256.Sum256(pubKeyBytes)
	keyID := base64.RawURLEncoding.EncodeToString(keyHash[:8])

	return &SVIDIssuerService{
		config:        config,
		jwtSigningKey: jwtKey,
		jwtKeyID:      keyID,
	}, nil
}

// IssueX509SVID issues an X.509 SVID.
func (s *SVIDIssuerService) IssueX509SVID(ctx context.Context, req *X509SVIDRequest) (*X509SVID, error) {
	if req == nil {
		return nil, fmt.Errorf("request required")
	}

	if err := ValidateSPIFFEID(req.SPIFFEID); err != nil {
		return nil, fmt.Errorf("invalid SPIFFE ID: %w", err)
	}

	// Validate TTL
	ttl := req.TTL
	if ttl == 0 {
		ttl = s.config.DefaultTTL
	}
	if ttl > s.config.MaxTTL {
		ttl = s.config.MaxTTL
	}

	// Generate or use provided key
	var privateKey crypto.PrivateKey
	var publicKey crypto.PublicKey

	if req.CSR != nil {
		// Parse CSR
		csr, err := x509.ParseCertificateRequest(req.CSR)
		if err != nil {
			return nil, fmt.Errorf("invalid CSR: %w", err)
		}
		publicKey = csr.PublicKey
		// Note: When using CSR, private key is not returned
	} else {
		// Generate new key pair
		key, err := s.generateKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate key: %w", err)
		}
		privateKey = key
		publicKey = publicKeyFromPrivate(key)
	}

	// Parse IP addresses
	var ipAddresses []net.IP
	for _, ipStr := range req.IPAddresses {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			ipAddresses = append(ipAddresses, ip)
		}
	}

	// Sign certificate
	cert, err := s.config.CA.SignX509SVID(req.SPIFFEID, publicKey, ttl, req.DNSNames, ipAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to sign certificate: %w", err)
	}

	// Build certificate chain
	trustChain := s.config.CA.GetTrustChain()
	certs := make([]*x509.Certificate, 0, len(trustChain)+1)
	certs = append(certs, cert)
	certs = append(certs, trustChain...)

	now := time.Now()
	return &X509SVID{
		SPIFFEID:     req.SPIFFEID,
		Certificates: certs,
		PrivateKey:   privateKey,
		ExpiresAt:    cert.NotAfter,
		IssuedAt:     now,
		Hint:         req.Hint,
	}, nil
}

// IssueJWTSVID issues a JWT SVID.
func (s *SVIDIssuerService) IssueJWTSVID(ctx context.Context, req *JWTSVIDRequest) (*JWTSVID, error) {
	if req == nil {
		return nil, fmt.Errorf("request required")
	}

	if err := ValidateSPIFFEID(req.SPIFFEID); err != nil {
		return nil, fmt.Errorf("invalid SPIFFE ID: %w", err)
	}

	if len(req.Audience) == 0 {
		return nil, fmt.Errorf("audience required")
	}

	// Validate TTL
	ttl := req.TTL
	if ttl == 0 {
		ttl = s.config.DefaultTTL
	}
	if ttl > s.config.MaxTTL {
		ttl = s.config.MaxTTL
	}

	now := time.Now()
	expiresAt := now.Add(ttl)

	// Build JWT claims
	claims := map[string]interface{}{
		"sub": req.SPIFFEID.String(),
		"aud": req.Audience,
		"iat": now.Unix(),
		"exp": expiresAt.Unix(),
		"iss": fmt.Sprintf("spiffe://%s", s.config.TrustDomain),
	}

	// Add extra claims
	for k, v := range req.ExtraClaims {
		if k != "sub" && k != "aud" && k != "iat" && k != "exp" && k != "iss" {
			claims[k] = v
		}
	}

	// Sign JWT
	token, err := s.signJWT(claims)
	if err != nil {
		return nil, fmt.Errorf("failed to sign JWT: %w", err)
	}

	return &JWTSVID{
		SPIFFEID:  req.SPIFFEID,
		Token:     token,
		ExpiresAt: expiresAt,
		IssuedAt:  now,
		Audience:  req.Audience,
		Claims:    claims,
	}, nil
}

// RenewX509SVID renews an existing X.509 SVID.
func (s *SVIDIssuerService) RenewX509SVID(ctx context.Context, current *X509SVID) (*X509SVID, error) {
	if current == nil {
		return nil, fmt.Errorf("current SVID required")
	}

	// Calculate new TTL based on original lifetime
	originalLifetime := current.ExpiresAt.Sub(current.IssuedAt)
	ttl := originalLifetime
	if ttl > s.config.MaxTTL {
		ttl = s.config.MaxTTL
	}

	// Extract DNS names and IP addresses from current certificate
	var dnsNames []string
	var ipAddresses []string
	if len(current.Certificates) > 0 {
		dnsNames = current.Certificates[0].DNSNames
		for _, ip := range current.Certificates[0].IPAddresses {
			ipAddresses = append(ipAddresses, ip.String())
		}
	}

	return s.IssueX509SVID(ctx, &X509SVIDRequest{
		SPIFFEID:    current.SPIFFEID,
		TTL:         ttl,
		DNSNames:    dnsNames,
		IPAddresses: ipAddresses,
		Hint:        current.Hint,
	})
}

// GetJWTAuthority returns the JWT signing public key as a JWTAuthority.
func (s *SVIDIssuerService) GetJWTAuthority() JWTAuthority {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return JWTAuthority{
		KeyID:     s.jwtKeyID,
		PublicKey: publicKeyFromPrivate(s.jwtSigningKey),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour), // JWT key valid for 1 year
	}
}

// generateKey generates a new private key.
func (s *SVIDIssuerService) generateKey() (crypto.PrivateKey, error) {
	switch s.config.KeyType {
	case "ecdsa-p256":
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "ecdsa-p384":
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "rsa-2048":
		return rsa.GenerateKey(rand.Reader, 2048)
	case "rsa-4096":
		return rsa.GenerateKey(rand.Reader, 4096)
	default:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
}

// signJWT signs claims as a JWT.
func (s *SVIDIssuerService) signJWT(claims map[string]interface{}) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build header
	header := map[string]interface{}{
		"typ": "JWT",
		"alg": "ES256",
		"kid": s.jwtKeyID,
	}

	// Encode header and claims
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Sign
	signingInput := headerB64 + "." + claimsB64
	hash := sha256.Sum256([]byte(signingInput))

	ecdsaKey, ok := s.jwtSigningKey.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("invalid signing key type")
	}

	r, ss, err := ecdsa.Sign(rand.Reader, ecdsaKey, hash[:])
	if err != nil {
		return "", err
	}

	// Encode signature (P-256 signature is 64 bytes: r (32) + s (32))
	rBytes := r.Bytes()
	sBytes := ss.Bytes()

	// Pad to 32 bytes each
	sig := make([]byte, 64)
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + sigB64, nil
}

// publicKeyFromPrivate extracts the public key from a private key.
func publicKeyFromPrivate(key crypto.PrivateKey) crypto.PublicKey {
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	case *rsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

// SVIDCache caches SVIDs for reuse.
type SVIDCache struct {
	cache map[string]*cachedSVID
	mu    sync.RWMutex
}

type cachedSVID struct {
	svid      *X509SVID
	expiresAt time.Time
}

// NewSVIDCache creates a new SVID cache.
func NewSVIDCache() *SVIDCache {
	return &SVIDCache{
		cache: make(map[string]*cachedSVID),
	}
}

// Get retrieves a cached SVID.
func (c *SVIDCache) Get(spiffeID string) (*X509SVID, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.cache[spiffeID]
	if !ok {
		return nil, false
	}

	// Check if expired or should rotate
	if entry.svid.Expired() || entry.svid.ShouldRotate() {
		return nil, false
	}

	return entry.svid, true
}

// Put stores an SVID in the cache.
func (c *SVIDCache) Put(svid *X509SVID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[svid.SPIFFEID.String()] = &cachedSVID{
		svid:      svid,
		expiresAt: svid.ExpiresAt,
	}
}

// Delete removes an SVID from the cache.
func (c *SVIDCache) Delete(spiffeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, spiffeID)
}

// Cleanup removes expired SVIDs from the cache.
func (c *SVIDCache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for id, entry := range c.cache {
		if entry.svid.Expired() {
			delete(c.cache, id)
			count++
		}
	}

	return count
}

// Size returns the number of cached SVIDs.
func (c *SVIDCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}
