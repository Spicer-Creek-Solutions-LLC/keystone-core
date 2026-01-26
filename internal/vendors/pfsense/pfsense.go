// Package pfsense provides pfSense device adapters for proxy agents.
package pfsense

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

// Adapter implements VendorAdapter for pfSense devices.
// Uses the pfSense REST API package (https://github.com/jaredhendrickson13/pfsense-api).
type Adapter struct {
	vendors.BaseVendorAdapter
	config     *Config
	httpClient *http.Client
	baseURL    string
	token      string
}

// Config contains pfSense specific configuration.
type Config struct {
	*vendors.VendorConfig
	// Port is the API port (default 443).
	Port int `json:"port,omitempty"`
	// TLS enables HTTPS (default true).
	TLS bool `json:"tls,omitempty"`
	// InsecureSkipVerify skips TLS certificate verification.
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`
	// APIVersion is the API version (default "v1").
	APIVersion string `json:"api_version,omitempty"`
}

// DefaultConfig returns a default pfSense configuration.
func DefaultConfig() *Config {
	return &Config{
		VendorConfig: vendors.DefaultVendorConfig(),
		Port:         443,
		TLS:          true,
		APIVersion:   "v1",
	}
}

// NewAdapter creates a new pfSense adapter.
func NewAdapter(config *Config) *Adapter {
	if config == nil {
		config = DefaultConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}
	if config.APIVersion == "" {
		config.APIVersion = "v1"
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
	return vendors.VendorPfSense
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
	a.baseURL = fmt.Sprintf("%s://%s:%d/api/%s", scheme, device.Address, port, a.config.APIVersion)

	// Create HTTP client
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: a.config.InsecureSkipVerify,
		},
	}
	a.httpClient = &http.Client{
		Timeout:   a.Config.Timeout,
		Transport: transport,
	}

	// Store token if using bearer auth
	if bearerCred, ok := cred.(*credentials.RESTBearerCredential); ok {
		a.token = bearerCred.Token
	}

	// Test connection
	_, err := a.request(ctx, "GET", "/status/system", nil)
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
	a.token = ""
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

	_, err := a.request(ctx, "GET", "/status/system", nil)
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
	endpoint := "/diagnostics/config_history"
	if section != "" {
		switch section {
		case "firewall":
			endpoint = "/firewall/rule"
		case "alias":
			endpoint = "/firewall/alias"
		case "interface":
			endpoint = "/interface"
		case "nat":
			endpoint = "/firewall/nat/port_forward"
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
		Vendor: "Netgate",
		OSType: "pfSense",
		Raw:    make(map[string]string),
	}

	// Get system status
	statusResp, err := a.request(ctx, "GET", "/status/system", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get system status: %w", err)
	}
	facts.Raw["status"] = string(statusResp)

	var status APIResponse
	if err := json.Unmarshal(statusResp, &status); err == nil && status.Data != nil {
		if sysStatus, ok := status.Data.(map[string]interface{}); ok {
			if hostname, ok := sysStatus["hostname"].(string); ok {
				facts.Hostname = hostname
			}
			if version, ok := sysStatus["system_version"].(string); ok {
				facts.OSVersion = version
			}
			if platform, ok := sysStatus["system_platform"].(string); ok {
				facts.Model = platform
			}
			if uptime, ok := sysStatus["uptime"].(string); ok {
				facts.Raw["uptime"] = uptime
			}
		}
	}

	// Get interfaces
	ifResp, err := a.request(ctx, "GET", "/status/interface", nil)
	if err == nil {
		facts.Raw["interfaces"] = string(ifResp)
		a.parseInterfaces(ifResp, facts)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *Adapter) SaveConfig(ctx context.Context) error {
	// pfSense automatically saves config, no separate save step needed
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

// APIResponse represents a pfSense API response.
type APIResponse struct {
	Code    int         `json:"code"`
	Status  string      `json:"status"`
	Return  int         `json:"return"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// request makes an HTTP request to the pfSense API.
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

	// Set authentication
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	} else if basicCred, ok := a.Credential.(*credentials.RESTBasicCredential); ok {
		// pfSense API also supports basic auth with client ID/token
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

	// Parse response to check for API errors
	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err == nil {
		if apiResp.Code != 200 && apiResp.Code != 0 {
			return nil, fmt.Errorf("API error %d: %s", apiResp.Code, apiResp.Message)
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// parseInterfaces parses interface status response.
func (a *Adapter) parseInterfaces(data []byte, facts *vendors.DeviceFacts) {
	var resp APIResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return
	}

	ifData, ok := resp.Data.(map[string]interface{})
	if !ok {
		return
	}

	for name, ifInfo := range ifData {
		info, ok := ifInfo.(map[string]interface{})
		if !ok {
			continue
		}

		iface := vendors.InterfaceFact{
			Name: name,
		}

		if status, ok := info["status"].(string); ok {
			if status == "up" {
				iface.OperStatus = "up"
				iface.AdminStatus = "up"
			} else {
				iface.OperStatus = "down"
				iface.AdminStatus = "down"
			}
		}

		if ipaddr, ok := info["ipaddr"].(string); ok && ipaddr != "" {
			iface.IPAddresses = append(iface.IPAddresses, ipaddr)
		}

		if mac, ok := info["macaddr"].(string); ok {
			iface.MacAddress = mac
		}

		if mtu, ok := info["mtu"].(float64); ok {
			iface.MTU = int(mtu)
		}

		if media, ok := info["media"].(string); ok {
			iface.Description = media
		}

		facts.Interfaces = append(facts.Interfaces, iface)
	}
}

// Firewall operations

// Alias represents a pfSense firewall alias.
type Alias struct {
	ID          int      `json:"id,omitempty"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Address     []string `json:"address"`
	Description string   `json:"descr,omitempty"`
	Detail      []string `json:"detail,omitempty"`
}

// GetAlias retrieves a firewall alias by name.
func (a *Adapter) GetAlias(ctx context.Context, name string) (*Alias, error) {
	resp, err := a.request(ctx, "GET", "/firewall/alias", nil)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(resp, &apiResp); err != nil {
		return nil, err
	}

	aliases, ok := apiResp.Data.([]interface{})
	if !ok {
		return nil, nil
	}

	for _, aliasData := range aliases {
		aliasMap, ok := aliasData.(map[string]interface{})
		if !ok {
			continue
		}

		if aliasMap["name"] == name {
			alias := &Alias{
				Name: name,
			}
			if t, ok := aliasMap["type"].(string); ok {
				alias.Type = t
			}
			if d, ok := aliasMap["descr"].(string); ok {
				alias.Description = d
			}
			if id, ok := aliasMap["id"].(float64); ok {
				alias.ID = int(id)
			}
			if addrs, ok := aliasMap["address"].([]interface{}); ok {
				for _, addr := range addrs {
					if a, ok := addr.(string); ok {
						alias.Address = append(alias.Address, a)
					}
				}
			}
			return alias, nil
		}
	}

	return nil, nil
}

// CreateAlias creates a firewall alias.
func (a *Adapter) CreateAlias(ctx context.Context, alias *Alias) error {
	data, _ := json.Marshal(alias)
	resp, err := a.request(ctx, "POST", "/firewall/alias", data)
	if err != nil {
		return err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(resp, &apiResp); err != nil {
		return err
	}

	if apiResp.Code != 200 {
		return fmt.Errorf("failed to create alias: %s", apiResp.Message)
	}

	return nil
}

// UpdateAlias updates a firewall alias.
func (a *Adapter) UpdateAlias(ctx context.Context, alias *Alias) error {
	data, _ := json.Marshal(alias)
	resp, err := a.request(ctx, "PUT", "/firewall/alias", data)
	if err != nil {
		return err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(resp, &apiResp); err != nil {
		return err
	}

	if apiResp.Code != 200 {
		return fmt.Errorf("failed to update alias: %s", apiResp.Message)
	}

	return nil
}

// DeleteAlias deletes a firewall alias by ID.
func (a *Adapter) DeleteAlias(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/firewall/alias?id=%d", id)
	resp, err := a.request(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(resp, &apiResp); err != nil {
		return err
	}

	if apiResp.Code != 200 {
		return fmt.Errorf("failed to delete alias: %s", apiResp.Message)
	}

	return nil
}

// Rule represents a pfSense firewall rule.
type Rule struct {
	Tracker     int    `json:"tracker,omitempty"`
	Type        string `json:"type,omitempty"`
	Interface   string `json:"interface"`
	IPProtocol  string `json:"ipprotocol,omitempty"`
	Protocol    string `json:"protocol"`
	Source      string `json:"src"`
	SrcPort     string `json:"srcport,omitempty"`
	Destination string `json:"dst"`
	DstPort     string `json:"dstport,omitempty"`
	Description string `json:"descr,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
	Top         bool   `json:"top,omitempty"`
}

// GetRules retrieves firewall rules.
func (a *Adapter) GetRules(ctx context.Context) ([]Rule, error) {
	resp, err := a.request(ctx, "GET", "/firewall/rule", nil)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(resp, &apiResp); err != nil {
		return nil, err
	}

	rulesData, ok := apiResp.Data.([]interface{})
	if !ok {
		return nil, nil
	}

	var rules []Rule
	for _, ruleData := range rulesData {
		ruleMap, ok := ruleData.(map[string]interface{})
		if !ok {
			continue
		}

		rule := Rule{}
		if v, ok := ruleMap["tracker"].(float64); ok {
			rule.Tracker = int(v)
		}
		if v, ok := ruleMap["interface"].(string); ok {
			rule.Interface = v
		}
		if v, ok := ruleMap["protocol"].(string); ok {
			rule.Protocol = v
		}
		if v, ok := ruleMap["src"].(string); ok {
			rule.Source = v
		}
		if v, ok := ruleMap["dst"].(string); ok {
			rule.Destination = v
		}
		if v, ok := ruleMap["descr"].(string); ok {
			rule.Description = v
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

// CreateRule creates a firewall rule.
func (a *Adapter) CreateRule(ctx context.Context, rule *Rule) error {
	data, _ := json.Marshal(rule)
	resp, err := a.request(ctx, "POST", "/firewall/rule", data)
	if err != nil {
		return err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(resp, &apiResp); err != nil {
		return err
	}

	if apiResp.Code != 200 {
		return fmt.Errorf("failed to create rule: %s", apiResp.Message)
	}

	return nil
}

// DeleteRule deletes a firewall rule by tracker ID.
func (a *Adapter) DeleteRule(ctx context.Context, tracker int) error {
	endpoint := fmt.Sprintf("/firewall/rule?tracker=%d", tracker)
	resp, err := a.request(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(resp, &apiResp); err != nil {
		return err
	}

	if apiResp.Code != 200 {
		return fmt.Errorf("failed to delete rule: %s", apiResp.Message)
	}

	return nil
}

// ApplyChanges applies pending firewall changes.
func (a *Adapter) ApplyChanges(ctx context.Context) error {
	_, err := a.request(ctx, "POST", "/firewall/apply", nil)
	return err
}

// Service operations

// GetServices gets the list of services.
func (a *Adapter) GetServices(ctx context.Context) ([]map[string]interface{}, error) {
	resp, err := a.request(ctx, "GET", "/services", nil)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(resp, &apiResp); err != nil {
		return nil, err
	}

	services, ok := apiResp.Data.([]interface{})
	if !ok {
		return nil, nil
	}

	result := make([]map[string]interface{}, 0, len(services))
	for _, s := range services {
		if svc, ok := s.(map[string]interface{}); ok {
			result = append(result, svc)
		}
	}

	return result, nil
}

// ServiceControl controls a service (start, stop, restart).
func (a *Adapter) ServiceControl(ctx context.Context, service, action string) error {
	data := map[string]string{
		"service": service,
	}
	body, _ := json.Marshal(data)

	var endpoint string
	switch action {
	case "start":
		endpoint = "/services/start"
	case "stop":
		endpoint = "/services/stop"
	case "restart":
		endpoint = "/services/restart"
	default:
		return fmt.Errorf("unsupported action: %s", action)
	}

	_, err := a.request(ctx, "POST", endpoint, body)
	return err
}

func init() {
	// Register the adapter factory with the default registry
	vendors.Register(vendors.VendorPfSense, func(config *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		pfConfig := &Config{
			VendorConfig: config,
			Port:         443,
			TLS:          true,
			APIVersion:   "v1",
		}
		return NewAdapter(pfConfig), nil
	})
}
