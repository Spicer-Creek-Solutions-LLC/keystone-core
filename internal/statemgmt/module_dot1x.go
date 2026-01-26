package statemgmt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"
)

// Dot1xModule implements 802.1X authentication management
type Dot1xModule struct {
	*BaseModule
}

// NewDot1xModule creates a new 802.1X module
func NewDot1xModule() *Dot1xModule {
	return &Dot1xModule{
		BaseModule: NewBaseModule("dot1x", []string{"enabled", "disabled"}),
	}
}

// Dot1xConfig holds 802.1X configuration parameters
type Dot1xConfig struct {
	Name       string // Connection/profile name (defaults to interface name)
	Interface  string // Network interface (required)
	EAPMethod  string // EAP method: tls, ttls, peap (required)
	Identity   string // User identity/username (required)
	Password   string // Password for TTLS/PEAP (required for non-TLS methods)
	ClientCert string // Path to client certificate (required for TLS)
	ClientKey  string // Path to client private key (required for TLS)
	CACert     string // Path to CA certificate (optional but recommended)
	Phase2     string // Phase 2 authentication: mschapv2, pap, chap, md5, gtc (for TTLS/PEAP)
	Anonymous  string // Anonymous identity for outer authentication (optional)
}

// Dot1xBackend represents the available 802.1X management backend
type Dot1xBackend string

const (
	D1XUnknown       Dot1xBackend = "unknown"
	D1XWpaSupplicant Dot1xBackend = "wpa_supplicant" // Linux (wired and wireless)
	D1XNetworkManager Dot1xBackend = "networkmanager" // Linux with NetworkManager
	D1XDot3svc       Dot1xBackend = "dot3svc"        // Windows wired 802.1X
	D1XProfiles      Dot1xBackend = "profiles"       // macOS configuration profiles
)

// Valid EAP methods
var validEAPMethods = map[string]bool{
	"tls":  true,
	"ttls": true,
	"peap": true,
}

// Valid Phase 2 authentication methods
var validPhase2Methods = map[string]bool{
	"mschapv2": true,
	"pap":      true,
	"chap":     true,
	"md5":      true,
	"gtc":      true,
	"":         true, // optional
}

// Check checks the current state of 802.1X configuration
func (m *Dot1xModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseDot1xConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse 802.1X config: %w", err)
	}

	backend, err := m.detectDot1xBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to detect 802.1X backend: %w", err)
	}
	result.Metadata["backend"] = string(backend)
	result.Metadata["interface"] = config.Interface

	exists, isEnabled, err := m.checkDot1xExists(ctx, backend, config)
	if err != nil {
		return nil, fmt.Errorf("failed to check 802.1X: %w", err)
	}

	result.Present = exists
	if exists && isEnabled {
		result.CurrentState = "enabled"
	} else {
		result.CurrentState = "disabled"
	}

	// Determine if current state matches desired state
	result.Matches = (result.CurrentState == decl.State)

	switch decl.State {
	case "enabled":
		if !exists || !isEnabled {
			result.Diff["state"] = map[string]interface{}{
				"from": result.CurrentState,
				"to":   "enabled",
			}
		}
	case "disabled":
		if exists && isEnabled {
			result.Diff["state"] = map[string]interface{}{
				"from": "enabled",
				"to":   "disabled",
			}
		}
	}

	return result, nil
}

