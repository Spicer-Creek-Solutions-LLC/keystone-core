// Package execution provides secure command execution with injection prevention.
package execution

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
)

// Standard errors for command policy violations
var (
	ErrCommandBlocked        = errors.New("command is blocked by policy")
	ErrCommandNotAllowed     = errors.New("command not in allowlist")
	ErrShellInjectionDetected = errors.New("potential shell injection detected")
	ErrEmptyCommand          = errors.New("empty command")
	ErrInvalidCommand        = errors.New("invalid command format")
)

// permissiveWarnOnce ensures the permissive mode deprecation warning is only logged once
var permissiveWarnOnce sync.Once

// ExecutionMode defines the security mode for command execution
type ExecutionMode string

const (
	// ExecutionModeStrict only allows explicitly allowlisted commands
	// This is the most secure mode but requires upfront configuration
	ExecutionModeStrict ExecutionMode = "strict"

	// ExecutionModeNormal blocks known dangerous patterns but allows other commands
	// This provides reasonable security while being flexible
	ExecutionModeNormal ExecutionMode = "normal"

	// ExecutionModePermissive only blocks the most dangerous patterns
	// WARNING: This mode provides minimal protection and should only be used
	// in fully trusted environments
	ExecutionModePermissive ExecutionMode = "permissive"
)

// CommandPolicy defines the security policy for command execution
type CommandPolicy struct {
	mu sync.RWMutex

	// Mode defines the security level
	Mode ExecutionMode

	// AllowedCommands is the list of allowed commands (for strict mode)
	// Commands are matched by their base name (e.g., "ls", "cat", "kubectl")
	AllowedCommands map[string]bool

	// AllowedPatterns are regex patterns for allowed commands
	// Useful for allowing command families like "kubectl *"
	AllowedPatterns []*regexp.Regexp

	// BlockedCommands are always blocked regardless of mode
	BlockedCommands map[string]bool

	// BlockedPatterns are always blocked regardless of mode
	BlockedPatterns []*regexp.Regexp

	// AllowShellExecution permits shell-based execution (bash -c, etc.)
	// When false, only direct execution is allowed
	AllowShellExecution bool

	// MaxCommandLength is the maximum allowed command length (0 = no limit)
	MaxCommandLength int

	// AllowedEnvVars restricts which environment variables can be set
	// Empty means all are allowed
	AllowedEnvVars map[string]bool
}

// DefaultPolicy returns a secure default policy (normal mode)
func DefaultPolicy() *CommandPolicy {
	return &CommandPolicy{
		Mode:                ExecutionModeNormal,
		AllowedCommands:     make(map[string]bool),
		AllowedPatterns:     nil,
		BlockedCommands:     defaultBlockedCommands(),
		BlockedPatterns:     defaultBlockedPatterns(),
		AllowShellExecution: true, // Allow but with validation
		MaxCommandLength:    65536, // 64KB max command length
		AllowedEnvVars:      make(map[string]bool),
	}
}

// StrictPolicy returns a policy that only allows explicitly allowlisted commands
func StrictPolicy(allowedCommands []string) *CommandPolicy {
	policy := &CommandPolicy{
		Mode:                ExecutionModeStrict,
		AllowedCommands:     make(map[string]bool),
		AllowedPatterns:     nil,
		BlockedCommands:     defaultBlockedCommands(),
		BlockedPatterns:     defaultBlockedPatterns(),
		AllowShellExecution: false, // Direct execution only in strict mode
		MaxCommandLength:    32768, // 32KB max in strict mode
		AllowedEnvVars:      make(map[string]bool),
	}

	for _, cmd := range allowedCommands {
		policy.AllowedCommands[cmd] = true
	}

	return policy
}

// defaultBlockedCommands returns commands that are always blocked
func defaultBlockedCommands() map[string]bool {
	return map[string]bool{
		// Destructive commands
		"rm":      true,
		"rmdir":   true,
		"del":     true,
		"format":  true,
		"mkfs":    true,
		"dd":      true,
		"shred":   true,

		// Shell/interpreter spawning (bypass execution controls)
		"bash":       true,
		"sh":         true,
		"zsh":        true,
		"csh":        true,
		"tcsh":       true,
		"fish":       true,
		"ksh":        true,
		"dash":       true,
		"powershell": true,
		"pwsh":       true,
		"cmd":        true,

		// Compilers/interpreters that can execute arbitrary code
		"python":  true,
		"python3": true,
		"python2": true,
		"perl":    true,
		"ruby":    true,
		"php":     true,
		"node":    true,
		"nodejs":  true,

		// Network tools that could exfiltrate or attack
		"nc":       true,
		"netcat":   true,
		"ncat":     true,
		"socat":    true,
		"telnet":   true,
		"nmap":     true,
		"masscan":  true,

		// Privilege escalation
		"sudo":    true,
		"su":      true,
		"doas":    true,
		"pkexec":  true,
		"runas":   true,

		// System modification
		"reboot":   true,
		"shutdown": true,
		"halt":     true,
		"poweroff": true,
		"init":     true,
		"systemctl": true,  // Can stop critical services
	}
}

