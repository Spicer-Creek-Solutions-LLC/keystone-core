package providers

import (
	"fmt"

	"github.com/libdns/azure"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// AzureDNSCapabilities defines the capabilities of the Azure DNS provider.
var AzureDNSCapabilities = dns.ProviderCapabilities{
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
	MinTTL:              1, // Azure minimum TTL is 1 second
	MaxTTL:              2147483647,
	SupportsRootRecords: true,
	SupportsALIAS:       false, // Azure uses Alias Record Sets differently
}

// NewAzureDNSProvider creates a new Azure DNS provider.
func NewAzureDNSProvider(creds dns.ResolvedCredentials) (dns.Provider, error) {
	subscriptionID := creds.Extra["subscription_id"]
	resourceGroup := creds.Extra["resource_group"]
	tenantID := creds.Extra["tenant_id"]
	clientID := creds.Extra["client_id"]
	clientSecret := creds.Extra["client_secret"]

	if subscriptionID == "" {
		return nil, fmt.Errorf("azure: subscription_id is required")
	}
	if resourceGroup == "" {
		return nil, fmt.Errorf("azure: resource_group is required")
	}
	if tenantID == "" {
		return nil, fmt.Errorf("azure: tenant_id is required")
	}
	if clientID == "" {
		return nil, fmt.Errorf("azure: client_id is required")
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("azure: client_secret is required")
	}

	provider := &azure.Provider{
		SubscriptionId:    subscriptionID,
		ResourceGroupName: resourceGroup,
		TenantId:          tenantID,
		ClientId:          clientID,
		ClientSecret:      clientSecret,
	}

	return NewLibdnsAdapter(provider, AzureDNSCapabilities), nil
}

func init() {
	_ = dns.RegisterProvider("azure", NewAzureDNSProvider, AzureDNSCapabilities) //nolint:errcheck // provider registration in init
}
