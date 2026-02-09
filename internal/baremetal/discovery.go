// Package baremetal provides automated discovery and management of bare metal servers.
package baremetal

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// ServerState represents the current state of a bare metal server.
type ServerState string

const (
	// StateUnknown indicates the server state cannot be determined.
	StateUnknown ServerState = "unknown"
	// StateDiscovering indicates the server is being discovered.
	StateDiscovering ServerState = "discovering"
	// StateAvailable indicates the server is available for provisioning.
	StateAvailable ServerState = "available"
	// StateProvisioning indicates the server is being provisioned.
	StateProvisioning ServerState = "provisioning"
	// StateProvisioned indicates the server has been provisioned.
	StateProvisioned ServerState = "provisioned"
	// StateMaintenance indicates the server is under maintenance.
	StateMaintenance ServerState = "maintenance"
	// StateError indicates the server is in an error state.
	StateError ServerState = "error"
	// StateRetired indicates the server has been retired.
	StateRetired ServerState = "retired"
)

// DiscoveryMethod represents how a server was discovered.
type DiscoveryMethod string

const (
	// MethodIPMI indicates discovery via IPMI/BMC.
	MethodIPMI DiscoveryMethod = "ipmi"
	// MethodRedfish indicates discovery via Redfish API.
	MethodRedfish DiscoveryMethod = "redfish"
	// MethodSSH indicates discovery via SSH.
	MethodSSH DiscoveryMethod = "ssh"
	// MethodAgent indicates discovery via installed agent.
	MethodAgent DiscoveryMethod = "agent"
	// MethodManual indicates manual registration.
	MethodManual DiscoveryMethod = "manual"
	// MethodDHCP indicates discovery via DHCP snooping.
	MethodDHCP DiscoveryMethod = "dhcp"
	// MethodPXE indicates discovery via PXE boot.
	MethodPXE DiscoveryMethod = "pxe"
)

// CPUInfo contains CPU hardware information.
type CPUInfo struct {
	Model        string   `json:"model"`
	Vendor       string   `json:"vendor"`
	Cores        int      `json:"cores"`
	Threads      int      `json:"threads"`
	Sockets      int      `json:"sockets"`
	ClockMHz     float64  `json:"clock_mhz"`
	CacheMB      float64  `json:"cache_mb"`
	Features     []string `json:"features"`
	Architecture string   `json:"architecture"`
}

// MemoryInfo contains memory hardware information.
type MemoryInfo struct {
	TotalMB  int64      `json:"total_mb"`
	UsableMB int64      `json:"usable_mb"`
	Slots    int        `json:"slots"`
	Type     string     `json:"type"` // DDR4, DDR5, etc.
	SpeedMHz int        `json:"speed_mhz"`
	ECC      bool       `json:"ecc"`
	DIMMs    []DIMMInfo `json:"dimms,omitempty"`
}

// DIMMInfo contains information about individual DIMMs.
type DIMMInfo struct {
	Slot         string `json:"slot"`
	SizeMB       int64  `json:"size_mb"`
	Type         string `json:"type"`
	SpeedMHz     int    `json:"speed_mhz"`
	Manufacturer string `json:"manufacturer,omitempty"`
	PartNumber   string `json:"part_number,omitempty"`
}

// StorageDevice represents a storage device.
type StorageDevice struct {
	Name       string     `json:"name"`
	Type       string     `json:"type"` // hdd, ssd, nvme
	SizeMB     int64      `json:"size_mb"`
	Model      string     `json:"model"`
	Serial     string     `json:"serial,omitempty"`
	Rotational bool       `json:"rotational"`
	Transport  string     `json:"transport"` // sata, sas, nvme, scsi
	WWN        string     `json:"wwn,omitempty"`
	SMART      *SMARTInfo `json:"smart,omitempty"`
}

