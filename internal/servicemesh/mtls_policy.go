package servicemesh

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// PolicyMode defines the mTLS policy mode
type PolicyMode string

const (
	// PolicyModeStrict requires mTLS for all connections
	PolicyModeStrict PolicyMode = "STRICT"
	// PolicyModePermissive allows both mTLS and plaintext
	PolicyModePermissive PolicyMode = "PERMISSIVE"
	// PolicyModeDisable disables mTLS
	PolicyModeDisable PolicyMode = "DISABLE"
)

// MTLSPolicy defines an mTLS policy
type MTLSPolicy struct {
	// Name of the policy
	Name string `json:"name"`

	// Namespace the policy applies to
	Namespace string `json:"namespace,omitempty"`

	// Service the policy applies to (empty for namespace-wide)
	Service string `json:"service,omitempty"`

	// Port the policy applies to (0 for all ports)
	Port int `json:"port,omitempty"`

	// Mode is the mTLS mode
	Mode PolicyMode `json:"mode"`

	// PeerAuthentication specifies peer authentication requirements
	PeerAuthentication *PeerAuthentication `json:"peer_authentication,omitempty"`

	// DestinationRule specifies destination rules
	DestinationRule *DestinationRule `json:"destination_rule,omitempty"`

	// CreatedAt when the policy was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt when the policy was last updated
	UpdatedAt time.Time `json:"updated_at"`
}

// PeerAuthentication defines peer authentication settings
type PeerAuthentication struct {
	// Mode is the mTLS mode for peers
	Mode PolicyMode `json:"mode"`

	// PortLevelMtls allows port-specific mTLS settings
	PortLevelMtls map[int]PolicyMode `json:"port_level_mtls,omitempty"`
}

// DestinationRule defines traffic policy settings
type DestinationRule struct {
	// Host the rule applies to
	Host string `json:"host"`

	// TrafficPolicy defines the traffic policy
	TrafficPolicy *TrafficPolicy `json:"traffic_policy,omitempty"`
}

// TrafficPolicy defines how traffic is handled
type TrafficPolicy struct {
	// TLS settings
	TLS *TLSSettings `json:"tls,omitempty"`

	// ConnectionPool settings
	ConnectionPool *ConnectionPoolSettings `json:"connection_pool,omitempty"`

	// OutlierDetection settings
	OutlierDetection *OutlierDetection `json:"outlier_detection,omitempty"`
}

// TLSSettings configures TLS for connections
type TLSSettings struct {
	// Mode is the TLS mode (DISABLE, SIMPLE, MUTUAL, ISTIO_MUTUAL)
	Mode string `json:"mode"`

	// ClientCertificate path for mutual TLS
	ClientCertificate string `json:"client_certificate,omitempty"`

	// PrivateKey path for mutual TLS
	PrivateKey string `json:"private_key,omitempty"`

	// CaCertificates path for certificate verification
	CaCertificates string `json:"ca_certificates,omitempty"`

	// SNI server name indicator
	SNI string `json:"sni,omitempty"`

	// SubjectAltNames for certificate verification
	SubjectAltNames []string `json:"subject_alt_names,omitempty"`
}

// ConnectionPoolSettings for connection pooling
type ConnectionPoolSettings struct {
	// TCP connection pool settings
	TCP *TCPSettings `json:"tcp,omitempty"`

	// HTTP connection pool settings
	HTTP *HTTPSettings `json:"http,omitempty"`
}

// TCPSettings for TCP connections
type TCPSettings struct {
	// MaxConnections to a host
	MaxConnections int `json:"max_connections,omitempty"`

	// ConnectTimeout for TCP connections
	ConnectTimeout time.Duration `json:"connect_timeout,omitempty"`
}

// HTTPSettings for HTTP connections
type HTTPSettings struct {
	// H2UpgradePolicy for HTTP/2 upgrade
	H2UpgradePolicy string `json:"h2_upgrade_policy,omitempty"`

	// HTTP1MaxPendingRequests is max pending HTTP/1 requests
	HTTP1MaxPendingRequests int `json:"http1_max_pending_requests,omitempty"`

	// HTTP2MaxRequests is max concurrent HTTP/2 requests
	HTTP2MaxRequests int `json:"http2_max_requests,omitempty"`

	// MaxRequestsPerConnection before connection is closed
	MaxRequestsPerConnection int `json:"max_requests_per_connection,omitempty"`

	// MaxRetries before giving up
	MaxRetries int `json:"max_retries,omitempty"`
}

