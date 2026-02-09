// Package opnsense provides OPNsense device adapters for proxy agents.
package opnsense

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/proxy"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

// Adapter implements VendorAdapter for OPNsense devices.
type Adapter struct {
	vendors.BaseVendorAdapter
	config     *Config
	httpClient *http.Client
	baseURL    string
}

// Config contains OPNsense specific configuration.
type Config struct {
	*vendors.VendorConfig
	// Port is the API port (default 443).
	Port int `json:"port,omitempty"`
	// TLS enables HTTPS (default true).
	TLS bool `json:"tls,omitempty"`
	// InsecureSkipVerify skips TLS certificate verification.
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`
}

// DefaultConfig returns a default OPNsense configuration.
func DefaultConfig() *Config {
	return &Config{
		VendorConfig: vendors.DefaultVendorConfig(),
		Port:         443,
		TLS:          true,
	}
}

// NewAdapter creates a new OPNsense adapter.
func NewAdapter(config *Config) *Adapter {
	if config == nil {
		config = DefaultConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	return &Adapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		config: config,
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *Adapter) Vendor() vendors.VendorType {
	return vendors.VendorOPNsense
}

// Type implements ProtocolAdapter.Type.
func (a *Adapter) Type() protocols.ProtocolType {
	return protocols.ProtocolREST
}

// Connect implements ProtocolAdapter.Connect.
func (a *Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred

	// Build base URL
	scheme := "https"
	if !a.config.TLS {
		scheme = "http"
	}
	port := a.config.Port
	if port == 0 {
		port = 443
	}
	a.baseURL = fmt.Sprintf("%s://%s:%d/api", scheme, device.Address, port)

	// Create HTTP client
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			//nolint:gosec // G402: InsecureSkipVerify is user-controlled via config for devices with self-signed certs
			InsecureSkipVerify: a.config.InsecureSkipVerify,
		},
	}
	a.httpClient = &http.Client{
		Timeout:   a.Config.Timeout,
		Transport: transport,
	}

	// Test connection
	_, err := a.request(ctx, "GET", "/core/firmware/status", nil)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *Adapter) Disconnect(ctx context.Context) error {
	a.Connected = false
	a.httpClient = nil
	return nil
}

// Execute implements ProtocolAdapter.Execute.
func (a *Adapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	start := time.Now()
	result := &protocols.ExecuteResult{
		StartTime: start,
	}

	// Parse command as API endpoint
	// Format: METHOD /endpoint [body]
	parts := strings.SplitN(req.Command, " ", 3)
	if len(parts) < 2 {
		result.Error = "invalid command format, expected: METHOD /endpoint [body]"
		result.ExitCode = 1
		return result, fmt.Errorf("%s", result.Error)
	}

	method := parts[0]
	endpoint := parts[1]
	var body []byte
	if len(parts) > 2 {
		body = []byte(parts[2])
	}

	respBody, err := a.request(ctx, method, endpoint, body)
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(start)

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
		return result, err
	}

	result.Stdout = respBody
	result.ExitCode = 0
	return result, nil
}

// HealthCheck implements ProtocolAdapter.HealthCheck.
func (a *Adapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
	}

	_, err := a.request(ctx, "GET", "/core/system/status", nil)
	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("unhealthy: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "healthy"
	return result, nil
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *Adapter) GetConfig(ctx context.Context, section string) (string, error) {
	endpoint := "/core/backup/download/this"
	if section != "" {
		// OPNsense doesn't support section-based config retrieval via API
		// Return specific section's settings instead
		switch section {
		case "firewall":
			endpoint = "/firewall/filter/searchRule"
		case "alias":
			endpoint = "/firewall/alias/searchItem"
		case "interface":
			endpoint = "/interfaces/overview/export"
		default:
			return "", fmt.Errorf("unsupported config section: %s", section)
		}
	}

	resp, err := a.request(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", err
	}
	return string(resp), nil
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *Adapter) SetConfig(ctx context.Context, commands []string) error {
	// OPNsense uses structured API calls rather than CLI commands
	// Each command should be a JSON object with endpoint and data
	for _, cmd := range commands {
		var cmdObj struct {
			Method   string          `json:"method"`
			Endpoint string          `json:"endpoint"`
			Data     json.RawMessage `json:"data"`
		}

		if err := json.Unmarshal([]byte(cmd), &cmdObj); err != nil {
			return fmt.Errorf("invalid command format: %w", err)
		}

		if _, err := a.request(ctx, cmdObj.Method, cmdObj.Endpoint, cmdObj.Data); err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
	}

	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *Adapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "OPNsense",
		OSType: "OPNsense",
		Raw:    make(map[string]string),
	}

	// Get system status
	statusResp, err := a.request(ctx, "GET", "/core/system/status", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get system status: %w", err)
	}
	facts.Raw["status"] = string(statusResp)

	var status SystemStatus
	if err := json.Unmarshal(statusResp, &status); err == nil {
		facts.Hostname = status.Name
		facts.Uptime = time.Duration(status.Uptime) * time.Second
	}

	// Get firmware info
	firmwareResp, err := a.request(ctx, "GET", "/core/firmware/status", nil)
	if err == nil {
		facts.Raw["firmware"] = string(firmwareResp)

		var firmware FirmwareStatus
		if err := json.Unmarshal(firmwareResp, &firmware); err == nil {
			facts.OSVersion = firmware.ProductVersion
		}
	}

	// Get interfaces
	ifResp, err := a.request(ctx, "GET", "/diagnostics/interface/getInterfaceStatistics", nil)
	if err == nil {
		facts.Raw["interfaces"] = string(ifResp)
		a.parseInterfaces(ifResp, facts)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *Adapter) SaveConfig(ctx context.Context) error {
	// OPNsense automatically saves config, but we can trigger a backup
	// This is a no-op as OPNsense doesn't have a separate save step
	return nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *Adapter) IsConnected() bool {
	return a.Connected
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *Adapter) Metrics() *protocols.AdapterMetrics {
	return &protocols.AdapterMetrics{}
}

// request makes an HTTP request to the OPNsense API.
func (a *Adapter) request(ctx context.Context, method, endpoint string, body []byte) ([]byte, error) {
	url := a.baseURL + endpoint

	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// OPNsense uses API key + secret as Basic auth
	// Key is username, Secret is password
	if basicCred, ok := a.Credential.(*credentials.RESTBasicCredential); ok {
		req.SetBasicAuth(basicCred.Username, basicCred.Password)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// parseInterfaces parses interface statistics response.
func (a *Adapter) parseInterfaces(data []byte, facts *vendors.DeviceFacts) {
	var ifStats map[string]InterfaceStats
	if err := json.Unmarshal(data, &ifStats); err != nil {
		return
	}

	for name := range ifStats {
		stats := ifStats[name]
		iface := vendors.InterfaceFact{
			Name:        name,
			Description: stats.Description,
			MacAddress:  stats.MacAddress,
			MTU:         stats.MTU,
		}

		if stats.Status == "up" || stats.Status == "active" {
			iface.OperStatus = "up"
			iface.AdminStatus = "up"
		} else {
			iface.OperStatus = "down"
			iface.AdminStatus = "down"
		}

		if len(stats.IPAddresses) > 0 {
			iface.IPAddresses = stats.IPAddresses
		}

		facts.Interfaces = append(facts.Interfaces, iface)
	}
}

// SystemStatus represents OPNsense system status.
type SystemStatus struct {
	Name      string `json:"name"`
	Uptime    int64  `json:"uptime"`
	DateTime  string `json:"datetime"`
	Kernel    string `json:"kernel"`
	CPU       string `json:"cpu"`
	CPUUsage  string `json:"cpu_usage"`
	MemTotal  string `json:"mem_total"`
	MemUsed   string `json:"mem_used"`
	DiskUsage string `json:"disk_usage"`
}

// FirmwareStatus represents OPNsense firmware status.
type FirmwareStatus struct {
	ProductVersion    string `json:"product_version"`
	ProductName       string `json:"product_name"`
	ProductArch       string `json:"product_arch"`
	ProductNickname   string `json:"product_nickname"`
	ProductHash       string `json:"product_hash"`
	ProductMirror     string `json:"product_mirror"`
	ProductRepos      string `json:"product_repos"`
	ProductTime       string `json:"product_time"`
	LastCheck         string `json:"last_check"`
	OSVersion         string `json:"os_version"`
	NeedsReboot       string `json:"needs_reboot"`
	UpgradeNeedReboot string `json:"upgrade_needs_reboot"`
}

// InterfaceStats represents interface statistics.
type InterfaceStats struct {
	Name        string   `json:"name"`
	Description string   `json:"descr"`
	MacAddress  string   `json:"macaddr"`
	Status      string   `json:"status"`
	MTU         int      `json:"mtu"`
	IPAddresses []string `json:"ipaddr"`
	Media       string   `json:"media"`
	MediaRaw    string   `json:"mediaraw"`
	BytesIn     int64    `json:"inbytes"`
	BytesOut    int64    `json:"outbytes"`
	PacketsIn   int64    `json:"inpkts"`
	PacketsOut  int64    `json:"outpkts"`
	ErrorsIn    int64    `json:"inerrs"`
	ErrorsOut   int64    `json:"outerrs"`
}

// Firewall operations

// Alias represents an OPNsense firewall alias.
type Alias struct {
	UUID        string `json:"uuid,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Content     string `json:"content"`
	Description string `json:"description,omitempty"`
	Enabled     string `json:"enabled,omitempty"`
}

