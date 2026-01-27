package providers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/dns"
)

// DNSMadeEasyCapabilities defines the capabilities of the DNSMadeEasy provider.
var DNSMadeEasyCapabilities = dns.ProviderCapabilities{
	SupportedRecordTypes: []dns.RecordType{
		dns.RecordTypeA,
		dns.RecordTypeAAAA,
		dns.RecordTypeCNAME,
		dns.RecordTypeTXT,
		dns.RecordTypeMX,
		dns.RecordTypeSRV,
		dns.RecordTypeNS,
		dns.RecordTypePTR,
	},
	SupportsProxied:     false,
	MinTTL:              30,
	MaxTTL:              604800, // 7 days
	SupportsRootRecords: true,
	SupportsALIAS:       false,
}

const dnsMadeEasyAPIBase = "https://api.dnsmadeeasy.com/V2.0"

// DNSMadeEasyProvider implements dns.Provider for DNSMadeEasy.
type DNSMadeEasyProvider struct {
	apiKey    string
	secretKey string
	client    *http.Client
}

// NewDNSMadeEasyProvider creates a new DNSMadeEasy DNS provider.
func NewDNSMadeEasyProvider(creds dns.ResolvedCredentials) (dns.Provider, error) {
	apiKey := creds.APIKey
	if apiKey == "" {
		apiKey = creds.Extra["api_key"]
	}

	secretKey := creds.APIToken
	if secretKey == "" {
		secretKey = creds.Extra["secret_key"]
	}

	if apiKey == "" {
		return nil, fmt.Errorf("dnsmadeeasy: api_key is required")
	}
	if secretKey == "" {
		return nil, fmt.Errorf("dnsmadeeasy: secret_key is required")
	}

	return &DNSMadeEasyProvider{
		apiKey:    apiKey,
		secretKey: secretKey,
		client:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// dmeRecord represents a DNSMadeEasy record response.
type dmeRecord struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Value       string `json:"value"`
	TTL         int    `json:"ttl"`
	GtdLocation string `json:"gtdLocation,omitempty"`
	MxLevel     int    `json:"mxLevel,omitempty"`
	Weight      int    `json:"weight,omitempty"`
	Priority    int    `json:"priority,omitempty"`
	Port        int    `json:"port,omitempty"`
}

// dmeRecordsResponse represents the DNSMadeEasy records list response.
type dmeRecordsResponse struct {
	Data []dmeRecord `json:"data"`
}

// dmeDomain represents a DNSMadeEasy domain response.
type dmeDomain struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// dmeDomainsResponse represents the DNSMadeEasy domains list response.
type dmeDomainsResponse struct {
	Data []dmeDomain `json:"data"`
}

// GetRecords retrieves all DNS records for a zone.
func (p *DNSMadeEasyProvider) GetRecords(ctx context.Context, zone string) ([]dns.Record, error) {
	zone = strings.TrimSuffix(zone, ".")

	// First get the domain ID
	domainID, err := p.getDomainID(ctx, zone)
	if err != nil {
		return nil, err
	}

	// Get records for the domain
	url := fmt.Sprintf("%s/dns/managed/%d/records", dnsMadeEasyAPIBase, domainID)
	resp, err := p.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("get records: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get records failed: %s - %s", resp.Status, string(body))
	}

	var recordsResp dmeRecordsResponse
	if err := json.NewDecoder(resp.Body).Decode(&recordsResp); err != nil {
		return nil, fmt.Errorf("decode records: %w", err)
	}

	records := make([]dns.Record, 0, len(recordsResp.Data))
	for _, r := range recordsResp.Data {
		records = append(records, p.toRecord(r))
	}

	return records, nil
}

// CreateRecord creates a new DNS record.
func (p *DNSMadeEasyProvider) CreateRecord(ctx context.Context, zone string, record dns.Record) (*dns.Record, error) {
	zone = strings.TrimSuffix(zone, ".")

	domainID, err := p.getDomainID(ctx, zone)
	if err != nil {
		return nil, err
	}

	dmeRec := p.fromRecord(record)
	body, err := json.Marshal(dmeRec)
	if err != nil {
		return nil, fmt.Errorf("marshal record: %w", err)
	}

	url := fmt.Sprintf("%s/dns/managed/%d/records", dnsMadeEasyAPIBase, domainID)
	resp, err := p.doRequest(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("create record: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create record failed: %s - %s", resp.Status, string(respBody))
	}

	var created dmeRecord
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decode created record: %w", err)
	}

	result := p.toRecord(created)
	return &result, nil
}

