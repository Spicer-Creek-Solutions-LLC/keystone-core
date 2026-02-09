// Package vendors provides vendor-specific protocol adapters for network devices.
package vendors

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// VendorType represents the type of vendor device.
type VendorType string

const (
	// VendorCiscoIOS is Cisco IOS.
	VendorCiscoIOS VendorType = "cisco_ios"
	// VendorCiscoNXOS is Cisco NX-OS.
	VendorCiscoNXOS VendorType = "cisco_nxos"
	// VendorJuniperJUNOS is Juniper JUNOS.
	VendorJuniperJUNOS VendorType = "juniper_junos"
	// VendorAristaEOS is Arista EOS.
	VendorAristaEOS VendorType = "arista_eos"
	// VendorPfSense is pfSense.
	VendorPfSense VendorType = "pfsense"
	// VendorOPNsense is OPNsense.
	VendorOPNsense VendorType = "opnsense"
	// VendorVyOS is VyOS.
	VendorVyOS VendorType = "vyos"
	// VendorHPProCurve is HP ProCurve.
	VendorHPProCurve VendorType = "hp_procurve"
	// VendorHPArubaOS is HP ArubaOS (wireless controllers).
	VendorHPArubaOS VendorType = "hp_arubaos"
	// VendorHPAOSCX is Aruba AOS-CX (modern switches).
	VendorHPAOSCX VendorType = "hp_aoscx"
	// VendorDellOS10 is Dell OS10.
	VendorDellOS10 VendorType = "dell_os10"
	// VendorDellOS9 is Dell OS9 / FTOS (legacy Force10).
	VendorDellOS9 VendorType = "dell_os9"
	// VendorDellPowerSwitch is Dell PowerSwitch (N-series).
	VendorDellPowerSwitch VendorType = "dell_powerswitch"
	// VendorFortiOS is Fortinet FortiOS.
	VendorFortiOS VendorType = "fortinet_fortios"
	// VendorPANOS is Palo Alto PAN-OS.
	VendorPANOS VendorType = "paloalto_panos"
	// VendorF5BigIP is F5 BIG-IP.
	VendorF5BigIP VendorType = "f5_bigip"
	// VendorCheckpointGaia is Check Point Gaia.
	VendorCheckpointGaia VendorType = "checkpoint_gaia"
	// VendorMikroTikRouterOS is MikroTik RouterOS.
	VendorMikroTikRouterOS VendorType = "mikrotik_routeros"
	// VendorUbiquitiEdgeOS is Ubiquiti EdgeOS.
	VendorUbiquitiEdgeOS VendorType = "ubiquiti_edgeos"
	// VendorExtremeEXOS is Extreme Networks EXOS.
	VendorExtremeEXOS VendorType = "extreme_exos"
	// VendorNokiaSROS is Nokia SR OS.
	VendorNokiaSROS VendorType = "nokia_sros"
	// VendorHuaweiVRP is Huawei VRP.
	VendorHuaweiVRP VendorType = "huawei_vrp"
	// VendorMellanoxOnyx is Mellanox/NVIDIA Onyx.
	VendorMellanoxOnyx VendorType = "mellanox_onyx"
	// VendorAlliedTelesisAW is Allied Telesis AlliedWare Plus.
	VendorAlliedTelesisAW VendorType = "alliedtelesis_awplus"
	// VendorCienaSAOS is Ciena SAOS.
	VendorCienaSAOS VendorType = "ciena_saos"
)

// VendorAdapter extends ProtocolAdapter with vendor-specific capabilities.
type VendorAdapter interface {
	protocols.ProtocolAdapter

	// Vendor returns the vendor type.
	Vendor() VendorType

	// GetConfig retrieves device configuration.
	// section can be empty for full config, or specify a section like "interface", "routing", etc.
	GetConfig(ctx context.Context, section string) (string, error)

	// SetConfig applies configuration commands.
	SetConfig(ctx context.Context, commands []string) error

	// GetFacts retrieves device facts and metadata.
	GetFacts(ctx context.Context) (*DeviceFacts, error)

	// SaveConfig saves the running configuration to startup/persistent config.
	SaveConfig(ctx context.Context) error
}

