// Package snmp provides an SNMP protocol adapter for proxy agents.
package snmp

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/shawnbutts/keystone-core/pkg/credentials"
	"github.com/shawnbutts/keystone-core/pkg/protocols"
	"github.com/shawnbutts/keystone-core/pkg/proxy"
)

var (
	// Sync.Once to ensure deprecation warnings are only logged once per algorithm
	md5DeprecationOnce sync.Once
	desDeprecationOnce sync.Once
)

// SNMPVersion represents the SNMP protocol version.
type SNMPVersion int

const (
	// SNMPv1 is SNMP version 1.
	SNMPv1 SNMPVersion = iota
	// SNMPv2c is SNMP version 2c.
	SNMPv2c
	// SNMPv3 is SNMP version 3.
	SNMPv3
)

// String returns the string representation of the SNMP version.
func (v SNMPVersion) String() string {
	switch v {
	case SNMPv1:
		return "1"
	case SNMPv2c:
		return "2c"
	case SNMPv3:
		return "3"
	default:
		return "unknown"
	}
}

// Config contains SNMP adapter configuration.
type Config struct {
	// ConnectionConfig contains common connection settings.
	*protocols.ConnectionConfig

	// Port is the SNMP port (default 161).
	Port int `json:"port,omitempty"`

	// Version is the SNMP version (default v2c).
	Version SNMPVersion `json:"version,omitempty"`

	// Retries is the number of retries for SNMP operations.
	Retries int `json:"retries,omitempty"`

	// MaxOids is the maximum number of OIDs per request.
	MaxOids int `json:"max_oids,omitempty"`

	// MaxRepetitions is the max-repetitions for GETBULK.
	MaxRepetitions uint32 `json:"max_repetitions,omitempty"`
}

// DefaultConfig returns a default SNMP configuration.
func DefaultConfig() *Config {
	return &Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		Port:             161,
		Version:          SNMPv2c,
		Retries:          3,
		MaxOids:          60,
		MaxRepetitions:   10,
	}
}

// Adapter implements the SNMP protocol adapter.
type Adapter struct {
	config     *Config
	client     *gosnmp.GoSNMP
	device     *proxy.ProxiedDevice
	credential credentials.Credential
	mu         sync.RWMutex

	// Connection state
	connected   bool
	lastError   error
	lastConnect time.Time

	// Metrics
	metrics *protocols.AdapterMetrics
}

// NewAdapter creates a new SNMP adapter.
func NewAdapter(config *Config) *Adapter {
	if config == nil {
		config = DefaultConfig()
	}
	if config.ConnectionConfig == nil {
		config.ConnectionConfig = protocols.DefaultConnectionConfig()
	}
	return &Adapter{
		config:  config,
		metrics: &protocols.AdapterMetrics{},
	}
}

// Type returns the protocol type.
func (a *Adapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSNMP
}

// Connect establishes an SNMP connection to the device.
func (a *Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
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

	// Determine port
	port := a.config.Port
	if device.Port > 0 {
		port = device.Port
	}

	// Create SNMP client
	client := &gosnmp.GoSNMP{
		Target:         device.Address,
		Port:           uint16(port),
		Timeout:        a.config.Timeout,
		Retries:        a.config.Retries,
		MaxOids:        a.config.MaxOids,
		MaxRepetitions: a.config.MaxRepetitions,
	}

	// Configure based on version and credentials
	switch a.config.Version {
	case SNMPv1:
		client.Version = gosnmp.Version1
		if err := a.configureV1V2c(client, cred); err != nil {
			a.lastError = err
			a.metrics.ConnectionErrors++
			return err
		}

	case SNMPv2c:
		client.Version = gosnmp.Version2c
		if err := a.configureV1V2c(client, cred); err != nil {
			a.lastError = err
			a.metrics.ConnectionErrors++
			return err
		}

	case SNMPv3:
		client.Version = gosnmp.Version3
		if err := a.configureV3(client, cred); err != nil {
			a.lastError = err
			a.metrics.ConnectionErrors++
			return err
		}
	}

	// Connect
	if err := client.Connect(); err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("SNMP connection failed: %w", err)
	}

	a.client = client
	a.connected = true
	a.lastConnect = time.Now()
	a.lastError = nil
	a.metrics.ConnectionCount++
	a.metrics.LastActivity = time.Now()

	return nil
}

// configureV1V2c configures SNMP v1/v2c with community string.
func (a *Adapter) configureV1V2c(client *gosnmp.GoSNMP, cred credentials.Credential) error {
	switch c := cred.(type) {
	case *credentials.SNMPv2cCredential:
		client.Community = c.Community
	default:
		return fmt.Errorf("unsupported credential type for SNMPv1/v2c: %T", cred)
	}
	return nil
}

