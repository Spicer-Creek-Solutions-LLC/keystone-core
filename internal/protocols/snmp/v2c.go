// Package snmp provides an SNMP protocol adapter for proxy agents.
package snmp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// V2cAdapter extends Adapter with SNMPv2c-specific operations.
type V2cAdapter struct {
	*Adapter
}

// NewV2cAdapter creates a new SNMPv2c adapter.
func NewV2cAdapter(config *Config) *V2cAdapter {
	if config == nil {
		config = DefaultConfig()
	}
	config.Version = SNMPv2c
	return &V2cAdapter{
		Adapter: NewAdapter(config),
	}
}

// GetBulk performs an SNMP GETBULK operation.
func (a *V2cAdapter) GetBulk(ctx context.Context, oids []string, nonRepeaters uint8, maxRepetitions uint32) ([]SNMPVariable, error) {
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

// Table represents an SNMP table.
type Table struct {
	BaseOID string
	Columns map[string]string // Column name -> OID suffix
	Rows    []TableRow
}

// TableRow represents a row in an SNMP table.
type TableRow struct {
	Index  string
	Values map[string]interface{}
}

// GetTable retrieves an SNMP table.
func (a *V2cAdapter) GetTable(ctx context.Context, baseOID string, columns map[string]string) (*Table, error) {
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

	// Walk each column and build rows
	rowData := make(map[string]map[string]interface{}) // index -> column name -> value

	for colName, colOID := range columns {
		fullOID := baseOID + "." + colOID
		err := client.BulkWalk(fullOID, func(pdu gosnmp.SnmpPDU) error {
			// Extract index from OID
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

	// Convert to rows
	for index, values := range rowData {
		table.Rows = append(table.Rows, TableRow{
			Index:  index,
			Values: values,
		})
	}

	return table, nil
}

// extractIndex extracts the row index from an OID.
func extractIndex(baseOID, fullOID string) string {
	if len(fullOID) <= len(baseOID)+1 {
		return ""
	}
	return fullOID[len(baseOID)+1:]
}

// formatValue formats an SNMP PDU value.
func formatValue(pdu gosnmp.SnmpPDU) interface{} {
	switch pdu.Type {
	case gosnmp.OctetString:
		return string(pdu.Value.([]byte))
	case gosnmp.Integer:
		return pdu.Value.(int)
	case gosnmp.Counter32, gosnmp.Gauge32:
		return pdu.Value.(uint)
	case gosnmp.Counter64:
		return pdu.Value.(uint64)
	case gosnmp.TimeTicks:
		return pdu.Value.(uint32)
	case gosnmp.IPAddress:
		return pdu.Value.(string)
	default:
		return pdu.Value
	}
}

// TrapReceiver handles incoming SNMP traps.
type TrapReceiver struct {
	listener  *gosnmp.TrapListener
	handlers  []TrapHandler
	mu        sync.RWMutex
	running   bool
	community string
}

// TrapHandler is called when a trap is received.
type TrapHandler func(trap *Trap)

// Trap represents an SNMP trap.
type Trap struct {
	Source       string
	Enterprise   string
	GenericTrap  int
	SpecificTrap int
	Timestamp    uint32
	Variables    []SNMPVariable
	ReceivedAt   time.Time
}

// NewTrapReceiver creates a new trap receiver.
func NewTrapReceiver(community string) *TrapReceiver {
	return &TrapReceiver{
		handlers:  make([]TrapHandler, 0),
		community: community,
	}
}

// AddHandler adds a trap handler.
func (r *TrapReceiver) AddHandler(handler TrapHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers = append(r.handlers, handler)
}

// Start starts the trap receiver on the specified address.
func (r *TrapReceiver) Start(addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("trap receiver already running")
	}

	listener := gosnmp.NewTrapListener()
	listener.Params = gosnmp.Default
	listener.Params.Community = r.community

	listener.OnNewTrap = func(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
		trap := r.convertTrap(packet, addr)
		r.mu.RLock()
		handlers := make([]TrapHandler, len(r.handlers))
		copy(handlers, r.handlers)
		r.mu.RUnlock()

		for _, handler := range handlers {
			handler(trap)
		}
	}

	go func() {
		_ = listener.Listen(addr)
	}()

	r.listener = listener
	r.running = true
	return nil
}

// Stop stops the trap receiver.
func (r *TrapReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	r.listener.Close()
	r.running = false
	return nil
}

// convertTrap converts a gosnmp packet to a Trap.
func (r *TrapReceiver) convertTrap(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) *Trap {
	trap := &Trap{
		Source:     addr.String(),
		Variables:  convertVariables(packet.Variables),
		ReceivedAt: time.Now(),
	}

	// Extract trap-specific fields from variables
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

	return trap
}

// InformResponse represents a response to an SNMP INFORM.
type InformResponse struct {
	RequestID int32
	Error     gosnmp.SNMPError
	ErrorIdx  uint8
	Variables []SNMPVariable
}

// SendInform sends an SNMP INFORM request.
func (a *V2cAdapter) SendInform(ctx context.Context, target string, port uint16, variables []SNMPVariable) (*InformResponse, error) {
	a.mu.RLock()
	community := ""
	if a.client != nil {
		community = a.client.Community
	}
	a.mu.RUnlock()

	if community == "" {
		return nil, fmt.Errorf("no community string set")
	}

	// Create temporary client for inform
	client := &gosnmp.GoSNMP{
		Target:    target,
		Port:      port,
		Version:   gosnmp.Version2c,
		Community: community,
		Timeout:   a.config.Timeout,
		Retries:   a.config.Retries,
	}

	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect for inform: %w", err)
	}
	defer client.Conn.Close()

	pdus := make([]gosnmp.SnmpPDU, len(variables))
	for i, v := range variables {
		pdus[i] = gosnmp.SnmpPDU{
			Name:  v.OID,
			Type:  v.Type,
			Value: v.Value,
		}
	}

	// Send INFORM using SendTrap with IsInform=true
	// Unlike traps, INFORMs wait for acknowledgement from the NMS
	trap := gosnmp.SnmpTrap{
		Variables: pdus,
		IsInform:  true,
	}

	result, err := client.SendTrap(trap)
	if err != nil {
		return nil, fmt.Errorf("failed to send inform: %w", err)
	}

	// Convert response
	response := &InformResponse{
		RequestID: int32(result.RequestID),
		Error:     result.Error,
		ErrorIdx:  result.ErrorIndex,
		Variables: convertVariables(result.Variables),
	}

	return response, nil
}

// CommonOIDs provides common SNMP OIDs.
var CommonOIDs = struct {
	// System group
	SysDescr    string
	SysObjectID string
	SysUpTime   string
	SysContact  string
	SysName     string
	SysLocation string
	SysServices string

	// Interfaces group
	IfNumber    string
	IfIndex     string
	IfDescr     string
	IfType      string
	IfMtu       string
	IfSpeed     string
	IfPhysAddr  string
	IfAdminStat string
	IfOperStat  string
	IfInOctets  string
	IfOutOctets string
	IfInErrors  string
	IfOutErrors string

	// IP group
	IpForwarding    string
	IpDefaultTTL    string
	IpAdEntAddr     string
	IpAdEntNetMask  string
	IpAdEntIfIndex  string
	IpRouteDest     string
	IpRouteNextHop  string
	IpRouteIfIndex  string

	// SNMP group
	SNMPInPkts  string
	SNMPOutPkts string
}{
	// System group (.1.3.6.1.2.1.1)
	SysDescr:    ".1.3.6.1.2.1.1.1.0",
	SysObjectID: ".1.3.6.1.2.1.1.2.0",
	SysUpTime:   ".1.3.6.1.2.1.1.3.0",
	SysContact:  ".1.3.6.1.2.1.1.4.0",
	SysName:     ".1.3.6.1.2.1.1.5.0",
	SysLocation: ".1.3.6.1.2.1.1.6.0",
	SysServices: ".1.3.6.1.2.1.1.7.0",

	// Interfaces group (.1.3.6.1.2.1.2)
	IfNumber:    ".1.3.6.1.2.1.2.1.0",
	IfIndex:     ".1.3.6.1.2.1.2.2.1.1",
	IfDescr:     ".1.3.6.1.2.1.2.2.1.2",
	IfType:      ".1.3.6.1.2.1.2.2.1.3",
	IfMtu:       ".1.3.6.1.2.1.2.2.1.4",
	IfSpeed:     ".1.3.6.1.2.1.2.2.1.5",
	IfPhysAddr:  ".1.3.6.1.2.1.2.2.1.6",
	IfAdminStat: ".1.3.6.1.2.1.2.2.1.7",
	IfOperStat:  ".1.3.6.1.2.1.2.2.1.8",
	IfInOctets:  ".1.3.6.1.2.1.2.2.1.10",
	IfOutOctets: ".1.3.6.1.2.1.2.2.1.16",
	IfInErrors:  ".1.3.6.1.2.1.2.2.1.14",
	IfOutErrors: ".1.3.6.1.2.1.2.2.1.20",

	// IP group (.1.3.6.1.2.1.4)
	IpForwarding:   ".1.3.6.1.2.1.4.1.0",
	IpDefaultTTL:   ".1.3.6.1.2.1.4.2.0",
	IpAdEntAddr:    ".1.3.6.1.2.1.4.20.1.1",
	IpAdEntNetMask: ".1.3.6.1.2.1.4.20.1.3",
	IpAdEntIfIndex: ".1.3.6.1.2.1.4.20.1.2",
	IpRouteDest:    ".1.3.6.1.2.1.4.21.1.1",
	IpRouteNextHop: ".1.3.6.1.2.1.4.21.1.7",
	IpRouteIfIndex: ".1.3.6.1.2.1.4.21.1.2",

	// SNMP group (.1.3.6.1.2.1.11)
	SNMPInPkts:  ".1.3.6.1.2.1.11.1.0",
	SNMPOutPkts: ".1.3.6.1.2.1.11.2.0",
}

// GetSystemInfo retrieves common system information.
func (a *V2cAdapter) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
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

// SystemInfo contains system information.
type SystemInfo struct {
	Description string
	ObjectID    string
	UpTime      time.Duration
	Contact     string
	Name        string
	Location    string
}

// InterfaceInfo contains interface information.
type InterfaceInfo struct {
	Index       int
	Description string
	Type        int
	MTU         int
	Speed       uint
	PhysAddress string
	AdminStatus int
	OperStatus  int
	InOctets    uint
	OutOctets   uint
	InErrors    uint
	OutErrors   uint
}

// GetInterfaces retrieves interface information.
func (a *V2cAdapter) GetInterfaces(ctx context.Context) ([]InterfaceInfo, error) {
	// Get interface count
	vars, err := a.Get(ctx, []string{CommonOIDs.IfNumber})
	if err != nil {
		return nil, err
	}

	if len(vars) == 0 {
		return nil, fmt.Errorf("failed to get interface count")
	}

	// Walk interface table
	table, err := a.GetTable(ctx, ".1.3.6.1.2.1.2.2", map[string]string{
		"index":       "1.1",
		"description": "1.2",
		"type":        "1.3",
		"mtu":         "1.4",
		"speed":       "1.5",
		"phys_addr":   "1.6",
		"admin_stat":  "1.7",
		"oper_stat":   "1.8",
		"in_octets":   "1.10",
		"out_octets":  "1.16",
		"in_errors":   "1.14",
		"out_errors":  "1.20",
	})
	if err != nil {
		return nil, err
	}

	interfaces := make([]InterfaceInfo, len(table.Rows))
	for i, row := range table.Rows {
		iface := InterfaceInfo{}
		if v, ok := row.Values["index"].(int); ok {
			iface.Index = v
		}
		if v, ok := row.Values["description"].(string); ok {
			iface.Description = v
		}
		if v, ok := row.Values["type"].(int); ok {
			iface.Type = v
		}
		if v, ok := row.Values["mtu"].(int); ok {
			iface.MTU = v
		}
		if v, ok := row.Values["speed"].(uint); ok {
			iface.Speed = v
		}
		if v, ok := row.Values["phys_addr"].(string); ok {
			iface.PhysAddress = v
		}
		if v, ok := row.Values["admin_stat"].(int); ok {
			iface.AdminStatus = v
		}
		if v, ok := row.Values["oper_stat"].(int); ok {
			iface.OperStatus = v
		}
		if v, ok := row.Values["in_octets"].(uint); ok {
			iface.InOctets = v
		}
		if v, ok := row.Values["out_octets"].(uint); ok {
			iface.OutOctets = v
		}
		if v, ok := row.Values["in_errors"].(uint); ok {
			iface.InErrors = v
		}
		if v, ok := row.Values["out_errors"].(uint); ok {
			iface.OutErrors = v
		}
		interfaces[i] = iface
	}

	return interfaces, nil
}
