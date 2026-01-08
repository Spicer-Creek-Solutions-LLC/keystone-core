// Package snmp provides an SNMP protocol adapter for proxy agents.
package snmp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/shawnbutts/keystone-core/pkg/credentials"
	"github.com/shawnbutts/keystone-core/pkg/protocols"
	"github.com/shawnbutts/keystone-core/pkg/proxy"
)

// V3Adapter extends Adapter with SNMPv3-specific operations.
type V3Adapter struct {
	*Adapter
	engineID       string
	engineBoots    int32
	engineTime     int32
	contextName    string
	contextEngineID string
}

// V3Config extends Config with SNMPv3-specific settings.
type V3Config struct {
	*Config

	// ContextName is the SNMPv3 context name.
	ContextName string `json:"context_name,omitempty"`

	// ContextEngineID is the SNMPv3 context engine ID.
	ContextEngineID string `json:"context_engine_id,omitempty"`
}

// DefaultV3Config returns a default SNMPv3 configuration.
func DefaultV3Config() *V3Config {
	return &V3Config{
		Config: &Config{
			ConnectionConfig: protocols.DefaultConnectionConfig(),
			Port:             161,
			Version:          SNMPv3,
			Retries:          3,
			MaxOids:          60,
			MaxRepetitions:   10,
		},
	}
}

// NewV3Adapter creates a new SNMPv3 adapter.
func NewV3Adapter(config *V3Config) *V3Adapter {
	if config == nil {
		config = DefaultV3Config()
	}
	config.Version = SNMPv3
	return &V3Adapter{
		Adapter:     NewAdapter(config.Config),
		contextName: config.ContextName,
		contextEngineID: config.ContextEngineID,
	}
}

// Connect establishes an SNMPv3 connection with USM security.
func (a *V3Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Disconnect existing connection
	if a.client != nil {
		_ = a.client.Conn.Close()
		a.client = nil
		a.connected = false
	}

	a.device = device
	a.credential = cred

	// Validate credential type
	v3cred, ok := cred.(*credentials.SNMPv3Credential)
	if !ok {
		return fmt.Errorf("SNMPv3 requires SNMPv3Credential, got %T", cred)
	}

	// Determine port
	port := a.config.Port
	if device.Port > 0 {
		port = device.Port
	}

	// Create SNMP client
	client := &gosnmp.GoSNMP{
		Target:         device.Address,
		Port:           uint16(port),
		Version:        gosnmp.Version3,
		Timeout:        a.config.Timeout,
		Retries:        a.config.Retries,
		MaxOids:        a.config.MaxOids,
		MaxRepetitions: a.config.MaxRepetitions,
		SecurityModel:  gosnmp.UserSecurityModel,
		MsgFlags:       a.getMsgFlags(v3cred),
		SecurityParameters: &gosnmp.UsmSecurityParameters{
			UserName:                 v3cred.Username,
			AuthenticationProtocol:   a.getAuthProtocol(string(v3cred.AuthProtocol)),
			AuthenticationPassphrase: v3cred.AuthPassword,
			PrivacyProtocol:          a.getPrivProtocol(string(v3cred.PrivacyProtocol)),
			PrivacyPassphrase:        v3cred.PrivacyPassword,
		},
	}

	// Set context if specified
	if a.contextName != "" {
		client.ContextName = a.contextName
	}
	if a.contextEngineID != "" {
		client.ContextEngineID = a.contextEngineID
	}

	// Connect
	if err := client.Connect(); err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("SNMPv3 connection failed: %w", err)
	}

	a.client = client
	a.connected = true
	a.lastConnect = time.Now()
	a.lastError = nil
	a.metrics.ConnectionCount++
	a.metrics.LastActivity = time.Now()

	// Store engine information if available
	if usp, ok := client.SecurityParameters.(*gosnmp.UsmSecurityParameters); ok {
		a.engineID = string(usp.AuthoritativeEngineID)
		a.engineBoots = int32(usp.AuthoritativeEngineBoots)
		a.engineTime = int32(usp.AuthoritativeEngineTime)
	}

	return nil
}

// SecurityLevel represents the SNMPv3 security level.
type SecurityLevel int

const (
	// NoAuthNoPriv - No authentication, no privacy.
	NoAuthNoPriv SecurityLevel = iota
	// AuthNoPriv - Authentication, no privacy.
	AuthNoPriv
	// AuthPriv - Authentication and privacy.
	AuthPriv
)