// SMARTInfo contains SMART health information.
type SMARTInfo struct {
	Healthy            bool  `json:"healthy"`
	PowerOnHours       int64 `json:"power_on_hours"`
	Temperature        int   `json:"temperature_celsius"`
	WearPercentage     int   `json:"wear_percentage,omitempty"` // For SSDs
	ReallocatedSectors int   `json:"reallocated_sectors,omitempty"`
}

// StorageInfo contains storage hardware information.
type StorageInfo struct {
	Devices     []StorageDevice     `json:"devices"`
	TotalSizeMB int64               `json:"total_size_mb"`
	Controllers []StorageController `json:"controllers,omitempty"`
}

// StorageController represents a storage controller.
type StorageController struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // raid, hba, nvme
	Model      string `json:"model"`
	CacheSize  int64  `json:"cache_size_mb,omitempty"`
	BBUPresent bool   `json:"bbu_present,omitempty"`
}

// NetworkInterface represents a network interface.
type NetworkInterface struct {
	Name         string   `json:"name"`
	MAC          string   `json:"mac"`
	SpeedMbps    int      `json:"speed_mbps"`
	MTU          int      `json:"mtu"`
	Driver       string   `json:"driver,omitempty"`
	PCI          string   `json:"pci_address,omitempty"`
	IPs          []string `json:"ips,omitempty"`
	State        string   `json:"state"` // up, down, unknown
	Duplex       string   `json:"duplex,omitempty"`
	SRIOVCapable bool     `json:"sriov_capable,omitempty"`
	VFs          int      `json:"vfs,omitempty"` // Number of virtual functions
}

// NetworkInfo contains network hardware information.
type NetworkInfo struct {
	Interfaces     []NetworkInterface `json:"interfaces"`
	Hostname       string             `json:"hostname,omitempty"`
	FQDN           string             `json:"fqdn,omitempty"`
	DefaultGateway string             `json:"default_gateway,omitempty"`
	DNSServers     []string           `json:"dns_servers,omitempty"`
}

// BMCInfo contains Baseboard Management Controller information.
type BMCInfo struct {
	Type       string `json:"type"` // ipmi, redfish, ilo, idrac
	IP         string `json:"ip"`
	MACAddress string `json:"mac_address,omitempty"`
	Firmware   string `json:"firmware_version,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
}

// GPUInfo contains GPU hardware information.
type GPUInfo struct {
	Model             string `json:"model"`
	Vendor            string `json:"vendor"`
	MemoryMB          int64  `json:"memory_mb"`
	PCIAddress        string `json:"pci_address"`
	UUID              string `json:"uuid,omitempty"`
	ComputeCapability string `json:"compute_capability,omitempty"` // For NVIDIA
}

// HardwareInfo contains complete hardware information for a server.
type HardwareInfo struct {
	CPU      CPUInfo           `json:"cpu"`
	Memory   MemoryInfo        `json:"memory"`
	Storage  StorageInfo       `json:"storage"`
	Network  NetworkInfo       `json:"network"`
	BMC      *BMCInfo          `json:"bmc,omitempty"`
	GPUs     []GPUInfo         `json:"gpus,omitempty"`
	Vendor   string            `json:"vendor,omitempty"`
	Model    string            `json:"model,omitempty"`
	Serial   string            `json:"serial,omitempty"`
	UUID     string            `json:"uuid,omitempty"`
	BIOS     string            `json:"bios_version,omitempty"`
	Firmware map[string]string `json:"firmware,omitempty"`
}

// Server represents a discovered bare metal server.
type Server struct {
	ID              string            `json:"id"`
	Name            string            `json:"name,omitempty"`
	State           ServerState       `json:"state"`
	Hardware        HardwareInfo      `json:"hardware"`
	DiscoveredAt    time.Time         `json:"discovered_at"`
	LastSeenAt      time.Time         `json:"last_seen_at"`
	DiscoveryMethod DiscoveryMethod   `json:"discovery_method"`
	Location        *Location         `json:"location,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
	ProvisionedBy   string            `json:"provisioned_by,omitempty"`
	Cluster         string            `json:"cluster,omitempty"`
	Pool            string            `json:"pool,omitempty"`
}

