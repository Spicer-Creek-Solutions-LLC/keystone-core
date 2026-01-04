// Package federation provides trust federation between identity providers.
package federation

import (
	"context"
	"crypto/x509"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/identity"
)

// Manager implements the FederationManager interface.
type Manager struct {
	config *FederationConfig

	mu               sync.RWMutex
	federatedDomains map[string]*FederatedDomain
	started          bool
	stopCh           chan struct{}
}

// NewManager creates a new federation manager.
func NewManager(config *FederationConfig) (*Manager, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.LocalTrustDomain == "" {
		return nil, fmt.Errorf("local trust domain is required")
	}

	return &Manager{
		config:           config,
		federatedDomains: make(map[string]*FederatedDomain),
	}, nil
}

// AddFederatedDomain adds a new federated trust domain.
func (m *Manager) AddFederatedDomain(ctx context.Context, domain *FederatedDomain) error {
	if domain == nil {
		return fmt.Errorf("domain is required")
	}
	if domain.TrustDomain == "" {
		return fmt.Errorf("trust domain is required")
	}
	if domain.TrustDomain == m.config.LocalTrustDomain {
		return fmt.Errorf("cannot federate with local trust domain")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check max domains limit
	if m.config.MaxFederatedDomains > 0 && len(m.federatedDomains) >= m.config.MaxFederatedDomains {
		return fmt.Errorf("maximum number of federated domains reached (%d)", m.config.MaxFederatedDomains)
	}

	// Check if already exists
	if _, exists := m.federatedDomains[domain.TrustDomain]; exists {
		return fmt.Errorf("trust domain %s is already federated", domain.TrustDomain)
	}

	// Set defaults
	if domain.RefreshInterval == 0 {
		domain.RefreshInterval = m.config.DefaultRefreshInterval
	}
	if domain.Policy == nil && m.config.DefaultPolicy != nil {
		domain.Policy = m.config.DefaultPolicy
	}
	if domain.CreatedAt.IsZero() {
		domain.CreatedAt = time.Now()
	}
	domain.UpdatedAt = time.Now()

	// Set initial state
	if m.config.RequireApproval && domain.State == "" {
		domain.State = FederationStatePending
	} else if domain.State == "" {
		domain.State = FederationStateActive
	}

	// Store in memory
	m.federatedDomains[domain.TrustDomain] = domain

	// Persist if store is configured
	if m.config.Store != nil {
		if err := m.config.Store.Save(ctx, domain); err != nil {
			delete(m.federatedDomains, domain.TrustDomain)
			return fmt.Errorf("failed to persist federation: %w", err)
		}
	}

	// Emit event
	m.emitEvent(FederationEventAdded, domain.TrustDomain, nil)

	return nil
}

// RemoveFederatedDomain removes a federated trust domain.
func (m *Manager) RemoveFederatedDomain(ctx context.Context, trustDomain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.federatedDomains[trustDomain]; !exists {
		return fmt.Errorf("trust domain %s is not federated", trustDomain)
	}

	delete(m.federatedDomains, trustDomain)

	// Delete from store
	if m.config.Store != nil {
		if err := m.config.Store.Delete(ctx, trustDomain); err != nil {
			return fmt.Errorf("failed to delete from store: %w", err)
		}
	}

	// Emit event
	m.emitEvent(FederationEventRemoved, trustDomain, nil)

	return nil
}

// GetFederatedDomain retrieves a federated domain by trust domain.
func (m *Manager) GetFederatedDomain(ctx context.Context, trustDomain string) (*FederatedDomain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	domain, exists := m.federatedDomains[trustDomain]
	if !exists {
		return nil, fmt.Errorf("trust domain %s is not federated", trustDomain)
	}

	return domain, nil
}

// ListFederatedDomains lists all federated domains.
func (m *Manager) ListFederatedDomains(ctx context.Context) ([]*FederatedDomain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	domains := make([]*FederatedDomain, 0, len(m.federatedDomains))
	for _, domain := range m.federatedDomains {
		domains = append(domains, domain)
	}

	return domains, nil
}

// UpdateFederatedDomain updates a federated domain.
func (m *Manager) UpdateFederatedDomain(ctx context.Context, domain *FederatedDomain) error {
	if domain == nil {
		return fmt.Errorf("domain is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.federatedDomains[domain.TrustDomain]
	if !exists {
		return fmt.Errorf("trust domain %s is not federated", domain.TrustDomain)
	}

	// Preserve created time
	domain.CreatedAt = existing.CreatedAt
	domain.UpdatedAt = time.Now()

	m.federatedDomains[domain.TrustDomain] = domain

	// Persist if store is configured
	if m.config.Store != nil {
		if err := m.config.Store.Save(ctx, domain); err != nil {
			return fmt.Errorf("failed to persist update: %w", err)
		}
	}

	// Emit event
	m.emitEvent(FederationEventUpdated, domain.TrustDomain, nil)

	return nil
}

// RefreshTrustBundle refreshes the trust bundle for a domain.
func (m *Manager) RefreshTrustBundle(ctx context.Context, trustDomain string) error {
	m.mu.RLock()
	domain, exists := m.federatedDomains[trustDomain]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("trust domain %s is not federated", trustDomain)
	}

	if domain.BundleEndpoint == "" {
		return fmt.Errorf("no bundle endpoint configured for %s", trustDomain)
	}

	if m.config.BundleFetcher == nil {
		return fmt.Errorf("no bundle fetcher configured")
	}

	bundle, err := m.config.BundleFetcher.Fetch(ctx, domain.BundleEndpoint, domain.BundleEndpointProfile)
	if err != nil {
		return fmt.Errorf("failed to fetch trust bundle: %w", err)
	}

	m.mu.Lock()
	if d, exists := m.federatedDomains[trustDomain]; exists {
		d.TrustBundle = bundle
		d.UpdatedAt = time.Now()
	}
	m.mu.Unlock()

	// Emit event
	m.emitEvent(FederationEventRefreshed, trustDomain, nil)

	return nil
}

// GetAggregatedTrustBundle returns a combined trust bundle from all active federations.
func (m *Manager) GetAggregatedTrustBundle(ctx context.Context) (*identity.TrustBundle, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allCerts []*x509.Certificate
	var allJWTAuthorities []identity.JWTAuthority

	// Include local trust bundle
	if m.config.LocalTrustBundle != nil {
		allCerts = append(allCerts, m.config.LocalTrustBundle.X509Authorities...)
		allJWTAuthorities = append(allJWTAuthorities, m.config.LocalTrustBundle.JWTAuthorities...)
	}

	// Include federated trust bundles
	for _, domain := range m.federatedDomains {
		if !domain.IsActive() {
			continue
		}
		if domain.TrustBundle != nil {
			allCerts = append(allCerts, domain.TrustBundle.X509Authorities...)
			allJWTAuthorities = append(allJWTAuthorities, domain.TrustBundle.JWTAuthorities...)
		}
	}

	return &identity.TrustBundle{
		TrustDomain:     m.config.LocalTrustDomain,
		X509Authorities: allCerts,
		JWTAuthorities:  allJWTAuthorities,
		UpdatedAt:       time.Now(),
	}, nil
}

// ValidateSVID validates an SVID against federated trust bundles.
func (m *Manager) ValidateSVID(ctx context.Context, svid *identity.X509SVID) (*ValidationResult, error) {
	if svid == nil || len(svid.Certificates) == 0 {
		return &ValidationResult{
			Valid:       false,
			Error:       "no certificates in SVID",
			ValidatedAt: time.Now(),
		}, nil
	}

	spiffeID := svid.SPIFFEID
	trustDomain := spiffeID.TrustDomain

	result := &ValidationResult{
		SPIFFEID:    spiffeID,
		TrustDomain: trustDomain,
		ValidatedAt: time.Now(),
		ExpiresAt:   svid.ExpiresAt,
	}

	// Check if local domain
	if trustDomain == m.config.LocalTrustDomain {
		result.IsFederated = false
		if err := m.validateCertChain(svid.Certificates, m.config.LocalTrustBundle); err != nil {
			result.Valid = false
			result.Error = err.Error()
			return result, nil
		}
		result.Valid = true
		result.CertificateChain = svid.Certificates
		return result, nil
	}

	// Check federated domains
	m.mu.RLock()
	domain, exists := m.federatedDomains[trustDomain]
	m.mu.RUnlock()

	if !exists {
		result.Valid = false
		result.Error = fmt.Sprintf("trust domain %s is not federated", trustDomain)
		m.emitEvent(FederationEventValidationFailed, trustDomain, map[string]string{
			"reason":    "not_federated",
			"spiffe_id": spiffeID.String(),
		})
		return result, nil
	}

	if !domain.IsActive() {
		result.Valid = false
		result.Error = fmt.Sprintf("federation with %s is not active (state: %s)", trustDomain, domain.State)
		m.emitEvent(FederationEventValidationFailed, trustDomain, map[string]string{
			"reason":    "federation_not_active",
			"state":     string(domain.State),
			"spiffe_id": spiffeID.String(),
		})
		return result, nil
	}

	result.IsFederated = true
	result.FederationType = domain.Type

	// Validate certificate chain
	if domain.TrustBundle == nil || len(domain.TrustBundle.X509Authorities) == 0 {
		result.Valid = false
		result.Error = fmt.Sprintf("no trust bundle for %s", trustDomain)
		return result, nil
	}

	if err := m.validateCertChain(svid.Certificates, domain.TrustBundle); err != nil {
		result.Valid = false
		result.Error = err.Error()
		m.emitEvent(FederationEventValidationFailed, trustDomain, map[string]string{
			"reason":    "cert_validation_failed",
			"error":     err.Error(),
			"spiffe_id": spiffeID.String(),
		})
		return result, nil
	}

	// Apply policy
	if domain.Policy != nil {
		if err := m.applyPolicy(spiffeID, domain.Policy); err != nil {
			result.Valid = false
			result.Error = err.Error()
			result.MatchedPolicy = domain.Policy.Name
			m.emitEvent(FederationEventValidationFailed, trustDomain, map[string]string{
				"reason":    "policy_denied",
				"policy":    domain.Policy.Name,
				"error":     err.Error(),
				"spiffe_id": spiffeID.String(),
			})
			return result, nil
		}
		result.MatchedPolicy = domain.Policy.Name
	}

	result.Valid = true
	result.CertificateChain = svid.Certificates
	return result, nil
}

// Start starts background trust bundle refresh.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = true
	m.stopCh = make(chan struct{})
	m.mu.Unlock()

	// Load persisted federations
	if m.config.Store != nil {
		domains, err := m.config.Store.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to load federations: %w", err)
		}
		m.mu.Lock()
		for _, domain := range domains {
			m.federatedDomains[domain.TrustDomain] = domain
		}
		m.mu.Unlock()
	}

	// Start refresh loop
	go m.refreshLoop()

	return nil
}

