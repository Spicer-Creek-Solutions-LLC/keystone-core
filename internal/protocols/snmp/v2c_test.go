package snmp

import (
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestNewV2cAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewV2cAdapter(nil)
		if adapter == nil {
			t.Fatal("expected adapter to be created")
		}
		if adapter.config.Version != SNMPv2c {
			t.Errorf("Version = %v, want SNMPv2c", adapter.config.Version)
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &Config{
			Port:    1161,
			Version: SNMPv1, // Should be overridden
		}
		adapter := NewV2cAdapter(cfg)
		if adapter.config.Version != SNMPv2c {
			t.Errorf("Version = %v, want SNMPv2c (overridden)", adapter.config.Version)
		}
		if adapter.config.Port != 1161 {
			t.Errorf("Port = %d, want 1161", adapter.config.Port)
		}
	})
}

func TestExtractIndex(t *testing.T) {
	tests := []struct {
		name    string
		baseOID string
		fullOID string
		want    string
	}{
		{
			name:    "simple index",
			baseOID: ".1.3.6.1.2.1.2.2.1.2",
			fullOID: ".1.3.6.1.2.1.2.2.1.2.1",
			want:    "1",
		},
		{
			name:    "compound index",
			baseOID: ".1.3.6.1.2.1.4.20.1.1",
			fullOID: ".1.3.6.1.2.1.4.20.1.1.192.168.1.1",
			want:    "192.168.1.1",
		},
		{
			name:    "no index",
			baseOID: ".1.3.6.1.2.1.1.1",
			fullOID: ".1.3.6.1.2.1.1.1",
			want:    "",
		},
		{
			name:    "empty base",
			baseOID: "",
			fullOID: ".1.3.6.1.2.1.1.1.0",
			want:    "1.3.6.1.2.1.1.1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIndex(tt.baseOID, tt.fullOID)
			if got != tt.want {
				t.Errorf("extractIndex(%q, %q) = %q, want %q", tt.baseOID, tt.fullOID, got, tt.want)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name string
		pdu  gosnmp.SnmpPDU
		want interface{}
	}{
		{
			name: "OctetString",
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.OctetString, Value: []byte("test")},
			want: "test",
		},
		{
			name: "Integer",
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.Integer, Value: 42},
			want: 42,
		},
		{
			name: "Counter32",
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.Counter32, Value: uint(12345)},
			want: uint(12345),
		},
		{
			name: "Gauge32",
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.Gauge32, Value: uint(67890)},
			want: uint(67890),
		},
		{
			name: "Counter64",
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.Counter64, Value: uint64(9876543210)},
			want: uint64(9876543210),
		},
		{
			name: "TimeTicks",
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.TimeTicks, Value: uint32(12345)},
			want: uint32(12345),
		},
		{
			name: "IPAddress",
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.IPAddress, Value: "192.168.1.1"},
			want: "192.168.1.1",
		},
		{
			name: "Unknown type passthrough",
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.Opaque, Value: []byte{0x01, 0x02}},
			want: []byte{0x01, 0x02},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatValue(tt.pdu)
			// Compare values based on type
			switch want := tt.want.(type) {
			case string:
				if g, ok := got.(string); !ok || g != want {
					t.Errorf("formatValue() = %v (%T), want %v (%T)", got, got, want, want)
				}
			case int:
				if g, ok := got.(int); !ok || g != want {
					t.Errorf("formatValue() = %v (%T), want %v (%T)", got, got, want, want)
				}
			case uint:
				if g, ok := got.(uint); !ok || g != want {
					t.Errorf("formatValue() = %v (%T), want %v (%T)", got, got, want, want)
				}
			case uint32:
				if g, ok := got.(uint32); !ok || g != want {
					t.Errorf("formatValue() = %v (%T), want %v (%T)", got, got, want, want)
				}
			case uint64:
				if g, ok := got.(uint64); !ok || g != want {
					t.Errorf("formatValue() = %v (%T), want %v (%T)", got, got, want, want)
				}
			}
		})
	}
}

func TestNewTrapReceiver(t *testing.T) {
	receiver := NewTrapReceiver("public")

	if receiver == nil {
		t.Fatal("expected receiver to be created")
	}
	if receiver.community != "public" {
		t.Errorf("community = %v, want 'public'", receiver.community)
	}
	if len(receiver.handlers) != 0 {
		t.Errorf("handlers should be empty, got %d", len(receiver.handlers))
	}
	if receiver.running {
		t.Error("receiver should not be running")
	}
}