// Location represents the physical location of a server.
type Location struct {
	Datacenter string `json:"datacenter,omitempty"`
	Room       string `json:"room,omitempty"`
	Row        string `json:"row,omitempty"`
	Rack       string `json:"rack,omitempty"`
	Position   int    `json:"position,omitempty"` // U position in rack
	Region     string `json:"region,omitempty"`
	Zone       string `json:"zone,omitempty"`
}

// DiscoveryConfig configures the discovery process.
type DiscoveryConfig struct {
	// Network ranges to scan
	Networks []string `json:"networks"`

	// Discovery methods to use
	Methods []DiscoveryMethod `json:"methods"`

	// Credentials for different methods
	Credentials *DiscoveryCredentials `json:"credentials,omitempty"`

	// Scan interval for continuous discovery
	ScanInterval time.Duration `json:"scan_interval"`

	// Timeout for individual server discovery
	Timeout time.Duration `json:"timeout"`

	// Maximum concurrent discoveries
	Concurrency int `json:"concurrency"`

	// PXE boot configuration
	PXEConfig *PXEConfig `json:"pxe_config,omitempty"`

	// DHCP configuration
	DHCPConfig *DHCPConfig `json:"dhcp_config,omitempty"`

	// Labels to apply to discovered servers
	DefaultLabels map[string]string `json:"default_labels,omitempty"`

	// Location defaults
	DefaultLocation *Location `json:"default_location,omitempty"`
}

// DiscoveryCredentials contains credentials for discovery methods.
type DiscoveryCredentials struct {
	IPMI    *IPMICredentials    `json:"ipmi,omitempty"`
	SSH     *SSHCredentials     `json:"ssh,omitempty"`
	Redfish *RedfishCredentials `json:"redfish,omitempty"`
}

// IPMICredentials contains IPMI authentication credentials.
type IPMICredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// SSHCredentials contains SSH authentication credentials.
type SSHCredentials struct {
	Username       string `json:"username"`
	Password       string `json:"password,omitempty"`
	PrivateKey     string `json:"private_key,omitempty"`
	PrivateKeyPath string `json:"private_key_path,omitempty"`
}

// RedfishCredentials contains Redfish API credentials.
type RedfishCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// PXEConfig configures PXE boot discovery.
type PXEConfig struct {
	Interface  string `json:"interface"`
	TFTPRoot   string `json:"tftp_root"`
	BootImage  string `json:"boot_image"`
	KernelArgs string `json:"kernel_args,omitempty"`
}

// DHCPConfig configures DHCP snooping for discovery.
type DHCPConfig struct {
	Interface string `json:"interface"`
	LeaseFile string `json:"lease_file,omitempty"`
}

// DiscoveryEvent represents an event during discovery.
type DiscoveryEvent struct {
	Type      string    `json:"type"`
	ServerID  string    `json:"server_id,omitempty"`
	IP        string    `json:"ip,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Error     string    `json:"error,omitempty"`
}

// DiscoveryResult contains the result of a discovery operation.
type DiscoveryResult struct {
	ServersFound   int           `json:"servers_found"`
	ServersUpdated int           `json:"servers_updated"`
	ServersLost    int           `json:"servers_lost"`
	Errors         []string      `json:"errors,omitempty"`
	Duration       time.Duration `json:"duration"`
	StartedAt      time.Time     `json:"started_at"`
	CompletedAt    time.Time     `json:"completed_at"`
}

// DiscoveryDriver is the interface for discovery method implementations.
type DiscoveryDriver interface {
	// Method returns the discovery method.
	Method() DiscoveryMethod

	// Discover discovers servers in the given network range.
	Discover(ctx context.Context, network string) ([]*Server, error)

	// Probe probes a specific IP address.
	Probe(ctx context.Context, ip string) (*Server, error)

	// Refresh refreshes hardware information for a known server.
	Refresh(ctx context.Context, server *Server) error
}

// DiscoveryStore is the interface for persisting discovered servers.
type DiscoveryStore interface {
	// Get retrieves a server by ID.
	Get(ctx context.Context, id string) (*Server, error)

	// List lists all servers matching the filter.
	List(ctx context.Context, filter *ServerFilter) ([]*Server, error)

	// Save saves a server.
	Save(ctx context.Context, server *Server) error

	// Delete deletes a server.
	Delete(ctx context.Context, id string) error

	// UpdateState updates a server's state.
	UpdateState(ctx context.Context, id string, state ServerState) error
}

// ServerFilter filters servers when listing.
type ServerFilter struct {
	States       []ServerState     `json:"states,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Pool         string            `json:"pool,omitempty"`
	Cluster      string            `json:"cluster,omitempty"`
	Location     *Location         `json:"location,omitempty"`
	MinCPUCores  int               `json:"min_cpu_cores,omitempty"`
	MinMemoryMB  int64             `json:"min_memory_mb,omitempty"`
	MinStorageMB int64             `json:"min_storage_mb,omitempty"`
	HasGPU       bool              `json:"has_gpu,omitempty"`
}