// Stop stops the federation manager.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}
	m.started = false

	if m.stopCh != nil {
		close(m.stopCh)
	}

	return nil
}

// Private methods

func (m *Manager) validateCertChain(certs []*x509.Certificate, bundle *identity.TrustBundle) error {
	if len(certs) == 0 {
		return fmt.Errorf("no certificates to validate")
	}
	if bundle == nil || len(bundle.X509Authorities) == 0 {
		return fmt.Errorf("no trust bundle")
	}

	leaf := certs[0]

	// Check expiration
	now := time.Now()
	if now.Before(leaf.NotBefore) {
		return fmt.Errorf("certificate not yet valid")
	}
	if now.After(leaf.NotAfter) {
		return fmt.Errorf("certificate has expired")
	}

	// Build intermediates pool
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}

	// Build roots pool
	roots := x509.NewCertPool()
	for _, cert := range bundle.X509Authorities {
		roots.AddCert(cert)
	}

	// Verify certificate chain
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   now,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}

	if _, err := leaf.Verify(opts); err != nil {
		return fmt.Errorf("certificate chain verification failed: %w", err)
	}

	return nil
}

func (m *Manager) applyPolicy(spiffeID identity.SPIFFEID, policy *TrustPolicy) error {
	// Check denied paths first (takes precedence)
	for _, pattern := range policy.DeniedPaths {
		if matchPath(spiffeID.Path, pattern) {
			return fmt.Errorf("path %s is denied by policy", spiffeID.Path)
		}
	}

	// Check allowed paths (if specified)
	if len(policy.AllowedPaths) > 0 {
		allowed := false
		for _, pattern := range policy.AllowedPaths {
			if matchPath(spiffeID.Path, pattern) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("path %s is not allowed by policy", spiffeID.Path)
		}
	}

	// Check denied services
	serviceName := extractServiceName(spiffeID.Path)
	for _, denied := range policy.DeniedServices {
		if serviceName == denied {
			return fmt.Errorf("service %s is denied by policy", serviceName)
		}
	}

	// Check allowed services (if specified)
	if len(policy.AllowedServices) > 0 {
		allowed := false
		for _, svc := range policy.AllowedServices {
			if serviceName == svc {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("service %s is not allowed by policy", serviceName)
		}
	}

	return nil
}

func (m *Manager) emitEvent(eventType FederationEventType, trustDomain string, details map[string]string) {
	if m.config.EventCallback == nil {
		return
	}

	event := &FederationEvent{
		Type:        eventType,
		TrustDomain: trustDomain,
		Timestamp:   time.Now(),
		Details:     details,
	}
	m.config.EventCallback(event)
}

func (m *Manager) refreshLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.refreshAllBundles()
			m.checkExpirations()
		}
	}
}