// DeviceFacts contains device metadata and system information.
type DeviceFacts struct {
	// Hostname is the device hostname.
	Hostname string `json:"hostname"`
	// FQDN is the fully qualified domain name.
	FQDN string `json:"fqdn,omitempty"`
	// Model is the device model.
	Model string `json:"model"`
	// SerialNumber is the device serial number.
	SerialNumber string `json:"serial_number"`
	// Vendor is the device vendor.
	Vendor string `json:"vendor"`
	// OSType is the operating system type.
	OSType string `json:"os_type"`
	// OSVersion is the operating system version.
	OSVersion string `json:"os_version"`
	// Uptime is the device uptime.
	Uptime time.Duration `json:"uptime"`
	// Interfaces is a list of network interfaces.
	Interfaces []InterfaceFact `json:"interfaces,omitempty"`
	// MemoryTotal is the total memory in bytes.
	MemoryTotal int64 `json:"memory_total,omitempty"`
	// MemoryFree is the free memory in bytes.
	MemoryFree int64 `json:"memory_free,omitempty"`
	// CPUUsage is the CPU usage percentage.
	CPUUsage float64 `json:"cpu_usage,omitempty"`
	// Raw contains the raw output from the device.
	Raw map[string]string `json:"raw,omitempty"`
}

// InterfaceFact contains interface information.
type InterfaceFact struct {
	// Name is the interface name.
	Name string `json:"name"`
	// Description is the interface description.
	Description string `json:"description,omitempty"`
	// MacAddress is the MAC address.
	MacAddress string `json:"mac_address,omitempty"`
	// IPAddresses is a list of IP addresses.
	IPAddresses []string `json:"ip_addresses,omitempty"`
	// Speed is the interface speed in Mbps.
	Speed int `json:"speed,omitempty"`
	// MTU is the maximum transmission unit.
	MTU int `json:"mtu,omitempty"`
	// AdminStatus is the administrative status (up/down).
	AdminStatus string `json:"admin_status"`
	// OperStatus is the operational status (up/down).
	OperStatus string `json:"oper_status"`
	// Duplex is the duplex mode (full/half/auto).
	Duplex string `json:"duplex,omitempty"`
}

// InterfaceConfig contains interface configuration parameters.
type InterfaceConfig struct {
	// Name is the interface name.
	Name string `json:"name"`
	// Description is the interface description.
	Description string `json:"description,omitempty"`
	// IPAddress is the IP address with CIDR notation.
	IPAddress string `json:"ip_address,omitempty"`
	// Enabled specifies if the interface should be enabled.
	Enabled bool `json:"enabled"`
	// Speed is the interface speed in Mbps (0 for auto).
	Speed int `json:"speed,omitempty"`
	// MTU is the maximum transmission unit.
	MTU int `json:"mtu,omitempty"`
	// Duplex is the duplex mode (full/half/auto).
	Duplex string `json:"duplex,omitempty"`
}

// VLANConfig contains VLAN configuration parameters.
type VLANConfig struct {
	// ID is the VLAN ID.
	ID int `json:"id"`
	// Name is the VLAN name.
	Name string `json:"name"`
	// State is the VLAN state (active/suspend).
	State string `json:"state,omitempty"`
}

// RouteConfig contains static route configuration.
type RouteConfig struct {
	// Destination is the destination network in CIDR notation.
	Destination string `json:"destination"`
	// NextHop is the next hop IP address.
	NextHop string `json:"next_hop,omitempty"`
	// Interface is the outgoing interface.
	Interface string `json:"interface,omitempty"`
	// Metric is the route metric/administrative distance.
	Metric int `json:"metric,omitempty"`
	// Description is the route description.
	Description string `json:"description,omitempty"`
}

// ACLConfig contains access control list configuration.
type ACLConfig struct {
	// Name is the ACL name or number.
	Name string `json:"name"`
	// Type is the ACL type (standard/extended).
	Type string `json:"type,omitempty"`
	// Rules is a list of ACL rules.
	Rules []ACLRule `json:"rules"`
}

