// Package agent provides the Keystone Core agent implementation
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// EndpointType identifies the type of endpoint being advertised
type EndpointType string

const (
	// EndpointTypeNATS is a standard NATS endpoint
	EndpointTypeNATS EndpointType = "nats"
	// EndpointTypeNATSTLS is a TLS-secured NATS endpoint
	EndpointTypeNATSTLS EndpointType = "nats-tls"
	// EndpointTypeWebSocket is a WebSocket NATS endpoint
	EndpointTypeWebSocket EndpointType = "nats-ws"
	// EndpointTypeWebSocketTLS is a TLS WebSocket NATS endpoint
	EndpointTypeWebSocketTLS EndpointType = "nats-wss"
)

// EndpointAdvertisement represents an agent's advertised NATS endpoint
type EndpointAdvertisement struct {
	// AgentID is the unique identifier of the advertising agent
	AgentID string `json:"agent_id"`

	// EndpointType indicates the type of endpoint
	EndpointType EndpointType `json:"endpoint_type"`

	// Host is the hostname or IP address
	Host string `json:"host"`

	// Port is the port number
	Port int `json:"port"`

	// PublicHost is the externally reachable host (may differ from Host if behind NAT)
	PublicHost string `json:"public_host,omitempty"`

	// PublicPort is the externally reachable port (may differ from Port if NAT mapped)
	PublicPort int `json:"public_port,omitempty"`

	// LocalAddresses lists all local IP addresses
	LocalAddresses []string `json:"local_addresses,omitempty"`

	// TLSEnabled indicates if TLS is required
	TLSEnabled bool `json:"tls_enabled"`

	// AuthRequired indicates if authentication is required
	AuthRequired bool `json:"auth_required"`

	// Capabilities lists supported features
	Capabilities []string `json:"capabilities,omitempty"`

	// Metadata contains additional endpoint metadata
	Metadata map[string]string `json:"metadata,omitempty"`

	// TTL is how long this advertisement is valid (seconds)
	TTL int64 `json:"ttl"`

	// Timestamp is when this advertisement was created
	Timestamp time.Time `json:"timestamp"`

	// SequenceNumber increments with each advertisement update
	SequenceNumber int64 `json:"sequence_number"`

	// HealthStatus indicates the current health of the endpoint
	HealthStatus EndpointHealthStatus `json:"health_status"`
}

// EndpointHealthStatus represents the health of an advertised endpoint
type EndpointHealthStatus string

const (
	// EndpointHealthUnknown status is not known
	EndpointHealthUnknown EndpointHealthStatus = "unknown"
	// EndpointHealthHealthy endpoint is healthy and accepting connections
	EndpointHealthHealthy EndpointHealthStatus = "healthy"
	// EndpointHealthDegraded endpoint is operational but degraded
	EndpointHealthDegraded EndpointHealthStatus = "degraded"
	// EndpointHealthUnhealthy endpoint is unhealthy
	EndpointHealthUnhealthy EndpointHealthStatus = "unhealthy"
)

// GetURL returns the NATS URL for this endpoint
func (e *EndpointAdvertisement) GetURL() string {
	host := e.PublicHost
	if host == "" {
		host = e.Host
	}

	port := e.PublicPort
	if port == 0 {
		port = e.Port
	}

	var scheme string
	switch e.EndpointType {
	case EndpointTypeNATSTLS:
		scheme = "tls"
	case EndpointTypeWebSocket:
		scheme = "ws"
	case EndpointTypeWebSocketTLS:
		scheme = "wss"
	default:
		scheme = "nats"
	}

	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}

// IsExpired checks if this advertisement has expired
func (e *EndpointAdvertisement) IsExpired() bool {
	return time.Since(e.Timestamp) > time.Duration(e.TTL)*time.Second
}

// Validate validates the advertisement
func (e *EndpointAdvertisement) Validate() error {
	if e.AgentID == "" {
		return errors.New("agent_id is required")
	}
	if e.Host == "" {
		return errors.New("host is required")
	}
	if e.Port <= 0 || e.Port > 65535 {
		return errors.New("invalid port")
	}
	if e.TTL <= 0 {
		return errors.New("TTL must be positive")
	}
	return nil
}

