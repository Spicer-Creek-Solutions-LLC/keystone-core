// Package dns provides DNS record management via provider APIs.
package dns

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

// RecordType represents a DNS record type.
type RecordType string

// RecordTypeA constants define the supported types.
const (
	RecordTypeA     RecordType = "A"
	RecordTypeAAAA  RecordType = "AAAA"
	RecordTypeCNAME RecordType = "CNAME"
	RecordTypeTXT   RecordType = "TXT"
	RecordTypeMX    RecordType = "MX"
	RecordTypeSRV   RecordType = "SRV"
	RecordTypeCAA   RecordType = "CAA"
	RecordTypeNS    RecordType = "NS"
	RecordTypeALIAS RecordType = "ALIAS"
	RecordTypePTR   RecordType = "PTR"
)

// AllRecordTypes returns all supported record types.
func AllRecordTypes() []RecordType {
	return []RecordType{
		RecordTypeA,
		RecordTypeAAAA,
		RecordTypeCNAME,
		RecordTypeTXT,
		RecordTypeMX,
		RecordTypeSRV,
		RecordTypeCAA,
		RecordTypeNS,
		RecordTypeALIAS,
		RecordTypePTR,
	}
}

// IsValid checks if the record type is valid.
func (t RecordType) IsValid() bool {
	for _, rt := range AllRecordTypes() {
		if t == rt {
			return true
		}
	}
	return false
}