// Engine is the main discovery engine.
type Engine struct {
	config         *DiscoveryConfig
	store          DiscoveryStore
	drivers        map[DiscoveryMethod]DiscoveryDriver
	listeners      []DiscoveryListener
	profileMatcher *HardwareProfileMatcher
	mu             sync.RWMutex
	stopCh         chan struct{}
	running        bool
}

// DiscoveryListener is called when discovery events occur.
type DiscoveryListener func(event *DiscoveryEvent)

// NewEngine creates a new discovery engine.
func NewEngine(config *DiscoveryConfig, store DiscoveryStore) *Engine {
	if config.Concurrency == 0 {
		config.Concurrency = 10
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.ScanInterval == 0 {
		config.ScanInterval = 5 * time.Minute
	}

	return &Engine{
		config:  config,
		store:   store,
		drivers: make(map[DiscoveryMethod]DiscoveryDriver),
		stopCh:  make(chan struct{}),
	}
}

// RegisterDriver registers a discovery driver.
func (e *Engine) RegisterDriver(driver DiscoveryDriver) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.drivers[driver.Method()] = driver
}

// SetProfileMatcher sets the hardware profile matcher for automatic profile assignment.
func (e *Engine) SetProfileMatcher(matcher *HardwareProfileMatcher) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.profileMatcher = matcher
}

// applyProfile applies hardware profile matching to a server.
func (e *Engine) applyProfile(server *Server) {
	e.mu.RLock()
	matcher := e.profileMatcher
	e.mu.RUnlock()

	if matcher == nil {
		return
	}

	profile := matcher.Match(server)
	if profile == nil {
		return
	}

	// Initialize labels map if needed
	if server.Labels == nil {
		server.Labels = make(map[string]string)
	}

	// Apply profile labels (don't override existing labels)
	for k, v := range profile.Labels {
		if _, exists := server.Labels[k]; !exists {
			server.Labels[k] = v
		}
	}

	// Set hardware-profile label
	server.Labels["hardware-profile"] = profile.Name

	// Set pool if specified and not already set
	if profile.Pool != "" && server.Pool == "" {
		server.Pool = profile.Pool
	}
}

// AddListener adds a discovery event listener.
func (e *Engine) AddListener(listener DiscoveryListener) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners = append(e.listeners, listener)
}