// UpdateRecord updates an existing DNS record.
func (p *DNSMadeEasyProvider) UpdateRecord(ctx context.Context, zone string, record dns.Record) (*dns.Record, error) {
	zone = strings.TrimSuffix(zone, ".")

	domainID, err := p.getDomainID(ctx, zone)
	if err != nil {
		return nil, err
	}

	// Get record ID from our record ID field
	recordID, err := strconv.Atoi(record.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid record ID: %s", record.ID)
	}

	dmeRec := p.fromRecord(record)
	dmeRec.ID = recordID
	body, err := json.Marshal(dmeRec)
	if err != nil {
		return nil, fmt.Errorf("marshal record: %w", err)
	}

	url := fmt.Sprintf("%s/dns/managed/%d/records/%d", dnsMadeEasyAPIBase, domainID, recordID)
	resp, err := p.doRequest(ctx, "PUT", url, body)
	if err != nil {
		return nil, fmt.Errorf("update record: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update record failed: %s - %s", resp.Status, string(respBody))
	}

	// DNSMadeEasy returns empty body on update, return our input
	result := record
	return &result, nil
}

// DeleteRecord deletes a DNS record.
func (p *DNSMadeEasyProvider) DeleteRecord(ctx context.Context, zone string, record dns.Record) error {
	zone = strings.TrimSuffix(zone, ".")

	domainID, err := p.getDomainID(ctx, zone)
	if err != nil {
		return err
	}

	recordID, err := strconv.Atoi(record.ID)
	if err != nil {
		return fmt.Errorf("invalid record ID: %s", record.ID)
	}

	url := fmt.Sprintf("%s/dns/managed/%d/records/%d", dnsMadeEasyAPIBase, domainID, recordID)
	resp, err := p.doRequest(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete record failed: %s - %s", resp.Status, string(body))
	}

	return nil
}

// Capabilities returns the provider's capabilities.
func (p *DNSMadeEasyProvider) Capabilities() dns.ProviderCapabilities {
	return DNSMadeEasyCapabilities
}

// getDomainID retrieves the domain ID for a zone name.
func (p *DNSMadeEasyProvider) getDomainID(ctx context.Context, zone string) (int, error) {
	url := fmt.Sprintf("%s/dns/managed/name?domainname=%s", dnsMadeEasyAPIBase, zone)
	resp, err := p.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("get domain: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("get domain failed: %s - %s", resp.Status, string(body))
	}

	var domain dmeDomain
	if err := json.NewDecoder(resp.Body).Decode(&domain); err != nil {
		return 0, fmt.Errorf("decode domain: %w", err)
	}

	return domain.ID, nil
}

// doRequest performs an authenticated HTTP request to DNSMadeEasy.
func (p *DNSMadeEasyProvider) doRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	// DNSMadeEasy authentication
	now := time.Now().UTC().Format(time.RFC1123)
	hash := hmac.New(sha1.New, []byte(p.secretKey))
	hash.Write([]byte(now))
	signature := hex.EncodeToString(hash.Sum(nil))

	req.Header.Set("x-dnsme-apiKey", p.apiKey)
	req.Header.Set("x-dnsme-requestDate", now)
	req.Header.Set("x-dnsme-hmac", signature)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return p.client.Do(req)
}

// toRecord converts a DNSMadeEasy record to our Record type.
func (p *DNSMadeEasyProvider) toRecord(r dmeRecord) dns.Record {
	record := dns.Record{
		ID:    strconv.Itoa(r.ID),
		Type:  dns.RecordType(r.Type),
		Name:  r.Name,
		Value: r.Value,
		TTL:   r.TTL,
	}

	switch record.Type {
	case dns.RecordTypeMX:
		record.Priority = r.MxLevel
	case dns.RecordTypeSRV:
		record.Priority = r.Priority
		record.Weight = r.Weight
		record.Port = r.Port
	}

	return record
}

// fromRecord converts our Record type to a DNSMadeEasy record.
func (p *DNSMadeEasyProvider) fromRecord(r dns.Record) dmeRecord {
	dmeRec := dmeRecord{
		Name:        r.Name,
		Type:        string(r.Type),
		Value:       r.Value,
		TTL:         r.TTL,
		GtdLocation: "DEFAULT",
	}

	switch r.Type {
	case dns.RecordTypeMX:
		dmeRec.MxLevel = r.Priority
	case dns.RecordTypeSRV:
		dmeRec.Priority = r.Priority
		dmeRec.Weight = r.Weight
		dmeRec.Port = r.Port
	}

	return dmeRec
}

// Verify DNSMadeEasyProvider implements dns.Provider
var _ dns.Provider = (*DNSMadeEasyProvider)(nil)

func init() {
	dns.RegisterProvider("dnsmadeeasy", NewDNSMadeEasyProvider, DNSMadeEasyCapabilities)
}