// defaultBlockedPatterns returns regex patterns that are always blocked
func defaultBlockedPatterns() []*regexp.Regexp {
	patterns := []string{
		// Shell metacharacters that enable command chaining/injection
		`[;&|]`,                    // Command separators: ; & |
		"`",                        // Backtick command substitution
		`\$\(`,                     // $() command substitution
		`\$\{`,                     // ${} parameter expansion (can be dangerous)
		`>\s*/`,                    // Redirect to absolute path (overwrite system files)
		`>>\s*/`,                   // Append to absolute path
		`<\s*/etc/`,                // Read sensitive system files
		`\|\s*(bash|sh|zsh|python|perl|ruby|node)`, // Pipe to interpreter

		// Path traversal
		`\.\.\/`,                   // ../
		`\.\.\\`,                   // ..\

		// Dangerous patterns
		`/etc/passwd`,              // System files
		`/etc/shadow`,
		`/etc/sudoers`,
		`~/.ssh/`,                  // SSH keys
		`/root/`,                   // Root home
		`\beval\b`,                 // eval command
		`\bexec\b`,                 // exec command
		`\bsource\b`,               // source command
		`\.\s+/`,                   // . /path (source)

		// Windows-specific dangerous patterns
		`(?i)cmd\s*/c`,             // cmd /c
		`(?i)powershell\s+-`,       // powershell -command etc
		`(?i)\\windows\\system32`,  // System files

		// Base64 encoded commands (often used in attacks)
		`(?i)base64\s+-d`,          // base64 decode
		`(?i)base64\s+--decode`,

		// Network exfiltration patterns
		`(?i)curl\s+.*\s+-d`,       // curl with data (POST)
		`(?i)wget\s+.*-O\s*-`,      // wget to stdout
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if r, err := regexp.Compile(p); err == nil {
			compiled = append(compiled, r)
		}
	}
	return compiled
}

// Validate checks if a command is allowed by the policy
func (p *CommandPolicy) Validate(command string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Check for empty command
	if strings.TrimSpace(command) == "" {
		return ErrEmptyCommand
	}

	// Check command length
	if p.MaxCommandLength > 0 && len(command) > p.MaxCommandLength {
		return fmt.Errorf("%w: command exceeds maximum length of %d", ErrInvalidCommand, p.MaxCommandLength)
	}

	// Extract base command (first word)
	baseCommand := extractBaseCommand(command)
	if baseCommand == "" {
		return ErrEmptyCommand
	}

	// Always check blocked commands first
	if p.BlockedCommands[baseCommand] {
		return fmt.Errorf("%w: %s is blocked", ErrCommandBlocked, baseCommand)
	}

	// Check blocked patterns
	for _, pattern := range p.BlockedPatterns {
		if pattern.MatchString(command) {
			return fmt.Errorf("%w: matches blocked pattern", ErrShellInjectionDetected)
		}
	}

	// Mode-specific validation
	switch p.Mode {
	case ExecutionModeStrict:
		return p.validateStrict(command, baseCommand)
	case ExecutionModeNormal:
		return p.validateNormal(command, baseCommand)
	case ExecutionModePermissive:
		// Already passed blocked checks, allow it
		// DEPRECATED: Permissive mode provides minimal security and should not be used
		permissiveWarnOnce.Do(func() {
			log.Printf("DEPRECATED: ExecutionModePermissive provides minimal security protection and is deprecated. " +
				"Use ExecutionModeNormal instead. Permissive mode will be removed in a future release.")
		})
		return nil
	default:
		// Unknown mode, fail safe
		return fmt.Errorf("%w: unknown execution mode", ErrInvalidCommand)
	}
}