// Apply applies the 802.1X configuration
func (m *Dot1xModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		StartTime: startTime,
	}

	config, err := m.parseDot1xConfig(decl)
	if err != nil {
		result.Success = false
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	if err := m.validateDot1xConfig(config, decl.State); err != nil {
		result.Success = false
		result.Error = err
		result.Comment = fmt.Sprintf("Invalid config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	backend, err := m.detectDot1xBackend()
	if err != nil {
		result.Success = false
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to detect 802.1X backend: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Check current state
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Apply changes
	var applyErr error
	switch decl.State {
	case "enabled":
		applyErr = m.enableDot1x(ctx, backend, config, result)
	case "disabled":
		applyErr = m.disableDot1x(ctx, backend, config, result)
	default:
		applyErr = fmt.Errorf("unknown state: %s", decl.State)
	}

	if applyErr != nil {
		result.Success = false
		result.Error = applyErr
		result.Comment = fmt.Sprintf("Failed to apply state: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test verifies the 802.1X configuration matches the desired state
func (m *Dot1xModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// parseDot1xConfig parses the state declaration into Dot1xConfig
func (m *Dot1xModule) parseDot1xConfig(decl *StateDeclaration) (*Dot1xConfig, error) {
	config := &Dot1xConfig{}

	// Interface (required)
	if v, ok := decl.Parameters["interface"].(string); ok {
		config.Interface = v
	}
	if config.Interface == "" {
		return nil, fmt.Errorf("interface is required")
	}

	// Name (defaults to interface)
	if v, ok := decl.Parameters["name"].(string); ok {
		config.Name = v
	}
	if config.Name == "" {
		config.Name = fmt.Sprintf("dot1x-%s", config.Interface)
	}

	// EAP method
	if v, ok := decl.Parameters["eap_method"].(string); ok {
		config.EAPMethod = strings.ToLower(v)
	}
	// Also accept "eap" as parameter name
	if config.EAPMethod == "" {
		if v, ok := decl.Parameters["eap"].(string); ok {
			config.EAPMethod = strings.ToLower(v)
		}
	}

	// Identity
	if v, ok := decl.Parameters["identity"].(string); ok {
		config.Identity = v
	}

	// Password
	if v, ok := decl.Parameters["password"].(string); ok {
		config.Password = v
	}

	// Client certificate
	if v, ok := decl.Parameters["client_cert"].(string); ok {
		config.ClientCert = v
	}

	// Client key
	if v, ok := decl.Parameters["client_key"].(string); ok {
		config.ClientKey = v
	}

	// CA certificate
	if v, ok := decl.Parameters["ca_cert"].(string); ok {
		config.CACert = v
	}

	// Phase 2 authentication
	if v, ok := decl.Parameters["phase2"].(string); ok {
		config.Phase2 = strings.ToLower(v)
	}
	// Also accept "inner_auth" as parameter name
	if config.Phase2 == "" {
		if v, ok := decl.Parameters["inner_auth"].(string); ok {
			config.Phase2 = strings.ToLower(v)
		}
	}

	// Anonymous identity
	if v, ok := decl.Parameters["anonymous"].(string); ok {
		config.Anonymous = v
	}
	// Also accept "anonymous_identity" as parameter name
	if config.Anonymous == "" {
		if v, ok := decl.Parameters["anonymous_identity"].(string); ok {
			config.Anonymous = v
		}
	}

	return config, nil
}

// validateDot1xConfig validates the 802.1X configuration
func (m *Dot1xModule) validateDot1xConfig(config *Dot1xConfig, state string) error {
	// For disabled state, only interface is needed
	if state == "disabled" {
		return nil
	}

	// Interface is always required
	if config.Interface == "" {
		return fmt.Errorf("interface is required")
	}

	// EAP method is required for enabled state
	if config.EAPMethod == "" {
		return fmt.Errorf("eap_method is required")
	}
	if !validEAPMethods[config.EAPMethod] {
		return fmt.Errorf("invalid EAP method: %s (valid: tls, ttls, peap)", config.EAPMethod)
	}

	// Identity is required
	if config.Identity == "" {
		return fmt.Errorf("identity is required")
	}

	// Validate based on EAP method
	switch config.EAPMethod {
	case "tls":
		if config.ClientCert == "" {
			return fmt.Errorf("client_cert is required for EAP-TLS")
		}
		if config.ClientKey == "" {
			return fmt.Errorf("client_key is required for EAP-TLS")
		}
	case "ttls", "peap":
		if config.Password == "" {
			return fmt.Errorf("password is required for EAP-%s", strings.ToUpper(config.EAPMethod))
		}
		// Phase 2 defaults to mschapv2 if not specified
		if config.Phase2 == "" {
			config.Phase2 = "mschapv2"
		}
		if !validPhase2Methods[config.Phase2] {
			return fmt.Errorf("invalid phase2 method: %s (valid: mschapv2, pap, chap, md5, gtc)", config.Phase2)
		}
	}

	return nil
}

// detectDot1xBackend detects the available 802.1X backend
func (m *Dot1xModule) detectDot1xBackend() (Dot1xBackend, error) {
	switch runtime.GOOS {
	case "linux":
		// Check for NetworkManager first
		if _, err := exec.LookPath("nmcli"); err == nil {
			// Check if NetworkManager is running
			cmd := exec.Command("systemctl", "is-active", "NetworkManager")
			if err := cmd.Run(); err == nil {
				return D1XNetworkManager, nil
			}
		}
		// Fall back to wpa_supplicant
		if _, err := exec.LookPath("wpa_supplicant"); err == nil {
			return D1XWpaSupplicant, nil
		}
		return D1XUnknown, fmt.Errorf("no 802.1X backend found (need NetworkManager or wpa_supplicant)")

	case "darwin":
		return D1XProfiles, nil

	case "windows":
		return D1XDot3svc, nil

	default:
		return D1XUnknown, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// checkDot1xExists checks if 802.1X is configured and enabled
func (m *Dot1xModule) checkDot1xExists(ctx context.Context, backend Dot1xBackend, config *Dot1xConfig) (exists bool, enabled bool, err error) {
	switch backend {
	case D1XNetworkManager:
		return m.checkDot1xNetworkManager(ctx, config)
	case D1XWpaSupplicant:
		return m.checkDot1xWpaSupplicant(ctx, config)
	case D1XDot3svc:
		return m.checkDot1xWindows(ctx, config)
	case D1XProfiles:
		return m.checkDot1xMacOS(ctx, config)
	default:
		return false, false, fmt.Errorf("unsupported backend: %s", backend)
	}
}

// enableDot1x enables 802.1X authentication
func (m *Dot1xModule) enableDot1x(ctx context.Context, backend Dot1xBackend, config *Dot1xConfig, result *StateResult) error {
	switch backend {
	case D1XNetworkManager:
		return m.enableDot1xNetworkManager(ctx, config, result)
	case D1XWpaSupplicant:
		return m.enableDot1xWpaSupplicant(ctx, config, result)
	case D1XDot3svc:
		return m.enableDot1xWindows(ctx, config, result)
	case D1XProfiles:
		return m.enableDot1xMacOS(ctx, config, result)
	default:
		return fmt.Errorf("unsupported backend: %s", backend)
	}
}

// disableDot1x disables 802.1X authentication
func (m *Dot1xModule) disableDot1x(ctx context.Context, backend Dot1xBackend, config *Dot1xConfig, result *StateResult) error {
	switch backend {
	case D1XNetworkManager:
		return m.disableDot1xNetworkManager(ctx, config, result)
	case D1XWpaSupplicant:
		return m.disableDot1xWpaSupplicant(ctx, config, result)
	case D1XDot3svc:
		return m.disableDot1xWindows(ctx, config, result)
	case D1XProfiles:
		return m.disableDot1xMacOS(ctx, config, result)
	default:
		return fmt.Errorf("unsupported backend: %s", backend)
	}
}

// ============================================================================
// NetworkManager Backend (Linux with nmcli)
// ============================================================================

func (m *Dot1xModule) checkDot1xNetworkManager(ctx context.Context, config *Dot1xConfig) (bool, bool, error) {
	// Check if a connection exists for this interface with 802.1X
	cmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME,DEVICE,TYPE",
		"connection", "show", "--active")
	output, err := cmd.Output()
	if err != nil {
		// No active connections, check configured
		cmd = exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME,TYPE",
			"connection", "show")
		output, err = cmd.Output()
		if err != nil {
			return false, false, nil
		}
	}

	// Look for our connection
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, config.Name) {
			// Check if it has 802.1X configured
			detailCmd := exec.CommandContext(ctx, "nmcli", "-t", "-f",
				"802-1x.eap", "connection", "show", config.Name)
			detailOutput, err := detailCmd.Output()
			if err == nil && len(strings.TrimSpace(string(detailOutput))) > 0 {
				// Check if connection is active
				activeCmd := exec.CommandContext(ctx, "nmcli", "-t", "-f",
					"GENERAL.STATE", "connection", "show", config.Name)
				activeOutput, _ := activeCmd.Output()
				isActive := strings.Contains(string(activeOutput), "activated")
				return true, isActive, nil
			}
		}
	}

	return false, false, nil
}

func (m *Dot1xModule) enableDot1xNetworkManager(ctx context.Context, config *Dot1xConfig, result *StateResult) error {
	// Delete existing connection if it exists
	exec.CommandContext(ctx, "nmcli", "connection", "delete", config.Name).Run()

	// Build the nmcli command
	args := []string{"connection", "add",
		"type", "802-3-ethernet",
		"con-name", config.Name,
		"ifname", config.Interface,
		"802-1x.eap", config.EAPMethod,
		"802-1x.identity", config.Identity,
	}

	switch config.EAPMethod {
	case "tls":
		args = append(args,
			"802-1x.client-cert", config.ClientCert,
			"802-1x.private-key", config.ClientKey,
		)
		if config.CACert != "" {
			args = append(args, "802-1x.ca-cert", config.CACert)
		}
	case "ttls":
		args = append(args,
			"802-1x.phase2-auth", config.Phase2,
			"802-1x.password", config.Password,
		)
		if config.CACert != "" {
			args = append(args, "802-1x.ca-cert", config.CACert)
		}
		if config.Anonymous != "" {
			args = append(args, "802-1x.anonymous-identity", config.Anonymous)
		}
	case "peap":
		args = append(args,
			"802-1x.phase2-auth", config.Phase2,
			"802-1x.password", config.Password,
		)
		if config.CACert != "" {
			args = append(args, "802-1x.ca-cert", config.CACert)
		}
		if config.Anonymous != "" {
			args = append(args, "802-1x.anonymous-identity", config.Anonymous)
		}
	}

	cmd := exec.CommandContext(ctx, "nmcli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create 802.1X connection: %w: %s", err, output)
	}

	// Activate the connection
	activateCmd := exec.CommandContext(ctx, "nmcli", "connection", "up", config.Name)
	output, err = activateCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to activate 802.1X connection: %w: %s", err, output)
	}

	result.Comment = fmt.Sprintf("802.1X enabled on %s using %s", config.Interface, config.EAPMethod)
	return nil
}

func (m *Dot1xModule) disableDot1xNetworkManager(ctx context.Context, config *Dot1xConfig, result *StateResult) error {
	// Deactivate and delete the connection
	exec.CommandContext(ctx, "nmcli", "connection", "down", config.Name).Run()

	cmd := exec.CommandContext(ctx, "nmcli", "connection", "delete", config.Name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore error if connection doesn't exist
		if !strings.Contains(string(output), "not found") {
			return fmt.Errorf("failed to delete 802.1X connection: %w: %s", err, output)
		}
	}

	result.Comment = fmt.Sprintf("802.1X disabled on %s", config.Interface)
	return nil
}

// ============================================================================
// wpa_supplicant Backend (Linux)
// ============================================================================

const wpaSupplicantDot1xConfPath = "/etc/wpa_supplicant/wpa_supplicant-%s.conf"

func (m *Dot1xModule) checkDot1xWpaSupplicant(ctx context.Context, config *Dot1xConfig) (bool, bool, error) {
	confPath := fmt.Sprintf(wpaSupplicantDot1xConfPath, config.Interface)

	// Check if config file exists
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		return false, false, nil
	}

	// Check if wpa_supplicant is running for this interface
	cmd := exec.CommandContext(ctx, "wpa_cli", "-i", config.Interface, "status")
	output, err := cmd.Output()
	if err != nil {
		return true, false, nil // Config exists but not running
	}

	// Check if authenticated
	if strings.Contains(string(output), "wpa_state=COMPLETED") {
		return true, true, nil
	}

	return true, false, nil
}

func (m *Dot1xModule) enableDot1xWpaSupplicant(ctx context.Context, config *Dot1xConfig, result *StateResult) error {
	// Generate wpa_supplicant configuration
	confContent := m.generateWpaSupplicantDot1xConfig(config)

	confPath := fmt.Sprintf(wpaSupplicantDot1xConfPath, config.Interface)
	confDir := filepath.Dir(confPath)

	// Ensure directory exists
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write configuration file
	if err := os.WriteFile(confPath, []byte(confContent), 0600); err != nil {
		return fmt.Errorf("failed to write wpa_supplicant config: %w", err)
	}

	// Stop any existing wpa_supplicant for this interface
	exec.CommandContext(ctx, "wpa_cli", "-i", config.Interface, "terminate").Run()

	// Start wpa_supplicant
	cmd := exec.CommandContext(ctx, "wpa_supplicant",
		"-B", // Background
		"-i", config.Interface,
		"-c", confPath,
		"-D", "wired", // Wired driver
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start wpa_supplicant: %w: %s", err, output)
	}

	result.Comment = fmt.Sprintf("802.1X enabled on %s using wpa_supplicant", config.Interface)
	return nil
}

func (m *Dot1xModule) disableDot1xWpaSupplicant(ctx context.Context, config *Dot1xConfig, result *StateResult) error {
	// Stop wpa_supplicant
	exec.CommandContext(ctx, "wpa_cli", "-i", config.Interface, "terminate").Run()

	// Remove configuration file
	confPath := fmt.Sprintf(wpaSupplicantDot1xConfPath, config.Interface)
	os.Remove(confPath)

	result.Comment = fmt.Sprintf("802.1X disabled on %s", config.Interface)
	return nil
}

// generateWpaSupplicantDot1xConfig generates wpa_supplicant configuration for 802.1X
func (m *Dot1xModule) generateWpaSupplicantDot1xConfig(config *Dot1xConfig) string {
	var buf bytes.Buffer

	buf.WriteString("ctrl_interface=/var/run/wpa_supplicant\n")
	buf.WriteString("ctrl_interface_group=root\n")
	buf.WriteString("ap_scan=0\n\n")
	buf.WriteString("network={\n")
	buf.WriteString("    key_mgmt=IEEE8021X\n")
	buf.WriteString("    eapol_flags=0\n")

	switch config.EAPMethod {
	case "tls":
		buf.WriteString("    eap=TLS\n")
		buf.WriteString(fmt.Sprintf("    identity=\"%s\"\n", config.Identity))
		buf.WriteString(fmt.Sprintf("    client_cert=\"%s\"\n", config.ClientCert))
		buf.WriteString(fmt.Sprintf("    private_key=\"%s\"\n", config.ClientKey))
		if config.CACert != "" {
			buf.WriteString(fmt.Sprintf("    ca_cert=\"%s\"\n", config.CACert))
		}

	case "ttls":
		buf.WriteString("    eap=TTLS\n")
		buf.WriteString(fmt.Sprintf("    identity=\"%s\"\n", config.Identity))
		buf.WriteString(fmt.Sprintf("    password=\"%s\"\n", config.Password))
		phase2 := m.mapPhase2Method(config.Phase2)
		buf.WriteString(fmt.Sprintf("    phase2=\"auth=%s\"\n", phase2))
		if config.CACert != "" {
			buf.WriteString(fmt.Sprintf("    ca_cert=\"%s\"\n", config.CACert))
		}
		if config.Anonymous != "" {
			buf.WriteString(fmt.Sprintf("    anonymous_identity=\"%s\"\n", config.Anonymous))
		}

	case "peap":
		buf.WriteString("    eap=PEAP\n")
		buf.WriteString(fmt.Sprintf("    identity=\"%s\"\n", config.Identity))
		buf.WriteString(fmt.Sprintf("    password=\"%s\"\n", config.Password))
		phase2 := m.mapPhase2Method(config.Phase2)
		buf.WriteString(fmt.Sprintf("    phase2=\"auth=%s\"\n", phase2))
		if config.CACert != "" {
			buf.WriteString(fmt.Sprintf("    ca_cert=\"%s\"\n", config.CACert))
		}
		if config.Anonymous != "" {
			buf.WriteString(fmt.Sprintf("    anonymous_identity=\"%s\"\n", config.Anonymous))
		}
	}

	buf.WriteString("}\n")

	return buf.String()
}

// mapPhase2Method maps our phase2 names to wpa_supplicant names
func (m *Dot1xModule) mapPhase2Method(phase2 string) string {
	switch phase2 {
	case "mschapv2":
		return "MSCHAPV2"
	case "pap":
		return "PAP"
	case "chap":
		return "CHAP"
	case "md5":
		return "MD5"
	case "gtc":
		return "GTC"
	default:
		return "MSCHAPV2"
	}
}

// ============================================================================
// Windows Backend (dot3svc / netsh lan)
// ============================================================================

func (m *Dot1xModule) checkDot1xWindows(ctx context.Context, config *Dot1xConfig) (bool, bool, error) {
	// Check if 802.1X is enabled on the interface
	cmd := exec.CommandContext(ctx, "netsh", "lan", "show", "interfaces")
	output, err := cmd.Output()
	if err != nil {
		return false, false, nil
	}

	// Look for our interface
	outputStr := string(output)
	if !strings.Contains(outputStr, config.Interface) {
		return false, false, nil
	}

	// Check if 802.1X profile exists
	cmd = exec.CommandContext(ctx, "netsh", "lan", "show", "profiles")
	output, err = cmd.Output()
	if err != nil {
		return false, false, nil
	}

	if strings.Contains(string(output), config.Name) {
		// Check if it's enabled
		cmd = exec.CommandContext(ctx, "netsh", "lan", "show", "settings")
		output, _ = cmd.Output()
		isEnabled := strings.Contains(string(output), "Enabled")
		return true, isEnabled, nil
	}

	return false, false, nil
}

func (m *Dot1xModule) enableDot1xWindows(ctx context.Context, config *Dot1xConfig, result *StateResult) error {
	// Generate Windows 802.1X profile XML
	profileXML := m.generateWindowsDot1xProfileXML(config)

	// Write profile to temp file
	tmpFile, err := os.CreateTemp("", "dot1x-profile-*.xml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(profileXML); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write profile: %w", err)
	}
	tmpFile.Close()

	// Enable 802.1X on the interface
	enableCmd := exec.CommandContext(ctx, "netsh", "lan", "set", "autoconfig",
		"enabled=yes", fmt.Sprintf("interface=\"%s\"", config.Interface))
	if output, err := enableCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable 802.1X autoconfig: %w: %s", err, output)
	}

	// Add the profile
	addCmd := exec.CommandContext(ctx, "netsh", "lan", "add", "profile",
		fmt.Sprintf("filename=\"%s\"", tmpFile.Name()),
		fmt.Sprintf("interface=\"%s\"", config.Interface))
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add 802.1X profile: %w: %s", err, output)
	}

	result.Comment = fmt.Sprintf("802.1X enabled on %s using %s", config.Interface, config.EAPMethod)
	return nil
}

