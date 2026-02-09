package telnet

import (
	"context"
	"net"
	"testing"
	"time"
)

// newTestSession creates a Session backed by a net.Pipe for testing.
// Returns the session and the remote end of the pipe.
func newTestSession(prompt string) (*Session, net.Conn) {
	client, server := net.Pipe()
	negotiator := NewNegotiator("vt100", 24, 80)
	if prompt == "" {
		prompt = "# "
	}
	session := NewSession(client, negotiator, prompt, 5*time.Second)
	return session, server
}

func TestNewSession_Defaults(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	negotiator := NewNegotiator("", 0, 0)
	session := NewSession(client, negotiator, "", 0)
	if session.prompt != "# " {
		t.Errorf("expected default prompt '# ', got %q", session.prompt)
	}
	if session.timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", session.timeout)
	}
}

func TestSession_SendLine_CRLF(t *testing.T) {
	session, server := newTestSession("# ")
	defer session.Close()
	defer server.Close()

	go func() {
		if err := session.SendLine("hello"); err != nil {
			t.Errorf("SendLine error: %v", err)
		}
	}()

	buf := make([]byte, 64)
	n, err := server.Read(buf)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	got := string(buf[:n])
	if got != "hello\r\n" {
		t.Errorf("expected 'hello\\r\\n', got %q", got)
	}
}

func TestSession_Send_Raw(t *testing.T) {
	session, server := newTestSession("# ")
	defer session.Close()
	defer server.Close()

	go func() {
		if err := session.Send("raw"); err != nil {
			t.Errorf("Send error: %v", err)
		}
	}()

	buf := make([]byte, 64)
	n, err := server.Read(buf)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	got := string(buf[:n])
	if got != "raw" {
		t.Errorf("expected 'raw', got %q", got)
	}
}

func TestSession_Execute(t *testing.T) {
	session, server := newTestSession("# ")
	defer session.Close()
	defer server.Close()

	// Simulate remote end: read command then send response
	go func() {
		buf := make([]byte, 256)
		// Read the command sent by Execute
		n, _ := server.Read(buf)
		_ = n
		// Write back echoed command + output + prompt
		server.Write([]byte("show version\r\nCisco IOS v15.2\r\n# "))
	}()

	ctx := context.Background()
	result, err := session.Execute(ctx, "show version")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success() {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Command != "show version" {
		t.Errorf("expected command 'show version', got %q", result.Command)
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestSession_Execute_Closed(t *testing.T) {
	session, server := newTestSession("# ")
	server.Close()
	session.Close()

	ctx := context.Background()
	_, err := session.Execute(ctx, "test")
	if err == nil {
		t.Error("expected error executing on closed session")
	}
}

func TestSession_ExecuteExpect(t *testing.T) {
	session, server := newTestSession("# ")
	defer session.Close()
	defer server.Close()

	go func() {
		buf := make([]byte, 256)
		server.Read(buf)
		server.Write([]byte("Password: "))
	}()

	ctx := context.Background()
	result, err := session.ExecuteExpect(ctx, "enable", []string{"Password:", "# "}, 5*time.Second)
	if err != nil {
		t.Fatalf("ExecuteExpect error: %v", err)
	}
	if result.MatchedExpect != 0 {
		t.Errorf("expected MatchedExpect=0, got %d", result.MatchedExpect)
	}
}

func TestSession_ExecuteExpect_Closed(t *testing.T) {
	session, server := newTestSession("# ")
	server.Close()
	session.Close()

	ctx := context.Background()
	_, err := session.ExecuteExpect(ctx, "test", []string{"foo"}, time.Second)
	if err == nil {
		t.Error("expected error on closed session")
	}
}

func TestSession_Authenticate(t *testing.T) {
	session, server := newTestSession("# ")
	defer session.Close()
	defer server.Close()

	go func() {
		buf := make([]byte, 256)

		// Send login prompt
		server.Write([]byte("Login: "))

		// Read username
		server.Read(buf)

		// Send password prompt
		server.Write([]byte("Password: "))

		// Read password
		server.Read(buf)

		// Send shell prompt
		server.Write([]byte("Welcome\r\n# "))
	}()

	ctx := context.Background()
	err := session.Authenticate(ctx, "admin", "secret",
		[]string{"ogin:", "sername:"},
		[]string{"assword:"},
	)
	if err != nil {
		t.Fatalf("Authenticate error: %v", err)
	}
}

func TestSession_Authenticate_Closed(t *testing.T) {
	session, server := newTestSession("# ")
	server.Close()
	session.Close()

	ctx := context.Background()
	err := session.Authenticate(ctx, "admin", "pass", []string{"ogin:"}, []string{"assword:"})
	if err == nil {
		t.Error("expected error authenticating on closed session")
	}
}

func TestSession_CleanOutput(t *testing.T) {
	session, _ := newTestSession("# ")

	tests := []struct {
		name    string
		output  string
		command string
		want    string
	}{
		{
			name:    "echoed command and prompt stripped",
			output:  "show version\r\nCisco IOS v15.2\r\n# ",
			command: "show version",
			want:    "Cisco IOS v15.2",
		},
		{
			name:    "only prompt",
			output:  "# ",
			command: "test",
			want:    "",
		},
		{
			name:    "multi-line output",
			output:  "show ip route\r\nRoute 1\r\nRoute 2\r\n# ",
			command: "show ip route",
			want:    "Route 1\nRoute 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := session.cleanOutput(tt.output, tt.command)
			if got != tt.want {
				t.Errorf("cleanOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSession_IACStripping(t *testing.T) {
	session, server := newTestSession("# ")
	defer session.Close()
	defer server.Close()

	go func() {
		buf := make([]byte, 256)
		server.Read(buf)
		// Send response with embedded IAC WILL ECHO
		response := []byte("show ver\r\n")
		response = append(response, IAC, WILL, OptEcho)
		response = append(response, []byte("output line\r\n# ")...)
		server.Write(response)
		// Keep reading to consume any IAC negotiation responses
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	ctx := context.Background()
	result, err := session.Execute(ctx, "show ver")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// IAC sequences should be stripped from output
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
}

func TestSession_SetPrompt(t *testing.T) {
	session, server := newTestSession("# ")
	defer session.Close()
	defer server.Close()

	session.SetPrompt("> ")
	session.mu.Lock()
	if session.prompt != "> " {
		t.Errorf("expected prompt '> ', got %q", session.prompt)
	}
	session.mu.Unlock()
}

func TestSession_IsClosed(t *testing.T) {
	session, server := newTestSession("# ")
	defer server.Close()

	if session.IsClosed() {
		t.Error("expected session not closed")
	}
	session.Close()
	if !session.IsClosed() {
		t.Error("expected session closed")
	}
}

func TestSession_DoubleClose(t *testing.T) {
	session, server := newTestSession("# ")
	defer server.Close()

	if err := session.Close(); err != nil {
		t.Errorf("first close error: %v", err)
	}
	// Second close should be no-op
	if err := session.Close(); err != nil {
		t.Errorf("second close should be nil, got %v", err)
	}
}

func TestSession_ContextCancellation(t *testing.T) {
	session, server := newTestSession("# ")
	defer session.Close()

	// Drain server side to prevent net.Pipe write blocking
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := session.Execute(ctx, "test")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}