// GetAlias retrieves a firewall alias by name.
func (a *Adapter) GetAlias(ctx context.Context, name string) (*Alias, error) {
	resp, err := a.request(ctx, "GET", "/firewall/alias/searchItem", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Rows []Alias `json:"rows"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}

	for _, alias := range result.Rows {
		if alias.Name == name {
			return &alias, nil
		}
	}

	return nil, nil
}

// CreateAlias creates a firewall alias.
func (a *Adapter) CreateAlias(ctx context.Context, alias *Alias) error {
	data := map[string]interface{}{
		"alias": map[string]string{
			"name":        alias.Name,
			"type":        alias.Type,
			"content":     alias.Content,
			"description": alias.Description,
			"enabled":     "1",
		},
	}

	body, _ := json.Marshal(data)
	_, err := a.request(ctx, "POST", "/firewall/alias/addItem", body)
	if err != nil {
		return err
	}

	// Apply changes
	_, err = a.request(ctx, "POST", "/firewall/alias/reconfigure", nil)
	return err
}

// DeleteAlias deletes a firewall alias by UUID.
func (a *Adapter) DeleteAlias(ctx context.Context, uuid string) error {
	endpoint := fmt.Sprintf("/firewall/alias/delItem/%s", uuid)
	_, err := a.request(ctx, "POST", endpoint, nil)
	if err != nil {
		return err
	}

	// Apply changes
	_, err = a.request(ctx, "POST", "/firewall/alias/reconfigure", nil)
	return err
}

// Rule represents an OPNsense firewall rule.
type Rule struct {
	UUID        string `json:"uuid,omitempty"`
	Sequence    int    `json:"sequence,omitempty"`
	Interface   string `json:"interface"`
	Direction   string `json:"direction"`
	Action      string `json:"action"`
	Protocol    string `json:"protocol"`
	Source      string `json:"source"`
	SourcePort  string `json:"source_port,omitempty"`
	Destination string `json:"destination"`
	DestPort    string `json:"destination_port,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     string `json:"enabled,omitempty"`
}

// GetRules retrieves firewall rules.
func (a *Adapter) GetRules(ctx context.Context) ([]Rule, error) {
	resp, err := a.request(ctx, "GET", "/firewall/filter/searchRule", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Rows []Rule `json:"rows"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}

	return result.Rows, nil
}

// CreateRule creates a firewall rule.
func (a *Adapter) CreateRule(ctx context.Context, rule *Rule) error {
	data := map[string]interface{}{
		"rule": map[string]interface{}{
			"interface":        rule.Interface,
			"direction":        rule.Direction,
			"action":           rule.Action,
			"protocol":         rule.Protocol,
			"source_net":       rule.Source,
			"source_port":      rule.SourcePort,
			"destination_net":  rule.Destination,
			"destination_port": rule.DestPort,
			"description":      rule.Description,
			"enabled":          "1",
		},
	}

	body, _ := json.Marshal(data)
	_, err := a.request(ctx, "POST", "/firewall/filter/addRule", body)
	if err != nil {
		return err
	}

	// Apply changes
	_, err = a.request(ctx, "POST", "/firewall/filter/apply", nil)
	return err
}

// DeleteRule deletes a firewall rule by UUID.
func (a *Adapter) DeleteRule(ctx context.Context, uuid string) error {
	endpoint := fmt.Sprintf("/firewall/filter/delRule/%s", uuid)
	_, err := a.request(ctx, "POST", endpoint, nil)
	if err != nil {
		return err
	}

	// Apply changes
	_, err = a.request(ctx, "POST", "/firewall/filter/apply", nil)
	return err
}

// Service operations

// ServiceControl controls a service (start, stop, restart).
func (a *Adapter) ServiceControl(ctx context.Context, service, action string) error {
	endpoint := fmt.Sprintf("/core/service/%s/%s", service, action)
	_, err := a.request(ctx, "POST", endpoint, nil)
	return err
}

// GetServiceStatus gets the status of a service.
func (a *Adapter) GetServiceStatus(ctx context.Context, service string) (bool, error) {
	endpoint := fmt.Sprintf("/core/service/%s/status", service)
	resp, err := a.request(ctx, "GET", endpoint, nil)
	if err != nil {
		return false, err
	}

	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(resp, &status); err != nil {
		return false, err
	}

	return status.Status == "running", nil
}

func init() {
	// Register the adapter factory with the default registry
	vendors.Register(vendors.VendorOPNsense, func(config *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		opnConfig := &Config{
			VendorConfig: config,
			Port:         443,
			TLS:          true,
		}
		return NewAdapter(opnConfig), nil
	})
}