// emit sends an event to all listeners.
func (e *Engine) emit(event *DiscoveryEvent) {
	e.mu.RLock()
	listeners := make([]DiscoveryListener, len(e.listeners))
	copy(listeners, e.listeners)
	e.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// Start starts continuous discovery.
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("discovery engine already running")
	}
	e.running = true
	e.stopCh = make(chan struct{})
	e.mu.Unlock()

	e.emit(&DiscoveryEvent{
		Type:      "engine_started",
		Timestamp: time.Now(),
		Message:   "Discovery engine started",
	})

	// Run initial discovery
	go func() {
		if _, err := e.RunDiscovery(ctx); err != nil {
			e.emit(&DiscoveryEvent{
				Type:      "discovery_error",
				Timestamp: time.Now(),
				Message:   "Initial discovery failed",
				Error:     err.Error(),
			})
		}
	}()

	// Start periodic discovery
	go func() {
		ticker := time.NewTicker(e.config.ScanInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-e.stopCh:
				return
			case <-ticker.C:
				if _, err := e.RunDiscovery(ctx); err != nil {
					e.emit(&DiscoveryEvent{
						Type:      "discovery_error",
						Timestamp: time.Now(),
						Message:   "Periodic discovery failed",
						Error:     err.Error(),
					})
				}
			}
		}
	}()

	return nil
}

// Stop stops the discovery engine.
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}

	close(e.stopCh)
	e.running = false
	e.mu.Unlock()

	e.emit(&DiscoveryEvent{
		Type:      "engine_stopped",
		Timestamp: time.Now(),
		Message:   "Discovery engine stopped",
	})
}

// RunDiscovery runs a single discovery pass.
func (e *Engine) RunDiscovery(ctx context.Context) (*DiscoveryResult, error) {
	startedAt := time.Now()
	result := &DiscoveryResult{
		StartedAt: startedAt,
	}

	e.emit(&DiscoveryEvent{
		Type:      "discovery_started",
		Timestamp: startedAt,
		Message:   fmt.Sprintf("Starting discovery of %d networks", len(e.config.Networks)),
	})

	var allServers []*Server
	var errors []string
	var mu sync.Mutex

	// Create worker pool
	sem := make(chan struct{}, e.config.Concurrency)
	var wg sync.WaitGroup

	for _, network := range e.config.Networks {
		for _, method := range e.config.Methods {
			e.mu.RLock()
			driver, ok := e.drivers[method]
			e.mu.RUnlock()

			if !ok {
				continue
			}

			wg.Add(1)
			sem <- struct{}{}

			go func(net string, drv DiscoveryDriver) {
				defer wg.Done()
				defer func() { <-sem }()

				dctx, cancel := context.WithTimeout(ctx, e.config.Timeout)
				defer cancel()

				servers, err := drv.Discover(dctx, net)

				mu.Lock()
				if err != nil {
					errors = append(errors, fmt.Sprintf("%s discovery of %s: %v", drv.Method(), net, err))
				} else {
					allServers = append(allServers, servers...)
				}
				mu.Unlock()
			}(network, driver)
		}
	}

	wg.Wait()

	// Process discovered servers
	existingServers, err := e.store.List(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing servers: %w", err)
	}

	existingMap := make(map[string]*Server)
	for _, s := range existingServers {
		existingMap[s.ID] = s
	}

	seenIDs := make(map[string]bool)

	for _, server := range allServers {
		seenIDs[server.ID] = true

		// Apply default labels
		if e.config.DefaultLabels != nil {
			if server.Labels == nil {
				server.Labels = make(map[string]string)
			}
			for k, v := range e.config.DefaultLabels {
				if _, ok := server.Labels[k]; !ok {
					server.Labels[k] = v
				}
			}
		}

		// Apply default location
		if server.Location == nil && e.config.DefaultLocation != nil {
			server.Location = e.config.DefaultLocation
		}

		if existing, ok := existingMap[server.ID]; ok {
			// Update existing server
			server.State = existing.State
			server.ProvisionedBy = existing.ProvisionedBy
			server.Cluster = existing.Cluster
			server.Pool = existing.Pool
			result.ServersUpdated++

			e.emit(&DiscoveryEvent{
				Type:      "server_updated",
				ServerID:  server.ID,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("Updated server %s", server.ID),
			})
		} else {
			// New server
			server.State = StateAvailable
			server.DiscoveredAt = time.Now()
			result.ServersFound++

			// Apply hardware profile matching for new servers
			e.applyProfile(server)

			e.emit(&DiscoveryEvent{
				Type:      "server_discovered",
				ServerID:  server.ID,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("Discovered new server %s", server.ID),
			})
		}

		server.LastSeenAt = time.Now()

		if err := e.store.Save(ctx, server); err != nil {
			errors = append(errors, fmt.Sprintf("failed to save server %s: %v", server.ID, err))
		}
	}

	// Mark servers that were not seen
	for id, server := range existingMap {
		if !seenIDs[id] && server.State == StateAvailable {
			result.ServersLost++
			e.emit(&DiscoveryEvent{
				Type:      "server_lost",
				ServerID:  id,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("Server %s no longer reachable", id),
			})
		}
	}

	result.Errors = errors
	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	e.emit(&DiscoveryEvent{
		Type:      "discovery_completed",
		Timestamp: result.CompletedAt,
		Message:   fmt.Sprintf("Discovery completed: %d found, %d updated, %d lost", result.ServersFound, result.ServersUpdated, result.ServersLost),
	})

	return result, nil
}

