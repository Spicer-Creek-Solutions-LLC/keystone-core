package providers

import (
	"fmt"

	"github.com/libdns/route53"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// Route53Capabilities defines the capabilities of the AWS Route53 provider.
var Route53Capabilities = dns.ProviderCapabilities{
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
		dns.RecordTypeALIAS, // Route53 supports alias records
	},
	SupportsProxied:     false,
	MinTTL:              0, // Route53 allows TTL of 0
	MaxTTL:              2147483647,
	SupportsRootRecords: true,
	SupportsALIAS:       true, // Route53 supports ALIAS records natively
}

// NewRoute53Provider creates a new AWS Route53 DNS provider.
func NewRoute53Provider(creds dns.ResolvedCredentials) (dns.Provider, error) {
	accessKeyID := creds.Extra["access_key_id"]
	if accessKeyID == "" {
		accessKeyID = creds.APIKey
	}

	secretAccessKey := creds.Extra["secret_access_key"]
	if secretAccessKey == "" {
		secretAccessKey = creds.APIToken
	}

	if accessKeyID == "" || secretAccessKey == "" {
		return nil, fmt.Errorf("route53: access_key_id and secret_access_key are required")
	}

	region := creds.Extra["region"]
	if region == "" {
		region = "us-east-1" // Default region
	}

	provider := &route53.Provider{
		AccessKeyId:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Region:          region,
	}

	// Note: max_retries from creds.Extra is handled internally by the provider

	return NewLibdnsAdapter(provider, Route53Capabilities), nil
}

func init() {
	_ = dns.RegisterProvider("route53", NewRoute53Provider, Route53Capabilities) //nolint:errcheck // provider registration in init
}
