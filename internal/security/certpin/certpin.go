// Package certpin provides certificate pinning for service-to-service communication.
package certpin

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// PinType represents the type of certificate pinning
type PinType string

const (
	// PinTypeSPKI pins the Subject Public Key Info (recommended)
	PinTypeSPKI PinType = "spki"
	// PinTypeCertificate pins the entire certificate
	PinTypeCertificate PinType = "certificate"
	// PinTypePublicKey pins only the public key
	PinTypePublicKey PinType = "public_key"
)

// EnforcementMode determines how pin violations are handled
type EnforcementMode string

const (
	// ModeEnforce rejects connections that fail pin validation
	ModeEnforce EnforcementMode = "enforce"
	// ModeReportOnly logs violations but allows connections
	ModeReportOnly EnforcementMode = "report_only"
	// ModeDisabled disables pinning entirely
	ModeDisabled EnforcementMode = "disabled"
)

// Pin represents a certificate pin
type Pin struct {
	// Hash is the SHA-256 hash of the pinned data
	Hash string

	// Type is the type of pin
	Type PinType

	// Comment is an optional description
	Comment string

	// ExpiresAt is when this pin expires
	ExpiresAt *time.Time

	// IsBackup indicates this is a backup pin for rotation
	IsBackup bool
}

// ServiceConfig configures pinning for a specific service
type ServiceConfig struct {
	// Name is the service identifier
	Name string

	// Hosts are the hostnames this config applies to
	Hosts []string

	// Pins are the allowed certificate pins
	Pins []*Pin

	// Mode is the enforcement mode
	Mode EnforcementMode

	// IncludeSubdomains applies to all subdomains
	IncludeSubdomains bool

	// ReportURI is where to send violation reports
	ReportURI string
}

// Config configures the certificate pinner
type Config struct {
	// DefaultMode is the default enforcement mode
	DefaultMode EnforcementMode

	// Services is the per-service configuration
	Services []*ServiceConfig

	// OnViolation is called when a pin violation occurs
	OnViolation func(report *ViolationReport)

	// CacheExpiry is how long to cache verification results
	CacheExpiry time.Duration

	// AllowSystemRoots allows system CA roots in addition to pins
	AllowSystemRoots bool
}

// DefaultConfig returns a default pinning configuration
func DefaultConfig() *Config {
	return &Config{
		DefaultMode:      ModeEnforce,
		Services:         make([]*ServiceConfig, 0),
		CacheExpiry:      5 * time.Minute,
		AllowSystemRoots: false,
	}
}

// ViolationReport contains information about a pin violation
type ViolationReport struct {
	// Timestamp is when the violation occurred
	Timestamp time.Time

	// Hostname is the hostname being connected to
	Hostname string

	// Port is the port being connected to
	Port string

	// ServedCertificateChain is the certificate chain from the server
	ServedCertificateChain []string

	// ValidatedCertificateChain is the validated chain (if any)
	ValidatedCertificateChain []string

	// KnownPins are the expected pins
	KnownPins []string

	// EffectiveExpirationDate is when the current pins expire
	EffectiveExpirationDate *time.Time

	// IncludeSubdomains indicates if subdomains are included
	IncludeSubdomains bool

	// EnforcementMode is the current mode
	EnforcementMode EnforcementMode
}

// Pinner performs certificate pinning verification
type Pinner struct {
	config     *Config
	services   map[string]*ServiceConfig
	mu         sync.RWMutex
	cache      map[string]*cacheEntry
	cacheMu    sync.RWMutex
	stats      *Stats
}

type cacheEntry struct {
	valid     bool
	expiresAt time.Time
}

// NewPinner creates a new certificate pinner
func NewPinner(config *Config) *Pinner {
	if config == nil {
		config = DefaultConfig()
	}

	p := &Pinner{
		config:   config,
		services: make(map[string]*ServiceConfig),
		cache:    make(map[string]*cacheEntry),
		stats:    NewStats(),
	}

	// Index services by hostname
	for _, svc := range config.Services {
		for _, host := range svc.Hosts {
			p.services[host] = svc
		}
	}

	return p
}

// AddService adds or updates a service configuration
func (p *Pinner) AddService(svc *ServiceConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, host := range svc.Hosts {
		p.services[host] = svc
	}
}

// RemoveService removes a service configuration
func (p *Pinner) RemoveService(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for host, svc := range p.services {
		if svc.Name == name {
			delete(p.services, host)
		}
	}
}

// GetService returns the service config for a hostname
func (p *Pinner) GetService(hostname string) *ServiceConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Exact match
	if svc, ok := p.services[hostname]; ok {
		return svc
	}

	// Check for wildcard/subdomain matches
	parts := strings.Split(hostname, ".")
	for i := 1; i < len(parts); i++ {
		parent := strings.Join(parts[i:], ".")
		if svc, ok := p.services[parent]; ok && svc.IncludeSubdomains {
			return svc
		}
	}

	return nil
}

