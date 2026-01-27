// Package providers implements DNS provider integrations using the libdns interface.
package providers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/libdns/libdns"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// LibdnsProvider is the interface that libdns providers implement.
// This matches the libdns provider interfaces (RecordGetter, RecordAppender, RecordSetter, RecordDeleter).
type LibdnsProvider interface {
	libdns.RecordGetter
	libdns.RecordAppender
	libdns.RecordSetter
	libdns.RecordDeleter
}

// LibdnsAdapter wraps a libdns provider to implement our dns.Provider interface.
type LibdnsAdapter struct {
	provider LibdnsProvider
	caps     dns.ProviderCapabilities
}

// NewLibdnsAdapter creates a new adapter for a libdns provider.
func NewLibdnsAdapter(provider LibdnsProvider, caps dns.ProviderCapabilities) *LibdnsAdapter {
	return &LibdnsAdapter{
		provider: provider,
		caps:     caps,
	}
}

// GetRecords retrieves all DNS records for a zone.
func (a *LibdnsAdapter) GetRecords(ctx context.Context, zone string) ([]dns.Record, error) {
	zone = normalizeZone(zone)

	records, err := a.provider.GetRecords(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("libdns GetRecords: %w", err)
	}

	result := make([]dns.Record, 0, len(records))
	for _, r := range records {
		record, err := fromLibdnsRecord(r, zone)
		if err != nil {
			// Skip records we can't parse
			continue
		}
		result = append(result, record)
	}

	return result, nil
}

// CreateRecord creates a new DNS record.
func (a *LibdnsAdapter) CreateRecord(ctx context.Context, zone string, record dns.Record) (*dns.Record, error) {
	zone = normalizeZone(zone)

	libRecord := toLibdnsRecord(record)
	created, err := a.provider.AppendRecords(ctx, zone, []libdns.Record{libRecord})
	if err != nil {
		return nil, fmt.Errorf("libdns AppendRecords: %w", err)
	}

	if len(created) == 0 {
		return nil, fmt.Errorf("no record returned from provider")
	}

	result, err := fromLibdnsRecord(created[0], zone)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateRecord updates an existing DNS record.
func (a *LibdnsAdapter) UpdateRecord(ctx context.Context, zone string, record dns.Record) (*dns.Record, error) {
	zone = normalizeZone(zone)

	libRecord := toLibdnsRecord(record)
	updated, err := a.provider.SetRecords(ctx, zone, []libdns.Record{libRecord})
	if err != nil {
		return nil, fmt.Errorf("libdns SetRecords: %w", err)
	}

	if len(updated) == 0 {
		return nil, fmt.Errorf("no record returned from provider")
	}

	result, err := fromLibdnsRecord(updated[0], zone)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteRecord deletes a DNS record.
func (a *LibdnsAdapter) DeleteRecord(ctx context.Context, zone string, record dns.Record) error {
	zone = normalizeZone(zone)

	libRecord := toLibdnsRecord(record)
	_, err := a.provider.DeleteRecords(ctx, zone, []libdns.Record{libRecord})
	if err != nil {
		return fmt.Errorf("libdns DeleteRecords: %w", err)
	}

	return nil
}

// Capabilities returns the provider's capabilities.
func (a *LibdnsAdapter) Capabilities() dns.ProviderCapabilities {
	return a.caps
}

// toLibdnsRecord converts our Record to a libdns.RR.
func toLibdnsRecord(r dns.Record) libdns.RR {
	data := formatLibdnsValue(r)

	return libdns.RR{
		Name: r.Name,
		Type: string(r.Type),
		TTL:  time.Duration(r.TTL) * time.Second,
		Data: data,
	}
}

// formatLibdnsValue formats the record value for libdns Data field.
// For MX/SRV records, includes priority/weight/port in the value.
func formatLibdnsValue(r dns.Record) string {
	switch r.Type {
	case dns.RecordTypeMX:
		// MX format: priority target
		return fmt.Sprintf("%d %s", r.Priority, r.Value)
	case dns.RecordTypeSRV:
		// SRV format: priority weight port target
		return fmt.Sprintf("%d %d %d %s", r.Priority, r.Weight, r.Port, r.Value)
	default:
		return r.Value
	}
}

// fromLibdnsRecord converts a libdns.Record to our Record type.
func fromLibdnsRecord(r libdns.Record, zone string) (dns.Record, error) {
	rr := r.RR()
	recordType := dns.RecordType(rr.Type)

	record := dns.Record{
		Type: recordType,
		Name: rr.Name,
		TTL:  int(rr.TTL.Seconds()),
	}

	// Parse the data field based on record type
	switch recordType {
	case dns.RecordTypeMX:
		// MX format: priority target
		parts := strings.Fields(rr.Data)
		if len(parts) >= 2 {
			if priority, err := strconv.Atoi(parts[0]); err == nil {
				record.Priority = priority
			}
			record.Value = parts[1]
		} else {
			record.Value = rr.Data
		}
	case dns.RecordTypeSRV:
		// SRV format: priority weight port target
		parts := strings.Fields(rr.Data)
		if len(parts) >= 4 {
			if priority, err := strconv.Atoi(parts[0]); err == nil {
				record.Priority = priority
			}
			if weight, err := strconv.Atoi(parts[1]); err == nil {
				record.Weight = weight
			}
			if port, err := strconv.Atoi(parts[2]); err == nil {
				record.Port = port
			}
			record.Value = parts[3]
		} else {
			record.Value = rr.Data
		}
	default:
		record.Value = rr.Data
	}

	return record, nil
}

// normalizeZone ensures zone has a trailing dot.
func normalizeZone(zone string) string {
	if !strings.HasSuffix(zone, ".") {
		return zone + "."
	}
	return zone
}

// Verify LibdnsAdapter implements dns.Provider
var _ dns.Provider = (*LibdnsAdapter)(nil)