func (m *Dot1xModule) disableDot1xWindows(ctx context.Context, config *Dot1xConfig, result *StateResult) error {
	// Delete the profile
	deleteCmd := exec.CommandContext(ctx, "netsh", "lan", "delete", "profile",
		fmt.Sprintf("name=\"%s\"", config.Name),
		fmt.Sprintf("interface=\"%s\"", config.Interface))
	deleteCmd.Run() // Ignore error if profile doesn't exist

	// Disable 802.1X on the interface
	disableCmd := exec.CommandContext(ctx, "netsh", "lan", "set", "autoconfig",
		"enabled=no", fmt.Sprintf("interface=\"%s\"", config.Interface))
	if output, err := disableCmd.CombinedOutput(); err != nil {
		// Ignore if already disabled
		if !strings.Contains(string(output), "disabled") {
			return fmt.Errorf("failed to disable 802.1X: %w: %s", err, output)
		}
	}

	result.Comment = fmt.Sprintf("802.1X disabled on %s", config.Interface)
	return nil
}

// Windows 802.1X profile XML template
const windowsDot1xProfileTemplate = `<?xml version="1.0"?>
<LANProfile xmlns="http://www.microsoft.com/networking/LAN/profile/v1">
    <MSM>
        <security>
            <OneXEnforced>true</OneXEnforced>
            <OneXEnabled>true</OneXEnabled>
            <OneX xmlns="http://www.microsoft.com/networking/OneX/v1">
                <EAPConfig>
                    <EapHostConfig xmlns="http://www.microsoft.com/provisioning/EapHostConfig">
                        {{- if eq .EAPMethod "tls"}}
                        <EapMethod>
                            <Type xmlns="http://www.microsoft.com/provisioning/EapCommon">13</Type>
                            <VendorId xmlns="http://www.microsoft.com/provisioning/EapCommon">0</VendorId>
                            <VendorType xmlns="http://www.microsoft.com/provisioning/EapCommon">0</VendorType>
                            <AuthorId xmlns="http://www.microsoft.com/provisioning/EapCommon">0</AuthorId>
                        </EapMethod>
                        <Config xmlns="http://www.microsoft.com/provisioning/EapHostConfig">
                            <Eap xmlns="http://www.microsoft.com/provisioning/BaseEapConnectionPropertiesV1">
                                <Type>13</Type>
                                <EapType xmlns="http://www.microsoft.com/provisioning/EapTlsConnectionPropertiesV1">
                                    <CredentialsSource>
                                        <CertificateStore>
                                            <SimpleCertSelection>true</SimpleCertSelection>
                                        </CertificateStore>
                                    </CredentialsSource>
                                    <ServerValidation>
                                        <DisableUserPromptForServerValidation>false</DisableUserPromptForServerValidation>
                                        <ServerNames></ServerNames>
                                    </ServerValidation>
                                    <DifferentUsername>false</DifferentUsername>
                                </EapType>
                            </Eap>
                        </Config>
                        {{- else if eq .EAPMethod "peap"}}
                        <EapMethod>
                            <Type xmlns="http://www.microsoft.com/provisioning/EapCommon">25</Type>
                            <VendorId xmlns="http://www.microsoft.com/provisioning/EapCommon">0</VendorId>
                            <VendorType xmlns="http://www.microsoft.com/provisioning/EapCommon">0</VendorType>
                            <AuthorId xmlns="http://www.microsoft.com/provisioning/EapCommon">0</AuthorId>
                        </EapMethod>
                        <Config xmlns="http://www.microsoft.com/provisioning/EapHostConfig">
                            <Eap xmlns="http://www.microsoft.com/provisioning/BaseEapConnectionPropertiesV1">
                                <Type>25</Type>
                                <EapType xmlns="http://www.microsoft.com/provisioning/MsPeapConnectionPropertiesV1">
                                    <ServerValidation>
                                        <DisableUserPromptForServerValidation>false</DisableUserPromptForServerValidation>
                                        <ServerNames></ServerNames>
                                    </ServerValidation>
                                    <FastReconnect>true</FastReconnect>
                                    <InnerEapOptional>false</InnerEapOptional>
                                    <Eap xmlns="http://www.microsoft.com/provisioning/BaseEapConnectionPropertiesV1">
                                        <Type>26</Type>
                                        <EapType xmlns="http://www.microsoft.com/provisioning/MsChapV2ConnectionPropertiesV1">
                                            <UseWinLogonCredentials>false</UseWinLogonCredentials>
                                        </EapType>
                                    </Eap>
                                    <EnableQuarantineChecks>false</EnableQuarantineChecks>
                                    <RequireCryptoBinding>false</RequireCryptoBinding>
                                </EapType>
                            </Eap>
                        </Config>
                        {{- else if eq .EAPMethod "ttls"}}
                        <EapMethod>
                            <Type xmlns="http://www.microsoft.com/provisioning/EapCommon">21</Type>
                            <VendorId xmlns="http://www.microsoft.com/provisioning/EapCommon">0</VendorId>
                            <VendorType xmlns="http://www.microsoft.com/provisioning/EapCommon">0</VendorType>
                            <AuthorId xmlns="http://www.microsoft.com/provisioning/EapCommon">311</AuthorId>
                        </EapMethod>
                        <Config xmlns="http://www.microsoft.com/provisioning/EapHostConfig">
                            <EapTtls xmlns="http://www.microsoft.com/provisioning/EapTtlsConnectionPropertiesV1">
                                <ServerValidation>
                                    <DisablePrompt>false</DisablePrompt>
                                </ServerValidation>
                                <Phase2Authentication>
                                    <MSCHAPv2Authentication>
                                        <UseWinlogonCredentials>false</UseWinlogonCredentials>
                                    </MSCHAPv2Authentication>
                                </Phase2Authentication>
                            </EapTtls>
                        </Config>
                        {{- end}}
                    </EapHostConfig>
                </EAPConfig>
            </OneX>
        </security>
    </MSM>
</LANProfile>`