// configureV3 configures SNMP v3 with USM security.
func (a *Adapter) configureV3(client *gosnmp.GoSNMP, cred credentials.Credential) error {
	switch c := cred.(type) {
	case *credentials.SNMPv3Credential:
		client.SecurityModel = gosnmp.UserSecurityModel
		client.MsgFlags = a.getMsgFlags(c)
		client.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 c.Username,
			AuthenticationProtocol:   a.getAuthProtocol(string(c.AuthProtocol)),
			AuthenticationPassphrase: c.AuthPassword,
			PrivacyProtocol:          a.getPrivProtocol(string(c.PrivacyProtocol)),
			PrivacyPassphrase:        c.PrivacyPassword,
		}
	default:
		return fmt.Errorf("unsupported credential type for SNMPv3: %T", cred)
	}
	return nil
}

// getMsgFlags returns the SNMP message flags for the given credential.
func (a *Adapter) getMsgFlags(cred *credentials.SNMPv3Credential) gosnmp.SnmpV3MsgFlags {
	if cred.AuthProtocol == "" && cred.PrivacyProtocol == "" {
		return gosnmp.NoAuthNoPriv
	}
	if cred.PrivacyProtocol == "" {
		return gosnmp.AuthNoPriv
	}
	return gosnmp.AuthPriv
}

// getAuthProtocol returns the gosnmp authentication protocol.
func (a *Adapter) getAuthProtocol(proto string) gosnmp.SnmpV3AuthProtocol {
	switch proto {
	case "MD5":
		md5DeprecationOnce.Do(func() {
			log.Printf("WARNING: MD5 authentication is cryptographically weak and deprecated. Consider using SHA-256 or stronger.")
		})
		return gosnmp.MD5
	case "SHA":
		return gosnmp.SHA
	case "SHA224":
		return gosnmp.SHA224
	case "SHA256":
		return gosnmp.SHA256
	case "SHA384":
		return gosnmp.SHA384
	case "SHA512":
		return gosnmp.SHA512
	default:
		return gosnmp.NoAuth
	}
}

// getPrivProtocol returns the gosnmp privacy protocol.
func (a *Adapter) getPrivProtocol(proto string) gosnmp.SnmpV3PrivProtocol {
	switch proto {
	case "DES":
		desDeprecationOnce.Do(func() {
			log.Printf("WARNING: DES encryption is cryptographically weak and deprecated. Consider using AES-128 or stronger.")
		})
		return gosnmp.DES
	case "AES":
		return gosnmp.AES
	case "AES192":
		return gosnmp.AES192
	case "AES256":
		return gosnmp.AES256
	case "AES192C":
		return gosnmp.AES192C
	case "AES256C":
		return gosnmp.AES256C
	default:
		return gosnmp.NoPriv
	}
}

// Execute executes an SNMP operation.
// For SNMP, the "command" is interpreted as an operation type:
// - "get <oid>..." - GET operation
// - "set <oid> <type> <value>" - SET operation
// - "walk <oid>" - WALK operation
// - "bulk <oid>" - GETBULK operation
func (a *Adapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	result := &protocols.ExecuteResult{
		StartTime: time.Now(),
	}

	// Parse the command to determine operation
	output, err := a.executeOperation(ctx, req.Command)
	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
	} else {
		result.Stdout = output
		result.ExitCode = 0
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Update metrics
	a.mu.Lock()
	a.metrics.ExecutionCount++
	if result.Error != "" {
		a.metrics.ExecutionErrors++
	}
	a.metrics.LastActivity = time.Now()
	a.mu.Unlock()

	return result, nil
}

// executeOperation executes the SNMP operation based on command.
func (a *Adapter) executeOperation(ctx context.Context, command string) ([]byte, error) {
	// This is a simplified implementation - in production, parse the command properly
	// For now, treat the command as a single OID for GET
	if command == "" {
		return nil, fmt.Errorf("no OID specified")
	}

	result, err := a.client.Get([]string{command})
	if err != nil {
		return nil, err
	}

	return formatSNMPResult(result), nil
}

// formatSNMPResult formats an SNMP packet as bytes.
func formatSNMPResult(packet *gosnmp.SnmpPacket) []byte {
	var output string
	for _, variable := range packet.Variables {
		output += formatPDU(variable) + "\n"
	}
	return []byte(output)
}

// formatPDU formats a single SNMP PDU.
func formatPDU(pdu gosnmp.SnmpPDU) string {
	switch pdu.Type {
	case gosnmp.OctetString:
		return fmt.Sprintf("%s = STRING: %s", pdu.Name, string(pdu.Value.([]byte)))
	case gosnmp.ObjectIdentifier:
		return fmt.Sprintf("%s = OID: %s", pdu.Name, pdu.Value.(string))
	case gosnmp.Integer:
		return fmt.Sprintf("%s = INTEGER: %d", pdu.Name, pdu.Value.(int))
	case gosnmp.Counter32:
		return fmt.Sprintf("%s = Counter32: %d", pdu.Name, pdu.Value.(uint))
	case gosnmp.Counter64:
		return fmt.Sprintf("%s = Counter64: %d", pdu.Name, pdu.Value.(uint64))
	case gosnmp.Gauge32:
		return fmt.Sprintf("%s = Gauge32: %d", pdu.Name, pdu.Value.(uint))
	case gosnmp.TimeTicks:
		return fmt.Sprintf("%s = Timeticks: (%d)", pdu.Name, pdu.Value.(uint32))
	case gosnmp.IPAddress:
		return fmt.Sprintf("%s = IpAddress: %s", pdu.Name, pdu.Value.(string))
	case gosnmp.Null:
		return fmt.Sprintf("%s = NULL", pdu.Name)
	case gosnmp.NoSuchObject:
		return fmt.Sprintf("%s = No Such Object", pdu.Name)
	case gosnmp.NoSuchInstance:
		return fmt.Sprintf("%s = No Such Instance", pdu.Name)
	case gosnmp.EndOfMibView:
		return fmt.Sprintf("%s = End of MIB View", pdu.Name)
	default:
		return fmt.Sprintf("%s = %v", pdu.Name, pdu.Value)
	}
}