// DiscoverServer discovers a single server by IP.
func (e *Engine) DiscoverServer(ctx context.Context, ip string) (*Server, error) {
	for _, method := range e.config.Methods {
		e.mu.RLock()
		driver, ok := e.drivers[method]
		e.mu.RUnlock()

		if !ok {
			continue
		}

		dctx, cancel := context.WithTimeout(ctx, e.config.Timeout)
		server, err := driver.Probe(dctx, ip)
		cancel()

		if err == nil && server != nil {
			server.DiscoveryMethod = method
			server.DiscoveredAt = time.Now()
			server.LastSeenAt = time.Now()
			server.State = StateAvailable

			// Apply hardware profile matching
			e.applyProfile(server)

			if err := e.store.Save(ctx, server); err != nil {
				return nil, fmt.Errorf("failed to save server: %w", err)
			}

			e.emit(&DiscoveryEvent{
				Type:      "server_discovered",
				ServerID:  server.ID,
				IP:        ip,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("Discovered server %s at %s", server.ID, ip),
			})

			return server, nil
		}
	}

	return nil, fmt.Errorf("no server found at %s", ip)
}

// RefreshServer refreshes hardware information for a server.
func (e *Engine) RefreshServer(ctx context.Context, serverID string) error {
	server, err := e.store.Get(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}

	e.mu.RLock()
	driver, ok := e.drivers[server.DiscoveryMethod]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no driver for method %s", server.DiscoveryMethod)
	}

	if err := driver.Refresh(ctx, server); err != nil {
		return fmt.Errorf("failed to refresh server: %w", err)
	}

	server.LastSeenAt = time.Now()

	if err := e.store.Save(ctx, server); err != nil {
		return fmt.Errorf("failed to save server: %w", err)
	}

	return nil
}

// GetServer retrieves a server by ID.
func (e *Engine) GetServer(ctx context.Context, id string) (*Server, error) {
	return e.store.Get(ctx, id)
}

// ListServers lists servers matching the filter.
func (e *Engine) ListServers(ctx context.Context, filter *ServerFilter) ([]*Server, error) {
	return e.store.List(ctx, filter)
}

// ListAvailableServers lists servers that are available for provisioning.
func (e *Engine) ListAvailableServers(ctx context.Context) ([]*Server, error) {
	return e.store.List(ctx, &ServerFilter{
		States: []ServerState{StateAvailable},
	})
}

// SetServerState sets the state of a server.
func (e *Engine) SetServerState(ctx context.Context, serverID string, state ServerState) error {
	if err := e.store.UpdateState(ctx, serverID, state); err != nil {
		return err
	}

	e.emit(&DiscoveryEvent{
		Type:      "server_state_changed",
		ServerID:  serverID,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Server %s state changed to %s", serverID, state),
	})

	return nil
}

