// Package providers implements DNS provider integrations for Keystone Core.
//
// This package provides implementations for various DNS providers using the
// libdns interface (https://github.com/libdns/libdns). Each provider is
// automatically registered with the dns.DefaultRegistry when the package
// is imported.
//
// # Supported Providers
//
// The following providers are currently supported:
//
//   - cloudflare: Cloudflare DNS
//   - route53: Amazon Route 53
//   - gcp/googleclouddns: Google Cloud DNS
//   - azure: Microsoft Azure DNS
//   - digitalocean: DigitalOcean DNS
//   - dnsmadeeasy: DNSMadeEasy
//   - hetzner: Hetzner DNS
//
// # Usage
//
// Import this package to register all providers:
//
//	import _ "github.com/shawnbutts/keystone-core/internal/dns/providers"
//
// Then use the dns.DefaultRegistry to create providers:
//
//	provider, err := dns.DefaultRegistry.CreateProvider("cloudflare", creds)
//
// # Adding New Providers
//
// To add a new provider:
//
//  1. Create a new file (e.g., myprovider.go)
//  2. Import the libdns provider package
//  3. Define the provider capabilities
//  4. Create a factory function that returns a LibdnsAdapter
//  5. Register the provider in init()
//
// Example:
//
//	func init() {
//	    dns.RegisterProvider("myprovider", NewMyProvider, MyProviderCapabilities)
//	}
package providers