// Disconnect closes the SNMP connection.
func (a *Adapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil && a.client.Conn != nil {
		err := a.client.Conn.Close()
		a.client = nil
		a.connected = false
		return err
	}
	return nil
}

// HealthCheck performs a health check on the SNMP connection.
func (a *Adapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
		Details:   make(map[string]interface{}),
	}

	if !connected || client == nil {
		result.Healthy = false
		result.Status = "not connected"
		return result, nil
	}

	// Try to get sysUpTime.0 as health check
	start := time.Now()
	_, err := client.Get([]string{".1.3.6.1.2.1.1.3.0"})
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["last_connect"] = a.lastConnect
	result.Details["version"] = a.config.Version.String()

	return result, nil
}

// IsConnected returns true if the SNMP connection is active.
func (a *Adapter) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected && a.client != nil
}

// Client returns the underlying SNMP client.
func (a *Adapter) Client() *gosnmp.GoSNMP {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.client
}

// Metrics returns the adapter metrics.
func (a *Adapter) Metrics() *protocols.AdapterMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.metrics
}

// Get performs an SNMP GET operation.
func (a *Adapter) Get(ctx context.Context, oids []string) ([]SNMPVariable, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	result, err := client.Get(oids)
	if err != nil {
		return nil, err
	}

	return convertVariables(result.Variables), nil
}

// GetNext performs an SNMP GETNEXT operation.
func (a *Adapter) GetNext(ctx context.Context, oids []string) ([]SNMPVariable, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	result, err := client.GetNext(oids)
	if err != nil {
		return nil, err
	}

	return convertVariables(result.Variables), nil
}

// Set performs an SNMP SET operation.
func (a *Adapter) Set(ctx context.Context, pdus []SNMPVariable) error {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return fmt.Errorf("not connected")
	}

	gosnmpPDUs := make([]gosnmp.SnmpPDU, len(pdus))
	for i, pdu := range pdus {
		gosnmpPDUs[i] = gosnmp.SnmpPDU{
			Name:  pdu.OID,
			Type:  pdu.Type,
			Value: pdu.Value,
		}
	}

	_, err := client.Set(gosnmpPDUs)
	return err
}

// Walk performs an SNMP WALK operation.
func (a *Adapter) Walk(ctx context.Context, rootOid string) ([]SNMPVariable, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	var results []SNMPVariable
	err := client.Walk(rootOid, func(pdu gosnmp.SnmpPDU) error {
		results = append(results, SNMPVariable{
			OID:   pdu.Name,
			Type:  pdu.Type,
			Value: pdu.Value,
		})
		return nil
	})

	return results, err
}

// BulkWalk performs an SNMP GETBULK WALK operation (v2c/v3 only).
func (a *Adapter) BulkWalk(ctx context.Context, rootOid string) ([]SNMPVariable, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	if a.config.Version == SNMPv1 {
		// Fall back to regular walk for v1
		return a.Walk(ctx, rootOid)
	}

	var results []SNMPVariable
	err := client.BulkWalk(rootOid, func(pdu gosnmp.SnmpPDU) error {
		results = append(results, SNMPVariable{
			OID:   pdu.Name,
			Type:  pdu.Type,
			Value: pdu.Value,
		})
		return nil
	})

	return results, err
}

// SNMPVariable represents an SNMP variable binding.
type SNMPVariable struct {
	OID   string
	Type  gosnmp.Asn1BER
	Value interface{}
}

// convertVariables converts gosnmp PDUs to SNMPVariables.
func convertVariables(pdus []gosnmp.SnmpPDU) []SNMPVariable {
	vars := make([]SNMPVariable, len(pdus))
	for i, pdu := range pdus {
		vars[i] = SNMPVariable{
			OID:   pdu.Name,
			Type:  pdu.Type,
			Value: pdu.Value,
		}
	}
	return vars
}

// NewAdapterFactory creates an adapter factory for SNMP.
func NewAdapterFactory(config *Config) protocols.AdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.ProtocolAdapter, error) {
		cfg := config
		if cfg == nil {
			cfg = DefaultConfig()
		}
		cfg.ConnectionConfig = connConfig
		return NewAdapter(cfg), nil
	}
}

// init registers the SNMP adapter with the default registry.
func init() {
	protocols.Register(protocols.ProtocolSNMP, NewAdapterFactory(nil))
}