func (m *Manager) refreshAllBundles() {
	m.mu.RLock()
	domains := make([]*FederatedDomain, 0, len(m.federatedDomains))
	for _, d := range m.federatedDomains {
		domains = append(domains, d)
	}
	m.mu.RUnlock()

	for _, domain := range domains {
		if !domain.IsActive() {
			continue
		}
		if domain.BundleEndpoint == "" {
			continue
		}

		// Check if refresh is needed
		if domain.TrustBundle != nil &&
			time.Since(domain.TrustBundle.UpdatedAt) < domain.RefreshInterval {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := m.RefreshTrustBundle(ctx, domain.TrustDomain); err != nil {
			// Log error but continue
		}
		cancel()
	}
}

func (m *Manager) checkExpirations() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for trustDomain, domain := range m.federatedDomains {
		if domain.State == FederationStateActive && domain.IsExpired() {
			domain.State = FederationStateExpired
			m.emitEvent(FederationEventExpired, trustDomain, nil)
		}
	}
}

// Helper functions

func matchPath(path, pattern string) bool {
	// Simple glob matching
	if pattern == "*" || pattern == "/**" {
		return true
	}

	// Convert pattern to glob-compatible
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(path, prefix)
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		// Match only direct children
		if !strings.HasPrefix(path, prefix+"/") {
			return false
		}
		rest := strings.TrimPrefix(path, prefix+"/")
		return !strings.Contains(rest, "/")
	}

	// Try filepath.Match for other patterns
	matched, _ := filepath.Match(pattern, path)
	return matched
}

func extractServiceName(path string) string {
	// Common SPIFFE ID path patterns:
	// /ns/{namespace}/sa/{serviceaccount}
	// /service/{name}
	// /agent/{id}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}

	switch parts[0] {
	case "service":
		return parts[1]
	case "ns":
		if len(parts) >= 4 && parts[2] == "sa" {
			return parts[3]
		}
	case "sa":
		return parts[1]
	}

	return parts[len(parts)-1]
}

// Verify Manager implements FederationManager
var _ FederationManager = (*Manager)(nil)
