package providers

import (
	"fmt"

	"github.com/libdns/digitalocean"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// DigitalOceanCapabilities defines the capabilities of the DigitalOcean provider.
var DigitalOceanCapabilities = dns.ProviderCapabilities{
	SupportedRecordTypes: []dns.RecordType{
		dns.RecordTypeA,
		dns.RecordTypeAAAA,
		dns.RecordTypeCNAME,
		dns.RecordTypeTXT,
		dns.RecordTypeMX,
		dns.RecordTypeSRV,
		dns.RecordTypeNS,
	},
	SupportsProxied:     false,
	MinTTL:              30, // DigitalOcean minimum TTL
	MaxTTL:              86400,
	SupportsRootRecords: true,
	SupportsALIAS:       false,
}

// NewDigitalOceanProvider creates a new DigitalOcean DNS provider.
func NewDigitalOceanProvider(creds dns.ResolvedCredentials) (dns.Provider, error) {
	apiToken := creds.APIToken
	if apiToken == "" {
		apiToken = creds.Extra["api_token"]
	}

	if apiToken == "" {
		return nil, fmt.Errorf("digitalocean: API token is required")
	}

	provider := &digitalocean.Provider{
		APIToken: apiToken,
	}

	return NewLibdnsAdapter(provider, DigitalOceanCapabilities), nil
}

func init() {
	_ = dns.RegisterProvider("digitalocean", NewDigitalOceanProvider, DigitalOceanCapabilities) //nolint:errcheck // provider registration in init
}
