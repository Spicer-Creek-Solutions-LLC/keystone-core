package snmp

import (
	"testing"

	"github.com/gosnmp/gosnmp"

	"github.com/shawnbutts/keystone-core/pkg/credentials"
	"github.com/shawnbutts/keystone-core/pkg/protocols"
)

func TestSNMPVersionString(t *testing.T) {
	tests := []struct {
		version SNMPVersion
		want    string
	}{
		{SNMPv1, "1"},
		{SNMPv2c, "2c"},
		{SNMPv3, "3"},
		{SNMPVersion(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.version.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Port != 161 {
		t.Errorf("Port = %d, want 161", cfg.Port)
	}
	if cfg.Version != SNMPv2c {
		t.Errorf("Version = %v, want SNMPv2c", cfg.Version)
	}
	if cfg.Retries != 3 {
		t.Errorf("Retries = %d, want 3", cfg.Retries)
	}
	if cfg.MaxOids != 60 {
		t.Errorf("MaxOids = %d, want 60", cfg.MaxOids)
	}
	if cfg.MaxRepetitions != 10 {
		t.Errorf("MaxRepetitions = %d, want 10", cfg.MaxRepetitions)
	}
	if cfg.ConnectionConfig == nil {
		t.Error("ConnectionConfig should not be nil")
	}
}

func TestNewAdapter(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
		},
		{
			name: "custom config",
			config: &Config{
				Port:    1161,
				Version: SNMPv3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewAdapter(tt.config)
			if adapter == nil {
				t.Fatal("expected adapter to be created")
			}
			if adapter.config == nil {
				t.Error("config should not be nil")
			}
			if adapter.metrics == nil {
				t.Error("metrics should not be nil")
			}
		})
	}
}

func TestAdapterType(t *testing.T) {
	adapter := NewAdapter(nil)
	if adapter.Type() != protocols.ProtocolSNMP {
		t.Errorf("Type() = %v, want ProtocolSNMP", adapter.Type())
	}
}

func TestAdapterIsConnected(t *testing.T) {
	adapter := NewAdapter(nil)

	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestAdapterMetrics(t *testing.T) {
	adapter := NewAdapter(nil)
	metrics := adapter.Metrics()

	if metrics == nil {
		t.Error("Metrics() should not return nil")
	}
}

func TestAdapterClient(t *testing.T) {
	adapter := NewAdapter(nil)
	client := adapter.Client()

	if client != nil {
		t.Error("Client() should be nil when not connected")
	}
}

func TestConfigureV1V2c(t *testing.T) {
	adapter := NewAdapter(nil)

	t.Run("valid SNMPv2c credential", func(t *testing.T) {
		client := &gosnmp.GoSNMP{}
		cred := &credentials.SNMPv2cCredential{Community: "public"}

		err := adapter.configureV1V2c(client, cred)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if client.Community != "public" {
			t.Errorf("Community = %v, want 'public'", client.Community)
		}
	})

	t.Run("invalid credential type", func(t *testing.T) {
		client := &gosnmp.GoSNMP{}
		cred := &credentials.RESTBasicCredential{}

		err := adapter.configureV1V2c(client, cred)
		if err == nil {
			t.Error("expected error for invalid credential type")
		}
	})
}

func TestConfigureV3(t *testing.T) {
	adapter := NewAdapter(nil)

	t.Run("valid SNMPv3 credential", func(t *testing.T) {
		client := &gosnmp.GoSNMP{}
		cred := &credentials.SNMPv3Credential{
			Username:        "admin",
			AuthProtocol:    "SHA256",
			AuthPassword:    "authpass",
			PrivacyProtocol: "AES",
			PrivacyPassword: "privpass",
		}

		err := adapter.configureV3(client, cred)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if client.SecurityModel != gosnmp.UserSecurityModel {
			t.Error("SecurityModel should be UserSecurityModel")
		}
		if client.MsgFlags != gosnmp.AuthPriv {
			t.Errorf("MsgFlags = %v, want AuthPriv", client.MsgFlags)
		}
	})

	t.Run("invalid credential type", func(t *testing.T) {
		client := &gosnmp.GoSNMP{}
		cred := &credentials.SNMPv2cCredential{}

		err := adapter.configureV3(client, cred)
		if err == nil {
			t.Error("expected error for invalid credential type")
		}
	})
}

func TestGetMsgFlags(t *testing.T) {
	adapter := NewAdapter(nil)

	tests := []struct {
		name string
		cred *credentials.SNMPv3Credential
		want gosnmp.SnmpV3MsgFlags
	}{
		{
			name: "NoAuthNoPriv",
			cred: &credentials.SNMPv3Credential{},
			want: gosnmp.NoAuthNoPriv,
		},
		{
			name: "AuthNoPriv",
			cred: &credentials.SNMPv3Credential{
				AuthProtocol: "SHA",
			},
			want: gosnmp.AuthNoPriv,
		},
		{
			name: "AuthPriv",
			cred: &credentials.SNMPv3Credential{
				AuthProtocol:    "SHA",
				PrivacyProtocol: "AES",
			},
			want: gosnmp.AuthPriv,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.getMsgFlags(tt.cred)
			if got != tt.want {
				t.Errorf("getMsgFlags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAuthProtocol(t *testing.T) {
	adapter := NewAdapter(nil)

	tests := []struct {
		proto string
		want  gosnmp.SnmpV3AuthProtocol
	}{
		{"", gosnmp.NoAuth},
		{"MD5", gosnmp.MD5},
		{"SHA", gosnmp.SHA},
		{"SHA224", gosnmp.SHA224},
		{"SHA256", gosnmp.SHA256},
		{"SHA384", gosnmp.SHA384},
		{"SHA512", gosnmp.SHA512},
		{"INVALID", gosnmp.NoAuth},
	}

	for _, tt := range tests {
		t.Run(tt.proto, func(t *testing.T) {
			got := adapter.getAuthProtocol(tt.proto)
			if got != tt.want {
				t.Errorf("getAuthProtocol(%q) = %v, want %v", tt.proto, got, tt.want)
			}
		})
	}
}

func TestGetPrivProtocol(t *testing.T) {
	adapter := NewAdapter(nil)

	tests := []struct {
		proto string
		want  gosnmp.SnmpV3PrivProtocol
	}{
		{"", gosnmp.NoPriv},
		{"DES", gosnmp.DES},
		{"AES", gosnmp.AES},
		{"AES192", gosnmp.AES192},
		{"AES256", gosnmp.AES256},
		{"AES192C", gosnmp.AES192C},
		{"AES256C", gosnmp.AES256C},
		{"INVALID", gosnmp.NoPriv},
	}

	for _, tt := range tests {
		t.Run(tt.proto, func(t *testing.T) {
			got := adapter.getPrivProtocol(tt.proto)
			if got != tt.want {
				t.Errorf("getPrivProtocol(%q) = %v, want %v", tt.proto, got, tt.want)
			}
		})
	}
}

func TestFormatPDU(t *testing.T) {
	tests := []struct {
		name string
		pdu  gosnmp.SnmpPDU
		want string
	}{
		{
			name: "OctetString",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("Test System")},
			want: ".1.3.6.1.2.1.1.1.0 = STRING: Test System",
		},
		{
			name: "ObjectIdentifier",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.2.0", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4.1.9"},
			want: ".1.3.6.1.2.1.1.2.0 = OID: .1.3.6.1.4.1.9",
		},
		{
			name: "Integer",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.7.0", Type: gosnmp.Integer, Value: 72},
			want: ".1.3.6.1.2.1.1.7.0 = INTEGER: 72",
		},
		{
			name: "Counter32",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.2.2.1.10.1", Type: gosnmp.Counter32, Value: uint(123456)},
			want: ".1.3.6.1.2.1.2.2.1.10.1 = Counter32: 123456",
		},
		{
			name: "Counter64",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.31.1.1.1.6.1", Type: gosnmp.Counter64, Value: uint64(9876543210)},
			want: ".1.3.6.1.2.1.31.1.1.1.6.1 = Counter64: 9876543210",
		},
		{
			name: "Gauge32",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.2.2.1.5.1", Type: gosnmp.Gauge32, Value: uint(1000000000)},
			want: ".1.3.6.1.2.1.2.2.1.5.1 = Gauge32: 1000000000",
		},
		{
			name: "TimeTicks",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(123456789)},
			want: ".1.3.6.1.2.1.1.3.0 = Timeticks: (123456789)",
		},
		{
			name: "IPAddress",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.4.20.1.1.192.168.1.1", Type: gosnmp.IPAddress, Value: "192.168.1.1"},
			want: ".1.3.6.1.2.1.4.20.1.1.192.168.1.1 = IpAddress: 192.168.1.1",
		},
		{
			name: "Null",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.1.0", Type: gosnmp.Null, Value: nil},
			want: ".1.3.6.1.2.1.1.1.0 = NULL",
		},
		{
			name: "NoSuchObject",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.99.0", Type: gosnmp.NoSuchObject, Value: nil},
			want: ".1.3.6.1.2.1.1.99.0 = No Such Object",
		},
		{
			name: "NoSuchInstance",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.1.99", Type: gosnmp.NoSuchInstance, Value: nil},
			want: ".1.3.6.1.2.1.1.1.99 = No Such Instance",
		},
		{
			name: "EndOfMibView",
			pdu:  gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.999", Type: gosnmp.EndOfMibView, Value: nil},
			want: ".1.3.6.1.2.1.999 = End of MIB View",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPDU(tt.pdu)
			if got != tt.want {
				t.Errorf("formatPDU() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatSNMPResult(t *testing.T) {
	packet := &gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{Name: ".1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("Test")},
			{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(12345)},
		},
	}

	result := formatSNMPResult(packet)
	resultStr := string(result)

	if len(result) == 0 {
		t.Error("expected non-empty result")
	}

	// Check that both OIDs are in the output
	if !contains(resultStr, ".1.3.6.1.2.1.1.1.0") {
		t.Error("result should contain first OID")
	}
	if !contains(resultStr, ".1.3.6.1.2.1.1.3.0") {
		t.Error("result should contain second OID")
	}
}

func TestConvertVariables(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{Name: ".1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("Test")},
		{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.Integer, Value: 42},
	}

	vars := convertVariables(pdus)

	if len(vars) != 2 {
		t.Errorf("len(vars) = %d, want 2", len(vars))
	}

	if vars[0].OID != ".1.3.6.1.2.1.1.1.0" {
		t.Errorf("vars[0].OID = %v, want .1.3.6.1.2.1.1.1.0", vars[0].OID)
	}
	if vars[0].Type != gosnmp.OctetString {
		t.Errorf("vars[0].Type = %v, want OctetString", vars[0].Type)
	}

	if vars[1].OID != ".1.3.6.1.2.1.1.3.0" {
		t.Errorf("vars[1].OID = %v, want .1.3.6.1.2.1.1.3.0", vars[1].OID)
	}
	if vars[1].Type != gosnmp.Integer {
		t.Errorf("vars[1].Type = %v, want Integer", vars[1].Type)
	}
}

func TestNewAdapterFactory(t *testing.T) {
	factory := NewAdapterFactory(nil)
	if factory == nil {
		t.Fatal("expected factory to be created")
	}

	connConfig := protocols.DefaultConnectionConfig()
	adapter, err := factory(connConfig)
	if err != nil {
		t.Errorf("factory failed: %v", err)
	}
	if adapter == nil {
		t.Error("expected adapter from factory")
	}
	if adapter.Type() != protocols.ProtocolSNMP {
		t.Errorf("expected SNMP protocol, got %v", adapter.Type())
	}
}

func TestCommonOIDs(t *testing.T) {
	// Verify some common OIDs are defined correctly
	if CommonOIDs.SysDescr != ".1.3.6.1.2.1.1.1.0" {
		t.Errorf("SysDescr = %v, want .1.3.6.1.2.1.1.1.0", CommonOIDs.SysDescr)
	}
	if CommonOIDs.SysUpTime != ".1.3.6.1.2.1.1.3.0" {
		t.Errorf("SysUpTime = %v, want .1.3.6.1.2.1.1.3.0", CommonOIDs.SysUpTime)
	}
	if CommonOIDs.SysName != ".1.3.6.1.2.1.1.5.0" {
		t.Errorf("SysName = %v, want .1.3.6.1.2.1.1.5.0", CommonOIDs.SysName)
	}
	if CommonOIDs.IfNumber != ".1.3.6.1.2.1.2.1.0" {
		t.Errorf("IfNumber = %v, want .1.3.6.1.2.1.2.1.0", CommonOIDs.IfNumber)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
