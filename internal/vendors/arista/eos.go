// Package arista provides Arista device adapters for proxy agents.
package arista

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/protocols/ssh"
	"github.com/shawnbutts/keystone-core/internal/proxy"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

// EOSAdapter implements VendorAdapter for Arista EOS devices.
// It supports both SSH CLI and eAPI (JSON-RPC over HTTP) modes.
type EOSAdapter struct {
	vendors.BaseVendorAdapter
	config     *EOSConfig
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	httpClient *http.Client
	inEnable   bool
	inConfig   bool
}

// EOSConfig contains Arista EOS specific configuration.
type EOSConfig struct {
	*vendors.VendorConfig
	// Mode is the connection mode (ssh or eapi).
	Mode string `json:"mode,omitempty"`
	// EAPIPort is the eAPI HTTP/HTTPS port (default 443).
	EAPIPort int `json:"eapi_port,omitempty"`
	// EAPITLS enables HTTPS for eAPI (default true).
	EAPITLS bool `json:"eapi_tls,omitempty"`
	// EAPIInsecure skips TLS verification.
	EAPIInsecure bool `json:"eapi_insecure,omitempty"`
	// Secret is the enable secret.
	Secret string `json:"secret,omitempty"`
}

// DefaultEOSConfig returns a default EOS configuration.
func DefaultEOSConfig() *EOSConfig {
	return &EOSConfig{
		VendorConfig: vendors.DefaultVendorConfig(),
		Mode:         "ssh",
		EAPIPort:     443,
		EAPITLS:      true,
	}
}

// NewEOSAdapter creates a new Arista EOS adapter.
func NewEOSAdapter(config *EOSConfig) *EOSAdapter {
	if config == nil {
		config = DefaultEOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	adapter := &EOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		config: config,
	}

	if config.Mode == "ssh" {
		sshConfig := ssh.DefaultConfig()
		sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
		sshConfig.Timeout = config.Timeout
		adapter.sshAdapter = ssh.NewAdapter(sshConfig)
	}

	return adapter
}

// Vendor implements VendorAdapter.Vendor.
func (a *EOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorAristaEOS
}

// Type implements ProtocolAdapter.Type.
func (a *EOSAdapter) Type() protocols.ProtocolType {
	if a.config.Mode == "eapi" {
		return protocols.ProtocolREST
	}
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *EOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred

	if a.config.Mode == "eapi" {
		return a.connectEAPI(ctx, device, cred)
	}
	return a.connectSSH(ctx, device, cred)
}

// connectSSH establishes an SSH connection.
func (a *EOSAdapter) connectSSH(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:         "arista_eos",
		Prompts:        []string{">", "#", "(config"},
		EnableCmd:      "enable",
		EnablePassword: a.Config.EnablePassword,
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	// Disable paging - best-effort, non-fatal if unsupported
	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "terminal length 0")
	}

	// Enter enable mode if needed
	if a.Config.EnablePassword != "" || a.config.Secret != "" {
		if err := a.enterEnable(ctx); err != nil {
			return fmt.Errorf("failed to enter enable mode: %w", err)
		}
	}

	a.Connected = true
	return nil
}

// connectEAPI establishes an eAPI connection.
func (a *EOSAdapter) connectEAPI(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.httpClient = &http.Client{
		Timeout: a.Config.Timeout,
	}

	// Test connection with a simple command
	_, err := a.executeEAPI(ctx, []string{"show version"})
	if err != nil {
		return fmt.Errorf("eAPI connection test failed: %w", err)
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *EOSAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false
	a.inEnable = false
	a.inConfig = false

	if a.shell != nil {
		a.shell.Close()
		a.shell = nil
	}

	if a.sshAdapter != nil {
		return a.sshAdapter.Disconnect(ctx)
	}
	return nil
}

// Execute implements ProtocolAdapter.Execute.
func (a *EOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	start := time.Now()
	result := &protocols.ExecuteResult{
		StartTime: start,
	}

	var output string
	var err error

	if a.config.Mode == "eapi" {
		results, execErr := a.executeEAPI(ctx, []string{req.Command})
		if execErr != nil {
			err = execErr
		} else if len(results) > 0 {
			output = results[0]
		}
	} else {
		output, err = a.runCommand(ctx, req.Command)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(start)

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
		return result, err
	}

	result.Stdout = []byte(output)
	result.ExitCode = 0
	return result, nil
}

// HealthCheck implements ProtocolAdapter.HealthCheck.
func (a *EOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
	}

	var err error
	if a.config.Mode == "eapi" {
		_, err = a.executeEAPI(ctx, []string{"show version"})
	} else {
		_, err = a.runCommand(ctx, "show version")
	}

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
func (a *EOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	command := "show running-config"
	if section != "" {
		command = fmt.Sprintf("show running-config section %s", section)
	}

	if a.config.Mode == "eapi" {
		results, err := a.executeEAPI(ctx, []string{command})
		if err != nil {
			return "", err
		}
		if len(results) > 0 {
			return results[0], nil
		}
		return "", nil
	}
	return a.runCommand(ctx, command)
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *EOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	if a.config.Mode == "eapi" {
		return a.setConfigEAPI(ctx, commands)
	}
	return a.setConfigSSH(ctx, commands)
}

// setConfigSSH applies configuration via SSH.
func (a *EOSAdapter) setConfigSSH(ctx context.Context, commands []string) error {
	// Enter config mode
	if err := a.enterConfig(ctx); err != nil {
		return err
	}

	// Execute commands
	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			_ = a.exitConfig(ctx) //nolint:errcheck // best-effort exit config on error
			return fmt.Errorf("config command failed: %s: %w", cmd, err)
		}
	}

	// Exit config mode
	return a.exitConfig(ctx)
}