// ACLRule contains a single ACL rule.
type ACLRule struct {
	// Sequence is the rule sequence number.
	Sequence int `json:"sequence,omitempty"`
	// Action is the rule action (permit/deny).
	Action string `json:"action"`
	// Protocol is the protocol (ip/tcp/udp/icmp/any).
	Protocol string `json:"protocol"`
	// Source is the source address.
	Source string `json:"source"`
	// SourcePort is the source port (for TCP/UDP).
	SourcePort string `json:"source_port,omitempty"`
	// Destination is the destination address.
	Destination string `json:"destination"`
	// DestinationPort is the destination port (for TCP/UDP).
	DestinationPort string `json:"destination_port,omitempty"`
	// Log enables logging for the rule.
	Log bool `json:"log,omitempty"`
}

// CommandResult contains the result of a command execution.
type CommandResult struct {
	// Command is the executed command.
	Command string `json:"command"`
	// Output is the command output.
	Output string `json:"output"`
	// Error is the error message if any.
	Error string `json:"error,omitempty"`
	// ExitCode is the exit code (0 for success).
	ExitCode int `json:"exit_code"`
	// Duration is the command execution duration.
	Duration time.Duration `json:"duration"`
}

// BaseVendorAdapter provides common functionality for vendor adapters.
type BaseVendorAdapter struct {
	// Protocol is the underlying protocol adapter (SSH, REST, etc.).
	Protocol protocols.ProtocolAdapter
	// Device is the proxied device.
	Device *proxy.ProxiedDevice
	// Credential is the device credential.
	Credential credentials.Credential
	// Config contains adapter configuration.
	Config *VendorConfig
	// Connected indicates if the adapter is connected.
	Connected bool
}

// VendorConfig contains common vendor adapter configuration.
type VendorConfig struct {
	// Timeout is the command timeout.
	Timeout time.Duration `json:"timeout,omitempty"`
	// EnablePrompt is the enable mode prompt pattern.
	EnablePrompt string `json:"enable_prompt,omitempty"`
	// ConfigPrompt is the config mode prompt pattern.
	ConfigPrompt string `json:"config_prompt,omitempty"`
	// EnablePassword is the enable mode password.
	EnablePassword string `json:"enable_password,omitempty"`
	// PrivilegeLevel is the required privilege level.
	PrivilegeLevel int `json:"privilege_level,omitempty"`
	// DisablePaging disables output paging.
	DisablePaging bool `json:"disable_paging,omitempty"`
}

// DefaultVendorConfig returns a default vendor configuration.
func DefaultVendorConfig() *VendorConfig {
	return &VendorConfig{
		Timeout:        60 * time.Second,
		EnablePrompt:   "#",
		ConfigPrompt:   "(config",
		DisablePaging:  true,
		PrivilegeLevel: 15,
	}
}

// Type implements ProtocolAdapter.Type.
func (b *BaseVendorAdapter) Type() protocols.ProtocolType {
	if b.Protocol != nil {
		return b.Protocol.Type()
	}
	return protocols.ProtocolType("vendor")
}

