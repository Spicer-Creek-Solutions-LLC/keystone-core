package identity

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// NATSIdentityIntegration provides SPIFFE-based identity for NATS connections.
type NATSIdentityIntegration struct {
	config       *NATSIdentityConfig
	identClient  *AgentIdentityClient
	tlsConfig    *tls.Config
	tlsConfigMu  sync.RWMutex

	// For server-side verification
	allowedSPIFFEPrefixes []string
}

// NewNATSIdentityIntegration creates a new NATS identity integration.
func NewNATSIdentityIntegration(config *NATSIdentityConfig, client *AgentIdentityClient) (*NATSIdentityIntegration, error) {
	if config == nil {
		return nil, fmt.Errorf("config required")
	}

	n := &NATSIdentityIntegration{
		config:                config,
		identClient:           client,
		allowedSPIFFEPrefixes: config.AllowedSPIFFEIDPrefixes,
	}

	// Register for SVID rotation updates
	if client != nil {
		_ = client.WatchX509SVID(nil, func(oldSVID, newSVID *X509SVID) {
			n.updateTLSConfig(newSVID)
		})
	}

	return n, nil
}

// GetClientTLSConfig returns a TLS config for NATS client connections.
func (n *NATSIdentityIntegration) GetClientTLSConfig() (*tls.Config, error) {
	n.tlsConfigMu.RLock()
	defer n.tlsConfigMu.RUnlock()

	if n.identClient == nil {
		return nil, fmt.Errorf("identity client not configured")
	}

	baseTLS := n.identClient.GetTLSConfig()
	if baseTLS == nil {
		return nil, fmt.Errorf("TLS config not available")
	}

	// Create client-specific config
	return &tls.Config{
		GetClientCertificate: func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			config := n.identClient.GetTLSConfig()
			if config == nil || len(config.Certificates) == 0 {
				return nil, fmt.Errorf("no certificate available")
			}
			return &config.Certificates[0], nil
		},
		RootCAs:            baseTLS.RootCAs,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
	}, nil
}

// GetServerTLSConfig returns a TLS config for NATS server connections.
func (n *NATSIdentityIntegration) GetServerTLSConfig() (*tls.Config, error) {
	n.tlsConfigMu.RLock()
	defer n.tlsConfigMu.RUnlock()

	if n.identClient == nil {
		return nil, fmt.Errorf("identity client not configured")
	}

	baseTLS := n.identClient.GetTLSConfig()
	if baseTLS == nil {
		return nil, fmt.Errorf("TLS config not available")
	}

	// Create server-specific config
	config := &tls.Config{
		GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
			config := n.identClient.GetTLSConfig()
			if config == nil || len(config.Certificates) == 0 {
				return nil, fmt.Errorf("no certificate available")
			}
			return &config.Certificates[0], nil
		},
		ClientCAs:  baseTLS.ClientCAs,
		MinVersion: tls.VersionTLS12,
	}

	if n.config.VerifyClientCert {
		config.ClientAuth = tls.RequireAndVerifyClientCert
		config.VerifyPeerCertificate = n.verifyPeerCertificate
	}

	return config, nil
}

// verifyPeerCertificate verifies the peer certificate's SPIFFE ID.
func (n *NATSIdentityIntegration) verifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(verifiedChains) == 0 || len(verifiedChains[0]) == 0 {
		return fmt.Errorf("no verified certificate chain")
	}

	cert := verifiedChains[0][0]

	// Extract SPIFFE ID from certificate URI SAN
	spiffeID, err := ExtractSPIFFEIDFromCert(cert)
	if err != nil {
		return fmt.Errorf("failed to extract SPIFFE ID: %w", err)
	}

	// Validate SPIFFE ID against allowed prefixes
	if len(n.allowedSPIFFEPrefixes) > 0 {
		allowed := false
		for _, prefix := range n.allowedSPIFFEPrefixes {
			if strings.HasPrefix(spiffeID.String(), prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("SPIFFE ID %s not in allowed prefixes", spiffeID.String())
		}
	}

	return nil
}

// updateTLSConfig updates the TLS config after SVID rotation.
func (n *NATSIdentityIntegration) updateTLSConfig(newSVID *X509SVID) {
	n.tlsConfigMu.Lock()
	defer n.tlsConfigMu.Unlock()

	if n.identClient != nil {
		n.tlsConfig = n.identClient.GetTLSConfig()
	}
}