// OutlierDetection for circuit breaking
type OutlierDetection struct {
	// Consecutive5xxErrors before ejection
	Consecutive5xxErrors int `json:"consecutive_5xx_errors,omitempty"`

	// ConsecutiveGatewayErrors before ejection
	ConsecutiveGatewayErrors int `json:"consecutive_gateway_errors,omitempty"`

	// Interval for checking outliers
	Interval time.Duration `json:"interval,omitempty"`

	// BaseEjectionTime for outliers
	BaseEjectionTime time.Duration `json:"base_ejection_time,omitempty"`

	// MaxEjectionPercent of hosts to eject
	MaxEjectionPercent int `json:"max_ejection_percent,omitempty"`
}

// AuthorizationAction defines the action for an authorization policy
type AuthorizationAction string

const (
	// AuthorizationActionAllow allows the request
	AuthorizationActionAllow AuthorizationAction = "ALLOW"
	// AuthorizationActionDeny denies the request
	AuthorizationActionDeny AuthorizationAction = "DENY"
	// AuthorizationActionAudit logs the request without blocking
	AuthorizationActionAudit AuthorizationAction = "AUDIT"
	// AuthorizationActionCustom uses a custom external authorizer
	AuthorizationActionCustom AuthorizationAction = "CUSTOM"
)

// AuthorizationPolicy defines authorization rules for service access
type AuthorizationPolicy struct {
	// Name of the policy
	Name string `json:"name"`

	// Namespace the policy applies to
	Namespace string `json:"namespace,omitempty"`

	// Action is the authorization action (ALLOW, DENY, AUDIT, CUSTOM)
	Action AuthorizationAction `json:"action"`

	// Rules define the authorization rules
	Rules []AuthorizationRule `json:"rules,omitempty"`

	// Selector for workload selection
	Selector map[string]string `json:"selector,omitempty"`

	// CreatedAt when the policy was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt when the policy was last updated
	UpdatedAt time.Time `json:"updated_at"`
}

// AuthorizationRule defines a single authorization rule
type AuthorizationRule struct {
	// From specifies the source of traffic
	From []RuleSource `json:"from,omitempty"`

	// To specifies the destination of traffic
	To []RuleDestination `json:"to,omitempty"`

	// When specifies additional conditions
	When []RuleCondition `json:"when,omitempty"`
}

// RuleSource defines traffic source matching criteria
type RuleSource struct {
	// Principals are identities derived from peer certificate
	Principals []string `json:"principals,omitempty"`

	// NotPrincipals are identities to exclude
	NotPrincipals []string `json:"not_principals,omitempty"`

	// RequestPrincipals are identities from request authentication
	RequestPrincipals []string `json:"request_principals,omitempty"`

	// NotRequestPrincipals are request identities to exclude
	NotRequestPrincipals []string `json:"not_request_principals,omitempty"`

	// Namespaces are source namespaces
	Namespaces []string `json:"namespaces,omitempty"`

	// NotNamespaces are namespaces to exclude
	NotNamespaces []string `json:"not_namespaces,omitempty"`

	// IPBlocks are source IP ranges in CIDR notation
	IPBlocks []string `json:"ip_blocks,omitempty"`

	// NotIPBlocks are IP ranges to exclude
	NotIPBlocks []string `json:"not_ip_blocks,omitempty"`
}

// RuleDestination defines traffic destination matching criteria
type RuleDestination struct {
	// Hosts are destination hosts
	Hosts []string `json:"hosts,omitempty"`

	// NotHosts are hosts to exclude
	NotHosts []string `json:"not_hosts,omitempty"`

	// Ports are destination ports
	Ports []string `json:"ports,omitempty"`

	// NotPorts are ports to exclude
	NotPorts []string `json:"not_ports,omitempty"`

	// Methods are HTTP methods
	Methods []string `json:"methods,omitempty"`

	// NotMethods are methods to exclude
	NotMethods []string `json:"not_methods,omitempty"`

	// Paths are URL paths
	Paths []string `json:"paths,omitempty"`

	// NotPaths are paths to exclude
	NotPaths []string `json:"not_paths,omitempty"`
}

// RuleCondition defines additional conditions for authorization
type RuleCondition struct {
	// Key is the attribute key (e.g., request.headers[x-custom-header])
	Key string `json:"key"`

	// Values are the expected values
	Values []string `json:"values,omitempty"`

	// NotValues are values to exclude
	NotValues []string `json:"not_values,omitempty"`
}

