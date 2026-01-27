package providers

import (
	"fmt"

	"github.com/libdns/googleclouddns"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// GoogleCloudDNSCapabilities defines the capabilities of the Google Cloud DNS provider.
var GoogleCloudDNSCapabilities = dns.ProviderCapabilities{
	SupportedRecordTypes: []dns.RecordType{
		dns.RecordTypeA,
		dns.RecordTypeAAAA,
		dns.RecordTypeCNAME,
		dns.RecordTypeTXT,
		dns.RecordTypeMX,
		dns.RecordTypeSRV,
		dns.RecordTypeCAA,
		dns.RecordTypeNS,
		dns.RecordTypePTR,
	},
	SupportsProxied:     false,
	MinTTL:              0, // GCP allows TTL of 0
	MaxTTL:              2147483647,
	SupportsRootRecords: true,
	SupportsALIAS:       false, // GCP doesn't support ALIAS records directly
}

// NewGoogleCloudDNSProvider creates a new Google Cloud DNS provider.
func NewGoogleCloudDNSProvider(creds dns.ResolvedCredentials) (dns.Provider, error) {
	project := creds.AccountID
	if project == "" {
		project = creds.Extra["project_id"]
	}
	if project == "" {
		project = creds.Extra["project"]
	}

	if project == "" {
		return nil, fmt.Errorf("gcp: project_id is required")
	}

	serviceAccountJSON := creds.Extra["service_account_json"]
	if serviceAccountJSON == "" {
		serviceAccountJSON = creds.Extra["credentials_json"]
	}

	provider := &googleclouddns.Provider{
		Project:            project,
		ServiceAccountJSON: serviceAccountJSON,
	}

	return NewLibdnsAdapter(provider, GoogleCloudDNSCapabilities), nil
}

func init() {
	dns.RegisterProvider("gcp", NewGoogleCloudDNSProvider, GoogleCloudDNSCapabilities)
	dns.RegisterProvider("googleclouddns", NewGoogleCloudDNSProvider, GoogleCloudDNSCapabilities)
}