// Verify verifies a certificate chain against configured pins
func (p *Pinner) Verify(hostname string, certs []*x509.Certificate) error {
	svc := p.GetService(hostname)
	if svc == nil {
		// No pinning configured for this host
		p.stats.RecordVerification(hostname, true, false)
		return nil
	}

	if svc.Mode == ModeDisabled {
		p.stats.RecordVerification(hostname, true, false)
		return nil
	}

	// Check cache
	cacheKey := p.cacheKey(hostname, certs)
	if entry := p.getCache(cacheKey); entry != nil {
		if entry.valid {
			p.stats.RecordVerification(hostname, true, true)
			return nil
		}
		p.stats.RecordVerification(hostname, false, true)
		if svc.Mode == ModeEnforce {
			return errors.New("certificate pinning failed (cached)")
		}
		return nil
	}

	// Verify pins
	valid := p.verifyPins(svc, certs)

	// Cache result
	p.setCache(cacheKey, valid)

	if !valid {
		p.stats.RecordViolation(hostname)
		p.reportViolation(hostname, svc, certs)

		if svc.Mode == ModeEnforce {
			return fmt.Errorf("certificate pinning failed for %s: no matching pin found", hostname)
		}
	}

	p.stats.RecordVerification(hostname, valid, false)
	return nil
}

func (p *Pinner) verifyPins(svc *ServiceConfig, certs []*x509.Certificate) bool {
	for _, cert := range certs {
		for _, pin := range svc.Pins {
			// Check expiration
			if pin.ExpiresAt != nil && time.Now().After(*pin.ExpiresAt) {
				continue
			}

			hash := p.computeHash(cert, pin.Type)
			if hash == pin.Hash {
				return true
			}
		}
	}
	return false
}

func (p *Pinner) computeHash(cert *x509.Certificate, pinType PinType) string {
	var data []byte

	switch pinType {
	case PinTypeSPKI:
		// Hash the SubjectPublicKeyInfo
		data = cert.RawSubjectPublicKeyInfo
	case PinTypeCertificate:
		// Hash the entire certificate
		data = cert.Raw
	case PinTypePublicKey:
		// Hash the public key bytes
		data = cert.RawSubjectPublicKeyInfo
	default:
		// Default to SPKI
		data = cert.RawSubjectPublicKeyInfo
	}

	hash := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(hash[:])
}

func (p *Pinner) cacheKey(hostname string, certs []*x509.Certificate) string {
	if len(certs) == 0 {
		return hostname
	}
	// Include first cert fingerprint in cache key
	hash := sha256.Sum256(certs[0].Raw)
	return hostname + ":" + hex.EncodeToString(hash[:8])
}

func (p *Pinner) getCache(key string) *cacheEntry {
	p.cacheMu.RLock()
	defer p.cacheMu.RUnlock()

	entry, ok := p.cache[key]
	if !ok {
		return nil
	}
	if time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry
}

func (p *Pinner) setCache(key string, valid bool) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()

	p.cache[key] = &cacheEntry{
		valid:     valid,
		expiresAt: time.Now().Add(p.config.CacheExpiry),
	}
}

// ClearCache clears the verification cache
func (p *Pinner) ClearCache() {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	p.cache = make(map[string]*cacheEntry)
}

func (p *Pinner) reportViolation(hostname string, svc *ServiceConfig, certs []*x509.Certificate) {
	if p.config.OnViolation == nil {
		return
	}

	report := &ViolationReport{
		Timestamp:              time.Now(),
		Hostname:               hostname,
		EnforcementMode:        svc.Mode,
		IncludeSubdomains:      svc.IncludeSubdomains,
		ServedCertificateChain: make([]string, 0, len(certs)),
		KnownPins:              make([]string, 0, len(svc.Pins)),
	}

	// Add served certificate fingerprints
	for _, cert := range certs {
		hash := sha256.Sum256(cert.Raw)
		report.ServedCertificateChain = append(
			report.ServedCertificateChain,
			base64.StdEncoding.EncodeToString(hash[:]),
		)
	}

	// Add known pins
	for _, pin := range svc.Pins {
		report.KnownPins = append(report.KnownPins, pin.Hash)
		if pin.ExpiresAt != nil {
			if report.EffectiveExpirationDate == nil || pin.ExpiresAt.Before(*report.EffectiveExpirationDate) {
				report.EffectiveExpirationDate = pin.ExpiresAt
			}
		}
	}

	p.config.OnViolation(report)
}