// ConnectionResult contains the result of a connection test
type ConnectionResult struct {
	// Connected indicates if connection succeeded
	Connected bool `json:"connected"`

	// TLSEnabled indicates if TLS was used
	TLSEnabled bool `json:"tls_enabled"`

	// TLSVersion is the negotiated TLS version
	TLSVersion string `json:"tls_version,omitempty"`

	// CipherSuite is the negotiated cipher suite
	CipherSuite string `json:"cipher_suite,omitempty"`

	// PeerCertificates are the peer's certificate chain
	PeerCertificates []*x509.Certificate `json:"-"`

	// PeerSPIFFEID is the SPIFFE ID from peer certificate
	PeerSPIFFEID string `json:"peer_spiffe_id,omitempty"`

	// Error is any error that occurred
	Error error `json:"error,omitempty"`

	// Duration of the connection attempt
	Duration time.Duration `json:"duration"`
}

// ConnectionTester tests mTLS connections to services
type ConnectionTester struct {
	timeout  time.Duration
	metadata *Metadata
}

// NewConnectionTester creates a new connection tester
func NewConnectionTester(timeout time.Duration, metadata *Metadata) *ConnectionTester {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &ConnectionTester{
		timeout:  timeout,
		metadata: metadata,
	}
}

// TestPlaintext tests if a plaintext connection succeeds
func (t *ConnectionTester) TestPlaintext(address string) (bool, error) {
	d := &net.Dialer{Timeout: t.timeout}
	conn, err := d.DialContext(context.Background(), "tcp", address)
	if err != nil {
		return false, err
	}
	conn.Close()
	return true, nil
}

func newTLSBaseConfig(host, minVersion string) *tls.Config {
	return &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: false,
		MinVersion:         parseTLSMinVersion(minVersion), // #nosec G402 -- validated to TLS 1.2+ defaults
	}
}

func parseTLSMinVersion(value string) uint16 {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1.3", "tls1.3", "tls13":
		return tls.VersionTLS13
	case "1.2", "tls1.2", "tls12":
		return tls.VersionTLS12
	default:
		return tls.VersionTLS13
	}
}

