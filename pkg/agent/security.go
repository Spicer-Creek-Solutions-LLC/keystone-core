package agent

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// SecurityConfig contains security settings for the agent
type SecurityConfig struct {
	// Authorization settings
	Authorization AuthorizationConfig

	// Command filtering
	CommandFilter CommandFilterConfig
}

// AuthorizationConfig configures command authorization
type AuthorizationConfig struct {
	// Enabled enables authorization checks on commands
	Enabled bool

	// SharedSecret is used for HMAC-based command signing
	// Commands must include a valid signature to be executed
	SharedSecret string

	// AllowedPrincipals is a list of principals (users/services) allowed to execute commands
	// If empty and Enabled is true, all authenticated principals are allowed
	AllowedPrincipals []string

	// RequireSignature requires commands to be cryptographically signed
	RequireSignature bool
}

// CommandFilterConfig configures command allowlist/blocklist
type CommandFilterConfig struct {
	// Mode: "allowlist" (default, more secure) or "blocklist"
	Mode string

	// Allowlist of permitted commands (exact match or glob patterns)
	// Only used when Mode is "allowlist"
	Allowlist []string

	// Blocklist of denied commands (exact match or glob patterns)
	// Only used when Mode is "blocklist"
	Blocklist []string

	// AllowBuiltins allows shell builtins (echo, cd, etc.)
	AllowBuiltins bool

	// BlockedPatterns are regex patterns that block commands
	// Applied regardless of mode
	BlockedPatterns []string

	// ExemptCommands are commands that bypass BlockedPatterns checks
	// Use this to allow specific commands like "mkfs" when needed
	// Patterns can be exact command names or glob patterns (e.g., "mkfs.*", "/sbin/mkfs*")
	ExemptCommands []string

	// MaxArgLength limits argument length to prevent injection
	MaxArgLength int

	// BlockEnvOverrides prevents dangerous environment variable overrides
	BlockEnvOverrides bool

	// BlockedEnvVars are environment variables that cannot be set
	BlockedEnvVars []string
}

// DefaultSecurityConfig returns a secure default configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		Authorization: AuthorizationConfig{
			Enabled:          false, // Disabled by default for backwards compatibility
			RequireSignature: false,
		},
		CommandFilter: CommandFilterConfig{
			Mode:          "blocklist", // Blocklist mode for backwards compatibility
			AllowBuiltins: true,
			MaxArgLength:  65536, // 64KB max per argument
			BlockEnvOverrides: true,
			BlockedEnvVars: []string{
				"LD_PRELOAD",
				"LD_LIBRARY_PATH",
				"DYLD_INSERT_LIBRARIES",
				"PYTHONPATH",
				"RUBYLIB",
				"PERL5LIB",
				"NODE_PATH",
			},
			BlockedPatterns: []string{
				`;\s*rm\s+-rf\s+/`,     // Dangerous rm commands
				`>\s*/dev/sd[a-z]`,     // Writing to block devices
				`mkfs\.`,               // Filesystem creation
				`dd\s+.*of=/dev/`,      // dd to devices
			},
		},
	}
}

// SecurityEnforcer enforces security policies on command execution
type SecurityEnforcer struct {
	config          *SecurityConfig
	allowlistRegex  []*regexp.Regexp
	blocklistRegex  []*regexp.Regexp
	blockedPatterns []*regexp.Regexp
	exemptRegex     []*regexp.Regexp
	mu              sync.RWMutex
}