// EndpointAdvertiserConfig configures the endpoint advertiser
type EndpointAdvertiserConfig struct {
	// AgentID is the agent's unique identifier
	AgentID string

	// LocalHost is the local bind address
	LocalHost string

	// LocalPort is the local port
	LocalPort int

	// PublicHost overrides the detected public IP (optional)
	PublicHost string

	// PublicPort overrides the local port for external access (optional)
	PublicPort int

	// EndpointType is the type of endpoint being advertised
	EndpointType EndpointType

	// TLSEnabled indicates if TLS is enabled
	TLSEnabled bool

	// AuthRequired indicates if authentication is required
	AuthRequired bool

	// Capabilities lists supported features
	Capabilities []string

	// Metadata contains additional endpoint metadata
	Metadata map[string]string

	// TTL is the advertisement TTL in seconds (default: 30)
	TTL int64

	// AdvertiseInterval is how often to publish advertisements (default: 10s)
	AdvertiseInterval time.Duration

	// DetectPublicIP enables automatic public IP detection (default: true)
	DetectPublicIP bool

	// PublicIPServices is a list of services to query for public IP
	PublicIPServices []string
}

// DefaultEndpointAdvertiserConfig returns default configuration
func DefaultEndpointAdvertiserConfig(agentID string, port int) *EndpointAdvertiserConfig {
	return &EndpointAdvertiserConfig{
		AgentID:           agentID,
		LocalHost:         "0.0.0.0",
		LocalPort:         port,
		EndpointType:      EndpointTypeNATS,
		TTL:               30,
		AdvertiseInterval: 10 * time.Second,
		DetectPublicIP:    true,
		PublicIPServices: []string{
			"https://api.ipify.org",
			"https://ifconfig.me/ip",
			"https://icanhazip.com",
		},
	}
}

// EndpointAdvertiser manages endpoint advertisement
type EndpointAdvertiser struct {
	config *EndpointAdvertiserConfig

	// State
	running        atomic.Bool
	sequenceNumber atomic.Int64
	lastAdvertised atomic.Value // *EndpointAdvertisement
	publicIP       atomic.Value // string
	localAddresses atomic.Value // []string

	// Callbacks
	onAdvertise func(adv *EndpointAdvertisement) error

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewEndpointAdvertiser creates a new endpoint advertiser
func NewEndpointAdvertiser(config *EndpointAdvertiserConfig) (*EndpointAdvertiser, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}
	if config.AgentID == "" {
		return nil, errors.New("agent_id is required")
	}
	if config.LocalPort <= 0 || config.LocalPort > 65535 {
		return nil, errors.New("invalid local port")
	}

	// Set defaults
	if config.TTL <= 0 {
		config.TTL = 30
	}
	if config.AdvertiseInterval <= 0 {
		config.AdvertiseInterval = 10 * time.Second
	}

	return &EndpointAdvertiser{
		config: config,
	}, nil
}

// SetAdvertiseCallback sets the callback for publishing advertisements
func (a *EndpointAdvertiser) SetAdvertiseCallback(cb func(adv *EndpointAdvertisement) error) {
	a.mu.Lock()
	a.onAdvertise = cb
	a.mu.Unlock()
}

// Start starts the endpoint advertiser
func (a *EndpointAdvertiser) Start(ctx context.Context) error {
	if a.running.Load() {
		return errors.New("advertiser already running")
	}

	a.ctx, a.cancel = context.WithCancel(ctx)
	a.running.Store(true)

	// Collect local addresses
	a.updateLocalAddresses()

	// Detect public IP if enabled
	if a.config.DetectPublicIP {
		go a.detectPublicIP()
	}

	// Start advertisement loop
	a.wg.Add(1)
	go a.advertiseLoop()

	return nil
}

// Stop stops the endpoint advertiser
func (a *EndpointAdvertiser) Stop() error {
	if !a.running.Load() {
		return nil
	}

	a.running.Store(false)
	if a.cancel != nil {
		a.cancel()
	}
	a.wg.Wait()

	return nil
}

