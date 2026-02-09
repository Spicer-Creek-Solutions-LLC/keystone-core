package auth

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	"github.com/shawnbutts/keystone-core/internal/config"
)

// mTLS-specific errors
var (
	ErrNoPeerInfo         = errors.New("no peer info in context")
	ErrNoTLSInfo          = errors.New("no TLS info in peer")
	ErrNoClientCert       = errors.New("no client certificate provided")
	ErrCertNotVerified    = errors.New("client certificate not verified")
	ErrNoMatchingCertRole = errors.New("no role mapping for certificate")
	ErrInvalidCertPattern = errors.New("invalid certificate pattern")
)

// MTLSAuthenticator authenticates using client TLS certificates
type MTLSAuthenticator struct {
	mu                sync.RWMutex
	requireClientCert bool
	certRoles         map[string]Role // pattern -> role
	compiledPatterns  map[string]*regexp.Regexp
	patternOrder      []string // patterns in priority order (most specific first)
	defaultRole       Role
}

// NewMTLSAuthenticator creates a new mTLS authenticator from config
func NewMTLSAuthenticator(cfg config.MTLSAuthConfig) (*MTLSAuthenticator, error) {
	auth := &MTLSAuthenticator{
		requireClientCert: cfg.RequireClientCert,
		certRoles:         make(map[string]Role),
		compiledPatterns:  make(map[string]*regexp.Regexp),
		patternOrder:      make([]string, 0, len(cfg.CertRoles)),
		defaultRole:       RoleReadonly,
	}

	// Parse and compile cert role patterns
	for pattern, roleStr := range cfg.CertRoles {
		role, err := parseRole(roleStr)
		if err != nil {
			return nil, fmt.Errorf("invalid role %q for pattern %q: %w", roleStr, pattern, err)
		}

		// Compile glob pattern to regex
		regex, err := compileGlobPattern(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}

		auth.certRoles[pattern] = role
		auth.compiledPatterns[pattern] = regex
		auth.patternOrder = append(auth.patternOrder, pattern)
	}

	// Sort patterns by specificity (most specific first)
	// Specificity: longer patterns first, patterns without wildcards before wildcards
	sortPatternsBySpecificity(auth.patternOrder)

	return auth, nil
}

// Name returns the authenticator name
func (a *MTLSAuthenticator) Name() string {
	return "mtls"
}

// Authenticate validates mTLS credentials from context
// For mTLS, the credential parameter is ignored - we extract from TLS peer info
func (a *MTLSAuthenticator) Authenticate(ctx context.Context, credential string) (*Principal, error) {
	return a.AuthenticateFromContext(ctx)
}

// AuthenticateFromContext extracts and validates the client certificate from context
func (a *MTLSAuthenticator) AuthenticateFromContext(ctx context.Context) (*Principal, error) {
	// Extract peer info from context
	p, ok := peer.FromContext(ctx)
	if !ok {
		if a.requireClientCert {
			return nil, ErrNoPeerInfo
		}
		return nil, ErrNoCredentials
	}

	// Get TLS info
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		if a.requireClientCert {
			return nil, ErrNoTLSInfo
		}
		return nil, ErrNoCredentials
	}

	// Check for verified client certificates
	if len(tlsInfo.State.VerifiedChains) == 0 {
		if a.requireClientCert {
			return nil, ErrNoClientCert
		}
		return nil, ErrNoCredentials
	}

	// Get the client certificate (first cert in first verified chain)
	if len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return nil, ErrNoClientCert
	}

	clientCert := tlsInfo.State.VerifiedChains[0][0]

	// Extract identity info from certificate
	identity := a.extractIdentity(clientCert)

	// Determine role based on CN/SAN patterns
	role, err := a.determineRole(clientCert)
	if err != nil {
		return nil, err
	}

	// Build principal
	principal := &Principal{
		ID:              fmt.Sprintf("mtls:%s", identity),
		Name:            identity,
		Role:            role,
		AuthMethod:      "mtls",
		AuthenticatedAt: time.Now(),
		Metadata:        make(map[string]string),
	}

	// Add certificate metadata
	principal.Metadata["cn"] = clientCert.Subject.CommonName
	principal.Metadata["serial"] = clientCert.SerialNumber.String()
	principal.Metadata["issuer"] = clientCert.Issuer.CommonName
	principal.Metadata["not_before"] = clientCert.NotBefore.Format(time.RFC3339)
	principal.Metadata["not_after"] = clientCert.NotAfter.Format(time.RFC3339)

	// Add organization if present
	if len(clientCert.Subject.Organization) > 0 {
		principal.Metadata["org"] = strings.Join(clientCert.Subject.Organization, ",")
	}

	// Add SANs
	if len(clientCert.DNSNames) > 0 {
		principal.Metadata["dns_names"] = strings.Join(clientCert.DNSNames, ",")
	}
	if len(clientCert.EmailAddresses) > 0 {
		principal.Metadata["emails"] = strings.Join(clientCert.EmailAddresses, ",")
	}

	return principal, nil
}

// extractIdentity extracts the primary identity from the certificate
func (a *MTLSAuthenticator) extractIdentity(cert *x509.Certificate) string {
	// Prefer CN
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}

	// Fall back to first DNS SAN
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}

	// Fall back to first email SAN
	if len(cert.EmailAddresses) > 0 {
		return cert.EmailAddresses[0]
	}

	// Fall back to serial number
	return cert.SerialNumber.String()
}