// String returns the string representation.
func (s SecurityLevel) String() string {
	switch s {
	case NoAuthNoPriv:
		return "noAuthNoPriv"
	case AuthNoPriv:
		return "authNoPriv"
	case AuthPriv:
		return "authPriv"
	default:
		return "unknown"
	}
}

// GetSecurityLevel returns the current security level.
func (a *V3Adapter) GetSecurityLevel() SecurityLevel {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.client == nil {
		return NoAuthNoPriv
	}

	switch a.client.MsgFlags {
	case gosnmp.NoAuthNoPriv:
		return NoAuthNoPriv
	case gosnmp.AuthNoPriv:
		return AuthNoPriv
	case gosnmp.AuthPriv:
		return AuthPriv
	default:
		return NoAuthNoPriv
	}
}

// GetEngineInfo returns the SNMP engine information.
func (a *V3Adapter) GetEngineInfo() EngineInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return EngineInfo{
		EngineID:    a.engineID,
		EngineBoots: a.engineBoots,
		EngineTime:  a.engineTime,
	}
}

// EngineInfo contains SNMP engine information.
type EngineInfo struct {
	EngineID    string
	EngineBoots int32
	EngineTime  int32
}

// DiscoverEngineID performs SNMPv3 engine discovery.
func (a *V3Adapter) DiscoverEngineID(ctx context.Context) (string, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return "", fmt.Errorf("not connected")
	}

	// Engine discovery is typically done automatically during connection
	// Return the cached engine ID
	return a.engineID, nil
}

// USMUser represents an SNMPv3 USM user.
type USMUser struct {
	Name           string
	AuthProtocol   string
	AuthPassphrase string
	PrivProtocol   string
	PrivPassphrase string
	EngineID       string
}

// USMUserManager manages USM users.
type USMUserManager struct {
	users map[string]*USMUser
	mu    sync.RWMutex
}

// NewUSMUserManager creates a new USM user manager.
func NewUSMUserManager() *USMUserManager {
	return &USMUserManager{
		users: make(map[string]*USMUser),
	}
}

// AddUser adds a USM user.
func (m *USMUserManager) AddUser(user *USMUser) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user.Name] = user
}

// GetUser gets a USM user.
func (m *USMUserManager) GetUser(name string) *USMUser {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.users[name]
}

// RemoveUser removes a USM user.
func (m *USMUserManager) RemoveUser(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.users, name)
}

// ListUsers lists all USM users.
func (m *USMUserManager) ListUsers() []*USMUser {
	m.mu.RLock()
	defer m.mu.RUnlock()
	users := make([]*USMUser, 0, len(m.users))
	for _, user := range m.users {
		users = append(users, user)
	}
	return users
}

// V3TrapReceiver handles SNMPv3 traps and informs.
type V3TrapReceiver struct {
	listener    *gosnmp.TrapListener
	handlers    []TrapHandler
	mu          sync.RWMutex
	running     bool
	userManager *USMUserManager
}

// NewV3TrapReceiver creates a new SNMPv3 trap receiver.
func NewV3TrapReceiver(userManager *USMUserManager) *V3TrapReceiver {
	return &V3TrapReceiver{
		handlers:    make([]TrapHandler, 0),
		userManager: userManager,
	}
}

// AddHandler adds a trap handler.
func (r *V3TrapReceiver) AddHandler(handler TrapHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers = append(r.handlers, handler)
}

// Start starts the SNMPv3 trap receiver.
func (r *V3TrapReceiver) Start(addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("trap receiver already running")
	}

	listener := gosnmp.NewTrapListener()
	listener.Params = gosnmp.Default
	listener.Params.Version = gosnmp.Version3
	listener.Params.SecurityModel = gosnmp.UserSecurityModel
	listener.Params.MsgFlags = gosnmp.AuthPriv

	// Note: In a real implementation, you'd need to set up USM table
	// This is a simplified version

	listener.OnNewTrap = func(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
		r.handleTrap(packet, addr)
	}

	go func() {
		_ = listener.Listen(addr)
	}()

	r.listener = listener
	r.running = true
	return nil
}

// Stop stops the trap receiver.
func (r *V3TrapReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	r.listener.Close()
	r.running = false
	return nil
}