// TestMTLS tests an mTLS connection and returns detailed results
func (t *ConnectionTester) TestMTLS(address string) (*ConnectionResult, error) {
	start := time.Now()
	result := &ConnectionResult{}

	// Parse address to get host for SNI
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	minVersion := ""
	if t.metadata != nil && t.metadata.TLSConfig != nil {
		minVersion = t.metadata.TLSConfig.MinVersion
	}

	// Build TLS config
	tlsConfig := newTLSBaseConfig(host, minVersion)

	// Load CA certificate if available
	if t.metadata != nil && t.metadata.TLSConfig != nil && t.metadata.TLSConfig.CAFile != "" {
		caPEM, err := os.ReadFile(t.metadata.TLSConfig.CAFile)
		if err == nil {
			roots := x509.NewCertPool()
			if roots.AppendCertsFromPEM(caPEM) {
				tlsConfig.RootCAs = roots
			}
		}
	}

	// Load client certificate for mutual TLS if available
	if t.metadata != nil && t.metadata.TLSConfig != nil {
		if t.metadata.TLSConfig.CertChainFile != "" && t.metadata.TLSConfig.PrivateKeyFile != "" {
			cert, err := tls.LoadX509KeyPair(t.metadata.TLSConfig.CertChainFile, t.metadata.TLSConfig.PrivateKeyFile)
			if err == nil {
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}
	}

	// Attempt TLS connection
	dialer := &net.Dialer{Timeout: t.timeout}
	netConn, err := (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(context.Background(), "tcp", address)
	result.Duration = time.Since(start)

	if err != nil {
		result.Connected = false
		result.Error = err
		return result, nil
	}
	defer netConn.Close()

	result.Connected = true
	result.TLSEnabled = true

	// Extract connection state
	conn := netConn.(*tls.Conn)
	state := conn.ConnectionState()
	result.TLSVersion = tlsVersionString(state.Version)
	result.CipherSuite = tls.CipherSuiteName(state.CipherSuite)
	result.PeerCertificates = state.PeerCertificates

	// Extract SPIFFE ID from peer certificate
	if len(state.PeerCertificates) > 0 {
		result.PeerSPIFFEID = extractSPIFFEID(state.PeerCertificates[0])
	}

	return result, nil
}

// VerifyPeerIdentity verifies that the peer certificate has the expected SPIFFE ID
func (t *ConnectionTester) VerifyPeerIdentity(certs []*x509.Certificate, expectedSPIFFE string) bool {
	if len(certs) == 0 || expectedSPIFFE == "" {
		return false
	}

	actualSPIFFE := extractSPIFFEID(certs[0])
	if actualSPIFFE == "" {
		return false
	}

	// Exact match or wildcard match
	if actualSPIFFE == expectedSPIFFE {
		return true
	}

	// Check if expected is a prefix pattern (e.g., spiffe://trust.domain/ns/*)
	if strings.HasSuffix(expectedSPIFFE, "/*") {
		prefix := strings.TrimSuffix(expectedSPIFFE, "*")
		return strings.HasPrefix(actualSPIFFE, prefix)
	}

	return false
}

// extractSPIFFEID extracts the SPIFFE ID from a certificate's URI SAN
func extractSPIFFEID(cert *x509.Certificate) string {
	for _, uri := range cert.URIs {
		if strings.HasPrefix(uri.String(), "spiffe://") {
			return uri.String()
		}
	}
	return ""
}

// PolicyVerificationResult contains the result of policy verification
type PolicyVerificationResult struct {
	// Policy that was verified
	Policy *MTLSPolicy `json:"policy"`

	// Passed indicates if verification passed
	Passed bool `json:"passed"`

	// Checks contains individual check results
	Checks []PolicyCheck `json:"checks"`

	// VerifiedAt when verification was performed
	VerifiedAt time.Time `json:"verified_at"`

	// Duration of verification
	Duration time.Duration `json:"duration"`

	// Message summary message
	Message string `json:"message"`
}

// PolicyCheck represents a single policy check
type PolicyCheck struct {
	// Name of the check
	Name string `json:"name"`

	// Description of what the check verifies
	Description string `json:"description"`

	// Passed indicates if the check passed
	Passed bool `json:"passed"`

	// Message contains details
	Message string `json:"message"`

	// Severity of the check (critical, warning, info)
	Severity string `json:"severity"`
}

// PolicyVerifier verifies mTLS policies
type PolicyVerifier struct {
	meshType MeshType
	config   *Config
	metadata *Metadata
	mu       sync.RWMutex
}

// NewPolicyVerifier creates a new policy verifier
func NewPolicyVerifier(meshType MeshType, config *Config) *PolicyVerifier {
	if config == nil {
		config = DefaultConfig()
	}
	return &PolicyVerifier{
		meshType: meshType,
		config:   config,
	}
}

// SetMetadata sets the mesh metadata for verification
func (v *PolicyVerifier) SetMetadata(metadata *Metadata) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.metadata = metadata
}

// VerifyPolicy verifies that an mTLS policy is correctly enforced
func (v *PolicyVerifier) VerifyPolicy(ctx context.Context, policy *MTLSPolicy) (*PolicyVerificationResult, error) {
	start := time.Now()
	result := &PolicyVerificationResult{
		Policy:     policy,
		Passed:     true,
		Checks:     make([]PolicyCheck, 0),
		VerifiedAt: start,
	}

	// Run all checks
	checks := []func(*MTLSPolicy) PolicyCheck{
		v.checkCertificateExists,
		v.checkCertificateValidity,
		v.checkCertificateChain,
		v.checkPolicyModeConsistency,
		v.checkSPIFFEIdentity,
		v.checkTLSConfiguration,
		v.checkConnectionSecurity,
	}

	for _, check := range checks {
		c := check(policy)
		result.Checks = append(result.Checks, c)
		if !c.Passed && c.Severity == "critical" {
			result.Passed = false
		}
	}

	result.Duration = time.Since(start)

	// Generate summary message
	passedCount := 0
	for _, c := range result.Checks {
		if c.Passed {
			passedCount++
		}
	}
	result.Message = fmt.Sprintf("%d of %d checks passed", passedCount, len(result.Checks))

	return result, nil
}

// checkCertificateExists verifies certificates exist
func (v *PolicyVerifier) checkCertificateExists(policy *MTLSPolicy) PolicyCheck {
	check := PolicyCheck{
		Name:        "certificate_exists",
		Description: "Verify mTLS certificates exist",
		Severity:    "critical",
	}

	v.mu.RLock()
	metadata := v.metadata
	v.mu.RUnlock()

	if metadata == nil || metadata.TLSConfig == nil {
		check.Passed = false
		check.Message = "No TLS metadata available"
		return check
	}

	tlsConfig := metadata.TLSConfig

	// Check certificate chain
	if tlsConfig.CertChainFile != "" {
		if _, err := os.Stat(tlsConfig.CertChainFile); os.IsNotExist(err) {
			check.Passed = false
			check.Message = fmt.Sprintf("Certificate chain file not found: %s", tlsConfig.CertChainFile)
			return check
		}
	}

	// Check private key
	if tlsConfig.PrivateKeyFile != "" {
		if _, err := os.Stat(tlsConfig.PrivateKeyFile); os.IsNotExist(err) {
			check.Passed = false
			check.Message = fmt.Sprintf("Private key file not found: %s", tlsConfig.PrivateKeyFile)
			return check
		}
	}

	// Check CA certificate
	if tlsConfig.CAFile != "" {
		if _, err := os.Stat(tlsConfig.CAFile); os.IsNotExist(err) {
			check.Passed = false
			check.Message = fmt.Sprintf("CA certificate file not found: %s", tlsConfig.CAFile)
			return check
		}
	}

	check.Passed = true
	check.Message = "All certificate files exist"
	return check
}

// checkCertificateValidity verifies certificate validity period
func (v *PolicyVerifier) checkCertificateValidity(policy *MTLSPolicy) PolicyCheck {
	check := PolicyCheck{
		Name:        "certificate_validity",
		Description: "Verify certificate validity period",
		Severity:    "critical",
	}

	v.mu.RLock()
	metadata := v.metadata
	v.mu.RUnlock()

	if metadata == nil || metadata.TLSConfig == nil {
		check.Passed = false
		check.Message = "No TLS metadata available"
		return check
	}

	tlsConfig := metadata.TLSConfig

	// Load and parse certificate
	if tlsConfig.CertChainFile == "" {
		check.Passed = false
		check.Message = "No certificate chain file configured"
		return check
	}

	certPEM, err := os.ReadFile(tlsConfig.CertChainFile)
	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Failed to read certificate: %v", err)
		return check
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		check.Passed = false
		check.Message = "Failed to decode PEM block"
		return check
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Failed to parse certificate: %v", err)
		return check
	}

	now := time.Now()

	// Check if certificate is valid
	if now.Before(cert.NotBefore) {
		check.Passed = false
		check.Message = fmt.Sprintf("Certificate not yet valid (valid from %s)", cert.NotBefore)
		return check
	}

	if now.After(cert.NotAfter) {
		check.Passed = false
		check.Message = fmt.Sprintf("Certificate has expired (expired %s)", cert.NotAfter)
		return check
	}

	// Warn if expiring soon (within 7 days)
	if cert.NotAfter.Sub(now) < 7*24*time.Hour {
		check.Passed = true
		check.Severity = "warning"
		check.Message = fmt.Sprintf("Certificate expires soon: %s", cert.NotAfter)
		return check
	}

	check.Passed = true
	check.Message = fmt.Sprintf("Certificate valid until %s", cert.NotAfter)
	return check
}

