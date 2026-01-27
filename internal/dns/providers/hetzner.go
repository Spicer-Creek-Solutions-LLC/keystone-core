package providers

import (
	"fmt"

	"github.com/libdns/hetzner"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// HetznerCapabilities defines the capabilities of the Hetzner DNS provider.
var HetznerCapabilities = dns.ProviderCapabilities{
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
	SupportsProxied:     false,
	MinTTL:              60,
	MaxTTL:              86400,
	SupportsRootRecords: true,
	SupportsALIAS:       false,
}

// NewHetznerProvider creates a new Hetzner DNS provider.
func NewHetznerProvider(creds dns.ResolvedCredentials) (dns.Provider, error) {
	authAPIToken := creds.APIToken
	if authAPIToken == "" {
		authAPIToken = creds.Extra["api_token"]
	}

	if authAPIToken == "" {
		return nil, fmt.Errorf("hetzner: API token is required")
	}

	provider := &hetzner.Provider{
		AuthAPIToken: authAPIToken,
	}

	return NewLibdnsAdapter(provider, HetznerCapabilities), nil
}

func init() {
	dns.RegisterProvider("hetzner", NewHetznerProvider, HetznerCapabilities)
}