// ExtractSPIFFEIDFromCert extracts the SPIFFE ID from a certificate's URI SAN.
func ExtractSPIFFEIDFromCert(cert *x509.Certificate) (SPIFFEID, error) {
	for _, uri := range cert.URIs {
		if uri.Scheme == "spiffe" {
			return ParseSPIFFEID(uri.String())
		}
	}
	return SPIFFEID{}, fmt.Errorf("no SPIFFE ID found in certificate")
}

// SPIFFEIDAuthorizer authorizes NATS operations based on SPIFFE ID.
type SPIFFEIDAuthorizer struct {
	// Rules maps SPIFFE ID patterns to allowed subjects
	rules []SPIFFEAuthRule
	mu    sync.RWMutex
}

// SPIFFEAuthRule defines an authorization rule based on SPIFFE ID.
type SPIFFEAuthRule struct {
	// SPIFFEIDPattern is a pattern to match SPIFFE IDs.
	// Supports wildcards: spiffe://domain/agent/* matches all agents.
	SPIFFEIDPattern string

	// AllowPublish lists subjects the identity can publish to.
	AllowPublish []string

	// AllowSubscribe lists subjects the identity can subscribe to.
	AllowSubscribe []string

	// DenyPublish lists subjects the identity cannot publish to.
	DenyPublish []string

	// DenySubscribe lists subjects the identity cannot subscribe to.
	DenySubscribe []string
}

// NewSPIFFEIDAuthorizer creates a new SPIFFE ID authorizer.
func NewSPIFFEIDAuthorizer() *SPIFFEIDAuthorizer {
	return &SPIFFEIDAuthorizer{
		rules: make([]SPIFFEAuthRule, 0),
	}
}

// AddRule adds an authorization rule.
func (a *SPIFFEIDAuthorizer) AddRule(rule SPIFFEAuthRule) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rules = append(a.rules, rule)
}

// CanPublish checks if a SPIFFE ID can publish to a subject.
func (a *SPIFFEIDAuthorizer) CanPublish(spiffeID SPIFFEID, subject string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, rule := range a.rules {
		if matchSPIFFEPattern(rule.SPIFFEIDPattern, spiffeID.String()) {
			// Check deny first
			for _, deny := range rule.DenyPublish {
				if matchSubjectPattern(deny, subject) {
					return false
				}
			}
			// Check allow
			for _, allow := range rule.AllowPublish {
				if matchSubjectPattern(allow, subject) {
					return true
				}
			}
		}
	}

	return false
}

// CanSubscribe checks if a SPIFFE ID can subscribe to a subject.
func (a *SPIFFEIDAuthorizer) CanSubscribe(spiffeID SPIFFEID, subject string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, rule := range a.rules {
		if matchSPIFFEPattern(rule.SPIFFEIDPattern, spiffeID.String()) {
			// Check deny first
			for _, deny := range rule.DenySubscribe {
				if matchSubjectPattern(deny, subject) {
					return false
				}
			}
			// Check allow
			for _, allow := range rule.AllowSubscribe {
				if matchSubjectPattern(allow, subject) {
					return true
				}
			}
		}
	}

	return false
}

// GetPermissionsForSPIFFEID returns the NATS permissions for a SPIFFE ID.
// Returns (publishAllowed, subscribeAllowed).
func (a *SPIFFEIDAuthorizer) GetPermissionsForSPIFFEID(spiffeID SPIFFEID) (publish []string, subscribe []string) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, rule := range a.rules {
		if matchSPIFFEPattern(rule.SPIFFEIDPattern, spiffeID.String()) {
			publish = append(publish, rule.AllowPublish...)
			subscribe = append(subscribe, rule.AllowSubscribe...)
		}
	}

	return publish, subscribe
}

