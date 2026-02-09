package telnet

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// SessionResult contains the result of a session command.
type SessionResult struct {
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
func (r *SessionResult) Success() bool {
	return r.Error == ""
}

// Session manages an interactive Telnet session over a raw TCP connection.
type Session struct {
	conn       net.Conn
	negotiator *Negotiator
	prompt     string
	timeout    time.Duration
	mu         sync.Mutex
	closed     bool
}

// NewSession creates a new Telnet session.
func NewSession(conn net.Conn, negotiator *Negotiator, prompt string, timeout time.Duration) *Session {
	if prompt == "" {
		prompt = "# "
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Session{
		conn:       conn,
		negotiator: negotiator,
		prompt:     prompt,
		timeout:    timeout,
	}
}

// Authenticate performs prompt-based login.
// It waits for the login prompt, sends the username, waits for the password
// prompt, sends the password, then waits for the shell prompt.
func (s *Session) Authenticate(ctx context.Context, username, password string, loginPrompts, passwordPrompts []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session is closed")
	}

	// Wait for login prompt
	_, _, err := s.readUntilExpect(ctx, loginPrompts, s.timeout)
	if err != nil {
		return fmt.Errorf("waiting for login prompt: %w", err)
	}

	// Send username
	if err := s.sendLine(username); err != nil {
		return fmt.Errorf("sending username: %w", err)
	}

	// Wait for password prompt
	_, _, err = s.readUntilExpect(ctx, passwordPrompts, s.timeout)
	if err != nil {
		return fmt.Errorf("waiting for password prompt: %w", err)
	}

	// Send password
	if err := s.sendLine(password); err != nil {
		return fmt.Errorf("sending password: %w", err)
	}

	// Wait for shell prompt
	_, err = s.readUntilPrompt(ctx, s.timeout)
	if err != nil {
		return fmt.Errorf("waiting for shell prompt after login: %w", err)
	}

	return nil
}

// Execute sends a command and reads until the prompt returns.
func (s *Session) Execute(ctx context.Context, command string) (*SessionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("session is closed")
	}

	result := &SessionResult{
		Command:   command,
		StartTime: time.Now(),
	}

	// Send command
	if err := s.sendLine(command); err != nil {
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

	result.Output = s.cleanOutput(output, command)
	return result, nil
}

// ExecuteExpect sends a command and waits for one of the expected patterns.
func (s *Session) ExecuteExpect(ctx context.Context, command string, expects []string, timeout time.Duration) (*SessionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("session is closed")
	}

	result := &SessionResult{
		Command:   command,
		StartTime: time.Now(),
	}

	// Send command
	if err := s.sendLine(command); err != nil {
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

// Send sends raw data to the session.
func (s *Session) Send(data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session is closed")
	}

	return s.send(data)
}

// SendLine sends a line with CR+LF termination per RFC 854.
func (s *Session) SendLine(line string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session is closed")
	}

	return s.sendLine(line)
}

// send writes raw data to the connection (must hold mu).
func (s *Session) send(data string) error {
	_, err := s.conn.Write([]byte(data))
	return err
}

// sendLine writes a line with CR+LF termination (must hold mu).
func (s *Session) sendLine(line string) error {
	_, err := s.conn.Write([]byte(line + "\r\n"))
	return err
}

// readUntilPrompt reads output until the configured prompt is seen.
func (s *Session) readUntilPrompt(ctx context.Context, timeout time.Duration) (string, error) {
	return s.readUntil(ctx, s.prompt, timeout)
}

// readUntil reads output until the pattern is seen, processing IAC negotiation.
func (s *Session) readUntil(ctx context.Context, pattern string, timeout time.Duration) (string, error) {
	var buf bytes.Buffer
	readBuf := make([]byte, 4096)

	deadline := time.Now().Add(timeout)

	for {
		select {
		case <-ctx.Done():
			return buf.String(), ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return buf.String(), fmt.Errorf("timeout waiting for pattern: %s", pattern)
		}

		// Set a short read deadline so we don't block forever
		_ = s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, err := s.conn.Read(readBuf)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// Read timeout, check context and try again
				if err := wait.ForContext(ctx, 10*time.Millisecond); err != nil {
					return buf.String(), err
				}
				continue
			}
			if errors.Is(err, io.EOF) {
				return buf.String(), io.EOF
			}
			return buf.String(), err
		}

		if n > 0 {
			// Process IAC sequences
			clean, responses := s.negotiator.ProcessData(readBuf[:n])
			if len(responses) > 0 {
				_, _ = s.conn.Write(responses)
			}
			buf.Write(clean)

			if strings.Contains(buf.String(), pattern) {
				return buf.String(), nil
			}
		}
	}
}

// readUntilExpect reads until one of the expected patterns is seen.
func (s *Session) readUntilExpect(ctx context.Context, expects []string, timeout time.Duration) (output string, matchIndex int, err error) {
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

		_ = s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, err := s.conn.Read(readBuf)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				if err := wait.ForContext(ctx, 10*time.Millisecond); err != nil {
					return buf.String(), -1, err
				}
				continue
			}
			if errors.Is(err, io.EOF) {
				return buf.String(), -1, io.EOF
			}
			return buf.String(), -1, err
		}

		if n > 0 {
			clean, responses := s.negotiator.ProcessData(readBuf[:n])
			if len(responses) > 0 {
				_, _ = s.conn.Write(responses)
			}
			buf.Write(clean)

			output := buf.String()
			for i, expect := range expects {
				if strings.Contains(output, expect) {
					return output, i, nil
				}
			}
		}
	}
}

// cleanOutput removes the echoed command and prompt from output.
func (s *Session) cleanOutput(output, command string) string {
	lines := strings.Split(output, "\n")
	var cleaned []string

	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		// Skip the first line (echoed command)
		if i == 0 && strings.HasSuffix(strings.TrimSpace(trimmed), command) {
			continue
		}
		// Skip the last line (prompt)
		if i == len(lines)-1 && strings.Contains(trimmed, s.prompt) {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}

	return strings.Join(cleaned, "\n")
}

// SetPrompt sets the expected prompt pattern.
func (s *Session) SetPrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompt = prompt
}

// Close closes the session and underlying connection.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.conn.Close()
}

// IsClosed returns true if the session is closed.
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