// Connect implements ProtocolAdapter.Connect.
func (b *BaseVendorAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	b.Device = device
	b.Credential = cred
	if b.Protocol != nil {
		return b.Protocol.Connect(ctx, device, cred)
	}
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (b *BaseVendorAdapter) Disconnect(ctx context.Context) error {
	b.Connected = false
	if b.Protocol != nil {
		return b.Protocol.Disconnect(ctx)
	}
	return nil
}

// Execute implements ProtocolAdapter.Execute.
func (b *BaseVendorAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	if b.Protocol != nil {
		return b.Protocol.Execute(ctx, req)
	}
	return nil, nil
}

// HealthCheck implements ProtocolAdapter.HealthCheck.
func (b *BaseVendorAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
	if b.Protocol != nil {
		return b.Protocol.HealthCheck(ctx)
	}
	return &protocols.HealthCheckResult{
		Healthy:   b.Connected,
		Status:    "unknown",
		LastCheck: time.Now(),
	}, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (b *BaseVendorAdapter) IsConnected() bool {
	if b.Protocol != nil {
		return b.Protocol.IsConnected()
	}
	return b.Connected
}

// Metrics returns adapter metrics.
// Note: ProtocolAdapter interface does not include Metrics(), so we return
// empty metrics. Vendor-specific adapters can override this method to provide
// their own metrics tracking.
func (b *BaseVendorAdapter) Metrics() *protocols.AdapterMetrics {
	return &protocols.AdapterMetrics{}
}

// SetConnected sets the connected state.
func (b *BaseVendorAdapter) SetConnected(connected bool) {
	b.Connected = connected
}

// SystemInfo contains basic system information from a device.
type SystemInfo struct {
	// Hostname is the device hostname.
	Hostname string `json:"hostname"`
	// Vendor is the device vendor.
	Vendor string `json:"vendor"`
	// Model is the device model.
	Model string `json:"model"`
	// Version is the software version.
	Version string `json:"version"`
	// SerialNumber is the device serial number.
	SerialNumber string `json:"serial_number,omitempty"`
	// Uptime is the device uptime as a string.
	Uptime string `json:"uptime,omitempty"`
}

// DeviceConfig contains device configuration.
type DeviceConfig struct {
	// Running is the running configuration.
	Running string `json:"running"`
	// Startup is the startup configuration.
	Startup string `json:"startup,omitempty"`
	// Timestamp is when the configuration was retrieved.
	Timestamp time.Time `json:"timestamp"`
}

// NewBaseVendorAdapter creates a new base vendor adapter.
func NewBaseVendorAdapter(config *VendorConfig, vendor VendorType) (*BaseVendorAdapter, error) {
	if config == nil {
		config = DefaultVendorConfig()
	}
	return &BaseVendorAdapter{
		Config: config,
	}, nil
}

// VendorAdapterFactoryFunc is a factory function type for creating vendor adapters with protocol adapters.
type VendorAdapterFactoryFunc func(config *VendorConfig, adapter interface{}) (VendorAdapter, error)

// vendorFactoryFuncs stores factory functions that take an adapter parameter.
var vendorFactoryFuncs = make(map[VendorType]VendorAdapterFactoryFunc)

// RegisterVendorFactory registers a vendor adapter factory function.
func RegisterVendorFactory(vendor VendorType, factory VendorAdapterFactoryFunc) {
	vendorFactoryFuncs[vendor] = factory
}

// CreateWithAdapter creates a vendor adapter using a factory function with an adapter parameter.
func CreateWithAdapter(vendor VendorType, config *VendorConfig, adapter interface{}) (VendorAdapter, error) {
	factory, ok := vendorFactoryFuncs[vendor]
	if !ok {
		return nil, &VendorNotFoundError{Vendor: vendor}
	}
	return factory(config, adapter)
}

// VendorRegistry manages vendor adapters.
type VendorRegistry struct {
	factories map[VendorType]VendorAdapterFactory
}

// VendorAdapterFactory creates a vendor adapter.
type VendorAdapterFactory func(config *VendorConfig) (VendorAdapter, error)

// NewVendorRegistry creates a new vendor registry.
func NewVendorRegistry() *VendorRegistry {
	return &VendorRegistry{
		factories: make(map[VendorType]VendorAdapterFactory),
	}
}

// Register registers a vendor adapter factory.
func (r *VendorRegistry) Register(vendor VendorType, factory VendorAdapterFactory) {
	r.factories[vendor] = factory
}

// Create creates a vendor adapter.
func (r *VendorRegistry) Create(vendor VendorType, config *VendorConfig) (VendorAdapter, error) {
	factory, ok := r.factories[vendor]
	if !ok {
		return nil, &VendorNotFoundError{Vendor: vendor}
	}
	return factory(config)
}

// ListVendors returns all registered vendors.
func (r *VendorRegistry) ListVendors() []VendorType {
	vendors := make([]VendorType, 0, len(r.factories))
	for v := range r.factories {
		vendors = append(vendors, v)
	}
	return vendors
}

// VendorNotFoundError is returned when a vendor is not found.
type VendorNotFoundError struct {
	Vendor VendorType
}

func (e *VendorNotFoundError) Error() string {
	return "vendor not found: " + string(e.Vendor)
}

// DefaultRegistry is the default vendor registry.
var DefaultRegistry = NewVendorRegistry()

// Register registers a vendor adapter factory with the default registry.
func Register(vendor VendorType, factory VendorAdapterFactory) {
	DefaultRegistry.Register(vendor, factory)
}

// Create creates a vendor adapter from the default registry.
func Create(vendor VendorType, config *VendorConfig) (VendorAdapter, error) {
	return DefaultRegistry.Create(vendor, config)
}