// determineRole finds the role for a certificate based on CN/SAN patterns
func (a *MTLSAuthenticator) determineRole(cert *x509.Certificate) (Role, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Collect all identities to check
	identities := make([]string, 0, 1+len(cert.DNSNames)+len(cert.EmailAddresses)+len(cert.URIs))
	identities = append(identities, cert.Subject.CommonName)
	identities = append(identities, cert.DNSNames...)
	identities = append(identities, cert.EmailAddresses...)

	// Add URI SANs
	for _, uri := range cert.URIs {
		identities = append(identities, uri.String())
	}

	// Check each identity against patterns in priority order (most specific first)
	for _, pattern := range a.patternOrder {
		regex := a.compiledPatterns[pattern]
		for _, identity := range identities {
			if identity == "" {
				continue
			}
			if regex.MatchString(identity) {
				return a.certRoles[pattern], nil
			}
		}
	}

	// If no pattern matches but we have a certificate, use default role
	if len(a.certRoles) == 0 {
		return a.defaultRole, nil
	}

	// If patterns are configured but none match, that's an error
	return "", ErrNoMatchingCertRole
}

// AddCertRole adds a certificate pattern to role mapping at runtime
func (a *MTLSAuthenticator) AddCertRole(pattern, roleStr string) error {
	role, err := parseRole(roleStr)
	if err != nil {
		return err
	}

	regex, err := compileGlobPattern(pattern)
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if pattern already exists
	exists := false
	for _, p := range a.patternOrder {
		if p == pattern {
			exists = true
			break
		}
	}

	a.certRoles[pattern] = role
	a.compiledPatterns[pattern] = regex

	if !exists {
		a.patternOrder = append(a.patternOrder, pattern)
		sortPatternsBySpecificity(a.patternOrder)
	}

	return nil
}

// RemoveCertRole removes a certificate pattern mapping
func (a *MTLSAuthenticator) RemoveCertRole(pattern string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.certRoles, pattern)
	delete(a.compiledPatterns, pattern)

	// Remove from patternOrder
	for i, p := range a.patternOrder {
		if p == pattern {
			a.patternOrder = append(a.patternOrder[:i], a.patternOrder[i+1:]...)
			break
		}
	}
}

// SetDefaultRole sets the default role when no patterns are configured
func (a *MTLSAuthenticator) SetDefaultRole(role Role) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.defaultRole = role
}

// parseRole converts a string to a Role
func parseRole(roleStr string) (Role, error) {
	switch strings.ToLower(roleStr) {
	case "admin":
		return RoleAdmin, nil
	case "operator":
		return RoleOperator, nil
	case "readonly", "read-only", "viewer":
		return RoleReadonly, nil
	default:
		return "", fmt.Errorf("invalid role: %s", roleStr)
	}
}

// compileGlobPattern converts a glob pattern to a regex
// Supports:
//   - * matches any characters except dots
//   - ** matches any characters including dots
//   - ? matches a single character
//   - Literal matching for other characters
func compileGlobPattern(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, ErrInvalidCertPattern
	}

	// Check if it's already a regex (starts with ^)
	if strings.HasPrefix(pattern, "^") {
		return regexp.Compile(pattern)
	}

	// Convert glob to regex
	var regexBuilder strings.Builder
	regexBuilder.WriteString("^")

	i := 0
	for i < len(pattern) {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// ** matches anything
				regexBuilder.WriteString(".*")
				i += 2
			} else {
				// * matches anything except dots
				regexBuilder.WriteString("[^.]*")
				i++
			}
		case '?':
			regexBuilder.WriteString(".")
			i++
		case '.', '+', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\':
			// Escape regex metacharacters
			regexBuilder.WriteString("\\")
			regexBuilder.WriteByte(c)
			i++
		default:
			regexBuilder.WriteByte(c)
			i++
		}
	}

	regexBuilder.WriteString("$")
	return regexp.Compile(regexBuilder.String())
}

// sortPatternsBySpecificity sorts patterns so that more specific patterns come first.
// Specificity is determined by:
// 1. Patterns without wildcards are more specific than patterns with wildcards
// 2. Longer patterns are more specific than shorter patterns
// 3. Single wildcards (*) are more specific than double wildcards (**)
func sortPatternsBySpecificity(patterns []string) {
	sort.Slice(patterns, func(i, j int) bool {
		return patternSpecificity(patterns[i]) > patternSpecificity(patterns[j])
	})
}

// patternSpecificity returns a score for pattern specificity (higher = more specific)
func patternSpecificity(pattern string) int {
	score := len(pattern) * 10 // Base score is length * 10

	// Penalize wildcards
	if strings.Contains(pattern, "**") {
		score -= 1000 // Heavy penalty for ** (matches anything)
	} else if strings.Contains(pattern, "*") {
		score -= 100 // Moderate penalty for * (matches segment)
	}

	if strings.Contains(pattern, "?") {
		score -= 10 // Small penalty for ?
	}

	// Bonus for exact matches (no wildcards at all)
	if !strings.ContainsAny(pattern, "*?") {
		score += 500
	}

	return score
}

// ExtractClientCertFromContext is a helper to get the client certificate from context
func ExtractClientCertFromContext(ctx context.Context) (*x509.Certificate, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, ErrNoPeerInfo
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, ErrNoTLSInfo
	}

	if len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return nil, ErrNoClientCert
	}

	return tlsInfo.State.VerifiedChains[0][0], nil
}