// checkCertificateChain verifies the certificate chain
func (v *PolicyVerifier) checkCertificateChain(policy *MTLSPolicy) PolicyCheck {
	check := PolicyCheck{
		Name:        "certificate_chain",
		Description: "Verify certificate chain is valid",
		Severity:    "critical",
	}

	v.mu.RLock()
	metadata := v.metadata
	v.mu.RUnlock()

	if metadata == nil || metadata.TLSConfig == nil {
		check.Passed = false
		check.Message = "No TLS metadata available"
		return check
	}

	tlsConfig := metadata.TLSConfig

	// Load certificate chain
	certPEM, err := os.ReadFile(tlsConfig.CertChainFile)
	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Failed to read certificate chain: %v", err)
		return check
	}

	// Load CA certificate
	caPEM, err := os.ReadFile(tlsConfig.CAFile)
	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Failed to read CA certificate: %v", err)
		return check
	}

	// Parse certificates
	var certs []*x509.Certificate
	for len(certPEM) > 0 {
		block, rest := pem.Decode(certPEM)
		if block == nil {
			break
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			check.Passed = false
			check.Message = fmt.Sprintf("Failed to parse certificate: %v", err)
			return check
		}
		certs = append(certs, cert)
		certPEM = rest
	}

	if len(certs) == 0 {
		check.Passed = false
		check.Message = "No certificates found in chain"
		return check
	}

	// Create root cert pool
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		check.Passed = false
		check.Message = "Failed to parse CA certificate"
		return check
	}

	// Create intermediate pool from chain (excluding leaf)
	intermediates := x509.NewCertPool()
	for i := 1; i < len(certs); i++ {
		intermediates.AddCert(certs[i])
	}

	// Verify the leaf certificate
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
	}

	_, err = certs[0].Verify(opts)
	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Certificate chain verification failed: %v", err)
		return check
	}

	check.Passed = true
	check.Message = fmt.Sprintf("Certificate chain valid with %d certificates", len(certs))
	return check
}

