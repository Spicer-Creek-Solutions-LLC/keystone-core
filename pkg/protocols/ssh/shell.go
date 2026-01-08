// Package ssh provides an SSH protocol adapter for proxy agents.
package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/shawnbutts/keystone-core/pkg/protocols"
)

// Shell represents an interactive SSH shell session.
type Shell struct {
	session  *ssh.Session
	stdin    io.WriteCloser
	stdout   io.Reader
	stderr   io.Reader
	adapter  *Adapter
	mu       sync.Mutex
	closed   bool
	prompt   string
	timeout  time.Duration
}

// ShellConfig configures an interactive shell.
type ShellConfig struct {
	// PTYConfig is the PTY configuration.
	PTYConfig *protocols.PTYConfig

	// Prompt is the expected shell prompt pattern.
	Prompt string

	// Timeout is the default command timeout.
	Timeout time.Duration

	// Environment variables to set.
	Environment map[string]string
}

// DefaultShellConfig returns a default shell configuration.
func DefaultShellConfig() *ShellConfig {
	return &ShellConfig{
		PTYConfig: protocols.DefaultPTYConfig(),
		Prompt:    "$ ",
		Timeout:   30 * time.Second,
	}
}

// NewShell creates a new interactive shell session.
func (a *Adapter) NewShell(ctx context.Context, config *ShellConfig) (*Shell, error) {
	if config == nil {
		config = DefaultShellConfig()
	}

	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Request PTY
	ptyConfig := config.PTYConfig
	if ptyConfig == nil {
		ptyConfig = protocols.DefaultPTYConfig()
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty(ptyConfig.Term, int(ptyConfig.Rows), int(ptyConfig.Cols), modes); err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to request PTY: %w", err)
	}

	// Set environment variables
	for k, v := range config.Environment {
		_ = session.Setenv(k, v)
	}

	// Get stdin/stdout/stderr pipes
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start shell
	if err := session.Shell(); err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to start shell: %w", err)
	}

	shell := &Shell{
		session: session,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		adapter: a,
		prompt:  config.Prompt,
		timeout: config.Timeout,
	}

	// Wait for initial prompt
	if _, err := shell.readUntilPrompt(ctx, config.Timeout); err != nil {
		shell.Close()
		return nil, fmt.Errorf("failed to get initial prompt: %w", err)
	}

	return shell, nil
}

// Execute executes a command in the shell and waits for the prompt.
func (s *Shell) Execute(ctx context.Context, command string) (*ShellResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("shell is closed")
	}

	result := &ShellResult{
		Command:   command,
		StartTime: time.Now(),
	}

	// Send command
	if _, err := fmt.Fprintf(s.stdin, "%s\n", command); err != nil {
		result.Error = fmt.Sprintf("failed to send command: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, err
	}

	// Read output until prompt
	timeout := s.timeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}

	output, err := s.readUntilPrompt(ctx, timeout)
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	// Clean output - remove the echoed command and prompt
	result.Output = s.cleanOutput(output, command)

	return result, nil
}

// ExecuteExpect executes a command and expects specific output patterns.
func (s *Shell) ExecuteExpect(ctx context.Context, command string, expects []string, timeout time.Duration) (*ShellResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("shell is closed")
	}

	result := &ShellResult{
		Command:   command,
		StartTime: time.Now(),
	}

	// Send command
	if _, err := fmt.Fprintf(s.stdin, "%s\n", command); err != nil {
		result.Error = fmt.Sprintf("failed to send command: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, err
	}

	// Read until we see one of the expected patterns
	output, matchedIdx, err := s.readUntilExpect(ctx, expects, timeout)
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Output = output
	result.MatchedExpect = matchedIdx

	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	return result, nil
}

// Send sends raw data to the shell without waiting for a response.
func (s *Shell) Send(data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("shell is closed")
	}

	_, err := s.stdin.Write([]byte(data))
	return err
}

// SendLine sends a line to the shell (with newline).
func (s *Shell) SendLine(line string) error {
	return s.Send(line + "\n")
}