// NewSecurityEnforcer creates a new security enforcer
func NewSecurityEnforcer(config *SecurityConfig) (*SecurityEnforcer, error) {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	se := &SecurityEnforcer{
		config: config,
	}

	// Compile allowlist patterns
	for _, pattern := range config.CommandFilter.Allowlist {
		re, err := compileGlobPattern(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid allowlist pattern %q: %w", pattern, err)
		}
		se.allowlistRegex = append(se.allowlistRegex, re)
	}

	// Compile blocklist patterns
	for _, pattern := range config.CommandFilter.Blocklist {
		re, err := compileGlobPattern(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid blocklist pattern %q: %w", pattern, err)
		}
		se.blocklistRegex = append(se.blocklistRegex, re)
	}

	// Compile blocked patterns
	for _, pattern := range config.CommandFilter.BlockedPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid blocked pattern %q: %w", pattern, err)
		}
		se.blockedPatterns = append(se.blockedPatterns, re)
	}

	// Compile exempt command patterns
	for _, pattern := range config.CommandFilter.ExemptCommands {
		re, err := compileGlobPattern(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid exempt command pattern %q: %w", pattern, err)
		}
		se.exemptRegex = append(se.exemptRegex, re)
	}

	return se, nil
}

// AuthorizeCommand checks if a command is authorized to execute
func (se *SecurityEnforcer) AuthorizeCommand(principal, command string, signature string) error {
	se.mu.RLock()
	defer se.mu.RUnlock()

	if !se.config.Authorization.Enabled {
		return nil // Authorization disabled
	}

	// Check principal is allowed
	if len(se.config.Authorization.AllowedPrincipals) > 0 {
		allowed := false
		for _, p := range se.config.Authorization.AllowedPrincipals {
			if p == principal || p == "*" {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("principal %q is not authorized to execute commands", principal)
		}
	}

	// Check signature if required
	if se.config.Authorization.RequireSignature {
		if signature == "" {
			return errors.New("command signature is required")
		}

		if !se.verifySignature(command, signature) {
			return errors.New("invalid command signature")
		}
	}

	return nil
}

// ValidateCommand checks if a command passes security filters
func (se *SecurityEnforcer) ValidateCommand(command string, args []string, env map[string]string, workingDir string) error {
	se.mu.RLock()
	defer se.mu.RUnlock()

	// Check command against blocklist/allowlist
	if err := se.validateCommandBinary(command); err != nil {
		return err
	}

	// Check for blocked patterns in full command line (unless command is exempt)
	if !se.isExemptCommand(command) {
		fullCommand := command + " " + strings.Join(args, " ")
		for _, re := range se.blockedPatterns {
			if re.MatchString(fullCommand) {
				return fmt.Errorf("command matches blocked pattern: %s", re.String())
			}
		}
	}

	// Check argument length
	if se.config.CommandFilter.MaxArgLength > 0 {
		for i, arg := range args {
			if len(arg) > se.config.CommandFilter.MaxArgLength {
				return fmt.Errorf("argument %d exceeds maximum length (%d > %d)",
					i, len(arg), se.config.CommandFilter.MaxArgLength)
			}
		}
	}

	// Check environment variables
	if se.config.CommandFilter.BlockEnvOverrides && len(env) > 0 {
		for _, blocked := range se.config.CommandFilter.BlockedEnvVars {
			if _, exists := env[blocked]; exists {
				return fmt.Errorf("environment variable %q is not allowed", blocked)
			}
		}
	}

	// Validate working directory (no path traversal)
	if workingDir != "" {
		cleanPath := filepath.Clean(workingDir)
		if strings.Contains(cleanPath, "..") {
			return errors.New("working directory contains path traversal")
		}
	}

	return nil
}

// validateCommandBinary checks if the command binary is allowed
func (se *SecurityEnforcer) validateCommandBinary(command string) error {
	// Get the base command name (without path)
	baseName := filepath.Base(command)

	switch se.config.CommandFilter.Mode {
	case "allowlist":
		// In allowlist mode, command must match at least one pattern
		if len(se.allowlistRegex) == 0 {
			return errors.New("allowlist mode enabled but no patterns configured")
		}

		for _, re := range se.allowlistRegex {
			if re.MatchString(command) || re.MatchString(baseName) {
				return nil
			}
		}
		return fmt.Errorf("command %q is not in allowlist", command)

	case "blocklist", "":
		// In blocklist mode, command must not match any pattern
		for _, re := range se.blocklistRegex {
			if re.MatchString(command) || re.MatchString(baseName) {
				return fmt.Errorf("command %q is blocked", command)
			}
		}
		return nil

	default:
		return fmt.Errorf("invalid command filter mode: %s", se.config.CommandFilter.Mode)
	}
}

// isExemptCommand checks if a command is exempt from blocked pattern checks
func (se *SecurityEnforcer) isExemptCommand(command string) bool {
	if len(se.exemptRegex) == 0 {
		return false
	}

	baseName := filepath.Base(command)
	for _, re := range se.exemptRegex {
		if re.MatchString(command) || re.MatchString(baseName) {
			return true
		}
	}
	return false
}

// verifySignature verifies an HMAC-SHA256 signature
func (se *SecurityEnforcer) verifySignature(message, signature string) bool {
	if se.config.Authorization.SharedSecret == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(se.config.Authorization.SharedSecret))
	mac.Write([]byte(message))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(signature))
}