// checkPolicyModeConsistency verifies policy mode is consistent
func (v *PolicyVerifier) checkPolicyModeConsistency(policy *MTLSPolicy) PolicyCheck {
	check := PolicyCheck{
		Name:        "policy_mode_consistency",
		Description: "Verify mTLS policy mode is consistent",
		Severity:    "warning",
	}

	v.mu.RLock()
	metadata := v.metadata
	v.mu.RUnlock()

	if metadata == nil || metadata.TLSConfig == nil {
		check.Passed = true
		check.Message = "No TLS metadata to compare against"
		return check
	}

	configuredMode := PolicyMode(metadata.TLSConfig.Mode)
	policyMode := policy.Mode

	if configuredMode != policyMode {
		check.Passed = false
		check.Message = fmt.Sprintf("Policy mode mismatch: configured=%s, policy=%s",
			configuredMode, policyMode)
		return check
	}

	// If STRICT mode, verify TLS is enabled
	if policyMode == PolicyModeStrict && !metadata.TLSConfig.Enabled {
		check.Passed = false
		check.Message = "Policy requires STRICT mTLS but TLS is not enabled"
		return check
	}

	check.Passed = true
	check.Message = fmt.Sprintf("Policy mode %s is consistent", policyMode)
	return check
}

// checkSPIFFEIdentity verifies SPIFFE identity is configured
func (v *PolicyVerifier) checkSPIFFEIdentity(policy *MTLSPolicy) PolicyCheck {
	check := PolicyCheck{
		Name:        "spiffe_identity",
		Description: "Verify SPIFFE identity is configured",
		Severity:    "warning",
	}

	v.mu.RLock()
	metadata := v.metadata
	v.mu.RUnlock()

	if metadata == nil || metadata.TLSConfig == nil {
		check.Passed = false
		check.Message = "No TLS metadata available"
		return check
	}

	spiffeID := metadata.TLSConfig.SPIFFEID
	if spiffeID == "" {
		check.Passed = false
		check.Message = "No SPIFFE identity configured"
		return check
	}

	// Validate SPIFFE ID format
	if !strings.HasPrefix(spiffeID, "spiffe://") {
		check.Passed = false
		check.Message = fmt.Sprintf("Invalid SPIFFE ID format: %s", spiffeID)
		return check
	}

	// Check trust domain matches
	if metadata.TrustDomain != "" {
		expectedPrefix := fmt.Sprintf("spiffe://%s/", metadata.TrustDomain)
		if !strings.HasPrefix(spiffeID, expectedPrefix) {
			check.Passed = false
			check.Message = fmt.Sprintf("SPIFFE ID trust domain mismatch: expected %s, got %s",
				metadata.TrustDomain, spiffeID)
			return check
		}
	}

	check.Passed = true
	check.Message = fmt.Sprintf("SPIFFE identity configured: %s", spiffeID)
	return check
}

// checkTLSConfiguration verifies TLS configuration
func (v *PolicyVerifier) checkTLSConfiguration(policy *MTLSPolicy) PolicyCheck {
	check := PolicyCheck{
		Name:        "tls_configuration",
		Description: "Verify TLS configuration settings",
		Severity:    "warning",
	}

	v.mu.RLock()
	metadata := v.metadata
	v.mu.RUnlock()

	if metadata == nil || metadata.TLSConfig == nil {
		check.Passed = false
		check.Message = "No TLS metadata available"
		return check
	}

	tlsConfig := metadata.TLSConfig

	// For STRICT mode, verify all TLS settings are configured
	if policy.Mode == PolicyModeStrict {
		if tlsConfig.CertChainFile == "" {
			check.Passed = false
			check.Message = "STRICT mode requires certificate chain"
			return check
		}
		if tlsConfig.PrivateKeyFile == "" {
			check.Passed = false
			check.Message = "STRICT mode requires private key"
			return check
		}
		if tlsConfig.CAFile == "" {
			check.Passed = false
			check.Message = "STRICT mode requires CA certificate"
			return check
		}
	}

	check.Passed = true
	check.Message = "TLS configuration is complete"
	return check
}

