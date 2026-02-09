// Package discovery provides network discovery for proxied devices.
package discovery

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// Discovery provides device discovery capabilities.
type Discovery struct {
	// config holds discovery configuration
	config Config

	// scanners are the registered discovery scanners
	scanners map[string]Scanner

	// matchers match discovered devices to profiles
	matchers []ProfileMatcher

	// eventEmitter emits discovery events
	eventEmitter EventEmitter

	// mu protects internal state
	mu sync.RWMutex

	// discovered holds discovered devices pending approval
	discovered map[string]*DiscoveredDevice

	// approved holds approved devices
	approved map[string]*DiscoveredDevice

	// stopCh is used to stop background scanning
	stopCh chan struct{}

	// running indicates if background scanning is running
	running bool
}

// Config configures device discovery.
type Config struct {
	// ScanInterval is how often to scan for new devices
	ScanInterval time.Duration

	// ScanTimeout is the timeout for scan operations
	ScanTimeout time.Duration

	// MaxConcurrent is the maximum concurrent scan operations
	MaxConcurrent int

	// AutoApprove automatically approves matching devices
	AutoApprove bool

	// AutoApproveProfiles are profiles to auto-approve
	AutoApproveProfiles []string

	// Networks are the networks to scan
	Networks []string

	// ExcludeNetworks are networks to exclude
	ExcludeNetworks []string

	// ExcludeHosts are specific hosts to exclude
	ExcludeHosts []string

	// SNMPCommunity is the SNMP community for discovery
	SNMPCommunity string

	// SSHPort is the SSH port to probe
	SSHPort int

	// SNMPPort is the SNMP port to probe
	SNMPPort int
}

// DefaultConfig returns default discovery configuration.
func DefaultConfig() Config {
	return Config{
		ScanInterval:  1 * time.Hour,
		ScanTimeout:   30 * time.Second,
		MaxConcurrent: 50,
		AutoApprove:   false,
		SSHPort:       22,
		SNMPPort:      161,
		SNMPCommunity: "public",
	}
}

// Scanner is the interface for discovery scanners.
type Scanner interface {
	// Name returns the scanner name
	Name() string

	// Scan performs a scan and returns discovered devices
	Scan(ctx context.Context, targets []string) ([]*DiscoveredDevice, error)
}

// ProfileMatcher matches discovered devices to profiles.
type ProfileMatcher interface {
	// Match returns the profile name for a device, or empty if no match
	Match(device *DiscoveredDevice) string
}

// EventEmitter emits discovery events.
type EventEmitter interface {
	Emit(event Event)
}

// Event represents a discovery event.
type Event struct {
	Type      EventType
	DeviceID  string
	Device    *DiscoveredDevice
	Timestamp time.Time
	Error     error
}

// EventType represents the type of discovery event.
type EventType string

// EventDeviceDiscovered constants define the events.
const (
	EventDeviceDiscovered EventType = "device.discovered"
	EventDeviceApproved   EventType = "device.approved"
	EventDeviceRejected   EventType = "device.rejected"
	EventScanStarted      EventType = "scan.started"
	EventScanCompleted    EventType = "scan.completed"
	EventScanFailed       EventType = "scan.failed"
)

// DiscoveredDevice represents a discovered device.
type DiscoveredDevice struct {
	// ID is a unique identifier for the discovered device
	ID string `json:"id"`

	// Address is the network address
	Address string `json:"address"`

	// Hostname is the hostname (if discovered)
	Hostname string `json:"hostname,omitempty"`

	// Port is the primary port
	Port int `json:"port"`

	// Protocol is the discovered protocol
	Protocol Protocol `json:"protocol"`

	// Type is the device type
	Type DeviceType `json:"type,omitempty"`

	// Vendor is the device vendor
	Vendor string `json:"vendor,omitempty"`

	// Model is the device model
	Model string `json:"model,omitempty"`

	// Version is the firmware/software version
	Version string `json:"version,omitempty"`

	// SysDescr is the SNMP sysDescr
	SysDescr string `json:"sys_descr,omitempty"`

	// Banner is the SSH/protocol banner
	Banner string `json:"banner,omitempty"`

	// MACAddress is the MAC address (if discovered)
	MACAddress string `json:"mac_address,omitempty"`

	// Neighbors are LLDP/CDP neighbors
	Neighbors []Neighbor `json:"neighbors,omitempty"`

	// MatchedProfile is the auto-matched profile
	MatchedProfile string `json:"matched_profile,omitempty"`

	// SuggestedCredentials are suggested credential types
	SuggestedCredentials []string `json:"suggested_credentials,omitempty"`

	// DiscoveryTime is when the device was discovered
	DiscoveryTime time.Time `json:"discovery_time"`

	// LastSeen is when the device was last seen
	LastSeen time.Time `json:"last_seen"`

	// Status is the device status
	Status Status `json:"status"`

	// Metadata contains additional discovery metadata
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Protocol is the protocol used for discovery.
type Protocol string

// ProtocolSSH and related constants.
const (
	ProtocolSSH   Protocol = "ssh"
	ProtocolSNMP  Protocol = "snmp"
	ProtocolHTTP  Protocol = "http"
	ProtocolHTTPS Protocol = "https"
	ProtocolWinRM Protocol = "winrm"
	ProtocolICMP  Protocol = "icmp"
)

// DeviceType is the type of discovered device.
type DeviceType string

// DeviceType constants define the supported types.
const (
	DeviceTypeUnknown        DeviceType = "unknown"
	DeviceTypeRouter         DeviceType = "router"
	DeviceTypeSwitch         DeviceType = "switch"
	DeviceTypeFirewall       DeviceType = "firewall"
	DeviceTypeServer         DeviceType = "server"
	DeviceTypeWorkstation    DeviceType = "workstation"
	DeviceTypeAccessPoint    DeviceType = "access_point"
	DeviceTypeLoadBalancer   DeviceType = "load_balancer"
	DeviceTypePrinter        DeviceType = "printer"
	DeviceTypeStorage        DeviceType = "storage"
	DeviceTypeVirtualMachine DeviceType = "vm"
	DeviceTypeContainer      DeviceType = "container"
)

// Status is the status of a discovered device.
type Status string

// StatusPending constants define the possible statuses.
const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusIgnored  Status = "ignored"
)

