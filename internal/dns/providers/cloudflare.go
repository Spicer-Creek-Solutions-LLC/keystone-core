package providers

import (
	"fmt"

	"github.com/libdns/cloudflare"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// CloudflareCapabilities defines the capabilities of the Cloudflare provider.
var CloudflareCapabilities = dns.ProviderCapabilities{
	SupportedRecordTypes: []dns.RecordType{
		dns.RecordTypeA,
		dns.RecordTypeAAAA,
		dns.RecordTypeCNAME,
		dns.RecordTypeTXT,
		dns.RecordTypeMX,
		dns.RecordTypeSRV,
		dns.RecordTypeCAA,
		dns.RecordTypeNS,
	},
	SupportsProxied:     true,
	MinTTL:              60, // Cloudflare minimum TTL (1 minute, or "Auto" which is ~300)
	MaxTTL:              86400,
	SupportsRootRecords: true,
	SupportsALIAS:       false, // Cloudflare uses CNAME flattening at root instead
}

// NewCloudflareProvider creates a new Cloudflare DNS provider.
func NewCloudflareProvider(creds dns.ResolvedCredentials) (dns.Provider, error) {
	// Cloudflare supports API token (recommended) or API key + email
	apiToken := creds.APIToken
	if apiToken == "" {
		apiToken = creds.Extra["api_token"]
	}

	if apiToken == "" {
		return nil, fmt.Errorf("cloudflare: API token is required")
	}

	provider := &cloudflare.Provider{
		APIToken: apiToken,
	}

	return NewLibdnsAdapter(provider, CloudflareCapabilities), nil
}

func init() {
	_ = dns.RegisterProvider("cloudflare", NewCloudflareProvider, CloudflareCapabilities) //nolint:errcheck // provider registration in init
}