// SignCommand creates an HMAC-SHA256 signature for a command
func (se *SecurityEnforcer) SignCommand(command string) string {
	se.mu.RLock()
	defer se.mu.RUnlock()

	if se.config.Authorization.SharedSecret == "" {
		return ""
	}

	mac := hmac.New(sha256.New, []byte(se.config.Authorization.SharedSecret))
	mac.Write([]byte(command))
	return hex.EncodeToString(mac.Sum(nil))
}

// compileGlobPattern converts a glob pattern to a regex
func compileGlobPattern(pattern string) (*regexp.Regexp, error) {
	// If it's already a regex (starts with ^), use as-is
	if strings.HasPrefix(pattern, "^") {
		return regexp.Compile(pattern)
	}

	// Convert glob to regex
	regexPattern := "^"
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// ** matches anything including /
				regexPattern += ".*"
				i++
			} else {
				// * matches anything except /
				regexPattern += "[^/]*"
			}
		case '?':
			regexPattern += "[^/]"
		case '.', '+', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\':
			regexPattern += "\\" + string(c)
		default:
			regexPattern += string(c)
		}
	}
	regexPattern += "$"

	return regexp.Compile(regexPattern)
}

// UpdateConfig updates the security configuration
func (se *SecurityEnforcer) UpdateConfig(config *SecurityConfig) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	// Recompile patterns
	var allowlistRegex, blocklistRegex, blockedPatterns, exemptRegex []*regexp.Regexp

	for _, pattern := range config.CommandFilter.Allowlist {
		re, err := compileGlobPattern(pattern)
		if err != nil {
			return fmt.Errorf("invalid allowlist pattern %q: %w", pattern, err)
		}
		allowlistRegex = append(allowlistRegex, re)
	}

	for _, pattern := range config.CommandFilter.Blocklist {
		re, err := compileGlobPattern(pattern)
		if err != nil {
			return fmt.Errorf("invalid blocklist pattern %q: %w", pattern, err)
		}
		blocklistRegex = append(blocklistRegex, re)
	}

	for _, pattern := range config.CommandFilter.BlockedPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid blocked pattern %q: %w", pattern, err)
		}
		blockedPatterns = append(blockedPatterns, re)
	}

	for _, pattern := range config.CommandFilter.ExemptCommands {
		re, err := compileGlobPattern(pattern)
		if err != nil {
			return fmt.Errorf("invalid exempt command pattern %q: %w", pattern, err)
		}
		exemptRegex = append(exemptRegex, re)
	}

	se.config = config
	se.allowlistRegex = allowlistRegex
	se.blocklistRegex = blocklistRegex
	se.blockedPatterns = blockedPatterns
	se.exemptRegex = exemptRegex

	return nil
}

// Config returns the current security configuration
func (se *SecurityEnforcer) Config() *SecurityConfig {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.config
}