// setConfigEAPI applies configuration via eAPI.
func (a *EOSAdapter) setConfigEAPI(ctx context.Context, commands []string) error {
	// Prepend configure and append end
	fullCommands := make([]string, 0, len(commands)+2)
	fullCommands = append(fullCommands, "configure")
	fullCommands = append(fullCommands, commands...)
	fullCommands = append(fullCommands, "end")

	_, err := a.executeEAPI(ctx, fullCommands)
	return err
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *EOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Arista",
		OSType: "EOS",
		Raw:    make(map[string]string),
	}

	// Get version info
	var versionOutput string
	var err error

	if a.config.Mode == "eapi" {
		results, execErr := a.executeEAPI(ctx, []string{"show version"})
		if execErr != nil {
			return nil, execErr
		}
		if len(results) > 0 {
			versionOutput = results[0]
		}
	} else {
		versionOutput, err = a.runCommand(ctx, "show version")
		if err != nil {
			return nil, fmt.Errorf("failed to get version: %w", err)
		}
	}
	facts.Raw["version"] = versionOutput

	// Parse version output
	a.parseVersion(versionOutput, facts)

	// Get hostname
	var hostnameOutput string
	if a.config.Mode == "eapi" {
		results, _ := a.executeEAPI(ctx, []string{"show hostname"})
		if len(results) > 0 {
			hostnameOutput = results[0]
		}
	} else {
		hostnameOutput, _ = a.runCommand(ctx, "show hostname")
	}
	facts.Raw["hostname"] = hostnameOutput
	a.parseHostname(hostnameOutput, facts)

	// Get interfaces
	var ifOutput string
	if a.config.Mode == "eapi" {
		results, _ := a.executeEAPI(ctx, []string{"show interfaces status"})
		if len(results) > 0 {
			ifOutput = results[0]
		}
	} else {
		ifOutput, _ = a.runCommand(ctx, "show interfaces status")
	}
	facts.Raw["interfaces"] = ifOutput
	a.parseInterfaces(ifOutput, facts)

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *EOSAdapter) SaveConfig(ctx context.Context) error {
	if a.config.Mode == "eapi" {
		_, err := a.executeEAPI(ctx, []string{"copy running-config startup-config"})
		return err
	}
	_, err := a.runCommand(ctx, "copy running-config startup-config")
	return err
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *EOSAdapter) IsConnected() bool {
	return a.Connected
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *EOSAdapter) Metrics() *protocols.AdapterMetrics {
	if a.sshAdapter != nil {
		return a.sshAdapter.Metrics()
	}
	return &protocols.AdapterMetrics{}
}

// runCommand executes a command via SSH.
func (a *EOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

// enterEnable enters enable mode.
func (a *EOSAdapter) enterEnable(ctx context.Context) error {
	if a.inEnable {
		return nil
	}

	password := a.Config.EnablePassword
	if a.config.Secret != "" {
		password = a.config.Secret
	}

	result, err := a.shell.Execute(ctx, "enable")
	if err != nil {
		return err
	}

	// Check if password is required
	if strings.Contains(strings.ToLower(result.Output), "password") {
		if password == "" {
			return fmt.Errorf("enable password required but not configured")
		}
		if _, err := a.shell.Execute(ctx, password); err != nil {
			return err
		}
	}

	a.inEnable = true
	return nil
}

// enterConfig enters configuration mode.
func (a *EOSAdapter) enterConfig(ctx context.Context) error {
	if a.inConfig {
		return nil
	}

	if !a.inEnable {
		if err := a.enterEnable(ctx); err != nil {
			return err
		}
	}

	if _, err := a.runCommand(ctx, "configure terminal"); err != nil {
		return err
	}

	a.inConfig = true
	return nil
}

// exitConfig exits configuration mode.
func (a *EOSAdapter) exitConfig(ctx context.Context) error {
	if !a.inConfig {
		return nil
	}

	if _, err := a.runCommand(ctx, "end"); err != nil {
		return err
	}

	a.inConfig = false
	return nil
}

// eAPIRequest represents an eAPI JSON-RPC request.
type eAPIRequest struct {
	JSONRPC string     `json:"jsonrpc"`
	Method  string     `json:"method"`
	Params  eAPIParams `json:"params"`
	ID      string     `json:"id"`
}

type eAPIParams struct {
	Version int      `json:"version"`
	Cmds    []string `json:"cmds"`
	Format  string   `json:"format"`
}

// eAPIResponse represents an eAPI JSON-RPC response.
type eAPIResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      string       `json:"id"`
	Result  []eAPIResult `json:"result,omitempty"`
	Error   *eAPIError   `json:"error,omitempty"`
}