// Neighbor represents an LLDP/CDP neighbor.
type Neighbor struct {
	LocalPort    string `json:"local_port"`
	RemoteDevice string `json:"remote_device"`
	RemotePort   string `json:"remote_port"`
	Protocol     string `json:"protocol"` // lldp or cdp
}

// NewDiscovery creates a new discovery service.
func NewDiscovery(config Config) *Discovery {
	return &Discovery{
		config:     config,
		scanners:   make(map[string]Scanner),
		matchers:   make([]ProfileMatcher, 0),
		discovered: make(map[string]*DiscoveredDevice),
		approved:   make(map[string]*DiscoveredDevice),
		stopCh:     make(chan struct{}),
	}
}

// RegisterScanner registers a discovery scanner.
func (d *Discovery) RegisterScanner(scanner Scanner) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.scanners[scanner.Name()] = scanner
}

// RegisterMatcher registers a profile matcher.
func (d *Discovery) RegisterMatcher(matcher ProfileMatcher) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.matchers = append(d.matchers, matcher)
}

// SetEventEmitter sets the event emitter.
func (d *Discovery) SetEventEmitter(emitter EventEmitter) {
	d.eventEmitter = emitter
}

// Scan performs a discovery scan.
func (d *Discovery) Scan(ctx context.Context) (*ScanResult, error) {
	startTime := time.Now()

	result := &ScanResult{
		StartTime: startTime,
		Networks:  d.config.Networks,
	}

	// Emit scan started event
	if d.eventEmitter != nil {
		d.eventEmitter.Emit(Event{
			Type:      EventScanStarted,
			Timestamp: startTime,
		})
	}

	// Get targets from networks
	targets, err := d.expandNetworks(d.config.Networks)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	// Filter excluded hosts
	targets = d.filterExcluded(targets)
	result.TotalTargets = len(targets)

	// Run all scanners
	var allDevices []*DiscoveredDevice
	devicesMu := sync.Mutex{}

	for _, scanner := range d.scanners {
		devices, err := scanner.Scan(ctx, targets)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", scanner.Name(), err))
			continue
		}

		devicesMu.Lock()
		allDevices = append(allDevices, devices...)
		devicesMu.Unlock()
	}

	// Process discovered devices
	for _, device := range allDevices {
		// Match to profile
		for _, matcher := range d.matchers {
			if profile := matcher.Match(device); profile != "" {
				device.MatchedProfile = profile
				break
			}
		}

		// Add to discovered
		d.mu.Lock()
		if existing, ok := d.discovered[device.ID]; ok {
			// Update existing device
			existing.LastSeen = time.Now()
			existing.MatchedProfile = device.MatchedProfile
		} else {
			device.Status = StatusPending
			d.discovered[device.ID] = device
			result.NewDevices++

			// Emit discovery event
			if d.eventEmitter != nil {
				d.eventEmitter.Emit(Event{
					Type:      EventDeviceDiscovered,
					DeviceID:  device.ID,
					Device:    device,
					Timestamp: time.Now(),
				})
			}

			// Auto-approve if enabled
			if d.config.AutoApprove && device.MatchedProfile != "" {
				for _, profile := range d.config.AutoApproveProfiles {
					if device.MatchedProfile == profile {
						device.Status = StatusApproved
						d.approved[device.ID] = device
						result.AutoApproved++
						break
					}
				}
			}
		}
		d.mu.Unlock()
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	result.TotalDiscovered = len(allDevices)

	// Emit scan completed event
	if d.eventEmitter != nil {
		d.eventEmitter.Emit(Event{
			Type:      EventScanCompleted,
			Timestamp: result.EndTime,
		})
	}

	return result, nil
}