// Record represents a DNS record.
type Record struct {
	// Type is the DNS record type (A, AAAA, CNAME, etc.)
	Type RecordType `json:"type" yaml:"type"`

	// Name is the record name (subdomain or @ for apex)
	Name string `json:"name" yaml:"name"`

	// Value is the record value (IP address, hostname, etc.)
	Value string `json:"value" yaml:"value"`

	// TTL is the time-to-live in seconds
	TTL int `json:"ttl" yaml:"ttl"`

	// Priority is used for MX and SRV records
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`

	// Weight is used for SRV records
	Weight int `json:"weight,omitempty" yaml:"weight,omitempty"`

	// Port is used for SRV records
	Port int `json:"port,omitempty" yaml:"port,omitempty"`

	// Proxied indicates if the record should be proxied (Cloudflare-specific)
	Proxied *bool `json:"proxied,omitempty" yaml:"proxied,omitempty"`

	// ID is the provider-specific record ID (set after creation)
	ID string `json:"id,omitempty" yaml:"id,omitempty"`
}

// Key returns a unique key for the record within a zone.
func (r *Record) Key() string {
	return fmt.Sprintf("%s:%s:%s", r.Type, r.Name, r.Value)
}

// Validate validates the record.
func (r *Record) Validate() error {
	if !r.Type.IsValid() {
		return fmt.Errorf("invalid record type: %s", r.Type)
	}

	if r.Name == "" {
		return fmt.Errorf("record name is required")
	}

	if r.Value == "" {
		return fmt.Errorf("record value is required")
	}

	if r.TTL < 0 {
		return fmt.Errorf("TTL must be non-negative")
	}

	// Type-specific validation
	switch r.Type {
	case RecordTypeA:
		if ip := net.ParseIP(r.Value); ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid IPv4 address for A record: %s", r.Value)
		}
	case RecordTypeAAAA:
		if ip := net.ParseIP(r.Value); ip == nil || ip.To4() != nil {
			return fmt.Errorf("invalid IPv6 address for AAAA record: %s", r.Value)
		}
	case RecordTypeMX:
		if r.Priority < 0 || r.Priority > 65535 {
			return fmt.Errorf("MX priority must be between 0 and 65535")
		}
	case RecordTypeSRV:
		if r.Priority < 0 || r.Priority > 65535 {
			return fmt.Errorf("SRV priority must be between 0 and 65535")
		}
		if r.Weight < 0 || r.Weight > 65535 {
			return fmt.Errorf("SRV weight must be between 0 and 65535")
		}
		if r.Port < 0 || r.Port > 65535 {
			return fmt.Errorf("SRV port must be between 0 and 65535")
		}
	default:
		// Other record types have no type-specific validation
	}

	return nil
}

// Normalize normalizes the record for comparison.
func (r *Record) Normalize(zone string) *Record {
	normalized := *r

	// Normalize name
	normalized.Name = NormalizeName(r.Name, zone)

	// Normalize value for CNAME, MX, NS, SRV (should be FQDN)
	switch r.Type {
	case RecordTypeCNAME, RecordTypeMX, RecordTypeNS, RecordTypeSRV, RecordTypeALIAS:
		normalized.Value = NormalizeFQDN(r.Value)
	default:
		// Other record types don't require value normalization
	}

	// Default TTL if not specified
	if normalized.TTL == 0 {
		normalized.TTL = 300 // 5 minutes default
	}

	return &normalized
}

// Equal checks if two records are equal (ignoring ID).
func (r *Record) Equal(other *Record) bool {
	if r.Type != other.Type {
		return false
	}
	if r.Name != other.Name {
		return false
	}
	if r.Value != other.Value {
		return false
	}
	if r.TTL != other.TTL {
		return false
	}
	if r.Priority != other.Priority {
		return false
	}
	if r.Weight != other.Weight {
		return false
	}
	if r.Port != other.Port {
		return false
	}
	return true
}

// Zone represents a DNS zone configuration.
type Zone struct {
	// Name is the zone name (e.g., "example.com")
	Name string `json:"name" yaml:"name"`

	// Provider is the DNS provider name
	Provider string `json:"provider" yaml:"provider"`

	// Credentials contains provider credentials
	Credentials Credentials `json:"credentials" yaml:"credentials"`

	// Records is the list of desired DNS records
	Records []Record `json:"records" yaml:"records"`
}

// Validate validates the zone configuration.
func (z *Zone) Validate() error {
	if z.Name == "" {
		return fmt.Errorf("zone name is required")
	}

	if !IsValidZone(z.Name) {
		return fmt.Errorf("invalid zone name: %s", z.Name)
	}

	if z.Provider == "" {
		return fmt.Errorf("provider is required")
	}

	for i, record := range z.Records {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("record[%d]: %w", i, err)
		}
	}

	return nil
}

// NormalizeRecords normalizes all records in the zone.
func (z *Zone) NormalizeRecords() []Record {
	normalized := make([]Record, len(z.Records))
	for i, record := range z.Records {
		normalized[i] = *record.Normalize(z.Name)
	}
	return normalized
}

// Credentials represents provider credentials.
type Credentials struct {
	// SecretRef is a reference to a secret (e.g., "secret://dns/cloudflare")
	SecretRef string `json:"secret_ref,omitempty" yaml:"secret_ref,omitempty"`

	// APIKey is a direct API key (not recommended for production)
	APIKey string `json:"api_key,omitempty" yaml:"api_key,omitempty"`

	// APIToken is a direct API token
	APIToken string `json:"api_token,omitempty" yaml:"api_token,omitempty"`

	// Additional provider-specific fields
	Extra map[string]string `json:"extra,omitempty" yaml:"extra,omitempty"`
}

// IsEmpty checks if credentials are empty.
func (c *Credentials) IsEmpty() bool {
	return c.SecretRef == "" && c.APIKey == "" && c.APIToken == "" && len(c.Extra) == 0
}

// ProviderCapabilities describes what a DNS provider supports.
type ProviderCapabilities struct {
	// SupportedRecordTypes lists record types the provider supports
	SupportedRecordTypes []RecordType

	// SupportsProxied indicates if the provider supports proxied records
	SupportsProxied bool

	// MinTTL is the minimum TTL supported
	MinTTL int

	// MaxTTL is the maximum TTL supported (0 = unlimited)
	MaxTTL int

	// SupportsRootRecords indicates if apex/root records are supported
	SupportsRootRecords bool

	// SupportsALIAS indicates if ALIAS/ANAME records are supported
	SupportsALIAS bool

	// RateLimitPerSecond is the API rate limit
	RateLimitPerSecond int
}

// SupportsRecordType checks if the provider supports a record type.
func (c *ProviderCapabilities) SupportsRecordType(t RecordType) bool {
	for _, rt := range c.SupportedRecordTypes {
		if t == rt {
			return true
		}
	}
	return false
}

// NormalizeName normalizes a record name relative to a zone.
func NormalizeName(name, zone string) string {
	name = strings.TrimSuffix(name, ".")
	name = strings.ToLower(name)

	// Handle apex records
	if name == "@" || name == "" || name == zone {
		return "@"
	}

	// Remove zone suffix if present
	zoneSuffix := "." + zone
	name = strings.TrimSuffix(name, zoneSuffix)

	return name
}

// NormalizeFQDN ensures a hostname is a fully qualified domain name.
func NormalizeFQDN(name string) string {
	name = strings.ToLower(name)
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	return name
}

// IsValidZone checks if a zone name is valid.
func IsValidZone(zone string) bool {
	zone = strings.TrimSuffix(zone, ".")
	if zone == "" {
		return false
	}

	// Basic DNS name validation
	dnsNameRegex := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	return dnsNameRegex.MatchString(zone)
}

// RecordSet represents a set of records with the same name and type.
type RecordSet struct {
	Name    string
	Type    RecordType
	Records []Record
}

// SyncResult represents the result of a sync operation.
type SyncResult struct {
	// Zone is the zone that was synced
	Zone string

	// Created is the number of records created
	Created int

	// Updated is the number of records updated
	Updated int

	// Deleted is the number of records deleted
	Deleted int

	// Unchanged is the number of records unchanged
	Unchanged int

	// Errors contains any errors that occurred
	Errors []error

	// Duration is how long the sync took
	Duration time.Duration

	// Changes contains detailed change information
	Changes []RecordChange
}

// HasErrors returns true if there were any errors.
func (r *SyncResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// TotalChanges returns the total number of changes made.
func (r *SyncResult) TotalChanges() int {
	return r.Created + r.Updated + r.Deleted
}