// checkConnectionSecurity performs a real connection security check
func (v *PolicyVerifier) checkConnectionSecurity(policy *MTLSPolicy) PolicyCheck {
	check := PolicyCheck{
		Name:        "connection_security",
		Description: "Verify actual connection security",
		Severity:    "info",
	}

	// Skip actual connection test if no service specified
	if policy.Service == "" {
		check.Passed = true
		check.Message = "No service specified for connection test"
		return check
	}

	v.mu.RLock()
	metadata := v.metadata
	v.mu.RUnlock()

	// Build service address
	port := policy.Port
	if port == 0 {
		port = 443 // Default HTTPS port
	}
	address := fmt.Sprintf("%s.%s.svc.cluster.local:%d", policy.Service, policy.Namespace, port)

	// Create connection tester
	tester := NewConnectionTester(5*time.Second, metadata)

	// Test based on policy mode
	switch policy.Mode {
	case PolicyModeStrict:
		// For STRICT mode:
		// 1. Plaintext should fail OR connect should require TLS handshake
		// 2. mTLS should succeed
		plaintextOK, _ := tester.TestPlaintext(address)
		if plaintextOK {
			// Plaintext connected - this might be OK if the service redirects or has a TLS handshake requirement
			// Try mTLS to verify it's properly configured
			result, err := tester.TestMTLS(address)
			if err != nil {
				check.Passed = false
				check.Message = fmt.Sprintf("mTLS connection test error: %v", err)
				return check
			}
			if !result.Connected {
				check.Passed = false
				check.Message = fmt.Sprintf("mTLS connection failed: %v", result.Error)
				return check
			}
			// mTLS succeeded, verify SPIFFE identity if expected
			if metadata != nil && metadata.TLSConfig != nil && metadata.TLSConfig.SPIFFEID != "" {
				expectedPattern := buildExpectedSPIFFEPattern(policy.Namespace, policy.Service, metadata.TrustDomain)
				if !tester.VerifyPeerIdentity(result.PeerCertificates, expectedPattern) {
					check.Severity = "warning"
					check.Passed = true
					check.Message = fmt.Sprintf("mTLS connected but SPIFFE identity mismatch (expected pattern: %s, got: %s)",
						expectedPattern, result.PeerSPIFFEID)
					return check
				}
			}
			check.Passed = true
			check.Message = fmt.Sprintf("STRICT mode: mTLS connection verified (%s, %s)",
				result.TLSVersion, result.CipherSuite)
		} else {
			// Plaintext failed - good for STRICT mode, now verify mTLS works
			result, err := tester.TestMTLS(address)
			if err != nil {
				check.Passed = false
				check.Message = fmt.Sprintf("mTLS connection test error: %v", err)
				return check
			}
			if result.Connected {
				check.Passed = true
				check.Message = fmt.Sprintf("STRICT mode: plaintext rejected, mTLS verified (%s)",
					result.TLSVersion)
			} else {
				// Both failed - service might be unreachable
				check.Passed = true
				check.Severity = "warning"
				check.Message = fmt.Sprintf("Service unreachable for connection test: %v", result.Error)
			}
		}

	case PolicyModePermissive:
		// For PERMISSIVE mode: both plaintext and mTLS should work
		result, err := tester.TestMTLS(address)
		if err != nil {
			check.Passed = false
			check.Message = fmt.Sprintf("Connection test error: %v", err)
			return check
		}
		if result.Connected {
			check.Passed = true
			check.Message = fmt.Sprintf("PERMISSIVE mode: mTLS available (%s, %s)",
				result.TLSVersion, result.CipherSuite)
		} else {
			// mTLS failed, try plaintext
			plaintextOK, _ := tester.TestPlaintext(address)
			if plaintextOK {
				check.Passed = true
				check.Severity = "warning"
				check.Message = "PERMISSIVE mode: only plaintext available (mTLS recommended)"
			} else {
				check.Passed = true
				check.Severity = "warning"
				check.Message = "Service unreachable for connection test"
			}
		}

	case PolicyModeDisable:
		// For DISABLE mode: plaintext should work
		plaintextOK, err := tester.TestPlaintext(address)
		switch {
		case plaintextOK:
			check.Passed = true
			check.Message = "DISABLE mode: plaintext connection available"
		case err != nil:
			check.Passed = true
			check.Severity = "warning"
			check.Message = fmt.Sprintf("Service unreachable: %v", err)
		default:
			check.Passed = true
			check.Message = "DISABLE mode: connection test completed"
		}

	default:
		check.Passed = true
		check.Message = fmt.Sprintf("Unknown policy mode: %s", policy.Mode)
	}

	return check
}

// buildExpectedSPIFFEPattern builds the expected SPIFFE ID pattern for a service
func buildExpectedSPIFFEPattern(namespace, service, trustDomain string) string {
	if trustDomain == "" {
		trustDomain = "cluster.local"
	}
	// Istio SPIFFE ID format: spiffe://<trust-domain>/ns/<namespace>/sa/<service-account>
	// We use a wildcard for the service account
	return fmt.Sprintf("spiffe://%s/ns/%s/*", trustDomain, namespace)
}