// Advertise publishes an endpoint advertisement immediately
func (a *EndpointAdvertiser) Advertise() error {
	adv := a.buildAdvertisement()

	a.mu.RLock()
	cb := a.onAdvertise
	a.mu.RUnlock()

	if cb == nil {
		return errors.New("no advertise callback set")
	}

	if err := cb(adv); err != nil {
		return fmt.Errorf("failed to publish advertisement: %w", err)
	}

	a.lastAdvertised.Store(adv)
	return nil
}

// GetLastAdvertisement returns the last published advertisement
func (a *EndpointAdvertiser) GetLastAdvertisement() *EndpointAdvertisement {
	if v := a.lastAdvertised.Load(); v != nil {
		return v.(*EndpointAdvertisement)
	}
	return nil
}

// GetPublicIP returns the detected public IP
func (a *EndpointAdvertiser) GetPublicIP() string {
	if v := a.publicIP.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// GetLocalAddresses returns all local IP addresses
func (a *EndpointAdvertiser) GetLocalAddresses() []string {
	if v := a.localAddresses.Load(); v != nil {
		return v.([]string)
	}
	return nil
}

// IsRunning returns true if the advertiser is running
func (a *EndpointAdvertiser) IsRunning() bool {
	return a.running.Load()
}

// buildAdvertisement creates an endpoint advertisement
func (a *EndpointAdvertiser) buildAdvertisement() *EndpointAdvertisement {
	seqNum := a.sequenceNumber.Add(1)

	host := a.config.LocalHost
	if host == "0.0.0.0" || host == "::" {
		// Use first local address
		if addrs := a.GetLocalAddresses(); len(addrs) > 0 {
			host = addrs[0]
		} else {
			host = "localhost"
		}
	}

	publicHost := a.config.PublicHost
	if publicHost == "" && a.config.DetectPublicIP {
		publicHost = a.GetPublicIP()
	}

	publicPort := a.config.PublicPort
	if publicPort == 0 {
		publicPort = a.config.LocalPort
	}

	endpointType := a.config.EndpointType
	if a.config.TLSEnabled && endpointType == EndpointTypeNATS {
		endpointType = EndpointTypeNATSTLS
	}

	return &EndpointAdvertisement{
		AgentID:        a.config.AgentID,
		EndpointType:   endpointType,
		Host:           host,
		Port:           a.config.LocalPort,
		PublicHost:     publicHost,
		PublicPort:     publicPort,
		LocalAddresses: a.GetLocalAddresses(),
		TLSEnabled:     a.config.TLSEnabled,
		AuthRequired:   a.config.AuthRequired,
		Capabilities:   a.config.Capabilities,
		Metadata:       a.config.Metadata,
		TTL:            a.config.TTL,
		Timestamp:      time.Now().UTC(),
		SequenceNumber: seqNum,
		HealthStatus:   EndpointHealthHealthy,
	}
}

// advertiseLoop periodically publishes advertisements
func (a *EndpointAdvertiser) advertiseLoop() {
	defer a.wg.Done()

	// Initial advertisement
	if err := a.Advertise(); err != nil {
		fmt.Printf("Warning: failed to publish initial advertisement: %v\n", err)
	}

	ticker := time.NewTicker(a.config.AdvertiseInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.Advertise(); err != nil {
				fmt.Printf("Warning: failed to publish advertisement: %v\n", err)
			}
		case <-a.ctx.Done():
			return
		}
	}
}

// detectPublicIP attempts to detect the public IP address
func (a *EndpointAdvertiser) detectPublicIP() {
	for _, service := range a.config.PublicIPServices {
		ip, err := fetchPublicIP(a.ctx, service)
		if err != nil {
			continue
		}
		a.publicIP.Store(ip)
		return
	}
}

// updateLocalAddresses collects all local IP addresses
func (a *EndpointAdvertiser) updateLocalAddresses() {
	addrs, err := getLocalAddresses()
	if err != nil {
		return
	}
	a.localAddresses.Store(addrs)
}

// fetchPublicIP fetches the public IP from a given service
func fetchPublicIP(ctx context.Context, serviceURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serviceURL, http.NoBody)
	if err != nil {
		return "", err
	}

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("service returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}

	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return "", errors.New("invalid IP response")
	}

	return ip, nil
}

