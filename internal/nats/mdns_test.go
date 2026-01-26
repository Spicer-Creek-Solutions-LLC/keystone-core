package nats

import (
	"net"
	"testing"

	"github.com/hashicorp/mdns"
)

func TestMDNSDiscovererAddEntry(t *testing.T) {
	config := DefaultMDNSDiscoveryConfig()
	config.DefaultScheme = SchemeTLS
	config.Domain = "local."

	discoverer, err := NewMDNSDiscoverer(config)
	if err != nil {
		t.Fatalf("NewMDNSDiscoverer() error = %v", err)
	}

	endpoints := make(map[string]*DiscoveredEndpoint)
	entry := &mdns.ServiceEntry{
		Name:   "nats._nats._tcp.local.",
		AddrV4: net.ParseIP("10.0.0.1"),
		Port:   4222,
	}

	discoverer.addMDNSEntry(endpoints, entry)

	endpoint := endpoints["10.0.0.1:4222"]
	if endpoint == nil {
		t.Fatal("expected endpoint to be added")
	}
	if endpoint.URL != "tls://10.0.0.1:4222" {
		t.Fatalf("URL = %q, want %q", endpoint.URL, "tls://10.0.0.1:4222")
	}
	if !endpoint.TLS {
		t.Fatal("expected TLS to be true for tls scheme")
	}
	if endpoint.Metadata["service"] != "_nats._tcp" {
		t.Fatalf("metadata service = %q, want %q", endpoint.Metadata["service"], "_nats._tcp")
	}
	if endpoint.Metadata["domain"] != "local." {
		t.Fatalf("metadata domain = %q, want %q", endpoint.Metadata["domain"], "local.")
	}
}

func TestMDNSDiscovererAddEntryMissingPort(t *testing.T) {
	config := DefaultMDNSDiscoveryConfig()
	discoverer, err := NewMDNSDiscoverer(config)
	if err != nil {
		t.Fatalf("NewMDNSDiscoverer() error = %v", err)
	}

	endpoints := make(map[string]*DiscoveredEndpoint)
	entry := &mdns.ServiceEntry{
		Name:   "nats._nats._tcp.local.",
		AddrV4: net.ParseIP("10.0.0.2"),
	}

	discoverer.addMDNSEntry(endpoints, entry)
	if len(endpoints) != 0 {
		t.Fatalf("expected no endpoints, got %d", len(endpoints))
	}
}