// validateStrict checks command against strict allowlist
func (p *CommandPolicy) validateStrict(command, baseCommand string) error {
	// Check explicit allowlist
	if p.AllowedCommands[baseCommand] {
		return nil
	}

	// Check allowed patterns
	for _, pattern := range p.AllowedPatterns {
		if pattern.MatchString(command) {
			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrCommandNotAllowed, baseCommand)
}

// validateNormal performs normal mode validation
func (p *CommandPolicy) validateNormal(command, baseCommand string) error {
	// In normal mode, commands pass if they're not blocked
	// Additional checks for shell injection patterns
	if containsShellMetacharacters(command) {
		return fmt.Errorf("%w: command contains shell metacharacters", ErrShellInjectionDetected)
	}

	return nil
}

// ValidateForShell validates a command that will be executed via shell
func (p *CommandPolicy) ValidateForShell(command string) error {
	p.mu.RLock()
	allowShell := p.AllowShellExecution
	mode := p.Mode
	p.mu.RUnlock()

	if !allowShell {
		return fmt.Errorf("%w: shell execution is disabled by policy", ErrCommandBlocked)
	}

	// Standard validation
	if err := p.Validate(command); err != nil {
		return err
	}

	// In permissive mode, skip the dangerous pattern check
	if mode == ExecutionModePermissive {
		return nil
	}

	// Additional strict checks for shell execution
	if containsDangerousShellPatterns(command) {
		return fmt.Errorf("%w: command contains dangerous shell patterns", ErrShellInjectionDetected)
	}

	return nil
}

// AddAllowedCommand adds a command to the allowlist
func (p *CommandPolicy) AddAllowedCommand(command string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.AllowedCommands[command] = true
}

// RemoveAllowedCommand removes a command from the allowlist
func (p *CommandPolicy) RemoveAllowedCommand(command string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.AllowedCommands, command)
}

// AddBlockedCommand adds a command to the blocklist
func (p *CommandPolicy) AddBlockedCommand(command string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.BlockedCommands[command] = true
}

// SetMode changes the execution mode
func (p *CommandPolicy) SetMode(mode ExecutionMode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Mode = mode
}

// extractBaseCommand extracts the first word (command name) from a command string
func extractBaseCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}

	// Handle paths (extract just the binary name)
	// e.g., "/usr/bin/ls" -> "ls", "./script.sh" -> "script.sh"
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}

	cmd := parts[0]

	// Extract basename from path
	lastSlash := strings.LastIndex(cmd, "/")
	if lastSlash >= 0 && lastSlash < len(cmd)-1 {
		cmd = cmd[lastSlash+1:]
	}
	lastBackslash := strings.LastIndex(cmd, "\\")
	if lastBackslash >= 0 && lastBackslash < len(cmd)-1 {
		cmd = cmd[lastBackslash+1:]
	}

	return strings.ToLower(cmd)
}

// containsShellMetacharacters checks for dangerous shell metacharacters
func containsShellMetacharacters(command string) bool {
	// Characters that have special meaning in shells
	dangerousChars := []string{
		";",   // Command separator
		"&",   // Background/AND
		"|",   // Pipe
		"`",   // Command substitution
		"$(",  // Command substitution
		"${",  // Parameter expansion
		"$()", // Command substitution
		">>",  // Append redirect
		"<<",  // Here-document
		"<(",  // Process substitution
		">(",  // Process substitution
	}

	for _, char := range dangerousChars {
		if strings.Contains(command, char) {
			return true
		}
	}

	// Check for unquoted backticks
	if strings.Contains(command, "`") {
		return true
	}

	return false
}

// containsDangerousShellPatterns checks for patterns that are dangerous when executed via shell
func containsDangerousShellPatterns(command string) bool {
	dangerousPatterns := []string{
		"&&",          // AND operator
		"||",          // OR operator
		"\n",          // Newline (command separator)
		"\r",          // Carriage return
		"$((",         // Arithmetic expansion
		"$[",          // Arithmetic (old style)
		"!$",          // History expansion
		"!!",          // History repeat
		"!-",          // History relative
		"eval ",       // Eval command
		"exec ",       // Exec command
		"source ",     // Source command
		". /",         // Dot source
		". ./",        // Dot source relative
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(command, pattern) {
			return true
		}
	}

	return false
}

// SanitizeForLogging removes sensitive parts from a command for safe logging
func SanitizeForLogging(command string) string {
	// Truncate long commands
	if len(command) > 200 {
		return command[:200] + "... (truncated)"
	}

	// Mask potential secrets (basic patterns)
	patterns := []struct {
		pattern *regexp.Regexp
		replace string
	}{
		{regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|key|api.key|apikey)=\S+`), "$1=****"},
		{regexp.MustCompile(`(?i)(authorization|auth):\s*[^'"\s]+(\s+[^'"\s]+)?`), "$1: ****"},
		{regexp.MustCompile(`(?i)--(password|passwd|token|secret|key)[\s=]\S+`), "--$1=****"},
	}

	result := command
	for _, p := range patterns {
		result = p.pattern.ReplaceAllString(result, p.replace)
	}

	return result
}