// ScanResult is the result of a discovery scan.
type ScanResult struct {
	StartTime       time.Time     `json:"start_time"`
	EndTime         time.Time     `json:"end_time"`
	Duration        time.Duration `json:"duration"`
	Networks        []string      `json:"networks"`
	TotalTargets    int           `json:"total_targets"`
	TotalDiscovered int           `json:"total_discovered"`
	NewDevices      int           `json:"new_devices"`
	AutoApproved    int           `json:"auto_approved"`
	Errors          []string      `json:"errors,omitempty"`
	Error           string        `json:"error,omitempty"`
}

// expandNetworks expands network CIDRs to individual IPs.
func (d *Discovery) expandNetworks(networks []string) ([]string, error) {
	var targets []string

	for _, network := range networks {
		_, ipnet, err := net.ParseCIDR(network)
		if err != nil {
			// Try as single IP
			if ip := net.ParseIP(network); ip != nil {
				targets = append(targets, network)
				continue
			}
			return nil, fmt.Errorf("invalid network: %s", network)
		}

		// Expand CIDR
		for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); incrementIP(ip) {
			targets = append(targets, ip.String())
		}
	}

	return targets, nil
}

// incrementIP increments an IP address.
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// filterExcluded removes excluded hosts from targets.
func (d *Discovery) filterExcluded(targets []string) []string {
	excludeSet := make(map[string]bool)
	for _, host := range d.config.ExcludeHosts {
		excludeSet[host] = true
	}

	// Expand excluded networks
	for _, network := range d.config.ExcludeNetworks {
		expanded, _ := d.expandNetworks([]string{network})
		for _, ip := range expanded {
			excludeSet[ip] = true
		}
	}

	var filtered []string
	for _, target := range targets {
		if !excludeSet[target] {
			filtered = append(filtered, target)
		}
	}

	return filtered
}

// GetDiscovered returns all discovered devices.
func (d *Discovery) GetDiscovered() []*DiscoveredDevice {
	d.mu.RLock()
	defer d.mu.RUnlock()

	devices := make([]*DiscoveredDevice, 0, len(d.discovered))
	for _, device := range d.discovered {
		devices = append(devices, device)
	}
	return devices
}

// GetPending returns pending (unapproved) devices.
func (d *Discovery) GetPending() []*DiscoveredDevice {
	d.mu.RLock()
	defer d.mu.RUnlock()

	devices := make([]*DiscoveredDevice, 0)
	for _, device := range d.discovered {
		if device.Status == StatusPending {
			devices = append(devices, device)
		}
	}
	return devices
}

// GetApproved returns approved devices.
func (d *Discovery) GetApproved() []*DiscoveredDevice {
	d.mu.RLock()
	defer d.mu.RUnlock()

	devices := make([]*DiscoveredDevice, 0, len(d.approved))
	for _, device := range d.approved {
		devices = append(devices, device)
	}
	return devices
}

// Approve approves a discovered device.
func (d *Discovery) Approve(deviceID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	device, ok := d.discovered[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	device.Status = StatusApproved
	d.approved[deviceID] = device

	if d.eventEmitter != nil {
		d.eventEmitter.Emit(Event{
			Type:      EventDeviceApproved,
			DeviceID:  deviceID,
			Device:    device,
			Timestamp: time.Now(),
		})
	}

	return nil
}

// Reject rejects a discovered device.
func (d *Discovery) Reject(deviceID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	device, ok := d.discovered[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	device.Status = StatusRejected
	delete(d.approved, deviceID)

	if d.eventEmitter != nil {
		d.eventEmitter.Emit(Event{
			Type:      EventDeviceRejected,
			DeviceID:  deviceID,
			Device:    device,
			Timestamp: time.Now(),
		})
	}

	return nil
}

// Ignore ignores a discovered device.
func (d *Discovery) Ignore(deviceID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	device, ok := d.discovered[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	device.Status = StatusIgnored
	delete(d.approved, deviceID)

	return nil
}

// Remove removes a device from discovery.
func (d *Discovery) Remove(deviceID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.discovered, deviceID)
	delete(d.approved, deviceID)

	return nil
}

// StartBackgroundScanning starts periodic scanning.
func (d *Discovery) StartBackgroundScanning(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("background scanning already running")
	}
	d.running = true
	d.mu.Unlock()

	go func() {
		ticker := time.NewTicker(d.config.ScanInterval)
		defer ticker.Stop()

		// Do initial scan
		d.Scan(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-d.stopCh:
				return
			case <-ticker.C:
				d.Scan(ctx)
			}
		}
	}()

	return nil
}

// StopBackgroundScanning stops periodic scanning.
func (d *Discovery) StopBackgroundScanning() {
	d.mu.Lock()
	if d.running {
		close(d.stopCh)
		d.running = false
		d.stopCh = make(chan struct{})
	}
	d.mu.Unlock()
}