// VerifyConnectionSecurity performs an actual TLS connection verification
func (v *PolicyVerifier) VerifyConnectionSecurity(ctx context.Context, address string, expectedMode PolicyMode) (*PolicyCheck, error) {
	check := &PolicyCheck{
		Name:        "live_connection_security",
		Description: "Verify live connection security",
		Severity:    "critical",
	}

	// Parse address
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	// Try plaintext connection
	d := &net.Dialer{Timeout: 5 * time.Second}
	plaintextConn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		// Connection failed
		if expectedMode == PolicyModeStrict {
			check.Passed = true
			check.Message = "Plaintext connection correctly rejected"
		} else {
			check.Passed = false
			check.Message = fmt.Sprintf("Connection failed: %v", err)
		}
		return check, nil
	}
	plaintextConn.Close()

	// For STRICT mode, plaintext should have been rejected
	if expectedMode == PolicyModeStrict {
		check.Passed = false
		check.Message = "STRICT mode should reject plaintext connections"
		return check, nil
	}

	// Try TLS connection
	v.mu.RLock()
	metadata := v.metadata
	v.mu.RUnlock()

	minVersion := ""
	if metadata != nil && metadata.TLSConfig != nil {
		minVersion = metadata.TLSConfig.MinVersion
	}

	netConn, err := (&tls.Dialer{Config: newTLSBaseConfig(host, minVersion)}).DialContext(ctx, "tcp", address)
	if err != nil {
		// If we have metadata, try with custom CA
		if metadata != nil && metadata.TLSConfig != nil && metadata.TLSConfig.CAFile != "" {
			caPEM, err := os.ReadFile(metadata.TLSConfig.CAFile)
			if err == nil {
				roots := x509.NewCertPool()
				roots.AppendCertsFromPEM(caPEM)

				tlsConfig := newTLSBaseConfig(host, minVersion)
				tlsConfig.RootCAs = roots
				netConn, err = (&tls.Dialer{Config: tlsConfig}).DialContext(ctx, "tcp", address)
				if err == nil {
					defer netConn.Close()
					check.Passed = true
					check.Message = "TLS connection verified with custom CA"
					return check, nil
				}
			}
		}
		check.Passed = false
		check.Message = fmt.Sprintf("TLS connection failed: %v", err)
		return check, nil
	}
	defer netConn.Close()

	// Verify connection state
	tlsConn := netConn.(*tls.Conn)
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		check.Passed = false
		check.Message = "No peer certificates received"
		return check, nil
	}

	check.Passed = true
	check.Message = fmt.Sprintf("TLS connection verified (version: %s, cipher: %s)",
		tlsVersionString(state.Version),
		tls.CipherSuiteName(state.CipherSuite))

	return check, nil
}

// tlsVersionString converts TLS version to string
func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("0x%04x", version)
	}
}

// PolicyStore stores and retrieves mTLS policies
type PolicyStore struct {
	policies map[string]*MTLSPolicy // key: namespace/service
	mu       sync.RWMutex
}

// NewPolicyStore creates a new policy store
func NewPolicyStore() *PolicyStore {
	return &PolicyStore{
		policies: make(map[string]*MTLSPolicy),
	}
}

// Add adds a policy to the store
func (s *PolicyStore) Add(policy *MTLSPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := policyKey(policy.Namespace, policy.Service)
	policy.CreatedAt = time.Now()
	policy.UpdatedAt = policy.CreatedAt
	s.policies[key] = policy
}

// Get retrieves a policy from the store
func (s *PolicyStore) Get(namespace, service string) (*MTLSPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := policyKey(namespace, service)
	policy, ok := s.policies[key]
	if ok {
		return policy, true
	}

	// Try namespace-wide policy
	key = policyKey(namespace, "")
	policy, ok = s.policies[key]
	return policy, ok
}

// Remove removes a policy from the store
func (s *PolicyStore) Remove(namespace, service string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := policyKey(namespace, service)
	if _, ok := s.policies[key]; ok {
		delete(s.policies, key)
		return true
	}
	return false
}

// List returns all policies
func (s *PolicyStore) List() []*MTLSPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	policies := make([]*MTLSPolicy, 0, len(s.policies))
	for _, p := range s.policies {
		policies = append(policies, p)
	}
	return policies
}

// policyKey generates a key for a policy
func policyKey(namespace, service string) string {
	if service == "" {
		return namespace
	}
	return namespace + "/" + service
}

// GetEffectivePolicy returns the effective policy for a service
func (s *PolicyStore) GetEffectivePolicy(namespace, service string) *MTLSPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check service-specific policy first
	key := policyKey(namespace, service)
	if policy, ok := s.policies[key]; ok {
		return policy
	}

	// Fall back to namespace policy
	key = policyKey(namespace, "")
	if policy, ok := s.policies[key]; ok {
		return policy
	}

	// Fall back to default policy
	if policy, ok := s.policies["default"]; ok {
		return policy
	}

	// Return strict mode as default
	return &MTLSPolicy{
		Name: "default-strict",
		Mode: PolicyModeStrict,
	}
}