// Read reads available output from the shell.
func (s *Shell) Read(timeout time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return "", fmt.Errorf("shell is closed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return s.readWithTimeout(ctx, timeout)
}

// readUntilPrompt reads output until the prompt is seen.
func (s *Shell) readUntilPrompt(ctx context.Context, timeout time.Duration) (string, error) {
	return s.readUntil(ctx, s.prompt, timeout)
}

// readUntil reads output until the pattern is seen.
func (s *Shell) readUntil(ctx context.Context, pattern string, timeout time.Duration) (string, error) {
	var buf bytes.Buffer
	readBuf := make([]byte, 4096)

	deadline := time.Now().Add(timeout)

	for {
		// Check context and timeout
		select {
		case <-ctx.Done():
			return buf.String(), ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return buf.String(), fmt.Errorf("timeout waiting for pattern: %s", pattern)
		}

		// Set read deadline on underlying connection (if possible)
		// This is a simplified implementation - real code would need access to the connection

		n, err := s.stdout.Read(readBuf)
		if err != nil && err != io.EOF {
			return buf.String(), err
		}

		if n > 0 {
			buf.Write(readBuf[:n])

			// Check if we've seen the pattern
			if strings.Contains(buf.String(), pattern) {
				return buf.String(), nil
			}
		}

		if err == io.EOF {
			return buf.String(), io.EOF
		}

		// Small sleep to prevent busy loop
		time.Sleep(10 * time.Millisecond)
	}
}

// readUntilExpect reads until one of the expected patterns is seen.
func (s *Shell) readUntilExpect(ctx context.Context, expects []string, timeout time.Duration) (string, int, error) {
	var buf bytes.Buffer
	readBuf := make([]byte, 4096)

	deadline := time.Now().Add(timeout)

	for {
		select {
		case <-ctx.Done():
			return buf.String(), -1, ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return buf.String(), -1, fmt.Errorf("timeout waiting for expected patterns")
		}

		n, err := s.stdout.Read(readBuf)
		if err != nil && err != io.EOF {
			return buf.String(), -1, err
		}

		if n > 0 {
			buf.Write(readBuf[:n])

			// Check each expected pattern
			output := buf.String()
			for i, expect := range expects {
				if strings.Contains(output, expect) {
					return output, i, nil
				}
			}
		}

		if err == io.EOF {
			return buf.String(), -1, io.EOF
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// readWithTimeout reads available output with a timeout.
func (s *Shell) readWithTimeout(ctx context.Context, timeout time.Duration) (string, error) {
	var buf bytes.Buffer
	readBuf := make([]byte, 4096)

	deadline := time.Now().Add(timeout)

	for {
		select {
		case <-ctx.Done():
			return buf.String(), nil
		default:
		}

		if time.Now().After(deadline) {
			return buf.String(), nil
		}

		n, err := s.stdout.Read(readBuf)
		if err != nil {
			if err == io.EOF {
				return buf.String(), nil
			}
			return buf.String(), err
		}

		if n > 0 {
			buf.Write(readBuf[:n])
		} else {
			break
		}
	}

	return buf.String(), nil
}

// cleanOutput removes the echoed command and prompt from output.
func (s *Shell) cleanOutput(output string, command string) string {
	lines := strings.Split(output, "\n")
	var cleaned []string

	for i, line := range lines {
		// Skip the first line (echoed command) and last line (prompt)
		if i == 0 && strings.HasSuffix(strings.TrimSpace(line), command) {
			continue
		}
		if i == len(lines)-1 && strings.Contains(line, s.prompt) {
			continue
		}
		cleaned = append(cleaned, line)
	}

	return strings.Join(cleaned, "\n")
}

// SetPrompt sets the expected prompt pattern.
func (s *Shell) SetPrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompt = prompt
}

// SetTimeout sets the default timeout.
func (s *Shell) SetTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeout = timeout
}

// Close closes the shell session.
func (s *Shell) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// Send exit command
	_, _ = s.stdin.Write([]byte("exit\n"))

	// Close stdin
	_ = s.stdin.Close()

	// Close session
	return s.session.Close()
}

// IsClosed returns true if the shell is closed.
func (s *Shell) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// ShellResult contains the result of a shell command.
type ShellResult struct {
	// Command is the executed command.
	Command string `json:"command"`

	// Output is the command output.
	Output string `json:"output"`

	// Error is any error that occurred.
	Error string `json:"error,omitempty"`

	// StartTime is when the command started.
	StartTime time.Time `json:"start_time"`

	// EndTime is when the command ended.
	EndTime time.Time `json:"end_time"`

	// Duration is how long the command took.
	Duration time.Duration `json:"duration"`

	// MatchedExpect is the index of the matched expect pattern (-1 if none).
	MatchedExpect int `json:"matched_expect,omitempty"`
}

// Success returns true if the command succeeded (no error).
func (r *ShellResult) Success() bool {
	return r.Error == ""
}

// NetworkDeviceShell provides specialized shell handling for network devices.
type NetworkDeviceShell struct {
	*Shell
	vendor   string
	prompts  []string
	enableCmd string
}

// NetworkDeviceConfig configures a network device shell.
type NetworkDeviceConfig struct {
	// Vendor is the device vendor (cisco, juniper, arista, etc).
	Vendor string

	// Prompts is a list of expected prompts.
	Prompts []string

	// EnableCmd is the command to enter privileged mode.
	EnableCmd string

	// EnablePassword is the enable password.
	EnablePassword string
}

// NewNetworkDeviceShell creates a network device shell.
func (a *Adapter) NewNetworkDeviceShell(ctx context.Context, config *NetworkDeviceConfig) (*NetworkDeviceShell, error) {
	shellConfig := DefaultShellConfig()

	// Set prompts based on vendor
	prompts := config.Prompts
	enableCmd := config.EnableCmd
	if len(prompts) == 0 {
		switch strings.ToLower(config.Vendor) {
		case "cisco", "ios":
			prompts = []string{">", "#"}
			enableCmd = "enable"
		case "juniper", "junos":
			prompts = []string{">", "#", "%"}
			enableCmd = "configure"
		case "arista", "eos":
			prompts = []string{">", "#"}
			enableCmd = "enable"
		case "vyos":
			prompts = []string{"$", "#"}
			enableCmd = "configure"
		default:
			prompts = []string{"$ ", "# ", "> "}
		}
	}

	// Use the first prompt for initial detection
	if len(prompts) > 0 {
		shellConfig.Prompt = prompts[0]
	}

	shell, err := a.NewShell(ctx, shellConfig)
	if err != nil {
		return nil, err
	}

	nds := &NetworkDeviceShell{
		Shell:     shell,
		vendor:    config.Vendor,
		prompts:   prompts,
		enableCmd: enableCmd,
	}

	// Enter enable mode if needed
	if config.EnablePassword != "" && enableCmd != "" {
		if _, err := nds.Enable(ctx, config.EnablePassword); err != nil {
			nds.Close()
			return nil, fmt.Errorf("failed to enter enable mode: %w", err)
		}
	}

	return nds, nil
}

// Enable enters privileged/enable mode.
func (n *NetworkDeviceShell) Enable(ctx context.Context, password string) (*ShellResult, error) {
	// Send enable command
	result, err := n.ExecuteExpect(ctx, n.enableCmd, append([]string{"Password:", "password:"}, n.prompts...), n.timeout)
	if err != nil {
		return result, err
	}

	// If password prompt, send password
	if result.MatchedExpect < 2 { // Matched password prompt
		result, err = n.ExecuteExpect(ctx, password, n.prompts, n.timeout)
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

// Configure enters configuration mode.
func (n *NetworkDeviceShell) Configure(ctx context.Context) (*ShellResult, error) {
	switch strings.ToLower(n.vendor) {
	case "cisco", "ios", "arista", "eos":
		return n.Execute(ctx, "configure terminal")
	case "juniper", "junos":
		return n.Execute(ctx, "configure")
	case "vyos":
		return n.Execute(ctx, "configure")
	default:
		return n.Execute(ctx, "configure terminal")
	}
}

// Exit exits the current mode.
func (n *NetworkDeviceShell) Exit(ctx context.Context) (*ShellResult, error) {
	return n.Execute(ctx, "exit")
}

// Commit commits configuration changes (for devices that support it).
func (n *NetworkDeviceShell) Commit(ctx context.Context) (*ShellResult, error) {
	switch strings.ToLower(n.vendor) {
	case "juniper", "junos", "vyos":
		return n.Execute(ctx, "commit")
	default:
		return nil, nil // No commit needed for Cisco-like devices
	}
}

// Save saves the running configuration.
func (n *NetworkDeviceShell) Save(ctx context.Context) (*ShellResult, error) {
	switch strings.ToLower(n.vendor) {
	case "cisco", "ios":
		return n.ExecuteExpect(ctx, "copy running-config startup-config",
			[]string{"[confirm]", "#", "Destination filename"},
			n.timeout)
	case "arista", "eos":
		return n.Execute(ctx, "copy running-config startup-config")
	case "juniper", "junos":
		return n.Execute(ctx, "commit")
	case "vyos":
		return n.Execute(ctx, "save")
	default:
		return n.Execute(ctx, "write memory")
	}
}

// GetConfig retrieves the running configuration.
func (n *NetworkDeviceShell) GetConfig(ctx context.Context) (*ShellResult, error) {
	switch strings.ToLower(n.vendor) {
	case "cisco", "ios", "arista", "eos":
		return n.Execute(ctx, "show running-config")
	case "juniper", "junos":
		return n.Execute(ctx, "show configuration")
	case "vyos":
		return n.Execute(ctx, "show configuration commands")
	default:
		return n.Execute(ctx, "show running-config")
	}
}