// getLocalAddresses returns all non-loopback local IP addresses
func getLocalAddresses() ([]string, error) {
	var addresses []string

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			// Prefer IPv4 for now
			if ip4 := ip.To4(); ip4 != nil {
				addresses = append(addresses, ip4.String())
			}
		}
	}

	return addresses, nil
}

// EndpointRegistry tracks discovered agent endpoints
type EndpointRegistry struct {
	mu        sync.RWMutex
	endpoints map[string]*EndpointAdvertisement // keyed by agent ID
	onChange  func(agentID string, adv *EndpointAdvertisement)
}

// NewEndpointRegistry creates a new endpoint registry
func NewEndpointRegistry() *EndpointRegistry {
	return &EndpointRegistry{
		endpoints: make(map[string]*EndpointAdvertisement),
	}
}

// SetChangeCallback sets a callback for endpoint changes
func (r *EndpointRegistry) SetChangeCallback(cb func(agentID string, adv *EndpointAdvertisement)) {
	r.mu.Lock()
	r.onChange = cb
	r.mu.Unlock()
}

// Register registers or updates an endpoint advertisement
func (r *EndpointRegistry) Register(adv *EndpointAdvertisement) error {
	if err := adv.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	existing := r.endpoints[adv.AgentID]

	// Only update if sequence number is higher
	if existing != nil && adv.SequenceNumber <= existing.SequenceNumber {
		r.mu.Unlock()
		return nil // Stale advertisement
	}

	r.endpoints[adv.AgentID] = adv
	onChange := r.onChange
	r.mu.Unlock()

	if onChange != nil {
		onChange(adv.AgentID, adv)
	}

	return nil
}

// Unregister removes an endpoint
func (r *EndpointRegistry) Unregister(agentID string) {
	r.mu.Lock()
	delete(r.endpoints, agentID)
	onChange := r.onChange
	r.mu.Unlock()

	if onChange != nil {
		onChange(agentID, nil)
	}
}

// Get returns an endpoint by agent ID
func (r *EndpointRegistry) Get(agentID string) *EndpointAdvertisement {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.endpoints[agentID]
}

// GetHealthy returns all healthy endpoints
func (r *EndpointRegistry) GetHealthy() []*EndpointAdvertisement {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var healthy []*EndpointAdvertisement
	for _, adv := range r.endpoints {
		if !adv.IsExpired() && adv.HealthStatus == EndpointHealthHealthy {
			healthy = append(healthy, adv)
		}
	}
	return healthy
}

// GetAll returns all endpoints
func (r *EndpointRegistry) GetAll() []*EndpointAdvertisement {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]*EndpointAdvertisement, 0, len(r.endpoints))
	for _, adv := range r.endpoints {
		all = append(all, adv)
	}
	return all
}

// Count returns the number of registered endpoints
func (r *EndpointRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.endpoints)
}

// CleanExpired removes expired endpoints
func (r *EndpointRegistry) CleanExpired() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := 0
	for agentID, adv := range r.endpoints {
		if adv.IsExpired() {
			delete(r.endpoints, agentID)
			removed++
		}
	}
	return removed
}

// MarshalJSON implements json.Marshaler for EndpointAdvertisement
func (e *EndpointAdvertisement) MarshalJSON() ([]byte, error) {
	type Alias EndpointAdvertisement
	return json.Marshal(&struct {
		*Alias
		TimestampUnix int64 `json:"timestamp_unix"`
	}{
		Alias:         (*Alias)(e),
		TimestampUnix: e.Timestamp.Unix(),
	})
}

// UnmarshalJSON implements json.Unmarshaler for EndpointAdvertisement
func (e *EndpointAdvertisement) UnmarshalJSON(data []byte) error {
	type Alias EndpointAdvertisement
	aux := &struct {
		*Alias
		TimestampUnix int64 `json:"timestamp_unix"`
	}{
		Alias: (*Alias)(e),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if e.Timestamp.IsZero() && aux.TimestampUnix > 0 {
		e.Timestamp = time.Unix(aux.TimestampUnix, 0)
	}

	return nil
}