type eAPIResult struct {
	Output string `json:"output,omitempty"`
}

type eAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    []struct {
		Errors []string `json:"errors"`
	} `json:"data,omitempty"`
}

// executeEAPI executes commands via eAPI.
func (a *EOSAdapter) executeEAPI(ctx context.Context, commands []string) ([]string, error) {
	scheme := "https"
	if !a.config.EAPITLS {
		scheme = "http"
	}

	port := a.config.EAPIPort
	if port == 0 {
		port = 443
	}

	url := fmt.Sprintf("%s://%s:%d/command-api", scheme, a.Device.Address, port)

	request := eAPIRequest{
		JSONRPC: "2.0",
		Method:  "runCmds",
		Params: eAPIParams{
			Version: 1,
			Cmds:    commands,
			Format:  "text",
		},
		ID: fmt.Sprintf("kscore-%d", time.Now().UnixNano()),
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Set authentication
	if basicCred, ok := a.Credential.(*credentials.RESTBasicCredential); ok {
		req.SetBasicAuth(basicCred.Username, basicCred.Password)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("eAPI request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("eAPI request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp eAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("eAPI error %d: %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	results := make([]string, len(apiResp.Result))
	for i, r := range apiResp.Result {
		results[i] = r.Output
	}

	return results, nil
}

// parseVersion parses version output.
func (a *EOSAdapter) parseVersion(output string, facts *vendors.DeviceFacts) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Arista") && strings.Contains(line, "EOS") {
			// Parse model from line like "Arista DCS-7050TX-64-R"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				facts.Model = parts[1]
			}
		}

		if strings.Contains(line, "Software image version:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				facts.OSVersion = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "Serial number:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "Uptime:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				facts.Uptime = parseUptime(strings.TrimSpace(parts[1]))
			}
		}
	}
}

// parseHostname parses hostname output.
func (a *EOSAdapter) parseHostname(output string, facts *vendors.DeviceFacts) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Hostname:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "FQDN:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				facts.FQDN = strings.TrimSpace(parts[1])
			}
		}
	}
}

// parseInterfaces parses interface status output.
func (a *EOSAdapter) parseInterfaces(output string, facts *vendors.DeviceFacts) {
	lines := strings.Split(output, "\n")
	ifacePattern := regexp.MustCompile(`^(Et\S+|Po\S+|Vlan\d+|Ma\S+)\s+`)

	for _, line := range lines {
		if !ifacePattern.MatchString(line) {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		iface := vendors.InterfaceFact{
			Name:       fields[0],
			OperStatus: strings.ToLower(fields[2]),
		}

		switch iface.OperStatus {
		case "connected":
			iface.OperStatus = "up"
			iface.AdminStatus = "up"
		case "notconnect":
			iface.OperStatus = "down"
			iface.AdminStatus = "up"
		case "disabled":
			iface.OperStatus = "down"
			iface.AdminStatus = "down"
		}

		// Parse speed if present
		if len(fields) > 4 {
			speedStr := fields[4]
			if speed, err := strconv.Atoi(strings.TrimSuffix(speedStr, "G")); err == nil {
				if strings.HasSuffix(speedStr, "G") {
					iface.Speed = speed * 1000
				} else {
					iface.Speed = speed
				}
			}
		}

		facts.Interfaces = append(facts.Interfaces, iface)
	}
}

// parseUptime parses uptime string to duration.
func parseUptime(s string) time.Duration {
	// Parse formats like "1 day, 2 hours, 30 minutes"
	var total time.Duration

	// Extract days
	if idx := strings.Index(s, "day"); idx > 0 {
		numStr := strings.TrimSpace(s[:idx])
		if days, err := strconv.Atoi(strings.Fields(numStr)[len(strings.Fields(numStr))-1]); err == nil {
			total += time.Duration(days) * 24 * time.Hour
		}
	}

	// Extract hours
	if idx := strings.Index(s, "hour"); idx > 0 {
		before := s[:idx]
		fields := strings.Fields(before)
		if len(fields) > 0 {
			if hours, err := strconv.Atoi(fields[len(fields)-1]); err == nil {
				total += time.Duration(hours) * time.Hour
			}
		}
	}

	// Extract minutes
	if idx := strings.Index(s, "minute"); idx > 0 {
		before := s[:idx]
		fields := strings.Fields(before)
		if len(fields) > 0 {
			if minutes, err := strconv.Atoi(fields[len(fields)-1]); err == nil {
				total += time.Duration(minutes) * time.Minute
			}
		}
	}

	return total
}

func init() {
	// Register the adapter factory with the default registry
	vendors.Register(vendors.VendorAristaEOS, func(config *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		eosConfig := &EOSConfig{
			VendorConfig: config,
		}
		return NewEOSAdapter(eosConfig), nil
	})
}