func TestTrapReceiverAddHandler(t *testing.T) {
	receiver := NewTrapReceiver("public")

	handlerCalled := false
	receiver.AddHandler(func(trap *Trap) {
		handlerCalled = true
	})

	if len(receiver.handlers) != 1 {
		t.Errorf("handlers count = %d, want 1", len(receiver.handlers))
	}

	// Call the handler to verify it was added correctly
	receiver.handlers[0](&Trap{})
	if !handlerCalled {
		t.Error("handler should have been called")
	}
}

func TestTrapReceiverStopWhenNotRunning(t *testing.T) {
	receiver := NewTrapReceiver("public")

	err := receiver.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestTableStructure(t *testing.T) {
	table := &Table{
		BaseOID: ".1.3.6.1.2.1.2.2",
		Columns: map[string]string{
			"index": "1.1",
			"descr": "1.2",
		},
		Rows: []TableRow{
			{
				Index: "1",
				Values: map[string]interface{}{
					"index": 1,
					"descr": "eth0",
				},
			},
			{
				Index: "2",
				Values: map[string]interface{}{
					"index": 2,
					"descr": "eth1",
				},
			},
		},
	}

	if table.BaseOID != ".1.3.6.1.2.1.2.2" {
		t.Errorf("BaseOID = %v", table.BaseOID)
	}
	if len(table.Columns) != 2 {
		t.Errorf("Columns count = %d", len(table.Columns))
	}
	if len(table.Rows) != 2 {
		t.Errorf("Rows count = %d", len(table.Rows))
	}
	if table.Rows[0].Index != "1" {
		t.Errorf("Rows[0].Index = %v", table.Rows[0].Index)
	}
	if table.Rows[0].Values["descr"] != "eth0" {
		t.Errorf("Rows[0].Values['descr'] = %v", table.Rows[0].Values["descr"])
	}
}

func TestTrapStructure(t *testing.T) {
	trap := &Trap{
		Source:       "192.168.1.1:162",
		Enterprise:   ".1.3.6.1.4.1.9.9.1.2.3",
		GenericTrap:  6,
		SpecificTrap: 1,
		Timestamp:    12345,
		Variables: []Variable{
			{OID: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(12345)},
		},
	}

	if trap.Source != "192.168.1.1:162" {
		t.Errorf("Source = %v", trap.Source)
	}
	if trap.Enterprise != ".1.3.6.1.4.1.9.9.1.2.3" {
		t.Errorf("Enterprise = %v", trap.Enterprise)
	}
	if trap.GenericTrap != 6 {
		t.Errorf("GenericTrap = %d", trap.GenericTrap)
	}
	if trap.SpecificTrap != 1 {
		t.Errorf("SpecificTrap = %d", trap.SpecificTrap)
	}
	if len(trap.Variables) != 1 {
		t.Errorf("Variables count = %d", len(trap.Variables))
	}
}

func TestSystemInfoStructure(t *testing.T) {
	info := &SystemInfo{
		Description: "Test System",
		ObjectID:    ".1.3.6.1.4.1.9.1.1",
		UpTime:      12345678,
		Contact:     "admin@example.com",
		Name:        "test-router",
		Location:    "Data Center 1",
	}

	if info.Description != "Test System" {
		t.Errorf("Description = %v", info.Description)
	}
	if info.Name != "test-router" {
		t.Errorf("Name = %v", info.Name)
	}
}

func TestInterfaceInfoStructure(t *testing.T) {
	iface := InterfaceInfo{
		Index:       1,
		Description: "GigabitEthernet0/1",
		Type:        6,
		MTU:         1500,
		Speed:       1000000000,
		PhysAddress: "00:11:22:33:44:55",
		AdminStatus: 1,
		OperStatus:  1,
		InOctets:    123456,
		OutOctets:   654321,
		InErrors:    0,
		OutErrors:   0,
	}

	if iface.Index != 1 {
		t.Errorf("Index = %d", iface.Index)
	}
	if iface.Description != "GigabitEthernet0/1" {
		t.Errorf("Description = %v", iface.Description)
	}
	if iface.MTU != 1500 {
		t.Errorf("MTU = %d", iface.MTU)
	}
	if iface.Speed != 1000000000 {
		t.Errorf("Speed = %d", iface.Speed)
	}
}

func TestInformResponseStructure(t *testing.T) {
	resp := &InformResponse{
		RequestID: 12345,
		Error:     gosnmp.NoError,
		ErrorIdx:  0,
		Variables: []Variable{
			{OID: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(12345)},
		},
	}

	if resp.RequestID != 12345 {
		t.Errorf("RequestID = %d", resp.RequestID)
	}
	if resp.Error != gosnmp.NoError {
		t.Errorf("Error = %v", resp.Error)
	}
	if len(resp.Variables) != 1 {
		t.Errorf("Variables count = %d", len(resp.Variables))
	}
}