func (m *Dot1xModule) generateWindowsDot1xProfileXML(config *Dot1xConfig) string {
	tmpl, err := template.New("dot1x").Parse(windowsDot1xProfileTemplate)
	if err != nil {
		return ""
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return ""
	}

	return buf.String()
}

// ============================================================================
// macOS Backend (Configuration Profiles)
// ============================================================================

func (m *Dot1xModule) checkDot1xMacOS(ctx context.Context, config *Dot1xConfig) (bool, bool, error) {
	// Check for configuration profile
	cmd := exec.CommandContext(ctx, "profiles", "-L", "-v")
	output, err := cmd.Output()
	if err != nil {
		return false, false, nil
	}

	// Look for 802.1X profile
	if strings.Contains(string(output), "com.apple.firstactiveethernet.managed") ||
		strings.Contains(string(output), config.Name) {
		return true, true, nil
	}

	return false, false, nil
}

func (m *Dot1xModule) enableDot1xMacOS(ctx context.Context, config *Dot1xConfig, result *StateResult) error {
	// Generate mobileconfig profile
	profileContent := m.generateMacOSDot1xProfile(config)

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "dot1x-*.mobileconfig")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(profileContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write profile: %w", err)
	}
	tmpFile.Close()

	// Install profile
	cmd := exec.CommandContext(ctx, "profiles", "-I", "-F", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install 802.1X profile: %w: %s", err, output)
	}

	result.Comment = fmt.Sprintf("802.1X profile installed on %s", config.Interface)
	return nil
}