// VerifyCallback returns a function suitable for use as tls.Config.VerifyPeerCertificate
func (p *Pinner) VerifyCallback(hostname string) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		// Use verified chains if available, otherwise parse raw certs
		var certs []*x509.Certificate
		if len(verifiedChains) > 0 {
			certs = verifiedChains[0]
		} else {
			certs = make([]*x509.Certificate, 0, len(rawCerts))
			for _, raw := range rawCerts {
				cert, err := x509.ParseCertificate(raw)
				if err != nil {
					return fmt.Errorf("failed to parse certificate: %w", err)
				}
				certs = append(certs, cert)
			}
		}

		return p.Verify(hostname, certs)
	}
}

// TLSConfig returns a TLS config with certificate pinning enabled
func (p *Pinner) TLSConfig(hostname string) *tls.Config {
	return &tls.Config{
		ServerName:            hostname,
		VerifyPeerCertificate: p.VerifyCallback(hostname),
		MinVersion:            tls.VersionTLS12,
	}
}

// Dialer returns a dialer function that performs certificate pinning
func (p *Pinner) Dialer(hostname string, timeout time.Duration) func(network, addr string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{Timeout: timeout}
		conn, err := tls.DialWithDialer(dialer, network, addr, p.TLSConfig(hostname))
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
}

// Stats returns the current statistics
func (p *Pinner) Stats() *Stats {
	return p.stats
}

// ComputeSPKIPin computes the SPKI pin for a certificate
func ComputeSPKIPin(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(hash[:])
}

// ComputeCertificatePin computes the certificate pin for a certificate
func ComputeCertificatePin(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return base64.StdEncoding.EncodeToString(hash[:])
}

// ParsePEM parses a PEM-encoded certificate
func ParsePEM(pemData []byte) ([]*x509.Certificate, error) {
	return x509.ParseCertificates(pemData)
}

// NewPin creates a new pin from a certificate
func NewPin(cert *x509.Certificate, pinType PinType, comment string) *Pin {
	var hash string
	switch pinType {
	case PinTypeSPKI:
		hash = ComputeSPKIPin(cert)
	case PinTypeCertificate:
		hash = ComputeCertificatePin(cert)
	default:
		hash = ComputeSPKIPin(cert)
		pinType = PinTypeSPKI
	}

	return &Pin{
		Hash:    hash,
		Type:    pinType,
		Comment: comment,
	}
}

// NewPinFromHash creates a pin from an existing hash
func NewPinFromHash(hash string, pinType PinType, comment string) *Pin {
	return &Pin{
		Hash:    hash,
		Type:    pinType,
		Comment: comment,
	}
}

// Stats tracks certificate pinning statistics
type Stats struct {
	mu              sync.Mutex
	TotalVerifications int64
	SuccessfulVerifications int64
	FailedVerifications int64
	CacheHits       int64
	CacheMisses     int64
	Violations      int64
	ByHost          map[string]*HostStats
}

// HostStats tracks stats for a specific hostname
type HostStats struct {
	Verifications int64
	Successes     int64
	Failures      int64
	Violations    int64
}

// NewStats creates a new stats tracker
func NewStats() *Stats {
	return &Stats{
		ByHost: make(map[string]*HostStats),
	}
}

// RecordVerification records a verification attempt
func (s *Stats) RecordVerification(hostname string, success bool, cached bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalVerifications++
	if success {
		s.SuccessfulVerifications++
	} else {
		s.FailedVerifications++
	}
	if cached {
		s.CacheHits++
	} else {
		s.CacheMisses++
	}

	if _, ok := s.ByHost[hostname]; !ok {
		s.ByHost[hostname] = &HostStats{}
	}
	s.ByHost[hostname].Verifications++
	if success {
		s.ByHost[hostname].Successes++
	} else {
		s.ByHost[hostname].Failures++
	}
}

// RecordViolation records a pin violation
func (s *Stats) RecordViolation(hostname string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Violations++
	if _, ok := s.ByHost[hostname]; !ok {
		s.ByHost[hostname] = &HostStats{}
	}
	s.ByHost[hostname].Violations++
}

// Snapshot returns a copy of current stats
func (s *Stats) Snapshot() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := Stats{
		TotalVerifications:      s.TotalVerifications,
		SuccessfulVerifications: s.SuccessfulVerifications,
		FailedVerifications:     s.FailedVerifications,
		CacheHits:               s.CacheHits,
		CacheMisses:             s.CacheMisses,
		Violations:              s.Violations,
		ByHost:                  make(map[string]*HostStats),
	}

	for host, stats := range s.ByHost {
		snapshot.ByHost[host] = &HostStats{
			Verifications: stats.Verifications,
			Successes:     stats.Successes,
			Failures:      stats.Failures,
			Violations:    stats.Violations,
		}
	}

	return snapshot
}