// handleTrap handles an incoming trap.
func (r *V3TrapReceiver) handleTrap(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
	trap := &Trap{
		Source:     addr.String(),
		Variables:  convertVariables(packet.Variables),
		ReceivedAt: time.Now(),
	}

	// Extract trap-specific fields
	for _, v := range packet.Variables {
		switch v.Name {
		case ".1.3.6.1.6.3.1.1.4.1.0": // snmpTrapOID
			if oid, ok := v.Value.(string); ok {
				trap.Enterprise = oid
			}
		case ".1.3.6.1.2.1.1.3.0": // sysUpTime
			if ticks, ok := v.Value.(uint32); ok {
				trap.Timestamp = ticks
			}
		}
	}

	r.mu.RLock()
	handlers := make([]TrapHandler, len(r.handlers))
	copy(handlers, r.handlers)
	r.mu.RUnlock()

	for _, handler := range handlers {
		handler(trap)
	}
}

// AuthProtocols lists supported authentication protocols.
var AuthProtocols = []string{
	"MD5",
	"SHA",
	"SHA224",
	"SHA256",
	"SHA384",
	"SHA512",
}

// PrivProtocols lists supported privacy protocols.
var PrivProtocols = []string{
	"DES",
	"AES",
	"AES192",
	"AES256",
	"AES192C",
	"AES256C",
}

// ValidateAuthProtocol validates an authentication protocol.
func ValidateAuthProtocol(proto string) bool {
	for _, p := range AuthProtocols {
		if p == proto {
			return true
		}
	}
	return proto == "" // empty is valid (noAuth)
}

// ValidatePrivProtocol validates a privacy protocol.
func ValidatePrivProtocol(proto string) bool {
	for _, p := range PrivProtocols {
		if p == proto {
			return true
		}
	}
	return proto == "" // empty is valid (noPriv)
}

// GetBulk performs an SNMP GETBULK operation.
func (a *V3Adapter) GetBulk(ctx context.Context, oids []string, nonRepeaters uint8, maxRepetitions uint32) ([]SNMPVariable, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	result, err := client.GetBulk(oids, nonRepeaters, maxRepetitions)
	if err != nil {
		return nil, err
	}

	return convertVariables(result.Variables), nil
}

// GetTable retrieves an SNMP table (inherited from V2cAdapter).
func (a *V3Adapter) GetTable(ctx context.Context, baseOID string, columns map[string]string) (*Table, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	table := &Table{
		BaseOID: baseOID,
		Columns: columns,
		Rows:    make([]TableRow, 0),
	}

	rowData := make(map[string]map[string]interface{})

	for colName, colOID := range columns {
		fullOID := baseOID + "." + colOID
		err := client.BulkWalk(fullOID, func(pdu gosnmp.SnmpPDU) error {
			index := extractIndex(fullOID, pdu.Name)
			if index == "" {
				return nil
			}

			if _, ok := rowData[index]; !ok {
				rowData[index] = make(map[string]interface{})
			}
			rowData[index][colName] = formatValue(pdu)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk column %s: %w", colName, err)
		}
	}

	for index, values := range rowData {
		table.Rows = append(table.Rows, TableRow{
			Index:  index,
			Values: values,
		})
	}

	return table, nil
}

// GetSystemInfo retrieves common system information.
func (a *V3Adapter) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	oids := []string{
		CommonOIDs.SysDescr,
		CommonOIDs.SysObjectID,
		CommonOIDs.SysUpTime,
		CommonOIDs.SysContact,
		CommonOIDs.SysName,
		CommonOIDs.SysLocation,
	}

	vars, err := a.Get(ctx, oids)
	if err != nil {
		return nil, err
	}

	info := &SystemInfo{}
	for _, v := range vars {
		switch v.OID {
		case CommonOIDs.SysDescr:
			if val, ok := v.Value.([]byte); ok {
				info.Description = string(val)
			}
		case CommonOIDs.SysObjectID:
			if val, ok := v.Value.(string); ok {
				info.ObjectID = val
			}
		case CommonOIDs.SysUpTime:
			if val, ok := v.Value.(uint32); ok {
				info.UpTime = time.Duration(val) * 10 * time.Millisecond
			}
		case CommonOIDs.SysContact:
			if val, ok := v.Value.([]byte); ok {
				info.Contact = string(val)
			}
		case CommonOIDs.SysName:
			if val, ok := v.Value.([]byte); ok {
				info.Name = string(val)
			}
		case CommonOIDs.SysLocation:
			if val, ok := v.Value.([]byte); ok {
				info.Location = string(val)
			}
		}
	}

	return info, nil
}

// NewV3AdapterFactory creates an adapter factory for SNMPv3.
func NewV3AdapterFactory(config *V3Config) protocols.AdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.ProtocolAdapter, error) {
		cfg := config
		if cfg == nil {
			cfg = DefaultV3Config()
		}
		cfg.ConnectionConfig = connConfig
		return NewV3Adapter(cfg), nil
	}
}