func (m *Dot1xModule) disableDot1xMacOS(ctx context.Context, config *Dot1xConfig, result *StateResult) error {
	// Remove profile by identifier
	profileID := fmt.Sprintf("com.keystone.dot1x.%s", config.Name)
	cmd := exec.CommandContext(ctx, "profiles", "-R", "-p", profileID)
	cmd.Run() // Ignore error if profile doesn't exist

	result.Comment = fmt.Sprintf("802.1X profile removed from %s", config.Interface)
	return nil
}

// macOS mobileconfig profile template for 802.1X
const macOSDot1xProfileTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>PayloadContent</key>
    <array>
        <dict>
            <key>AutoJoin</key>
            <true/>
            <key>EAPClientConfiguration</key>
            <dict>
                <key>AcceptEAPTypes</key>
                <array>
                    {{- if eq .EAPMethod "tls"}}
                    <integer>13</integer>
                    {{- else if eq .EAPMethod "ttls"}}
                    <integer>21</integer>
                    {{- else if eq .EAPMethod "peap"}}
                    <integer>25</integer>
                    {{- end}}
                </array>
                <key>UserName</key>
                <string>{{.Identity}}</string>
                {{- if or (eq .EAPMethod "ttls") (eq .EAPMethod "peap")}}
                <key>UserPassword</key>
                <string>{{.Password}}</string>
                {{- end}}
            </dict>
            <key>Interface</key>
            <string>FirstActiveEthernet</string>
            <key>PayloadDescription</key>
            <string>802.1X authentication for {{.Interface}}</string>
            <key>PayloadDisplayName</key>
            <string>Ethernet - {{.Name}}</string>
            <key>PayloadIdentifier</key>
            <string>com.keystone.dot1x.{{.Name}}.ethernet</string>
            <key>PayloadType</key>
            <string>com.apple.firstactiveethernet.managed</string>
            <key>PayloadUUID</key>
            <string>{{.Name}}</string>
            <key>PayloadVersion</key>
            <integer>1</integer>
            <key>SetupModes</key>
            <array>
                <string>System</string>
            </array>
        </dict>
    </array>
    <key>PayloadDescription</key>
    <string>802.1X Configuration</string>
    <key>PayloadDisplayName</key>
    <string>802.1X - {{.Name}}</string>
    <key>PayloadIdentifier</key>
    <string>com.keystone.dot1x.{{.Name}}</string>
    <key>PayloadOrganization</key>
    <string>Keystone Core</string>
    <key>PayloadRemovalDisallowed</key>
    <false/>
    <key>PayloadType</key>
    <string>Configuration</string>
    <key>PayloadUUID</key>
    <string>com.keystone.dot1x.{{.Name}}</string>
    <key>PayloadVersion</key>
    <integer>1</integer>
</dict>
</plist>`

func (m *Dot1xModule) generateMacOSDot1xProfile(config *Dot1xConfig) string {
	tmpl, err := template.New("dot1x").Parse(macOSDot1xProfileTemplate)
	if err != nil {
		return ""
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return ""
	}

	return buf.String()
}

func init() {
	RegisterModule(NewDot1xModule())
}