// AssignToPool assigns a server to a pool.
func (e *Engine) AssignToPool(ctx context.Context, serverID, pool string) error {
	server, err := e.store.Get(ctx, serverID)
	if err != nil {
		return err
	}

	server.Pool = pool
	return e.store.Save(ctx, server)
}

// InMemoryStore is an in-memory implementation of DiscoveryStore.
type InMemoryStore struct {
	servers map[string]*Server
	mu      sync.RWMutex
}

// NewInMemoryStore creates a new in-memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		servers: make(map[string]*Server),
	}
}

// Get retrieves a server by ID.
func (s *InMemoryStore) Get(_ context.Context, id string) (*Server, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	server, ok := s.servers[id]
	if !ok {
		return nil, fmt.Errorf("server not found: %s", id)
	}

	// Return a copy
	data, _ := json.Marshal(server)
	var copied Server
	_ = json.Unmarshal(data, &copied)
	return &copied, nil
}

// List lists servers matching the filter.
func (s *InMemoryStore) List(_ context.Context, filter *ServerFilter) ([]*Server, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Server

	for _, server := range s.servers {
		if filter != nil && !matchesFilter(server, filter) {
			continue
		}

		// Return a copy
		data, _ := json.Marshal(server)
		var copied Server
		_ = json.Unmarshal(data, &copied)
		result = append(result, &copied)
	}

	return result, nil
}

// Save saves a server.
func (s *InMemoryStore) Save(_ context.Context, server *Server) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store a copy
	data, _ := json.Marshal(server)
	var copied Server
	_ = json.Unmarshal(data, &copied)
	s.servers[server.ID] = &copied

	return nil
}

// Delete deletes a server.
func (s *InMemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.servers, id)
	return nil
}

// UpdateState updates a server's state.
func (s *InMemoryStore) UpdateState(_ context.Context, id string, state ServerState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	server, ok := s.servers[id]
	if !ok {
		return fmt.Errorf("server not found: %s", id)
	}

	server.State = state
	return nil
}

// matchesFilter checks if a server matches the filter criteria.
func matchesFilter(server *Server, filter *ServerFilter) bool {
	// Check state
	if len(filter.States) > 0 {
		stateMatch := false
		for _, s := range filter.States {
			if server.State == s {
				stateMatch = true
				break
			}
		}
		if !stateMatch {
			return false
		}
	}

	// Check labels
	for k, v := range filter.Labels {
		if server.Labels[k] != v {
			return false
		}
	}

	// Check pool
	if filter.Pool != "" && server.Pool != filter.Pool {
		return false
	}

	// Check cluster
	if filter.Cluster != "" && server.Cluster != filter.Cluster {
		return false
	}

	// Check minimum CPU cores
	if filter.MinCPUCores > 0 && server.Hardware.CPU.Cores < filter.MinCPUCores {
		return false
	}

	// Check minimum memory
	if filter.MinMemoryMB > 0 && server.Hardware.Memory.TotalMB < filter.MinMemoryMB {
		return false
	}

	// Check minimum storage
	if filter.MinStorageMB > 0 && server.Hardware.Storage.TotalSizeMB < filter.MinStorageMB {
		return false
	}

	// Check GPU
	if filter.HasGPU && len(server.Hardware.GPUs) == 0 {
		return false
	}

	// Check location
	if filter.Location != nil {
		if filter.Location.Datacenter != "" && server.Location != nil && server.Location.Datacenter != filter.Location.Datacenter {
			return false
		}
		if filter.Location.Rack != "" && server.Location != nil && server.Location.Rack != filter.Location.Rack {
			return false
		}
		if filter.Location.Region != "" && server.Location != nil && server.Location.Region != filter.Location.Region {
			return false
		}
	}

	return true
}

// ParseCIDR parses a CIDR string and returns the IP addresses in the range.
func ParseCIDR(cidr string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); incrementIP(ip) {
		ips = append(ips, ip.String())
	}

	// Remove network and broadcast addresses for IPv4
	if len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}

	return ips, nil
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