// DefaultAgentAuthRules returns default authorization rules for agents.
func DefaultAgentAuthRules(trustDomain, clusterPrefix string) []SPIFFEAuthRule {
	return []SPIFFEAuthRule{
		{
			// Agents can publish/subscribe to their own subjects
			SPIFFEIDPattern: fmt.Sprintf("spiffe://%s/agent/*", trustDomain),
			AllowPublish: []string{
				fmt.Sprintf("%s.agent.*.heartbeat", clusterPrefix),
				fmt.Sprintf("%s.agent.*.response", clusterPrefix),
				fmt.Sprintf("%s.agent.*.events", clusterPrefix),
			},
			AllowSubscribe: []string{
				fmt.Sprintf("%s.agent.*.command", clusterPrefix),
				fmt.Sprintf("%s.agent.*.state", clusterPrefix),
				fmt.Sprintf("%s.broadcast.*", clusterPrefix),
			},
		},
	}
}

// DefaultServerAuthRules returns default authorization rules for servers.
func DefaultServerAuthRules(trustDomain, clusterPrefix string) []SPIFFEAuthRule {
	return []SPIFFEAuthRule{
		{
			// Servers can publish/subscribe to everything
			SPIFFEIDPattern: fmt.Sprintf("spiffe://%s/server/*", trustDomain),
			AllowPublish:    []string{">"},
			AllowSubscribe:  []string{">"},
		},
	}
}

// NATSAuthorizationMapper maps SPIFFE IDs to NATS authorization.
type NATSAuthorizationMapper struct {
	trustDomain   string
	clusterPrefix string
	authorizer    *SPIFFEIDAuthorizer
}

// NewNATSAuthorizationMapper creates a new NATS authorization mapper.
func NewNATSAuthorizationMapper(trustDomain, clusterPrefix string) *NATSAuthorizationMapper {
	mapper := &NATSAuthorizationMapper{
		trustDomain:   trustDomain,
		clusterPrefix: clusterPrefix,
		authorizer:    NewSPIFFEIDAuthorizer(),
	}

	// Add default rules
	for _, rule := range DefaultAgentAuthRules(trustDomain, clusterPrefix) {
		mapper.authorizer.AddRule(rule)
	}
	for _, rule := range DefaultServerAuthRules(trustDomain, clusterPrefix) {
		mapper.authorizer.AddRule(rule)
	}

	return mapper
}

// MapCertToPermissions extracts SPIFFE ID from cert and returns NATS permissions.
func (m *NATSAuthorizationMapper) MapCertToPermissions(cert *x509.Certificate) (publish []string, subscribe []string, err error) {
	spiffeID, err := ExtractSPIFFEIDFromCert(cert)
	if err != nil {
		return nil, nil, err
	}

	publish, subscribe = m.authorizer.GetPermissionsForSPIFFEID(spiffeID)
	return publish, subscribe, nil
}

// Helper functions

// matchSPIFFEPattern matches a SPIFFE ID against a pattern.
// Supports * as wildcard for a single path segment.
func matchSPIFFEPattern(pattern, spiffeID string) bool {
	// Parse both as URLs for proper matching
	patternURL, err := url.Parse(pattern)
	if err != nil {
		return false
	}
	idURL, err := url.Parse(spiffeID)
	if err != nil {
		return false
	}

	// Trust domains must match exactly
	if patternURL.Host != idURL.Host {
		return false
	}

	// Match paths with wildcards
	patternParts := strings.Split(strings.Trim(patternURL.Path, "/"), "/")
	idParts := strings.Split(strings.Trim(idURL.Path, "/"), "/")

	if len(patternParts) != len(idParts) {
		// Special case: pattern ending with /* matches any depth
		if len(patternParts) > 0 && patternParts[len(patternParts)-1] == "*" {
			return len(idParts) >= len(patternParts)-1
		}
		return false
	}

	for i, part := range patternParts {
		if part == "*" {
			continue
		}
		if part != idParts[i] {
			return false
		}
	}

	return true
}

// matchSubjectPattern matches a NATS subject against a pattern.
// Supports * for single token and > for multiple tokens.
func matchSubjectPattern(pattern, subject string) bool {
	if pattern == ">" {
		return true
	}

	patternParts := strings.Split(pattern, ".")
	subjectParts := strings.Split(subject, ".")

	for i, part := range patternParts {
		if part == ">" {
			// > matches remaining parts
			return true
		}

		if i >= len(subjectParts) {
			return false
		}

		if part == "*" {
			continue
		}

		if part != subjectParts[i] {
			return false
		}
	}

	return len(patternParts) == len(subjectParts)
}
