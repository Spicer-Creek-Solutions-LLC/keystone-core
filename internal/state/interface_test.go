package state

import (
	"strings"
	"testing"
)

func TestPostgreSQLConfig_BuildDSN_ExtraFields(t *testing.T) {
	cfg := &PostgreSQLConfig{
		Host:            "2001:db8::1",
		Port:            5432,
		Database:        "keystone",
		User:            "kscore",
		Password:        "secret",
		SSLMode:         "require",
		SSLRootCert:     "/path/ca.pem",
		SSLCert:         "/path/cert.pem",
		SSLKey:          "/path/key.pem",
		ConnectTimeout:  10,
		ApplicationName: "kscore-test",
	}

	dsn := cfg.BuildDSN()
	if dsn == "" {
		t.Fatal("Expected DSN to be built")
	}
	if !containsAll(dsn, []string{
		"host=[2001:db8::1]",
		"port=5432",
		"dbname=keystone",
		"user=kscore",
		"password=secret",
		"sslmode=require",
		"sslrootcert=/path/ca.pem",
		"sslcert=/path/cert.pem",
		"sslkey=/path/key.pem",
		"connect_timeout=10",
		"application_name=kscore-test",
	}) {
		t.Fatalf("DSN missing expected fields: %s", dsn)
	}

	cfg.Host = "[2001:db8::2]"
	dsn = cfg.BuildDSN()
	if !containsAll(dsn, []string{"host=[2001:db8::2]"}) {
		t.Fatalf("Expected bracketed IPv6 to normalize: %s", dsn)
	}

	if empty := (*PostgreSQLConfig)(nil).BuildDSN(); empty != "" {
		t.Errorf("Expected empty DSN for nil config, got %q", empty)
	}
}

func TestPostgreSQLConfig_Validate_Errors(t *testing.T) {
	if err := (*PostgreSQLConfig)(nil).Validate(); err == nil {
		t.Fatal("Expected error for nil config")
	}

	cfg := &PostgreSQLConfig{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Expected error for missing host/database")
	}

	cfg.Host = "localhost"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Expected error for missing database")
	}

	cfg.Database = "keystone"
	cfg.SSLMode = "not-valid"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Expected error for invalid sslmode")
	}

	cfg.SSLMode = "require"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Unexpected validation error: %v", err)
	}
}

func TestIsIPv6Address_Variants(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"2001:db8::1", true},
		{"[2001:db8::1]", true},
		{"fe80::1%eth0", true},
		{"127.0.0.1", false},
		{"example.com", false},
		{"2001:db8::1:5432", true},
	}

	for _, tt := range tests {
		if got := isIPv6Address(tt.addr); got != tt.want {
			t.Errorf("isIPv6Address(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func containsAll(s string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
